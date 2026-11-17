package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
)

func TestUserUpdate_PasswordRetryPolicy(t *testing.T) {
	for _, withPassword := range []bool{true, false} {
		name, wantAttempts := "profile", 4
		if withPassword {
			name, wantAttempts = "password", 1
		}
		t.Run(name, func(t *testing.T) {
			fake, api := newFakeRauthy(t)
			current := &rauthy.UserResponse{ID: "u-1", Email: "ada@example.com", Language: "en"}
			fake.answer("GET /users/u-1", current)
			fake.handle("PUT /users/u-1", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			})
			r := &userResource{api: api}
			var state userResourceModel
			applyUser(&state, current)
			plan := state
			config := plan
			if withPassword {
				plan.PasswordRotationTrigger = types.StringValue("1")
				config = plan
				config.PasswordWO = types.StringValue("SyntheticPassword!42")
			}
			configPlan := resourcePlan(t, r, &config)
			resp := resource.UpdateResponse{State: emptyState(t, r)}
			r.Update(context.Background(), resource.UpdateRequest{
				Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state),
				Config: tfsdk.Config(configPlan),
			}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected failed update")
			}
			if got := len(fake.bodies("PUT /users/u-1")); got != wantAttempts {
				t.Fatalf("PUT attempts = %d, want %d", got, wantAttempts)
			}
		})
	}
}
