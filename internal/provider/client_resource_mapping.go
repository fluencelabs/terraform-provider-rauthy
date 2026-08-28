package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// applyResponse copies a client as Rauthy reports it into the model. It leaves
// the three secret-management attributes alone: `secret` is fetched from a
// different endpoint, and the rotation trigger and cache window exist only in
// Terraform, so a refresh must not clobber them.
func applyResponse(ctx context.Context, m *clientResourceModel, resp *client.ClientResponse, diags *diag.Diagnostics) {
	_ = ctx

	m.ID = types.StringValue(resp.ID)
	m.Name = optionalString(resp.Name)
	m.Enabled = types.BoolValue(resp.Enabled)
	m.Confidential = types.BoolValue(resp.Confidential)
	m.RedirectURIs = stringsToSet(resp.RedirectURIs)
	m.PostLogoutRedirectURIs = stringsToSet(resp.PostLogoutRedirectURIs)
	m.AllowedOrigins = stringsToSet(resp.AllowedOrigins)
	m.FlowsEnabled = stringsToSet(resp.FlowsEnabled)
	m.AccessTokenAlg = types.StringValue(resp.AccessTokenAlg)
	m.IDTokenAlg = types.StringValue(resp.IDTokenAlg)
	m.AuthCodeLifetime = types.Int64Value(resp.AuthCodeLifetime)
	m.AccessTokenLifetime = types.Int64Value(resp.AccessTokenLifetime)
	m.Scopes = stringsToSet(resp.Scopes)
	m.DefaultScopes = stringsToSet(resp.DefaultScopes)
	m.Challenges = stringsToSet(resp.Challenges)
	m.ForceMFA = types.BoolValue(resp.ForceMFA)
	m.ClientURI = optionalString(resp.ClientURI)
	m.Contacts = stringsToSet(resp.Contacts)
	m.BackchannelLogoutURI = optionalString(resp.BackchannelLogoutURI)
	m.RestrictGroupPrefix = optionalString(resp.RestrictGroupPrefix)

	if resp.Scim == nil {
		m.Scim = nil
	} else {
		m.Scim = &scimModel{
			BearerToken:     types.StringValue(resp.Scim.BearerToken),
			BaseURI:         types.StringValue(resp.Scim.BaseURI),
			SyncGroups:      types.BoolValue(resp.Scim.SyncGroups),
			GroupSyncPrefix: optionalString(resp.Scim.GroupSyncPrefix),
		}
	}

	_ = diags
}

// fillUnknownFromResponse resolves the attributes that are still unknown after
// planning a create.
//
// Every optional-and-computed attribute the practitioner left out of the
// configuration is unknown at that point, but PUT /clients/{id} is a full
// replacement and requires a concrete value for each. Rauthy assigned defaults
// during the POST, so those are what we send back.
func fillUnknownFromResponse(
	ctx context.Context,
	m *clientResourceModel,
	resp *client.ClientResponse,
	diags *diag.Diagnostics,
) {
	_ = ctx
	_ = diags

	if m.Enabled.IsUnknown() {
		m.Enabled = types.BoolValue(resp.Enabled)
	}
	if m.FlowsEnabled.IsUnknown() {
		m.FlowsEnabled = stringsToSet(resp.FlowsEnabled)
	}
	if m.AccessTokenAlg.IsUnknown() {
		m.AccessTokenAlg = types.StringValue(resp.AccessTokenAlg)
	}
	if m.IDTokenAlg.IsUnknown() {
		m.IDTokenAlg = types.StringValue(resp.IDTokenAlg)
	}
	if m.AuthCodeLifetime.IsUnknown() {
		m.AuthCodeLifetime = types.Int64Value(resp.AuthCodeLifetime)
	}
	if m.AccessTokenLifetime.IsUnknown() {
		m.AccessTokenLifetime = types.Int64Value(resp.AccessTokenLifetime)
	}
	if m.Scopes.IsUnknown() {
		m.Scopes = stringsToSet(resp.Scopes)
	}
	if m.DefaultScopes.IsUnknown() {
		m.DefaultScopes = stringsToSet(resp.DefaultScopes)
	}
	if m.Challenges.IsUnknown() {
		m.Challenges = stringsToSet(resp.Challenges)
	}
	if m.ForceMFA.IsUnknown() {
		m.ForceMFA = types.BoolValue(resp.ForceMFA)
	}
	if m.Secret.IsUnknown() {
		m.Secret = types.StringNull()
	}
}

// buildUpdateRequest renders the model as the body of PUT /clients/{id}.
//
// The non-pointer fields are required by Rauthy even when the practitioner did
// not configure them, which is why they are optional-and-computed in the schema
// rather than plain optional.
func buildUpdateRequest(
	ctx context.Context,
	m *clientResourceModel,
	diags *diag.Diagnostics,
) client.UpdateClientRequest {
	req := client.UpdateClientRequest{
		ID:                     m.ID.ValueString(),
		Name:                   stringPtr(m.Name),
		Confidential:           m.Confidential.ValueBool(),
		RedirectURIs:           setToStrings(ctx, m.RedirectURIs, diags),
		PostLogoutRedirectURIs: setToStrings(ctx, m.PostLogoutRedirectURIs, diags),
		AllowedOrigins:         setToStrings(ctx, m.AllowedOrigins, diags),
		Enabled:                m.Enabled.ValueBool(),
		FlowsEnabled:           setToStrings(ctx, m.FlowsEnabled, diags),
		AccessTokenAlg:         m.AccessTokenAlg.ValueString(),
		IDTokenAlg:             m.IDTokenAlg.ValueString(),
		AuthCodeLifetime:       m.AuthCodeLifetime.ValueInt64(),
		AccessTokenLifetime:    m.AccessTokenLifetime.ValueInt64(),
		Scopes:                 setToStrings(ctx, m.Scopes, diags),
		DefaultScopes:          setToStrings(ctx, m.DefaultScopes, diags),
		Challenges:             setToStrings(ctx, m.Challenges, diags),
		ForceMFA:               m.ForceMFA.ValueBool(),
		ClientURI:              stringPtr(m.ClientURI),
		Contacts:               setToStrings(ctx, m.Contacts, diags),
		BackchannelLogoutURI:   stringPtr(m.BackchannelLogoutURI),
		RestrictGroupPrefix:    stringPtr(m.RestrictGroupPrefix),
	}

	if m.Scim != nil {
		req.Scim = &client.ScimClientRequestResponse{
			BearerToken:     m.Scim.BearerToken.ValueString(),
			BaseURI:         m.Scim.BaseURI.ValueString(),
			SyncGroups:      m.Scim.SyncGroups.ValueBool(),
			GroupSyncPrefix: stringPtr(m.Scim.GroupSyncPrefix),
		}
	}

	return req
}
