package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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
	if json.Unmarshal([]byte(v.ValueString()), &lhs) != nil ||
		json.Unmarshal([]byte(other.ValueString()), &rhs) != nil {
		return v.ValueString() == other.ValueString(), diags
	}
	return reflect.DeepEqual(lhs, rhs), diags
}

func newJSONString(s string) jsonString {
	return jsonString{StringValue: basetypes.NewStringValue(s)}
}

func nullJSONString() jsonString {
	return jsonString{StringValue: basetypes.NewStringNull()}
}
