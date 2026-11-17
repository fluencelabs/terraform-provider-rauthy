package provider

import (
	"encoding/json"
	"strconv"
	"strings"
)

// unstableJSONNumber returns the first unsupported numeric literal in valid JSON,
// including numbers nested in objects or arrays. Strings containing digits are data.
func unstableJSONNumber(document string) string {
	decoder := json.NewDecoder(strings.NewReader(document))
	decoder.UseNumber()
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		if number, ok := token.(json.Number); ok && !stableJSONNumber(number.String()) {
			return number.String()
		}
	}
}

// stableJSONNumber conservatively bounds numbers before serde_json's two
// parse/serialize passes. Integer literals retain the full i64/u64 range.
// Floating-point literals have at most 15 significant mantissa digits and a
// decimal scale within [-15,15]. Their significand and power-of-ten operands
// are exactly representable in f64, avoiding serde_json's lossy accumulation.
func stableJSONNumber(text string) bool {
	if !strings.ContainsAny(text, ".eE") {
		_, signedErr := strconv.ParseInt(text, 10, 64)
		_, unsignedErr := strconv.ParseUint(text, 10, 64)
		return signedErr == nil || unsignedErr == nil
	}
	const maxDigits = 15
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(text), "e")
	exponent := int64(0)
	if hasExponent {
		parsed, err := strconv.ParseInt(exponentText, 10, 32)
		if err != nil {
			return false
		}
		exponent = parsed
	}
	integer, fraction, _ := strings.Cut(strings.TrimPrefix(mantissa, "-"), ".")
	digits := strings.TrimLeft(integer+fraction, "0")
	scale := exponent - int64(len(fraction))
	return len(digits) <= maxDigits && scale >= -maxDigits && scale <= maxDigits
}
