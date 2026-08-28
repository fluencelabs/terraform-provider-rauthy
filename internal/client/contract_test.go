package client_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	validator "github.com/pb33f/libopenapi-validator"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/client/mock"
)

// The contract tests validate the bodies this package sends, and the bodies it
// expects back, against the OpenAPI spec Rauthy itself publishes. They catch
// request/response drift after a Rauthy upgrade without a live instance.
//
// The spec is vendored under mock/testdata by `make openapi-refresh` (Docker,
// run by hand at version bumps). Without it these tests skip rather than fail,
// so a fresh checkout is not blocked on Docker.
func newContractValidator(t *testing.T) validator.Validator {
	t.Helper()

	v, warnings, err := mock.NewValidator()
	if errors.Is(err, mock.ErrNoSpec) {
		t.Skip("no vendored Rauthy OpenAPI spec; run `make openapi-refresh` to generate it")
	}
	if err != nil {
		t.Fatalf("build validator: %v", err)
	}
	for _, w := range warnings {
		t.Logf("spec build warning (tolerated): %v", w)
	}
	return v
}

// apiPath prefixes a spec path with the API base path the provider talks to.
func apiPath(p string) string { return client.APIBasePath + p }

func validateRequest(t *testing.T, v validator.Validator, method, path string, body any) (bool, string) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, "http://rauthy.test"+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	ok, valErrs := v.ValidateHttpRequest(req)
	return ok, joinErrs(valErrs)
}

func validateResponse(t *testing.T, v validator.Validator, method, path string, status int, body string) (bool, string) {
	t.Helper()

	req := httptest.NewRequest(method, "http://rauthy.test"+path, nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(status)
	_, _ = rec.WriteString(body)

	ok, valErrs := v.ValidateHttpResponse(req, rec.Result())
	return ok, joinErrs(valErrs)
}

func joinErrs[E error](errs []E) string {
	var sb strings.Builder
	for _, e := range errs {
		sb.WriteString(e.Error())
		sb.WriteString("; ")
	}
	return sb.String()
}

func TestContract_CreateClientRequest(t *testing.T) {
	v := newContractValidator(t)

	name := "Example App"
	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/clients"), client.NewClientRequest{
		ID:                     "example-app",
		Name:                   &name,
		Confidential:           true,
		RedirectURIs:           []string{"https://app.example.com/callback"},
		PostLogoutRedirectURIs: []string{"https://app.example.com/"},
	})
	if !ok {
		t.Errorf("POST /clients body rejected by the spec: %s", msg)
	}
}

func TestContract_UpdateClientRequest(t *testing.T) {
	v := newContractValidator(t)

	name := "Example App"
	uri := "https://app.example.com"
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/clients/example-app"), client.UpdateClientRequest{
		ID:                     "example-app",
		Name:                   &name,
		Confidential:           true,
		RedirectURIs:           []string{"https://app.example.com/callback"},
		PostLogoutRedirectURIs: []string{"https://app.example.com/"},
		AllowedOrigins:         []string{"https://app.example.com"},
		Enabled:                true,
		FlowsEnabled:           []string{"authorization_code", "refresh_token"},
		AccessTokenAlg:         "EdDSA",
		IDTokenAlg:             "EdDSA",
		AuthCodeLifetime:       60,
		AccessTokenLifetime:    600,
		Scopes:                 []string{"openid", "profile", "email"},
		DefaultScopes:          []string{"openid"},
		Challenges:             []string{"S256"},
		ForceMFA:               false,
		ClientURI:              &uri,
		Contacts:               []string{"ops@example.com"},
	})
	if !ok {
		t.Errorf("PUT /clients/{id} body rejected by the spec: %s", msg)
	}
}

// A PUT is a full replacement and Rauthy's deserializer requires these fields.
// If a future spec makes them optional this test tells us the provider may stop
// sending them.
func TestContract_UpdateClientRequiresEnabledAndFlows(t *testing.T) {
	v := newContractValidator(t)

	partial := map[string]any{
		"id":            "example-app",
		"confidential":  true,
		"redirect_uris": []string{"https://app.example.com/callback"},
	}
	ok, _ := validateRequest(t, v, http.MethodPut, apiPath("/clients/example-app"), partial)
	if ok {
		t.Error("PUT /clients/{id} accepted a body without enabled/flows_enabled/scopes; " +
			"the spec no longer requires them, revisit UpdateClientRequest")
	}
}

func TestContract_ClientResponse(t *testing.T) {
	v := newContractValidator(t)

	body := `{
		"id": "example-app",
		"name": "Example App",
		"enabled": true,
		"confidential": true,
		"redirect_uris": ["https://app.example.com/callback"],
		"flows_enabled": ["authorization_code"],
		"access_token_alg": "EdDSA",
		"id_token_alg": "EdDSA",
		"auth_code_lifetime": 60,
		"access_token_lifetime": 600,
		"scopes": ["openid"],
		"default_scopes": ["openid"],
		"force_mfa": false
	}`

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/clients/example-app"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /clients/{id} response rejected by the spec: %s", msg)
	}

	// The same body must round-trip into our type without losing fields.
	var decoded client.ClientResponse
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode into ClientResponse: %v", err)
	}
	if decoded.AccessTokenAlg != "EdDSA" || decoded.AccessTokenLifetime != 600 {
		t.Errorf("ClientResponse lost fields on decode: %+v", decoded)
	}
}

// The secret read is registered on POST, not GET. If a Rauthy upgrade moves it,
// the spec will no longer have a POST operation on this path and this fails.
func TestContract_SecretReadIsPOST(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/clients/example-app/secret"), nil)
	if !ok {
		t.Errorf("POST /clients/{id}/secret rejected by the spec: %s", msg)
	}
}

func TestContract_RotateSecretRequest(t *testing.T) {
	v := newContractValidator(t)

	hours := int64(6)
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/clients/example-app/secret"),
		client.ClientSecretRequest{CacheCurrentHours: &hours})
	if !ok {
		t.Errorf("PUT /clients/{id}/secret body rejected by the spec: %s", msg)
	}
}

// Rauthy derives its OpenAPI document from utoipa, which does not emit the
// `validator` crate's ranges and regexes into the schema: `cache_current_hours`
// is documented as "Value between 1 and 24" in prose but has only
// `minimum: 0` machine-readable, and the lifetimes and algorithm patterns are
// prose too. So the contract layer can police field sets and types but NOT
// value ranges — those are guarded provider-side, and this test pins the
// assumption so we notice if a future Rauthy starts emitting them.
func TestContract_SpecDoesNotCarryValueRanges(t *testing.T) {
	v := newContractValidator(t)

	tooMany := int64(48)
	ok, _ := validateRequest(t, v, http.MethodPut, apiPath("/clients/example-app/secret"),
		client.ClientSecretRequest{CacheCurrentHours: &tooMany})
	if !ok {
		t.Log("the spec now rejects cache_current_hours=48; ranges became machine-readable, " +
			"the contract tests can start asserting them")
	}
}
