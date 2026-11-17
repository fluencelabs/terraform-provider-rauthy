package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ resource.Resource                = (*pamGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*pamGroupResource)(nil)
	_ resource.ResourceWithImportState = (*pamGroupResource)(nil)
)

// NewPamGroupResource returns the rauthy_pam_group resource.
func NewPamGroupResource() resource.Resource { return &pamGroupResource{} }

type pamGroupResource struct {
	api *rauthy.Client
}

type pamGroupResourceModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Typ  types.String `tfsdk:"typ"`
}

func (r *pamGroupResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_pam_group"
}

// Schema declares both attributes as RequiresReplace. That is not caution:
// Rauthy exposes no PUT for a PAM group at all, so a change to either the name
// or the type can only be a destroy-and-create.
func (r *pamGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a POSIX group in Rauthy's PAM subsystem, the part of Rauthy that acts as " +
			"an authentication source for hosts and SSH logins.\n\n" +
			"A PAM group is a real group in the host's namespace: its `id` is the numeric gid handed to " +
			"`rauthy_pam_host.gid` and to a `rauthy_pam_user` membership.\n\n" +
			"Rauthy offers no update endpoint for a PAM group, so changing either the name or the type " +
			"replaces the group, which means a new gid. Rauthy refuses to delete a group a host still " +
			"points at, and Terraform deletes before it creates, so a host group in use needs " +
			"`lifecycle { create_before_destroy = true }`: Terraform then creates the new group, moves the " +
			"hosts that refer to `rauthy_pam_group.<name>.id`, and deletes the old one. Group names are " +
			"unique, so with create_before_destroy a change of `typ` alone fails on the name: change the " +
			"name in the same apply. User memberships do not block a deletion: Rauthy removes them with " +
			"the group, and Terraform adds the new gid back to users that refer to it.\n\n" +
			"Requires these API key rights: `Pam` read, create, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Numeric gid Rauthy assigned to the group.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the group, for example `developers`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"typ": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Group type. `generic` is the ordinary supplementary group and the one " +
					"to reach for; `host` is what a `rauthy_pam_host` points its `gid` at; `local` mirrors a " +
					"group that exists only on the machine. `user` and `immutable` are Rauthy's own — it " +
					"creates a `user` group for every PAM user and marks its built-ins `immutable` — and " +
					"although a live server accepts both here, declaring one is asking for trouble.",
				Validators: []validator.String{validators.PamGroupType()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *pamGroupResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*rauthy.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *rauthy.Client, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	r.api = api
}

// ImportState takes the numeric gid, the value Rauthy assigned and shows in the
// Admin UI.
func (r *pamGroupResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	gid, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid PAM group import id",
			fmt.Sprintf("Expected the numeric gid of the group, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), gid)...)
}

func (r *pamGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pamGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreatePamGroup(ctx, rauthy.PamGroupCreateRequest{
		Name: plan.Name.ValueString(),
		Typ:  plan.Typ.ValueString(),
	})
	if err != nil {
		detail := err.Error()
		// Rauthy inserts with ON CONFLICT (name) DO NOTHING, so a taken name
		// comes back as a 404 "no rows returned".
		if apiErr, ok := errors.AsType[*rauthy.APIError](err); ok &&
			apiErr.StatusCode == http.StatusNotFound && apiErr.ErrorCode == "NotFound" &&
			(apiErr.Message == "no rows returned" || apiErr.Message == "No results found") {
			detail += "\n\nA PAM group with this name already exists. Group names are unique: when replacing a " +
				"group with create_before_destroy, change its name in the same apply."
		}
		resp.Diagnostics.AddError("Could not create Rauthy PAM group "+plan.Name.ValueString(), detail)
		return
	}

	plan.ID = types.Int64Value(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Typ = types.StringValue(created.Typ)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pamGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pamGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gid := state.ID.ValueInt64()
	got, err := r.api.GetPamGroup(ctx, gid)
	if rauthy.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy PAM group "+strconv.FormatInt(gid, 10), err.Error())
		return
	}

	state.ID = types.Int64Value(got.ID)
	state.Name = types.StringValue(got.Name)
	state.Typ = types.StringValue(got.Typ)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update cannot happen — every attribute requires replacement — but the
// interface demands the method, so it fails loudly rather than silently
// pretending to have written something.
func (r *pamGroupResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"PAM groups cannot be updated in place",
		"Rauthy exposes no update endpoint for a PAM group. Reaching this code means a schema change "+
			"dropped a RequiresReplace plan modifier. This is a bug in the provider.",
	)
}

func (r *pamGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pamGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gid := state.ID.ValueInt64()
	if err := r.api.DeletePamGroup(ctx, gid); err != nil && !rauthy.IsNotFound(err) {
		detail := err.Error()
		if apiErr, ok := errors.AsType[*rauthy.APIError](err); ok {
			switch {
			case apiErr.StatusCode == http.StatusBadRequest && apiErr.ErrorCode == "BadRequest" &&
				(apiErr.Message == "Cannot delete group with assigned hosts" || apiErr.Message == "Error code 787"):
				detail += "\n\nRauthy refuses to delete a group a host still points at. Move the host to another " +
					"group first, or, when the group is being replaced, set lifecycle { create_before_destroy = true } on it."
			case apiErr.StatusCode == http.StatusInternalServerError && apiErr.ErrorCode == "Database" &&
				apiErr.Message == "db error":
				// Rauthy's Postgres conversion exposes only tokio-postgres's
				// Display text, which hides the SQLSTATE and constraint name.
				detail += "\n\nIf a host still points at this group, move it to another group first, or, when the group is " +
					"being replaced, set lifecycle { create_before_destroy = true } on it. Rauthy's Postgres response " +
					"does not identify the database error; check the Rauthy logs to confirm the cause."
			}
		}
		resp.Diagnostics.AddError("Could not delete Rauthy PAM group "+strconv.FormatInt(gid, 10), detail)
	}
}
