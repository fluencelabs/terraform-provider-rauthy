package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRound2LogoFallback(t *testing.T) {
	ctx := context.Background()
	for _, data := range []string{"custom", "global"} {
		f, api := newFakeRauthy(t)
		f.handle(
			"GET /clients/example-app/logo",
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(data)) },
		)
		r := &clientImageResource{api: api, kind: rauthy.ImageLogo}
		state := clientImageResourceModel{
			ClientID:   types.StringValue("example-app"),
			StoredHash: types.StringValue(hashBytes([]byte("custom"))),
		}
		resp := resource.ReadResponse{State: resourceState(t, r, &state)}
		r.Read(ctx, resource.ReadRequest{State: resp.State}, &resp)
		if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() != (data == "global") {
			t.Fatalf("fallback not detected: %v", resp.Diagnostics)
		}
	}
}
