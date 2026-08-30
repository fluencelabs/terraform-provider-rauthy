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

// The method and status parameters are spelled out at every call site even
// though every response contract checked so far is a 200 on GET; they keep the
// helper honest if a non-GET response is ever validated.
//
//nolint:unparam // deliberately general; see above
func validateResponse(
	t *testing.T,
	v validator.Validator,
	method, path string,
	status int,
	body string,
) (bool, string) {
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
		"force_mfa": false,
		"claims_at_root": false
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

func TestContract_RoleRequest(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/roles"), client.RoleRequest{Role: "admin"})
	if !ok {
		t.Errorf("POST /roles body rejected by the spec: %s", msg)
	}

	ok, msg = validateRequest(t, v, http.MethodPut, apiPath("/roles/role-1"), client.RoleRequest{Role: "operator"})
	if !ok {
		t.Errorf("PUT /roles/{id} body rejected by the spec: %s", msg)
	}
}

func TestContract_RoleResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/roles"), http.StatusOK,
		`[{"id":"role-1","name":"admin"}]`)
	if !ok {
		t.Errorf("GET /roles response rejected by the spec: %s", msg)
	}
}

func TestContract_GroupRequest(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/groups"), client.GroupRequest{Group: "developers"})
	if !ok {
		t.Errorf("POST /groups body rejected by the spec: %s", msg)
	}

	ok, msg = validateRequest(t, v, http.MethodPut, apiPath("/groups/group-1"), client.GroupRequest{Group: "ops"})
	if !ok {
		t.Errorf("PUT /groups/{id} body rejected by the spec: %s", msg)
	}
}

func TestContract_GroupResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/groups"), http.StatusOK,
		`[{"id":"group-1","name":"developers"}]`)
	if !ok {
		t.Errorf("GET /groups response rejected by the spec: %s", msg)
	}
}

func TestContract_PasswordPolicyRequest(t *testing.T) {
	v := newContractValidator(t)

	digits := int32(1)
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/password_policy"), client.PasswordPolicy{
		LengthMin:     12,
		LengthMax:     128,
		IncludeDigits: &digits,
	})
	if !ok {
		t.Errorf("PUT /password_policy body rejected by the spec: %s", msg)
	}
}

func TestContract_PasswordPolicyResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/password_policy"), http.StatusOK,
		`{"length_min":12,"length_max":128,"include_digits":1,"include_lower_case":null,`+
			`"include_upper_case":null,"include_special":null,"valid_days":null,"not_recently_used":null}`)
	if !ok {
		t.Errorf("GET /password_policy response rejected by the spec: %s", msg)
	}
}

func TestContract_ScopeRequest(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/scopes"), client.ScopeRequest{
		Scope:             "read:billing",
		AttrIncludeAccess: []string{"department", "cost_center"},
		AttrIncludeID:     []string{"department"},
	})
	if !ok {
		t.Errorf("POST /scopes body rejected by the spec: %s", msg)
	}

	ok, msg = validateRequest(t, v, http.MethodPut, apiPath("/scopes/scope-1"), client.ScopeRequest{
		Scope: "read:billing",
	})
	if !ok {
		t.Errorf("PUT /scopes/{id} body rejected by the spec: %s", msg)
	}
}

// KNOWN SPEC INACCURACY. The vendored document says a scope's attr_include_*
// come back as a string, and that is true only of POST /scopes. Against a live
// Rauthy 0.36.2, GET /scopes and PUT /scopes/{id} answer with an array — the
// handlers return different types and the OpenAPI document records only one.
//
// This test pins the divergence rather than the belief: the string form is what
// the spec accepts, the array form is what the spec rejects and the server
// nonetheless sends. AttrList decodes both. When a Rauthy release makes the
// document agree with the API, the second half fails and this compensation can
// be reconsidered.
func TestContract_ScopeResponseShapeDivergesFromTheSpec(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/scopes"), http.StatusOK,
		`[{"id":"scope-1","name":"read:billing","claims_at_root":false,`+
			`"attr_include_access":"department,cost_center","attr_include_id":null}]`)
	if !ok {
		t.Errorf("GET /scopes response rejected by the spec: %s", msg)
	}

	// The array form — what a live instance actually returns from this very
	// endpoint — is rejected by the document.
	ok, _ = validateResponse(t, v, http.MethodGet, apiPath("/scopes"), http.StatusOK,
		`[{"id":"scope-1","name":"read:billing","claims_at_root":false,"attr_include_access":["department"]}]`)
	if ok {
		t.Error("the spec now accepts the array form GET /scopes really returns; the document and the " +
			"API may have been reconciled — re-check whether AttrList still needs to decode both")
	}
}

func TestContract_AuthProviderRequest(t *testing.T) {
	v := newContractValidator(t)

	secret := "upstream-secret"
	jwks := "https://idp.example.com/jwks"
	body := client.AuthProviderRequest{
		Name:                  "Example IdP",
		Typ:                   "oidc",
		Enabled:               true,
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
		UserinfoEndpoint:      "https://idp.example.com/userinfo",
		JwksEndpoint:          &jwks,
		ClientID:              "rauthy",
		ClientSecret:          &secret,
		Scope:                 "email openid profile",
		UsePKCE:               true,
		ClientSecretBasic:     true,
	}

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/providers/create"), body)
	if !ok {
		t.Errorf("POST /providers/create body rejected by the spec: %s", msg)
	}

	// The same type serves both verbs; a body good enough to create with must
	// be good enough to update with.
	ok, msg = validateRequest(t, v, http.MethodPut, apiPath("/providers/provider-1"), body)
	if !ok {
		t.Errorf("PUT /providers/{id} body rejected by the spec: %s", msg)
	}
}

// Creating an upstream provider is POST /providers/create; POST /providers is
// the listing. The spec is the authority for that, so pin it: if POST /providers
// ever stops accepting an empty body, the two have swapped roles.
func TestContract_AuthProviderCreateIsNotPostProviders(t *testing.T) {
	v := newContractValidator(t)

	ok, _ := validateRequest(t, v, http.MethodPost, apiPath("/providers"), nil)
	if !ok {
		t.Error("POST /providers no longer accepts an empty body; it may no longer be the list endpoint")
	}
}

func TestContract_AuthProviderResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodPost, apiPath("/providers"), http.StatusOK,
		`[{"id":"provider-1","name":"Example IdP","typ":"oidc","enabled":true,`+
			`"issuer":"https://idp.example.com",`+
			`"authorization_endpoint":"https://idp.example.com/authorize",`+
			`"token_endpoint":"https://idp.example.com/token",`+
			`"userinfo_endpoint":"https://idp.example.com/userinfo","jwks_endpoint":null,`+
			`"client_id":"rauthy","client_secret":"upstream-secret",`+
			`"scope":"email+openid+profile",`+
			`"admin_claim_path":null,"admin_claim_value":null,`+
			`"mfa_claim_path":null,"mfa_claim_value":null,`+
			`"use_pkce":true,"client_secret_basic":true,"client_secret_post":false,`+
			`"auto_onboarding":false,"auto_link":false}]`)
	if !ok {
		t.Errorf("POST /providers response rejected by the spec: %s", msg)
	}
}

// KNOWN API/SPEC DIVERGENCE, and the one that shapes the whole resource: an
// upstream provider's `scope` cannot be sent back in the form it is read in.
//
// The document types the field as a plain string in both directions and says
// nothing more, because utoipa drops the `validator` constraints. A live 0.36.2
// validates writes against ^[a-zA-Z0-9-_/:\s*]{0,512}$, which has no `+` in the
// character class, and yet stores and returns the list `+`-joined. Creating with
// "openid profile email" answers 200 with "openid+profile+email", and feeding
// that back to the update endpoint answers 400 with a payload validation error
// naming the scope field.
//
// Both forms satisfy the document, so the contract validator cannot see any of
// this — which is exactly why it is written down here. The provider package
// models the field as a set and converts in both directions.
func TestContract_AuthProviderScopeIsNotRoundTrippable(t *testing.T) {
	v := newContractValidator(t)

	for _, scope := range []string{"openid profile email", "openid+profile+email"} {
		ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/providers/provider-1"),
			client.AuthProviderRequest{
				Name: "Example IdP", Typ: "oidc", Enabled: true,
				Issuer:                "https://idp.example.com",
				AuthorizationEndpoint: "https://idp.example.com/authorize",
				TokenEndpoint:         "https://idp.example.com/token",
				UserinfoEndpoint:      "https://idp.example.com/userinfo",
				ClientID:              "rauthy",
				Scope:                 scope,
			})
		if !ok {
			t.Errorf("the spec now rejects scope %q, so it may have grown the pattern a live "+
				"server enforces; re-check splitAuthProviderScope: %s", scope, msg)
		}
	}
}

func TestContract_AuthProviderLookup(t *testing.T) {
	v := newContractValidator(t)

	issuer := "accounts.google.com"
	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/providers/lookup"),
		client.AuthProviderLookupRequest{Issuer: &issuer})
	if !ok {
		t.Errorf("POST /providers/lookup body rejected by the spec: %s", msg)
	}

	ok, msg = validateResponse(t, v, http.MethodPost, apiPath("/providers/lookup"), http.StatusOK,
		`{"issuer":"https://accounts.google.com",`+
			`"authorization_endpoint":"https://accounts.google.com/o/oauth2/v2/auth",`+
			`"token_endpoint":"https://oauth2.googleapis.com/token",`+
			`"userinfo_endpoint":"https://openidconnect.googleapis.com/v1/userinfo",`+
			`"jwks_endpoint":"https://www.googleapis.com/oauth2/v3/certs",`+
			`"scope":"openid profile email ","use_pkce":true,`+
			`"client_secret_basic":true,"client_secret_post":true}`)
	if !ok {
		t.Errorf("POST /providers/lookup response rejected by the spec: %s", msg)
	}
}

// The PAM contract tests come in two flavours. The ordinary ones assert that a
// body this package sends or accepts matches the spec. The ones named
// *_SpecDivergence assert the opposite — that the vendored spec rejects what a
// live 0.36.2 requires — and exist so that a spec bump which finally fixes the
// document fails here loudly instead of leaving a stale comment behind.

func TestContract_CreatePamUserRequest(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/pam/users"), client.PamUserCreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
	})
	if !ok {
		t.Errorf("POST /pam/users body rejected by the spec: %s", msg)
	}
}

func TestContract_UpdatePamUserRequest(t *testing.T) {
	v := newContractValidator(t)

	homeDir := "/home/alice"
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/pam/users/100000"), client.PamUserUpdateRequest{
		Shell:   "/bin/zsh",
		HomeDir: &homeDir,
		Groups: []client.PamGroupUserLink{
			{UID: 100000, GID: 100002, Wheel: true},
		},
	})
	if !ok {
		t.Errorf("PUT /pam/users/{uid} body rejected by the spec: %s", msg)
	}
}

func TestContract_PamUserDetailsResponse(t *testing.T) {
	v := newContractValidator(t)

	body := `{"id":100000,"name":"alice","gid":100003,"email":"alice@example.com",` +
		`"shell":"/bin/zsh","home_dir":"/home/alice",` +
		`"groups":[{"uid":100000,"gid":100002,"wheel":true}],"authorized_keys":[]}`
	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/pam/users/100000"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /pam/users/{uid} response rejected by the spec: %s", msg)
	}

	var got client.PamUserDetailsResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.GID != 100003 || len(got.Groups) != 1 || !got.Groups[0].Wheel {
		t.Errorf("decoded %+v", got)
	}
}

func TestContract_PamGroupResponse(t *testing.T) {
	v := newContractValidator(t)

	// Validated against the create operation's response, which is the only
	// place the spec describes a PamGroupResponse at all — see the divergence
	// below for why that operation is not what its summary claims.
	ok, msg := validateResponse(t, v, http.MethodPost, apiPath("/pam/groups"), http.StatusOK,
		`[{"id":100002,"name":"developers","typ":"generic"}]`)
	if !ok {
		t.Errorf("PAM group response rejected by the spec: %s", msg)
	}
}

// DIVERGENCE ONE, pinned. POST /pam/groups is documented as a listing: no
// request body, and an array of groups in the response. A live server takes a
// PamGroupCreateRequest there and answers with the single group it created,
// which is the shape the spec refuses. The pin is on the response rather than
// the request because the validator ignores a body on an operation that
// declares none, so only the response half of the mix-up is observable here.
func TestContract_CreatePamGroupResponse_SpecDivergence(t *testing.T) {
	v := newContractValidator(t)

	ok, _ := validateResponse(t, v, http.MethodPost, apiPath("/pam/groups"), http.StatusOK,
		`{"id":100002,"name":"developers","typ":"generic"}`)
	if ok {
		t.Error("the spec now accepts a single group from POST /pam/groups; " +
			"the create/list mix-up documented in client/pam.go may be fixed — re-check it")
	}
}

// DIVERGENCE THREE, pinned. The spec has no POST /pam/hosts at all, so it
// cannot validate the create body a live server accepts.
func TestContract_CreatePamHostRequest_SpecDivergence(t *testing.T) {
	v := newContractValidator(t)

	ok, _ := validateRequest(t, v, http.MethodPost, apiPath("/pam/hosts"), client.PamHostCreateRequest{
		Hostname: "build01",
		GID:      100001,
	})
	if ok {
		t.Error("the spec now describes POST /pam/hosts; " +
			"the create operation documented as missing in client/pam.go may be back — re-check it")
	}
}

// The address list divergence, pinned from both directions: the spec types
// `ips` as a string, so the array a live server requires is rejected, and the
// array a live server returns is rejected too.
func TestContract_PamHostAddresses_SpecDivergence(t *testing.T) {
	v := newContractValidator(t)

	notes := "CI builder"
	ok, _ := validateRequest(t, v, http.MethodPut, apiPath("/pam/hosts/h1"), client.PamHostUpdateRequest{
		Hostname:          "build01",
		GID:               100001,
		ForceMfa:          true,
		LocalPasswordOnly: false,
		IPs:               []string{"10.0.0.10"},
		Aliases:           []string{"ci"},
		Notes:             &notes,
	})
	if ok {
		t.Error("the spec now accepts an array for PamHostUpdateRequest.ips; " +
			"the string/array divergence documented in client/pam.go may be fixed — re-check it")
	}

	body := `{"id":"h1","hostname":"build01","gid":100001,"force_mfa":true,` +
		`"local_password_only":false,"notes":null,"ips":["10.0.0.10"],"aliases":["ci"]}`
	ok, _ = validateResponse(t, v, http.MethodGet, apiPath("/pam/hosts/h1"), http.StatusOK, body)
	if ok {
		t.Error("the spec now accepts an array for PamHostDetailsResponse.ips; re-check the divergence")
	}

	// Whatever the document says, the shape the server actually sends must
	// decode cleanly into the wire type.
	var got client.PamHostDetailsResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.IPs) != 1 || got.IPs[0] != "10.0.0.10" {
		t.Errorf("decoded %+v", got)
	}
}

func TestContract_PamHostSecretResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodPost, apiPath("/pam/hosts/h1/secret"), http.StatusOK,
		`{"id":"h1","secret":"s3cr3t"}`)
	if !ok {
		t.Errorf("POST /pam/hosts/{id}/secret response rejected by the spec: %s", msg)
	}
}

func TestContract_BlacklistIPRequest(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/blacklist"), client.IPBlacklistRequest{
		IP:  "203.0.113.7",
		Exp: 4102444800,
	})
	if !ok {
		t.Errorf("POST /blacklist body rejected by the spec: %s", msg)
	}
}

func TestContract_BlacklistResponse(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/blacklist"), http.StatusOK,
		`{"ips":[{"ip":"203.0.113.7","exp":4102444800},{"ip":"2001:db8::2","exp":4102444800}]}`)
	if !ok {
		t.Errorf("GET /blacklist response rejected by the spec: %s", msg)
	}
}
