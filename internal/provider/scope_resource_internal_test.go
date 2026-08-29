package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Rauthy answers with null for an empty mapping and for an absent one alike.
// These attributes are Optional and not Computed, so state after apply must
// equal the configuration exactly: an explicit `[]` that came back as null
// would abort the apply. attrSet keeps the two apart.
func TestAttrSet_KeepsEmptyDistinctFromUnset(t *testing.T) {
	t.Parallel()

	emptySet := types.SetValueMust(types.StringType, nil)

	cases := map[string]struct {
		values []string
		prior  types.Set
		want   types.Set
	}{
		"server has values": {
			values: []string{"department"},
			prior:  types.SetNull(types.StringType),
			want:   setOf(t, "department"),
		},
		"nothing on either side": {
			values: nil,
			prior:  types.SetNull(types.StringType),
			want:   types.SetNull(types.StringType),
		},
		"config wrote an empty set": {
			values: nil,
			prior:  emptySet,
			want:   emptySet,
		},
		"config dropped a mapping it used to have": {
			values: nil,
			prior:  setOf(t, "department"),
			want:   types.SetNull(types.StringType),
		},
		"unknown prior falls back to null": {
			values: nil,
			prior:  types.SetUnknown(types.StringType),
			want:   types.SetNull(types.StringType),
		},
	}

	for name, tc := range cases {
		if got := attrSet(tc.values, tc.prior); !got.Equal(tc.want) {
			t.Errorf("%s: attrSet(%v, %v) = %v, want %v", name, tc.values, tc.prior, got, tc.want)
		}
	}
}

// applyScope reads the model's own values before overwriting them, so an empty
// mapping in the plan survives a response that reports nothing.
func TestApplyScope_PreservesAnEmptyMappingFromThePlan(t *testing.T) {
	t.Parallel()

	emptySet := types.SetValueMust(types.StringType, nil)
	plan := scopeResourceModel{
		Name:              types.StringValue("read:billing"),
		AttrIncludeAccess: emptySet,
		AttrIncludeID:     types.SetNull(types.StringType),
	}

	applyScope(&plan, &client.ScopeResponse{ID: "scope-1", Name: "read:billing"})

	if plan.ID.ValueString() != "scope-1" {
		t.Errorf("id = %q, want scope-1", plan.ID.ValueString())
	}
	if !plan.AttrIncludeAccess.Equal(emptySet) {
		t.Errorf("attr_include_access = %v, want the empty set the plan carried", plan.AttrIncludeAccess)
	}
	if !plan.AttrIncludeID.IsNull() {
		t.Errorf("attr_include_id = %v, want null", plan.AttrIncludeID)
	}
}

// The decoded mapping lands in the set attribute.
func TestApplyScope_SplitsTheJoinedMapping(t *testing.T) {
	t.Parallel()

	model := scopeResourceModel{}
	applyScope(&model, &client.ScopeResponse{
		ID:                "scope-1",
		Name:              "read:billing",
		AttrIncludeAccess: client.AttrList{"department", "cost_center"},
	})

	if want := setOf(t, "department", "cost_center"); !model.AttrIncludeAccess.Equal(want) {
		t.Errorf("attr_include_access = %v, want %v", model.AttrIncludeAccess, want)
	}
}
