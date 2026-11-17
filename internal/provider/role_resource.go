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
	_ resource.Resource                = (*roleResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleResource)(nil)
	_ resource.ResourceWithImportState = (*roleResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*roleResource)(nil)
)

// NewRoleResource returns the rauthy_role resource.
func NewRoleResource() resource.Resource { return &roleResource{} }

type roleResource struct {
	api *rauthy.Client
}

type roleResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a role in Rauthy. Roles are assigned to users and can be requested by " +
			"clients through the `roles` scope.\n\n" +
			"Renaming a role replaces it wholesale in Rauthy, which drops any `meta` payload another tool " +
			"may have attached to it. This provider does not manage `meta`, so it should own the roles it " +
			"declares.\n\n" +
			"If a conflicting name is created in the same apply or a planned name is unknown, refusal may occur " +
			"during apply after other resources have been written, but the refused operation does not corrupt user assignments.\n\n" +
			"Requires these API key rights: `Roles` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the role.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the role, for example `admin`. Changing it is refused: Rauthy's rename drops the role from its users on PostgreSQL and " +
					"rewrites other role names in user assignments on Hiqlite. Create the new role, move the users, " +
					"then remove the old one. Deleting is refused while another role name starts or ends with this one.",
				Validators: []validator.String{validators.RoleName()},
			},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState takes the role id. The id is not the name: Rauthy assigns it, and
// `terraform import` needs the value shown in the Admin UI or returned by the
// API.
func (r *roleResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateRole(ctx, rauthy.RoleRequest{Role: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy role "+plan.Name.ValueString(), err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetRole(ctx, id)
	if rauthy.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy role "+id, err.Error())
		return
	}

	state.ID = types.StringValue(got.ID)
	state.Name = types.StringValue(got.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: ModifyPlan refuses a change of name, the only
// configurable attribute.
func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ModifyPlan refuses a rename: Rauthy would drop the role from its users on
// PostgreSQL and rewrite other role names inside user assignments on Hiqlite.
func (r *roleResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	var state, plan roleResourceModel
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
	current, err := r.api.GetRole(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy role before checking its rename", err.Error())
		return
	}
	if plan.Name.ValueString() == current.Name {
		return
	}
	resp.Diagnostics.AddAttributeError(path.Root("name"), "Role rename is not supported",
		"Rauthy's rename drops the role from its users on PostgreSQL and rewrites other role names in user "+
			"assignments on Hiqlite. Create the new role as a separate resource, move the users, then remove "+
			"the old one.")
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	r.checkDeletion(ctx, id, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteRole(ctx, id); err != nil && !rauthy.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy role "+id, err.Error())
	}
}

func (r *roleResource) checkDeletion(ctx context.Context, id string, diags *diag.Diagnostics) {
	all, err := r.api.ListRoles(ctx)
	if err != nil {
		diags.AddError("Could not list Rauthy roles to check the deletion of "+id, err.Error())
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
		advice := fmt.Sprintf("Delete %q in a separate apply first, then retry.", other)
		if other == "rauthy_admin" {
			advice = "The built-in rauthy_admin role cannot be deleted. Keep this role managed without deleting it."
		}
		diags.AddError("Unsafe role deletion",
			fmt.Sprintf("Rauthy removes role %q from users by substring, so it would also cut %q in "+
				"their assignments. %s",
				name, other, advice))
		return
	}
}
