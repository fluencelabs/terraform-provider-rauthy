package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRound2PublicTriggerRemoval(t *testing.T) {
	ctx := context.Background()
	f, api := newFakeRauthy(t)
	c := fullResponse()
	f.answer("GET /clients/example-app", c)
	public := *c
	public.Confidential = false
	f.answer("PUT /clients/example-app", &public)
	r := &clientResource{api: api}
	state := clientModelOf(t, c)
	state.SecretRotationTrigger = types.StringValue("1")
	plan := state
	plan.Confidential = types.BoolValue(false)
	plan.SecretRotationTrigger = types.StringNull()
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got clientResourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() || !got.Secret.IsNull() || !got.SecretRotationTrigger.IsNull() {
		t.Fatalf("public transition: %+v %v", got, d)
	}
}

func TestRound2PublicTriggerSetRejected(t *testing.T) {
	ctx := context.Background()
	f, api := newFakeRauthy(t)
	c := fullResponse()
	c.Confidential = false
	f.answer("GET /clients/example-app", c)
	f.answer("PUT /clients/example-app", c)
	r := &clientResource{api: api}
	state := clientModelOf(t, c)
	plan := state
	plan.SecretRotationTrigger = types.StringValue("rotate")
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}, &resp)
	if !resp.Diagnostics.HasError() || len(f.bodies("PUT /clients/example-app/secret")) != 0 {
		t.Fatalf("public secret rotation attempted: %v", resp.Diagnostics)
	}
}
