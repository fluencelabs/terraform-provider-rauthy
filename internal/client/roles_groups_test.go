package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func TestCreateRole_SendsRoleField(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"role-1","name":"admin"}`))
	})

	got, err := c.CreateRole(context.Background(), client.RoleRequest{Role: "admin"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/auth/v1/roles" {
		t.Errorf("got %s %s, want POST /auth/v1/roles", gotMethod, gotPath)
	}
	if gotBody["role"] != "admin" {
		t.Errorf("body = %v, want role=admin", gotBody)
	}
	if got.ID != "role-1" || got.Name != "admin" {
		t.Errorf("got %+v", got)
	}
}

// Rauthy has no GET /roles/{id}, so GetRole filters the list. A role that is
// not in it has to look like a 404 to the resource layer, which relies on
// IsNotFound to drop the resource from state.
func TestGetRole_MissingRoleIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/auth/v1/roles" {
			t.Errorf("got %s %s, want GET /auth/v1/roles", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"role-1","name":"admin"}]`))
	})

	got, err := c.GetRole(context.Background(), "role-1")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.Name != "admin" {
		t.Errorf("got %+v, want admin", got)
	}

	if _, missingErr := c.GetRole(context.Background(), "role-2"); !client.IsNotFound(missingErr) {
		t.Errorf("GetRole(missing) error = %v, want a 404", missingErr)
	}
}

func TestGetRoleByName_MissingRoleIsNotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"role-1","name":"admin"}]`))
	})

	got, err := c.GetRoleByName(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetRoleByName: %v", err)
	}
	if got.ID != "role-1" {
		t.Errorf("got %+v, want role-1", got)
	}

	if _, missingErr := c.GetRoleByName(context.Background(), "nope"); !client.IsNotFound(missingErr) {
		t.Errorf("GetRoleByName(missing) error = %v, want a 404", missingErr)
	}
}

func TestUpdateRole_PutsToRolePath(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"role-1","name":"operator"}`))
	})

	got, err := c.UpdateRole(context.Background(), "role-1", client.RoleRequest{Role: "operator"})
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/auth/v1/roles/role-1" {
		t.Errorf("got %s %s, want PUT /auth/v1/roles/role-1", gotMethod, gotPath)
	}
	if got.Name != "operator" {
		t.Errorf("got %+v", got)
	}
}

// Group ids come back from Rauthy and are used verbatim in the path, so they
// have to survive escaping.
func TestGroupPaths_EscapeTheID(t *testing.T) {
	t.Parallel()

	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
	})

	if err := c.DeleteGroup(context.Background(), "a/b"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if gotPath != "/auth/v1/groups/a%2Fb" {
		t.Errorf("path = %q, want the id escaped", gotPath)
	}
}

func TestCreateGroup_SendsGroupField(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"group-1","name":"developers"}`))
	})

	got, err := c.CreateGroup(context.Background(), client.GroupRequest{Group: "developers"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if gotBody["group"] != "developers" {
		t.Errorf("body = %v, want group=developers", gotBody)
	}
	if got.ID != "group-1" {
		t.Errorf("got %+v", got)
	}
}

// An unset optional field must serialise as an explicit null, not be omitted:
// PUT /password_policy replaces the policy, and a missing field is how a rule
// gets switched off.
func TestUpdatePasswordPolicy_SendsExplicitNulls(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/auth/v1/password_policy" {
			t.Errorf("got %s %s, want PUT /auth/v1/password_policy", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"length_min":12,"length_max":128,"include_digits":1}`))
	})

	digits := int32(1)
	got, err := c.UpdatePasswordPolicy(context.Background(), client.PasswordPolicy{
		LengthMin:     12,
		LengthMax:     128,
		IncludeDigits: &digits,
	})
	if err != nil {
		t.Fatalf("UpdatePasswordPolicy: %v", err)
	}

	for _, field := range []string{"include_lower_case", "include_upper_case", "valid_days", "not_recently_used"} {
		v, present := gotBody[field]
		if !present {
			t.Errorf("%s omitted from the body, want an explicit null", field)
			continue
		}
		if v != nil {
			t.Errorf("%s = %v, want null", field, v)
		}
	}
	if got.IncludeDigits == nil || *got.IncludeDigits != 1 {
		t.Errorf("include_digits = %v, want 1", got.IncludeDigits)
	}
	if got.ValidDays != nil {
		t.Errorf("valid_days = %v, want nil", got.ValidDays)
	}
}
