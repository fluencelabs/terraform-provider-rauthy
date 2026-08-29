# Look up a user this configuration does not manage, by email or by id.
data "rauthy_user" "ada" {
  email = "ada@example.com"
}

data "rauthy_user" "by_id" {
  id = "9f9a1a1e-0d3f-4f1e-9a1b-2c3d4e5f6a7b"
}
