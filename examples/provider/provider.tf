terraform {
  # 1.11 is the floor, not a preference: rauthy_user.password_wo and
  # rauthy_auth_provider.client_secret_wo are write-only attributes, which
  # earlier versions of Terraform cannot parse at all.
  required_version = ">= 1.11.0"

  required_providers {
    rauthy = {
      source = "fluencelabs/rauthy"
    }
  }
}

provider "rauthy" {
  # The instance root; the provider appends /auth/v1 itself.
  url = "https://auth.example.com"

  # `<name>$<secret>`, as shown once when the key is created in the Admin UI.
  # Reads RAUTHY_URL / RAUTHY_API_KEY when these are omitted.
  api_key = var.rauthy_api_key
}
