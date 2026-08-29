package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*userResource)(nil)
	_ resource.ResourceWithConfigure   = (*userResource)(nil)
	_ resource.ResourceWithImportState = (*userResource)(nil)
)

// NewUserResource returns the rauthy_user resource.
func NewUserResource() resource.Resource { return &userResource{} }

type userResource struct {
	api *client.Client
}

type userValuesModel struct {
	Birthdate types.String `tfsdk:"birthdate"`
	City      types.String `tfsdk:"city"`
	Country   types.String `tfsdk:"country"`
	Phone     types.String `tfsdk:"phone"`
	Street    types.String `tfsdk:"street"`
	Zip       types.String `tfsdk:"zip"`
	TZ        types.String `tfsdk:"tz"`
}

type userResourceModel struct {
	ID                types.String     `tfsdk:"id"`
	Email             types.String     `tfsdk:"email"`
	GivenName         types.String     `tfsdk:"given_name"`
	FamilyName        types.String     `tfsdk:"family_name"`
	Language          types.String     `tfsdk:"language"`
	Enabled           types.Bool       `tfsdk:"enabled"`
	EmailVerified     types.Bool       `tfsdk:"email_verified"`
	Roles             types.Set        `tfsdk:"roles"`
	Groups            types.Set        `tfsdk:"groups"`
	UserExpires       types.Int64      `tfsdk:"user_expires"`
	Password          types.String     `tfsdk:"password"`
	UserValues        *userValuesModel `tfsdk:"user_values"`
	AccountType       types.String     `tfsdk:"account_type"`
	CreatedAt         types.Int64      `tfsdk:"created_at"`
	PreferredUsername types.String     `tfsdk:"preferred_username"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ImportState takes the Rauthy-assigned user id, not the email address.
func (r *userResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Create makes the user in two calls.
//
// POST /users carries only a fraction of a user — Rauthy's NewUserRequest has
// no `enabled`, `email_verified`, password or profile values — so the rest has
// to follow in a PUT, the same shape the client resource uses. The PUT is
// unconditional: `enabled` defaults to true here while a freshly created Rauthy
// user is disabled until its initial password reset, so skipping it when the
// configuration looks "empty" would leave state lying about the account.
func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateUser(ctx, client.NewUserRequest{
		Email:       plan.Email.ValueString(),
		Language:    plan.Language.ValueString(),
		Roles:       setToStringsNonNil(ctx, plan.Roles, &resp.Diagnostics),
		Groups:      setToStrings(ctx, plan.Groups, &resp.Diagnostics),
		GivenName:   stringPtr(plan.GivenName),
		FamilyName:  stringPtr(plan.FamilyName),
		UserExpires: int64Ptr(plan.UserExpires),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create Rauthy user "+plan.Email.ValueString(), err.Error())
		return
	}

	// The user exists from here on, so state must record its id even if the
	// PUT that follows fails; otherwise the next apply tries to create the
	// same email again and Rauthy rejects it as a duplicate.
	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), created.ID)...)

	updated, err := r.api.UpdateUser(ctx, created.ID, buildUpdateUserRequest(ctx, &plan, &resp.Diagnostics))
	if err != nil {
		resp.Diagnostics.AddError("Could not configure Rauthy user "+created.ID, err.Error())
		return
	}

	applyUser(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	got, err := r.api.GetUser(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy user "+id, err.Error())
		return
	}

	applyUser(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := plan.ID.ValueString()
	updated, err := r.api.UpdateUser(ctx, id, buildUpdateUserRequest(ctx, &plan, &resp.Diagnostics))
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy user "+id, err.Error())
		return
	}

	applyUser(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	if err := r.api.DeleteUser(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy user "+id, err.Error())
	}
}

func buildUpdateUserRequest(
	ctx context.Context,
	m *userResourceModel,
	diags *diag.Diagnostics,
) client.UpdateUserRequest {
	req := client.UpdateUserRequest{
		Email:         m.Email.ValueString(),
		Enabled:       m.Enabled.ValueBool(),
		EmailVerified: m.EmailVerified.ValueBool(),
		Roles:         setToStringsNonNil(ctx, m.Roles, diags),
		Groups:        setToStringsNonNil(ctx, m.Groups, diags),
		GivenName:     stringPtr(m.GivenName),
		FamilyName:    stringPtr(m.FamilyName),
		Language:      stringPtr(m.Language),
		Password:      stringPtr(m.Password),
		UserExpires:   int64Ptr(m.UserExpires),
	}
	if m.UserValues != nil {
		req.UserValues = &client.UserValues{
			Birthdate: stringPtr(m.UserValues.Birthdate),
			City:      stringPtr(m.UserValues.City),
			Country:   stringPtr(m.UserValues.Country),
			Phone:     stringPtr(m.UserValues.Phone),
			Street:    stringPtr(m.UserValues.Street),
			Zip:       stringPtr(m.UserValues.Zip),
			TZ:        stringPtr(m.UserValues.TZ),
		}
	}
	return req
}

// applyUser folds a response into the model.
//
// The model's own values are read before being overwritten — the plan at
// Create and Update, the prior state at Read — because several attributes are
// Optional without being Computed, and Terraform requires post-apply state to
// equal the configuration exactly. Rauthy collapses an empty set and an absent
// one into the same null, so the response alone cannot tell `[]` from unset.
//
// `password` is never returned and is left untouched for the same reason.
func applyUser(m *userResourceModel, u *client.UserResponse) {
	m.ID = types.StringValue(u.ID)
	m.Email = types.StringValue(u.Email)
	m.Enabled = types.BoolValue(u.Enabled)
	m.EmailVerified = types.BoolValue(u.EmailVerified)
	m.Language = types.StringValue(u.Language)
	m.GivenName = types.StringPointerValue(u.GivenName)
	m.FamilyName = types.StringPointerValue(u.FamilyName)
	m.Roles = optionalStringSet(u.Roles, m.Roles)
	m.Groups = optionalStringSet(u.Groups, m.Groups)
	m.UserExpires = types.Int64PointerValue(u.UserExpires)
	m.AccountType = types.StringValue(u.AccountType)
	m.CreatedAt = types.Int64Value(u.CreatedAt)
	m.PreferredUsername = types.StringPointerValue(u.UserValues.PreferredUsername)

	if m.UserValues != nil {
		m.UserValues = &userValuesModel{
			Birthdate: types.StringPointerValue(u.UserValues.Birthdate),
			City:      types.StringPointerValue(u.UserValues.City),
			Country:   types.StringPointerValue(u.UserValues.Country),
			Phone:     types.StringPointerValue(u.UserValues.Phone),
			Street:    types.StringPointerValue(u.UserValues.Street),
			Zip:       types.StringPointerValue(u.UserValues.Zip),
			TZ:        types.StringPointerValue(u.UserValues.TZ),
		}
	}
}

// optionalStringSet renders a set, keeping `[]` distinct from unset.
func optionalStringSet(values []string, prior types.Set) types.Set {
	if len(values) > 0 {
		return stringsToSet(values)
	}
	if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
		return prior
	}
	return types.SetNull(types.StringType)
}

// setToStringsNonNil is setToStrings for the fields Rauthy requires to be
// present: a nil slice marshals to `null`, which its deserializer rejects.
func setToStringsNonNil(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	out := setToStrings(ctx, set, diags)
	if out == nil {
		return []string{}
	}
	return out
}
