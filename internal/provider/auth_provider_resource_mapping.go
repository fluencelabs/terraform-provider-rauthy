package provider

import (
	"context"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// joinAuthProviderScopes renders the scope set as the string Rauthy's request
// validator accepts: a single space between elements.
//
// The elements are sorted so that a set — which has no order — always produces
// the same wire value. Without that, an unrelated change could rewrite `scope`
// on the server for no reason.
func joinAuthProviderScopes(ctx context.Context, set types.Set, diags *diag.Diagnostics) string {
	values := setToStrings(ctx, set, diags)
	slices.Sort(values)
	return strings.Join(values, " ")
}

// splitAuthProviderScope parses the scope string Rauthy hands back.
//
// This is the seventh place where a live Rauthy contradicts its own document,
// and the only one so far where a value cannot be sent back as it was received.
// `scope` is validated on the way in against ^[a-zA-Z0-9-_/:\s*]{0,512}$ —
// whitespace-separated, and `+` is not in the character class — but Rauthy
// stores and returns the list `+`-joined, because that is the form it splices
// into the upstream authorization URL. Feeding a read straight back into a
// write is therefore a guaranteed 400:
//
//	PUT /providers/{id} {"scope": "openid+profile+email", ...}
//	400 Payload validation error: ValidationErrors({"scope": ...})
//
// Two further wrinkles, both observed on 0.36.2 rather than documented. The
// join collapses runs of spaces, so "openid  profile" comes back as
// "openid+profile"; and only the space character is translated, so a tab
// survives verbatim as a separator. POST /providers/lookup, meanwhile, returns
// the same field space-separated with a trailing space — the two endpoints do
// not agree with each other.
//
// Modelling the attribute as a set of scopes rather than a string makes all of
// that the provider's problem instead of the practitioner's: splitting on `+`
// is unambiguous because the request validator forbids `+` inside a scope, and
// any of the whitespace forms parses the same way.
func splitAuthProviderScope(scope string) []string {
	// Never nil: `scopes` is a required attribute, so an empty result has to
	// be an empty set rather than a null one.
	fields := strings.Fields(strings.ReplaceAll(scope, "+", " "))
	if fields == nil {
		return []string{}
	}
	return fields
}

// buildAuthProviderRequest renders the model as the body of POST
// /providers/create and PUT /providers/{id}, which share one type.
func buildAuthProviderRequest(
	ctx context.Context,
	m *authProviderResourceModel,
	diags *diag.Diagnostics,
) client.AuthProviderRequest {
	return client.AuthProviderRequest{
		Name:                  m.Name.ValueString(),
		Typ:                   m.Type.ValueString(),
		Enabled:               m.Enabled.ValueBool(),
		Issuer:                m.Issuer.ValueString(),
		AuthorizationEndpoint: m.AuthorizationEndpoint.ValueString(),
		TokenEndpoint:         m.TokenEndpoint.ValueString(),
		UserinfoEndpoint:      m.UserinfoEndpoint.ValueString(),
		JwksEndpoint:          stringPtr(m.JwksEndpoint),
		ClientID:              m.ClientID.ValueString(),
		// Always resent, never omitted: a PUT without this field erases the
		// stored secret rather than leaving it alone.
		ClientSecret:      stringPtr(m.ClientSecret),
		Scope:             joinAuthProviderScopes(ctx, m.Scopes, diags),
		AdminClaimPath:    stringPtr(m.AdminClaimPath),
		AdminClaimValue:   stringPtr(m.AdminClaimValue),
		MfaClaimPath:      stringPtr(m.MfaClaimPath),
		MfaClaimValue:     stringPtr(m.MfaClaimValue),
		UsePKCE:           m.UsePKCE.ValueBool(),
		ClientSecretBasic: m.ClientSecretBasic.ValueBool(),
		ClientSecretPost:  m.ClientSecretPost.ValueBool(),
		AutoOnboarding:    m.AutoOnboarding.ValueBool(),
		AutoLink:          m.AutoLink.ValueBool(),
	}
}

// applyAuthProviderResponse copies a provider as Rauthy reports it into the
// model.
func applyAuthProviderResponse(m *authProviderResourceModel, resp *client.AuthProviderResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Type = types.StringValue(resp.Typ)
	m.Enabled = types.BoolValue(resp.Enabled)
	m.Issuer = types.StringValue(resp.Issuer)
	m.AuthorizationEndpoint = types.StringValue(resp.AuthorizationEndpoint)
	m.TokenEndpoint = types.StringValue(resp.TokenEndpoint)
	m.UserinfoEndpoint = types.StringValue(resp.UserinfoEndpoint)
	m.JwksEndpoint = optionalString(resp.JwksEndpoint)
	m.ClientID = types.StringValue(resp.ClientID)
	m.ClientSecret = optionalString(resp.ClientSecret)
	m.Scopes = stringsToSet(splitAuthProviderScope(resp.Scope))
	m.AdminClaimPath = optionalString(resp.AdminClaimPath)
	m.AdminClaimValue = optionalString(resp.AdminClaimValue)
	m.MfaClaimPath = optionalString(resp.MfaClaimPath)
	m.MfaClaimValue = optionalString(resp.MfaClaimValue)
	m.UsePKCE = types.BoolValue(resp.UsePKCE)
	m.ClientSecretBasic = types.BoolValue(resp.ClientSecretBasic)
	m.ClientSecretPost = types.BoolValue(resp.ClientSecretPost)
	m.AutoOnboarding = types.BoolValue(resp.AutoOnboarding)
	m.AutoLink = types.BoolValue(resp.AutoLink)
}
