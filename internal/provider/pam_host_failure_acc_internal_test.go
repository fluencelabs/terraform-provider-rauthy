package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//nolint:gocognit // Exercise the write, failed refresh, partial state and remote state together.
func TestAccPamHost_failedRefresh(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" || os.Getenv("RAUTHY_URL") == "" || os.Getenv("RAUTHY_API_KEY") == "" {
		t.Skip("live Rauthy required")
	}
	ctx := context.Background()
	api, err := rauthy.New(os.Getenv("RAUTHY_URL"), os.Getenv("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	group, err := api.CreatePamGroup(ctx, rauthy.PamGroupCreateRequest{Name: "partialhost", Typ: "host"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := api.DeletePamGroup(ctx, group.ID); e != nil {
			t.Error(e)
		}
	})
	host, err := api.CreatePamHost(ctx, rauthy.PamHostCreateRequest{Hostname: "partial01", GID: group.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := api.DeletePamHost(ctx, host.ID); e != nil {
			t.Error(e)
		}
	})
	if err = api.UpdatePamHost(
		ctx,
		host.ID,
		rauthy.PamHostUpdateRequest{
			Hostname: "partial01",
			GID:      group.ID,
			IPs:      []string{"10.0.0.10"},
			Aliases:  []string{"ci"},
		},
	); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(os.Getenv("RAUTHY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var written atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if written.Load() && req.Method == http.MethodGet {
			forbid(w, req)
			return
		}
		if req.Method == http.MethodPut {
			written.Store(true)
		}
		proxy.ServeHTTP(w, req)
	}))
	t.Cleanup(srv.Close)
	viaProxy, err := rauthy.New(srv.URL, os.Getenv("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	r := &pamHostResource{api: viaProxy}
	prior := pamHostResourceModel{
		ID:                types.StringValue(host.ID),
		Hostname:          types.StringValue("partial01"),
		GID:               types.Int64Value(group.ID),
		ForceMfa:          types.BoolValue(false),
		LocalPasswordOnly: types.BoolValue(false),
		IPs:               stringSet(t, "10.0.0.10"),
		Aliases:           stringSet(t, "ci"),
	}
	secret, err := api.GetPamHostSecret(ctx, host.ID)
	if err != nil {
		t.Fatal(err)
	}
	prior.Secret = types.StringValue(secret.Secret.Reveal())
	plan := prior
	plan.Hostname = types.StringValue("partial02")
	plan.IPs = types.SetUnknown(types.StringType)
	plan.Aliases = types.SetUnknown(types.StringType)
	plan.Secret = types.StringUnknown()
	resp := resource.UpdateResponse{State: resourceState(t, r, &prior)}
	r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &prior)}, &resp)
	var got pamHostResourceModel
	d := resp.State.Get(ctx, &got)
	if !resp.Diagnostics.HasError() || d.HasError() || !resp.State.Raw.IsFullyKnown() || !got.IPs.Equal(prior.IPs) ||
		!got.Aliases.Equal(prior.Aliases) ||
		!got.Secret.Equal(prior.Secret) {
		t.Fatal("failed refresh did not retain concrete state")
	}
	remote, err := api.GetPamHost(ctx, host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if remote.Hostname != "partial02" || !stringsToSet(remote.IPs).Equal(prior.IPs) ||
		!stringsToSet(remote.Aliases).Equal(prior.Aliases) {
		t.Fatal("live update did not retain addresses and aliases")
	}
}
