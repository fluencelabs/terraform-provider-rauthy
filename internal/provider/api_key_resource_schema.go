package provider

import (
	"context"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

// apiKeyResourceDescription is the resource's own documentation. The secret
// caveats are the whole story of this resource, so they are told up front
// rather than buried in the attribute that carries them.
const apiKeyResourceDescription = "An admin API key of the Rauthy instance itself — the same kind of " +
	"credential this provider authenticates with.\n\n" +
	"It exists for bootstrapping: create the first key by hand (Rauthy's `BOOTSTRAP_API_KEY`, or the " +
	"admin UI), configure the provider with it, and manage every further key from Terraform. The key " +
	"Terraform itself uses needs `ApiKeys` read, create, update and delete, an access group that " +
	"exists only from Rauthy 0.36 onwards — on 0.35 and earlier `/api_keys` accepted nothing but an " +
	"admin browser session and no API key could reach it at all.\n\n" +
	"## The secret\n\n" +
	"Rauthy discloses a key's secret exactly once, in the plain-text answer to the call that mints " +
	"it. There is no endpoint that reads it back. That has three consequences:\n\n" +
	"- **The secret is in your Terraform state, in the clear.** Anyone who can read the state file " +
	"holds a working admin credential, and a key that carries `ApiKeys` rights can mint further keys " +
	"with any rights at all — treat the state as the credential store it now is, and keep it in a " +
	"backend with encryption and access control.\n" +
	"- **An imported key has no secret.** `terraform import` recovers the name, expiry and grants, " +
	"but `secret` stays null. Rotate the key if you need Terraform to hold a usable credential for " +
	"it, and exclude `secret` from `ImportStateVerify` in any test that imports one.\n" +
	"- **Losing it means rotating it.** Change `secret_rotation_trigger` to mint a replacement.\n\n" +
	"## Renaming\n\n" +
	"`PUT /api_keys/{name}` applies only the expiry and the grants: it compares the name in the body " +
	"against the one in the path and rejects a mismatch outright. Changing `name` therefore destroys " +
	"the key and creates a new one — with a new secret, and a window in which neither is usable by " +
	"whatever held the old one."

// apiKeySecretNote is repeated on the two attributes that carry the credential,
// because either one alone is enough to leak it.
const apiKeySecretNote = "\n\nThis value is stored in Terraform state in the clear and cannot be " +
	"re-read from Rauthy."

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := make(map[string]schema.Attribute)
	maps.Copy(attributes, apiKeyIdentityAttributes())
	maps.Copy(attributes, apiKeyAccessAttributes())
	maps.Copy(attributes, apiKeySecretAttributes())

	resp.Schema = schema.Schema{
		MarkdownDescription: apiKeyResourceDescription,
		Attributes:          attributes,
	}
}

// apiKeyIdentityAttributes covers who the key is and how long it lives.
func apiKeyIdentityAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Required: true,
			MarkdownDescription: "Name of the API key, which is also its identity: the first half of the " +
				"`<name>$<secret>` credential and the path segment every `/api_keys` call uses. " +
				"Must match `^[a-zA-Z0-9_-/]{2,24}$`.\n\n" +
				"Changing it replaces the key; Rauthy has no rename.",
			Validators: []validator.String{validators.APIKeyName()},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"expires_at": schema.Int64Attribute{
			Optional: true,
			MarkdownDescription: "Unix timestamp in seconds at which the key stops working. Omit for a key " +
				"that never expires.\n\n" +
				"Rauthy rejects a timestamp in the past, so this cannot be used to retire a key — " +
				"delete it instead. Note also that a fixed timestamp in a configuration silently " +
				"becomes a past one as time passes, which turns the next unrelated update into an " +
				"error; compute it from `time_offset` or similar rather than writing it down.",
		},
		"created_at": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Unix timestamp in seconds at which Rauthy created the key.",
		},
	}
}

// apiKeyAccessAttributes covers what the key is allowed to do.
func apiKeyAccessAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"access": schema.SetNestedAttribute{
			Required: true,
			MarkdownDescription: "The key's rights, one entry per access group. A group not listed here " +
				"grants nothing, and an empty set is a key that can do nothing at all — which Rauthy " +
				"accepts.\n\n" +
				"This is a set rather than a list: Rauthy stores the entries in the order they were " +
				"sent and makes no promise to keep it, so ordering must not be part of the comparison.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"group": schema.StringAttribute{
						Required: true,
						MarkdownDescription: "The access group. One of `Blacklist`, `Clients`, `Events`, " +
							"`Generic`, `Groups`, `Roles`, `Secrets`, `Sessions`, `Scopes`, " +
							"`UserAttributes`, `Users`, `Pam`, `AuthProviders`, `ApiKeys`.\n\n" +
							"`ApiKeys` grants management of API keys themselves, including the power " +
							"to mint keys with rights the granting key does not have. `UserAttributes` " +
							"and `ApiKeys` exist only from Rauthy 0.36 onwards.",
						Validators: []validator.String{validators.AccessGroup()},
					},
					"access_rights": schema.SetAttribute{
						Required:    true,
						ElementType: types.StringType,
						MarkdownDescription: "What the key may do within the group: any of `read`, " +
							"`create`, `update`, `delete`. May be empty.",
						Validators: []validator.Set{validators.AccessRightsSet()},
					},
				},
			},
		},
	}
}

// apiKeySecretAttributes covers the credential and its rotation.
func apiKeySecretAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"secret": schema.StringAttribute{
			Computed:  true,
			Sensitive: true,
			MarkdownDescription: "The full credential, in the `<name>$<secret>` form an " +
				"`Authorization: API-Key` header takes — ready to hand to another provider " +
				"configuration as-is, no assembly required.\n\n" +
				"Null for a key that was imported, since Rauthy never discloses an existing key's " +
				"secret; rotate it to obtain one." + apiKeySecretNote,
		},
		"secret_rotation_trigger": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "An arbitrary value whose every change rotates the secret. Setting it " +
				"for the first time, or removing it, counts as a change.\n\n" +
				"Rotation is immediate and total: the old secret stops working the moment the new one " +
				"is issued, with none of the overlap window `rauthy_client` gets from " +
				"`secret_cache_current_hours`. Anything still presenting the old credential — " +
				"including a Terraform provider configured with this very key — breaks at that " +
				"instant." + apiKeySecretNote,
		},
	}
}
