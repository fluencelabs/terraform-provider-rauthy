// A host group: this is what a rauthy_pam_host points its gid at.
resource "rauthy_pam_group" "servers" {
  name = "servers"
  typ  = "host"
}

// An ordinary supplementary group, handed out to users.
resource "rauthy_pam_group" "developers" {
  name = "developers"
  typ  = "generic"
}
