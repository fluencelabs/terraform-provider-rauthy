package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type attributeGuardCase struct {
	name, other, failure string
	missing, wantError   bool
}

func TestUserAttributeDeletionGuard(t *testing.T) {
	for _, tc := range []attributeGuardCase{
		{name: "prefix", other: "developer", wantError: true},
		{name: "suffix", other: "xdev", wantError: true},
		{name: "interior", other: "xdevx", wantError: true},
		{name: "exact mapping", other: "dev"},
		{name: "unrelated", other: "department"},
		{name: "missing", other: "developer", missing: true},
		{name: "attribute read error", failure: "/users/attr", wantError: true},
		{name: "scope read error", failure: "/scopes", wantError: true},
	} {
		for _, mapping := range []string{"access", "id"} {
			t.Run(tc.name+"/"+mapping, func(t *testing.T) {
				testUserAttributeDeletionGuard(t, tc, mapping)
			})
		}
	}
}

func testUserAttributeDeletionGuard(t *testing.T, tc attributeGuardCase, mapping string) {
	t.Helper()
	f, api := newFakeRauthy(t)
	attrs := []rauthy.UserAttrResponse{}
	if !tc.missing {
		attrs = append(attrs, rauthy.UserAttrResponse{Name: "dev"})
	}
	f.answer("GET /users/attr", map[string]any{"values": attrs})
	scope := rauthy.ScopeResponse{ID: "scope", Name: "mapped"}
	if mapping == "access" {
		scope.AttrIncludeAccess = []string{tc.other}
	} else {
		scope.AttrIncludeID = []string{tc.other}
	}
	f.answer("GET /scopes", []rauthy.ScopeResponse{scope})
	if tc.failure != "" {
		f.handle(
			"GET "+tc.failure,
			func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unavailable", http.StatusForbidden) },
		)
	}
	f.answer("DELETE /users/attr/dev", nil)
	r := &userAttributeResource{api: api}
	// The key used by DELETE is authoritative even when the name in state is stale.
	model := userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("stale")}
	state := resourceState(t, r, model)
	modifier, ok := any(r).(resource.ResourceWithModifyPlan)
	if !ok {
		t.Fatal("user attribute must guard deletion during planning")
	}
	checkAttributeGuardPlans(t, modifier, model, tc.wantError)
	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() != tc.wantError {
		t.Fatalf("delete: %v", resp.Diagnostics)
	}
	if (len(f.bodies("DELETE /users/attr/dev")) == 0) != tc.wantError {
		t.Fatal("unexpected DELETE count")
	}
}

func checkAttributeGuardPlans(
	t *testing.T,
	r resource.ResourceWithModifyPlan,
	model userAttributeResourceModel,
	unsafe bool,
) {
	t.Helper()
	state := resourceState(t, r, model)
	for _, op := range []string{"destroy", "replace", "unknown", "unchanged", "create"} {
		plan := tfsdk.Plan(emptyState(t, r))
		prior := state
		if op != "destroy" {
			model.Name = types.StringValue("replacement")
			if op == "unknown" {
				model.Name = types.StringUnknown()
			}
			if op == "unchanged" {
				model.Name = types.StringValue("dev")
			}
			plan = resourcePlan(t, r, model)
		}
		if op == "create" {
			prior = emptyState(t, r)
		}
		var resp resource.ModifyPlanResponse
		r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{State: prior, Plan: plan}, &resp)
		wantError := unsafe && op != "unchanged" && op != "create"
		if resp.Diagnostics.HasError() != wantError {
			t.Errorf("%s plan: %v, wantError=%v", op, resp.Diagnostics, wantError)
		}
	}
}
