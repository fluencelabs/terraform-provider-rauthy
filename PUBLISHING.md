# Publishing

This provider publishes to the **Terraform Registry** (registry.terraform.io),
and can additionally be listed on the **OpenTofu Registry**
(search.opentofu.org). Both consume the same GPG-signed GitHub release produced
by `.goreleaser.yml` / `.github/workflows/release.yml`.

Once published, it is consumed like any other provider — no local setup on the
part of whoever uses it:

```hcl
terraform {
  required_providers {
    rauthy = {
      source  = "fluencelabs/rauthy"
      version = "~> 0.1"
    }
  }
}
```

## Status

- [x] Repository is public (a requirement: the registry only accepts public repos)
- [x] Repository is named `terraform-provider-rauthy` (the registry derives the
      provider name from it)
- [x] `terraform-registry-manifest.json` declares protocol version 6.0
- [x] Release automation wired up (release-please + GoReleaser)
- [ ] GPG signing key generated and added to the repo secrets
- [ ] Provider registered on registry.terraform.io under the `fluencelabs` namespace
- [ ] First release cut

The three unchecked items are one-time setup and are described below. They
require repository-owner and organisation rights, so they are done by hand
rather than by CI.

## One-time setup

### 1. Generate a GPG signing key

The registry refuses a release whose `SHA256SUMS` signature does not verify
against a public key registered to the namespace, so this is not optional.

**The key must be RSA or DSA.** The registry API rejects the ECC keys `gpg`
offers by default, ed25519 included, so pick "RSA and RSA" in the prompt rather
than accepting the default.

```sh
# Generate a key: choose (1) RSA and RSA, 4096 bits, no expiry or a long one.
# An expiring key means the release workflow starts failing on a date nobody
# has written down.
gpg --full-generate-key

# Find the key id (the long hex after rsa4096/).
gpg --list-secret-keys --keyid-format=long

# Export the PRIVATE key (for CI). Keep this secret.
gpg --armor --export-secret-keys <KEY_ID> > private.asc

# Export the PUBLIC key (for the registry).
gpg --armor --export <KEY_ID> > public.asc
```

### 2. Add the CI secrets

In the GitHub repo: **Settings → Secrets and variables → Actions → New secret**

| Secret            | Value                                          |
| ----------------- | ---------------------------------------------- |
| `GPG_PRIVATE_KEY` | contents of `private.asc`                      |
| `PASSPHRASE`      | the passphrase set when generating the key     |

Set `PASSPHRASE` even if it feels redundant: the import step in the release
workflow reads it, and a key created with a passphrase fails to import without
it. If the key has no passphrase, leave the secret unset.

The release workflow imports this key and exports its fingerprint as
`GPG_FINGERPRINT`, which `.goreleaser.yml` uses to sign the checksums.

Delete `private.asc` from disk once the secret is set.

### 3. Register on the Terraform Registry

1. Sign in at https://registry.terraform.io with GitHub.
2. **Publish → Provider**, authorize the app, select this repository.
3. Add the **public** key (`public.asc`) to the `fluencelabs` namespace under
   **User Settings → Signing Keys**.

Publishing under an organisation namespace requires the GitHub organisation to
have authorized the registry's OAuth app, and the person doing it to be an
organisation owner.

### 4. Optionally, register on the OpenTofu Registry

OpenTofu uses a PR-based registry: open a PR against
https://github.com/opentofu/registry adding a provider entry under the
namespace along with the GPG public key. See
https://github.com/opentofu/registry#adding-a-provider for the current format.

## Cutting a release

Releases are automated with [release-please](https://github.com/googleapis/release-please)
driven by [Conventional Commits](https://www.conventionalcommits.org/):

- `feat: ...` → minor bump
- `fix: ...` → patch bump
- `feat!: ...` or a `BREAKING CHANGE:` footer → major bump

The flow:

1. Push commits to `main`. The `release` workflow opens (or updates) a
   **release PR** that bumps the version in `.release-please-manifest.json` and
   writes `CHANGELOG.md`.
2. **Merge the release PR.** release-please then creates the `vX.Y.Z` tag and a
   GitHub Release containing the changelog.
3. In the same workflow run, GoReleaser builds the platform binaries, signs the
   checksums with the imported key, and **appends** the artifacts to that
   release.
4. The registry picks up the new release automatically via its webhook, usually
   within a few minutes.

> Tags are `vMAJOR.MINOR.PATCH`, produced by release-please.

> **Why not `git tag` manually?** A tag pushed by CI's `GITHUB_TOKEN` does not
> trigger other workflows, so signing and building run in the *same* workflow
> right after release-please, gated on `release_created`. Merging the release PR
> is the trigger.

### Versioning before 1.0

`release-please-config.json` sets `initial-version` to `0.1.0`, because
release-please otherwise cuts `1.0.0` as the first release of a package that has
never been released. It also sets `bump-minor-pre-major`, so while the version
is below `1.0.0` a `feat!:` or `BREAKING CHANGE:` bumps the **minor** rather
than jumping to `2.0.0`: `0.1.0 → 0.2.0`. That is the usual reading of semver
for a pre-1.0 project, and it is deliberate here — the provider has not yet been
exercised against a live Rauthy instance, so the schema should be free to change
without burning major versions.

Cut `1.0.0` when the resource has been run against a real instance and its
attributes are considered settled. From that point on a renamed or removed
attribute means a major bump.

### The first release

The version in `.release-please-manifest.json` starts at `0.0.0`. The first
merge to `main` carrying `feat:` commits produces `v0.1.0`. Until the registry
lists that version, `terraform init` cannot resolve `fluencelabs/rauthy` — there
is nothing to download yet.

## Using an unreleased build

Before the first release, or to test a change without cutting one, point
Terraform at a locally built binary:

```sh
go build -o ~/go/bin/terraform-provider-rauthy .
```

```hcl
# ~/.terraformrc  (OpenTofu: ~/.tofurc)
provider_installation {
  dev_overrides {
    "fluencelabs/rauthy" = "/Users/you/go/bin"
  }
  direct {}
}
```

Run `terraform plan` directly — with `dev_overrides` in place `terraform init`
reports an error and is skipped. Remove the block once the provider is published,
or Terraform will keep using the local binary and ignore the version constraint.
