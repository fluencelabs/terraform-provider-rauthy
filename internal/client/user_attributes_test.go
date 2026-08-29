package client_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func TestContract_UserAttrRequest(t *testing.T) {
	v := newContractValidator(t)

	desc := "cost-center"
	editable := false
	body := client.UserAttrRequest{
		Name:         "department",
		Desc:         &desc,
		DefaultValue: json.RawMessage(`"engineering"`),
		UserEditable: &editable,
	}

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/users/attr"), body)
	if !ok {
		t.Errorf("POST /users/attr body rejected by the spec: %s", msg)
	}

	ok, msg = validateRequest(t, v, http.MethodPut, apiPath("/users/attr/department"), body)
	if !ok {
		t.Errorf("PUT /users/attr/{name} body rejected by the spec: %s", msg)
	}
}

// default_value is `serde_json::Value` upstream, so anything that is JSON is a
// legal default. The provider passes the configured document through untouched
// and this pins that the spec really does allow more than a string.
func TestContract_UserAttrRequestTakesAnyJSONDefault(t *testing.T) {
	v := newContractValidator(t)

	for name, raw := range map[string]string{
		"string": `"engineering"`,
		"number": `42`,
		"bool":   `true`,
		"array":  `["a","b"]`,
		"object": `{"a":1}`,
	} {
		ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/users/attr"), client.UserAttrRequest{
			Name:         "department",
			DefaultValue: json.RawMessage(raw),
		})
		if !ok {
			t.Errorf("POST /users/attr with a %s default rejected by the spec: %s", name, msg)
		}
	}
}

// Only the name is required; the provider relies on that when a configuration
// sets nothing but a name.
func TestContract_UserAttrRequestNeedsOnlyAName(t *testing.T) {
	v := newContractValidator(t)

	ok, msg := validateRequest(t, v, http.MethodPost, apiPath("/users/attr"),
		client.UserAttrRequest{Name: "department"})
	if !ok {
		t.Errorf("POST /users/attr with only a name rejected by the spec: %s", msg)
	}

	ok, _ = validateRequest(t, v, http.MethodPost, apiPath("/users/attr"), map[string]any{
		"desc": "cost-center",
	})
	if ok {
		t.Error("POST /users/attr accepted a body without a name; the spec no longer requires it, " +
			"revisit UserAttrRequest")
	}
}

// The list is an object with a `values` array, not the bare array that /scopes,
// /roles and /groups answer with. userAttrListResponse exists for that reason,
// so both halves of this test matter.
func TestContract_UserAttrListResponseIsWrapped(t *testing.T) {
	v := newContractValidator(t)

	body := `{"values":[{"name":"department","desc":"cost-center",` +
		`"default_value":"engineering","user_editable":true}]}`

	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/users/attr"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /users/attr response rejected by the spec: %s", msg)
	}

	if ok, _ = validateResponse(t, v, http.MethodGet, apiPath("/users/attr"), http.StatusOK,
		`[{"name":"department","user_editable":true}]`); ok {
		t.Error("GET /users/attr now accepts a bare array; the response was unwrapped, " +
			"revisit userAttrListResponse")
	}
}

func TestContract_UserAttrResponse(t *testing.T) {
	v := newContractValidator(t)

	body := `{"name":"department","desc":"cost-center","default_value":{"code":7},"user_editable":true}`

	ok, msg := validateResponse(t, v, http.MethodPost, apiPath("/users/attr"), http.StatusOK, body)
	if !ok {
		t.Errorf("POST /users/attr response rejected by the spec: %s", msg)
	}

	var decoded client.UserAttrResponse
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode into UserAttrResponse: %v", err)
	}
	if decoded.Name != "department" || !decoded.UserEditable {
		t.Errorf("UserAttrResponse lost fields on decode: %+v", decoded)
	}
	if decoded.Desc == nil || *decoded.Desc != "cost-center" {
		t.Errorf("desc did not decode: %+v", decoded.Desc)
	}
	// The default is kept as raw bytes so a non-string document survives.
	if string(decoded.DefaultValue) != `{"code":7}` {
		t.Errorf("default_value = %s, want the object verbatim", decoded.DefaultValue)
	}
}

// An attribute with no description and no default: both fields are nullable
// upstream and the provider must cope with them being absent entirely.
func TestContract_UserAttrResponseWithoutOptionalFields(t *testing.T) {
	v := newContractValidator(t)

	body := `{"values":[{"name":"department","user_editable":false}]}`
	ok, msg := validateResponse(t, v, http.MethodGet, apiPath("/users/attr"), http.StatusOK, body)
	if !ok {
		t.Errorf("GET /users/attr response without optional fields rejected by the spec: %s", msg)
	}

	var decoded struct {
		Values []client.UserAttrResponse `json:"values"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Values) != 1 || decoded.Values[0].Desc != nil || decoded.Values[0].DefaultValue != nil {
		t.Errorf("absent optional fields did not decode as nil: %+v", decoded.Values)
	}
}

// `typ` is documented as "Currently ignored - will be implemented in a future
// version" and its only member is Email, which is why UserAttrRequest omits it.
// This pins the assumption: if a Rauthy release grows the enum, the field is
// worth exposing and this stops being a one-value no-op.
func TestContract_UserAttrTypIsStillASingleIgnoredValue(t *testing.T) {
	v := newContractValidator(t)

	if ok, _ := validateRequest(t, v, http.MethodPost, apiPath("/users/attr"), map[string]any{
		"name": "department",
		"typ":  "Phone",
	}); ok {
		t.Error("POST /users/attr accepted typ=Phone; UserAttrConfigTyp gained members, " +
			"reconsider exposing typ on rauthy_user_attribute")
	}
}
