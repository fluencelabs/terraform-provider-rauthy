package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func guardedResource(kind string, api *rauthy.Client, name types.String) (resource.ResourceWithModifyPlan, any) {
	id := types.StringValue("target")
	switch kind {
	case "role":
		return &roleResource{api: api}, roleResourceModel{ID: id, Name: name}
	case "group":
		return &groupResource{api: api}, groupResourceModel{ID: id, Name: name}
	default:
		return &scopeResource{api: api}, scopeResourceModel{
			ID: id, Name: name, AttrIncludeAccess: types.SetNull(types.StringType),
			AttrIncludeID: types.SetNull(types.StringType), ClaimsAtRoot: types.BoolValue(false),
		}
	}
}

func TestSubstringGuardDeletion(t *testing.T) {
	for _, kind := range []string{"role", "group", "scope"} {
		t.Run(kind, func(t *testing.T) {
			testSubstringGuardDeletion(t, kind)
		})
	}
}

func testSubstringGuardDeletion(t *testing.T, kind string) {
	t.Helper()
	ctx := context.Background()
	for _, tc := range []struct {
		name, actual, other           string
		missing, readError, wantError bool
	}{
		{name: "prefix with stale state", actual: "dev", other: "developer", wantError: true},
		{name: "suffix with stale state", actual: "dev", other: "xdev", wantError: true},
		{name: "safe current name despite stale collision", actual: "unique", other: "stale-extra"},
		{name: "interior substring is safe", actual: "dev", other: "xdevx"},
		{name: "missing target", other: "developer", missing: true},
		{name: "read failure", readError: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			route := "/" + kind + "s"
			entries := []map[string]string{{"id": "other", "name": tc.other}}
			if !tc.missing {
				entries = append(entries, map[string]string{"id": "target", "name": tc.actual})
			}
			f.answer("GET "+route, entries)
			if tc.readError {
				f.handle(
					"GET "+route,
					func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unavailable", http.StatusForbidden) },
				)
			}
			f.answer("DELETE "+route+"/target", nil)
			r, model := guardedResource(kind, api, types.StringValue("stale"))
			state := resourceState(t, r, model)
			var planResp resource.ModifyPlanResponse
			r.ModifyPlan(
				ctx,
				resource.ModifyPlanRequest{State: state, Plan: tfsdk.Plan(emptyState(t, r))},
				&planResp,
			)
			if planResp.Diagnostics.HasError() != tc.wantError {
				t.Errorf("destroy plan: %v", planResp.Diagnostics)
			}
			var deleteResp resource.DeleteResponse
			r.Delete(ctx, resource.DeleteRequest{State: state}, &deleteResp)
			if deleteResp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("delete: %v", deleteResp.Diagnostics)
			}
			writes := len(f.bodies("DELETE " + route + "/target"))
			if (writes == 0) != tc.wantError {
				t.Fatalf("delete calls=%d, wantError=%v", writes, tc.wantError)
			}
		})
	}
}

func TestSubstringGuardScopeRename(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, current, planned, other string
		wantError                     bool
	}{
		{"stale state unchanged in plan", "ro", "stale", "profile", true},
		{"stale state changed in plan", "ro", "read", "profile", true},
		{"safe rename", "unique", "read", "profile", false},
		{"mapping only", "ro", "ro", "profile", false},
		{"openid recovery", "pen", "read", "openid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.answer(
				"GET /scopes",
				[]rauthy.ScopeResponse{{ID: "target", Name: tc.current}, {ID: "other", Name: tc.other}},
			)
			f.answer("PUT /scopes/target", rauthy.ScopeResponse{ID: "target", Name: tc.planned})
			stateName := "stale"
			if tc.other == "openid" {
				stateName = tc.current
			}
			r, old := guardedResource("scope", api, types.StringValue(stateName))
			_, planned := guardedResource("scope", api, types.StringValue(tc.planned))
			state, plan := resourceState(t, r, old), resourcePlan(t, r, planned)
			var pr resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, &pr)
			if pr.Diagnostics.HasError() != tc.wantError {
				t.Errorf("plan: %v", pr.Diagnostics)
			}
			if tc.other == "openid" {
				checkOpenIDAdvice(t, pr.Diagnostics)
			}
			ur := resource.UpdateResponse{State: emptyState(t, r)}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &ur)
			if ur.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("update: %v", ur.Diagnostics)
			}
			if (len(f.bodies("PUT /scopes/target")) == 0) != tc.wantError {
				t.Fatal("unexpected PUT count")
			}
		})
	}
}

func TestSubstringGuardRoleGroupCurrentName(t *testing.T) {
	for _, kind := range []string{"role", "group"} {
		t.Run(kind, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.answer("GET /"+kind+"s", []map[string]string{{"id": "target", "name": "current"}})
			r, old := guardedResource(kind, api, types.StringValue("stale"))
			for _, name := range []string{"stale", "current"} {
				_, planned := guardedResource(kind, api, types.StringValue(name))
				var resp resource.ModifyPlanResponse
				r.ModifyPlan(
					context.Background(),
					resource.ModifyPlanRequest{State: resourceState(t, r, old), Plan: resourcePlan(t, r, planned)},
					&resp,
				)
				if resp.Diagnostics.HasError() != (name == "stale") {
					t.Fatalf("name=%s: %v", name, resp.Diagnostics)
				}
			}
		})
	}
}

func checkOpenIDAdvice(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	for _, d := range diags {
		if !strings.Contains(d.Detail(), "cannot be removed or renamed") ||
			strings.Contains(d.Detail(), `Remove or rename "openid"`) {
			t.Fatalf("advice: %s", d.Detail())
		}
	}
}
