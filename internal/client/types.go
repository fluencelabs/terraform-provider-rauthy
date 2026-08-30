package client

// Wire types mirroring Rauthy's `rauthy_api_types::clients` at v0.36.2.
//
// Field sets and JSON names are load-bearing: the API silently ignores unknown
// fields on POST /clients and rejects a PUT that omits a required one. The
// contract tests in this package validate both directions against the vendored
// OpenAPI spec.

// NewClientRequest is the body of POST /clients.
//
// This is deliberately tiny: Rauthy's NewClientRequest carries only these
// fields and silently drops everything else. The full configuration has to
// follow in a PUT.
type NewClientRequest struct {
	ID                     string   `json:"id"`
	Name                   *string  `json:"name,omitempty"`
	Confidential           bool     `json:"confidential"`
	RedirectURIs           []string `json:"redirect_uris"`
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris,omitempty"`
}

// ScimClientRequestResponse is the SCIM sub-object, shared by the update
// request and the client response.
type ScimClientRequestResponse struct {
	BearerToken     string  `json:"bearer_token"`
	BaseURI         string  `json:"base_uri"`
	SyncGroups      bool    `json:"sync_groups"`
	GroupSyncPrefix *string `json:"group_sync_prefix,omitempty"`
}

// UpdateClientRequest is the body of PUT /clients/{id}.
//
// PUT is a full replacement: every non-pointer field below is required by
// Rauthy's deserializer, so the caller must supply a value even for settings it
// does not care about. Omitting Enabled or FlowsEnabled is a 400, not a no-op.
type UpdateClientRequest struct {
	ID                     string                     `json:"id"`
	Name                   *string                    `json:"name,omitempty"`
	Confidential           bool                       `json:"confidential"`
	RedirectURIs           []string                   `json:"redirect_uris"`
	PostLogoutRedirectURIs []string                   `json:"post_logout_redirect_uris,omitempty"`
	AllowedOrigins         []string                   `json:"allowed_origins,omitempty"`
	Enabled                bool                       `json:"enabled"`
	FlowsEnabled           []string                   `json:"flows_enabled"`
	AccessTokenAlg         string                     `json:"access_token_alg"`
	IDTokenAlg             string                     `json:"id_token_alg"`
	AuthCodeLifetime       int64                      `json:"auth_code_lifetime"`
	AccessTokenLifetime    int64                      `json:"access_token_lifetime"`
	Scopes                 []string                   `json:"scopes"`
	DefaultScopes          []string                   `json:"default_scopes"`
	Challenges             []string                   `json:"challenges,omitempty"`
	ForceMFA               bool                       `json:"force_mfa"`
	ClientURI              *string                    `json:"client_uri,omitempty"`
	Contacts               []string                   `json:"contacts,omitempty"`
	BackchannelLogoutURI   *string                    `json:"backchannel_logout_uri,omitempty"`
	RestrictGroupPrefix    *string                    `json:"restrict_group_prefix,omitempty"`
	Scim                   *ScimClientRequestResponse `json:"scim,omitempty"`
}

// ClientResponse is the body returned by GET/POST/PUT on /clients[/{id}].
//
//nolint:revive // mirrors Rauthy's own type name; renaming would obscure the mapping
type ClientResponse struct {
	ID                     string                     `json:"id"`
	Name                   *string                    `json:"name,omitempty"`
	Enabled                bool                       `json:"enabled"`
	Confidential           bool                       `json:"confidential"`
	RedirectURIs           []string                   `json:"redirect_uris"`
	PostLogoutRedirectURIs []string                   `json:"post_logout_redirect_uris,omitempty"`
	AllowedOrigins         []string                   `json:"allowed_origins,omitempty"`
	FlowsEnabled           []string                   `json:"flows_enabled"`
	AccessTokenAlg         string                     `json:"access_token_alg"`
	IDTokenAlg             string                     `json:"id_token_alg"`
	AuthCodeLifetime       int64                      `json:"auth_code_lifetime"`
	AccessTokenLifetime    int64                      `json:"access_token_lifetime"`
	Scopes                 []string                   `json:"scopes"`
	DefaultScopes          []string                   `json:"default_scopes"`
	Challenges             []string                   `json:"challenges,omitempty"`
	ForceMFA               bool                       `json:"force_mfa"`
	ClientURI              *string                    `json:"client_uri,omitempty"`
	Contacts               []string                   `json:"contacts,omitempty"`
	BackchannelLogoutURI   *string                    `json:"backchannel_logout_uri,omitempty"`
	RestrictGroupPrefix    *string                    `json:"restrict_group_prefix,omitempty"`
	Scim                   *ScimClientRequestResponse `json:"scim,omitempty"`
}

// ClientSecretRequest is the body of PUT /clients/{id}/secret. A nil
// CacheCurrentHours means the previous secret stops being accepted immediately.
//
//nolint:revive // mirrors Rauthy's own type name; renaming would obscure the mapping
type ClientSecretRequest struct {
	CacheCurrentHours *int64 `json:"cache_current_hours,omitempty"`
}

// ClientSecretResponse is returned by POST and PUT on /clients/{id}/secret.
// Secret is nil for a public client.
//
//nolint:revive // mirrors Rauthy's own type name; renaming would obscure the mapping
type ClientSecretResponse struct {
	ID           string  `json:"id"`
	Confidential bool    `json:"confidential"`
	Secret       *string `json:"secret,omitempty"`
}
