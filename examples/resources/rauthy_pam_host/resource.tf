resource "rauthy_pam_group" "servers" {
  name = "servers"
  typ  = "host"
}

resource "rauthy_pam_host" "build01" {
  hostname = "build01.example.com"
  gid      = rauthy_pam_group.servers.id

  force_mfa           = true
  local_password_only = false

  ips     = ["10.0.0.10", "2001:db8::10"]
  aliases = ["build01", "ci.example.com"]
  notes   = "CI builder"
}

// The secret a PAM/NSS client on the host authenticates with, alongside the
// host id. Sensitive, so it never appears in plan output.
output "build01_host_secret" {
  value     = rauthy_pam_host.build01.secret
  sensitive = true
}
