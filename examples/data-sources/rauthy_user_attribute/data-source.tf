# Refer to an attribute that was defined outside this configuration, so that a
# scope mapping a name that no longer exists fails instead of being dropped.
data "rauthy_user_attribute" "department" {
  name = "department"
}
