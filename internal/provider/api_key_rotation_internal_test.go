package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
)

// As with a client secret, an API key rotation is sent once and never
// retried. When the request fails after it went out, the operator has to be
// told the secret may have changed anyway.
func TestAPIKeyUpdate_FailedRotationSaysItMayHaveHappened(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fake, api := newFakeRauthy(t)
	fake.handle("PUT /api_keys/deploy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	fake.handle("PUT /api_keys/deploy/secret", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	r := &apiKeyResource{api: api}

	state := apiKeyResourceModel{
		Name:                  types.StringValue("deploy"),
		Access:                accessSetOf(t, rauthy.APIKeyAccess{Group: "Users", AccessRights: []string{"read"}}),
		ExpiresAt:             types.Int64Null(),
		CreatedAt:             types.Int64Value(1),
		Secret:                types.StringValue("deploy$old"),
		SecretRotationTrigger: types.StringValue("1"),
	}
	plan := state
	plan.SecretRotationTrigger = types.StringValue("2")
	plan.Secret = types.StringUnknown()

	req := resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, req, &resp)

	if n := len(fake.bodies("PUT /api_keys/deploy/secret")); n != 1 {
		t.Errorf("the rotation was sent %d times, want once", n)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update succeeded although the rotation failed")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "may have rotated") {
		t.Errorf("the error does not say the secret may have changed anyway:\n%s", detail)
	}
	var got apiKeyResourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if !got.SecretRotationTrigger.Equal(state.SecretRotationTrigger) {
		t.Error("failed rotation consumed the pending trigger")
	}
	if !apiKeyRotationRequested(&got, &plan) {
		t.Error("the next apply would not retry the pending rotation")
	}
	if !got.Secret.Equal(state.Secret) {
		t.Error("failed rotation did not preserve the prior secret")
	}
}
