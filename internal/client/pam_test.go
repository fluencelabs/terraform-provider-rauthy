package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// The PAM endpoints are the ones where the OpenAPI document and the live server
// disagree most, so each divergence the client depends on is pinned here: if a
// future refactor "corrects" the code back to what the document says, one of
// these fails instead of a live acceptance run three steps later.

// DIVERGENCE ONE. The document has POST /pam/groups as the listing and knows no
// GET at all; a live server has it the other way round.
func TestPamGroups_PostCreatesAndGetLists(t *testing.T) {
	t.Parallel()

	var createMethod, createPath, listMethod, listPath string
	var createBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			createMethod, createPath = r.Method, r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			_, _ = w.Write([]byte(`{"id":100002,"name":"developers","typ":"generic"}`))
			return
		}
		listMethod, listPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`[{"id":100002,"name":"developers","typ":"generic"}]`))
	})

	created, err := c.CreatePamGroup(context.Background(), client.PamGroupCreateRequest{
		Name: "developers", Typ: "generic",
	})
	if err != nil {
		t.Fatalf("CreatePamGroup: %v", err)
	}
	if createMethod != http.MethodPost || createPath != "/auth/v1/pam/groups" {
		t.Errorf("got %s %s, want POST /auth/v1/pam/groups", createMethod, createPath)
	}
	if createBody["name"] != "developers" || createBody["typ"] != "generic" {
		t.Errorf("create body = %v", createBody)
	}
	if created.ID != 100002 {
		t.Errorf("created = %+v", created)
	}

	groups, err := c.ListPamGroups(context.Background())
	if err != nil {
		t.Fatalf("ListPamGroups: %v", err)
	}
	if listMethod != http.MethodGet || listPath != "/auth/v1/pam/groups" {
		t.Errorf("got %s %s, want GET /auth/v1/pam/groups", listMethod, listPath)
	}
	if len(groups) != 1 || groups[0].Name != "developers" {
		t.Errorf("groups = %+v", groups)
	}
}

// GetPamGroup is a filtered listing, so a gid nobody has must come back as a
// synthetic 404 rather than as an empty group.
func TestGetPamGroup_MissingGidIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":100002,"name":"developers","typ":"generic"}]`))
	})

	if _, err := c.GetPamGroup(context.Background(), 100003); !client.IsNotFound(err) {
		t.Fatalf("err = %v, want a 404", err)
	}
}

// DIVERGENCE TWO. The document lists no DELETE for a PAM user; the server has
// one, and without it the resource could not be destroyed.
func TestDeletePamUser_UsesDeleteOnTheUserPath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
	})

	if err := c.DeletePamUser(context.Background(), 100000); err != nil {
		t.Fatalf("DeletePamUser: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/auth/v1/pam/users/100000" {
		t.Errorf("got %s %s, want DELETE /auth/v1/pam/users/100000", gotMethod, gotPath)
	}
}

// DIVERGENCE THREE. The document has no POST /pam/hosts; the server does, which
// is what makes rauthy_pam_host a create-able resource instead of an
// import-only one.
func TestCreatePamHost_PostsToHosts(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(
			`{"id":"cZNsjgeHChYfPCTrVYBQovsj","name":"build01","aliases":[],"addresses":[]}`))
	})

	got, err := c.CreatePamHost(context.Background(), client.PamHostCreateRequest{
		Hostname: "build01", GID: 100001, ForceMfa: true,
	})
	if err != nil {
		t.Fatalf("CreatePamHost: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/pam/hosts" {
		t.Errorf("got %s %s, want POST /auth/v1/pam/hosts", gotMethod, gotPath)
	}
	// local_password_only is a required field of the body and false is a
	// meaningful value for it, so it must be on the wire rather than omitted.
	if _, ok := gotBody["local_password_only"]; !ok {
		t.Errorf("body = %v, want local_password_only present", gotBody)
	}
	if got.ID != "cZNsjgeHChYfPCTrVYBQovsj" {
		t.Errorf("got %+v", got)
	}
}

// The host's address list is `ips` in the details and `addresses` in the
// listing, and the document types both as a bare string. A live server sends an
// array in both places; decoding must follow the server.
func TestPamHost_AddressListsDecodeAsArrays(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/v1/pam/hosts" {
			_, _ = w.Write([]byte(
				`[{"id":"h1","name":"build01","aliases":["ci"],"addresses":["10.0.0.10"]}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"h1","hostname":"build01","gid":100001,"force_mfa":false,` +
			`"local_password_only":false,"notes":null,"ips":["10.0.0.10","2001:db8::10"],"aliases":["ci"]}`))
	})

	hosts, err := c.ListPamHosts(context.Background())
	if err != nil {
		t.Fatalf("ListPamHosts: %v", err)
	}
	if len(hosts) != 1 || len(hosts[0].Addresses) != 1 || hosts[0].Addresses[0] != "10.0.0.10" {
		t.Errorf("hosts = %+v", hosts)
	}

	details, err := c.GetPamHost(context.Background(), "h1")
	if err != nil {
		t.Fatalf("GetPamHost: %v", err)
	}
	if len(details.IPs) != 2 || details.Notes != nil {
		t.Errorf("details = %+v", details)
	}
}

// Reading a host's secret is a POST, which is easy to mistake for the rotation
// on the same path — that one is a PUT, and the provider must never issue it.
func TestGetPamHostSecret_PostsAndNeverRotates(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"h1","secret":"s3cr3t"}`))
	})

	got, err := c.GetPamHostSecret(context.Background(), "h1")
	if err != nil {
		t.Fatalf("GetPamHostSecret: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/pam/hosts/h1/secret" {
		t.Errorf("got %s %s, want POST /auth/v1/pam/hosts/h1/secret", gotMethod, gotPath)
	}
	if got.Secret != "s3cr3t" {
		t.Errorf("got %+v", got)
	}
}

// The PUT body must carry an empty array rather than null when the membership
// set is cleared: `groups` is a required field, and null is a 422.
func TestUpdatePamUser_SendsEmptyGroupsAsArray(t *testing.T) {
	t.Parallel()

	var gotBody map[string]json.RawMessage
	c := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
	})

	err := c.UpdatePamUser(context.Background(), 100000, client.PamUserUpdateRequest{
		Shell:  "/bin/bash",
		Groups: []client.PamGroupUserLink{},
	})
	if err != nil {
		t.Fatalf("UpdatePamUser: %v", err)
	}
	if string(gotBody["groups"]) != "[]" {
		t.Errorf("groups = %s, want []", gotBody["groups"])
	}
	if _, ok := gotBody["home_dir"]; ok {
		t.Errorf("home_dir must be omitted when unset, body = %v", gotBody)
	}
}
