package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Rauthy stores the default as parsed JSON and re-serialises it its own way, so
// the bytes coming back are rarely the bytes the configuration wrote. Semantic
// equality is what stops that from being a perpetual diff.
func TestJSONString_SemanticEquals(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		a, b string
		want bool
	}{
		"identical":               {`{"a":1}`, `{"a":1}`, true},
		"whitespace only":         {"{\n  \"a\": 1\n}", `{"a":1}`, true},
		"reordered object keys":   {`{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		"different value":         {`{"a":1}`, `{"a":2}`, false},
		"array order is not free": {`[1,2]`, `[2,1]`, false},
		"scalar string":           {`"engineering"`, `  "engineering" `, true},
		// Neither side parses, so this falls back to byte equality rather than
		// silently calling two broken values equal.
		"both unparseable and equal":   {`not json`, `not json`, true},
		"both unparseable and unequal": {`not json`, `also not json`, false},
	}

	for name, tc := range cases {
		got, diags := newJSONString(tc.a).StringSemanticEquals(context.Background(), newJSONString(tc.b))
		if diags.HasError() {
			t.Fatalf("%s: unexpected diagnostics: %v", name, diags)
		}
		if got != tc.want {
			t.Errorf("%s: %q == %q is %v, want %v", name, tc.a, tc.b, got, tc.want)
		}
	}
}

// The mapping side stays simple now that equality is semantic: it renders
// Rauthy's own spelling and lets the framework put the configured one back.
func TestDefaultValueString(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		raw  string
		want jsonString
	}{
		"nothing stored":          {"", nullJSONString()},
		"an explicit JSON null":   {"null", nullJSONString()},
		"whitespace is stripped":  {`{"code": 7}`, newJSONString(`{"code":7}`)},
		"a scalar string default": {`"engineering"`, newJSONString(`"engineering"`)},
	}

	for name, tc := range cases {
		if got := defaultValueString(json.RawMessage(tc.raw)); !got.Equal(tc.want) {
			t.Errorf("%s: defaultValueString(%q) = %v, want %v", name, tc.raw, got, tc.want)
		}
	}
}

// The whole point of keeping the default as raw bytes: a non-string document
// has to survive the round trip unflattened.
func TestApplyUserAttr_KeepsANonStringDefault(t *testing.T) {
	t.Parallel()

	model := userAttributeResourceModel{}
	applyUserAttr(&model, &client.UserAttrResponse{
		Name:         "department",
		DefaultValue: json.RawMessage(`{"code": 7, "label": "eng"}`),
		UserEditable: true,
	})

	if model.ID.ValueString() != "department" || model.Name.ValueString() != "department" {
		t.Errorf("id/name = %q/%q, want department", model.ID.ValueString(), model.Name.ValueString())
	}
	if want := `{"code":7,"label":"eng"}`; model.DefaultValue.ValueString() != want {
		t.Errorf("default_value = %q, want %q", model.DefaultValue.ValueString(), want)
	}
	if !model.UserEditable.ValueBool() {
		t.Error("user_editable did not survive")
	}
	if !model.Desc.IsNull() {
		t.Errorf("desc = %v, want null", model.Desc)
	}
}

// A null default must be omitted from the body rather than sent as JSON null:
// Rauthy would store the null as the default rather than treating it as absent.
func TestBuildUserAttrRequest_OmitsAnAbsentDefault(t *testing.T) {
	t.Parallel()

	body := buildUserAttrRequest(&userAttributeResourceModel{
		Name:         types.StringValue("department"),
		Desc:         types.StringNull(),
		DefaultValue: nullJSONString(),
		UserEditable: types.BoolValue(true),
	})

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(raw), `{"name":"department","user_editable":true}`; got != want {
		t.Errorf("request body = %s, want %s", got, want)
	}
}
