package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/fluencelabs/rauthy-go"
)

// fakeRauthy is an in-process stand-in for the Rauthy admin API. Each route is
// keyed by "METHOD /path" below /auth/v1 and answers with a JSON body; every
// request body is recorded so a test can assert on what the provider sent.
type fakeRauthy struct {
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	sent   map[string][]json.RawMessage
}

func newFakeRauthy(t *testing.T) (*fakeRauthy, *rauthy.Client) {
	t.Helper()

	f := &fakeRauthy{routes: map[string]http.HandlerFunc{}, sent: map[string][]json.RawMessage{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path[len(rauthy.APIBasePath):]
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))

		f.mu.Lock()
		f.sent[key] = append(f.sent[key], body)
		h, ok := f.routes[key]
		f.mu.Unlock()

		if !ok {
			t.Errorf("unexpected request %s", key)
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := newRauthyClient(srv.URL, "test$key", rauthy.WithRetryPolicy(4, 0, 0))
	if err != nil {
		t.Fatalf("rauthy.New: %v", err)
	}
	return f, c
}

// answer registers a route that replies 200 with v encoded as JSON.
func (f *fakeRauthy) answer(route string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[route] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}

// handle registers a route with a handler of the test's own.
func (f *fakeRauthy) handle(route string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[route] = h
}

// bodies returns every request body sent to route, in order.
func (f *fakeRauthy) bodies(route string) []json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[route]
}

// lastBody decodes the last request body sent to route into v.
func (f *fakeRauthy) lastBody(t *testing.T, route string, v any) {
	t.Helper()
	sent := f.bodies(route)
	if len(sent) == 0 {
		t.Fatalf("nothing was sent to %s", route)
	}
	if err := json.Unmarshal(sent[len(sent)-1], v); err != nil {
		t.Fatalf("decode the body sent to %s: %v", route, err)
	}
}

// resourceState renders a model as the state of resource r. The plan of an
// update is the same value with a different wrapper; see resourcePlan.
func resourceState(t *testing.T, r resource.Resource, model any) tfsdk.State {
	t.Helper()
	ctx := context.Background()

	var sresp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sresp)
	state := tfsdk.State{
		Schema: sresp.Schema,
		Raw:    tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("render state: %v", diags)
	}
	return state
}

func resourcePlan(t *testing.T, r resource.Resource, model any) tfsdk.Plan {
	t.Helper()
	state := resourceState(t, r, model)
	return tfsdk.Plan(state)
}

// emptyState is the state a Create or Update writes its result into.
func emptyState(t *testing.T, r resource.Resource) tfsdk.State {
	t.Helper()
	ctx := context.Background()

	var sresp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sresp)
	return tfsdk.State{
		Schema: sresp.Schema,
		Raw:    tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil),
	}
}

// dataSourceConfig renders a model as the configuration of data source d.
func dataSourceConfig(t *testing.T, d datasource.DataSource, model any) tfsdk.Config {
	t.Helper()
	ctx := context.Background()

	var sresp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sresp)
	holder := tfsdk.State{
		Schema: sresp.Schema,
		Raw:    tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil),
	}
	if diags := holder.Set(ctx, model); diags.HasError() {
		t.Fatalf("render config: %v", diags)
	}
	return tfsdk.Config(holder)
}
