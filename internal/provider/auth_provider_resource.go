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
	_ resource.Resource                = (*authProviderResource)(nil)
	_ resource.ResourceWithConfigure   = (*authProviderResource)(nil)
	_ resource.ResourceWithImportState = (*authProviderResource)(nil)
)

// NewAuthProviderResource returns the rauthy_auth_provider resource.
func NewAuthProviderResource() resource.Resource { return &authProviderResource{} }

type authProviderResource struct {
	api *client.Client
}

type authProviderResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Type                  types.String `tfsdk:"type"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	Issuer                types.String `tfsdk:"issuer"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	UserinfoEndpoint      types.String `tfsdk:"userinfo_endpoint"`
	JwksEndpoint          types.String `tfsdk:"jwks_endpoint"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecretWO        types.String `tfsdk:"client_secret_wo"`
	ClientSecretTrigger   types.String `tfsdk:"client_secret_rotation_trigger"`
	Scopes                types.Set    `tfsdk:"scopes"`
	AdminClaimPath        types.String `tfsdk:"admin_claim_path"`
	AdminClaimValue       types.String `tfsdk:"admin_claim_value"`
	MfaClaimPath          types.String `tfsdk:"mfa_claim_path"`
	MfaClaimValue         types.String `tfsdk:"mfa_claim_value"`
	UsePKCE               types.Bool   `tfsdk:"use_pkce"`
	ClientSecretBasic     types.Bool   `tfsdk:"client_secret_basic"`
	ClientSecretPost      types.Bool   `tfsdk:"client_secret_post"`
	AutoOnboarding        types.Bool   `tfsdk:"auto_onboarding"`
	AutoLink              types.Bool   `tfsdk:"auto_link"`
}

func (r *authProviderResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider"
}

func (r *authProviderResource) Configure(
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

func (r *authProviderResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *authProviderResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan authProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret := writeOnlyString(ctx, req.Config, path.Root("client_secret_wo"), &resp.Diagnostics)
	body := buildAuthProviderRequest(ctx, &plan, secret, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateAuthProvider(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy auth provider", err.Error())
		return
	}

	applyAuthProviderResponse(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state authProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetAuthProvider(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy auth provider "+id, err.Error())
		return
	}

	applyAuthProviderResponse(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *authProviderResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan authProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PUT is a full replacement and a body without the secret erases the stored
	// one, so the write-only value has to be pulled out of the configuration on
	// every single update, not just the ones that mean to change it.
	secret := writeOnlyString(ctx, req.Config, path.Root("client_secret_wo"), &resp.Diagnostics)

	id := plan.ID.ValueString()
	body := buildAuthProviderRequest(ctx, &plan, secret, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.api.UpdateAuthProvider(ctx, id, body); err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy auth provider "+id, err.Error())
		return
	}

	// PUT answers with an empty body, so the stored form has to be fetched
	// back. That matters here rather than being a formality: Rauthy rewrites
	// `scope` on the way in, and the resource's state must hold what the server
	// actually kept.
	got, err := r.api.GetAuthProvider(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Could not re-read Rauthy auth provider "+id+" after updating it", err.Error())
		return
	}

	applyAuthProviderResponse(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authProviderResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state authProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	err := r.api.DeleteAuthProvider(ctx, id)
	if err == nil || client.IsNotFound(err) {
		return
	}

	// Rauthy refuses to delete a provider that accounts still log in through.
	// Naming those accounts turns an opaque failure into something actionable.
	detail := err.Error()
	if linked, linkErr := r.api.AuthProviderLinkedUsers(ctx, id); linkErr == nil && len(linked) > 0 {
		detail += fmt.Sprintf("\n\n%d user account(s) are still linked to this provider, "+
			"starting with %s. Unlink or delete them first.", len(linked), linked[0].Email)
	}
	resp.Diagnostics.AddError("Could not delete Rauthy auth provider "+id, detail)
}
