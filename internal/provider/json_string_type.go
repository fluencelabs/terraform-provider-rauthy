package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// jsonString is a string attribute holding a JSON document, where two spellings
// of the same document are not a change.
//
// It exists because Rauthy stores a user attribute's default as parsed JSON
// (`serde_json::Value`) and re-serialises it its own way, so the bytes that
// come back are almost never the bytes the configuration wrote. Without
// semantic equality every plan wants to rewrite the value and every apply is
// followed by a diff that cannot be closed.
//
// HashiCorp ships exactly this as jsontypes.Normalized in
// terraform-plugin-framework-jsontypes. It is not a dependency of this provider
// and pulling in a module for one attribute is not worth it — the whole of it
// is the sixty lines below — so the type is implemented here instead.
//
// Equality is structural rather than textual: both sides are decoded and
// compared as values, so reordered object keys and reformatted whitespace are
// both non-changes. That is stricter than the compaction this used to do, which
// could not see past key order.
type jsonStringType struct {
	basetypes.StringType
}

var (
	_ basetypes.StringTypable                    = jsonStringType{}
	_ basetypes.StringValuableWithSemanticEquals = jsonString{}
)

func (t jsonStringType) Equal(o attr.Type) bool {
	other, ok := o.(jsonStringType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t jsonStringType) String() string { return "provider.jsonStringType" }

func (t jsonStringType) ValueFromString(
	_ context.Context,
	in basetypes.StringValue,
) (basetypes.StringValuable, diag.Diagnostics) {
	return jsonString{StringValue: in}, nil
}

func (t jsonStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	value, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	str, ok := value.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T, expected basetypes.StringValue", value)
	}
	valuable, diags := t.ValueFromString(ctx, str)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to jsonString: %v", diags)
	}
	return valuable, nil
}

func (t jsonStringType) ValueType(context.Context) attr.Value { return jsonString{} }

// jsonString is the value type of jsonStringType.
type jsonString struct {
	basetypes.StringValue
}

func (v jsonString) Type(context.Context) attr.Type { return jsonStringType{} }

func (v jsonString) Equal(o attr.Value) bool {
	other, ok := o.(jsonString)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals reports whether the two values are the same JSON
// document. Anything that does not parse falls back to byte equality, so an
// invalid value still produces a diff rather than silently comparing equal —
// the resource rejects those at plan time anyway.
func (v jsonString) StringSemanticEquals(
	_ context.Context,
	newValuable basetypes.StringValuable,
) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	other, ok := newValuable.(jsonString)
	if !ok {
		diags.AddError(
			"Semantic equality check error",
			fmt.Sprintf("Expected provider.jsonString, got %T. This is a bug in the provider.", newValuable),
		)
		return false, diags
	}

	var lhs, rhs any
	left := json.NewDecoder(strings.NewReader(v.ValueString()))
	right := json.NewDecoder(strings.NewReader(other.ValueString()))
	left.UseNumber()
	right.UseNumber()
	if !json.Valid([]byte(v.ValueString())) || !json.Valid([]byte(other.ValueString())) || left.Decode(&lhs) != nil ||
		right.Decode(&rhs) != nil {
		return v.ValueString() == other.ValueString(), diags
	}
	return reflect.DeepEqual(rauthyJSONNumbers(lhs), rauthyJSONNumbers(rhs)), diags
}

func newJSONString(s string) jsonString {
	return jsonString{StringValue: basetypes.NewStringValue(s)}
}

func nullJSONString() jsonString {
	return jsonString{StringValue: basetypes.NewStringNull()}
}

// rauthyJSONNumbers preserves serde_json integers and rounds floating-point numbers
// as serde_json 1.0.151/1.0.154 do without arbitrary_precision or float_roundtrip.
func rauthyJSONNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		text := v.String()
		if !strings.ContainsAny(text, ".eE") {
			if _, err := strconv.ParseUint(text, 10, 64); err == nil {
				number, _ := new(big.Rat).SetString(text)
				return number
			}
			if _, err := strconv.ParseInt(text, 10, 64); err == nil {
				number, _ := new(big.Rat).SetString(text)
				return number
			}
		}
		if number := serdeJSONFloat(text); !math.IsInf(number, 0) {
			return new(big.Rat).SetFloat64(number)
		}
	case []any:
		for i, element := range v {
			v[i] = rauthyJSONNumbers(element)
		}
	case map[string]any:
		for key, element := range v {
			v[key] = rauthyJSONNumbers(element)
		}
	}
	return value
}

// serdeJSONFloat follows serde_json's parse_integer, parse_decimal and
// f64_from_parts: accumulate a u64 significand, discard overflowing digits,
// then scale the rounded significand by a single power of ten. ParseFloat
// alone differs because it rounds the complete decimal in one operation.
// The caller has already checked JSON syntax.
func serdeJSONFloat(text string) float64 {
	const maxPower = 308
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(text), "e")
	exponent := 0
	if hasExponent {
		parsed, err := strconv.ParseInt(exponentText, 10, 32)
		if err != nil {
			parsed = math.MaxInt32
			if strings.HasPrefix(exponentText, "-") {
				parsed = math.MinInt32
			}
		}
		exponent = int(parsed)
	}
	significand, adjustment := serdeJSONSignificand(mantissa)
	exponent += adjustment
	number := float64(significand)
	// serde_json scales subnormal values in stages to avoid an infinite divisor.
	for exponent < -maxPower && number != 0 {
		number /= 1e308
		exponent += maxPower
	}
	if number != 0 {
		switch {
		case exponent > maxPower:
			number = math.Inf(1)
		case exponent >= 0:
			number *= jsonPowerOfTen(exponent)
		default:
			number /= jsonPowerOfTen(-exponent)
		}
	}
	if negative {
		number = -number
	}
	return number
}

// serde_json's POW10 table contains correctly rounded decimal literals.
func jsonPowerOfTen(exponent int) float64 {
	number, _ := strconv.ParseFloat("1e"+strconv.Itoa(exponent), 64)
	return number
}

func serdeJSONSignificand(mantissa string) (uint64, int) {
	const radix = 10
	exponent := 0
	var significand uint64
	fraction, overflow := false, false
	for i := range mantissa {
		digit := mantissa[i]
		if digit == '.' {
			overflow = false
			fraction = true
			continue
		}
		value := uint64(digit - '0')
		if !overflow && significand > (math.MaxUint64-value)/radix {
			overflow = true
		}
		if overflow {
			if !fraction {
				exponent++
			}
			continue
		}
		significand = significand*radix + value
		if fraction {
			exponent--
		}
	}
	return significand, exponent
}
