// Package client is a thin HTTP client for the Rauthy admin API (/auth/v1).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// APIBasePath is appended to the configured Rauthy URL. Callers configure the
// provider with the bare instance URL (https://auth.example.com); the API base
// path is ours to know.
const APIBasePath = "/auth/v1"

const defaultTimeout = 30 * time.Second

// Client talks to the Rauthy admin API using an API key.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	// userAttrMu serialises writes to the user attribute definitions, which
	// Rauthy stores as one cached list and rewrites without locking. See the
	// note above userAttrPathBase in user_attributes.go for what happens
	// without it.
	userAttrMu sync.Mutex
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying HTTP client. Used by tests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// New builds a Client for a Rauthy instance. rawURL is the instance root, for
// example https://auth.example.com; APIBasePath is appended automatically. If
// rawURL already ends in the API base path it is not appended twice, so that a
// pasted Swagger URL does not silently produce /auth/v1/auth/v1.
func New(rawURL, apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("rauthy url must not be empty")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("rauthy api key must not be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rauthy url %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("rauthy url %q must use http or https", rawURL)
	}

	base := strings.TrimRight(u.String(), "/")
	if !strings.HasSuffix(base, APIBasePath) {
		base += APIBasePath
	}

	c := &Client{
		baseURL:    base,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// BaseURL returns the API root, including the /auth/v1 base path.
func (c *Client) BaseURL() string { return c.baseURL }

// do executes a request against path (relative to the API root), sending body
// as JSON when non-nil and decoding the response into out when non-nil.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s %s body: %w", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	// Rauthy expects the key as `API-Key <name>$<secret>`.
	req.Header.Set("Authorization", "API-Key "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s %s response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(method, path, resp.StatusCode, raw)
	}

	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if decodeErr := json.Unmarshal(raw, out); decodeErr != nil {
		return fmt.Errorf("decode %s %s response: %w", method, path, decodeErr)
	}
	return nil
}
