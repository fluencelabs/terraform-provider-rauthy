package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// UserAttrRequest is the body of POST /users/attr and PUT /users/attr/{name}.
//
// DefaultValue is arbitrary JSON upstream (`serde_json::Value`), so it is kept
// as raw bytes rather than being forced into a Go type: Rauthy stores whatever
// it is handed and gives it back unchanged.
//
// `typ` is deliberately absent. Rauthy's own OpenAPI document describes it as
// "Currently ignored - will be implemented in a future version", and its only
// enum member is `Email`, so sending it would be a no-op field that the
// provider would then have to keep in state forever for compatibility.
type UserAttrRequest struct {
	Name         string          `json:"name"`
	Desc         *string         `json:"desc,omitempty"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	UserEditable *bool           `json:"user_editable,omitempty"`
}

// UserAttrResponse is one configured user attribute as Rauthy returns it.
// Unlike the request, user_editable is always present here.
type UserAttrResponse struct {
	Name         string          `json:"name"`
	Desc         *string         `json:"desc,omitempty"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	UserEditable bool            `json:"user_editable"`
}

// userAttrListResponse wraps the list. GET /users/attr does not answer with a
// bare array the way /scopes, /roles and /groups do — it wraps the attributes
// in an object under `values`.
type userAttrListResponse struct {
	Values []UserAttrResponse `json:"values"`
}

const userAttrPathBase = "/users/attr"

// KNOWN SERVER-SIDE RACE, found against a live Rauthy 0.35.2 and not visible in
// the OpenAPI document. Rauthy keeps the whole set of user attribute
// definitions in one cached list and every write is a read-modify-write of it,
// with no locking. Overlapping writes therefore lose one another: three pairs
// of concurrent POSTs left the list holding one attribute twice and two others
// not at all. The rows do reach the database — a later DELETE rebuilds the
// cache and the missing ones reappear — but until something invalidates it,
// GET /users/attr serves the corrupted list, so an attribute created moments
// earlier is invisible.
//
// That is fatal here rather than merely untidy: a scope's attr_include_*
// mapping is filtered against exactly that list, so a scope applied alongside
// the attributes it maps silently loses them. And Terraform applies independent
// resources in parallel — ten at a time by default — which makes overlapping
// creates the normal case rather than an unlucky one.
//
// Writes are therefore serialised through Client.userAttrMu. That covers this
// process, which is all that can be covered: two `terraform apply` runs against
// one instance can still race each other, and so can the Admin UI. Reads are
// deliberately left unserialised — they do not mutate the list, and blocking
// them would not make a concurrently corrupted list any more accurate.
//
// If a Rauthy release makes these writes atomic, the mutex can go.

func userAttrPath(name string) string {
	return userAttrPathBase + "/" + url.PathEscape(name)
}

// CreateUserAttr issues POST /users/attr. Requires the UserAttributes:Create
// right.
func (c *Client) CreateUserAttr(ctx context.Context, req UserAttrRequest) (*UserAttrResponse, error) {
	c.userAttrMu.Lock()
	defer c.userAttrMu.Unlock()

	var out UserAttrResponse
	if err := c.do(ctx, http.MethodPost, userAttrPathBase, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListUserAttrs issues GET /users/attr. Requires UserAttributes:Read.
func (c *Client) ListUserAttrs(ctx context.Context) ([]UserAttrResponse, error) {
	var out userAttrListResponse
	if err := c.do(ctx, http.MethodGet, userAttrPathBase, nil, &out); err != nil {
		return nil, err
	}
	return out.Values, nil
}

// GetUserAttr returns the attribute with the given name. As with roles, groups
// and scopes, Rauthy offers no per-name read, so the list is filtered here and
// a missing attribute yields a synthetic 404.
func (c *Client) GetUserAttr(ctx context.Context, name string) (*UserAttrResponse, error) {
	attrs, err := c.ListUserAttrs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range attrs {
		if attrs[i].Name == name {
			return &attrs[i], nil
		}
	}
	return nil, notFoundError(userAttrPathBase, "no user attribute named "+name)
}

// UpdateUserAttr issues PUT /users/attr/{name}. Requires UserAttributes:Update.
//
// The attribute is addressed by its current name and the body carries the name
// it should have afterwards, which reads like a rename — but is not one, and
// the provider never uses it as one. Against a live Rauthy 0.35.2 a PUT that
// changes the name leaves the old name occupied: it disappears from
// GET /users/attr, yet a later POST under it fails with "User attribute config
// does already exist", so the name is unusable until someone deletes it by
// hand. Same root cause as the race above — the cached list and the stored rows
// disagree. rauthy_user_attribute therefore replaces the resource on a rename
// instead, and this method only ever carries the name it was given.
//
// The spec declares no response body for this operation and a live instance
// does send one; it is discarded either way, and the caller reads the attribute
// back rather than trusting it.
func (c *Client) UpdateUserAttr(ctx context.Context, name string, req UserAttrRequest) error {
	c.userAttrMu.Lock()
	defer c.userAttrMu.Unlock()

	return c.do(ctx, http.MethodPut, userAttrPath(name), req, nil)
}

// DeleteUserAttr issues DELETE /users/attr/{name}. Requires
// UserAttributes:Delete.
func (c *Client) DeleteUserAttr(ctx context.Context, name string) error {
	c.userAttrMu.Lock()
	defer c.userAttrMu.Unlock()

	return c.do(ctx, http.MethodDelete, userAttrPath(name), nil, nil)
}
