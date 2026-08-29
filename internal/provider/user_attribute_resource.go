package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ resource.Resource                   = (*userAttributeResource)(nil)
	_ resource.ResourceWithConfigure      = (*userAttributeResource)(nil)
	_ resource.ResourceWithImportState    = (*userAttributeResource)(nil)
	_ resource.ResourceWithValidateConfig = (*userAttributeResource)(nil)
)

// NewUserAttributeResource returns the rauthy_user_attribute resource.
func NewUserAttributeResource() resource.Resource { return &userAttributeResource{} }

type userAttributeResource struct {
	api *client.Client
}

type userAttributeResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Desc         types.String `tfsdk:"desc"`
	DefaultValue types.String `tfsdk:"default_value"`
	UserEditable types.Bool   `tfsdk:"user_editable"`
}

func (r *userAttributeResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_user_attribute"
}

func (r *userAttributeResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom user attribute definition in Rauthy.\n\n" +
			"An attribute has to be defined on the instance before anything can reference it: " +
			"`rauthy_scope.attr_include_access` and `rauthy_scope.attr_include_id` are filtered against " +
			"the known definitions and unknown names are dropped silently, so a scope that maps an " +
			"attribute should depend on the `rauthy_user_attribute` that defines it.\n\n" +
			"Rauthy has no identifier for an attribute other than its name, so `id` mirrors `name` and a " +
			"rename is a real rename rather than a replacement.\n\n" +
			"Requires these API key rights: `UserAttributes` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Identifier of the attribute. Rauthy keys attributes by name, so this " +
					"is always equal to `name`; it exists so the resource can be imported and referenced " +
					"the way every other resource is.",
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the attribute, for example `department`. This is the key it " +
					"appears under in a user's `attributes` and in the tokens a mapping scope issues. " +
					"Renaming in place is supported.",
				Validators: []validator.String{validators.UserAttrConfigName()},
			},
			"desc": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Description of the attribute, shown in the Admin UI. Rauthy validates " +
					"it against `^[a-zA-Z0-9-_/]{0,128}$`, which notably excludes spaces.",
				Validators: []validator.String{validators.UserAttrDesc()},
			},
			"default_value": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Default value for the attribute, as a JSON document — Rauthy stores " +
					"arbitrary JSON here, not just strings, so a plain string default has to be written as " +
					"`jsonencode(\"engineering\")` or `\"\\\"engineering\\\"\"`. Whitespace is not " +
					"significant: the value is compared after compaction, so reformatting the JSON does " +
					"not produce a diff.",
			},
			"user_editable": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Whether users may change this attribute on themselves from the " +
					"account page. Defaults to `false`, which keeps the attribute admin-only.",
			},
		},
	}
}

func (r *userAttributeResource) Configure(
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

// ValidateConfig rejects a default_value that is not JSON at plan time. Rauthy
// would answer with a deserialization error at apply, naming neither the
// resource nor the attribute.
func (r *userAttributeResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var cfg userAttributeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.DefaultValue.IsNull() || cfg.DefaultValue.IsUnknown() {
		return
	}
	if !json.Valid([]byte(cfg.DefaultValue.ValueString())) {
		resp.Diagnostics.AddAttributeError(
			path.Root("default_value"),
			"default_value is not valid JSON",
			"Rauthy stores this attribute's default as arbitrary JSON. Wrap a plain string with "+
				"jsonencode(), for example jsonencode(\"engineering\").",
		)
	}
}

// ImportState takes the attribute name, which is also its id.
func (r *userAttributeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// Read() looks the attribute up by name, and an import has no prior state
	// to take it from, so the id doubles as the name here too.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *userAttributeResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan userAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateUserAttr(ctx, buildUserAttrRequest(&plan))
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not create Rauthy user attribute "+plan.Name.ValueString(), err.Error())
		return
	}

	applyUserAttr(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	got, err := r.api.GetUserAttr(ctx, name)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy user attribute "+name, err.Error())
		return
	}

	applyUserAttr(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userAttributeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state userAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The attribute is addressed by the name it currently has; the new one
	// travels in the body.
	current := state.Name.ValueString()
	if err := r.api.UpdateUserAttr(ctx, current, buildUserAttrRequest(&plan)); err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy user attribute "+current, err.Error())
		return
	}

	// PUT answers with an empty 200, so the stored attribute has to be read
	// back to learn what Rauthy actually kept.
	updated, err := r.api.GetUserAttr(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not read back Rauthy user attribute "+plan.Name.ValueString(), err.Error())
		return
	}

	applyUserAttr(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userAttributeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state userAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	if err := r.api.DeleteUserAttr(ctx, name); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy user attribute "+name, err.Error())
	}
}

func buildUserAttrRequest(m *userAttributeResourceModel) client.UserAttrRequest {
	editable := m.UserEditable.ValueBool()
	return client.UserAttrRequest{
		Name:         m.Name.ValueString(),
		Desc:         stringPtr(m.Desc),
		DefaultValue: rawJSON(m.DefaultValue),
		UserEditable: &editable,
	}
}

// applyUserAttr folds a response into the model.
//
// The model's own default_value is read before being overwritten: at Create and
// Update it holds the plan, at Read the prior state, and it decides how the
// value is spelled — see defaultValueString.
func applyUserAttr(m *userAttributeResourceModel, a *client.UserAttrResponse) {
	m.ID = types.StringValue(a.Name)
	m.Name = types.StringValue(a.Name)
	m.Desc = optionalString(a.Desc)
	m.DefaultValue = defaultValueString(a.DefaultValue, m.DefaultValue)
	m.UserEditable = types.BoolValue(a.UserEditable)
}

// rawJSON turns the configured default into request bytes. A null or unknown
// value is omitted from the body rather than sent as JSON null, which is how
// Rauthy is told the attribute has no default.
func rawJSON(v types.String) json.RawMessage {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return json.RawMessage(v.ValueString())
}

// defaultValueString renders the stored default back into the string attribute.
//
// default_value is Optional and not Computed, so post-apply state must equal
// the configuration exactly — and the configuration is a *rendering* of a JSON
// document, not the document itself. `{"a": 1}` and `{"a":1}` mean the same
// thing to Rauthy, which stores the parsed value and re-serialises it its own
// way. Handing Terraform the server's spelling of a value the configuration
// spelled differently aborts the apply with "inconsistent result after apply",
// so when the two are semantically equal the configured spelling wins.
//
// Equality is compaction-based, which does not see through reordered object
// keys; two spellings that differ only in key order would show as a permanent
// diff. Rauthy preserves key order for the values it round-trips, so this has
// not been worth a full structural comparison.
func defaultValueString(raw json.RawMessage, prior types.String) types.String {
	compacted, ok := compactJSON(raw)
	if !ok || compacted == "null" {
		return types.StringNull()
	}
	if !prior.IsNull() && !prior.IsUnknown() {
		if priorCompacted, priorOK := compactJSON([]byte(prior.ValueString())); priorOK &&
			priorCompacted == compacted {
			return prior
		}
	}
	return types.StringValue(compacted)
}

// compactJSON strips insignificant whitespace. It reports false for empty or
// malformed input, which for a response means the attribute has no default.
func compactJSON(raw []byte) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", false
	}
	return buf.String(), true
}
