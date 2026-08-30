// A PAM user attaches POSIX attributes to a Rauthy identity that already
// exists; it never creates one. The email is the link between the two.
resource "rauthy_user" "alice" {
  email       = "alice@example.com"
  given_name  = "Alice"
  family_name = "Anderson"
}

resource "rauthy_pam_group" "developers" {
  name = "developers"
  typ  = "generic"
}

resource "rauthy_pam_user" "alice" {
  username = "alice"
  email    = rauthy_user.alice.email

  shell    = "/bin/zsh"
  home_dir = "/home/alice"

  // The complete membership set. Rauthy replaces it wholesale on every write,
  // so an omitted group is a removed group — including the personal group it
  // created for this account, which cannot be listed here because its gid is
  // an attribute of this very resource. Leave `groups` out entirely to keep
  // whatever Rauthy set up instead.
  groups = [
    {
      gid   = rauthy_pam_group.developers.id
      wheel = true
    },
  ]
}
