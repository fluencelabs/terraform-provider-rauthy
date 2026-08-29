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
