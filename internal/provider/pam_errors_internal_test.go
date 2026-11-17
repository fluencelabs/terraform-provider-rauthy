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
}

func TestPamGroupDeleteHints(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		code, message string
		hint          bool
	}{
		{"host", 400, "BadRequest", "Cannot delete group with assigned hosts", true},
		{"sqlite", 400, "BadRequest", "Error code 787: Foreign key constraint failed", true},
		{"sqlite database", 500, "Database", "Error code 787: Foreign key constraint failed", false},
		{"postgres", 500, "Database", "db error", true},
		{"wrong host status", 500, "BadRequest", "Cannot delete group with assigned hosts", false},
		{"original forbidden assertion", 403, "BadRequest", "Error code 787", false},
		{"forbidden postgres", 403, "Database", "db error", false},
		{"wrong postgres status", 400, "Database", "db error", false},
		{"forbidden", 403, "Forbidden", "Error code 787", false},
		{"other database", 500, "Database", "connection closed", false},
		{"other constraint", 400, "BadRequest", "Error code 1555", false},
		{"immutable", 400, "BadRequest", "Immutable groups cannot be deleted", false},
		{"unstructured", 500, "", "Error code 787", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.handle("DELETE /pam/groups/100001", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": tc.code, "message": tc.message})
			})
			r := &pamGroupResource{api: api}
			state := pamGroupResourceModel{ID: types.Int64Value(100001)}
			var resp resource.DeleteResponse
			r.Delete(context.Background(), resource.DeleteRequest{State: resourceState(t, r, &state)}, &resp)
			if !resp.Diagnostics.HasError() ||
				strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "create_before_destroy") != tc.hint {
				t.Fatalf("wrong advice: %v", resp.Diagnostics)
			}
			if tc.name == "postgres" && (!strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "If a host") ||
				!strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "check the Rauthy logs")) {
				t.Fatalf("opaque Postgres error must not claim a confirmed foreign-key violation: %v", resp.Diagnostics)
			}
		})
	}
}

func TestPamGroupCreateHints(t *testing.T) {
	for _, tc := range []struct {
		name, code, message string
		status              int
		plain, hint         bool
	}{
		{"sqlite duplicate", "NotFound", "no rows returned", 404, false, true},
		{"postgres duplicate", "NotFound", "No results found", 404, false, true},
		{"other structured 404", "NotFound", "Route not found", 404, false, false},
		{"wrong status", "NotFound", "no rows returned", 500, false, false},
		{"proxy", "", "404 page not found", 404, true, false},
		{"missing code", "", "no rows returned", 404, false, false},
		{"other code", "BadRequest", "no rows returned", 404, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.handle("POST /pam/groups", func(w http.ResponseWriter, _ *http.Request) {
				if tc.plain {
					http.Error(w, tc.message, tc.status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"timestamp": 1700000000,
					"error":     tc.code,
					"message":   tc.message,
				})
			})
			r := &pamGroupResource{api: api}
			plan := pamGroupResourceModel{Name: types.StringValue("test"), Typ: types.StringValue("generic")}
			var resp resource.CreateResponse
			r.Create(context.Background(), resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
			if !resp.Diagnostics.HasError() ||
				strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "already exists") != tc.hint {
				t.Fatalf("wrong advice: %v", resp.Diagnostics)
			}
		})
	}
}
