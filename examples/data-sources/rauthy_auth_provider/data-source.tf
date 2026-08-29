# Refer to an upstream provider this configuration does not manage.
# Rauthy does not enforce unique provider names; a name shared by two providers
# is an error here rather than an arbitrary pick.
data "rauthy_auth_provider" "corp" {
  name = "Example Corp"
}
