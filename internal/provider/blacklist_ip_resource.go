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
	_ resource.Resource                = (*blacklistIPResource)(nil)
	_ resource.ResourceWithConfigure   = (*blacklistIPResource)(nil)
	_ resource.ResourceWithImportState = (*blacklistIPResource)(nil)
)

// NewBlacklistIPResource returns the rauthy_blacklist_ip resource.
func NewBlacklistIPResource() resource.Resource { return &blacklistIPResource{} }

type blacklistIPResource struct {
	api *client.Client
}

type blacklistIPResourceModel struct {
	ID  types.String `tfsdk:"id"`
	IP  types.String `tfsdk:"ip"`
	Exp types.Int64  `tfsdk:"exp"`
}

func (r *blacklistIPResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_blacklist_ip"
}

func (r *blacklistIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Blocks an IP address in Rauthy until a given point in time. Requests from a " +
			"blacklisted address are rejected before authentication.\n\n" +
			"**This resource expires on its own.** The entry is not permanent state the way a client or a " +
			"group is: Rauthy drops it the moment `exp` passes, with nothing on the Terraform side " +
			"involved. A refresh after that point finds the entry gone and removes it from state, exactly " +
			"as if it had been deleted by hand, and the next apply recreates it — which is only possible " +
			"while `exp` is still in the future. An `exp` that has passed by the time the apply runs is " +
			"an error rather than a silent no-op, because Rauthy accepts such a request with `200 OK` and " +
			"then discards it. Manage long blocks with this resource; short ones belong to Rauthy's own " +
			"automatic blacklisting, not to Terraform.\n\n" +
			"Changing `exp` updates the entry in place; changing `ip` replaces the resource.\n\n" +
			"Requires these API key rights: `Blacklist` read, create, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The blacklisted address as Rauthy renders it, which may differ in " +
					"spelling from `ip` — Rauthy parses the address and prints it back canonically, so " +
					"`2001:0DB8::0002` becomes `2001:db8::2`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "IPv4 or IPv6 address to block, for example `203.0.113.7`. " +
					"Kept exactly as written; see `id` for Rauthy's own rendering.",
				Validators: []validator.String{validators.IPAddress()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"exp": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Unix timestamp in seconds at which the block expires. Must be in " +
					"the future when the apply runs, and is rejected outright below `1719784800` " +
					"(2024-07-01), a fixed lower bound Rauthy enforces.",
				Validators: []validator.Int64{validators.BlacklistExp()},
			},
		},
	}
}

func (r *blacklistIPResource) Configure(
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

// ImportState takes the IP address. Both `id` and `ip` are set from it, so an
// import spelled non-canonically produces a one-time diff on `id` rather than
// a silent mismatch.
func (r *blacklistIPResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), req.ID)...)
}

func (r *blacklistIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan blacklistIPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *blacklistIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state blacklistIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip := state.IP.ValueString()
	got, err := r.api.GetBlacklistedIP(ctx, ip)
	if client.IsNotFound(err) {
		// The entry either expired or was lifted in the Admin UI. Both are
		// removals as far as Terraform is concerned; erroring here would leave
		// every configuration holding an expired block permanently broken.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Rauthy blacklist entry for "+ip, err.Error())
		return
	}

	state.ID = types.StringValue(got.IP)
	state.Exp = types.Int64Value(got.Exp)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only ever changes `exp`; `ip` is the identity and forces a replace.
func (r *blacklistIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan blacklistIPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *blacklistIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state blacklistIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Deleting by the canonical id rather than the configured spelling: the
	// endpoint answers 200 to anything, so a spelling Rauthy does not recognise
	// would look like a successful delete and leave the block in place.
	id := state.ID.ValueString()
	if err := r.api.DeleteBlacklistedIP(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not remove Rauthy blacklist entry for "+id, err.Error())
	}
}

// write posts the entry and reads it back, filling in the computed id.
//
// The read-back is not belt and braces. Rauthy answers 200 to a POST whose exp
// has already passed and then throws the entry away, so trusting the status
// code would write state for a block that does not exist and hand Terraform an
// apply that silently achieved nothing.
func (r *blacklistIPResource) write(ctx context.Context, plan *blacklistIPResourceModel, diags *diag.Diagnostics) {
	ip := plan.IP.ValueString()
	err := r.api.BlacklistIP(ctx, client.IPBlacklistRequest{
		IP:  ip,
		Exp: plan.Exp.ValueInt64(),
	})
	if err != nil {
		diags.AddError("Could not blacklist "+ip+" in Rauthy", err.Error())
		return
	}

	got, err := r.api.GetBlacklistedIP(ctx, ip)
	if client.IsNotFound(err) {
		diags.AddError(
			"Rauthy discarded the blacklist entry for "+ip,
			"Rauthy accepted the request and then dropped the entry, which it does when `exp` is already "+
				"in the past. Set `exp` to a timestamp in the future.",
		)
		return
	}
	if err != nil {
		diags.AddError("Could not read back the Rauthy blacklist entry for "+ip, err.Error())
		return
	}

	plan.ID = types.StringValue(got.IP)
	plan.Exp = types.Int64Value(got.Exp)
}
