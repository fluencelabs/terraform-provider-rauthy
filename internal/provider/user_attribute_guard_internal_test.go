package provider

import (
	"context"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

func TestUserAttributeListAcceptsRustFields(t *testing.T) {
	body := []byte(`{"values":[
		{"name":"","user_editable":false},
		{"name":"Dev","desc":null,"default_value":null,"typ":null,"user_editable":true},
		{"name":"dev","desc":"department","default_value":{"quota":9007199254740993},"typ":"Email","user_editable":false}
	]}`)
	attr, found, err := decodeUserAttrList(body, "dev")
	if err != nil || !found || attr.Name != "dev" || attr.UserEditable ||
		attr.Desc == nil || *attr.Desc != "department" || attr.Typ == nil || *attr.Typ != "Email" ||
		string(attr.DefaultValue) != `{"quota":9007199254740993}` {
		t.Fatalf(
			"expected the exact dev entry with its optional fields and JSON default: %s, %v",
			attr.DefaultValue,
			err,
		)
	}
}

func TestUserAttributeValidListReadback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs []rauthy.UserAttrResponse
	}{
		{name: "false", attrs: []rauthy.UserAttrResponse{{Name: "dev", DefaultValue: []byte("42")}}},
		{name: "true", attrs: []rauthy.UserAttrResponse{{Name: "dev", DefaultValue: []byte("42"), UserEditable: true}}},
		{name: "omitted", attrs: []rauthy.UserAttrResponse{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.answer("PUT /users/attr/dev", nil)
			f.answer("GET /users/attr", map[string]any{"values": tc.attrs})
			r := &userAttributeResource{api: api}
			ctx := t.Context()
			prior := userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("dev")}
			state := resourceState(t, r, prior)
			plan := prior
			plan.DefaultValue = newJSONString("24")
			present := len(tc.attrs) != 0
			if present {
				plan.UserEditable = types.BoolValue(!tc.attrs[0].UserEditable)
			}
			update := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: resourcePlan(t, r, plan)}, &update)
			if !present {
				if !update.Diagnostics.HasError() || !update.State.Raw.Equal(resourceState(t, r, plan).Raw) {
					t.Fatalf("valid list omitting dev must diagnose the miss and retain the applied plan: %v, %v",
						update.Diagnostics, update.State.Raw)
				}
				return
			}
			want := plan
			want.UserEditable = types.BoolValue(tc.attrs[0].UserEditable)
			want.DefaultValue = newJSONString("42")
			if update.Diagnostics.HasError() || !update.State.Raw.Equal(resourceState(t, r, want).Raw) {
				t.Errorf("readback must publish the returned values: %v, %v", update.Diagnostics, update.State.Raw)
			}

			d := &userAttributeDataSource{api: api}
			config := dataSourceConfig(t, d, userAttributeDataSourceModel{Name: prior.Name})
			data := datasource.ReadResponse{State: tfsdk.State(config)}
			d.Read(ctx, datasource.ReadRequest{Config: config}, &data)
			var got userAttributeDataSourceModel
			if diags := data.State.Get(ctx, &got); data.Diagnostics.HasError() || diags.HasError() ||
				!got.UserEditable.Equal(want.UserEditable) || got.DefaultValue.ValueString() != "42" {
				t.Errorf("data source must publish the returned values: %v, %v", data.Diagnostics, got)
			}
		})
	}
}

func TestUserAttributeMalformedList(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		headers    http.Header
		detail     string
	}{
		{name: "missing values", detail: "values", body: `{}`, status: http.StatusOK},
		{name: "null values", detail: "values", body: `{"values":null}`, status: http.StatusOK},
		{name: "null entry", detail: "values", body: `{"values":[null]}`, status: http.StatusOK},
		{name: "null entry with target", detail: "values", body: `{"values":[{"name":"dev","user_editable":true},null]}`, status: http.StatusOK},
		{name: "invalid values type", detail: "values", body: `{"values":42}`, status: http.StatusOK},
		{name: "no content", detail: "values", status: http.StatusNoContent},
		{name: "partial content", body: `{"values":[]}`, status: http.StatusPartialContent},
		{name: "empty body", status: http.StatusOK},
		{name: "null envelope", body: `null`, status: http.StatusOK},
		{name: "array envelope", body: `[]`, status: http.StatusOK},
		{name: "scalar entry", body: `{"values":[true]}`, status: http.StatusOK},
		{name: "missing name", body: `{"values":[{"user_editable":true}]}`, status: http.StatusOK},
		{name: "empty entry", body: `{"values":[{}]}`, status: http.StatusOK},
		{name: "null name", body: `{"values":[{"name":null,"user_editable":true}]}`, status: http.StatusOK},
		{name: "numeric name", body: `{"values":[{"name":3,"user_editable":true}]}`, status: http.StatusOK},
		{name: "wrong field case", body: `{"values":[{"Name":"dev","user_editable":true}]}`, status: http.StatusOK},
		{name: "missing editable", body: `{"values":[{"name":"dev"}]}`, status: http.StatusOK},
		{name: "null editable", body: `{"values":[{"name":"dev","user_editable":null}]}`, status: http.StatusOK},
		{name: "string editable", body: `{"values":[{"name":"dev","user_editable":"true"}]}`, status: http.StatusOK},
		{name: "invalid desc", body: `{"values":[{"name":"dev","user_editable":true,"desc":false}]}`, status: http.StatusOK},
		{name: "invalid typ", body: `{"values":[{"name":"dev","user_editable":true,"typ":false}]}`, status: http.StatusOK},
		{name: "unknown typ", body: `{"values":[{"name":"dev","user_editable":true,"typ":"email"}]}`, status: http.StatusOK},
		{name: "unknown field", body: `{"values":[{"name":"dev","user_editable":true,"extra":0}]}`, status: http.StatusOK},
		{name: "duplicate names", body: `{"values":[{"name":"other","user_editable":true},{"name":"other","user_editable":false}]}`, status: http.StatusOK},
		{name: "duplicate target", body: `{"values":[{"name":"dev","user_editable":true},{"name":"dev","user_editable":false}]}`, status: http.StatusOK},
		{name: "duplicate empty names", body: `{"values":[{"name":"","user_editable":true},{"name":"","user_editable":false}]}`, status: http.StatusOK},
		{name: "duplicate fields", body: `{"values":[{"name":"dev","name":"other","user_editable":true}]}`, status: http.StatusOK},
		{name: "truncated 200", detail: "unexpected EOF", body: `{"values":`, status: http.StatusOK, headers: http.Header{"Content-Length": {"100"}}},
		{name: "truncated 404", detail: "unexpected EOF", body: `{"error":`, status: http.StatusNotFound, headers: http.Header{"Content-Length": {"100"}}},
		{name: "truncated 403", detail: "unexpected EOF", body: `{"error":`, status: http.StatusForbidden, headers: http.Header{"Content-Length": {"100"}}},
		{name: "invalid redirect location", detail: "Location header does not parse", status: http.StatusTemporaryRedirect, headers: http.Header{"Location": {"%"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkMalformedUserAttributeList(t, tc.status, tc.body, tc.headers, tc.detail)
		})
	}
}

func checkMalformedUserAttributeList(t *testing.T, status int, body string, headers http.Header, detail string) {
	t.Helper()
	f, api := newFakeRauthy(t)
	f.handle("GET /users/attr", func(w http.ResponseWriter, _ *http.Request) {
		maps.Copy(w.Header(), headers)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	f.answer("PUT /users/attr/dev", nil)
	r := &userAttributeResource{api: api}
	ctx := t.Context()
	model := userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("dev")}
	state := resourceState(t, r, model)
	check := func(operation string, diags diag.Diagnostics, got, want tfsdk.State) {
		t.Helper()
		if !diags.HasError() ||
			!strings.Contains(diags[0].Detail(), "GET /users/attr returned HTTP "+strconv.Itoa(status)) ||
			!strings.Contains(diags[0].Detail(), detail) ||
			!got.Raw.Equal(want.Raw) {
			t.Errorf("%s must report the list HTTP status and preserve state: %v, %v", operation, diags, got.Raw)
		}
	}
	read := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &read)
	check("read", read.Diagnostics, read.State, state)

	deletion := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deletion)
	check("delete", deletion.Diagnostics, deletion.State, state)
	if len(f.bodies("DELETE /users/attr/dev")) != 0 {
		t.Error("invalid list allowed DELETE")
	}

	model.Name = types.StringValue("replacement")
	for _, plan := range []tfsdk.Plan{resourcePlan(t, r, model), tfsdk.Plan(emptyState(t, r))} {
		modify := resource.ModifyPlanResponse{Plan: plan}
		r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, &modify)
		check("modify plan", modify.Diagnostics, tfsdk.State(modify.Plan), tfsdk.State(plan))
		if len(modify.RequiresReplace) != 0 {
			t.Error("invalid list added a replacement path")
		}
	}

	model.Name = types.StringValue("dev")
	model.UserEditable = types.BoolValue(true)
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: resourcePlan(t, r, model)}, &update)
	check("update readback", update.Diagnostics, update.State, state)
	if len(f.bodies("PUT /users/attr/dev")) != 1 {
		t.Error("expected readback after a successful PUT")
	}

	d := &userAttributeDataSource{api: api}
	config := dataSourceConfig(t, d, userAttributeDataSourceModel{Name: types.StringValue("dev")})
	initial := tfsdk.State(config)
	data := datasource.ReadResponse{State: initial}
	d.Read(ctx, datasource.ReadRequest{Config: config}, &data)
	check("data source", data.Diagnostics, data.State, initial)
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
	checkAttributeGuardPlans(t, modifier, model, tc.wantError, tc.failure == "/users/attr")
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
	unsafe, invalidList bool,
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
		if op == "replace" && !invalidList {
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
