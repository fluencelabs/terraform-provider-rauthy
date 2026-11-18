---
page_title: "rauthy Provider"
description: |-
  Manage OIDC clients in a Rauthy identity provider with Terraform.
---

# rauthy Provider

Manages OIDC clients in [Rauthy](https://github.com/sebadob/rauthy) through its admin API.

A confidential client's secret cannot be supplied from outside: Rauthy generates it, and the field is
explicitly not deserialized from the create request. The secret is therefore read back from Rauthy as
a computed attribute, which is the reason this provider exists rather than a `random_password` and a
bootstrap file.

## Requirements

**Terraform 1.11 or later.** Two attributes — `rauthy_user.password_wo` and
`rauthy_auth_provider.client_secret_wo` — are write-only, a feature Terraform gained in 1.11.
Against an older version they are not merely ignored: the configuration fails to parse.

**Rauthy 0.36.2 or later.** The provider is built against Rauthy 0.36.2, and its acceptance suite
must pass against that release. CI also runs the suite against Rauthy's newest image, as an early
warning that is allowed to fail.

### Rauthy 0.35

Rauthy 0.35 is not supported. The acceptance suite was run once against 0.35.2. Part of the provider
works there and part does not. No other 0.35 release was tried, and 0.34 and earlier are untested.

Rauthy 0.35 answers `200` to a setting it does not know and then drops it. The provider gets no
error, so most of these failures do not show up during the apply. They show up as a diff on the
next plan, or as a value that never reached the server.

What does not work on 0.35.2:

| What                                         | What happens                                                                                     |
|----------------------------------------------|--------------------------------------------------------------------------------------------------|
| `rauthy_api_key`                             | `HTTP 401: No valid session` — 0.35 has no `ApiKeys` access group, so no API key can manage API keys |
| `rauthy_auth_provider` and its data source   | `HTTP 401: No valid session` on `/providers` — 0.35 has no `AuthProviders` access group            |
| `rauthy_client_favicon`                      | `PUT /clients/{id}/favicon` answers `404`                                                        |
| `rauthy_scope.claims_at_root`                | not stored; it reads back as `false` and the next plan sets it again                             |
| `rauthy_pam_user.home_dir`                   | reads back empty                                                                                 |
| client settings `rauthy_client` leaves alone | `allowed_resources`, `default_aud`, `claims` and `claims_at_root` are not stored at all          |

Because the PAM test stopped at `home_dir`, updates and import of `rauthy_pam_user` and
`rauthy_pam_host` were never reached on 0.35.2.

What passed on 0.35.2: `rauthy_client` (create, update, secret rotation, public clients) and its
data source, `rauthy_client_logo`, `rauthy_scope` without `claims_at_root` and its data source,
`rauthy_role`, `rauthy_group` and their data sources, `rauthy_user` (with roles, groups and
`password_wo`) and its data source, `rauthy_user_attribute` and its data source,
`rauthy_blacklist_ip`, `rauthy_password_policy` and `rauthy_pam_group`.
`rauthy_auth_provider_lookup` is not covered by the acceptance suite on any Rauthy version.

## Upgrading

From 0.3.x, the provider talks to Rauthy through the
[rauthy-go](https://github.com/fluencelabs/rauthy-go) client, which changes three things an
existing setup can run into:

* **Redirects are not followed.** A `3xx` answer is an error that names where it pointed, so the
  API key is never sent anywhere else. Set `url` to the address Rauthy answers on directly — an
  `http://` URL that redirects to `https://` now fails instead of being followed.
* **`url` must not carry a query or a fragment.** One is refused when the provider is configured;
  before, it ended up in front of every request path.
* **A secret rotation is sent once and never retried.** This covers `rauthy_client` and
  `rauthy_api_key`. If the request fails after it went out (a `5xx`, a dropped connection), the
  error says Rauthy may have rotated the secret anyway; the next apply rotates again.

## Known limits

* **Proxies and gateways.** Preserve Rauthy's response bodies, content types, and HTTP
  status codes. A route 404 with an empty or plain-text response reports an error and
  preserves state. Client image reads treat a 404 with an empty body and no
  `Content-Type` header as a missing image, so a missing proxy route can drop a logo
  or favicon from state.
  Verify that the proxy forwards both image routes before refreshing state.
* **Conflicts discovered at apply.** A scope, role, group, or user attribute created in the same
  apply can introduce a substring conflict after planning. A new name that is unknown at plan
  time can defer a scope rename error to apply. The provider
  refuses the unsafe change before writing it, but other resources may already have changed.
  Use a name known at plan time or apply the dependency separately and replan.
  [Scope, role, and group guard test](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/substring_guard_acc_test.go#L15)
  and [attribute guard tests](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/user_attribute_guard_internal_test.go#L135).
* **User attribute names unknown at plan time.** When the name of an existing attribute is
  unknown at plan time and resolves to a different name during apply, the apply fails even
  if no scope uses the attribute. Terraform could not plan the replacement. The attribute
  keeps its old name, and its dependencies may already have changed. Use a name known at
  plan time or apply the dependency separately and replan.
* **Concurrent writes.** Rauthy provides no conditional write for substring checks. An
  external client or a separate provider instance can add a conflicting name between the
  check and the write, which can corrupt assignments or mappings. Manage scopes, roles,
  groups, and user attributes on each Rauthy instance through one provider configuration
  and avoid concurrent writes through other aliases or clients.
  [Provider locking tests](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/substring_atomic_internal_test.go#L100).
* **External attribute changes.** If you rename a user attribute outside Terraform, refresh
  drops the old name from state. Update the configuration and import the attribute under its new
  name. An existing saved destroy plan fails if the attribute was removed or renamed before
  apply. Refresh state and create a new plan.
  [External rename and stale destroy test](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/user_attribute_missing_acc_internal_test.go#L14).
* **Imported public clients.** Import leaves a public client's PKCE configuration as it
  was in Rauthy. If it lacks S256, the client cannot use S256 PKCE until the next apply.
  Run `terraform apply` immediately after import to enable S256.
  [Public client import test](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/client_resource_acc_test.go#L356).
* **SCIM sync errors.** When Rauthy returns HTTP 500 with error code `Scim` after saving a
  client configuration, the provider reports the error and reads the saved client back into
  state if the read succeeds. Rauthy retries recorded sync failures. Check the SCIM sync in
  Rauthy and verify the endpoint and bearer token.
  [Client state recovery test](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/client_scim_error_internal_test.go#L35).
* **PAM user field lengths.** Rauthy checks `shell` and `home_dir` at apply. A value over
  the limit can fail after the account has been created. Keep `shell` to 24 characters
  and `home_dir` to 128 characters at most, then apply with the corrected values.
* **PAM user and group name collisions.** Creating a PAM user with the name of an
  existing PAM group can fail with a 404 whose diagnostic points to the email. Check
  for a group with that name, then choose a different username or resolve the group collision.
* **JSON numbers.** The provider rejects numbers outside its supported ranges at plan time
  to prevent rounding in Rauthy. This includes some fractions that Rauthy can store safely.
  If you need one of those values, encode it as a string if its consumers support string values.
  See `rauthy_user_attribute.default_value` for the accepted ranges.
  [JSON number validation test](https://github.com/fluencelabs/terraform-provider-rauthy/blob/1deb3ec515461515c745aa23306132ddef66311c/internal/provider/json_numbers_internal_test.go#L13).
* **Interrupted deletes.** If a DELETE request is canceled or the proxy connection drops,
  Rauthy may finish the request after the provider releases its lock. Once you have confirmed
  the outcome in Rauthy, refresh state and create a new plan before making related changes.

### Depends on a fix in Rauthy

* **User attribute list cache.** On a cache miss, Rauthy's `find_all` reads the database
  and caches the result. An overlapping read can cache a stale snapshot after a write
  invalidates the list. The stale entry can last up to 12 hours. On a cache miss in
  v0.36.2, create can cache the new attribute twice by appending it to the list just read.
  Terraform refresh can drop an attribute from state, and readback after PUT can return
  an older value. Refresh after the cache expires or restart Rauthy and refresh.
  Avoid changing attributes concurrently from multiple clients.
* **Invalid PostgreSQL attribute type.** A row in `user_attr_config` with an invalid
  `typ` value causes a panic during row parsing on a cache miss, so
  `GET /users/attr` fails for the entire list. Correct the row in PostgreSQL.

## Example usage

```terraform
terraform {
  # 1.11 is the floor, not a preference: rauthy_user.password_wo and
  # rauthy_auth_provider.client_secret_wo are write-only attributes, which
  # earlier versions of Terraform cannot parse at all.
  required_version = ">= 1.11.0"

  required_providers {
    rauthy = {
      source = "fluencelabs/rauthy"
    }
  }
}

provider "rauthy" {
  # The instance root; the provider appends /auth/v1 itself.
  url = "https://auth.example.com"

  # `<name>$<secret>`, as shown once when the key is created in the Admin UI.
  # Reads RAUTHY_URL / RAUTHY_API_KEY when these are omitted.
  api_key = var.rauthy_api_key
}
```

## Secrets and Terraform state

Four attributes on this provider carry a credential. Two of them are kept out of state entirely and
two cannot be, and the difference is worth understanding before you decide how to protect your state
file.

| Attribute                                | In state? | Why                                                           |
|------------------------------------------|-----------|---------------------------------------------------------------|
| `rauthy_user.password_wo`                | no        | write-only; Rauthy never returns a password either            |
| `rauthy_auth_provider.client_secret_wo`  | no        | write-only; costs drift detection and import, deliberately    |
| `rauthy_client.secret`                   | **yes**   | generated by Rauthy, read back — an output, not an input      |
| `rauthy_api_key.secret`                  | **yes**   | generated by Rauthy, disclosed once — an output, not an input |

A write-only attribute is a one-way channel *into* the provider. It has no way to carry a value back
out, so it can only replace something the practitioner supplies. `rauthy_client.secret` and
`rauthy_api_key.secret` are the opposite case: Rauthy mints them and the whole point of the
attributes is to hand them onward to another provider configuration. Making either write-only would
not hide the credential, it would delete the feature.

So the state file for a configuration that manages clients or API keys **is** a credential store.
Keep it in a backend with encryption at rest and access control, and treat read access to it as
equivalent to holding the credentials themselves.

## API key rights

The provider authenticates with a Rauthy API key, created by hand in the Admin UI under *API Keys*.
Grant the rights for the operations the key performs:

| Group     | Rights                       | Used for                                                      |
|-----------|------------------------------|---------------------------------------------------------------|
| `Clients` | read, create, update, delete | the whole `rauthy_client` lifecycle                           |
| `Secrets` | read                         | reading a confidential client's secret on every refresh       |
| `Secrets` | update                       | rotating a client secret                                      |
| `UserAttributes` | read, delete           | listing attributes and deleting the old name on deletion or replacement |
| `Scopes`  | read                         | checking scope mappings before deleting or replacing a user attribute by name |

A key missing `Secrets:read` fails on every read of a confidential client; the provider names the
missing right rather than reporting a bare 403.

The first key is always created by hand. Keys after that can be managed with `rauthy_api_key`, by a
key that holds the `ApiKeys` access group. That group exists from Rauthy 0.36 on; on 0.35,
`/api_keys` accepts only an admin session.

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `api_key` (String, Sensitive) Rauthy API key in `<name>$<secret>` form. Falls back to the `RAUTHY_API_KEY` environment variable.

The key needs the access rights of the resources it manages: `Clients` read, create, update, delete and `Secrets` read and update for `rauthy_client`; `Clients:update` for `rauthy_client_logo` and `rauthy_client_favicon`; `Scopes` read, create, update, delete for `rauthy_scope`; `Roles` and `Groups` read, create, update, delete for `rauthy_role` and `rauthy_group`; `Users` read, create, update, delete for `rauthy_user`; `UserAttributes` read, create, update, delete for `rauthy_user_attribute`, plus `Scopes:read` for deletion and name replacement; `AuthProviders` read, create, update, delete for `rauthy_auth_provider` (that access group exists only from Rauthy 0.36 onwards); `ApiKeys` read, create, update, delete for `rauthy_api_key` (also 0.36 and later only — before it, `/api_keys` accepted an admin browser session and nothing else); `Blacklist` read, create, delete for `rauthy_blacklist_ip`; `Pam` read, create, update, delete for `rauthy_pam_group`, `rauthy_pam_user` and `rauthy_pam_host`; `Secrets:update` for `rauthy_password_policy`. `Secrets:read` is used on every refresh of a confidential client, `Secrets:update` only when rotating a secret.

Granting `ApiKeys` makes the configured key able to mint further keys with any rights at all, including rights it does not itself hold. Grant it only to the key that actually manages `rauthy_api_key` resources.
- `url` (String) Base URL of the Rauthy instance, for example `https://auth.example.com`. The `/auth/v1` API path is appended by the provider. Falls back to the `RAUTHY_URL` environment variable.

Give the address Rauthy answers on directly, without a query or fragment: the provider does not follow redirects, so that the API key is never sent anywhere else, and a URL that redirects (from `http://` to `https://`, say) fails with an error naming the target.
