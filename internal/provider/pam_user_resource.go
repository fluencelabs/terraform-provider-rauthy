package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*pamUserResource)(nil)
	_ resource.ResourceWithConfigure   = (*pamUserResource)(nil)
	_ resource.ResourceWithImportState = (*pamUserResource)(nil)
)

// NewPamUserResource returns the rauthy_pam_user resource.
func NewPamUserResource() resource.Resource { return &pamUserResource{} }

type pamUserResource struct {
	api *client.Client
}

type pamUserResourceModel struct {
	ID       types.Int64  `tfsdk:"id"`
	Username types.String `tfsdk:"username"`
	Email    types.String `tfsdk:"email"`
	GID      types.Int64  `tfsdk:"gid"`
	Shell    types.String `tfsdk:"shell"`
	HomeDir  types.String `tfsdk:"home_dir"`
	Groups   types.Set    `tfsdk:"groups"`
}

func (r *pamUserResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_pam_user"
}

func (r *pamUserResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
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

// ImportState takes the numeric uid.
func (r *pamUserResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	uid, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid PAM user import id",
			fmt.Sprintf("Expected the numeric uid of the account, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), uid)...)
}

// Create links an existing Rauthy identity to a POSIX account and then, if the
// configuration asks for anything beyond the username and email, issues the PUT
// that the create endpoint has no room for.
func (r *pamUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pamUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := plan.Username.ValueString()
	created, err := r.api.CreatePamUser(ctx, client.PamUserCreateRequest{
		Username: username,
		Email:    plan.Email.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not create Rauthy PAM user "+username,
			err.Error()+"\n\nA 404 here means no Rauthy user has the email "+plan.Email.ValueString()+
				", or one does and another PAM user has already claimed it.",
		)
		return
	}

	if pamUserNeedsUpdate(&plan) {
		r.applyPlan(ctx, created.ID, &plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Read back rather than assembling state from the create response: it
	// carries no memberships, and after the PUT above it would be stale anyway.
	r.refresh(ctx, created.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pamUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pamUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	uid := state.ID.ValueInt64()
	got, err := r.api.GetPamUser(ctx, uid)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy PAM user "+strconv.FormatInt(uid, 10), err.Error())
		return
	}

	resp.Diagnostics.Append(applyPamUserDetails(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *pamUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pamUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	uid := plan.ID.ValueInt64()
	r.applyPlan(ctx, uid, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, uid, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pamUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pamUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	uid := state.ID.ValueInt64()
	if err := r.api.DeletePamUser(ctx, uid); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy PAM user "+strconv.FormatInt(uid, 10), err.Error())
	}
}

// applyPlan issues the PUT, reading the account first so that the fields the
// plan leaves unset are resent as they are instead of being cleared.
func (r *pamUserResource) applyPlan(
	ctx context.Context,
	uid int64,
	plan *pamUserResourceModel,
	diags *diag.Diagnostics,
) {
	current, err := r.api.GetPamUser(ctx, uid)
	if err != nil {
		diags.AddError("Could not read Rauthy PAM user "+strconv.FormatInt(uid, 10), err.Error())
		return
	}

	body, d := pamUserUpdateBody(ctx, plan, current)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	if updateErr := r.api.UpdatePamUser(ctx, uid, body); updateErr != nil {
		diags.AddError("Could not update Rauthy PAM user "+strconv.FormatInt(uid, 10), updateErr.Error())
	}
}

// refresh reloads the account and folds it into the model.
func (r *pamUserResource) refresh(
	ctx context.Context,
	uid int64,
	model *pamUserResourceModel,
	diags *diag.Diagnostics,
) {
	got, err := r.api.GetPamUser(ctx, uid)
	if err != nil {
		diags.AddError("Could not read Rauthy PAM user "+strconv.FormatInt(uid, 10), err.Error())
		return
	}
	diags.Append(applyPamUserDetails(ctx, model, got)...)
}
