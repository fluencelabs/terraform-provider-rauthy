package provider

import (
	"context"
	"fmt"
	"math"

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
	_ resource.Resource                   = (*passwordPolicyResource)(nil)
	_ resource.ResourceWithConfigure      = (*passwordPolicyResource)(nil)
	_ resource.ResourceWithImportState    = (*passwordPolicyResource)(nil)
	_ resource.ResourceWithValidateConfig = (*passwordPolicyResource)(nil)
)

// NewPasswordPolicyResource returns the rauthy_password_policy resource.
func NewPasswordPolicyResource() resource.Resource { return &passwordPolicyResource{} }

type passwordPolicyResource struct {
	api *client.Client
}

// passwordPolicySingletonID is the value of the synthetic `id` attribute. The
// policy has no identifier of its own — there is only ever one — but Terraform
// tooling (notably `terraform import` and the acceptance-test harness) expects
// every resource to carry a primary identity, so one is supplied.
const passwordPolicySingletonID = "singleton"

type passwordPolicyResourceModel struct {
	ID               types.String `tfsdk:"id"`
	LengthMin        types.Int64  `tfsdk:"length_min"`
	LengthMax        types.Int64  `tfsdk:"length_max"`
	IncludeLowerCase types.Int64  `tfsdk:"include_lower_case"`
	IncludeUpperCase types.Int64  `tfsdk:"include_upper_case"`
	IncludeDigits    types.Int64  `tfsdk:"include_digits"`
	IncludeSpecial   types.Int64  `tfsdk:"include_special"`
	ValidDays        types.Int64  `tfsdk:"valid_days"`
	NotRecentlyUsed  types.Int64  `tfsdk:"not_recently_used"`
}

func (r *passwordPolicyResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_password_policy"
}

func (r *passwordPolicyResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	optional := func(desc string, v validator.Int64) schema.Int64Attribute {
		return schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: desc,
			Validators:          []validator.Int64{v},
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the password policy of a Rauthy instance.\n\n" +
			"The policy is a **singleton**: every Rauthy instance has exactly one and it can be neither " +
			"created nor deleted. Declaring this resource adopts the existing policy and replaces it; " +
			"destroying it only drops it from Terraform state and leaves the policy in place. " +
			"For the same reason, at most one `rauthy_password_policy` should exist per instance — two " +
			"would fight over the same object on every apply.\n\n" +
			"Every optional attribute left unset is sent as null, which **disables** that rule rather than " +
			"leaving it at its previous value: the API replaces the policy wholesale.\n\n" +
			"Requires the `Secrets:update` API key right.\n\n" +
			"**Drift is not detected.** Rauthy v0.35.2 guards `GET /password_policy` with session " +
			"authentication and accepts no API key there, so the provider cannot read the policy back. " +
			"A change made in the Admin UI stays invisible to Terraform until the next apply overwrites " +
			"it, and `terraform import` is unavailable for the same reason.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Always `" + passwordPolicySingletonID + "`. The policy has no " +
					"identifier of its own; this exists because Terraform expects one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"length_min": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Minimum password length (8-128).",
				Validators:          []validator.Int64{validators.PasswordLength()},
			},
			"length_max": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Maximum password length (8-128).",
				Validators:          []validator.Int64{validators.PasswordLength()},
			},
			"include_lower_case": optional(
				"Minimum number of lower-case characters (1-32). Unset disables the rule.",
				validators.PasswordCharClassCount()),
			"include_upper_case": optional(
				"Minimum number of upper-case characters (1-32). Unset disables the rule.",
				validators.PasswordCharClassCount()),
			"include_digits": optional(
				"Minimum number of digits (1-32). Unset disables the rule.",
				validators.PasswordCharClassCount()),
			"include_special": optional(
				"Minimum number of special characters (1-32). Unset disables the rule.",
				validators.PasswordCharClassCount()),
			"valid_days": optional(
				"Days a password stays valid before it must be changed (1-3650). Unset means it never expires.",
				validators.PasswordValidDays()),
			"not_recently_used": optional(
				"How many previous passwords may not be reused (1-10). Unset disables the check.",
				validators.PasswordNotRecentlyUsed()),
		},
	}
}

func (r *passwordPolicyResource) Configure(
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

// ImportState adopts the instance's policy — when Rauthy lets it. Reading the
// policy needs GET /password_policy, which v0.35.2 restricts to session
// authentication, so an API key cannot import. The attempt is still made: a
// future Rauthy that opens the endpoint makes import work with no code change.
func (r *passwordPolicyResource) ImportState(
	ctx context.Context,
	_ resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	got, err := r.api.GetPasswordPolicy(ctx)
	if isSessionOnly(err) {
		resp.Diagnostics.AddError(
			"Rauthy does not allow importing the password policy",
			"Importing requires reading the policy through GET /password_policy, and Rauthy v0.35.2 "+
				"guards that endpoint with session authentication — an API key is rejected. Write the "+
				"desired policy as a rauthy_password_policy resource and apply it instead: the resource "+
				"replaces whatever policy the instance currently has.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Rauthy password policy", err.Error())
		return
	}

	var state passwordPolicyResourceModel
	applyPasswordPolicy(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// isSessionOnly reports whether err is Rauthy refusing an API key on an
// endpoint that only accepts an admin session. GET /password_policy is the one
// such endpoint this provider touches.
func isSessionOnly(err error) bool {
	return client.IsUnauthorized(err) || client.IsForbidden(err)
}

// ValidateConfig catches a length range that cannot be satisfied at plan time,
// naming the attribute, rather than failing halfway through an apply.
func (r *passwordPolicyResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var config passwordPolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Either bound may still be unknown at plan time; there is nothing to
	// compare then.
	if config.LengthMin.IsNull() || config.LengthMin.IsUnknown() ||
		config.LengthMax.IsNull() || config.LengthMax.IsUnknown() {
		return
	}

	if config.LengthMin.ValueInt64() > config.LengthMax.ValueInt64() {
		resp.Diagnostics.AddAttributeError(
			path.Root("length_min"),
			"Invalid password length range",
			fmt.Sprintf("length_min (%d) must not exceed length_max (%d).",
				config.LengthMin.ValueInt64(), config.LengthMax.ValueInt64()),
		)
	}
}

// Create does not create anything: it replaces the policy that already exists.
func (r *passwordPolicyResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan passwordPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.put(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(passwordPolicySingletonID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *passwordPolicyResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state passwordPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Rauthy v0.35.2 accepts only a session on GET /password_policy, so this
	// refresh is expected to be refused. Keeping the prior state is the only
	// option that does not break every plan: the resource then reports no
	// drift rather than failing. The call is still attempted so that a Rauthy
	// which opens the endpoint to API keys starts detecting drift for free.
	got, err := r.api.GetPasswordPolicy(ctx)
	if isSessionOnly(err) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Rauthy password policy", err.Error())
		return
	}

	applyPasswordPolicy(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *passwordPolicyResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan passwordPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.put(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(passwordPolicySingletonID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op beyond dropping the resource from state: Rauthy has no way
// to remove its password policy.
func (r *passwordPolicyResource) Delete(
	_ context.Context,
	_ resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.Diagnostics.AddWarning(
		"The Rauthy password policy was left in place",
		"rauthy_password_policy has been removed from Terraform state, but Rauthy has no endpoint to delete "+
			"a password policy, so the instance keeps the settings last applied.",
	)
}

// put replaces the instance policy with the planned one.
//
// The response is deliberately discarded rather than folded back into the plan.
// Every attribute here is Optional and not Computed, so Terraform requires the
// state after apply to equal the plan exactly; writing back a response that
// Rauthy normalised — or an empty 200 body, which the client reports as success
// without touching the target — would abort the apply with "inconsistent result
// after apply" once the policy had already been replaced server-side. Any
// divergence instead shows up as ordinary drift on the next Read.
func (r *passwordPolicyResource) put(
	ctx context.Context,
	m *passwordPolicyResourceModel,
	diags *diag.Diagnostics,
) {
	_, err := r.api.UpdatePasswordPolicy(ctx, client.PasswordPolicy{
		LengthMin:        clampInt32(m.LengthMin.ValueInt64()),
		LengthMax:        clampInt32(m.LengthMax.ValueInt64()),
		IncludeLowerCase: int32Ptr(m.IncludeLowerCase),
		IncludeUpperCase: int32Ptr(m.IncludeUpperCase),
		IncludeDigits:    int32Ptr(m.IncludeDigits),
		IncludeSpecial:   int32Ptr(m.IncludeSpecial),
		ValidDays:        int32Ptr(m.ValidDays),
		NotRecentlyUsed:  int32Ptr(m.NotRecentlyUsed),
	})
	if err != nil {
		diags.AddError("Could not update the Rauthy password policy", err.Error())
	}
}

func applyPasswordPolicy(m *passwordPolicyResourceModel, p *client.PasswordPolicy) {
	m.ID = types.StringValue(passwordPolicySingletonID)
	m.LengthMin = types.Int64Value(int64(p.LengthMin))
	m.LengthMax = types.Int64Value(int64(p.LengthMax))
	m.IncludeLowerCase = optionalInt64(p.IncludeLowerCase)
	m.IncludeUpperCase = optionalInt64(p.IncludeUpperCase)
	m.IncludeDigits = optionalInt64(p.IncludeDigits)
	m.IncludeSpecial = optionalInt64(p.IncludeSpecial)
	m.ValidDays = optionalInt64(p.ValidDays)
	m.NotRecentlyUsed = optionalInt64(p.NotRecentlyUsed)
}

func int32Ptr(v types.Int64) *int32 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := clampInt32(v.ValueInt64())
	return &i
}

// clampInt32 narrows a Terraform Int64 to the int32 the Rauthy API uses.
//
// The schema validators already bound every one of these attributes far below
// the int32 range, so the clamp never fires in practice; it is here so the
// narrowing is provably safe rather than safe by argument.
func clampInt32(v int64) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	default:
		return int32(v)
	}
}

func optionalInt64(v *int32) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}
