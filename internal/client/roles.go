package client

import (
	"context"
	"net/http"
	"net/url"
)

// RoleResponse is a role as Rauthy returns it from /roles.
type RoleResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RoleRequest is the body of POST /roles and PUT /roles/{id}.
type RoleRequest struct {
	Role string `json:"role"`
}

func rolePath(id string) string {
	return "/roles/" + url.PathEscape(id)
}

// CreateRole issues POST /roles. Requires the Roles:Create right.
func (c *Client) CreateRole(ctx context.Context, req RoleRequest) (*RoleResponse, error) {
	var out RoleResponse
	if err := c.do(ctx, http.MethodPost, "/roles", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRoles issues GET /roles. Requires Roles:Read.
func (c *Client) ListRoles(ctx context.Context) ([]RoleResponse, error) {
	var out []RoleResponse
	if err := c.do(ctx, http.MethodGet, "/roles", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRole returns the role with the given id.
//
// Rauthy has no GET /roles/{id}: the whole list is fetched and filtered here.
// A role that is not in the list yields a synthetic 404, so callers can use
// IsNotFound the same way they do for clients.
func (c *Client) GetRole(ctx context.Context, id string) (*RoleResponse, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i], nil
		}
	}
	return nil, notFoundError("/roles", "role "+id+" does not exist")
}

// GetRoleByName returns the role with the given name, or a synthetic 404.
func (c *Client) GetRoleByName(ctx context.Context, name string) (*RoleResponse, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].Name == name {
			return &roles[i], nil
		}
	}
	return nil, notFoundError("/roles", "no role named "+name)
}

// UpdateRole issues PUT /roles/{id}, renaming the role. Requires Roles:Update.
func (c *Client) UpdateRole(ctx context.Context, id string, req RoleRequest) (*RoleResponse, error) {
	var out RoleResponse
	if err := c.do(ctx, http.MethodPut, rolePath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole issues DELETE /roles/{id}. Requires Roles:Delete.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, rolePath(id), nil, nil)
}
