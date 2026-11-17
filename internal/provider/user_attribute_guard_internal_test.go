package provider

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUserAttributeReadList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		attrs   []rauthy.UserAttrResponse
		present bool
	}{
		{name: "route missing", status: http.StatusNotFound},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "attribute missing", attrs: []rauthy.UserAttrResponse{{Name: "developer"}}},
		{name: "different case", attrs: []rauthy.UserAttrResponse{{Name: "Dev"}}},
		{name: "empty list", attrs: []rauthy.UserAttrResponse{}},
		{
			name:    "attribute present",
			attrs:   []rauthy.UserAttrResponse{{Name: "developer"}, {Name: "dev", UserEditable: true}},
			present: true,
		},
		{
			name:    "unnamed entry before target",
			attrs:   []rauthy.UserAttrResponse{{Name: ""}, {Name: "dev", UserEditable: true}},
			present: true,
		},
		{
			name:    "unnamed entry after target",
			attrs:   []rauthy.UserAttrResponse{{Name: "dev", UserEditable: true}, {Name: ""}},
			present: true,
		},
		{name: "only unnamed entry", attrs: []rauthy.UserAttrResponse{{Name: ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			r := &userAttributeResource{api: api}
			model := userAttributeResourceModel{
				ID: types.StringValue("dev"), Name: types.StringValue("dev"), UserEditable: types.BoolValue(false),
			}
			state := resourceState(t, r, model)
			want := emptyState(t, r)
			if tc.status != 0 {
				f.handle("GET /users/attr", func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "list unavailable", tc.status)
				})
				want = state
			} else {
				if tc.present {
					model.UserEditable = types.BoolValue(true)
					want = resourceState(t, r, model)
				}
				f.answer("GET /users/attr", map[string]any{"values": tc.attrs})
			}
			resp := resource.ReadResponse{State: state}
			r.Read(t.Context(), resource.ReadRequest{State: state}, &resp)
			if !resp.State.Raw.Equal(want.Raw) {
				t.Errorf("state = %v, want %v", resp.State.Raw, want.Raw)
			}
			if resp.Diagnostics.HasError() != (tc.status != 0) {
				t.Fatalf("unexpected diagnostics for status %d: %v", tc.status, resp.Diagnostics)
			}
			if tc.status != 0 && !strings.Contains(resp.Diagnostics[0].Detail(), strconv.Itoa(tc.status)) {
				t.Errorf("list API error lost status %d: %v", tc.status, resp.Diagnostics)
			}
		})
	}
}

func TestUserAttributeReadUsesSuccessfulRetry(t *testing.T) {
	f, api := newFakeRauthy(t)
	const route = "GET /users/attr"
	f.handle(route, func(w http.ResponseWriter, _ *http.Request) {
		if len(f.bodies(route)) == 1 {
			http.Error(w, `{"values":[{"name":"dev","user_editable":false}]}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"values":[{"name":"dev","user_editable":true}]}`)
	})
	r := &userAttributeResource{api: api}
	model := userAttributeResourceModel{
		ID: types.StringValue("dev"), Name: types.StringValue("dev"), UserEditable: types.BoolValue(false),
	}
	state := resourceState(t, r, model)
	model.UserEditable = types.BoolValue(true)
	want := resourceState(t, r, model)
	resp := resource.ReadResponse{State: state}
	r.Read(t.Context(), resource.ReadRequest{State: state}, &resp)
	if len(f.bodies(route)) != 2 || resp.Diagnostics.HasError() || !resp.State.Raw.Equal(want.Raw) {
		t.Fatalf(
			"expected refresh to use the successful retry: state=%v diagnostics=%v",
			resp.State.Raw,
			resp.Diagnostics,
		)
	}
}

func TestUserAttributeReadRefusesRedirect(t *testing.T) {
	f, api := newFakeRauthy(t)
	const target = "/redirect-target"
	f.handle("GET /users/attr", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, rauthy.APIBasePath+target, http.StatusTemporaryRedirect)
	})
	f.answer("GET "+target, map[string]any{"values": []rauthy.UserAttrResponse{}})

	r := &userAttributeResource{api: api}
	state := resourceState(t, r, userAttributeResourceModel{
		ID: types.StringValue("dev"), Name: types.StringValue("dev"),
	})
	resp := resource.ReadResponse{State: state}
	r.Read(t.Context(), resource.ReadRequest{State: state}, &resp)
	if len(f.bodies("GET "+target)) != 0 {
		t.Error("followed the HTTP redirect")
	}
	origin := strings.TrimSuffix(api.BaseURL().Reveal(), rauthy.APIBasePath)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics[0].Detail(), "307") ||
		!strings.Contains(resp.Diagnostics[0].Detail(), origin) || !resp.State.Raw.Equal(state.Raw) {
		t.Fatalf("expected a 307 diagnostic with the destination origin and preserved state: %v", resp.Diagnostics)
	}
}

func TestUserAttributeDataSourceDistinguishesMissingFromListErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		detail string
	}{
		{"attribute missing", http.StatusOK, "No user attribute with that name"},
		{"route missing", http.StatusNotFound, "GET /users/attr: HTTP 404: proxy route missing"},
		{"no content", http.StatusNoContent, "GET /users/attr returned HTTP 204"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			if tc.status == http.StatusOK {
				f.answer("GET /users/attr", map[string]any{"values": []rauthy.UserAttrResponse{{Name: "Dev"}}})
			} else {
				f.handle("GET /users/attr", func(w http.ResponseWriter, _ *http.Request) {
					if tc.status == http.StatusNotFound {
						http.Error(w, "proxy route missing", tc.status)
						return
					}
					w.WriteHeader(tc.status)
				})
			}
			d := &userAttributeDataSource{api: api}
			config := dataSourceConfig(t, d, userAttributeDataSourceModel{Name: types.StringValue("dev")})
			resp := datasource.ReadResponse{}
			d.Read(t.Context(), datasource.ReadRequest{Config: config}, &resp)
			if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics[0].Detail(), tc.detail) {
				t.Fatalf("expected diagnostic containing %q: %v", tc.detail, resp.Diagnostics)
			}
		})
	}
}

func TestUserAttributeUpdateReportsInvalidReadback(t *testing.T) {
	f, api := newFakeRauthy(t)
	f.answer("PUT /users/attr/dev", nil)
	f.handle("GET /users/attr", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r := &userAttributeResource{api: api}
	model := userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("dev")}
	state := resourceState(t, r, model)
	resp := resource.UpdateResponse{State: state}
	r.Update(t.Context(), resource.UpdateRequest{State: state, Plan: resourcePlan(t, r, model)}, &resp)
	if !resp.Diagnostics.HasError() ||
		!strings.Contains(resp.Diagnostics[0].Detail(), "GET /users/attr returned HTTP 204") {
		t.Fatalf("expected the invalid list response in the readback diagnostic: %v", resp.Diagnostics)
	}
}

func TestUserAttributeMalformedList(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing values", `{}`, http.StatusOK},
		{"null values", `{"values":null}`, http.StatusOK},
		{"null entry", `{"values":[null]}`, http.StatusOK},
		{"null entry with target", `{"values":[{"name":"dev"},null]}`, http.StatusOK},
		{"invalid values type", `{"values":42}`, http.StatusOK},
		{"no content", "", http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.handle("GET /users/attr", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			r := &userAttributeResource{api: api}
			ctx := t.Context()
			state := resourceState(t, r, userAttributeResourceModel{
				ID: types.StringValue("dev"), Name: types.StringValue("dev"),
			})

			read := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &read)
			operation := "GET /users/attr returned HTTP " + strconv.Itoa(tc.status)
			if !read.Diagnostics.HasError() || !strings.Contains(read.Diagnostics[0].Detail(), "values") ||
				!strings.Contains(read.Diagnostics[0].Detail(), operation) ||
				!read.State.Raw.Equal(state.Raw) {
				t.Errorf(
					"Read must report an invalid list and preserve state: %v, %v",
					read.Diagnostics,
					read.State.Raw,
				)
			}

			deletion := resource.DeleteResponse{State: state}
			r.Delete(ctx, resource.DeleteRequest{State: state}, &deletion)
			if !deletion.Diagnostics.HasError() || !strings.Contains(deletion.Diagnostics[0].Detail(), "values") ||
				!strings.Contains(deletion.Diagnostics[0].Detail(), operation) ||
				!deletion.State.Raw.Equal(state.Raw) || len(f.bodies("DELETE /users/attr/dev")) != 0 {
				t.Errorf(
					"Delete must report an invalid list, preserve state, and skip DELETE: %v",
					deletion.Diagnostics,
				)
			}
		})
	}
}

type attributeGuardCase struct {
	name, other, failure string
	missing, wantError   bool
	status               int
}

func TestUserAttributeDeletionGuard(t *testing.T) {
	for _, tc := range []attributeGuardCase{
		{name: "prefix", other: "developer", wantError: true},
		{name: "suffix", other: "xdev", wantError: true},
		{name: "interior", other: "xdevx", wantError: true},
		{name: "exact mapping", other: "dev"},
		{name: "unrelated", other: "department"},
		{name: "missing", other: "developer", missing: true, wantError: true},
		{name: "attribute list 404", failure: "/users/attr", status: http.StatusNotFound, wantError: true},
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
		scope.AttrIncludeAccess = []string{"department", tc.other}
	} else {
		scope.AttrIncludeID = []string{"department", tc.other}
	}
	f.answer("GET /scopes", []rauthy.ScopeResponse{scope})
	if tc.failure != "" {
		f.handle(
			"GET "+tc.failure,
			func(w http.ResponseWriter, _ *http.Request) {
				status := tc.status
				if status == 0 {
					status = http.StatusForbidden
				}
				http.Error(w, "unavailable", status)
			},
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
	resp := resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() != tc.wantError {
		t.Fatalf("delete: %v", resp.Diagnostics)
	}
	if (len(f.bodies("DELETE /users/attr/dev")) == 0) != (tc.wantError || tc.missing) {
		t.Fatal("unexpected DELETE count")
	}
	if tc.wantError && !resp.State.Raw.Equal(state.Raw) {
		t.Fatal("failed deletion changed state")
	}
	if tc.missing && !strings.Contains(resp.Diagnostics[0].Detail(), "refresh") {
		t.Fatalf("missing attribute diagnostic lacks recovery guidance: %v", resp.Diagnostics)
	}
	if tc.status == http.StatusNotFound && !strings.Contains(resp.Diagnostics[0].Detail(), "404") {
		t.Fatalf("attribute list failure lost API status: %v", resp.Diagnostics)
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
		wantError := unsafe && (op == "destroy" || op == "replace")
		var wantReplace path.Paths
		if op == "replace" {
			wantReplace = path.Paths{path.Root("name")}
		}
		if !slices.EqualFunc(resp.RequiresReplace, wantReplace, path.Path.Equal) {
			t.Errorf("%s plan: replacement paths %v, want %v", op, resp.RequiresReplace, wantReplace)
		}
		if resp.Diagnostics.HasError() != wantError {
			t.Errorf("%s plan: %v, wantError=%v", op, resp.Diagnostics, wantError)
		}
	}
}

func TestUserAttributeUpdateRejectsResolvedRename(t *testing.T) {
	_, api := newFakeRauthy(t)
	r := &userAttributeResource{api: api}
	prior := userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("dev")}
	state := resourceState(t, r, prior)
	prior.Name = types.StringValue("developer")
	plan := resourcePlan(t, r, prior)
	resp := resource.UpdateResponse{State: state}
	r.Update(t.Context(), resource.UpdateRequest{State: state, Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "User attribute name requires replacement" {
		t.Fatalf("expected a replacement diagnostic before any API call, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(state.Raw) {
		t.Fatal("rejected update changed state")
	}
}
