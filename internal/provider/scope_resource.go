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

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ resource.Resource                = (*scopeResource)(nil)
	_ resource.ResourceWithConfigure   = (*scopeResource)(nil)
	_ resource.ResourceWithImportState = (*scopeResource)(nil)
)

// NewScopeResource returns the rauthy_scope resource.
func NewScopeResource() resource.Resource { return &scopeResource{} }

type scopeResource struct {
	api *client.Client
}

type scopeResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	AttrIncludeAccess types.Set    `tfsdk:"attr_include_access"`
	AttrIncludeID     types.Set    `tfsdk:"attr_include_id"`
}

func (r *scopeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scope"
}

func (r *scopeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom OIDC scope in Rauthy. A scope created here can be referenced " +
			"by name from `rauthy_client.scopes` and `rauthy_client.default_scopes`.\n\n" +
			"The two `attr_include_*` sets map custom user attributes into the issued tokens. Rauthy stores " +
			"them as a single comma-joined string, so an attribute name containing a comma is not " +
			"representable.\n\n" +
			"Requires these API key rights: `Scopes` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rauthy-assigned identifier of the scope.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the scope, for example `read:billing`. This is the value clients " +
					"request. Renaming in place is supported.",
				Validators: []validator.String{validators.ScopeName()},
			},
			"attr_include_access": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "User attributes to include in the **access token** when this scope is granted.",
				Validators:          []validator.Set{validators.UserAttrNameSet()},
			},
			"attr_include_id": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "User attributes to include in the **ID token** when this scope is granted.",
				Validators:          []validator.Set{validators.UserAttrNameSet()},
			},
		},
	}
}

func (r *scopeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *client.Client, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	r.api = api
}

// ImportState takes the scope id, which Rauthy assigns; it is not the name.
func (r *scopeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *scopeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scopeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateScope(ctx, buildScopeRequest(ctx, &plan, &resp.Diagnostics))
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy scope "+plan.Name.ValueString(), err.Error())
		return
	}

	applyScope(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scopeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scopeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetScope(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy scope "+id, err.Error())
		return
	}

	applyScope(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *scopeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan scopeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()
	updated, err := r.api.UpdateScope(ctx, id, buildScopeRequest(ctx, &plan, &resp.Diagnostics))
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy scope "+id, err.Error())
		return
	}

	applyScope(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scopeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scopeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if err := r.api.DeleteScope(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy scope "+id, err.Error())
	}
}

func buildScopeRequest(ctx context.Context, m *scopeResourceModel, diags *diag.Diagnostics) client.ScopeRequest {
	return client.ScopeRequest{
		Scope:             m.Name.ValueString(),
		AttrIncludeAccess: setToStrings(ctx, m.AttrIncludeAccess, diags),
		AttrIncludeID:     setToStrings(ctx, m.AttrIncludeID, diags),
	}
}

// applyScope folds a response into the model. The attribute mappings arrive as
// one comma-joined string and are split back into sets here; order is not
// preserved by Rauthy, which is why these are sets rather than lists.
func applyScope(m *scopeResourceModel, s *client.ScopeResponse) {
	m.ID = types.StringValue(s.ID)
	m.Name = types.StringValue(s.Name)
	m.AttrIncludeAccess = stringsToSet(s.AttrIncludeAccessList())
	m.AttrIncludeID = stringsToSet(s.AttrIncludeIDList())
}
