package client

import (
	"context"
	"net/http"
	"net/url"
)

// GroupResponse is a group as Rauthy returns it from /groups.
type GroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GroupRequest is the body of POST /groups and PUT /groups/{id}.
type GroupRequest struct {
	Group string `json:"group"`
}

func groupPath(id string) string {
	return "/groups/" + url.PathEscape(id)
}

// CreateGroup issues POST /groups. Requires the Groups:Create right.
func (c *Client) CreateGroup(ctx context.Context, req GroupRequest) (*GroupResponse, error) {
	var out GroupResponse
	if err := c.do(ctx, http.MethodPost, "/groups", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListGroups issues GET /groups. Requires Groups:Read.
func (c *Client) ListGroups(ctx context.Context) ([]GroupResponse, error) {
	var out []GroupResponse
	if err := c.do(ctx, http.MethodGet, "/groups", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetGroup returns the group with the given id. As with roles, Rauthy has no
// per-id endpoint, so the list is filtered here and a missing group yields a
// synthetic 404.
func (c *Client) GetGroup(ctx context.Context, id string) (*GroupResponse, error) {
	groups, err := c.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == id {
			return &groups[i], nil
		}
	}
	return nil, notFoundError(groupPath(id), "group "+id+" does not exist")
}

// GetGroupByName returns the group with the given name, or a synthetic 404.
func (c *Client) GetGroupByName(ctx context.Context, name string) (*GroupResponse, error) {
	groups, err := c.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Name == name {
			return &groups[i], nil
		}
	}
	return nil, notFoundError("/groups", "no group named "+name)
}

// UpdateGroup issues PUT /groups/{id}, renaming the group. Requires
// Groups:Update.
func (c *Client) UpdateGroup(ctx context.Context, id string, req GroupRequest) (*GroupResponse, error) {
	var out GroupResponse
	if err := c.do(ctx, http.MethodPut, groupPath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteGroup issues DELETE /groups/{id}. Requires Groups:Delete.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, groupPath(id), nil, nil)
}
