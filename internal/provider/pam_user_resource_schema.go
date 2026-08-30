package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pamUserGroupAttrTypes is the object shape of one membership row in the
// `groups` set. The uid is absent on purpose: it is always this user's own id,
// and carrying it in the configuration would be a field that can only ever hold
// one correct value.
//
//nolint:gochecknoglobals // an attribute type table, read-only
var pamUserGroupAttrTypes = map[string]attr.Type{
	"gid":   types.Int64Type,
	"wheel": types.BoolType,
}

func (r *pamUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a POSIX account in Rauthy's PAM subsystem, the part of Rauthy that acts " +
			"as an authentication source for hosts and SSH logins.\n\n" +
			"This resource does not create an identity — it attaches POSIX attributes to a Rauthy user that " +
			"already exists, matched by email. The email must belong to an existing `rauthy_user` that no " +
			"other PAM user has claimed yet, or Rauthy answers with a bare `404 no rows returned`.\n\n" +
			"SSH keys are deliberately not managed here. Rauthy only lets the account holder add them, " +
			"through `/pam/users/self/authorized_keys`; an API key cannot, so they are runtime state.\n\n" +
			"Requires these API key rights: `Pam` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Numeric uid Rauthy assigned to the account.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "POSIX login name. Rauthy offers no way to change it afterwards, so a " +
					"different name replaces the account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Email of the existing Rauthy user this account belongs to. As with the " +
					"username, it cannot be changed in place.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gid": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "Numeric gid of the personal group Rauthy creates alongside the account. " +
					"It is not choosable, and it disappears with the account.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"shell": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Login shell. Rauthy defaults a new account to `/bin/bash` and does not " +
					"check that the value is a path, let alone that it exists on any host.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"home_dir": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Home directory. Rauthy defaults a new account to `/home/<username>`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"groups": schema.SetNestedAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Supplementary group memberships, as the complete set. Rauthy replaces " +
					"the whole membership list on every write, and it makes no exception for the personal " +
					"group in `gid` — declaring `groups = []` really does leave the account in no group at " +
					"all. Leave the attribute out to keep whatever Rauthy set up at creation.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"gid": schema.Int64Attribute{
							Required: true,
							MarkdownDescription: "Numeric gid of the group, normally " +
								"`rauthy_pam_group.<name>.id`.",
						},
						"wheel": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							MarkdownDescription: "Whether membership in this group grants sudo on the hosts " +
								"that use it.",
						},
					},
				},
			},
		},
	}
}
