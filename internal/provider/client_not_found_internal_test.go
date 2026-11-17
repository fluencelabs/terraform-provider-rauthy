package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestClientNotFound(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		missing                 bool
	}{
		{name: "empty route response"},
		{name: "plain route response", contentType: "text/plain", body: "404 page not found"},
		{
			name:        "Rauthy NotFound",
			contentType: "application/json",
			body:        `{"timestamp":1700000000,"error":"NotFound","message":"no rows returned"}`,
			missing:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			reply := func(w http.ResponseWriter, _ *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, tc.body)
			}
			f.handle("GET /clients/example-app", reply)
			f.handle("DELETE /clients/example-app", reply)

			ctx := context.Background()
			r := &clientResource{api: api}
			model := clientModelOf(t, fullResponse())
			prior := resourceState(t, r, &model)

			read := resource.ReadResponse{State: prior}
			r.Read(ctx, resource.ReadRequest{State: prior}, &read)
			wantState := prior
			if tc.missing {
				wantState = emptyState(t, r)
			}
			if !read.State.Raw.Equal(wantState.Raw) {
				t.Errorf("unexpected client state after read, want client removed: %t", tc.missing)
			}

			deleted := resource.DeleteResponse{State: prior}
			r.Delete(ctx, resource.DeleteRequest{State: prior}, &deleted)
			responses := map[string]diag.Diagnostics{"read": read.Diagnostics, "delete": deleted.Diagnostics}
			for method, diags := range responses {
				if got := diags.HasError(); got != !tc.missing {
					t.Errorf("%s error = %t, want %t, diagnostics: %v", method, got, !tc.missing, diags)
				} else if got && !strings.Contains(diags.Errors()[0].Detail(), "HTTP 404") {
					t.Errorf("%s must report the route's HTTP 404, diagnostics: %v", method, diags)
				}
			}
		})
	}
}
