package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
	"github.com/fluencelabs/terraform-provider-rauthy/rauthy"
)

var (
	_ resource.Resource                = (*groupResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupResource)(nil)
	_ resource.ResourceWithImportState = (*groupResource)(nil)
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
				Required:            true,
				MarkdownDescription: "Name of the group, for example `developers`. Renaming in place is supported.",
				Validators:          []validator.String{validators.GroupName()},
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

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()
	updated, err := r.api.UpdateGroup(ctx, id, rauthy.GroupRequest{Group: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy group "+id, err.Error())
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if err := r.api.DeleteGroup(ctx, id); err != nil && !rauthy.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy group "+id, err.Error())
	}
}
