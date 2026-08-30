package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Wire types for Rauthy's PAM subsystem (`rauthy_api_types::pam` at v0.36.2),
// the part that makes Rauthy an authentication source for hosts and SSH.
//
// Only the configuration surface is modelled here. Everything else under /pam —
// login, mfa, password, preflight, getent, whoami and users/self — is the
// runtime authentication flow a PAM/NSS client drives with a host secret, not
// something Terraform declares.
//
// THREE DIVERGENCES between the live 0.36.2 API and the OpenAPI document it
// publishes were found while writing this, all verified with curl against a
// real instance. They are called out at the method that hits each one; the
// short version is that the document's /pam section is stale, so nothing in
// here may be trusted to the spec alone.

// PamGroupType is the `typ` of a PAM group.
//
// A group's type decides what it may be attached to: `host` groups are what a
// host's gid points at, `user` groups are the personal group Rauthy creates
// alongside every PAM user, `generic` groups are the ordinary supplementary
// ones, `local` mirrors a group that exists only on the host, and `immutable`
// marks the built-ins (`wheel-rauthy`). A live server accepts all five on
// create, including `immutable` and `user`, so the restraint is the operator's.
type PamGroupType = string

// PamGroupResponse is one PAM group as Rauthy returns it. The id is the numeric
// gid, not an opaque string: PAM groups live in the host's group namespace.
type PamGroupResponse struct {
	ID   int64        `json:"id"`
	Name string       `json:"name"`
	Typ  PamGroupType `json:"typ"`
}

// PamGroupCreateRequest is the body of POST /pam/groups.
type PamGroupCreateRequest struct {
	Name string       `json:"name"`
	Typ  PamGroupType `json:"typ"`
}

// PamUserResponse is a PAM user as the create endpoint and the listing return
// it. Gid is the user's *personal* group, which Rauthy creates for it and does
// not let anyone choose; supplementary membership lives in the details.
type PamUserResponse struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	GID     int64  `json:"gid"`
	Email   string `json:"email"`
	Shell   string `json:"shell"`
	HomeDir string `json:"home_dir"`
}

// PamGroupUserLink is one membership row: which user, in which group, with
// wheel (sudo) or without. The uid is redundant when the row is addressed under
// /pam/users/{uid}, but the server requires it in the body regardless.
type PamGroupUserLink struct {
	UID   int64 `json:"uid"`
	GID   int64 `json:"gid"`
	Wheel bool  `json:"wheel"`
}

// PamUserDetailsResponse is GET /pam/users/{uid}: the user plus its group
// memberships. AuthorizedKeys is deliberately absent — SSH keys are added by
// the user through /pam/users/self/authorized_keys, never by an admin, so they
// are runtime state rather than configuration.
type PamUserDetailsResponse struct {
	ID      int64              `json:"id"`
	Name    string             `json:"name"`
	GID     int64              `json:"gid"`
	Email   string             `json:"email"`
	Shell   string             `json:"shell"`
	HomeDir string             `json:"home_dir"`
	Groups  []PamGroupUserLink `json:"groups"`
}

// PamUserCreateRequest is the body of POST /pam/users. The email must belong to
// an existing Rauthy user that is not linked to a PAM user yet — the endpoint
// links the two rather than creating an identity — and a mismatch surfaces as a
// bare 404 "no rows returned".
type PamUserCreateRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// PamUserUpdateRequest is the body of PUT /pam/users/{uid}.
//
// Groups is a full replacement, not a delta: whatever is sent becomes the
// membership set. That includes the personal group — a PUT with an empty list
// leaves the user in no groups at all, verified against a live 0.36.2 — so the
// caller owns the whole set and must resend every row it wants to keep.
//
// Username and email cannot be changed here; the update surface is only the
// shell, the home directory and the memberships.
type PamUserUpdateRequest struct {
	Shell   string             `json:"shell"`
	HomeDir *string            `json:"home_dir,omitempty"`
	Groups  []PamGroupUserLink `json:"groups"`
}

// PamHostSimpleResponse is one host in the listing.
//
// Addresses carries what the details endpoint calls `ips`, under a different
// name, and the OpenAPI document types both as a plain `string` — a live server
// sends an array of strings. Same shape, two names, one wrong type in the
// document.
type PamHostSimpleResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
	Addresses []string `json:"addresses"`
}

// PamHostDetailsResponse is GET /pam/hosts/{id}.
type PamHostDetailsResponse struct {
	ID                string   `json:"id"`
	Hostname          string   `json:"hostname"`
	GID               int64    `json:"gid"`
	ForceMfa          bool     `json:"force_mfa"`
	LocalPasswordOnly bool     `json:"local_password_only"`
	Notes             *string  `json:"notes"`
	IPs               []string `json:"ips"`
	Aliases           []string `json:"aliases"`
}

// PamHostCreateRequest is the body of POST /pam/hosts. It is a narrower type
// than the update: ips, aliases and notes cannot be set at creation time, so a
// caller that wants them issues a PUT straight afterwards.
type PamHostCreateRequest struct {
	Hostname          string `json:"hostname"`
	GID               int64  `json:"gid"`
	ForceMfa          bool   `json:"force_mfa"`
	LocalPasswordOnly bool   `json:"local_password_only"`
}

// PamHostUpdateRequest is the body of PUT /pam/hosts/{id}, a full replacement
// of every mutable field including the hostname — unlike the user attribute
// PUT, this one really is a rename and is used as one.
type PamHostUpdateRequest struct {
	Hostname          string   `json:"hostname"`
	GID               int64    `json:"gid"`
	ForceMfa          bool     `json:"force_mfa"`
	LocalPasswordOnly bool     `json:"local_password_only"`
	IPs               []string `json:"ips"`
	Aliases           []string `json:"aliases"`
	Notes             *string  `json:"notes,omitempty"`
}

// PamHostSecretResponse is the shared secret a host authenticates with.
type PamHostSecretResponse struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

const (
	pamGroupsPath = "/pam/groups"
	pamUsersPath  = "/pam/users"
	pamHostsPath  = "/pam/hosts"
)

func pamGroupPath(gid int64) string { return pamGroupsPath + "/" + strconv.FormatInt(gid, 10) }
func pamUserPath(uid int64) string  { return pamUsersPath + "/" + strconv.FormatInt(uid, 10) }

// Host ids are Rauthy-generated and match ^[a-zA-Z0-9]{24}$, so there is
// nothing here that url.PathEscape would change; it is applied anyway because
// the id can also arrive from `terraform import`, where it is whatever the
// operator typed.
func pamHostPath(id string) string { return pamHostsPath + "/" + url.PathEscape(id) }

// CreatePamGroup issues POST /pam/groups. Requires the Pam:Create right.
//
// DIVERGENCE ONE. The OpenAPI document describes this operation as "GET PAM
// groups", gives it no request body, and types the response as an array of
// groups — it documents a listing. A live 0.36.2 does the opposite: POST
// /pam/groups takes a PamGroupCreateRequest and answers with the single group
// it created, and an empty body is a 422. The listing is GET /pam/groups, an
// operation the document does not mention at all. The two got swapped in
// whatever annotation generates the spec.
func (c *Client) CreatePamGroup(ctx context.Context, req PamGroupCreateRequest) (*PamGroupResponse, error) {
	var out PamGroupResponse
	if err := c.do(ctx, http.MethodPost, pamGroupsPath, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPamGroups issues GET /pam/groups. Undocumented but real; see
// CreatePamGroup. Requires Pam:Read.
func (c *Client) ListPamGroups(ctx context.Context) ([]PamGroupResponse, error) {
	var out []PamGroupResponse
	if err := c.do(ctx, http.MethodGet, pamGroupsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPamGroup returns the group with the given gid. Rauthy offers no per-gid
// read, so — as with roles, groups and scopes — the list is filtered here and a
// missing group becomes a synthetic 404.
func (c *Client) GetPamGroup(ctx context.Context, gid int64) (*PamGroupResponse, error) {
	groups, err := c.ListPamGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == gid {
			return &groups[i], nil
		}
	}
	return nil, notFoundError(pamGroupsPath, "no PAM group with gid "+strconv.FormatInt(gid, 10))
}

// DeletePamGroup issues DELETE /pam/groups/{gid}. Requires Pam:Delete.
//
// Rauthy has no update endpoint for a PAM group at all: neither the name nor
// the type can be changed after creation, which is why rauthy_pam_group
// replaces on every change.
func (c *Client) DeletePamGroup(ctx context.Context, gid int64) error {
	return c.do(ctx, http.MethodDelete, pamGroupPath(gid), nil, nil)
}

// CreatePamUser issues POST /pam/users, linking an existing Rauthy identity to
// a POSIX account. Requires Pam:Create.
func (c *Client) CreatePamUser(ctx context.Context, req PamUserCreateRequest) (*PamUserResponse, error) {
	var out PamUserResponse
	if err := c.do(ctx, http.MethodPost, pamUsersPath, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPamUsers issues GET /pam/users. Requires Pam:Read.
func (c *Client) ListPamUsers(ctx context.Context) ([]PamUserResponse, error) {
	var out []PamUserResponse
	if err := c.do(ctx, http.MethodGet, pamUsersPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPamUser issues GET /pam/users/{uid}. Unlike the groups, this one is a real
// per-id read and a missing user is the server's own 404.
func (c *Client) GetPamUser(ctx context.Context, uid int64) (*PamUserDetailsResponse, error) {
	var out PamUserDetailsResponse
	if err := c.do(ctx, http.MethodGet, pamUserPath(uid), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePamUser issues PUT /pam/users/{uid}. Requires Pam:Update. The response
// has no body; the caller reads the user back rather than trusting the write.
func (c *Client) UpdatePamUser(ctx context.Context, uid int64, req PamUserUpdateRequest) error {
	return c.do(ctx, http.MethodPut, pamUserPath(uid), req, nil)
}

// DeletePamUser issues DELETE /pam/users/{uid}. Requires Pam:Delete.
//
// DIVERGENCE TWO. The OpenAPI document lists only GET and PUT for this path, so
// on paper a PAM user can be created and never removed — which would make it
// unmanageable by Terraform. A live 0.36.2 answers DELETE with 200 and the user
// is gone afterwards, together with the personal group Rauthy created for it.
// The document simply omits the operation.
func (c *Client) DeletePamUser(ctx context.Context, uid int64) error {
	return c.do(ctx, http.MethodDelete, pamUserPath(uid), nil, nil)
}

// CreatePamHost issues POST /pam/hosts. Requires Pam:Create.
//
// DIVERGENCE THREE, and the one that decided a resource. The OpenAPI document
// has no POST /pam/hosts — it offers GET, PUT and DELETE only, which reads as
// "hosts register themselves and an admin can then adjust or remove one", and
// would leave rauthy_pam_host as an import-only resource or a data source. A
// live 0.36.2 does accept POST /pam/hosts with a PamHostCreateRequest and
// answers with the new host, id and all. The schema for that body
// (PamHostCreateRequest) is even present in the document, orphaned, with no
// operation referring to it — so this looks like a lost annotation rather than
// a hidden endpoint. rauthy_pam_host is therefore an ordinary CRUD resource.
func (c *Client) CreatePamHost(ctx context.Context, req PamHostCreateRequest) (*PamHostSimpleResponse, error) {
	var out PamHostSimpleResponse
	if err := c.do(ctx, http.MethodPost, pamHostsPath, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPamHosts issues GET /pam/hosts. Requires Pam:Read.
func (c *Client) ListPamHosts(ctx context.Context) ([]PamHostSimpleResponse, error) {
	var out []PamHostSimpleResponse
	if err := c.do(ctx, http.MethodGet, pamHostsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPamHost issues GET /pam/hosts/{id}. Requires Pam:Read.
func (c *Client) GetPamHost(ctx context.Context, id string) (*PamHostDetailsResponse, error) {
	var out PamHostDetailsResponse
	if err := c.do(ctx, http.MethodGet, pamHostPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePamHost issues PUT /pam/hosts/{id}. Requires Pam:Update. No response
// body.
func (c *Client) UpdatePamHost(ctx context.Context, id string, req PamHostUpdateRequest) error {
	return c.do(ctx, http.MethodPut, pamHostPath(id), req, nil)
}

// DeletePamHost issues DELETE /pam/hosts/{id}. Requires Pam:Delete.
func (c *Client) DeletePamHost(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, pamHostPath(id), nil, nil)
}

// GetPamHostSecret issues POST /pam/hosts/{id}/secret, which reads the current
// secret rather than creating one — the verb is POST because the operation is
// privileged, not because it writes. Rotation is PUT on the same path, which
// this provider deliberately does not expose: a rotation would break every
// client already configured with the old secret, and Terraform has no way to
// know when that is wanted.
func (c *Client) GetPamHostSecret(ctx context.Context, id string) (*PamHostSecretResponse, error) {
	var out PamHostSecretResponse
	if err := c.do(ctx, http.MethodPost, pamHostPath(id)+"/secret", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
