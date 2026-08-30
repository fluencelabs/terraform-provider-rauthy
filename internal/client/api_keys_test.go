package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

const twoAPIKeysJSON = `{"keys":[` +
	`{"name":"tfacc","created":1788076915,"expires":null,` +
	`"access":[{"group":"Clients","access_rights":["read","create","update","delete"]}]},` +
	`{"name":"deploy","created":1788076923,"expires":1900000000,` +
	`"access":[{"group":"Users","access_rights":["read"]},{"group":"ApiKeys","access_rights":[]}]}` +
	`]}`

// The two secret-bearing endpoints answer with a bare line of text, not JSON.
// Sending it through the JSON path would fail in json.Unmarshal, so the plain
// path is the thing worth pinning.
func TestCreateAPIKey_ReadsAPlainTextSecret(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotAccept string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAccept = r.Method, r.URL.Path, r.Header.Get("Accept")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// The trailing newline is what a server may or may not append; either
		// way it must not end up inside the credential.
		_, _ = w.Write([]byte("deploy$s3cr3t\n"))
	})

	exp := int64(1900000000)
	got, err := c.CreateAPIKey(context.Background(), client.APIKeyRequest{
		Name:   "deploy",
		Exp:    &exp,
		Access: []client.APIKeyAccess{{Group: "Users", AccessRights: []string{"read"}}},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/api_keys" {
		t.Errorf("got %s %s, want POST /auth/v1/api_keys", gotMethod, gotPath)
	}
	if gotAccept != "text/plain" {
		t.Errorf("Accept = %q, want text/plain", gotAccept)
	}
	if got != "deploy$s3cr3t" {
		t.Errorf("secret = %q, want the trimmed <name>$<secret> form", got)
	}
	if gotBody["name"] != "deploy" || gotBody["exp"] != float64(exp) {
		t.Errorf("body = %v", gotBody)
	}
}

func TestRotateAPIKeySecret_ReadsAPlainTextSecret(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("deploy$rotated"))
	})

	got, err := c.RotateAPIKeySecret(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RotateAPIKeySecret: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/auth/v1/api_keys/deploy/secret" {
		t.Errorf("got %s %s, want PUT /auth/v1/api_keys/deploy/secret", gotMethod, gotPath)
	}
	if got != "deploy$rotated" {
		t.Errorf("secret = %q", got)
	}
}

// A failing plain-text call must still surface Rauthy's JSON error envelope:
// the error path is shared with the JSON one and the response is JSON even when
// the success body is not.
func TestCreateAPIKey_ReportsTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"timestamp":1,"error":"BadRequest","message":"UNIQUE constraint failed"}`))
	})

	_, err := c.CreateAPIKey(context.Background(), client.APIKeyRequest{Name: "deploy"})
	if err == nil {
		t.Fatal("want an error")
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want an *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "UNIQUE constraint failed" {
		t.Errorf("got %+v", apiErr)
	}
}

// The listing is wrapped in an object under `keys` rather than being a bare
// array, as /scopes, /roles and /groups are.
func TestListAPIKeys_UnwrapsKeys(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoAPIKeysJSON))
	})

	got, err := c.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(got) != 2 || got[1].Name != "deploy" {
		t.Fatalf("got %+v", got)
	}
	if got[1].Expires == nil || *got[1].Expires != 1900000000 {
		t.Errorf("expires = %v", got[1].Expires)
	}
	if len(got[1].Access) != 2 || len(got[1].Access[1].AccessRights) != 0 {
		t.Errorf("access = %+v", got[1].Access)
	}
}

// There is no GET /api_keys/{name}; a missing key is a conclusion this client
// draws from the listing.
func TestGetAPIKey_SyntheticNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/api_keys" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoAPIKeysJSON))
	})

	if _, err := c.GetAPIKey(context.Background(), "deploy"); err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	_, err := c.GetAPIKey(context.Background(), "absent")
	if !client.IsNotFound(err) {
		t.Errorf("want a 404, got %v", err)
	}
}

// Rauthy compares the name in the body against the one in the path, so the
// client must send the path's name and never a new one.
func TestUpdateAPIKey_SendsTheNameInTheBody(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
	})

	err := c.UpdateAPIKey(context.Background(), "deploy", client.APIKeyRequest{
		Name:   "deploy",
		Access: []client.APIKeyAccess{},
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey: %v", err)
	}
	if gotBody["name"] != "deploy" {
		t.Errorf("name = %v, want deploy", gotBody["name"])
	}
	// An omitted `access` is a deserialize error upstream, so the empty slice
	// has to survive marshalling as `[]` rather than disappearing.
	if _, ok := gotBody["access"].([]any); !ok {
		t.Errorf("access = %v, want an array", gotBody["access"])
	}
}
