package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AttrList is a scope's attribute mapping as it comes back from Rauthy, which
// is not one shape but two: POST /scopes answers with the stored form, a single
// comma-joined string, while GET /scopes and PUT /scopes/{id} answer with an
// array. The vendored OpenAPI document describes only the string form, so this
// divergence is invisible to the contract tests and was found by running
// against a live instance.
//
// Both are decoded into a slice, and an element that is empty after trimming is
// dropped: the joined form of "no attributes" is "", which naively splits to a
// one-element slice holding an empty name.
type AttrList []string

// UnmarshalJSON accepts null, a comma-joined string, or an array of strings.
func (a *AttrList) UnmarshalJSON(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*a = nil
		return nil
	}

	if trimmed[0] == '"' {
		var joined string
		if err := json.Unmarshal(trimmed, &joined); err != nil {
			return fmt.Errorf("decode attribute mapping %s: %w", trimmed, err)
		}
		*a = cleanAttrs(strings.Split(joined, ","))
		return nil
	}

	var list []string
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return fmt.Errorf("decode attribute mapping %s: %w", trimmed, err)
	}
	*a = cleanAttrs(list)
	return nil
}

func cleanAttrs(in []string) AttrList {
	out := make(AttrList, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ScopeResponse is a scope as Rauthy returns it.
type ScopeResponse struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	AttrIncludeAccess AttrList `json:"attr_include_access"`
	AttrIncludeID     AttrList `json:"attr_include_id"`
}

// ScopeRequest is the body of POST /scopes and PUT /scopes/{id}.
type ScopeRequest struct {
	Scope             string   `json:"scope"`
	AttrIncludeAccess []string `json:"attr_include_access,omitempty"`
	AttrIncludeID     []string `json:"attr_include_id,omitempty"`
}

func scopePath(id string) string {
	return "/scopes/" + url.PathEscape(id)
}

// CreateScope issues POST /scopes. Requires the Scopes:Create right.
func (c *Client) CreateScope(ctx context.Context, req ScopeRequest) (*ScopeResponse, error) {
	var out ScopeResponse
	if err := c.do(ctx, http.MethodPost, "/scopes", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListScopes issues GET /scopes. Requires Scopes:Read.
func (c *Client) ListScopes(ctx context.Context) ([]ScopeResponse, error) {
	var out []ScopeResponse
	if err := c.do(ctx, http.MethodGet, "/scopes", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetScope returns the scope with the given id. As with roles and groups,
// Rauthy has no per-id endpoint, so the list is filtered here and a missing
// scope yields a synthetic 404.
func (c *Client) GetScope(ctx context.Context, id string) (*ScopeResponse, error) {
	scopes, err := c.ListScopes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range scopes {
		if scopes[i].ID == id {
			return &scopes[i], nil
		}
	}
	return nil, notFoundError("/scopes", "scope "+id+" does not exist")
}

// GetScopeByName returns the scope with the given name, or a synthetic 404.
func (c *Client) GetScopeByName(ctx context.Context, name string) (*ScopeResponse, error) {
	scopes, err := c.ListScopes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range scopes {
		if scopes[i].Name == name {
			return &scopes[i], nil
		}
	}
	return nil, notFoundError("/scopes", "no scope named "+name)
}

// UpdateScope issues PUT /scopes/{id}. Requires Scopes:Update.
func (c *Client) UpdateScope(ctx context.Context, id string, req ScopeRequest) (*ScopeResponse, error) {
	var out ScopeResponse
	if err := c.do(ctx, http.MethodPut, scopePath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteScope issues DELETE /scopes/{id}. Requires Scopes:Delete.
func (c *Client) DeleteScope(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, scopePath(id), nil, nil)
}
