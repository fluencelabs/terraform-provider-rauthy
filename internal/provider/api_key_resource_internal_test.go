package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func accessSetOf(t *testing.T, entries ...client.APIKeyAccess) types.Set {
	t.Helper()

	elems := make([]attr.Value, 0, len(entries))
	for _, e := range entries {
		elems = append(elems, types.ObjectValueMust(apiKeyAccessAttrTypes, map[string]attr.Value{
			"group":         types.StringValue(e.Group),
			"access_rights": stringsToSet(e.AccessRights),
		}))
	}
	return types.SetValueMust(apiKeyAccessObjectType(), elems)
}

// A key's grants have to survive the round trip unchanged, because Rauthy is a
// full replacement on update: anything the mapping loses here is a right the
// next unrelated apply silently revokes.
func TestAPIKeyAccess_RoundTrips(t *testing.T) {
	t.Parallel()

	want := []client.APIKeyAccess{
		{Group: "Users", AccessRights: []string{"read", "create"}},
		{Group: "ApiKeys", AccessRights: []string{"read", "create", "update", "delete"}},
		// A group with no rights at all: Rauthy stores it and hands it back.
		{Group: "Events", AccessRights: []string{}},
	}

	var diags diag.Diagnostics
	got := apiKeyAccessFromSet(context.Background(), apiKeyAccessToSet(want), &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}

	byGroup := map[string][]string{}
	for _, g := range got {
		byGroup[g.Group] = g.AccessRights
	}
	for _, w := range want {
		rights, ok := byGroup[w.Group]
		if !ok {
			t.Errorf("group %s lost in the round trip", w.Group)
			continue
		}
		if len(rights) != len(w.AccessRights) {
			t.Errorf("group %s: rights = %v, want %v", w.Group, rights, w.AccessRights)
		}
	}
}

// A null or unknown set must not marshal as `"access": null`: Rauthy's
// deserializer requires the field and refuses a null, so the empty case has to
// be an empty slice rather than nil.
func TestAPIKeyAccessFromSet_NeverReturnsNil(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	for name, set := range map[string]types.Set{
		"null":    types.SetNull(apiKeyAccessObjectType()),
		"unknown": types.SetUnknown(apiKeyAccessObjectType()),
		"empty":   types.SetValueMust(apiKeyAccessObjectType(), nil),
	} {
		got := apiKeyAccessFromSet(context.Background(), set, &diags)
		if got == nil {
			t.Errorf("%s set produced a nil slice", name)
		}
		if len(got) != 0 {
			t.Errorf("%s set produced %+v", name, got)
		}
	}

	// A group whose rights set is null must likewise become `[]`.
	set := types.SetValueMust(apiKeyAccessObjectType(), []attr.Value{
		types.ObjectValueMust(apiKeyAccessAttrTypes, map[string]attr.Value{
			"group":         types.StringValue("Events"),
			"access_rights": types.SetNull(types.StringType),
		}),
	})
	got := apiKeyAccessFromSet(context.Background(), set, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(got) != 1 || got[0].AccessRights == nil {
		t.Errorf("got %+v, want one entry with a non-nil rights slice", got)
	}
}

// applyAPIKeyResponse must not touch the two attributes Rauthy knows nothing
// about. The secret is unreadable after creation, so a refresh that cleared it
// would throw away the only copy that exists.
func TestApplyAPIKeyResponse_LeavesTheSecretAlone(t *testing.T) {
	t.Parallel()

	m := &apiKeyResourceModel{
		Secret:                types.StringValue("deploy$s3cr3t"),
		SecretRotationTrigger: types.StringValue("v1"),
	}
	expires := int64(1900000000)
	applyAPIKeyResponse(m, &client.APIKeyResponse{
		Name:    "deploy",
		Created: 1788076923,
		Expires: &expires,
		Access:  []client.APIKeyAccess{{Group: "Users", AccessRights: []string{"read"}}},
	})

	if m.Secret.ValueString() != "deploy$s3cr3t" || m.SecretRotationTrigger.ValueString() != "v1" {
		t.Errorf("secret = %v, trigger = %v", m.Secret, m.SecretRotationTrigger)
	}
	if m.Name.ValueString() != "deploy" || m.CreatedAt.ValueInt64() != 1788076923 {
		t.Errorf("got %+v", m)
	}
	if m.ExpiresAt.ValueInt64() != expires {
		t.Errorf("expires_at = %v", m.ExpiresAt)
	}

	// An absent expiry has to become null, not zero: zero is a valid Unix
	// timestamp and would read as "expired in 1970".
	applyAPIKeyResponse(m, &client.APIKeyResponse{Name: "deploy", Created: 1, Access: nil})
	if !m.ExpiresAt.IsNull() {
		t.Errorf("expires_at = %v, want null", m.ExpiresAt)
	}
}

// Rotation must be driven by the trigger changing, never by anything else — an
// accidental rotation invalidates a live credential.
func TestAPIKeyRotationRequested(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		state, plan types.String
		want        bool
	}{
		"unchanged":    {types.StringValue("v1"), types.StringValue("v1"), false},
		"both unset":   {types.StringNull(), types.StringNull(), false},
		"changed":      {types.StringValue("v1"), types.StringValue("v2"), true},
		"newly set":    {types.StringNull(), types.StringValue("v1"), true},
		"newly unset":  {types.StringValue("v1"), types.StringNull(), true},
		"emptied":      {types.StringValue("v1"), types.StringValue(""), true},
		"empty stable": {types.StringValue(""), types.StringValue(""), false},
	}

	for name, tc := range cases {
		got := apiKeyRotationRequested(
			&apiKeyResourceModel{SecretRotationTrigger: tc.state},
			&apiKeyResourceModel{SecretRotationTrigger: tc.plan},
		)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

// The PUT compares the name in the body against the one in the path, so the
// body must always carry the resource's own name.
func TestBuildAPIKeyRequest(t *testing.T) {
	t.Parallel()

	var diags diag.Diagnostics
	m := &apiKeyResourceModel{
		Name:      types.StringValue("deploy"),
		ExpiresAt: types.Int64Null(),
		Access:    accessSetOf(t, client.APIKeyAccess{Group: "Users", AccessRights: []string{"read"}}),
	}
	req := buildAPIKeyRequest(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if req.Name != "deploy" {
		t.Errorf("name = %q", req.Name)
	}
	// A null expiry must be omitted, not sent as 0; Rauthy range-rejects a past
	// timestamp.
	if req.Exp != nil {
		t.Errorf("exp = %v, want nil", *req.Exp)
	}
	if len(req.Access) != 1 || req.Access[0].Group != "Users" {
		t.Errorf("access = %+v", req.Access)
	}
}
