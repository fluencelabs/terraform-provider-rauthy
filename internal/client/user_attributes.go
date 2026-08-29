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

func userAttrPath(name string) string {
	return userAttrPathBase + "/" + url.PathEscape(name)
}

// CreateUserAttr issues POST /users/attr. Requires the UserAttributes:Create
// right.
func (c *Client) CreateUserAttr(ctx context.Context, req UserAttrRequest) (*UserAttrResponse, error) {
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
// The attribute is addressed by its current name and the body carries the one
// it should have afterwards, so this is also the rename. It returns no body —
// a bare 200 — which is why the caller has to read the attribute back rather
// than folding a response into state.
func (c *Client) UpdateUserAttr(ctx context.Context, name string, req UserAttrRequest) error {
	return c.do(ctx, http.MethodPut, userAttrPath(name), req, nil)
}

// DeleteUserAttr issues DELETE /users/attr/{name}. Requires
// UserAttributes:Delete.
func (c *Client) DeleteUserAttr(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, userAttrPath(name), nil, nil)
}
