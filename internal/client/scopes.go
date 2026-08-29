package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// ScopeResponse is a scope as Rauthy returns it.
//
// Note the asymmetry with ScopeRequest: the request carries the attribute
// mappings as arrays, but every endpoint answers with Rauthy's `Scope`, where
// the same fields are a single comma-joined string (or null when unset). The
// contract tests pin both directions.
type ScopeResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	AttrIncludeAccess *string `json:"attr_include_access"`
	AttrIncludeID     *string `json:"attr_include_id"`
}

// AttrIncludeAccessList splits the comma-joined access-token attribute
// mapping. A null field yields nil, which the provider renders as an unset
// attribute rather than an empty set.
func (s *ScopeResponse) AttrIncludeAccessList() []string { return splitAttrs(s.AttrIncludeAccess) }

// AttrIncludeIDList splits the comma-joined id-token attribute mapping.
func (s *ScopeResponse) AttrIncludeIDList() []string { return splitAttrs(s.AttrIncludeID) }

func splitAttrs(raw *string) []string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
