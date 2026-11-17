package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPAMErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("host reads once", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.handle("GET /pam/hosts/host1", forbid)
		r := &pamHostResource{api: api}
		state := pamHostResourceModel{
			ID:      types.StringValue("host1"),
			IPs:     types.SetNull(types.StringType),
			Aliases: types.SetNull(types.StringType),
		}
		resp := resource.ReadResponse{State: resourceState(t, r, &state)}
		r.Read(ctx, resource.ReadRequest{State: resp.State}, &resp)
		if !resp.Diagnostics.HasError() || len(f.bodies("GET /pam/hosts/host1")) != 1 {
			t.Fatalf("forbidden read repeated: %v", resp.Diagnostics)
		}
	})
	t.Run("delete cause", func(t *testing.T) {
		for _, code := range []int{400, 403} {
			f, api := newFakeRauthy(t)
			f.handle("DELETE /pam/groups/100001", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_ = json.NewEncoder(w).
					Encode(map[string]string{"error": "BadRequest", "message": "Error code 787"})
			})
			r := &pamGroupResource{api: api}
			state := pamGroupResourceModel{ID: types.Int64Value(100001)}
			var resp resource.DeleteResponse
			r.Delete(ctx, resource.DeleteRequest{State: resourceState(t, r, &state)}, &resp)
			if !resp.Diagnostics.HasError() ||
				strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "create_before_destroy") != (code == 400) {
				t.Fatalf("wrong advice for %d: %v", code, resp.Diagnostics)
			}
		}
	})
}
