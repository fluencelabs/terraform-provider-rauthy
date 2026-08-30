// Package client is a thin HTTP client for the Rauthy admin API (/auth/v1).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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
	// retry bounds how hard every request tries before giving up. See retry.go.
	retry retryPolicy
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying HTTP client. Used by tests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithRetryPolicy replaces the retry budget. Used by tests, which need a
// backoff measured in microseconds rather than in seconds.
//
// This is deliberately not surfaced as a provider schema attribute. The budget
// is a property of how flaky the network between Terraform and Rauthy is, which
// the operator cannot usefully predict at configuration time, and every value
// worth choosing is already covered: the defaults ride out a blip, and a
// failure that outlives them is one a human should see rather than one the
// provider should keep hammering. Exposing `max_retries` would add a knob whose
// only honest documentation is "leave this alone", and a wrong setting turns a
// visible failure into a silent multi-minute stall inside an apply.
func WithRetryPolicy(maxAttempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		c.retry = retryPolicy{maxAttempts: maxAttempts, baseDelay: baseDelay, maxDelay: maxDelay}
	}
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
		retry: retryPolicy{
			maxAttempts: defaultMaxAttempts,
			baseDelay:   defaultBaseDelay,
			maxDelay:    defaultMaxDelay,
		},
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
	raw, err := c.doRaw(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}

	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if decodeErr := json.Unmarshal(raw, out); decodeErr != nil {
		return fmt.Errorf("decode %s %s response: %w", method, path, decodeErr)
	}
	return nil
}

// doText executes a request whose response is a bare string rather than JSON.
//
// Two Rauthy endpoints answer with `text/plain`: POST /api_keys and
// PUT /api_keys/{name}/secret both hand back the new API key as a naked
// `<name>$<secret>` line with no envelope around it. Feeding that to do() would
// fail in json.Unmarshal, so it gets its own path rather than a special case
// inside the JSON one.
func (c *Client) doText(ctx context.Context, method, path string, body any) (string, error) {
	raw, err := c.doRaw(ctx, method, path, body, "text/plain")
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
}

// doRaw sends body as JSON and returns the response body untouched. accept is
// sent as the Accept header; Rauthy ignores it, but it keeps the intent of each
// call visible on the wire. The binary endpoints do not come through here —
// they call send directly, since they neither send nor receive JSON.
func (c *Client) doRaw(ctx context.Context, method, path string, body any, accept string) ([]byte, error) {
	headers := map[string]string{"Accept": accept}

	var encoded []byte
	if body != nil {
		var err error
		if encoded, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("marshal %s %s body: %w", method, path, err)
		}
		headers["Content-Type"] = "application/json"
	}

	resp, err := c.send(ctx, method, path, encoded, headers)
	if err != nil {
		return nil, err
	}
	return resp.body, nil
}

// newRequest builds an authenticated request against path, relative to the API
// root. Content negotiation is left to the caller, because the branding
// endpoints neither send nor receive JSON.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build %s %s: %w", method, path, err)
	}
	// Rauthy expects the key as `API-Key <name>$<secret>`.
	req.Header.Set("Authorization", "API-Key "+c.apiKey)
	return req, nil
}

// download issues a GET and returns the raw body with its content type, for
// the endpoints that serve binary rather than JSON.
func (c *Client) download(ctx context.Context, path string) ([]byte, string, error) {
	resp, err := c.send(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, "", err
	}
	return resp.body, resp.contentType, nil
}

// upload issues a PUT with a pre-encoded body and an explicit content type.
func (c *Client) upload(ctx context.Context, path, contentType string, body []byte) error {
	// The multipart body is already encoded in full, so replaying it on a retry
	// costs nothing beyond a fresh reader over the same bytes.
	_, err := c.send(ctx, http.MethodPut, path, body, map[string]string{
		"Content-Type": contentType,
		"Accept":       "application/json",
	})
	return err
}

// multipartBody encodes one image as a single multipart/form-data part and
// returns the body together with the boundary-bearing content type.
func multipartBody(field string, img Image) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// The part carries both a filename and an explicit Content-Type: Rauthy
	// reads the declared type to decide how to handle the bytes, and the
	// filename is what marks the part as a file upload. The filename's value is
	// never used, so it only has to be present.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, field))
	h.Set("Content-Type", img.ContentType)

	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", fmt.Errorf("create multipart part: %w", err)
	}
	if _, writeErr := part.Write(img.Data); writeErr != nil {
		return nil, "", fmt.Errorf("write multipart part: %w", writeErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", closeErr)
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}
