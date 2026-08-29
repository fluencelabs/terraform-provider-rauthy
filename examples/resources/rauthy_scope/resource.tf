# A custom scope that adds user attributes to the issued tokens.
resource "rauthy_scope" "billing" {
  name = "read:billing"

  attr_include_access = ["department", "cost_center"]
  attr_include_id     = ["department"]
}

# Clients reference the scope by name.
resource "rauthy_client" "billing_app" {
  id            = "billing-app"
  name          = "Billing App"
  confidential  = true
  redirect_uris = ["https://billing.example.com/oidc/callback"]

  scopes         = ["openid", "profile", rauthy_scope.billing.name]
  default_scopes = ["openid"]
}
