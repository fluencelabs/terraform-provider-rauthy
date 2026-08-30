package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func groupSet(t *testing.T, rows ...pamUserGroupModel) types.Set {
	t.Helper()

	// Copied into a non-nil slice on purpose: SetValueFrom turns a nil slice
	// into a *null* set, which is exactly the case these tests need to keep
	// distinct from an empty one.
	elems := append([]pamUserGroupModel{}, rows...)
	set, diags := types.SetValueFrom(
		context.Background(),
		types.ObjectType{AttrTypes: pamUserGroupAttrTypes},
		elems,
	)
	if diags.HasError() {
		t.Fatalf("build set: %v", diags)
	}
	return set
}

// The uid the server insists on in every membership row is not in the
// configuration, so it has to be stamped in from the account's own id.
func TestPamUserGroupsFromSet_StampsUidAndSorts(t *testing.T) {
	t.Parallel()

	set := groupSet(t,
		pamUserGroupModel{GID: types.Int64Value(100007), Wheel: types.BoolValue(true)},
		pamUserGroupModel{GID: types.Int64Value(100002), Wheel: types.BoolValue(false)},
	)

	links, diags := pamUserGroupsFromSet(context.Background(), 100000, set)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	want := []client.PamGroupUserLink{
		{UID: 100000, GID: 100002, Wheel: false},
		{UID: 100000, GID: 100007, Wheel: true},
	}
	if len(links) != len(want) {
		t.Fatalf("links = %+v", links)
	}
	for i := range want {
		if links[i] != want[i] {
			t.Errorf("links[%d] = %+v, want %+v", i, links[i], want[i])
		}
	}
}

// An empty configured set must survive as an empty slice: `groups` is required
// in the PUT body, and a nil slice would marshal to null and be refused.
func TestPamUserGroupsFromSet_EmptyStaysEmptyNotNil(t *testing.T) {
	t.Parallel()

	links, diags := pamUserGroupsFromSet(context.Background(), 100000, groupSet(t))
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("links = %#v, want an empty non-nil slice", links)
	}
}

// A plan that leaves shell, home_dir and groups unset must resend what the
// server already has: the PUT is a full replacement, so "unset" would otherwise
// mean "cleared".
func TestPamUserUpdateBody_UnsetPlanResendsCurrent(t *testing.T) {
	t.Parallel()

	current := &client.PamUserDetailsResponse{
		ID: 100000, Name: "alice", GID: 100003, Email: "alice@example.com",
		Shell: "/bin/bash", HomeDir: "/home/alice",
		Groups: []client.PamGroupUserLink{
			{UID: 100000, GID: 100007, Wheel: true},
			{UID: 100000, GID: 100003, Wheel: false},
		},
	}
	plan := &pamUserResourceModel{
		Shell:   types.StringNull(),
		HomeDir: types.StringNull(),
		Groups:  types.SetNull(types.ObjectType{AttrTypes: pamUserGroupAttrTypes}),
	}

	body, diags := pamUserUpdateBody(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if body.Shell != "/bin/bash" {
		t.Errorf("shell = %q", body.Shell)
	}
	if body.HomeDir == nil || *body.HomeDir != "/home/alice" {
		t.Errorf("home_dir = %v", body.HomeDir)
	}
	if len(body.Groups) != 2 || body.Groups[0].GID != 100003 {
		t.Errorf("groups = %+v, want the current set sorted by gid", body.Groups)
	}
}

// The distinction that matters for the personal group: an explicitly empty set
// clears every membership rather than falling back to the current one.
func TestPamUserUpdateBody_ExplicitEmptySetClearsMemberships(t *testing.T) {
	t.Parallel()

	current := &client.PamUserDetailsResponse{
		ID: 100000, Shell: "/bin/bash", HomeDir: "/home/alice",
		Groups: []client.PamGroupUserLink{{UID: 100000, GID: 100003}},
	}
	plan := &pamUserResourceModel{
		Shell:   types.StringValue("/bin/zsh"),
		HomeDir: types.StringNull(),
		Groups:  groupSet(t),
	}

	body, diags := pamUserUpdateBody(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if body.Shell != "/bin/zsh" {
		t.Errorf("shell = %q", body.Shell)
	}
	if len(body.Groups) != 0 {
		t.Errorf("groups = %+v, want empty", body.Groups)
	}
}

// pamUserNeedsUpdate decides whether the create needs a follow-up PUT at all.
// Unknown must count as "not set": on the first apply every computed attribute
// is unknown, and treating that as a request would rewrite the account with
// nothing but its own defaults.
func TestPamUserNeedsUpdate(t *testing.T) {
	t.Parallel()

	nullSet := types.SetNull(types.ObjectType{AttrTypes: pamUserGroupAttrTypes})
	unknownSet := types.SetUnknown(types.ObjectType{AttrTypes: pamUserGroupAttrTypes})

	cases := map[string]struct {
		model pamUserResourceModel
		want  bool
	}{
		"nothing set": {
			model: pamUserResourceModel{
				Shell: types.StringNull(), HomeDir: types.StringNull(), Groups: nullSet,
			},
			want: false,
		},
		"all unknown": {
			model: pamUserResourceModel{
				Shell: types.StringUnknown(), HomeDir: types.StringUnknown(), Groups: unknownSet,
			},
			want: false,
		},
		"shell set": {
			model: pamUserResourceModel{
				Shell: types.StringValue("/bin/zsh"), HomeDir: types.StringNull(), Groups: nullSet,
			},
			want: true,
		},
		"groups set": {
			model: pamUserResourceModel{
				Shell: types.StringNull(), HomeDir: types.StringNull(), Groups: groupSet(t),
			},
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := pamUserNeedsUpdate(&tc.model); got != tc.want {
				t.Errorf("pamUserNeedsUpdate = %v, want %v", got, tc.want)
			}
		})
	}
}

// The host PUT has the same full-replacement trap, and the sets it carries are
// unknown on the very first apply.
func TestPamHostUpdateBody_FallsBackToCurrentAndSorts(t *testing.T) {
	t.Parallel()

	current := &client.PamHostDetailsResponse{
		ID: "h1", Hostname: "build01", GID: 100001,
		IPs: []string{"10.0.0.10"}, Aliases: []string{"ci"},
	}
	plan := &pamHostResourceModel{
		Hostname:          types.StringValue("build02"),
		GID:               types.Int64Value(100005),
		ForceMfa:          types.BoolValue(true),
		LocalPasswordOnly: types.BoolValue(false),
		IPs:               types.SetUnknown(types.StringType),
		Aliases:           stringSet(t, "zeta", "alpha"),
		Notes:             types.StringNull(),
	}

	body, diags := pamHostUpdateBody(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if len(body.IPs) != 1 || body.IPs[0] != "10.0.0.10" {
		t.Errorf("ips = %v, want the host's current addresses", body.IPs)
	}
	if len(body.Aliases) != 2 || body.Aliases[0] != "alpha" {
		t.Errorf("aliases = %v, want the planned set sorted", body.Aliases)
	}
	if body.Notes != nil {
		t.Errorf("notes = %v, want nil so the server clears it", body.Notes)
	}
	if body.Hostname != "build02" || body.GID != 100005 {
		t.Errorf("body = %+v", body)
	}
}

func stringSet(t *testing.T, values ...string) types.Set {
	t.Helper()

	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	set, diags := types.SetValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatalf("build set: %v", diags)
	}
	return set
}
