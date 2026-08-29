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

  client_id     = "rauthy"
  client_secret = var.corp_client_secret
  scopes        = ["openid", "profile", "email"]

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

  client_id     = var.github_client_id
  client_secret = var.github_client_secret
  scopes        = ["user:email"]

  client_secret_post = true
}
