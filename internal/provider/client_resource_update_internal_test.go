package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
)

// serverOnlyClient is a client carrying the settings rauthy_client has no
// attributes for: they exist on the server only, set in the Admin UI or by
// another tool.
func serverOnlyClient(confidential bool) *rauthy.ClientResponse {
	c := fullResponse()
	c.Confidential = confidential
	c.AllowedResources = []string{"https://api.example.com"}
	c.DefaultAud = []string{"billing-api"}
	c.Claims = json.RawMessage(`{"tier":"gold"}`)
	c.ClaimsAtRoot = true
	return c
}

// clientModelOf is the state rauthy_client holds for c.
func clientModelOf(t *testing.T, c *rauthy.ClientResponse) clientResourceModel {
	t.Helper()
	var m clientResourceModel
	var diags diag.Diagnostics
	applyResponse(context.Background(), &m, c, &diags)
	if diags.HasError() {
		t.Fatalf("applyResponse: %v", diags)
	}
	m.Secret = types.StringNull()
	if c.Confidential {
		m.Secret = types.StringValue("old-secret")
	}
	return m
}

// PUT /clients/{id} replaces the client wholesale, so a body built from the
// schema alone resets every setting the schema does not model. An update must
// carry those over from the server.
func TestClientUpdate_KeepsSettingsTheSchemaDoesNotModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	current := serverOnlyClient(false)
	fake, api := newFakeRauthy(t)
	fake.answer("GET /clients/example-app", current)
	fake.answer("PUT /clients/example-app", current)
	r := &clientResource{api: api}

	state := clientModelOf(t, current)
	plan := state
	plan.AccessTokenLifetime = types.Int64Value(900)

	req := resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}

	var sent rauthy.UpdateClientRequest
	fake.lastBody(t, "PUT /clients/example-app", &sent)
	if sent.AccessTokenLifetime != 900 {
		t.Errorf("access_token_lifetime sent = %d, want the planned 900", sent.AccessTokenLifetime)
	}
	if !slices.Equal(sent.AllowedResources, current.AllowedResources) {
		t.Errorf("allowed_resources sent = %v, want %v", sent.AllowedResources, current.AllowedResources)
	}
	if !slices.Equal(sent.DefaultAud, current.DefaultAud) {
		t.Errorf("default_aud sent = %v, want %v", sent.DefaultAud, current.DefaultAud)
	}
	if !bytes.Equal(sent.Claims, current.Claims) {
		t.Errorf("claims sent = %s, want %s", sent.Claims, current.Claims)
	}
	if !sent.ClaimsAtRoot {
		t.Error("claims_at_root sent = false, want the server's true")
	}
}

// The second call of a create is a PUT as well; it must not reset what the
// POST left behind either.
func TestClientCreate_KeepsSettingsTheSchemaDoesNotModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	created := serverOnlyClient(false)
	fake, api := newFakeRauthy(t)
	fake.answer("POST /clients", created)
	fake.answer("PUT /clients/example-app", created)
	r := &clientResource{api: api}

	plan := clientModelOf(t, created)
	plan.Secret = types.StringUnknown()

	req := resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}

	var sent rauthy.UpdateClientRequest
	fake.lastBody(t, "PUT /clients/example-app", &sent)
	if !sent.ClaimsAtRoot || !bytes.Equal(sent.Claims, created.Claims) ||
		!slices.Equal(sent.DefaultAud, created.DefaultAud) ||
		!slices.Equal(sent.AllowedResources, created.AllowedResources) {
		t.Errorf("the configuring PUT reset settings the POST answered with: %+v", sent)
	}
}

// A rotation is sent once and never retried, because a retry would rotate a
// second time. When the request fails after it went out, the operator has to
// be told the secret may have changed anyway.
func TestClientUpdate_FailedRotationSaysItMayHaveHappened(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	current := serverOnlyClient(true)
	fake, api := newFakeRauthy(t)
	fake.answer("GET /clients/example-app", current)
	fake.answer("PUT /clients/example-app", current)
	fake.handle("PUT /clients/example-app/secret", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	r := &clientResource{api: api}

	state := clientModelOf(t, current)
	state.SecretRotationTrigger = types.StringValue("1")
	plan := state
	plan.SecretRotationTrigger = types.StringValue("2")
	plan.Secret = types.StringUnknown()

	req := resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, req, &resp)

	if n := len(fake.bodies("PUT /clients/example-app/secret")); n != 1 {
		t.Errorf("the rotation was sent %d times, want once", n)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update succeeded although the rotation failed")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "may have rotated") {
		t.Errorf("the error does not say the secret may have changed anyway:\n%s", detail)
	}

	var saved clientResourceModel
	if diags := resp.State.Get(ctx, &saved); diags.HasError() {
		t.Fatalf("read state after failed rotation: %v", diags)
	}
	if !saved.SecretRotationTrigger.Equal(state.SecretRotationTrigger) {
		t.Errorf("rotation trigger = %v, want the previous %v",
			saved.SecretRotationTrigger, state.SecretRotationTrigger)
	}
	if !rotationRequested(&saved, &plan) {
		t.Error("failed rotation consumed the pending trigger")
	}
	if !saved.Secret.Equal(state.Secret) {
		t.Errorf("secret = %v, want the previous secret", saved.Secret)
	}
}
