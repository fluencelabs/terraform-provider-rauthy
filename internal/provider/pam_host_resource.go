package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*pamHostResource)(nil)
	_ resource.ResourceWithConfigure   = (*pamHostResource)(nil)
	_ resource.ResourceWithImportState = (*pamHostResource)(nil)
)

// NewPamHostResource returns the rauthy_pam_host resource.
func NewPamHostResource() resource.Resource { return &pamHostResource{} }

type pamHostResource struct {
	api *client.Client
}

type pamHostResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Hostname          types.String `tfsdk:"hostname"`
	GID               types.Int64  `tfsdk:"gid"`
	ForceMfa          types.Bool   `tfsdk:"force_mfa"`
	LocalPasswordOnly types.Bool   `tfsdk:"local_password_only"`
	IPs               types.Set    `tfsdk:"ips"`
	Aliases           types.Set    `tfsdk:"aliases"`
	Notes             types.String `tfsdk:"notes"`
	Secret            types.String `tfsdk:"secret"`
}

func (r *pamHostResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_pam_host"
}

func (r *pamHostResource) Configure(
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

// ImportState takes the host id. Importing is worth mentioning for this
// resource in particular: a host that registered itself against Rauthy is
// adopted this way, and everything about it is then managed as usual.
func (r *pamHostResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Create posts the host and then, when the configuration sets anything the
// create body has no room for, follows up with a PUT. POST /pam/hosts accepts
// only the hostname, the gid and the two booleans; addresses, aliases and notes
// are update-only fields.
func (r *pamHostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pamHostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreatePamHost(ctx, client.PamHostCreateRequest{
		Hostname:          plan.Hostname.ValueString(),
		GID:               plan.GID.ValueInt64(),
		ForceMfa:          plan.ForceMfa.ValueBool(),
		LocalPasswordOnly: plan.LocalPasswordOnly.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy PAM host "+plan.Hostname.ValueString(), err.Error())
		return
	}

	if isSet(plan.IPs) || isSet(plan.Aliases) || isSet(plan.Notes) {
		r.applyPlan(ctx, created.ID, &plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	r.refresh(ctx, created.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pamHostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pamHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if _, err := r.api.GetPamHost(ctx, id); client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}

	r.refresh(ctx, id, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *pamHostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pamHostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()
	r.applyPlan(ctx, id, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, id, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pamHostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pamHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if err := r.api.DeletePamHost(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy PAM host "+id, err.Error())
	}
}

// applyPlan issues the full-replacement PUT.
func (r *pamHostResource) applyPlan(
	ctx context.Context,
	id string,
	plan *pamHostResourceModel,
	diags *diag.Diagnostics,
) {
	current, err := r.api.GetPamHost(ctx, id)
	if err != nil {
		diags.AddError("Could not read Rauthy PAM host "+id, err.Error())
		return
	}

	body, d := pamHostUpdateBody(ctx, plan, current)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	if updateErr := r.api.UpdatePamHost(ctx, id, body); updateErr != nil {
		diags.AddError("Could not update Rauthy PAM host "+id, updateErr.Error())
	}
}

// refresh reloads the host and its secret. The secret lives behind its own
// endpoint, so a refresh is two calls; both need only the read right.
func (r *pamHostResource) refresh(
	ctx context.Context,
	id string,
	model *pamHostResourceModel,
	diags *diag.Diagnostics,
) {
	got, err := r.api.GetPamHost(ctx, id)
	if err != nil {
		diags.AddError("Could not read Rauthy PAM host "+id, err.Error())
		return
	}
	secret, err := r.api.GetPamHostSecret(ctx, id)
	if err != nil {
		diags.AddError("Could not read the secret of Rauthy PAM host "+id, err.Error())
		return
	}
	diags.Append(applyPamHostDetails(ctx, model, got, secret.Secret)...)
}

// applyPamHostDetails overwrites the model with the server's view.
func applyPamHostDetails(
	ctx context.Context,
	model *pamHostResourceModel,
	got *client.PamHostDetailsResponse,
	secret string,
) diag.Diagnostics {
	ips, diags := types.SetValueFrom(ctx, types.StringType, got.IPs)
	if diags.HasError() {
		return diags
	}
	aliases, d := types.SetValueFrom(ctx, types.StringType, got.Aliases)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	model.ID = types.StringValue(got.ID)
	model.Hostname = types.StringValue(got.Hostname)
	model.GID = types.Int64Value(got.GID)
	model.ForceMfa = types.BoolValue(got.ForceMfa)
	model.LocalPasswordOnly = types.BoolValue(got.LocalPasswordOnly)
	model.IPs = ips
	model.Aliases = aliases
	model.Notes = types.StringPointerValue(got.Notes)
	model.Secret = types.StringValue(secret)
	return diags
}

// pamHostUpdateBody builds the PUT body. `ips` and `aliases` are required
// fields of it, so an attribute the plan leaves unknown — which happens on the
// first apply, before either has ever been computed — is filled from the host's
// current value rather than sent as null.
func pamHostUpdateBody(
	ctx context.Context,
	plan *pamHostResourceModel,
	current *client.PamHostDetailsResponse,
) (client.PamHostUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	ips, d := pamHostStringSet(ctx, plan.IPs, current.IPs)
	diags.Append(d...)
	aliases, d := pamHostStringSet(ctx, plan.Aliases, current.Aliases)
	diags.Append(d...)

	req := client.PamHostUpdateRequest{
		Hostname:          plan.Hostname.ValueString(),
		GID:               plan.GID.ValueInt64(),
		ForceMfa:          plan.ForceMfa.ValueBool(),
		LocalPasswordOnly: plan.LocalPasswordOnly.ValueBool(),
		IPs:               ips,
		Aliases:           aliases,
		Notes:             plan.Notes.ValueStringPointer(),
	}
	return req, diags
}

// pamHostStringSet resolves one of the two string sets, sorting the result so
// the request body does not depend on the iteration order of a Terraform set.
func pamHostStringSet(ctx context.Context, planned types.Set, fallback []string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	out := make([]string, 0, len(fallback))
	if !isSet(planned) {
		out = append(out, fallback...)
	} else {
		diags = planned.ElementsAs(ctx, &out, false)
		if diags.HasError() {
			return nil, diags
		}
	}
	sort.Strings(out)
	return out, diags
}
