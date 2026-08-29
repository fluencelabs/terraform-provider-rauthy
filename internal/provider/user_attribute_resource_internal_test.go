package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// default_value is Optional and not Computed, so state after apply must equal
// the configuration exactly — but the configuration is a rendering of a JSON
// document and Rauthy re-serialises it its own way. defaultValueString keeps
// the configured spelling whenever it means the same thing.
func TestDefaultValueString(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		raw   string
		prior types.String
		want  types.String
	}{
		"nothing stored, nothing configured": {
			raw:   "",
			prior: types.StringNull(),
			want:  types.StringNull(),
		},
		"an explicit JSON null is no default": {
			raw:   "null",
			prior: types.StringNull(),
			want:  types.StringNull(),
		},
		"imported value takes the server spelling": {
			raw:   `{"code": 7}`,
			prior: types.StringNull(),
			want:  types.StringValue(`{"code":7}`),
		},
		"reformatting alone is not a change": {
			raw:   `{"code":7}`,
			prior: types.StringValue("{\n  \"code\": 7\n}"),
			want:  types.StringValue("{\n  \"code\": 7\n}"),
		},
		"a real change takes the server value": {
			raw:   `{"code":8}`,
			prior: types.StringValue(`{"code":7}`),
			want:  types.StringValue(`{"code":8}`),
		},
		"a dropped default becomes null": {
			raw:   "",
			prior: types.StringValue(`"engineering"`),
			want:  types.StringNull(),
		},
		"an unknown prior falls back to the server value": {
			raw:   `"engineering"`,
			prior: types.StringUnknown(),
			want:  types.StringValue(`"engineering"`),
		},
	}

	for name, tc := range cases {
		got := defaultValueString(json.RawMessage(tc.raw), tc.prior)
		if !got.Equal(tc.want) {
			t.Errorf("%s: defaultValueString(%q, %v) = %v, want %v", name, tc.raw, tc.prior, got, tc.want)
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
		DefaultValue: types.StringNull(),
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
