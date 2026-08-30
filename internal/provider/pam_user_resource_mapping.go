package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// pamUserGroupModel is one row of the `groups` set.
type pamUserGroupModel struct {
	GID   types.Int64 `tfsdk:"gid"`
	Wheel types.Bool  `tfsdk:"wheel"`
}

// pamUserGroupsToSet turns the membership rows Rauthy returned into the set the
// schema declares, dropping the uid each row carries — it is always the user's
// own id, so the configuration has no use for it.
func pamUserGroupsToSet(ctx context.Context, links []client.PamGroupUserLink) (types.Set, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: pamUserGroupAttrTypes}

	rows := make([]pamUserGroupModel, 0, len(links))
	for _, l := range links {
		rows = append(rows, pamUserGroupModel{
			GID:   types.Int64Value(l.GID),
			Wheel: types.BoolValue(l.Wheel),
		})
	}
	return types.SetValueFrom(ctx, elemType, rows)
}

// pamUserGroupsFromSet turns the configured set back into wire rows, stamping
// every one with the uid the server insists on even though the path already
// carries it.
//
// The result is sorted by gid so that the request body is stable: a Terraform
// set has no order, and an unstable body would make the contract tests and any
// request-level assertion flaky for no reason.
func pamUserGroupsFromSet(
	ctx context.Context,
	uid int64,
	set types.Set,
) ([]client.PamGroupUserLink, diag.Diagnostics) {
	var rows []pamUserGroupModel
	diags := set.ElementsAs(ctx, &rows, false)
	if diags.HasError() {
		return nil, diags
	}

	// A non-nil empty slice, not nil: `groups` is a required field of the PUT
	// body, and a nil slice would marshal to `null` and be rejected. An empty
	// list is a legitimate request — it clears every membership.
	links := make([]client.PamGroupUserLink, 0, len(rows))
	for _, row := range rows {
		links = append(links, client.PamGroupUserLink{
			UID:   uid,
			GID:   row.GID.ValueInt64(),
			Wheel: row.Wheel.ValueBool(),
		})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].GID < links[j].GID })
	return links, diags
}

// applyPamUserDetails overwrites every attribute of the model with what the
// server reports, which is the only source of truth after any write.
func applyPamUserDetails(
	ctx context.Context,
	model *pamUserResourceModel,
	got *client.PamUserDetailsResponse,
) diag.Diagnostics {
	groups, diags := pamUserGroupsToSet(ctx, got.Groups)
	if diags.HasError() {
		return diags
	}

	model.ID = types.Int64Value(got.ID)
	model.Username = types.StringValue(got.Name)
	model.Email = types.StringValue(got.Email)
	model.GID = types.Int64Value(got.GID)
	model.Shell = types.StringValue(got.Shell)
	model.HomeDir = types.StringValue(got.HomeDir)
	model.Groups = groups
	return diags
}

// pamUserUpdateBody builds the PUT body, falling back to the server's current
// values wherever the plan leaves an attribute unset. The fallback matters
// because the PUT is a full replacement: an omitted shell is not "leave it
// alone", it is a deserialization error, and an omitted group list would wipe
// the memberships.
func pamUserUpdateBody(
	ctx context.Context,
	plan *pamUserResourceModel,
	current *client.PamUserDetailsResponse,
) (client.PamUserUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	req := client.PamUserUpdateRequest{Shell: current.Shell}
	if !plan.Shell.IsNull() && !plan.Shell.IsUnknown() {
		req.Shell = plan.Shell.ValueString()
	}

	homeDir := current.HomeDir
	if !plan.HomeDir.IsNull() && !plan.HomeDir.IsUnknown() {
		homeDir = plan.HomeDir.ValueString()
	}
	req.HomeDir = &homeDir

	if plan.Groups.IsNull() || plan.Groups.IsUnknown() {
		links := make([]client.PamGroupUserLink, 0, len(current.Groups))
		links = append(links, current.Groups...)
		sort.Slice(links, func(i, j int) bool { return links[i].GID < links[j].GID })
		req.Groups = links
		return req, diags
	}

	links, d := pamUserGroupsFromSet(ctx, current.ID, plan.Groups)
	diags.Append(d...)
	req.Groups = links
	return req, diags
}

// pamUserNeedsUpdate reports whether the plan asks for anything the create
// endpoint could not express. POST /pam/users takes only a username and an
// email, so shell, home directory and memberships are always a second call —
// but only when the configuration actually sets one of them.
func pamUserNeedsUpdate(plan *pamUserResourceModel) bool {
	return isSet(plan.Shell) || isSet(plan.HomeDir) || isSet(plan.Groups)
}

func isSet(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }
