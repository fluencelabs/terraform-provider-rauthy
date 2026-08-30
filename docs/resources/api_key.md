---
page_title: "rauthy_api_key Resource - rauthy"
description: |-
    An admin API key of the Rauthy instance itself — the same kind of credential this provider authenticates with.
  It exists for bootstrapping: create the first key by hand (Rauthy's BOOTSTRAP_API_KEY, or the admin UI), configure the provider with it, and manage every further key from Terraform. The key Terraform itself uses needs ApiKeys read, create, update and delete, an access group that exists only from Rauthy 0.36 onwards — on 0.35 and earlier /api_keys accepted nothing but an admin browser session and no API key could reach it at all.
  The secret
  Rauthy discloses a key's secret exactly once, in the plain-text answer to the call that mints it. There is no endpoint that reads it back. That has three consequences:
  The secret is in your Terraform state, in the clear. Anyone who can read the state file holds a working admin credential, and a key that carries ApiKeys rights can mint further keys with any rights at all — treat the state as the credential store it now is, and keep it in a backend with encryption and access control.An imported key has no secret. terraform import recovers the name, expiry and grants, but secret stays null. Rotate the key if you need Terraform to hold a usable credential for it, and exclude secret from ImportStateVerify in any test that imports one.Losing it means rotating it. Change secret_rotation_trigger to mint a replacement.
  Renaming
  PUT /api_keys/{name} applies only the expiry and the grants: it compares the name in the body against the one in the path and rejects a mismatch outright. Changing name therefore destroys the key and creates a new one — with a new secret, and a window in which neither is usable by whatever held the old one.
---

# rauthy_api_key (Resource)

An admin API key of the Rauthy instance itself — the same kind of credential this provider authenticates with.

It exists for bootstrapping: create the first key by hand (Rauthy's `BOOTSTRAP_API_KEY`, or the admin UI), configure the provider with it, and manage every further key from Terraform. The key Terraform itself uses needs `ApiKeys` read, create, update and delete, an access group that exists only from Rauthy 0.36 onwards — on 0.35 and earlier `/api_keys` accepted nothing but an admin browser session and no API key could reach it at all.

## The secret

Rauthy discloses a key's secret exactly once, in the plain-text answer to the call that mints it. There is no endpoint that reads it back. That has three consequences:

- **The secret is in your Terraform state, in the clear.** Anyone who can read the state file holds a working admin credential, and a key that carries `ApiKeys` rights can mint further keys with any rights at all — treat the state as the credential store it now is, and keep it in a backend with encryption and access control.
- **An imported key has no secret.** `terraform import` recovers the name, expiry and grants, but `secret` stays null. Rotate the key if you need Terraform to hold a usable credential for it, and exclude `secret` from `ImportStateVerify` in any test that imports one.
- **Losing it means rotating it.** Change `secret_rotation_trigger` to mint a replacement.

## Renaming

`PUT /api_keys/{name}` applies only the expiry and the grants: it compares the name in the body against the one in the path and rejects a mismatch outright. Changing `name` therefore destroys the key and creates a new one — with a new secret, and a window in which neither is usable by whatever held the old one.

## Example Usage

```terraform
# A key for a CI pipeline that manages OIDC clients and reads their secrets.
resource "rauthy_api_key" "ci" {
  name = "ci-clients"

  access = [
    {
      group         = "Clients"
      access_rights = ["read", "create", "update", "delete"]
    },
    {
      group         = "Secrets"
      access_rights = ["read", "update"]
    },
  ]

  # Any change to this value mints a new secret and invalidates the old one
  # immediately — there is no overlap window. Bump it on whatever cadence your
  # rotation policy asks for.
  secret_rotation_trigger = "2026-q1"
}

# A key that expires on its own. `expires_at` is an absolute Unix timestamp, so
# do not hardcode one: a fixed value drifts into the past, and Rauthy then
# rejects the next update to the key with a range error. Derive it from a
# resource that moves, and replace the key when it rotates.
resource "time_rotating" "audit_key" {
  rotation_days = 90
}

resource "rauthy_api_key" "audit" {
  name = "audit-readonly"

  access = [
    {
      group         = "Users"
      access_rights = ["read"]
    },
    {
      group         = "Groups"
      access_rights = ["read"]
    },
  ]

  expires_at              = time_rotating.audit_key.unix + 90 * 24 * 3600
  secret_rotation_trigger = time_rotating.audit_key.id
}

# The point of the resource: hand the credential to whatever needs it. `secret`
# is already in the `<name>$<secret>` form the Authorization header takes, so it
# goes straight into another provider configuration.
#
# Note the bootstrap ordering this implies: the key that manages rauthy_api_key
# resources has to exist before Terraform runs — created by Rauthy's
# BOOTSTRAP_API_KEY or by hand in the admin UI — and cannot be one of them.
provider "rauthy" {
  alias   = "ci"
  url     = "https://auth.example.com"
  api_key = rauthy_api_key.ci.secret
}

output "audit_api_key" {
  value     = rauthy_api_key.audit.secret
  sensitive = true
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `access` (Attributes Set) The key's rights, one entry per access group. A group not listed here grants nothing, and an empty set is a key that can do nothing at all — which Rauthy accepts.

This is a set rather than a list: Rauthy stores the entries in the order they were sent and makes no promise to keep it, so ordering must not be part of the comparison. (see [below for nested schema](#nestedatt--access))
- `name` (String) Name of the API key, which is also its identity: the first half of the `<name>$<secret>` credential and the path segment every `/api_keys` call uses. Must match `^[a-zA-Z0-9_-/]{2,24}$`.

Changing it replaces the key; Rauthy has no rename.

### Optional

- `expires_at` (Number) Unix timestamp in seconds at which the key stops working. Omit for a key that never expires.

Rauthy rejects a timestamp in the past, so this cannot be used to retire a key — delete it instead. Note also that a fixed timestamp in a configuration silently becomes a past one as time passes, which turns the next unrelated update into an error; compute it from `time_offset` or similar rather than writing it down.
- `secret_rotation_trigger` (String) An arbitrary value whose every change rotates the secret. Setting it for the first time, or removing it, counts as a change.

Rotation is immediate and total: the old secret stops working the moment the new one is issued, with none of the overlap window `rauthy_client` gets from `secret_cache_current_hours`. Anything still presenting the old credential — including a Terraform provider configured with this very key — breaks at that instant.

This value is stored in Terraform state in the clear and cannot be re-read from Rauthy.

### Read-Only

- `created_at` (Number) Unix timestamp in seconds at which Rauthy created the key.
- `secret` (String, Sensitive) The full credential, in the `<name>$<secret>` form an `Authorization: API-Key` header takes — ready to hand to another provider configuration as-is, no assembly required.

Null for a key that was imported, since Rauthy never discloses an existing key's secret; rotate it to obtain one.

This value is stored in Terraform state in the clear and cannot be re-read from Rauthy.

<a id="nestedatt--access"></a>
### Nested Schema for `access`

Required:

- `access_rights` (Set of String) What the key may do within the group: any of `read`, `create`, `update`, `delete`. May be empty.
- `group` (String) The access group. One of `Blacklist`, `Clients`, `Events`, `Generic`, `Groups`, `Roles`, `Secrets`, `Sessions`, `Scopes`, `UserAttributes`, `Users`, `Pam`, `AuthProviders`, `ApiKeys`.

`ApiKeys` grants management of API keys themselves, including the power to mint keys with rights the granting key does not have. `UserAttributes` and `ApiKeys` exist only from Rauthy 0.36 onwards.

## Import

Import is supported using the following syntax:

```shell
# API keys are imported by their name, which is also their identity in Rauthy:
#   curl -H "Authorization: API-Key $RAUTHY_API_KEY" \
#     "$RAUTHY_URL/auth/v1/api_keys" | jq -r '.keys[].name'
#
# The secret does NOT come back. Rauthy discloses it once, when the key is
# minted, and has no endpoint that reads it afterwards — so an imported key has
# a null `secret` until you rotate it by setting secret_rotation_trigger.
terraform import rauthy_api_key.ci ci-clients
```
