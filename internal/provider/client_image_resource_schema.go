package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

func (r *clientImageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.description(),
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Id of the client this " + string(r.kind) + " belongs to. Changing it " +
					"replaces the resource, which uploads the image to the new client and removes it from the old one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content_base64": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The image, base64 encoded. Build it with Terraform's " +
					"[`filebase64`](https://developer.hashicorp.com/terraform/language/functions/filebase64) " +
					"function, for example `filebase64(\"${path.module}/logo.png\")`.\n\n" +
					"This attribute is the sole source of truth for the resource: the image is re-uploaded " +
					"whenever it changes, and never otherwise. It is stored in the Terraform state in full, so " +
					"prefer a modestly sized image.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"content_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Mime type declared for the upload. Rauthy trusts this value rather than " +
					"sniffing the bytes, and accepts only `" + client.MimePNG + "`, `" + client.MimeJPEG +
					"` and `" + client.MimeSVG + "`. Left unset, the provider derives it from the image's own " +
					"leading bytes; set it explicitly if that guess is wrong.",
				Validators: []validator.String{
					stringvalidator.OneOf(client.MimePNG, client.MimeJPEG, client.MimeSVG),
				},
			},
			"stored_hash": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "SHA-256, hex encoded, of the image bytes Rauthy serves back. This is not " +
					"the hash of `content_base64`: Rauthy re-encodes every raster upload to WebP and runs SVGs " +
					"through a sanitiser, so what is stored never matches what was sent. The value exists to " +
					"make out-of-band changes visible on a refresh; it does not by itself trigger a re-upload.",
			},
		},
	}
}

// description spells out the per-image guidance that is the only real
// difference between the two resources.
func (r *clientImageResource) description() string {
	const shared = "\n\nRauthy stores the image re-encoded rather than verbatim, and exposes no metadata about " +
		"it — no id, no timestamp, no checksum on any response — so this resource treats `content_base64` as " +
		"authoritative and reconciles only when that changes or when the image has disappeared from the server " +
		"entirely. An image replaced out of band through the Admin UI shows up as a changed `stored_hash` on the " +
		"next refresh, but is left alone until the configuration changes.\n\n" +
		"Requires the `Clients:update` API key right. Note that the OpenAPI document claims the logo upload " +
		"needs full `rauthy_admin` rights; on a live 0.36.2 an API key with `Clients:update` uploads both images."

	if r.kind == client.ImageLogo {
		return "Manages the custom logo shown on the login page for a Rauthy client." + shared +
			"\n\nRaster logos must be at least 84 pixels on a side; Rauthy rejects anything smaller. " +
			"An SVG is not subject to that limit and is the better choice, since it is stored as SVG " +
			"rather than transcoded."
	}
	return "Manages the custom favicon for a Rauthy client's login page." + shared +
		"\n\nA raster favicon must still be at least 84 pixels on a side — Rauthy applies the same floor it " +
		"applies to logos, so the usual 16 or 32 pixel favicon is rejected. Supply an SVG or a larger PNG."
}
