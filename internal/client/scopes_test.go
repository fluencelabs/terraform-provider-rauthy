package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// The request carries the attribute mappings as arrays; the response returns
// them comma-joined. Both halves of that asymmetry are pinned here.
func TestCreateScope_SendsArraysAndSplitsTheJoinedResponse(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"scope-1","name":"read:billing",` +
			`"attr_include_access":"department,cost_center","attr_include_id":"department"}`))
	})

	got, err := c.CreateScope(context.Background(), client.ScopeRequest{
		Scope:             "read:billing",
		AttrIncludeAccess: []string{"department", "cost_center"},
		AttrIncludeID:     []string{"department"},
	})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/auth/v1/scopes" {
		t.Errorf("got %s %s, want POST /auth/v1/scopes", gotMethod, gotPath)
	}
	access, ok := gotBody["attr_include_access"].([]any)
	if !ok || len(access) != 2 {
		t.Errorf("attr_include_access = %#v, want a two-element array", gotBody["attr_include_access"])
	}

	if want := (client.AttrList{"department", "cost_center"}); !reflect.DeepEqual(got.AttrIncludeAccess, want) {
		t.Errorf("AttrIncludeAccess = %v, want %v", got.AttrIncludeAccess, want)
	}
	if want := (client.AttrList{"department"}); !reflect.DeepEqual(got.AttrIncludeID, want) {
		t.Errorf("AttrIncludeID = %v, want %v", got.AttrIncludeID, want)
	}
}

// Rauthy answers with two different shapes for the same field: POST /scopes
// returns the stored comma-joined string, while GET /scopes and PUT
// /scopes/{id} return an array. The vendored OpenAPI document describes only
// the string, so this is invisible to the contract tests — it was found by
// running the acceptance suite against a live instance.
func TestAttrList_DecodesBothShapes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		raw  string
		want client.AttrList
	}{
		"null":                    {`null`, nil},
		"joined string":           {`"department,cost_center"`, client.AttrList{"department", "cost_center"}},
		"empty string":            {`""`, nil},
		"string of separators":    {`" , "`, nil},
		"padded joined string":    {`" department , cost_center "`, client.AttrList{"department", "cost_center"}},
		"array":                   {`["department","cost_center"]`, client.AttrList{"department", "cost_center"}},
		"empty array":             {`[]`, nil},
		"array holding one empty": {`[""]`, nil},
	}

	for name, tc := range cases {
		var got client.AttrList
		if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
			t.Errorf("%s: unmarshal %s: %v", name, tc.raw, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: unmarshal %s = %#v, want %#v", name, tc.raw, got, tc.want)
		}
	}
}

// "no attributes" comes back from Rauthy as an empty string or as an array
// holding one empty string; neither may become an attribute named "".
func TestAttrList_RejectsAnEmptyAttributeName(t *testing.T) {
	t.Parallel()

	var s client.ScopeResponse
	body := `{"id":"scope-1","name":"probe","attr_include_access":"","attr_include_id":[""]}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.AttrIncludeAccess != nil || s.AttrIncludeID != nil {
		t.Errorf("got access=%#v id=%#v, want both nil", s.AttrIncludeAccess, s.AttrIncludeID)
	}
}

// Unset mappings must be omitted from the body rather than sent as null: the
// field is Option<Vec<String>> upstream, and an explicit null is not the same
// as absent for Rauthy's deserializer.
func TestScopeRequest_OmitsEmptyMappings(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"scope-1","name":"plain"}`))
	})

	got, err := c.CreateScope(context.Background(), client.ScopeRequest{Scope: "plain"})
	if err != nil {
		t.Fatalf("CreateScope: %v", err)
	}
	for _, field := range []string{"attr_include_access", "attr_include_id"} {
		if _, present := gotBody[field]; present {
			t.Errorf("%s present in the body, want it omitted", field)
		}
	}
	if got.AttrIncludeAccess != nil {
		t.Errorf("AttrIncludeAccess = %v, want nil", got.AttrIncludeAccess)
	}
}

func TestGetScope_MissingScopeIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/auth/v1/scopes" {
			t.Errorf("got %s %s, want GET /auth/v1/scopes", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"scope-1","name":"openid"}]`))
	})

	if _, err := c.GetScope(context.Background(), "scope-1"); err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	if _, err := c.GetScope(context.Background(), "scope-2"); !client.IsNotFound(err) {
		t.Errorf("GetScope(missing) error = %v, want a 404", err)
	}
	if _, err := c.GetScopeByName(context.Background(), "nope"); !client.IsNotFound(err) {
		t.Errorf("GetScopeByName(missing) error = %v, want a 404", err)
	}
}

func TestUpdateScope_PutsToScopePath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"a/b","name":"renamed"}`))
	})

	if _, err := c.UpdateScope(context.Background(), "a/b", client.ScopeRequest{Scope: "renamed"}); err != nil {
		t.Fatalf("UpdateScope: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/auth/v1/scopes/a%2Fb" {
		t.Errorf("got %s %s, want PUT with the id escaped", gotMethod, gotPath)
	}
}
