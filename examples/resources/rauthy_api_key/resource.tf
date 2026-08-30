# A key for a CI pipeline that manages OIDC clients and reads their secrets.
resource "rauthy_api_key" "ci" {
  name = "ci-clients"

  access = [
    {
      group         = "Clients"
      access_rights = ["read", "create", "update", "delete"]
    },
    {
      group         = "Secrets"
      access_rights = ["read", "update"]
    },
  ]

  # Any change to this value mints a new secret and invalidates the old one
  # immediately — there is no overlap window. Bump it on whatever cadence your
  # rotation policy asks for.
  secret_rotation_trigger = "2026-q1"
}

# A key that expires on its own. `expires_at` is an absolute Unix timestamp, so
# do not hardcode one: a fixed value drifts into the past, and Rauthy then
# rejects the next update to the key with a range error. Derive it from a
# resource that moves, and replace the key when it rotates.
resource "time_rotating" "audit_key" {
  rotation_days = 90
}

resource "rauthy_api_key" "audit" {
  name = "audit-readonly"

  access = [
    {
      group         = "Users"
      access_rights = ["read"]
    },
    {
      group         = "Groups"
      access_rights = ["read"]
    },
  ]

  expires_at              = time_rotating.audit_key.unix + 90 * 24 * 3600
  secret_rotation_trigger = time_rotating.audit_key.id
}

# The point of the resource: hand the credential to whatever needs it. `secret`
# is already in the `<name>$<secret>` form the Authorization header takes, so it
# goes straight into another provider configuration.
#
# Note the bootstrap ordering this implies: the key that manages rauthy_api_key
# resources has to exist before Terraform runs — created by Rauthy's
# BOOTSTRAP_API_KEY or by hand in the admin UI — and cannot be one of them.
provider "rauthy" {
  alias   = "ci"
  url     = "https://auth.example.com"
  api_key = rauthy_api_key.ci.secret
}

output "audit_api_key" {
  value     = rauthy_api_key.audit.secret
  sensitive = true
}
