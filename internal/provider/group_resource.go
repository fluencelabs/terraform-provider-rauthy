package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ resource.Resource                = (*groupResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupResource)(nil)
	_ resource.ResourceWithImportState = (*groupResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*groupResource)(nil)
)

// NewGroupResource returns the rauthy_group resource.
func NewGroupResource() resource.Resource { return &groupResource{} }

type groupResource struct {
	api *rauthy.Client
}

type groupResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a group in Rauthy. Groups are assigned to users and can be requested by " +
			"clients through the `groups` scope.\n\n" +
			"Renaming a group replaces it wholesale in Rauthy, which drops any `meta` payload another tool " +
			"may have attached to it. This provider does not manage `meta`, so it should own the groups it " +
			"declares.\n\n" +
			"If a conflicting name is created in the same apply or a planned name is unknown, refusal may occur " +
			"during apply after other resources have been written, but the refused operation does not corrupt user assignments.\n\n" +
			"Requires these API key rights: `Groups` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the group, for example `developers`. Changing it is refused: Rauthy's rename drops the group from its users on PostgreSQL and " +
					"rewrites other group names in user assignments on Hiqlite. Create the new group, move the users, " +
					"then remove the old one. Deleting is refused while another group name starts or ends with this one.",
				Validators: []validator.String{validators.GroupName()},
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState takes the group id. The id is not the name: Rauthy assigns it, and
// `terraform import` needs the value shown in the Admin UI or returned by the
// API.
func (r *groupResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateGroup(ctx, rauthy.GroupRequest{Group: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy group "+plan.Name.ValueString(), err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetGroup(ctx, id)
	if rauthy.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy group "+id, err.Error())
		return
	}

	state.ID = types.StringValue(got.ID)
	state.Name = types.StringValue(got.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: ModifyPlan refuses a change of name, the only
// configurable attribute.
func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ModifyPlan refuses a rename: Rauthy would drop the group from its users on
// PostgreSQL and rewrite other group names inside user assignments on Hiqlite.
func (r *groupResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	var state, plan groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if req.Plan.Raw.IsNull() {
		r.checkDeletion(ctx, state.ID.ValueString(), &resp.Diagnostics)
		return
	}
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Name.IsUnknown() {
		return
	}
	current, err := r.api.GetGroup(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy group before checking its rename", err.Error())
		return
	}
	if plan.Name.ValueString() == current.Name {
		return
	}
	resp.Diagnostics.AddAttributeError(path.Root("name"), "Group rename is not supported",
		"Rauthy's rename drops the group from its users on PostgreSQL and rewrites other group names in user "+
			"assignments on Hiqlite. Create the new group as a separate resource, move the users, then remove "+
			"the old one.")
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	r.checkDeletion(ctx, id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteGroup(ctx, id); err != nil && !rauthy.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy group "+id, err.Error())
	}
}

func (r *groupResource) checkDeletion(ctx context.Context, id string, diags *diag.Diagnostics) {
	all, err := r.api.ListGroups(ctx)
	if err != nil {
		diags.AddError("Could not list Rauthy groups to check the deletion of "+id, err.Error())
		return
	}
	name := ""
	for _, x := range all {
		if x.ID == id {
			name = x.Name
			break
		}
	}
	if name == "" {
		return
	}
	names := make([]string, 0, len(all))
	for _, x := range all {
		names = append(names, x.Name)
	}
	if other, ok := cutName(name, names); ok {
		diags.AddError("Unsafe group deletion",
			fmt.Sprintf("Rauthy removes group %q from users by substring, so it would also cut %q in "+
				"their assignments. Delete %q in a separate apply first, then retry.",
				name, other, other))
		return
	}
}
