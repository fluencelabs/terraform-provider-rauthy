package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*apiKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*apiKeyResource)(nil)
	_ resource.ResourceWithImportState = (*apiKeyResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*apiKeyResource)(nil)
)

// NewAPIKeyResource returns the rauthy_api_key resource.
func NewAPIKeyResource() resource.Resource { return &apiKeyResource{} }

type apiKeyResource struct {
	api *client.Client
}

// apiKeyAccessModel is one `{group, access_rights}` grant. It lives inside a
// set, not a list, because Rauthy stores the grants in whatever order they
// arrived and makes no promise to keep it; comparing them as a set means a
// reordering upstream is not a permanent diff.
type apiKeyAccessModel struct {
	Group        types.String `tfsdk:"group"`
	AccessRights types.Set    `tfsdk:"access_rights"`
}

type apiKeyResourceModel struct {
	Name      types.String `tfsdk:"name"`
	Access    types.Set    `tfsdk:"access"`
	ExpiresAt types.Int64  `tfsdk:"expires_at"`
	CreatedAt types.Int64  `tfsdk:"created_at"`

	Secret                types.String `tfsdk:"secret"`
	SecretRotationTrigger types.String `tfsdk:"secret_rotation_trigger"`
}

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState takes the key's name, which is its identity in Rauthy.
//
// The secret cannot come back: Rauthy discloses it once, at creation, and has
// no endpoint that returns it afterwards. An imported key therefore has a null
// `secret` until it is rotated, and `terraform import` must be run with
// `secret` excluded from ImportStateVerify.
func (r *apiKeyResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := buildAPIKeyRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.api.CreateAPIKey(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy API key", err.Error())
		return
	}
	plan.Secret = types.StringValue(secret)

	// POST answers with the credential and nothing else, so created_at and the
	// stored form of the grants have to be read back separately. If that read
	// fails the key still exists and its secret is in hand — losing it here
	// would strand an unmanaged credential — so state is written regardless.
	got, err := r.api.GetAPIKey(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Rauthy API key created but could not be read back",
			fmt.Sprintf(
				"The key %q was created and its secret written to state, but reading it back failed, so "+
					"created_at is not populated. Re-run apply to refresh it.\n\nError: %s",
				plan.Name.ValueString(), err,
			),
		)
		return
	}

	applyAPIKeyResponse(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	got, err := r.api.GetAPIKey(ctx, name)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy API key "+name, err.Error())
		return
	}

	// Deliberately not touched: Secret and SecretRotationTrigger. The secret is
	// unreadable after creation, and the trigger exists only in Terraform, so a
	// refresh that rewrote either would destroy information rather than correct
	// it.
	applyAPIKeyResponse(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state apiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()

	body := buildAPIKeyRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.UpdateAPIKey(ctx, name, body); err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy API key "+name, err.Error())
		return
	}

	// Carry the old secret forward by default: an update does not touch it, and
	// there is no way to re-read it.
	plan.Secret = state.Secret

	// Rotation is never implicit: it happens only when the trigger value
	// actually changes.
	if apiKeyRotationRequested(&state, &plan) {
		rotated, err := r.api.RotateAPIKeySecret(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError("Could not rotate the secret of Rauthy API key "+name, err.Error())
			return
		}
		plan.Secret = types.StringValue(rotated)
	}

	got, err := r.api.GetAPIKey(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy API key "+name+" back after updating it", err.Error())
		return
	}
	applyAPIKeyResponse(&plan, got)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	err := r.api.DeleteAPIKey(ctx, name)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy API key "+name, err.Error())
	}
}

// ModifyPlan marks the secret unknown when this apply is going to replace it,
// so the plan does not promise the value currently in state.
func (r *apiKeyResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		// Create or destroy; nothing to compare against.
		return
	}

	var plan, state apiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if apiKeyRotationRequested(&state, &plan) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("secret"), types.StringUnknown())...)
	}
}

// apiKeyRotationRequested reports whether secret_rotation_trigger changed
// between state and plan. A trigger that is newly set, or newly removed,
// counts.
func apiKeyRotationRequested(state, plan *apiKeyResourceModel) bool {
	return !plan.SecretRotationTrigger.Equal(state.SecretRotationTrigger)
}
