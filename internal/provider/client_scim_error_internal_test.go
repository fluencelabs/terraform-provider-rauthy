package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"

	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
)

func TestScimSyncFailed(t *testing.T) {
	t.Parallel()
	scim := fmt.Errorf("update: %w", &rauthy.APIError{StatusCode: 500, ErrorCode: "Scim"})
	other := &rauthy.APIError{StatusCode: 500, ErrorCode: "Connection"}
	if !scimSyncFailed(scim) {
		t.Error("a wrapped Scim 500 is not recognised")
	}
	if scimSyncFailed(other) || scimSyncFailed(errors.New("boom")) || scimSyncFailed(nil) {
		t.Error("an error other than a Scim 500 is taken for a failed SCIM sync")
	}
}

func TestClientScimFailureSavesStoredState(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, readFails := range []bool{false, true} {
			for _, confidential := range []bool{false, true} {
				t.Run(
					fmt.Sprintf("create=%t/readFails=%t/confidential=%t", create, readFails, confidential),
					func(t *testing.T) {
						testClientScimFailure(t, create, readFails, confidential)
					},
				)
			}
		}
	}
}

func testClientScimFailure(t *testing.T, create, readFails, confidential bool) {
	t.Helper()
	ctx := context.Background()
	f, api := newFakeRauthy(t)
	current := fullResponse()
	current.Confidential = confidential
	stored := *current
	stored.Name = new("Saved Name")
	stored.AccessTokenLifetime = 900
	stored.Scim = nil
	var persisted atomic.Bool
	f.answer("POST /clients", current)
	secret := rauthy.NewSecret("old-secret")
	f.answer("POST /clients/example-app/secret", &rauthy.ClientSecretResponse{Secret: &secret})
	f.handle("GET /clients/example-app", func(w http.ResponseWriter, _ *http.Request) {
		if persisted.Load() && readFails {
			http.Error(w, `{"error":"Connection","message":"read failed"}`, http.StatusInternalServerError)
			return
		}
		value := current
		if persisted.Load() {
			value = &stored
		}
		_ = json.NewEncoder(w).Encode(value)
	})
	f.handle("PUT /clients/example-app", func(w http.ResponseWriter, req *http.Request) {
		var update rauthy.UpdateClientRequest
		if err := json.NewDecoder(req.Body).Decode(&update); err != nil {
			t.Error(err)
		}
		if update.AccessTokenLifetime != 900 {
			t.Errorf("lifetime = %d", update.AccessTokenLifetime)
		}
		persisted.Store(true)
		http.Error(w, `{"error":"Scim","message":"sync failed"}`, http.StatusInternalServerError)
	})
	r := &clientResource{api: api}
	old := clientModelOf(t, current)
	plan := old
	plan.Name = types.StringValue("Planned Name")
	plan.AccessTokenLifetime = types.Int64Value(900)
	plan.SecretRotationTrigger = types.StringValue("new-trigger")
	var state tfsdk.State
	var diags diag.Diagnostics
	if create {
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		state, diags = resp.State, resp.Diagnostics
	} else {
		resp := resource.UpdateResponse{State: resourceState(t, r, &old)}
		r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &old)}, &resp)
		state, diags = resp.State, resp.Diagnostics
	}
	checkScimDiagnostic(t, diags)
	var got clientResourceModel
	if d := state.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if readFails {
		if len(diags.Errors()) != 2 {
			t.Errorf("missing read failure: %v", diags)
		}
		if !got.Name.Equal(old.Name) {
			t.Error("failed read replaced known state")
		}

		return
	}
	want := clientModelOf(t, &stored)
	if create {
		want.SecretRotationTrigger = plan.SecretRotationTrigger
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state does not match persisted response: got %+v, want %+v", got, want)
	}
}

func checkScimDiagnostic(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if !diags.HasError() ||
		!strings.Contains(diags.Errors()[len(diags.Errors())-1].Detail(), "Rauthy stored the client's configuration") {
		t.Fatalf("missing saved-client diagnostic: %v", diags)
	}
}
