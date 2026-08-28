# A confidential client: Rauthy generates the secret, the provider reads it back.
resource "rauthy_client" "backend" {
  id            = "example-backend"
  name          = "Example Backend"
  confidential  = true
  redirect_uris = ["https://app.example.com/oidc/callback"]

  post_logout_redirect_uris = ["https://app.example.com/"]
  allowed_origins           = ["https://app.example.com"]

  flows_enabled         = ["authorization_code", "refresh_token"]
  scopes                = ["openid", "profile", "email"]
  default_scopes        = ["openid"]
  access_token_alg      = "EdDSA"
  id_token_alg          = "EdDSA"
  auth_code_lifetime    = 60
  access_token_lifetime = 600
  force_mfa             = false

  # Change this value to rotate the secret. The old one keeps working for
  # another 6 hours so consumers can catch up.
  secret_rotation_trigger    = "2026-01-01"
  secret_cache_current_hours = 6
}

# The secret is computed and sensitive; wiring it anywhere is up to you.
output "backend_client_secret" {
  value     = rauthy_client.backend.secret
  sensitive = true
}

# A public single-page app: no secret, and Rauthy requires S256 PKCE from it.
resource "rauthy_client" "spa" {
  id            = "example-spa"
  name          = "Example SPA"
  confidential  = false
  redirect_uris = ["https://spa.example.com/callback"]

  allowed_origins = ["https://spa.example.com"]
  flows_enabled   = ["authorization_code", "refresh_token"]
  scopes          = ["openid", "profile"]
  default_scopes  = ["openid"]
}
