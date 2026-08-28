package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// newTestClient wires a Client to a test server and records the last request it
// received.
func newTestClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := client.New(srv.URL, "tf$secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNew_AppendsAPIBasePath(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://auth.example.com":          "https://auth.example.com/auth/v1",
		"https://auth.example.com/":         "https://auth.example.com/auth/v1",
		"https://auth.example.com/auth/v1":  "https://auth.example.com/auth/v1",
		"https://auth.example.com/auth/v1/": "https://auth.example.com/auth/v1",
	}
	for in, want := range cases {
		c, err := client.New(in, "tf$secret")
		if err != nil {
			t.Fatalf("New(%q): %v", in, err)
		}
		if got := c.BaseURL(); got != want {
			t.Errorf("New(%q).BaseURL() = %q, want %q", in, got, want)
		}
	}
}

func TestNew_RejectsBadConfig(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ url, key string }{
		"empty url":     {"", "tf$secret"},
		"empty key":     {"https://auth.example.com", ""},
		"no scheme":     {"auth.example.com", "tf$secret"},
		"wrong scheme":  {"ftp://auth.example.com", "tf$secret"},
		"blank api key": {"https://auth.example.com", "   "},
	}
	for name, tc := range cases {
		if _, err := client.New(tc.url, tc.key); err == nil {
			t.Errorf("%s: New(%q, %q) succeeded, want error", name, tc.url, tc.key)
		}
	}
}

func TestCreateClient_SendsAPIKeyHeaderAndMinimalBody(t *testing.T) {
	t.Parallel()

	var gotAuth, gotPath, gotMethod string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","enabled":true,"confidential":true,` +
			`"redirect_uris":["https://app.example.com/cb"],"flows_enabled":["authorization_code"],` +
			`"access_token_alg":"EdDSA","id_token_alg":"EdDSA","auth_code_lifetime":60,` +
			`"access_token_lifetime":600,"scopes":["openid"],"default_scopes":["openid"],"force_mfa":false}`))
	})

	name := "App"
	_, err := c.CreateClient(context.Background(), client.NewClientRequest{
		ID:           "app",
		Name:         &name,
		Confidential: true,
		RedirectURIs: []string{"https://app.example.com/cb"},
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if gotAuth != "API-Key tf$secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "API-Key tf$secret")
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/clients" {
		t.Errorf("got %s %s, want POST /auth/v1/clients", gotMethod, gotPath)
	}
	// Anything beyond these five fields is silently dropped by Rauthy, so the
	// request type must not carry them at all.
	for _, unwanted := range []string{"flows_enabled", "scopes", "access_token_alg", "enabled", "secret"} {
		if _, ok := gotBody[unwanted]; ok {
			t.Errorf("POST /clients body carries %q, which Rauthy ignores", unwanted)
		}
	}
}

func TestGetClientSecret_UsesPOST(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"s3cr3t"}`))
	})

	got, err := c.GetClientSecret(context.Background(), "app")
	if err != nil {
		t.Fatalf("GetClientSecret: %v", err)
	}
	// Rauthy registers this read on POST, not GET.
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/clients/app/secret" {
		t.Errorf("got %s %s, want POST /auth/v1/clients/app/secret", gotMethod, gotPath)
	}
	if got.Secret == nil || *got.Secret != "s3cr3t" {
		t.Errorf("Secret = %v, want s3cr3t", got.Secret)
	}
}

func TestGetClientSecret_PublicClientHasNoSecret(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"spa","confidential":false}`))
	})

	got, err := c.GetClientSecret(context.Background(), "spa")
	if err != nil {
		t.Fatalf("GetClientSecret: %v", err)
	}
	if got.Secret != nil {
		t.Errorf("Secret = %v, want nil for a public client", *got.Secret)
	}
}

func TestGetClientSecret_403NamesTheMissingRight(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"Forbidden","message":"Access denied"}`))
	})

	_, err := c.GetClientSecret(context.Background(), "app")
	if err == nil {
		t.Fatal("GetClientSecret succeeded on 403, want error")
	}
	if !strings.Contains(err.Error(), "Secrets:Read") {
		t.Errorf("error %q does not name the missing Secrets:Read right", err)
	}
	if !client.IsForbidden(err) {
		t.Error("IsForbidden = false on a wrapped 403")
	}
}

func TestRotateClientSecret_SendsCacheCurrentHours(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"new"}`))
	})

	hours := int64(6)
	if _, err := c.RotateClientSecret(context.Background(), "app", &hours); err != nil {
		t.Fatalf("RotateClientSecret: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotBody["cache_current_hours"] != float64(6) {
		t.Errorf("cache_current_hours = %v, want 6", gotBody["cache_current_hours"])
	}
}

func TestRotateClientSecret_OmitsCacheCurrentHoursWhenUnset(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app","confidential":true,"secret":"new"}`))
	})

	if _, err := c.RotateClientSecret(context.Background(), "app", nil); err != nil {
		t.Fatalf("RotateClientSecret: %v", err)
	}
	if _, ok := gotBody["cache_current_hours"]; ok {
		t.Error("cache_current_hours present in body, want it omitted so Rauthy applies its own default")
	}
}

func TestNotFoundIsDetectable(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NotFound","message":"Client 'gone' not found"}`))
	})

	_, err := c.GetClient(context.Background(), "gone")
	if !client.IsNotFound(err) {
		t.Fatalf("IsNotFound = false for err %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not carry Rauthy's message", err)
	}
}

func TestIDIsPathEscaped(t *testing.T) {
	t.Parallel()

	// Rauthy client IDs may contain characters that are legal in the id regex
	// but not in a raw URL path, such as `?`, `#` and `%`.
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteClient(context.Background(), "a#b?c%d"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if want := "/auth/v1/clients/a%23b%3Fc%25d"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
