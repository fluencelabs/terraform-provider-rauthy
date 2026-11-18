package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestLogoFallback(t *testing.T) {
	ctx := context.Background()
	for _, data := range []string{"custom", "global", "replacement"} {
		f, api := newFakeRauthy(t)
		f.handle(
			"GET /clients/example-app/logo",
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(data)) },
		)
		r := &clientImageResource{api: api, kind: rauthy.ImageLogo}
		state := clientImageResourceModel{
			ClientID:      types.StringValue("example-app"),
			StoredHash:    types.StringValue(hashBytes([]byte("custom"))),
			ContentBase64: types.StringValue("Y3VzdG9t"),
		}
		resp := resource.ReadResponse{State: resourceState(t, r, &state)}
		r.Read(ctx, resource.ReadRequest{State: resp.State}, &resp)
		if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
			t.Fatalf("fallback not detected: %v", resp.Diagnostics)
		}
		var got clientImageResourceModel
		if d := resp.State.Get(ctx, &got); d.HasError() {
			t.Fatal(d)
		}
		if got.ContentBase64.IsNull() != (data != "custom") || got.StoredHash.ValueString() != hashBytes([]byte(data)) {
			t.Fatalf("drift did not schedule upload: %+v", got)
		}
		f.answer("DELETE /clients/example-app/logo", nil)
		deleted := resource.DeleteResponse{State: resp.State}
		r.Delete(ctx, resource.DeleteRequest{State: resp.State}, &deleted)
		if deleted.Diagnostics.HasError() || len(f.bodies("DELETE /clients/example-app/logo")) != 1 {
			t.Fatalf("lost logo ownership: %v", deleted.Diagnostics)
		}
	}
}

func TestLogoReadPreservesStateOnProxy404(t *testing.T) {
	f, api := newFakeRauthy(t)
	f.handle("GET /clients/example-app/logo", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "route missing", http.StatusNotFound)
	})
	r := &clientImageResource{api: api, kind: rauthy.ImageLogo}
	prior := resourceState(t, r, &clientImageResourceModel{
		ClientID:      types.StringValue("example-app"),
		StoredHash:    types.StringValue(hashBytes([]byte("custom"))),
		ContentBase64: types.StringValue("Y3VzdG9t"),
	})
	resp := resource.ReadResponse{State: prior}
	r.Read(t.Context(), resource.ReadRequest{State: prior}, &resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "HTTP 404") ||
		!resp.State.Raw.Equal(prior.Raw) {
		t.Fatalf("proxy 404 must report an error and retain the managed logo: %v, %v", resp.Diagnostics, resp.State.Raw)
	}
}
