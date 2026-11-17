package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func livePartialClients(t *testing.T, deny func(*http.Request) bool) (*rauthy.Client, *rauthy.Client) {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 for live Rauthy")
	}
	target, err := url.Parse(os.Getenv("RAUTHY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deny(r) {
			forbid(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	api, err := rauthy.New(srv.URL, os.Getenv("RAUTHY_API_KEY"), rauthy.WithRetryPolicy(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := rauthy.New(target.String(), os.Getenv("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	return api, direct
}

func TestAccPartialClient(t *testing.T) {
	ctx := context.Background()
	api, direct := livePartialClients(t, func(r *http.Request) bool {
		return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/secret")
	})
	t.Cleanup(func() {
		if deleteErr := direct.DeleteClient(ctx, "tf-partial-failure"); deleteErr != nil {
			t.Error(deleteErr)
		}
	})
	r := &clientResource{api: api}
	plan := clientModelOf(t, fullResponse())
	plan.ID = types.StringValue("tf-partial-failure")
	plan.Scim = nil
	plan.ForceMFA = types.BoolValue(false)
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
	var got clientResourceModel
	d := resp.State.Get(ctx, &got)
	if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueString() != "tf-partial-failure" {
		t.Fatalf("client lost: %v %v", resp.Diagnostics, d)
	}
	if _, readErr := direct.GetClient(ctx, got.ID.ValueString()); readErr != nil {
		t.Fatal(readErr)
	}
}

func TestAccPartialPamHost(t *testing.T) {
	ctx := context.Background()
	api, direct := livePartialClients(t, func(r *http.Request) bool {
		return r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pam/hosts/")
	})
	group, err := direct.CreatePamGroup(ctx, rauthy.PamGroupCreateRequest{Name: "partialfailure", Typ: "host"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if deleteErr := direct.DeletePamGroup(ctx, group.ID); deleteErr != nil {
			t.Error(deleteErr)
		}
	})
	r := &pamHostResource{api: api}
	plan := pamHostResourceModel{
		Hostname: types.StringValue("partialfailure"),
		GID:      types.Int64Value(group.ID),
		IPs:      types.SetUnknown(types.StringType),
		Aliases:  types.SetUnknown(types.StringType),
	}
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
	var got pamHostResourceModel
	d := resp.State.Get(ctx, &got)
	if !got.ID.IsNull() && !got.ID.IsUnknown() {
		t.Cleanup(func() {
			if deleteErr := direct.DeletePamHost(ctx, got.ID.ValueString()); deleteErr != nil {
				t.Error(deleteErr)
			}
		})
	}
	if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueString() == "" {
		t.Fatalf("host lost: %v %v", resp.Diagnostics, d)
	}
	if _, readErr := direct.GetPamHost(ctx, got.ID.ValueString()); readErr != nil {
		t.Fatal(readErr)
	}
}

func TestAccPartialPamUser(t *testing.T) {
	ctx := context.Background()
	api, direct := livePartialClients(t, func(r *http.Request) bool {
		return r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pam/users/")
	})
	name := "Round"
	user, err := direct.CreateUser(
		ctx,
		rauthy.NewUserRequest{Email: "partialfailure@example.com", GivenName: &name, Language: "en", Roles: []string{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if deleteErr := direct.DeleteUser(ctx, user.ID); deleteErr != nil {
			t.Error(deleteErr)
		}
	})
	r := &pamUserResource{api: api}
	plan := pamUserResourceModel{
		Username: types.StringValue("partialfailure"),
		Email:    types.StringValue("partialfailure@example.com"),
		Shell:    types.StringValue("/bin/zsh"),
		Groups:   types.SetUnknown(types.ObjectType{AttrTypes: pamUserGroupAttrTypes}),
	}
	resp := resource.CreateResponse{State: emptyState(t, r)}
	r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
	var got pamUserResourceModel
	d := resp.State.Get(ctx, &got)
	if got.ID.ValueInt64() != 0 {
		t.Cleanup(func() {
			if deleteErr := direct.DeletePamUser(ctx, got.ID.ValueInt64()); deleteErr != nil {
				t.Error(deleteErr)
			}
		})
	}
	if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueInt64() == 0 {
		t.Fatalf("user lost: %v %v", resp.Diagnostics, d)
	}
	if _, readErr := direct.GetPamUser(ctx, got.ID.ValueInt64()); readErr != nil {
		t.Fatal(readErr)
	}
}

func TestAccPartialRotation(t *testing.T) {
	ctx := context.Background()
	api, direct := livePartialClients(t, func(r *http.Request) bool {
		return r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/api_keys")
	})
	access := []rauthy.APIKeyAccess{{Group: "Users", AccessRights: []string{"read"}}}
	old, err := direct.CreateAPIKey(ctx, rauthy.APIKeyRequest{Name: "partialfailure", Access: access})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if deleteErr := direct.DeleteAPIKey(ctx, "partialfailure"); deleteErr != nil {
			t.Error(deleteErr)
		}
	})
	current, err := direct.GetAPIKey(ctx, "partialfailure")
	if err != nil {
		t.Fatal(err)
	}
	r := &apiKeyResource{api: api}
	state := apiKeyResourceModel{Secret: types.StringValue(old.Reveal()), SecretRotationTrigger: types.StringValue("1")}
	applyAPIKeyResponse(&state, current)
	plan := state
	plan.Secret = types.StringUnknown()
	plan.SecretRotationTrigger = types.StringValue("2")
	resp := resource.UpdateResponse{State: emptyState(t, r)}
	r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)}, &resp)
	var got apiKeyResourceModel
	var d diag.Diagnostics
	d.Append(resp.State.Get(ctx, &got)...)
	if !resp.Diagnostics.HasError() || d.HasError() || got.Secret.IsUnknown() || got.Secret.IsNull() ||
		got.Secret.Equal(state.Secret) {
		t.Fatalf("new secret lost: %v %v", resp.Diagnostics, d)
	}
	credential, err := rauthy.New(os.Getenv("RAUTHY_URL"), got.Secret.ValueString())
	if err != nil {
		t.Fatal(err)
	}
	if _, readErr := credential.GetUser(ctx, "missinguser"); !rauthy.IsNotFound(readErr) {
		t.Fatalf("saved rotated secret does not authenticate: %v", readErr)
	}
}
