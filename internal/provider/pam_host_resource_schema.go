package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

func (r *pamHostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a host in Rauthy's PAM subsystem — a machine whose logins Rauthy " +
			"authenticates, identified to Rauthy by its id and the shared secret below.\n\n" +
			"Rauthy's published OpenAPI document has no create operation for a host, which reads as though " +
			"hosts could only register themselves. A live 0.36.2 does accept `POST /pam/hosts`, so this is " +
			"an ordinary managed resource; see the note in `internal/client/pam.go` for the evidence.\n\n" +
			"Requires these API key rights: `Pam` read, create, update, delete.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Rauthy-assigned host id, 24 alphanumeric characters. This is what a " +
					"PAM/NSS client sends as `host_id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Hostname. Renaming is done in place — unlike most of this provider's " +
					"`PUT`-shaped updates, this one really is a rename.",
				Validators: []validator.String{validators.PamHostname()},
			},
			"gid": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Numeric gid of the group this host belongs to, normally a " +
					"`rauthy_pam_group` of type `host`. Rauthy does not enforce the type, so pointing a host " +
					"at a `generic` group works and is almost certainly a mistake.",
			},
			"force_mfa": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Require a second factor for every login on this host.",
			},
			"local_password_only": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Accept only the PAM remote password a user generates in their account " +
					"dashboard, never their normal Rauthy password.",
			},
			"ips": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "IP addresses of the host, v4 or v6. Rauthy calls the same list " +
					"`addresses` in its host listing.",
				Validators: []validator.Set{validators.PamHostIPSet()},
			},
			"aliases": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Additional names the host answers to. Each follows the hostname rules.",
				Validators:          []validator.Set{validators.PamHostAliasSet()},
			},
			"notes": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-text note stored with the host.",
			},
			"secret": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "Shared secret the host authenticates with, read back from Rauthy on " +
					"every refresh. Rotating it would break every client already configured with the old " +
					"value, so this provider never rotates it; use the Admin UI when a rotation is wanted.",
			},
		},
	}
}
