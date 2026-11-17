package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPamHostUpdateFailedRefreshKeepsKnownState(t *testing.T) {
	ctx := context.Background()
	f, api := newFakeRauthy(t)
	before := rauthy.PamHostDetailsResponse{
		ID: "host1", Hostname: "build01", GID: 100001,
		IPs: []string{"10.0.0.10"}, Aliases: []string{"ci"},
	}
	f.answer("GET /pam/hosts/host1", before)
	f.handle("PUT /pam/hosts/host1", func(w http.ResponseWriter, _ *http.Request) {
		// The write succeeds, but the subsequent details read fails.
		f.handle("GET /pam/hosts/host1", forbid)
		w.WriteHeader(http.StatusOK)
	})
	r := &pamHostResource{api: api}
	prior := pamHostResourceModel{
		ID: types.StringValue("host1"), Hostname: types.StringValue("build01"),
		GID: types.Int64Value(100001), ForceMfa: types.BoolValue(false),
		LocalPasswordOnly: types.BoolValue(false), IPs: stringSet(t, "10.0.0.10"),
		Aliases: stringSet(t, "ci"), Secret: types.StringValue("synthetic-secret"),
	}
	plan := prior
	plan.Hostname = types.StringValue("build02")
	plan.IPs = types.SetUnknown(types.StringType)
	plan.Aliases = types.SetUnknown(types.StringType)
	plan.Secret = types.StringUnknown()
	response := resource.UpdateResponse{State: resourceState(t, r, &prior)}
	r.Update(
		ctx,
		resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &prior)},
		&response,
	)
	var result pamHostResourceModel
	d := response.State.Get(ctx, &result)
	if d.HasError() || !response.Diagnostics.HasError() {
		t.Fatalf("wrong diagnostics: %v %v", d, response.Diagnostics)
	}
	if !result.IPs.Equal(prior.IPs) || !result.Aliases.Equal(prior.Aliases) || !result.Secret.Equal(prior.Secret) ||
		result.Hostname.ValueString() != "build02" {
		t.Fatalf("partial state lost: %+v", result)
	}
	if !response.State.Raw.IsFullyKnown() {
		t.Fatal("partial state contains unknown values")
	}
	var body rauthy.PamHostUpdateRequest
	f.lastBody(t, "PUT /pam/hosts/host1", &body)
	if len(body.IPs) != 1 || len(body.Aliases) != 1 {
		t.Fatalf("bad fixture: %+v", body)
	}
}

func TestBlacklistFailedReadDeleteCanonicalIPv6(t *testing.T) {
	ctx := context.Background()
	f, api := newFakeRauthy(t)
	f.answer("POST /blacklist", nil)
	f.handle("GET /blacklist", forbid)
	f.answer("DELETE /blacklist/2001:db8::2", nil)
	r := &blacklistIPResource{api: api}
	plan := blacklistIPResourceModel{IP: types.StringValue("2001:0DB8:0000::0002"), Exp: types.Int64Value(1900000000)}
	created := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &created)
	if !created.Diagnostics.HasError() || created.State.Raw.IsNull() {
		t.Fatalf("missing partial state: %v", created.Diagnostics)
	}
	deleted := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	if deleted.Diagnostics.HasError() || len(f.bodies("DELETE /blacklist/2001:db8::2")) != 1 {
		t.Fatalf("canonical deletion failed: %v", deleted.Diagnostics)
	}
}
