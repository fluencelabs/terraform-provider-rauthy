# The password policy is a singleton: this adopts and replaces the one the
# instance already has. Anything left unset is disabled, not preserved.
resource "rauthy_password_policy" "this" {
  length_min = 12
  length_max = 128

  include_lower_case = 1
  include_upper_case = 1
  include_digits     = 1
  include_special    = 1

  not_recently_used = 3
}
