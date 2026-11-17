package provider

import (
	"context"
	"os"
	"testing"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAccUserAttribute_externalRename(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run acceptance tests")
	}
	api, err := rauthy.New(os.Getenv("RAUTHY_URL"), os.Getenv("RAUTHY_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, name := range []string{"tp4-old", "tp4-older"} {
		if _, createErr := api.CreateUserAttr(ctx, rauthy.UserAttrRequest{Name: name}); createErr != nil {
			t.Fatal(createErr)
		}
	}
	defer func() {
		if deleteErr := api.DeleteUserAttr(ctx, "tp4-older"); deleteErr != nil {
			t.Error(deleteErr)
		}
		if deleteErr := api.DeleteUserAttr(ctx, "tp4-renamed"); deleteErr != nil {
			t.Error(deleteErr)
		}
	}()
	// An external rename leaves the old per-name cache even though it is absent
	// from GET /users/attr. A saved destroy plan can still carry that old key.
	if renameErr := api.UpdateUserAttr(ctx, "tp4-old", rauthy.UserAttrRequest{Name: "tp4-renamed"}); renameErr != nil {
		t.Fatal(renameErr)
	}
	if _, lookupErr := api.GetUserAttr(ctx, "tp4-old"); !rauthy.IsNotFound(lookupErr) {
		t.Fatalf("old key still listed: %v", lookupErr)
	}
	scope, createErr := api.CreateScope(ctx, rauthy.ScopeRequest{
		Scope: "tp4-external", AttrIncludeAccess: []string{"tp4-older"}, AttrIncludeID: []string{"tp4-older"},
	})
	if createErr != nil {
		t.Fatal(createErr)
	}
	defer func() {
		if deleteErr := api.DeleteScope(ctx, scope.ID); deleteErr != nil {
			t.Error(deleteErr)
		}
	}()
	r := &userAttributeResource{api: api}
	state := resourceState(
		t,
		r,
		userAttributeResourceModel{ID: types.StringValue("tp4-old"), Name: types.StringValue("tp4-old")},
	)
	resp := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("absent attribute remains in state")
	}
	got, readErr := api.GetScope(ctx, scope.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(got.AttrIncludeAccess) != 1 || got.AttrIncludeAccess[0] != "tp4-older" ||
		len(got.AttrIncludeID) != 1 || got.AttrIncludeID[0] != "tp4-older" {
		t.Fatalf("foreign mapping corrupted: %+v", got)
	}
}
