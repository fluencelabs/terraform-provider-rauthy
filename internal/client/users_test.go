package client_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func TestContract_CreateUserRequest(t *testing.T) {
	v := newContractValidator(t)

	given := "Ada"
	family := "Lovelace"
	expires := int64(4102444800)
	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/users"), client.NewUserRequest{
		Email:       "ada@example.com",
		Language:    "en",
		Roles:       []string{"admin"},
		Groups:      []string{"engineering"},
		GivenName:   &given,
		FamilyName:  &family,
		UserExpires: &expires,
	})
	if !ok {
		t.Errorf("POST /users body rejected by the spec: %s", msg)
	}
}

// Roles is required even when empty: a nil slice would marshal to null and
// Rauthy's deserializer rejects that, which is why the provider sends [].
func TestContract_CreateUserRequiresRoles(t *testing.T) {
	v := newContractValidator(t)

	partial := map[string]any{"email": "ada@example.com", "language": "en"}
	if ok, _ := validateRequest(t, v, http.MethodPost, apiPath("/users"), partial); ok {
		t.Error("POST /users accepted a body without roles; the spec no longer requires it, " +
			"revisit NewUserRequest")
	}
}

func TestContract_UpdateUserRequest(t *testing.T) {
	v := newContractValidator(t)

	given := "Ada"
	lang := "en"
	city := "London"
	tz := "Europe/London"
	ok, msg := validateRequest(t, v, http.MethodPut, apiPath("/users/u-1"), client.UpdateUserRequest{
		Email:         "ada@example.com",
		Enabled:       true,
		EmailVerified: true,
		Roles:         []string{"admin"},
		Groups:        []string{},
		GivenName:     &given,
		Language:      &lang,
		UserValues:    &client.UserValues{City: &city, TZ: &tz},
	})
	if !ok {
		t.Errorf("PUT /users/{id} body rejected by the spec: %s", msg)
	}
}

// A PUT is a full replacement and Rauthy's deserializer requires these four.
// If a future spec relaxes that, the provider may stop sending them.
func TestContract_UpdateUserRequiresEnabledAndVerified(t *testing.T) {
	v := newContractValidator(t)

	partial := map[string]any{"email": "ada@example.com", "roles": []string{}}
	if ok, _ := validateRequest(t, v, http.MethodPut, apiPath("/users/u-1"), partial); ok {
		t.Error("PUT /users/{id} accepted a body without enabled/email_verified; " +
			"the spec no longer requires them, revisit UpdateUserRequest")
	}
}

func TestContract_UserResponse(t *testing.T) {
	v := newContractValidator(t)

	body := `{
		"id": "u-1",
		"email": "ada@example.com",
		"given_name": "Ada",
		"language": "en",
		"roles": ["admin"],
		"groups": ["engineering"],
		"enabled": true,
		"email_verified": true,
		"created_at": 1700000000,
		"account_type": "password",
		"user_values": {"city": "London", "preferred_username": "ada"}
	}`

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/users/u-1"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /users/{id} response rejected by the spec: %s", msg)
	}

	var decoded client.UserResponse
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode into UserResponse: %v", err)
	}
	if decoded.ID != "u-1" || decoded.AccountType != "password" || !decoded.EmailVerified {
		t.Errorf("UserResponse lost fields on decode: %+v", decoded)
	}
	// preferred_username is promoted from the embedded UserValues, so a
	// mistake in that embedding shows up as a silently empty value.
	if decoded.UserValues.PreferredUsername == nil || *decoded.UserValues.PreferredUsername != "ada" {
		t.Errorf("preferred_username did not decode: %+v", decoded.UserValues)
	}
	if decoded.UserValues.City == nil || *decoded.UserValues.City != "London" {
		t.Errorf("user_values.city did not decode: %+v", decoded.UserValues)
	}
}
