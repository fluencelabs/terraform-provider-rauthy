package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
)

// echoScope answers a scope write with the scope it describes, as Rauthy does.
func echoScope(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req rauthy.ScopeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rauthy.ScopeResponse{
			ID:                id,
			Name:              req.Scope,
			AttrIncludeAccess: req.AttrIncludeAccess,
			AttrIncludeID:     req.AttrIncludeID,
			ClaimsAtRoot:      req.ClaimsAtRoot,
		})
	}
}

func scopeModel(claimsAtRoot types.Bool) scopeResourceModel {
	return scopeResourceModel{
		ID:                types.StringValue("scope-1"),
		Name:              types.StringValue("read:billing"),
		AttrIncludeAccess: types.SetNull(types.StringType),
		AttrIncludeID:     types.SetNull(types.StringType),
		ClaimsAtRoot:      claimsAtRoot,
	}
}

// PUT /scopes/{id} replaces the scope wholesale, so an update that leaves
// claims_at_root out turns it off. The update must carry the planned value,
// which is the prior one when the configuration does not set it.
func TestScopeUpdate_SendsClaimsAtRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fake, api := newFakeRauthy(t)
	fake.handle("PUT /scopes/scope-1", echoScope("scope-1"))
	r := &scopeResource{api: api}

	model := scopeModel(types.BoolValue(true))
	req := resource.UpdateRequest{Plan: resourcePlan(t, r, &model), State: resourceState(t, r, &model)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}

	var sent rauthy.ScopeRequest
	fake.lastBody(t, "PUT /scopes/scope-1", &sent)
	if !sent.ClaimsAtRoot {
		t.Error("the update reset claims_at_root: PUT /scopes/scope-1 sent false")
	}

	var got scopeResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if !got.ClaimsAtRoot.ValueBool() {
		t.Errorf("claims_at_root in state = %v, want true", got.ClaimsAtRoot)
	}
}

// Left out of the configuration at create, claims_at_root takes Rauthy's
// default and lands in state as a known value.
func TestScopeCreate_UnsetClaimsAtRootIsResolved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fake, api := newFakeRauthy(t)
	fake.handle("POST /scopes", echoScope("scope-1"))
	r := &scopeResource{api: api}

	model := scopeModel(types.BoolUnknown())
	model.ID = types.StringUnknown()
	req := resource.CreateRequest{Plan: resourcePlan(t, r, &model)}
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}

	var sent rauthy.ScopeRequest
	fake.lastBody(t, "POST /scopes", &sent)
	if sent.ClaimsAtRoot {
		t.Error("an unset claims_at_root was sent as true")
	}

	var got scopeResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.ClaimsAtRoot.IsUnknown() || got.ClaimsAtRoot.IsNull() || got.ClaimsAtRoot.ValueBool() {
		t.Errorf("claims_at_root in state = %v, want false", got.ClaimsAtRoot)
	}
}

// A refresh picks up the server's value, so a scope switched in the Admin UI
// shows up as a diff and existing state gains the attribute without one.
func TestApplyScope_ReadsClaimsAtRoot(t *testing.T) {
	t.Parallel()

	model := scopeResourceModel{}
	applyScope(&model, &rauthy.ScopeResponse{ID: "scope-1", Name: "read:billing", ClaimsAtRoot: true})
	if !model.ClaimsAtRoot.ValueBool() {
		t.Errorf("claims_at_root = %v, want true", model.ClaimsAtRoot)
	}
}

// Optional and computed, planning the prior value: a configuration that does
// not mention claims_at_root plans the prior value rather than an unknown, so
// the update above sends what Rauthy already has.
func TestScopeSchema_ClaimsAtRootKeepsThePriorValue(t *testing.T) {
	t.Parallel()

	var resp resource.SchemaResponse
	NewScopeResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	a, ok := resp.Schema.Attributes["claims_at_root"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("claims_at_root is %T, want a bool attribute", resp.Schema.Attributes["claims_at_root"])
	}
	if !a.Optional || !a.Computed || len(a.PlanModifiers) == 0 {
		t.Errorf("claims_at_root: optional=%v computed=%v plan modifiers=%d, want optional, computed and "+
			"a state-for-unknown modifier", a.Optional, a.Computed, len(a.PlanModifiers))
	}
}

// State written by a provider without claims_at_root has none, so with
// -refresh=false the planned value stays unknown even with UseStateForUnknown.
// The update must then carry what Rauthy holds, not a default.
func TestScopeUpdate_UnknownClaimsAtRootKeepsTheServerValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fake, api := newFakeRauthy(t)
	fake.answer("GET /scopes", []rauthy.ScopeResponse{
		{ID: "scope-1", Name: "read:billing", ClaimsAtRoot: true},
	})
	fake.handle("PUT /scopes/scope-1", echoScope("scope-1"))
	r := &scopeResource{api: api}

	state := scopeModel(types.BoolNull())
	plan := scopeModel(types.BoolUnknown())
	plan.Name = types.StringValue("read:invoices")

	req := resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}

	var sent rauthy.ScopeRequest
	fake.lastBody(t, "PUT /scopes/scope-1", &sent)
	if sent.Scope != "read:invoices" {
		t.Errorf("name sent = %q, want the planned read:invoices", sent.Scope)
	}
	if !sent.ClaimsAtRoot {
		t.Error("the update switched claims_at_root off although Rauthy holds true")
	}

	var got scopeResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if !got.ClaimsAtRoot.ValueBool() {
		t.Errorf("claims_at_root in state = %v, want true", got.ClaimsAtRoot)
	}
}

// With state that has no claims_at_root, the plan must stay unknown rather
// than copy the null: Update then fills it in from Rauthy, and a null plan
// would make the applied value an "inconsistent result after apply".
func TestScopeSchema_ClaimsAtRootStaysUnknownWithoutAPriorValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	r := NewScopeResource()
	var sresp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sresp)
	a, _ := sresp.Schema.Attributes["claims_at_root"].(schema.BoolAttribute)

	state := resourceState(t, r, ptr(scopeModel(types.BoolNull())))
	req := planmodifier.BoolRequest{
		Path:        path.Root("claims_at_root"),
		State:       state,
		StateValue:  types.BoolNull(),
		ConfigValue: types.BoolNull(),
		PlanValue:   types.BoolUnknown(),
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	for _, m := range a.PlanModifiers {
		m.PlanModifyBool(ctx, req, resp)
	}
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("planned claims_at_root = %v, want unknown", resp.PlanValue)
	}
}

func ptr[T any](v T) *T { return &v }
