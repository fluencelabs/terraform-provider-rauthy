package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

var (
	_ resource.Resource                = (*clientImageResource)(nil)
	_ resource.ResourceWithConfigure   = (*clientImageResource)(nil)
	_ resource.ResourceWithImportState = (*clientImageResource)(nil)
)

// NewClientLogoResource returns the rauthy_client_logo resource.
func NewClientLogoResource() resource.Resource {
	return &clientImageResource{kind: client.ImageLogo}
}

// NewClientFaviconResource returns the rauthy_client_favicon resource.
func NewClientFaviconResource() resource.Resource {
	return &clientImageResource{kind: client.ImageFavicon}
}

// clientImageResource backs both branding resources. The logo and the favicon
// differ only in their path segment and in the sizing advice in their docs, so
// they share one implementation rather than two near-identical copies.
type clientImageResource struct {
	api  *client.Client
	kind client.ImageKind
}

type clientImageResourceModel struct {
	ClientID      types.String `tfsdk:"client_id"`
	ContentBase64 types.String `tfsdk:"content_base64"`
	ContentType   types.String `tfsdk:"content_type"`
	StoredHash    types.String `tfsdk:"stored_hash"`
}

func (r *clientImageResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_client_" + string(r.kind)
}

func (r *clientImageResource) Configure(
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

// ImportState takes the client id. Import recovers the fact that an image
// exists and the hash of what Rauthy serves, but it cannot recover
// `content_base64`: the stored bytes are a transcode of the original upload, so
// there is nothing to write back that would round-trip. The first plan after an
// import therefore always shows `content_base64` being set, and applying it
// re-uploads the image the configuration names.
func (r *clientImageResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("client_id"), req, resp)
}

func (r *clientImageResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan clientImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.put(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientImageResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan clientImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The upload is a wholesale replacement, so there is nothing to clean up
	// first: a PUT over an existing image discards the old one.
	r.put(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clientImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ClientID.ValueString()
	got, err := r.api.GetClientImage(ctx, id, r.kind)
	if client.IsNotFound(err) {
		// A 404 means either the image was deleted out of band or the client
		// itself is gone. Both call for the same thing: drop the resource so
		// the next apply recreates it, which for a missing client will fail
		// loudly against the client resource instead.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the "+string(r.kind)+" of Rauthy client "+id, err.Error())
		return
	}

	state.StoredHash = types.StringValue(hashBytes(got.Data))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clientImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clientImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ClientID.ValueString()
	if err := r.api.DeleteClientImage(ctx, id, r.kind); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete the "+string(r.kind)+" of Rauthy client "+id, err.Error())
	}
}

// put uploads the planned image and fills in the two computed attributes. It
// reads the image back afterwards because the only durable fact Rauthy exposes
// about a stored image is its bytes — there is no id, timestamp or checksum on
// any response — so the hash has to be computed from a download.
func (r *clientImageResource) put(ctx context.Context, plan *clientImageResourceModel, diags *diag.Diagnostics) {
	id := plan.ClientID.ValueString()

	data, err := base64.StdEncoding.DecodeString(plan.ContentBase64.ValueString())
	if err != nil {
		diags.AddAttributeError(
			path.Root("content_base64"),
			"Invalid base64 content",
			"content_base64 must be standard base64. Build it with Terraform's filebase64() function. Decode error: "+
				err.Error(),
		)
		return
	}

	contentType := plan.ContentType.ValueString()
	if plan.ContentType.IsUnknown() || plan.ContentType.IsNull() {
		contentType, err = client.SniffImageMime(data)
		if err != nil {
			diags.AddAttributeError(
				path.Root("content_base64"),
				"Could not determine the image type",
				"Set content_type explicitly if the image is valid. "+err.Error(),
			)
			return
		}
	}

	upload := client.Image{Data: data, ContentType: contentType}
	if putErr := r.api.PutClientImage(ctx, id, r.kind, upload); putErr != nil {
		diags.AddError("Could not upload the "+string(r.kind)+" of Rauthy client "+id, putErr.Error())
		return
	}

	stored, err := r.api.GetClientImage(ctx, id, r.kind)
	if err != nil {
		diags.AddError("Could not read back the "+string(r.kind)+" of Rauthy client "+id, err.Error())
		return
	}

	plan.ContentType = types.StringValue(contentType)
	plan.StoredHash = types.StringValue(hashBytes(stored.Data))
}

// hashBytes is the digest recorded in stored_hash.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
