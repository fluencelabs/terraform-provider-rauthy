package client

import (
	"context"
	"net/http"
	"net/url"
)

// Wire types mirroring Rauthy's `rauthy_api_types::api_keys` at v0.36.2.
//
// These are the keys this provider itself authenticates with, so managing them
// only became possible in Rauthy 0.36: every /api_keys endpoint used to demand
// an admin browser session and answered `401 No valid session` to an API key,
// and no AccessGroup could grant otherwise. 0.36 added the `ApiKeys` group, and
// a key holding it drives all of the calls below — verified against a live
// 0.36.2, not merely read out of the spec.

// APIKeyAccess is one access-group grant on a key.
type APIKeyAccess struct {
	Group string `json:"group"`
	// AccessRights is Rauthy's lowercase quartet: read, create, update,
	// delete. An empty list is accepted and means the group grants nothing.
	AccessRights []string `json:"access_rights"`
}

// APIKeyRequest is the body of both POST /api_keys and PUT /api_keys/{name}.
//
// Rauthy uses one type for both, and both are full replacements: an update that
// omits Exp does not leave the stored expiry alone, it clears it. Verified on a
// live 0.36.2 — a PUT without `exp` over a key that had one comes back with
// `"expires": null` — so the caller must resend the expiry on every update.
//
// Exp must lie in the future: Rauthy range-validates it against roughly "now",
// so a timestamp already past is a 400 rather than an immediately-dead key.
type APIKeyRequest struct {
	Name   string         `json:"name"`
	Exp    *int64         `json:"exp,omitempty"`
	Access []APIKeyAccess `json:"access"`
}

// APIKeyResponse is a key's metadata. Note what is missing: the secret. Rauthy
// discloses it exactly once, in the plain-text answer to the call that minted
// it, and never again.
type APIKeyResponse struct {
	Name    string         `json:"name"`
	Created int64          `json:"created"`
	Expires *int64         `json:"expires,omitempty"`
	Access  []APIKeyAccess `json:"access"`
}

// apiKeysListResponse wraps the listing. GET /api_keys does not answer with a
// bare array the way /scopes, /roles and /groups do — it wraps the keys in an
// object under `keys`, as GET /users/attr does under `values`.
type apiKeysListResponse struct {
	Keys []APIKeyResponse `json:"keys"`
}

func apiKeyPath(name string) string {
	return "/api_keys/" + url.PathEscape(name)
}

// CreateAPIKey issues POST /api_keys and returns the full credential in
// Rauthy's `<name>$<secret>` form — the string a client sends as
// `Authorization: API-Key ...`, not the bare secret half.
//
// The response is `text/plain`, not JSON: the body is that one line and nothing
// else, which is why this goes through doText. This is the only moment the
// secret exists outside Rauthy's database; a caller that drops it cannot ask
// for it back and must rotate instead. Requires ApiKeys:Create.
func (c *Client) CreateAPIKey(ctx context.Context, req APIKeyRequest) (string, error) {
	secret, err := c.doText(ctx, http.MethodPost, "/api_keys", req)
	if err != nil {
		return "", annotateMissingRight(err, "ApiKeys", "Create")
	}
	return secret, nil
}

// ListAPIKeys issues GET /api_keys. Requires ApiKeys:Read.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKeyResponse, error) {
	var out apiKeysListResponse
	if err := c.do(ctx, http.MethodGet, "/api_keys", nil, &out); err != nil {
		return nil, annotateMissingRight(err, "ApiKeys", "Read")
	}
	return out.Keys, nil
}

// GetAPIKey returns the key with the given name.
//
// Rauthy has no GET /api_keys/{name}. GET /api_keys/{name}/test looks like one
// but is not: it validates the *caller's own* credential against that name and
// answers `403 Wrong API Key given` for anybody else's key, so it can never
// serve as a read. As with roles, groups and auth providers, "not there" is
// therefore something this client concludes from the listing and reports as a
// synthetic 404.
func (c *Client) GetAPIKey(ctx context.Context, name string) (*APIKeyResponse, error) {
	keys, err := c.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].Name == name {
			return &keys[i], nil
		}
	}
	return nil, notFoundError("/api_keys", "api key "+name+" does not exist")
}

// UpdateAPIKey issues PUT /api_keys/{name}, replacing the key's expiry and
// access wholesale. Rauthy answers with an empty body, so the caller must
// re-read to see the stored form. Requires ApiKeys:Update.
//
// req.Name must equal name. Rauthy compares the two and rejects a mismatch with
// `400 JSON payload does not match the Name from the path`, so this endpoint
// cannot rename anything — a rename is a destroy and recreate.
func (c *Client) UpdateAPIKey(ctx context.Context, name string, req APIKeyRequest) error {
	err := c.do(ctx, http.MethodPut, apiKeyPath(name), req, nil)
	return annotateMissingRight(err, "ApiKeys", "Update")
}

// RotateAPIKeySecret issues PUT /api_keys/{name}/secret and returns the new
// credential, again in `<name>$<secret>` form and again as `text/plain`.
//
// The old secret stops working the moment this returns; there is no overlap
// window of the kind an OIDC client secret gets from cache_current_hours.
// Requires ApiKeys:Update.
func (c *Client) RotateAPIKeySecret(ctx context.Context, name string) (string, error) {
	secret, err := c.doText(ctx, http.MethodPut, apiKeyPath(name)+"/secret", nil)
	if err != nil {
		return "", annotateMissingRight(err, "ApiKeys", "Update")
	}
	return secret, nil
}

// DeleteAPIKey issues DELETE /api_keys/{name}. Requires ApiKeys:Delete.
//
// A live 0.36.2 answers 200 for a name that does not exist, so a delete is
// idempotent whether or not the caller wants it to be.
func (c *Client) DeleteAPIKey(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, apiKeyPath(name), nil, nil)
	return annotateMissingRight(err, "ApiKeys", "Delete")
}
