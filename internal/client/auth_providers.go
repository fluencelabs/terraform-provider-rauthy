package client

import (
	"context"
	"net/http"
	"net/url"
)

// Wire types mirroring Rauthy's `rauthy_api_types::providers` at v0.36.2.

// AuthProviderRequest is the body of both POST /providers/create and
// PUT /providers/{id}.
//
// Rauthy uses one type for both, and PUT is a full replacement: every
// non-pointer field is required by the deserializer, and — this is the part
// that bites — a PUT that omits ClientSecret does not leave the stored secret
// alone, it erases it. Verified against a live 0.36.2: after a PUT without the
// field, the provider comes back with `"client_secret": null`. So the caller
// must resend the secret on every update.
type AuthProviderRequest struct {
	Name                  string  `json:"name"`
	Typ                   string  `json:"typ"`
	Enabled               bool    `json:"enabled"`
	Issuer                string  `json:"issuer"`
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	TokenEndpoint         string  `json:"token_endpoint"`
	UserinfoEndpoint      string  `json:"userinfo_endpoint"`
	JwksEndpoint          *string `json:"jwks_endpoint,omitempty"`
	ClientID              string  `json:"client_id"`
	ClientSecret          *string `json:"client_secret,omitempty"`
	Scope                 string  `json:"scope"`
	AdminClaimPath        *string `json:"admin_claim_path,omitempty"`
	AdminClaimValue       *string `json:"admin_claim_value,omitempty"`
	MfaClaimPath          *string `json:"mfa_claim_path,omitempty"`
	MfaClaimValue         *string `json:"mfa_claim_value,omitempty"`
	UsePKCE               bool    `json:"use_pkce"`
	ClientSecretBasic     bool    `json:"client_secret_basic"`
	ClientSecretPost      bool    `json:"client_secret_post"`
	AutoOnboarding        bool    `json:"auto_onboarding"`
	AutoLink              bool    `json:"auto_link"`
}

// AuthProviderResponse is an upstream provider as Rauthy reports it.
//
// ClientSecret is returned in the clear, so unlike an OIDC client's secret this
// one does not need a separate endpoint — and unlike most write-only secrets it
// does survive an import.
//
// Scope is NOT the string that was sent: see splitProviderScope in the provider
// package. Rauthy stores the scope list `+`-joined and hands it back that way,
// which its own request validator then refuses.
type AuthProviderResponse struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	Typ                   string  `json:"typ"`
	Enabled               bool    `json:"enabled"`
	Issuer                string  `json:"issuer"`
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	TokenEndpoint         string  `json:"token_endpoint"`
	UserinfoEndpoint      string  `json:"userinfo_endpoint"`
	JwksEndpoint          *string `json:"jwks_endpoint,omitempty"`
	ClientID              string  `json:"client_id"`
	ClientSecret          *string `json:"client_secret,omitempty"`
	Scope                 string  `json:"scope"`
	AdminClaimPath        *string `json:"admin_claim_path,omitempty"`
	AdminClaimValue       *string `json:"admin_claim_value,omitempty"`
	MfaClaimPath          *string `json:"mfa_claim_path,omitempty"`
	MfaClaimValue         *string `json:"mfa_claim_value,omitempty"`
	UsePKCE               bool    `json:"use_pkce"`
	ClientSecretBasic     bool    `json:"client_secret_basic"`
	ClientSecretPost      bool    `json:"client_secret_post"`
	AutoOnboarding        bool    `json:"auto_onboarding"`
	AutoLink              bool    `json:"auto_link"`
}

// AuthProviderLookupRequest is the body of POST /providers/lookup. Exactly one
// of the two fields is expected; Issuer is resolved by appending the
// well-known path, MetadataURL is fetched as given.
type AuthProviderLookupRequest struct {
	Issuer      *string `json:"issuer,omitempty"`
	MetadataURL *string `json:"metadata_url,omitempty"`
}

// AuthProviderLookupResponse is the discovery result of POST /providers/lookup.
//
// Its Scope is space-separated, not `+`-joined the way AuthProviderResponse's
// is — the two endpoints disagree about the same field on the same server.
type AuthProviderLookupResponse struct {
	Issuer                string  `json:"issuer"`
	AuthorizationEndpoint string  `json:"authorization_endpoint"`
	TokenEndpoint         string  `json:"token_endpoint"`
	UserinfoEndpoint      string  `json:"userinfo_endpoint"`
	JwksEndpoint          *string `json:"jwks_endpoint,omitempty"`
	Scope                 string  `json:"scope"`
	UsePKCE               bool    `json:"use_pkce"`
	ClientSecretBasic     bool    `json:"client_secret_basic"`
	ClientSecretPost      bool    `json:"client_secret_post"`
}

// AuthProviderLinkedUser is one account federated through a provider, as
// reported by GET /providers/{id}/delete_safe.
type AuthProviderLinkedUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func authProviderPath(id string) string {
	return "/providers/" + url.PathEscape(id)
}

// CreateAuthProvider issues POST /providers/create.
//
// Note the path: POST /providers is the *list* endpoint, not the create one.
// Rauthy serves the listing on POST because the admin UI posts a (currently
// empty) filter body to it, and creation was pushed down to /providers/create
// to get out of its way. Requires AuthProviders:Create.
func (c *Client) CreateAuthProvider(
	ctx context.Context,
	req AuthProviderRequest,
) (*AuthProviderResponse, error) {
	var out AuthProviderResponse
	if err := c.do(ctx, http.MethodPost, "/providers/create", req, &out); err != nil {
		return nil, annotateMissingRight(err, "AuthProviders", "Create")
	}
	return &out, nil
}

// ListAuthProviders issues POST /providers. See CreateAuthProvider for why the
// listing is a POST. Requires AuthProviders:Read.
func (c *Client) ListAuthProviders(ctx context.Context) ([]AuthProviderResponse, error) {
	var out []AuthProviderResponse
	if err := c.do(ctx, http.MethodPost, "/providers", nil, &out); err != nil {
		return nil, annotateMissingRight(err, "AuthProviders", "Read")
	}
	return out, nil
}

// GetAuthProvider returns the provider with the given id.
//
// Rauthy has no GET /providers/{id} — the only full read is the listing — so,
// as with roles and groups, "not there" is something this client concludes and
// reports as a synthetic 404.
func (c *Client) GetAuthProvider(ctx context.Context, id string) (*AuthProviderResponse, error) {
	providers, err := c.ListAuthProviders(ctx)
	if err != nil {
		return nil, err
	}
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i], nil
		}
	}
	return nil, notFoundError("/providers", "auth provider "+id+" does not exist")
}

// GetAuthProviderByName returns the single provider with the given name.
//
// Rauthy does not require provider names to be unique, so an ambiguous name is
// an error rather than an arbitrary pick.
func (c *Client) GetAuthProviderByName(ctx context.Context, name string) (*AuthProviderResponse, error) {
	providers, err := c.ListAuthProviders(ctx)
	if err != nil {
		return nil, err
	}
	var found *AuthProviderResponse
	for i := range providers {
		if providers[i].Name != name {
			continue
		}
		if found != nil {
			return nil, notFoundError("/providers",
				"more than one auth provider is named "+name+"; look it up by id instead")
		}
		found = &providers[i]
	}
	if found == nil {
		return nil, notFoundError("/providers", "no auth provider named "+name)
	}
	return found, nil
}

// UpdateAuthProvider issues PUT /providers/{id}, replacing the provider's
// configuration wholesale. Rauthy answers with an empty body, so the caller
// must re-read to see the stored form. Requires AuthProviders:Update.
func (c *Client) UpdateAuthProvider(ctx context.Context, id string, req AuthProviderRequest) error {
	err := c.do(ctx, http.MethodPut, authProviderPath(id), req, nil)
	return annotateMissingRight(err, "AuthProviders", "Update")
}

// DeleteAuthProvider issues DELETE /providers/{id}. Requires
// AuthProviders:Delete.
//
// A live 0.36.2 answers 200 for an id that does not exist, so a delete is
// idempotent whether or not the caller wants it to be.
func (c *Client) DeleteAuthProvider(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, authProviderPath(id), nil, nil)
	return annotateMissingRight(err, "AuthProviders", "Delete")
}

// AuthProviderLinkedUsers issues GET /providers/{id}/delete_safe, returning the
// accounts that currently log in through this provider. An empty slice means
// deleting it strands nobody.
func (c *Client) AuthProviderLinkedUsers(ctx context.Context, id string) ([]AuthProviderLinkedUser, error) {
	var out []AuthProviderLinkedUser
	if err := c.do(ctx, http.MethodGet, authProviderPath(id)+"/delete_safe", nil, &out); err != nil {
		return nil, annotateMissingRight(err, "AuthProviders", "Read")
	}
	return out, nil
}

// LookupAuthProvider issues POST /providers/lookup, asking Rauthy to fetch an
// upstream's OIDC discovery document and report the endpoints it advertises.
//
// This is a network call made by the Rauthy server, not by Terraform: it fails
// if the Rauthy instance cannot reach the issuer, even when the machine running
// Terraform can.
func (c *Client) LookupAuthProvider(
	ctx context.Context,
	req AuthProviderLookupRequest,
) (*AuthProviderLookupResponse, error) {
	var out AuthProviderLookupResponse
	if err := c.do(ctx, http.MethodPost, "/providers/lookup", req, &out); err != nil {
		return nil, annotateMissingRight(err, "AuthProviders", "Read")
	}
	return &out, nil
}
