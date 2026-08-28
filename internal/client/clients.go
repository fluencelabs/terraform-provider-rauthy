package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func clientPath(id string) string {
	return "/clients/" + url.PathEscape(id)
}

// CreateClient issues POST /clients.
//
// Rauthy accepts only the handful of fields in NewClientRequest here and
// silently ignores anything else, so a caller that wants a fully configured
// client must follow this with UpdateClient.
//
// Requires the Clients:Create right on the API key.
func (c *Client) CreateClient(ctx context.Context, req NewClientRequest) (*ClientResponse, error) {
	var out ClientResponse
	if err := c.do(ctx, http.MethodPost, "/clients", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetClient issues GET /clients/{id}. Requires Clients:Read.
func (c *Client) GetClient(ctx context.Context, id string) (*ClientResponse, error) {
	var out ClientResponse
	if err := c.do(ctx, http.MethodGet, clientPath(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateClient issues PUT /clients/{id}, replacing the client's configuration
// wholesale. Requires Clients:Update.
func (c *Client) UpdateClient(ctx context.Context, id string, req UpdateClientRequest) (*ClientResponse, error) {
	var out ClientResponse
	if err := c.do(ctx, http.MethodPut, clientPath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteClient issues DELETE /clients/{id}. Requires Clients:Delete.
func (c *Client) DeleteClient(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, clientPath(id), nil, nil)
}

// GetClientSecret reads the current secret.
//
// Note the verb: Rauthy serves this read on POST /clients/{id}/secret, not GET
// (the handler is named get_client_secret but is registered with #[post]).
// Requires Secrets:Read. The returned Secret is nil for a public client.
func (c *Client) GetClientSecret(ctx context.Context, id string) (*ClientSecretResponse, error) {
	var out ClientSecretResponse
	if err := c.do(ctx, http.MethodPost, clientPath(id)+"/secret", nil, &out); err != nil {
		return nil, annotateMissingRight(err, "Secrets", "Read")
	}
	return &out, nil
}

// RotateClientSecret issues PUT /clients/{id}/secret, generating a fresh
// secret. cacheCurrentHours (1..24), when non-nil, keeps the previous secret
// valid for that long so consumers can catch up. Requires Secrets:Update.
func (c *Client) RotateClientSecret(ctx context.Context, id string, cacheCurrentHours *int64) (*ClientSecretResponse, error) {
	var out ClientSecretResponse
	req := ClientSecretRequest{CacheCurrentHours: cacheCurrentHours}
	if err := c.do(ctx, http.MethodPut, clientPath(id)+"/secret", req, &out); err != nil {
		return nil, annotateMissingRight(err, "Secrets", "Update")
	}
	return &out, nil
}

// annotateMissingRight turns a bare 403 into a message that names the access
// right the API key is missing. An under-provisioned key is the first thing
// people trip over, and Rauthy's own 403 body does not say which right it
// wanted.
func annotateMissingRight(err error, group, right string) error {
	if !IsForbidden(err) {
		return err
	}
	return fmt.Errorf(
		"the Rauthy API key is missing the %s:%s right (grant it in the Admin UI under API Keys): %w",
		group, right, err,
	)
}
