terraform {
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
