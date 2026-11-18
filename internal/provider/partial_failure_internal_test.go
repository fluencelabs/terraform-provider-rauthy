package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func forbid(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":"Forbidden","message":"missing permission"}`, http.StatusForbidden)
}

func TestPartialFailure(t *testing.T) {
	ctx := context.Background()
	t.Run("API key rotation", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer("PUT /api_keys/deploy", nil)
		f.handle(
			"PUT /api_keys/deploy/secret",
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("deploy$new")) },
		)
		f.handle("GET /api_keys", forbid)
		r := &apiKeyResource{api: api}
		state := apiKeyResourceModel{
			Name:                  types.StringValue("deploy"),
			Access:                accessSetOf(t, rauthy.APIKeyAccess{Group: "Users", AccessRights: []string{"read"}}),
			CreatedAt:             types.Int64Value(1),
			Secret:                types.StringValue("deploy$old"),
			SecretRotationTrigger: types.StringValue("1"),
		}
		plan := state
		plan.Secret = types.StringUnknown()
		plan.SecretRotationTrigger = types.StringValue("2")
		resp := resource.UpdateResponse{State: emptyState(t, r)}
		r.Update(
			ctx,
			resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: resourceState(t, r, &state)},
			&resp,
		)
		var got apiKeyResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.Secret.ValueString() != "deploy$new" ||
			got.SecretRotationTrigger.ValueString() != "2" {
			t.Fatalf("rotation lost: %+v %v %v", got, resp.Diagnostics, d)
		}
	})
	t.Run("client secret read", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		c := fullResponse()
		f.answer("POST /clients", c)
		f.answer("PUT /clients/example-app", c)
		f.handle("POST /clients/example-app/secret", forbid)
		r := &clientResource{api: api}
		plan := clientModelOf(t, c)
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		var got clientResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueString() != c.ID {
			t.Fatalf("created client lost: %v %v", resp.Diagnostics, d)
		}
	})
	t.Run("PAM host read", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer("POST /pam/hosts", rauthy.PamHostSimpleResponse{ID: "host1", Name: "build01"})
		f.handle("GET /pam/hosts/host1", forbid)
		r := &pamHostResource{api: api}
		plan := pamHostResourceModel{
			Hostname: types.StringValue("build01"),
			GID:      types.Int64Value(100001),
			IPs:      types.SetUnknown(types.StringType),
			Aliases:  types.SetUnknown(types.StringType),
		}
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		var got pamHostResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueString() != "host1" {
			t.Fatalf("created host lost: %v %v", resp.Diagnostics, d)
		}
	})
	t.Run("PAM user read", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer(
			"POST /pam/users",
			rauthy.PamUserResponse{
				ID:      100001,
				Name:    "alice",
				Email:   "alice@example.com",
				GID:     100001,
				Shell:   "/bin/bash",
				HomeDir: "/home/alice",
			},
		)
		f.handle("GET /pam/users/100001", forbid)
		r := &pamUserResource{api: api}
		plan := pamUserResourceModel{
			Username: types.StringValue("alice"),
			Email:    types.StringValue("alice@example.com"),
			Shell:    types.StringValue("/bin/zsh"),
			Groups:   types.SetUnknown(types.ObjectType{AttrTypes: pamUserGroupAttrTypes}),
		}
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		var got pamUserResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueInt64() != 100001 {
			t.Fatalf("created user lost: %v %v", resp.Diagnostics, d)
		}
	})
}

func TestPartialFollowups(t *testing.T) {
	ctx := context.Background()
	t.Run("attribute update", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer("PUT /users/attr/attr", nil)
		f.handle("GET /users/attr", forbid)
		r := &userAttributeResource{api: api}
		plan := userAttributeResourceModel{
			ID:           types.StringValue("attr"),
			Name:         types.StringValue("attr"),
			DefaultValue: newJSONString(`42`),
		}
		prior := plan
		prior.DefaultValue = nullJSONString()
		state := resourceState(t, r, &prior)
		resp := resource.UpdateResponse{State: state}
		r.Update(ctx, resource.UpdateRequest{
			Plan: resourcePlan(t, r, &plan), State: state,
		}, &resp)
		if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(resourceState(t, r, &plan).Raw) {
			t.Fatalf("failed readback must retain the applied plan: %v, %v", resp.Diagnostics, resp.State.Raw)
		}
		var body rauthy.UserAttrRequest
		f.lastBody(t, "PUT /users/attr/attr", &body)
		if string(body.DefaultValue) != "42" {
			t.Errorf("PUT default_value = %s, want 42", body.DefaultValue)
		}
	})
	t.Run("attribute PUT failure", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.handle("PUT /users/attr/attr", forbid)
		r := &userAttributeResource{api: api}
		prior := userAttributeResourceModel{ID: types.StringValue("attr"), Name: types.StringValue("attr")}
		plan := prior
		plan.DefaultValue = newJSONString("42")
		state := resourceState(t, r, &prior)
		resp := resource.UpdateResponse{State: state}
		r.Update(ctx, resource.UpdateRequest{Plan: resourcePlan(t, r, &plan), State: state}, &resp)
		if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
			t.Fatalf("failed PUT must preserve prior state: %v, %v", resp.Diagnostics, resp.State.Raw)
		}
		if len(f.bodies("GET /users/attr")) != 0 {
			t.Error("failed PUT triggered a readback")
		}
	})
	t.Run("image upload", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer("PUT /clients/app/logo", nil)
		f.handle("GET /clients/app/logo", forbid)
		r := &clientImageResource{api: api, kind: rauthy.ImageLogo}
		plan := clientImageResourceModel{
			ClientID:      types.StringValue("app"),
			ContentType:   types.StringValue(rauthy.MimeSVG),
			ContentBase64: types.StringValue("PHN2Zy8+"),
		}
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		var got clientImageResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.ClientID.ValueString() != "app" ||
			!got.ContentBase64.Equal(plan.ContentBase64) {
			t.Fatalf("uploaded image lost: %v %v", resp.Diagnostics, d)
		}
	})
	t.Run("blacklist create", func(t *testing.T) {
		f, api := newFakeRauthy(t)
		f.answer("POST /blacklist", nil)
		f.handle("GET /blacklist", forbid)
		r := &blacklistIPResource{api: api}
		plan := blacklistIPResourceModel{IP: types.StringValue("192.0.2.1"), Exp: types.Int64Value(1900000000)}
		resp := resource.CreateResponse{State: emptyState(t, r)}
		r.Create(ctx, resource.CreateRequest{Plan: resourcePlan(t, r, &plan)}, &resp)
		var got blacklistIPResourceModel
		d := resp.State.Get(ctx, &got)
		if !resp.Diagnostics.HasError() || d.HasError() || got.ID.ValueString() != "192.0.2.1" {
			t.Fatalf("created blacklist entry lost: %v %v", resp.Diagnostics, d)
		}
	})
}
