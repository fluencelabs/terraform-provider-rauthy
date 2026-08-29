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

	if want := []string{"department", "cost_center"}; !reflect.DeepEqual(got.AttrIncludeAccessList(), want) {
		t.Errorf("AttrIncludeAccessList() = %v, want %v", got.AttrIncludeAccessList(), want)
	}
	if want := []string{"department"}; !reflect.DeepEqual(got.AttrIncludeIDList(), want) {
		t.Errorf("AttrIncludeIDList() = %v, want %v", got.AttrIncludeIDList(), want)
	}
}

// A null or blank mapping must read as "unset", not as an empty-string
// attribute name: the provider renders nil as a null set.
func TestScopeResponse_SplitsAbsentMappingsToNil(t *testing.T) {
	t.Parallel()

	blank := ""
	spaces := " , "
	joined := " department , cost_center "

	cases := map[string]struct {
		in   *string
		want []string
	}{
		"null":            {nil, nil},
		"empty string":    {&blank, nil},
		"only separators": {&spaces, nil},
		"padded":          {&joined, []string{"department", "cost_center"}},
	}
	for name, tc := range cases {
		s := client.ScopeResponse{AttrIncludeAccess: tc.in, AttrIncludeID: tc.in}
		if got := s.AttrIncludeAccessList(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: AttrIncludeAccessList() = %#v, want %#v", name, got, tc.want)
		}
		if got := s.AttrIncludeIDList(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: AttrIncludeIDList() = %#v, want %#v", name, got, tc.want)
		}
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
	if got.AttrIncludeAccessList() != nil {
		t.Errorf("AttrIncludeAccessList() = %v, want nil", got.AttrIncludeAccessList())
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
