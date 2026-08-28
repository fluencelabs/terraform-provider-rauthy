package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*clientResource)(nil)
	_ resource.ResourceWithConfigure   = (*clientResource)(nil)
	_ resource.ResourceWithImportState = (*clientResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*clientResource)(nil)
)

// NewClientResource returns the rauthy_client resource.
func NewClientResource() resource.Resource { return &clientResource{} }

type clientResource struct {
	api *client.Client
}

type scimModel struct {
	BearerToken     types.String `tfsdk:"bearer_token"`
	BaseURI         types.String `tfsdk:"base_uri"`
	SyncGroups      types.Bool   `tfsdk:"sync_groups"`
	GroupSyncPrefix types.String `tfsdk:"group_sync_prefix"`
}

type clientResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Confidential           types.Bool   `tfsdk:"confidential"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	RedirectURIs           types.Set    `tfsdk:"redirect_uris"`
	PostLogoutRedirectURIs types.Set    `tfsdk:"post_logout_redirect_uris"`
	AllowedOrigins         types.Set    `tfsdk:"allowed_origins"`
	FlowsEnabled           types.Set    `tfsdk:"flows_enabled"`
	AccessTokenAlg         types.String `tfsdk:"access_token_alg"`
	IDTokenAlg             types.String `tfsdk:"id_token_alg"`
	AuthCodeLifetime       types.Int64  `tfsdk:"auth_code_lifetime"`
	AccessTokenLifetime    types.Int64  `tfsdk:"access_token_lifetime"`
	Scopes                 types.Set    `tfsdk:"scopes"`
	DefaultScopes          types.Set    `tfsdk:"default_scopes"`
	Challenges             types.Set    `tfsdk:"challenges"`
	ForceMFA               types.Bool   `tfsdk:"force_mfa"`
	ClientURI              types.String `tfsdk:"client_uri"`
	Contacts               types.Set    `tfsdk:"contacts"`
	BackchannelLogoutURI   types.String `tfsdk:"backchannel_logout_uri"`
	RestrictGroupPrefix    types.String `tfsdk:"restrict_group_prefix"`
	Scim                   *scimModel   `tfsdk:"scim"`

	Secret                  types.String `tfsdk:"secret"`
	SecretRotationTrigger   types.String `tfsdk:"secret_rotation_trigger"`
	SecretCacheCurrentHours types.Int64  `tfsdk:"secret_cache_current_hours"`
}

func (r *clientResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client"
}

func (r *clientResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clientResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *clientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clientResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()

	// Creating a fully configured client takes two calls. POST /clients accepts
	// only id, name, confidential and the two URI lists, and silently drops
	// everything else; the rest has to follow in a PUT.
	created, err := r.api.CreateClient(ctx, client.NewClientRequest{
		ID:                     id,
		Name:                   stringPtr(plan.Name),
		Confidential:           plan.Confidential.ValueBool(),
		RedirectURIs:           setToStrings(ctx, plan.RedirectURIs, &resp.Diagnostics),
		PostLogoutRedirectURIs: setToStrings(ctx, plan.PostLogoutRedirectURIs, &resp.Diagnostics),
	})
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy client", err.Error())
		return
	}

	// Attributes the practitioner left unset are unknown in the plan; Rauthy
	// picked defaults for them during the POST, so take those.
	fillUnknownFromResponse(ctx, &plan, created, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	update := buildUpdateRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.api.UpdateClient(ctx, id, update)
	if err != nil {
		// The client exists but carries only what POST accepted. Saving it to
		// state is the lesser evil: dropping it here would leave an object in
		// Rauthy that Terraform no longer knows about.
		applyResponse(ctx, &plan, created, &resp.Diagnostics)
		plan.Secret = types.StringNull()
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Rauthy client created but not configured",
			fmt.Sprintf(
				"The client %q was created, but applying its full configuration failed, so it currently "+
					"holds only the defaults Rauthy assigns on creation. It has been written to state; "+
					"re-run apply to finish configuring it, or destroy it.\n\nError: %s",
				id, err,
			),
		)
		return
	}

	applyResponse(ctx, &plan, updated, &resp.Diagnostics)
	r.readSecretInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clientResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetClient(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy client "+id, err.Error())
		return
	}

	applyResponse(ctx, &state, got, &resp.Diagnostics)
	r.readSecretInto(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clientResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()

	update := buildUpdateRequest(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.api.UpdateClient(ctx, id, update)
	if err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy client "+id, err.Error())
		return
	}
	applyResponse(ctx, &plan, updated, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Rotation is never implicit: it happens only when the trigger value
	// actually changes.
	if rotationRequested(&state, &plan) {
		if !plan.Confidential.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root("secret_rotation_trigger"),
				"Cannot rotate the secret of a public client",
				"secret_rotation_trigger changed, but confidential is false and a public client has no secret. "+
					"Remove secret_rotation_trigger or set confidential = true.",
			)
			return
		}
		rotated, rotateErr := r.api.RotateClientSecret(ctx, id, int64Ptr(plan.SecretCacheCurrentHours))
		if rotateErr != nil {
			resp.Diagnostics.AddError("Could not rotate the secret of Rauthy client "+id, rotateErr.Error())
			return
		}
		plan.Secret = optionalString(rotated.Secret)
	} else {
		r.readSecretInto(ctx, &plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clientResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	err := r.api.DeleteClient(ctx, id)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy client "+id, err.Error())
	}
}

// ModifyPlan marks the secret as unknown whenever this apply is going to change
// it, so the plan does not promise the old value.
func (r *clientResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		// Create or destroy; nothing to compare against.
		return
	}

	var plan, state clientResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A rotation replaces the secret; flipping confidentiality either drops it
	// or brings a fresh one into being.
	if rotationRequested(&state, &plan) || !plan.Confidential.Equal(state.Confidential) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("secret"), types.StringUnknown())...)
	}
}

// rotationRequested reports whether secret_rotation_trigger changed between
// state and plan. A trigger that is newly set, or newly removed, counts.
func rotationRequested(state, plan *clientResourceModel) bool {
	return !plan.SecretRotationTrigger.Equal(state.SecretRotationTrigger)
}

// readSecretInto fills m.Secret from Rauthy, or sets it null for a public
// client, which has none.
func (r *clientResource) readSecretInto(ctx context.Context, m *clientResourceModel, diags *diag.Diagnostics) {
	if !m.Confidential.ValueBool() {
		m.Secret = types.StringNull()
		return
	}

	got, err := r.api.GetClientSecret(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Could not read the secret of Rauthy client "+m.ID.ValueString(), err.Error())
		return
	}
	m.Secret = optionalString(got.Secret)
}

func stringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func int64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func optionalString(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// setToStrings converts a set attribute to a slice. A null or unknown set
// becomes nil, which marshals to an omitted field.
func setToStrings(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	return out
}

// stringsToSet converts a slice back to a set attribute. An absent list becomes
// a null set rather than an empty one, mirroring Rauthy's Option<Vec<_>>.
func stringsToSet(values []string) types.Set {
	if values == nil {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}
