resource "rauthy_role" "engineer" {
  name = "engineer"
}

resource "rauthy_group" "platform" {
  name = "platform"
}

resource "rauthy_user" "ada" {
  email       = "ada@example.com"
  given_name  = "Ada"
  family_name = "Lovelace"
  language    = "en"

  # The account is usable straight away; leave email_verified at its default
  # to make the user confirm the address through Rauthy first.
  enabled        = true
  email_verified = true

  roles  = [rauthy_role.engineer.name]
  groups = [rauthy_group.platform.name]

  user_values = {
    city    = "London"
    country = "United Kingdom"
    tz      = "Europe/London"
  }
}

# A service account that needs a password rather than a passkey.
#
# password_wo is write-only: Terraform passes it to the provider on apply and
# stores nothing, so the password reaches neither the state file nor a saved
# plan. That needs Terraform 1.11 or later.
#
# The consequence to plan around is that a write-only value is invisible to the
# plan, so changing it alone produces no diff and nothing is applied. The
# trigger is what makes the change visible; bump it in the same commit as the
# password. Prefer leaving both unset for a human user and letting them set
# their own password through Rauthy's reset flow.
resource "rauthy_user" "svc" {
  email          = "svc-batch@example.com"
  given_name     = "Batch"
  language       = "en"
  email_verified = true

  password_wo               = var.svc_initial_password
  password_rotation_trigger = var.svc_password_version
}
