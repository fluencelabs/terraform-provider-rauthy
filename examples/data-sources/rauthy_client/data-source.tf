# Refer to an OIDC client that was created outside this configuration.
#
# The client secret is intentionally not exposed here; use the rauthy_client
# resource instead for a client Terraform manages and is responsible for
# rotating.
data "rauthy_client" "billing" {
  id = "billing-app"
}
