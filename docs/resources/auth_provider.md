---
page_title: "rauthy_auth_provider Resource - rauthy"
description: |-
    An upstream authentication provider: a federated OIDC or OAuth2 identity that Rauthy delegates login to, such as a corporate IdP, GitHub or Google.
  Creation is POST /providers/create, not POST /providers — Rauthy serves the listing on POST /providers, so the create endpoint had to move out of its way. There is no GET /providers/{id} at all: reading one back means fetching the whole list and picking it out, which the provider does for you.
  PUT /providers/{id} is a full replacement and the provider always sends every field, client_secret_wo included. Do not omit client_secret_wo from a configuration that had one: an update that does not carry it erases the stored secret.
  The upstream secret is a write-only attribute and is never written to Terraform state. That requires Terraform 1.11 or later; see client_secret_wo for what it costs.
  Rauthy assigns the id, so the endpoints, the client credentials and even the type can all be changed in place; nothing on this resource forces a replacement.
  The branding image at /providers/{id}/img is not managed here.
  Requires these API key rights: AuthProviders read, create, update, delete. That access group only exists from Rauthy 0.36 onwards — against 0.35 an API key cannot reach these endpoints at all.
---

# rauthy_auth_provider (Resource)

An upstream authentication provider: a federated OIDC or OAuth2 identity that Rauthy delegates login to, such as a corporate IdP, GitHub or Google.

Creation is `POST /providers/create`, not `POST /providers` — Rauthy serves the *listing* on `POST /providers`, so the create endpoint had to move out of its way. There is no `GET /providers/{id}` at all: reading one back means fetching the whole list and picking it out, which the provider does for you.

`PUT /providers/{id}` is a full replacement and the provider always sends every field, `client_secret_wo` included. Do not omit `client_secret_wo` from a configuration that had one: an update that does not carry it erases the stored secret.

The upstream secret is a write-only attribute and is never written to Terraform state. That requires Terraform 1.11 or later; see `client_secret_wo` for what it costs.

Rauthy assigns the id, so the endpoints, the client credentials and even the type can all be changed in place; nothing on this resource forces a replacement.

The branding image at `/providers/{id}/img` is not managed here.

Requires these API key rights: `AuthProviders` read, create, update, delete. That access group only exists from Rauthy 0.36 onwards — against 0.35 an API key cannot reach these endpoints at all.

## Example Usage

```terraform
# A corporate OIDC issuer, with the endpoints discovered rather than typed out.
# The lookup is performed by the Rauthy server, so it needs to be able to reach
# the issuer.
data "rauthy_auth_provider_lookup" "corp" {
  issuer = "https://idp.example.com"
}

resource "rauthy_auth_provider" "corp" {
  name = "Example Corp"
  type = "oidc"

  issuer                 = data.rauthy_auth_provider_lookup.corp.resolved_issuer
  authorization_endpoint = data.rauthy_auth_provider_lookup.corp.authorization_endpoint
  token_endpoint         = data.rauthy_auth_provider_lookup.corp.token_endpoint
  userinfo_endpoint      = data.rauthy_auth_provider_lookup.corp.userinfo_endpoint
  jwks_endpoint          = data.rauthy_auth_provider_lookup.corp.jwks_endpoint

  client_id = "rauthy"
  scopes    = ["openid", "profile", "email"]

  # The upstream secret is write-only: Terraform hands it to the provider and
  # stores nothing, so it never lands in the state file. That needs Terraform
  # 1.11 or later, and it means the value is invisible to the plan — bump the
  # trigger in the same commit as the secret, or the apply is skipped.
  client_secret_wo               = var.corp_client_secret
  client_secret_rotation_trigger = var.corp_client_secret_version

  # Everyone in the upstream's rauthy-admins group administers Rauthy too.
  admin_claim_path  = "$.groups"
  admin_claim_value = "rauthy-admins"

  # Create the local account on first login, and attach it to an existing one
  # with the same address. auto_link trusts the upstream's email verification;
  # only turn it on for an issuer you control.
  auto_onboarding = true
  auto_link       = true
}

# GitHub, which is OAuth2 rather than OIDC and has no discovery document.
resource "rauthy_auth_provider" "github" {
  name = "GitHub"
  type = "github"

  issuer                 = "github.com"
  authorization_endpoint = "https://github.com/login/oauth/authorize"
  token_endpoint         = "https://github.com/login/oauth/access_token"
  userinfo_endpoint      = "https://api.github.com/user"

  client_id = var.github_client_id
  scopes    = ["user:email"]

  client_secret_wo               = var.github_client_secret
  client_secret_rotation_trigger = var.github_client_secret_version

  client_secret_post = true
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `authorization_endpoint` (String) The upstream's authorization endpoint, where users are sent to log in.
- `client_id` (String) The client id Rauthy was issued by the upstream.
- `issuer` (String) The upstream's issuer, as it appears in the `iss` claim. Use the `rauthy_auth_provider_lookup` data source to discover the rest of the endpoints from it.
- `name` (String) Display name, shown on the login page as the button label. Rauthy does not require it to be unique.
- `scopes` (Set of String) Scopes Rauthy requests from the upstream, usually `openid`, `profile` and `email`.

Rauthy models this as one string but writes and reads it in two different forms — space-separated going in, `+`-joined coming back — so it is a set here and the provider does the conversion.
- `token_endpoint` (String) The upstream's token endpoint, where the authorization code is redeemed.
- `type` (String) Provider flavour: `oidc` for a standards-compliant OIDC issuer, `github` or `google` for the two Rauthy has built-in quirks for, `custom` for a plain OAuth2 endpoint with no discovery. Maps to Rauthy's `typ` field.
- `userinfo_endpoint` (String) The upstream's userinfo endpoint, read to build the local account.

### Optional

> **NOTE**: [Write-only arguments](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments) are supported in Terraform 1.11 and later.

- `admin_claim_path` (String) JSON path into the upstream's claims that decides whether a user is a Rauthy admin, for example `$.roles`. Paired with `admin_claim_value`.
- `admin_claim_value` (String) Value `admin_claim_path` must yield for the user to be an admin.
- `auto_link` (Boolean) Link an upstream identity to an existing local account with the same email address without asking. This trusts the upstream's email verification; leave it off for a provider that does not verify addresses.
- `auto_onboarding` (Boolean) Create a local account automatically the first time someone logs in through this provider, instead of requiring one to exist already.
- `client_secret_basic` (Boolean) Send the client credentials as an HTTP Basic header on the token request.
- `client_secret_post` (Boolean) Send the client credentials in the token request body. Some upstreams accept only this form; both may be enabled at once.
- `client_secret_rotation_trigger` (String) An arbitrary value whose every change makes the provider re-send `client_secret_wo`. Setting it for the first time, or removing it, counts as a change.

It exists because a write-only attribute is invisible to the plan: with no companion value that *is* tracked, Terraform cannot tell an apply carrying a new secret from one carrying the old, and skips the update entirely. This is the same mechanism `rauthy_api_key.secret_rotation_trigger` uses.

Any other change to the provider re-sends the secret too, since the update is a full replacement.
- `client_secret_wo` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The client secret Rauthy was issued by the upstream. At most 256 characters. Omit it for a public client that authenticates with PKCE alone.

This is a **write-only** attribute: Terraform hands it to the provider on the apply that uses it and stores nothing, so the upstream credential reaches neither the state file nor a saved plan. Requires Terraform 1.11 or later.

That trade is deliberate and it does cost something. Rauthy *does* return this secret in the clear on a read, so the provider could track it — and until this attribute became write-only it did, which is how a refresh used to notice a secret changed in the Admin UI and how an import used to recover a complete resource. Both of those are now gone: a drifted secret is invisible, and after `terraform import` the first apply re-asserts whatever the configuration says. Keeping a working upstream credential out of every state file was judged worth more than either.

Because nothing is stored, changing the value on its own produces no plan. Change `client_secret_rotation_trigger` alongside it to make Terraform apply the new secret.

Removing it from the configuration removes it from the provider: `PUT /providers/{id}` is a full replacement, so the update that follows carries no secret and Rauthy stores none.
- `enabled` (Boolean) Whether the provider is offered as a login option.
- `jwks_endpoint` (String) The upstream's JWKS endpoint. Without one Rauthy cannot verify the upstream's token signatures itself and falls back to the userinfo endpoint.
- `mfa_claim_path` (String) JSON path into the upstream's claims that says the user authenticated with MFA, for example `$.amr`. Paired with `mfa_claim_value`.
- `mfa_claim_value` (String) Value `mfa_claim_path` must yield for the login to count as MFA.
- `use_pkce` (Boolean) Use PKCE with the upstream. Leave it on unless the upstream rejects it.

### Read-Only

- `id` (String) Rauthy-assigned identifier of the provider.

## Import

Import is supported using the following syntax:

```shell
# Providers are imported by their Rauthy-assigned id. Rauthy has no
# GET /providers/{id}, so find it in the list — which is served on POST:
#   curl -X POST -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/providers" | jq -r '.[] | "\(.id)\t\(.name)"'
#
# The upstream secret is not recovered. Rauthy does return it on a read, but
# client_secret_wo is write-only and the provider deliberately drops it rather
# than write a working credential into state — so put the secret back into the
# configuration yourself, and the first apply after the import re-asserts it.
terraform import rauthy_auth_provider.corp aBMZzO5vPucY8OurKcHQqTK1
```
