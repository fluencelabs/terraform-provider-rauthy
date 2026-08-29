# Refer to a scope that was created outside this configuration.
data "rauthy_scope" "billing" {
  name = "read:billing"
}
