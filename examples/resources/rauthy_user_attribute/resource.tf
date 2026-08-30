# A custom user attribute. It has to exist before a scope can map it into the
# issued tokens.
resource "rauthy_user_attribute" "department" {
  name = "department"
  desc = "org-unit"

  # The default is a JSON document, so a plain string default is encoded.
  default_value = jsonencode("engineering")

  # Leave this false to keep the attribute admin-only.
  user_editable = false
}

# The scope references the attribute, which also orders the two applies.
resource "rauthy_scope" "billing" {
  name = "read:billing"

  attr_include_access = [rauthy_user_attribute.department.name]
  attr_include_id     = [rauthy_user_attribute.department.name]
}
