# Discover an upstream's OIDC endpoints instead of transcribing them.
# The Rauthy server fetches the discovery document, so this fails when Rauthy
# cannot reach the issuer even if the machine running Terraform can.
data "rauthy_auth_provider_lookup" "google" {
  issuer = "accounts.google.com"
}

# For an upstream that does not serve the document at the well-known path.
data "rauthy_auth_provider_lookup" "custom" {
  metadata_url = "https://idp.example.com/.well-known/custom-configuration"
}
