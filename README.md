# terraform-provider-rauthy

A Terraform provider for [Rauthy](https://github.com/sebadob/rauthy), managing OIDC clients through
Rauthy's admin API.

It exists because a confidential client's secret cannot be set from outside: Rauthy generates it and
`NewClientRequest.secret` is `#[serde(skip_deserializing)]`. The secret is therefore a **computed**
attribute read back from Rauthy, which rules out managing clients with `random_password` and a
hand-written bootstrap file.

Tracks Rauthy **0.36.2**. Requires **Terraform 1.11 or later** — see [Secrets and
state](#secrets-and-state) for why.

## Installation

Once the provider is published to the Terraform Registry:

```hcl
terraform {
  required_version = ">= 1.11.0"

  required_providers {
    rauthy = {
      source  = "fluencelabs/rauthy"
      version = "~> 0.1"
    }
  }
}
```

It is not published yet; see [PUBLISHING.md](PUBLISHING.md) for the remaining
one-time setup and for how to run against a locally built binary in the meantime.

## Provider configuration

```hcl
provider "rauthy" {
  # Base URL of the instance; the provider appends /auth/v1 itself.
  url = "https://auth.example.com" # or RAUTHY_URL

  # API key in `<name>$<secret>` form.
  api_key = var.rauthy_api_key # or RAUTHY_API_KEY
}
```

### Required API key rights

Create the key in the Rauthy Admin UI under *API Keys* with exactly these access rights:

| Group     | Rights                        | Used for                                              |
|-----------|-------------------------------|-------------------------------------------------------|
| `Clients` | read, create, update, delete  | the whole `rauthy_client` lifecycle                   |
| `Clients` | update                        | `rauthy_client_logo` and `rauthy_client_favicon`      |
| `Secrets` | read                          | reading the secret of a confidential client on every refresh |
| `Secrets` | update                        | rotating a client secret                              |

A key missing `Secrets:read` fails on every read of a confidential client; the provider reports the
missing right by name rather than surfacing a bare 403.

## Managing a client

```hcl
resource "rauthy_client" "backend" {
  id            = "example-backend"
  name          = "Example Backend"
  confidential  = true
  redirect_uris = ["https://app.example.com/oidc/callback"]

  flows_enabled         = ["authorization_code", "refresh_token"]
  scopes                = ["openid", "profile", "email"]
  default_scopes        = ["openid"]
  access_token_lifetime = 600

  # Rotation is never implicit: the secret changes only when this value does.
  secret_rotation_trigger    = "2026-01-01"
  secret_cache_current_hours = 6
}
```

Most attributes are optional *and* computed, because `PUT /clients/{id}` is a full replacement and
Rauthy requires a value for each of them. Leaving one out adopts Rauthy's own default; removing one
later keeps the last applied value rather than reverting it. See the resource documentation for the
full reasoning.

## Secrets and state

Secrets the practitioner **supplies** are write-only attributes, which Terraform passes to the
provider on apply and stores nowhere — not in the state file, not in a saved plan. That is
`rauthy_user.password_wo` and `rauthy_auth_provider.client_secret_wo`, and it is the reason for the
Terraform 1.11 floor: earlier versions cannot parse a write-only attribute at all.

Because a write-only value is invisible to the plan, each has a companion `*_rotation_trigger`.
Change the trigger in the same commit as the secret, or the apply is skipped.

Secrets Rauthy **generates** cannot be handled that way, and they stay in state in the clear:
`rauthy_client.secret` and `rauthy_api_key.secret`. Write-only is a one-way channel *into* the
provider; these travel the other way, from Rauthy out to whatever will authenticate with them.
Making them write-only would not hide the credential, it would delete the feature. If your
configuration manages clients or API keys, the state file is a credential store — put it in a
backend with encryption at rest and access control.

One deliberate loss is worth calling out. Rauthy *does* return an auth provider's upstream secret on
a read, so until it became write-only the provider tracked it, which is how a refresh noticed a
secret changed in the Admin UI and how `terraform import` recovered a complete resource. Both are
gone. Keeping a working upstream credential out of every state file was judged worth more.

## What this provider does not do

- It never writes a secret anywhere. `rauthy_client.secret` is a computed, sensitive attribute;
  wiring it into a secret store is the caller's job in HCL.
- It does not manage API keys. Rauthy's `/api_keys` endpoints require an **admin session**, not an
  API key (`principal.validate_admin_session()` in `src/api/src/api_keys.rs`), so a provider
  authenticated with an API key cannot create them. The bootstrap key is created by hand.

## Development

```sh
nix develop     # go, terraform, tfplugindocs, goreleaser
make build
make test
make docs
```

Building needs Go 1.27.4 or later (the `go` line in `go.mod`); the dev shell lets the `go` command
fetch that toolchain when nixpkgs ships an older one.

### The API client

The HTTP client for Rauthy's admin API is the separate module
[`github.com/fluencelabs/rauthy-go`](https://github.com/fluencelabs/rauthy-go) (package `rauthy`).
Its contract tests, which validate every request and response against the OpenAPI document Rauthy
serves, and the vendored copy of that document live there. This repository holds the provider and
its acceptance tests, which drive the client end to end against a live Rauthy:

```sh
eval "$(make -s rauthy-up)"   # needs Docker
make testacc
make rauthy-down
```

When moving to a new Rauthy release, bump `RAUTHY_VERSION` in the `Makefile` together with the
`rauthy-go` release that tracks it.

Rauthy's OpenAPI document does not carry the `validator` crate's ranges and regexes, so the value
constraints are enforced by provider-side validators instead.
