package provider

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/rauthy-go"
	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

var (
	_ resource.Resource                   = (*userAttributeResource)(nil)
	_ resource.ResourceWithConfigure      = (*userAttributeResource)(nil)
	_ resource.ResourceWithImportState    = (*userAttributeResource)(nil)
	_ resource.ResourceWithValidateConfig = (*userAttributeResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*userAttributeResource)(nil)
)

// NewUserAttributeResource returns the rauthy_user_attribute resource.
func NewUserAttributeResource() resource.Resource { return &userAttributeResource{} }

type userAttributeResource struct {
	api *rauthy.Client
}

type userAttributeResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Desc         types.String `tfsdk:"desc"`
	DefaultValue jsonString   `tfsdk:"default_value"`
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
			"Rauthy has no identifier for an attribute other than its name, so `id` mirrors `name`, and " +
			"changing the name replaces the attribute rather than renaming it in place.\n\n" +
			"Deletion and name replacement are refused when Rauthy would corrupt another attribute name " +
			"in a scope mapping by substring. Remove the conflicting mapping in a separate apply first.\n\n" +
			"Requires these API key rights: `UserAttributes` read, create, update, delete; `Scopes` read.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Identifier of the attribute. Rauthy keys attributes by name, so this " +
					"is always equal to `name`; it exists so the resource can be imported and referenced " +
					"the way every other resource is.",
				// Not UseStateForUnknown: the id is a copy of the planned
				// name, so it can simply be planned as one, and an unrelated
				// change then no longer shows `id` as known after apply.
				PlanModifiers: []planmodifier.String{idMirrorsName{}},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the attribute, for example `department`. This is the key it " +
					"appears under in a user's `attributes` and in the tokens a mapping scope issues. " +
					"Changing it replaces the attribute, because Rauthy's in-place rename leaks the old " +
					"name — see the note on UpdateUserAttr in the client.",
				Validators: []validator.String{validators.UserAttrConfigName()},
			},
			"desc": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Description of the attribute, shown in the Admin UI. Rauthy validates " +
					"it against `" + rauthy.AttributeDescriptionPattern().String() + "`.",
				Validators: []validator.String{validators.UserAttrDesc()},
			},
			"default_value": schema.StringAttribute{
				Optional:   true,
				CustomType: jsonStringType{},
				MarkdownDescription: "Default value for the attribute, as a JSON document — Rauthy stores " +
					"arbitrary JSON here, not just strings, so a plain string default has to be written as " +
					"`jsonencode(\"engineering\")` or `\"\\\"engineering\\\"\"`. Values are compared as " +
					"JSON rather than as text, so reformatting or reordering object keys is not a change. " +
					"Numbers must be integer literals in the i64/u64 range, or decimals with at most 15 " +
					"significant mantissa digits and decimal scale (exponent minus fractional digit count) " +
					"between -15 and 15, and a magnitude below 1e14. Other numbers are rejected before apply because Rauthy may round them during storage.",
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

// ValidateConfig rejects invalid JSON and unsupported numeric precision before
// mutation. Rauthy may round numbers during its two storage parse/serialize passes.
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
		return
	}
	if number := unstableJSONNumber(cfg.DefaultValue.ValueString()); number != "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("default_value"),
			"Unsupported JSON number",
			fmt.Sprintf(
				"Number %s may change during Rauthy's JSON storage round trip. Use an integer literal in the i64/u64 range, or a decimal with at most 15 significant mantissa digits and decimal scale (exponent minus fractional digit count) between -15 and 15 and a magnitude below 1e14.",
				number,
			),
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
	if !lockSubstringMutation(ctx, &resp.Diagnostics) {
		return
	}
	defer unlockSubstringMutation()

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
	attr, found, err := findUserAttr(ctx, r.api, name)
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy user attribute "+name, err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	applyUserAttr(&state, &attr)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userAttributeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	if !lockSubstringMutation(ctx, &resp.Diagnostics) {
		return
	}
	defer unlockSubstringMutation()

	var plan userAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var priorID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &priorID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A name unknown at plan time must resolve to the existing key for an update.
	if plan.Name.IsUnknown() || plan.Name.IsNull() || !plan.Name.Equal(priorID) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "User attribute name requires replacement",
			"Use a name known during planning so Terraform can plan the replacement.")
		return
	}

	name := priorID.ValueString()
	if err := r.api.UpdateUserAttr(ctx, name, buildUserAttrRequest(&plan)); err != nil {
		resp.Diagnostics.AddError("Could not update Rauthy user attribute "+name, err.Error())
		return
	}

	// The client discards the PUT body. Use the stored value when readback
	// succeeds and retain the applied plan when it fails.
	updated, found, err := findUserAttr(ctx, r.api, name)
	switch {
	case err != nil:
		resp.Diagnostics.AddError("Could not read back Rauthy user attribute "+name, err.Error())
	case !found:
		resp.Diagnostics.AddError("Could not read back Rauthy user attribute "+name,
			"Rauthy did not list the attribute after the update.")
	default:
		applyUserAttr(&plan, &updated)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ModifyPlan checks both explicit destruction and the deletion half of a name
// replacement. Attribute names are their keys, so read the current key from Rauthy.
func (r *userAttributeResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	var state userAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	replace := !req.Plan.Raw.IsNull()
	if replace {
		var plan userAttributeResourceModel
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if plan.Name.IsUnknown() || plan.Name.Equal(state.ID) {
			return
		}
	}
	if r.checkDeletion(ctx, state.ID.ValueString(), &resp.Diagnostics) && replace {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("name"))
	}
}

type userAttrListEntry struct {
	Name         *string         `json:"name"`
	Desc         *string         `json:"desc"`
	DefaultValue json.RawMessage `json:"default_value"`
	Typ          *string         `json:"typ"`
	UserEditable *bool           `json:"user_editable"`
}

func findUserAttr(ctx context.Context, api *rauthy.Client, name string) (rauthy.UserAttrResponse, bool, error) {
	var capture userAttrListResponseCapture
	_, err := api.ListUserAttrs(context.WithValue(ctx, userAttrListResponseKey{}, &capture))
	const listError = "GET /users/attr returned HTTP %d: %w"
	if err != nil {
		if _, apiError := errors.AsType[*rauthy.APIError](err); apiError || capture.status == 0 {
			return rauthy.UserAttrResponse{}, false, err
		}
		return rauthy.UserAttrResponse{}, false, fmt.Errorf(listError, capture.status, err)
	}
	if capture.status != http.StatusOK {
		return rauthy.UserAttrResponse{}, false, fmt.Errorf(
			"GET /users/attr returned HTTP %d: expected HTTP 200 for the values array", capture.status,
		)
	}
	attr, found, err := decodeUserAttrList(capture.body.Bytes(), name)
	if err != nil {
		return rauthy.UserAttrResponse{}, false, fmt.Errorf(listError, capture.status, err)
	}
	return attr, found, nil
}

func decodeUserAttrList(body []byte, name string) (rauthy.UserAttrResponse, bool, error) {
	var response struct {
		Values []*userAttrListEntry `json:"values"`
	}
	if err := jsonv2.Unmarshal(body, &response, jsonv2.RejectUnknownMembers(true)); err != nil {
		return rauthy.UserAttrResponse{}, false, fmt.Errorf("decode values: %w", err)
	}
	if response.Values == nil {
		return rauthy.UserAttrResponse{}, false, errors.New("response is missing the values array")
	}

	entries := make(map[string]*userAttrListEntry, len(response.Values))
	var conflictingName *string
	for i, entry := range response.Values {
		if entry == nil || entry.Name == nil || entry.UserEditable == nil {
			return rauthy.UserAttrResponse{}, false, fmt.Errorf(
				"values[%d] must be an object with a string name and a boolean user_editable", i,
			)
		}
		if entry.Typ != nil && *entry.Typ != "Email" {
			return rauthy.UserAttrResponse{}, false, fmt.Errorf("values[%d].typ must be Email or null", i)
		}
		key := *entry.Name
		previous, seen := entries[key]
		if !seen {
			entries[key] = entry
			continue
		}
		if reflect.DeepEqual(previous, entry) {
			continue
		}
		if name != "" && key == name {
			return rauthy.UserAttrResponse{}, false, fmt.Errorf(
				"values[%d] repeats an attribute name with different fields",
				i,
			)
		}
		conflictingName = entry.Name
	}
	match := entries[name]
	if name == "" || match == nil {
		// Conflicting rows can indicate a stale Rauthy cache. Such a list
		// cannot establish that an unlisted attribute is absent.
		if conflictingName != nil {
			return rauthy.UserAttrResponse{}, false, fmt.Errorf(
				"cannot determine whether attribute %q is absent: conflicting entries for %q may indicate a stale Rauthy cache",
				name,
				*conflictingName,
			)
		}
		return rauthy.UserAttrResponse{}, false, nil
	}
	return rauthy.UserAttrResponse{
		Name: name, Desc: match.Desc, DefaultValue: match.DefaultValue, Typ: match.Typ,
		UserEditable: *match.UserEditable,
	}, true, nil
}

// checkDeletion reports whether the attribute list was valid and adds any guard diagnostics.
func (r *userAttributeResource) checkDeletion(ctx context.Context, key string, diags *diag.Diagnostics) bool {
	_, found, err := findUserAttr(ctx, r.api, key)
	if err != nil {
		diags.AddError("Could not list Rauthy user attributes before checking deletion", err.Error())
		return false
	}
	if !found {
		diags.AddError(
			"User attribute not found in Rauthy's list",
			fmt.Sprintf("Rauthy did not list attribute %q. Retry if the list is stale. "+
				"If it was renamed outside Terraform, refresh state and import the attribute under its new name.", key),
		)
		return true
	}
	scopes, err := r.api.ListScopes(ctx)
	if err != nil {
		diags.AddError("Could not list Rauthy scopes before checking user attribute deletion", err.Error())
		return true
	}
	// Unlike role/group deletion, attribute deletion removes every occurrence,
	// including an interior substring, from both scope mapping lists.
	for _, scope := range scopes {
		for _, names := range [][]string{scope.AttrIncludeAccess, scope.AttrIncludeID} {
			if other, ok := containingName(key, names); ok {
				diags.AddError(
					"Unsafe user attribute deletion",
					fmt.Sprintf(
						"Rauthy removes attribute %q from scope mappings by substring, so it would also corrupt %q in scope %q. Remove that mapping in a separate apply first, then retry.",
						key,
						other,
						scope.Name,
					),
				)
				return true
			}
		}
	}
	return true
}

func (r *userAttributeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	if !lockSubstringMutation(ctx, &resp.Diagnostics) {
		return
	}
	defer unlockSubstringMutation()

	var state userAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.ID.ValueString()
	r.checkDeletion(ctx, name, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteUserAttr(ctx, name); err != nil && !rauthy.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete Rauthy user attribute "+name, err.Error())
	}
}

func buildUserAttrRequest(m *userAttributeResourceModel) rauthy.UserAttrRequest {
	editable := m.UserEditable.ValueBool()
	return rauthy.UserAttrRequest{
		Name:         m.Name.ValueString(),
		Desc:         stringPtr(m.Desc),
		DefaultValue: rawJSON(m.DefaultValue),
		UserEditable: &editable,
	}
}

// applyUserAttr folds a response into the model.
func applyUserAttr(m *userAttributeResourceModel, a *rauthy.UserAttrResponse) {
	m.ID = types.StringValue(a.Name)
	m.Name = types.StringValue(a.Name)
	m.Desc = optionalString(a.Desc)
	value := defaultValueString(a.DefaultValue)
	preserveNull := value.IsNull() && !m.DefaultValue.IsNull() && !m.DefaultValue.IsUnknown() &&
		bytes.Equal(bytes.TrimSpace([]byte(m.DefaultValue.ValueString())), []byte("null"))
	if !preserveNull {
		m.DefaultValue = value
	}

	m.UserEditable = types.BoolValue(a.UserEditable)
}

// rawJSON turns the configured default into request bytes. A null or unknown
// value is omitted from the body rather than sent as JSON null, which is how
// Rauthy is told the attribute has no default.
func rawJSON(v jsonString) json.RawMessage {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return json.RawMessage(v.ValueString())
}

// defaultValueString renders the stored default, compacted.
//
// This is Rauthy's spelling of the document, not the configuration's, and it
// deliberately makes no attempt to preserve the latter: jsonStringType's
// semantic equality puts the configured spelling back whenever the two are the
// same document, which is what keeps an apply from ending in "inconsistent
// result after apply" for an Optional, non-Computed attribute.
func defaultValueString(raw json.RawMessage) jsonString {
	compacted, ok := compactJSON(raw)
	if !ok || compacted == "null" {
		return nullJSONString()
	}
	return newJSONString(compacted)
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

// idMirrorsName plans the id as a copy of the planned name.
//
// Rauthy has no identifier for an attribute other than its name, so the two
// always agree — including after a rename, which is why UseStateForUnknown
// (which would keep the old name) is not what this attribute wants.
type idMirrorsName struct{}

func (idMirrorsName) Description(context.Context) string {
	return "Mirrors the planned name, which is also the attribute's identifier."
}

func (m idMirrorsName) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (idMirrorsName) PlanModifyString(
	ctx context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	// A null plan means destroy; there is nothing to mirror.
	if req.Plan.Raw.IsNull() {
		return
	}
	var name types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() || name.IsNull() || name.IsUnknown() {
		return
	}
	resp.PlanValue = types.StringValue(name.ValueString())
}
