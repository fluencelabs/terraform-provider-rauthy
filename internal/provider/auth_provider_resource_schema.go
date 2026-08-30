package provider

import (
	"context"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/provider/validators"
)

const authProviderResourceDescription = "An upstream authentication provider: a federated OIDC or " +
	"OAuth2 identity that Rauthy delegates login to, such as a corporate IdP, GitHub or Google.\n\n" +
	"Creation is `POST /providers/create`, not `POST /providers` — Rauthy serves the *listing* on " +
	"`POST /providers`, so the create endpoint had to move out of its way. There is no " +
	"`GET /providers/{id}` at all: reading one back means fetching the whole list and picking it " +
	"out, which the provider does for you.\n\n" +
	"`PUT /providers/{id}` is a full replacement and the provider always sends every field, " +
	"`client_secret_wo` included. Do not omit `client_secret_wo` from a configuration that had " +
	"one: an update that does not carry it erases the stored secret.\n\n" +
	"The upstream secret is a write-only attribute and is never written to Terraform state. " +
	"That requires Terraform 1.11 or later; see `client_secret_wo` for what it costs.\n\n" +
	"Rauthy assigns the id, so the endpoints, the client credentials and even the type can all be " +
	"changed in place; nothing on this resource forces a replacement.\n\n" +
	"The branding image at `/providers/{id}/img` is not managed here.\n\n" +
	"Requires these API key rights: `AuthProviders` read, create, update, delete. That access " +
	"group only exists from Rauthy 0.36 onwards — against 0.35 an API key cannot reach these " +
	"endpoints at all."

func (r *authProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := make(map[string]schema.Attribute)
	maps.Copy(attributes, authProviderIdentityAttributes())
	maps.Copy(attributes, authProviderEndpointAttributes())
	maps.Copy(attributes, authProviderCredentialAttributes())
	maps.Copy(attributes, authProviderClaimAttributes())

	resp.Schema = schema.Schema{
		MarkdownDescription: authProviderResourceDescription,
		Attributes:          attributes,
	}
}

// authProviderIdentityAttributes covers what the provider is called and whether
// it is offered on the login page.
func authProviderIdentityAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Rauthy-assigned identifier of the provider.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Required: true,
			MarkdownDescription: "Display name, shown on the login page as the button label. " +
				"Rauthy does not require it to be unique.",
			Validators: []validator.String{validators.AuthProviderName()},
		},
		"type": schema.StringAttribute{
			Required: true,
			MarkdownDescription: "Provider flavour: `oidc` for a standards-compliant OIDC issuer, " +
				"`github` or `google` for the two Rauthy has built-in quirks for, `custom` for a " +
				"plain OAuth2 endpoint with no discovery. Maps to Rauthy's `typ` field.",
			Validators: []validator.String{validators.AuthProviderType()},
		},
		"enabled": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
			MarkdownDescription: "Whether the provider is offered as a login option.",
		},
		"auto_onboarding": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
			MarkdownDescription: "Create a local account automatically the first time someone logs " +
				"in through this provider, instead of requiring one to exist already.",
		},
		"auto_link": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
			MarkdownDescription: "Link an upstream identity to an existing local account with the " +
				"same email address without asking. This trusts the upstream's email verification; " +
				"leave it off for a provider that does not verify addresses.",
		},
	}
}

// authProviderEndpointAttributes covers the upstream's OAuth2/OIDC endpoints.
func authProviderEndpointAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"issuer": schema.StringAttribute{
			Required: true,
			MarkdownDescription: "The upstream's issuer, as it appears in the `iss` claim. " +
				"Use the `rauthy_auth_provider_lookup` data source to discover the rest of the " +
				"endpoints from it.",
			Validators: []validator.String{validators.AuthProviderURI()},
		},
		"authorization_endpoint": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The upstream's authorization endpoint, where users are sent to log in.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
		"token_endpoint": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The upstream's token endpoint, where the authorization code is redeemed.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
		"userinfo_endpoint": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The upstream's userinfo endpoint, read to build the local account.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
		"jwks_endpoint": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "The upstream's JWKS endpoint. Without one Rauthy cannot verify the " +
				"upstream's token signatures itself and falls back to the userinfo endpoint.",
			Validators: []validator.String{validators.AuthProviderURI()},
		},
	}
}

// authProviderCredentialAttributes covers how Rauthy authenticates itself to
// the upstream.
func authProviderCredentialAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"client_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The client id Rauthy was issued by the upstream.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
		"client_secret_wo": schema.StringAttribute{
			Optional:  true,
			WriteOnly: true,
			MarkdownDescription: "The client secret Rauthy was issued by the upstream. At most 256 " +
				"characters. Omit it for a public client that authenticates with PKCE alone.\n\n" +
				"This is a **write-only** attribute: Terraform hands it to the provider on the apply " +
				"that uses it and stores nothing, so the upstream credential reaches neither the " +
				"state file nor a saved plan. Requires Terraform 1.11 or later.\n\n" +
				"That trade is deliberate and it does cost something. Rauthy *does* return this " +
				"secret in the clear on a read, so the provider could track it — and until this " +
				"attribute became write-only it did, which is how a refresh used to notice a secret " +
				"changed in the Admin UI and how an import used to recover a complete resource. " +
				"Both of those are now gone: a drifted secret is invisible, and after " +
				"`terraform import` the first apply re-asserts whatever the configuration says. " +
				"Keeping a working upstream credential out of every state file was judged worth " +
				"more than either.\n\n" +
				"Because nothing is stored, changing the value on its own produces no plan. Change " +
				"`client_secret_rotation_trigger` alongside it to make Terraform apply the new " +
				"secret.\n\n" +
				"Removing it from the configuration removes it from the provider: `PUT " +
				"/providers/{id}` is a full replacement, so the update that follows carries no " +
				"secret and Rauthy stores none.",
			Validators: []validator.String{validators.AuthProviderSecret()},
		},
		"client_secret_rotation_trigger": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "An arbitrary value whose every change makes the provider re-send " +
				"`client_secret_wo`. Setting it for the first time, or removing it, counts as a " +
				"change.\n\n" +
				"It exists because a write-only attribute is invisible to the plan: with no companion " +
				"value that *is* tracked, Terraform cannot tell an apply carrying a new secret from " +
				"one carrying the old, and skips the update entirely. This is the same mechanism " +
				"`rauthy_api_key.secret_rotation_trigger` uses.\n\n" +
				"Any other change to the provider re-sends the secret too, since the update is a " +
				"full replacement.",
		},
		"scopes": schema.SetAttribute{
			Required:    true,
			ElementType: types.StringType,
			MarkdownDescription: "Scopes Rauthy requests from the upstream, usually `openid`, " +
				"`profile` and `email`.\n\n" +
				"Rauthy models this as one string but writes and reads it in two different forms — " +
				"space-separated going in, `+`-joined coming back — so it is a set here and the " +
				"provider does the conversion.",
			Validators: []validator.Set{validators.AuthProviderScopeSet()},
		},
		"use_pkce": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
			MarkdownDescription: "Use PKCE with the upstream. Leave it on unless the upstream rejects it.",
		},
		"client_secret_basic": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(true),
			MarkdownDescription: "Send the client credentials as an HTTP Basic header on the token " +
				"request.",
		},
		"client_secret_post": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
			MarkdownDescription: "Send the client credentials in the token request body. Some " +
				"upstreams accept only this form; both may be enabled at once.",
		},
	}
}

// authProviderClaimAttributes covers the two upstream claims Rauthy can act on.
// Each is a path/value pair and only takes effect when both halves are set.
func authProviderClaimAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"admin_claim_path": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "JSON path into the upstream's claims that decides whether a user " +
				"is a Rauthy admin, for example `$.roles`. Paired with `admin_claim_value`.",
			Validators: []validator.String{validators.AuthProviderURI()},
		},
		"admin_claim_value": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Value `admin_claim_path` must yield for the user to be an admin.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
		"mfa_claim_path": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "JSON path into the upstream's claims that says the user " +
				"authenticated with MFA, for example `$.amr`. Paired with `mfa_claim_value`.",
			Validators: []validator.String{validators.AuthProviderURI()},
		},
		"mfa_claim_value": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Value `mfa_claim_path` must yield for the login to count as MFA.",
			Validators:          []validator.String{validators.AuthProviderURI()},
		},
	}
}
