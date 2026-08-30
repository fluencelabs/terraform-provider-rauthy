package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// apiKeyAccessAttrTypes is the shape of one `access` element. It is spelled out
// once here because both directions of the mapping need it, and a mismatch
// between them surfaces as an opaque type error at apply time.
//
//nolint:gochecknoglobals // a type descriptor, not mutable state
var apiKeyAccessAttrTypes = map[string]attr.Type{
	"group":         types.StringType,
	"access_rights": types.SetType{ElemType: types.StringType},
}

func apiKeyAccessObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: apiKeyAccessAttrTypes}
}

// applyAPIKeyResponse copies a key as Rauthy reports it into the model.
//
// It leaves `secret` and `secret_rotation_trigger` alone: the first is
// unreadable after creation, the second exists only in Terraform.
func applyAPIKeyResponse(m *apiKeyResourceModel, resp *client.APIKeyResponse) {
	m.Name = types.StringValue(resp.Name)
	m.CreatedAt = types.Int64Value(resp.Created)
	if resp.Expires == nil {
		m.ExpiresAt = types.Int64Null()
	} else {
		m.ExpiresAt = types.Int64Value(*resp.Expires)
	}
	m.Access = apiKeyAccessToSet(resp.Access)
}

// buildAPIKeyRequest renders the model as the body of POST /api_keys or
// PUT /api_keys/{name}.
//
// Both are full replacements and both carry the name — the PUT compares it
// against the path and rejects a mismatch — so the same body serves for either.
func buildAPIKeyRequest(
	ctx context.Context,
	m *apiKeyResourceModel,
	diags *diag.Diagnostics,
) client.APIKeyRequest {
	return client.APIKeyRequest{
		Name:   m.Name.ValueString(),
		Exp:    int64Ptr(m.ExpiresAt),
		Access: apiKeyAccessFromSet(ctx, m.Access, diags),
	}
}

// apiKeyAccessFromSet converts the `access` set into the wire form.
//
// A nil result would marshal as `"access": null`, which Rauthy's deserializer
// refuses, so an unset or empty set becomes an empty slice: a key with no
// grants at all is something Rauthy accepts and stores.
func apiKeyAccessFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []client.APIKeyAccess {
	out := []client.APIKeyAccess{}
	if set.IsNull() || set.IsUnknown() {
		return out
	}

	var models []apiKeyAccessModel
	diags.Append(set.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return out
	}

	for i := range models {
		rights := setToStrings(ctx, models[i].AccessRights, diags)
		if rights == nil {
			rights = []string{}
		}
		out = append(out, client.APIKeyAccess{
			Group:        models[i].Group.ValueString(),
			AccessRights: rights,
		})
	}
	return out
}

// apiKeyAccessToSet converts the wire form back into the `access` set.
//
// Rauthy echoes the grants back exactly as they were sent, order and all, and
// does not normalise them — so the set on both sides is Terraform's doing, not
// the server's, and it is what keeps a reordered list from reading as a diff.
func apiKeyAccessToSet(access []client.APIKeyAccess) types.Set {
	elems := make([]attr.Value, 0, len(access))
	for i := range access {
		rights := access[i].AccessRights
		if rights == nil {
			rights = []string{}
		}
		elems = append(elems, types.ObjectValueMust(apiKeyAccessAttrTypes, map[string]attr.Value{
			"group":         types.StringValue(access[i].Group),
			"access_rights": stringsToSet(rights),
		}))
	}
	return types.SetValueMust(apiKeyAccessObjectType(), elems)
}
