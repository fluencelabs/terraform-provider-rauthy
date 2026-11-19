# Changelog

## [0.4.1](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.4.0...v0.4.1) (2026-11-18)


### Bug Fixes

* accept identical user attribute list entries ([9bf13ef](https://github.com/fluencelabs/terraform-provider-rauthy/commit/9bf13ef9a7684b3dc914725adb56c81445e9afeb))
* **client:** a public client must keep S256 among its PKCE challenges ([c57f5de](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c57f5de8af42de0f558e05ea14840cc04c732562))
* **client:** a public client must keep S256 among its PKCE challenges ([18324fc](https://github.com/fluencelabs/terraform-provider-rauthy/commit/18324fc73828abe29f33dbb267302d9b2149a206))
* **client:** allow removing the rotation trigger when becoming public ([064c05f](https://github.com/fluencelabs/terraform-provider-rauthy/commit/064c05f7f3b2778425b0b7d2697f6949baef8455))
* **client:** leave challenges open when confidential is only known at apply ([4719fe7](https://github.com/fluencelabs/terraform-provider-rauthy/commit/4719fe7e93c5a19d9b1d3d6135fd65ec34f92cc3))
* **client:** plan S256 for a public client that leaves challenges out ([bec9076](https://github.com/fluencelabs/terraform-provider-rauthy/commit/bec9076548d93c22ebdbdbb6a421a0810ef70fb0))
* **client:** say that Rauthy stored the client when only its SCIM sync failed ([705f45b](https://github.com/fluencelabs/terraform-provider-rauthy/commit/705f45b899784f9df72aa7a316d43193970b6b58))
* converge on values Rauthy normalizes and keep state on partial failures ([e18a8e1](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e18a8e1f9453fb42f5f6d148dc3b1f3ab4f88c8a))
* defer access group checks for unknown rights ([0a77db0](https://github.com/fluencelabs/terraform-provider-rauthy/commit/0a77db03692dddb6879924a259e7d822f1e8393a))
* detect missing logos and enforce provider checks ([9cb2eb5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/9cb2eb5c9f523a98a2bc414193adf05196dfa077))
* distinguish user attribute list failures ([7dff279](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7dff279fd766332a121b9bbd2709660cb5a37e1b))
* guard substring deletions during plan using current names ([56b1cae](https://github.com/fluencelabs/terraform-provider-rauthy/commit/56b1cae9c933be64ab6eab0f28f068d1f6ed0833))
* guard user attribute deletion against scope mapping corruption ([88ff5cc](https://github.com/fluencelabs/terraform-provider-rauthy/commit/88ff5ccde7cb4ae6d6afa54d15706de482e460bb))
* keep a changed logo managed, match Rauthy's JSON numbers, keep PAM state on a failed read ([ed00dae](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ed00daea8be8430b6cbed4552ee739dd29db4be2))
* keep state true after a SCIM sync failure; require both Rauthy versions in CI ([7ef6e47](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7ef6e47c34198d06f7b6430a714e3bc6211dc13c))
* lowercase emails the way Rust does before comparing ([5766d5f](https://github.com/fluencelabs/terraform-provider-rauthy/commit/5766d5fa7036272d26ce8bb059c494359c089a6f))
* lowercase emails with x/text/cases, as Rust's to_lowercase does ([176afb3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/176afb3f220519ceb5f8bc8532eab12ef287fdc3))
* make protected role recovery advice actionable ([a71e376](https://github.com/fluencelabs/terraform-provider-rauthy/commit/a71e376a51f325d2f1d3f53f48d7103edfc9c107))
* match exact Rauthy responses for PAM group hints ([b293f66](https://github.com/fluencelabs/terraform-provider-rauthy/commit/b293f66ce396add9ff04dbf883bb8496e77f9ef3))
* never let a rename rewrite unrelated scopes, roles or groups ([206e845](https://github.com/fluencelabs/terraform-provider-rauthy/commit/206e845e95213f513ce4fba4f4c68a2516f40d2d))
* never let a rename rewrite unrelated scopes, roles or groups ([6553912](https://github.com/fluencelabs/terraform-provider-rauthy/commit/65539128405ef166c0b9492f7585d640382282b0))
* one email rule for users, PAM users and the user lookup; keep PAM user groups across plans ([f677a21](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f677a21f3f80b6c7b3017801ca377425db4fc8b9))
* **pam_host:** describe local_password_only as Rauthy applies it ([aedad27](https://github.com/fluencelabs/terraform-provider-rauthy/commit/aedad27a8637fabb3358481240e24a4e984f78c9))
* **pam_host:** describe local_password_only as Rauthy applies it ([3772b68](https://github.com/fluencelabs/terraform-provider-rauthy/commit/3772b68ea47d99d7ef30f55843a89a30fb0dd718))
* **pam:** document replacing a group in use with create_before_destroy ([60fdf17](https://github.com/fluencelabs/terraform-provider-rauthy/commit/60fdf176f9986a181f33c81cbb7e855478cf6461))
* **pam:** document replacing a group in use with create_before_destroy ([f21d832](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f21d832a5aaf8dbc0178e06bbee12ba7d6668cbe))
* **pam:** hint on every refused group delete; document type changes and user memberships ([2a71d8b](https://github.com/fluencelabs/terraform-provider-rauthy/commit/2a71d8b9e018ff896753d3dade00256853a524dc))
* **pam:** hint on SQLite's foreign key error code that Rauthy actually returns ([15419b3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/15419b3eed76a96ba887b4c47b9a7c4d549fdcb0))
* **pam:** hint only on host references; name a taken group name on create ([bf51c90](https://github.com/fluencelabs/terraform-provider-rauthy/commit/bf51c906e21c00c520105946c6e17b3b93a92e3b))
* **pam:** read hosts once and report the actual delete failure ([4042163](https://github.com/fluencelabs/terraform-provider-rauthy/commit/40421633fac1ba82d403ba2a136e8ead71e2dbae))
* **pam:** recognise Rauthy's own errors for group hints ([a29b517](https://github.com/fluencelabs/terraform-provider-rauthy/commit/a29b517f9d1eb2900da7405165c9ff31fa072dd4))
* persist successful writes before fallible follow-up reads ([67bafbb](https://github.com/fluencelabs/terraform-provider-rauthy/commit/67bafbb5f51a6d9717b8f662cb0774fb2729e867))
* preserve planned values across Rauthy normalization ([6f53a18](https://github.com/fluencelabs/terraform-provider-rauthy/commit/6f53a180618d764a309e7d5e9e9094aaf7d87dde))
* preserve user attribute state and defer unknown name checks ([c8ae034](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c8ae0348d9880dac10b8ba15cd7efb0b4a8a2084))
* preserve user attribute state on list errors ([4435f03](https://github.com/fluencelabs/terraform-provider-rauthy/commit/4435f037fd0635db7797174d88e15393c47bef3b))
* qualify PAM group recovery hints with Rauthy errors ([8238b17](https://github.com/fluencelabs/terraform-provider-rauthy/commit/8238b17df936d23516558c7bd92aa2720a37f681))
* refresh client state after failed SCIM sync ([8f99a08](https://github.com/fluencelabs/terraform-provider-rauthy/commit/8f99a085ff5042bf99c8a86fe29652df1aefa999))
* refuse a deletion only when another name starts with this one ([d29b046](https://github.com/fluencelabs/terraform-provider-rauthy/commit/d29b046e8338757ffbabe0d3e4a8110cb7e31db5))
* refuse a deletion when another name starts or ends with this one ([c098a81](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c098a81f67677085db557a149ff1a4d5e1d6662e))
* refuse commas where Rauthy stores comma-separated lists ([6033fc3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/6033fc305849ec4865d59224c08af372bf17acfe))
* refuse duplicate API key access groups and half claim pairs; plan unknown SCIM ([87e3904](https://github.com/fluencelabs/terraform-provider-rauthy/commit/87e3904a7e828094b859214415bfc6767483b73c))
* refuse empty contacts; point the normalization test at its PAM group ([19180fe](https://github.com/fluencelabs/terraform-provider-rauthy/commit/19180fef5d69ccbc082332f812ca2cf17102f354))
* refuse role and group renames and substring-unsafe deletions ([75196e9](https://github.com/fluencelabs/terraform-provider-rauthy/commit/75196e95363dabc167b0ebe6328b2523de1203b4))
* refuse unsafe scope, role and group deletes at plan, by current name ([8bf1b35](https://github.com/fluencelabs/terraform-provider-rauthy/commit/8bf1b35ce1e5ec7996c3143354d142431e4c28cd))
* refuse uppercase non-ASCII in emails instead of emulating Rust lowercasing ([3255d40](https://github.com/fluencelabs/terraform-provider-rauthy/commit/3255d407a25368b5f7d39f04c59eeac0e866a926))
* reject malformed user attribute lists ([deff45e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/deff45ed8fe59dbc1a3f2e8db3a7d9ef218e7efd))
* require consistent lists to confirm user attribute absence ([93efce9](https://github.com/fluencelabs/terraform-provider-rauthy/commit/93efce9a08f6edf13b7e80637f2678ebec71b2aa))
* retain applied user attribute state on readback failure ([b087805](https://github.com/fluencelabs/terraform-provider-rauthy/commit/b0878052010c16d9e3efd944fd15edb0a8fd3965))
* retain managed image and PAM state on drift and read errors ([0863ca3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/0863ca339d2cfb250d3fbc4cb7377d7593275bd0))
* serialize substring guards with conflicting provider writes ([ea6c09d](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ea6c09da99aa5755cb235119dbe6d8fddcc25962))
* take client constraints from rauthy-go v0.2.0 ([ad77e32](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ad77e329a1c6f304c3898f887804c3114305084c))
* take client constraints from rauthy-go v0.2.0 ([ed81618](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ed816186b4d63b76fc6cae843ffc1c278b698207))
* tolerate unnamed user attributes in lists ([4113ac5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/4113ac53990bbb06def4b239830563b4b3253445))
* **user_attribute:** keep the applied value after a successful PUT; isolate conflicting duplicates ([9a0740b](https://github.com/fluencelabs/terraform-provider-rauthy/commit/9a0740b25e5ab4d594a9b711a911c51de8d4d06a))
* **user_attribute:** keep the attribute managed when Rauthy's list misses it; plan unknown unchanged names ([39760fe](https://github.com/fluencelabs/terraform-provider-rauthy/commit/39760feb468aded241444535fea36c4259d8ec03))
* **user_attribute:** never rewrite another attribute's scope mapping ([46fc02a](https://github.com/fluencelabs/terraform-provider-rauthy/commit/46fc02aa68c8fe83d2eed553f823327fff867656))
* **user_attribute:** report list failures precisely; pin list handling in tests ([02cc76a](https://github.com/fluencelabs/terraform-provider-rauthy/commit/02cc76a5cb48273b822a163e58ea2ccf2ca4934e))
* **user_attribute:** treat a 404 from the attribute list as an API error ([1deb3ec](https://github.com/fluencelabs/terraform-provider-rauthy/commit/1deb3ec515461515c745aa23306132ddef66311c))
* **user_attribute:** trust absence only from a consistent list ([7eb7514](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7eb7514d4dc3137e8b16cc164cbccd0dc2944178))
* **user-attribute:** bound decimal magnitude so the second storage round trip is exact ([7be0cbc](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7be0cbc4b6246d69343aa83887e56dc32acf09d9))
* **user-attribute:** reject unstable JSON numbers before apply ([40bce76](https://github.com/fluencelabs/terraform-provider-rauthy/commit/40bce7662cd2ba48b939834c2915b82965cabdbb))
* **user:** keep profile and password on unrelated updates ([693b2c9](https://github.com/fluencelabs/terraform-provider-rauthy/commit/693b2c945a4a52a8b46314fd11ab550b0a81962a))
* **user:** keep profile and password on unrelated updates ([290070e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/290070edad0ba6af0967dff896f9a23286c03b2b))
* validate access grants and upstream claim mappings ([6f1d6e2](https://github.com/fluencelabs/terraform-provider-rauthy/commit/6f1d6e27e165c2a58af38395473fd25c17fec99b))
* validate user attribute lists before changing state ([35b9d64](https://github.com/fluencelabs/terraform-provider-rauthy/commit/35b9d642c34ab5b9e6a3e40fb456488cb2fff6cc))
* **validators:** bound attribute names to 32 and refuse an empty zip ([27326dc](https://github.com/fluencelabs/terraform-provider-rauthy/commit/27326dc7bfa8ceaf5fca2f48148a04b294b396ad))

## [0.4.0](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.3.0...v0.4.0) (2026-11-15)


### Features

* **scope:** manage claims_at_root ([f3fb709](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f3fb70999f9576f6e9be8c781088aa2b19f840e2))


### Bug Fixes

* **api-key:** say a failed rotation may have reached Rauthy ([7618d19](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7618d19d3b5c3f10a78c882466a27a5d8536e6d1))
* **auth-provider:** report an ambiguous name as such ([8a235a3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/8a235a335e64a7381434f416d630fea2093b8132))
* **client:** keep the settings rauthy_client does not model ([cb5e7d9](https://github.com/fluencelabs/terraform-provider-rauthy/commit/cb5e7d912332ef25cbaa3d5661512e148f83fcb6))
* **provider:** refuse redirects and query URLs, never re-send a rotation ([e743559](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e743559978b9461854cd0331bd43dcc4e75bb034))
* **scope:** never send a claims_at_root the plan does not know ([7158e66](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7158e6631f694e1c25f5d5b7664f6acb8e87918f))
* **scope:** plan claims_at_root unknown when state has none ([cf0726e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/cf0726eda293897ce0cce7936cdbc06fa5b2db2f))

## [0.3.0](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.2.0...v0.3.0) (2026-08-30)

This release roughly triples the provider's coverage of Rauthy and tracks Rauthy 0.36.2. It contains two changes that can break an existing configuration; both are listed first.

### Breaking changes

* **Terraform 1.11 or newer is now required.** The provider uses write-only attributes, which older Terraform does not understand. Configurations on Terraform 1.10 and below will fail to load the provider.
* **Client ids are validated more strictly.** Rauthy 0.36 narrowed its own rule for `rauthy_client.id` to `^[a-zA-Z0-9._\-]{2,256}$`, and the provider now matches it. An id containing `/`, `:`, `%` or other punctuation that used to pass validation is now rejected at plan time. Such ids are already refused by any Rauthy 0.36 server, so this moves the failure from apply to plan rather than introducing one — but a plan that used to succeed may now stop.

### Secrets in state

Supplied secrets no longer sit in Terraform state: `rauthy_user.password` and `rauthy_auth_provider.client_secret` are replaced by the write-only `password_wo` and `client_secret_wo`, each with a rotation trigger. Because a write-only value is invisible to the plan, the trigger is what makes an update happen at all.

Two secrets deliberately remain in state. `rauthy_client.secret` and `rauthy_api_key.secret` are generated by Rauthy and exist to be passed onward to another configuration or an output; a write-only attribute only carries values *into* the provider, so converting them would remove the feature rather than protect it. The provider documentation now says so explicitly, and a state file holding either must be treated as a credential store.

### New resources and data sources

* `rauthy_user` and its data source, plus `rauthy_user_attribute` and its data source — custom attributes can now be created by Terraform and referenced from `rauthy_scope`, which previously required creating them by hand.
* `rauthy_auth_provider` for upstream federated identity providers, with a lookup data source and `rauthy_auth_provider_lookup` for OIDC discovery.
* `rauthy_api_key` for Rauthy's own admin API keys, including secret rotation. This is possible only from Rauthy 0.36, which added the `ApiKeys` access group.
* `rauthy_pam_group`, `rauthy_pam_user` and `rauthy_pam_host`, covering Rauthy as an authentication source for hosts and SSH.
* `rauthy_client_logo` and `rauthy_client_favicon` for per-client branding.
* `rauthy_blacklist_ip` for blocking an address until an expiry.
* `rauthy_scope` and its data source, and a `rauthy_client` data source, completing the lookup coverage.

### Other changes

* Rauthy 0.36.2 is now the pinned and tested version, and the vendored OpenAPI document was refreshed to match.
* The API client retries transient failures — connection errors, 429 and 5xx — for idempotent requests. Creating `POST` requests are deliberately not retried past the dial, so a lost response cannot duplicate a resource.
* The acceptance suite runs against a live Rauthy in CI, on both the pinned version and the latest release.

### Bug fixes

* `rauthy_password_policy` could not be refreshed or imported ([c43e243](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c43e243590bfedcde03e016a189243a252a770d0))
* An empty attribute mapping on `rauthy_scope` is kept distinct from an absent one ([e9355b5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e9355b58e168967318257367d8145f88bdd89f4e))
* `rauthy_user` required `given_name`, which Rauthy demands on update despite documenting it as optional ([f708891](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f708891267dbb224b63d60f1e7179ee235f7cca4))
* The acceptance CI job can no longer pass while testing nothing ([561e6b5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/561e6b5f1d2e0d9533854062de353dacf2d56b87))

## [0.2.0](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.1.1...v0.2.0) (2026-08-29)


### Features

* add role, group and password policy resources ([db2c97d](https://github.com/fluencelabs/terraform-provider-rauthy/commit/db2c97d86669d7315e484a8064efbd42b3c173c4))
* add role, group and password policy resources ([c6f45df](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c6f45df4550b43fa515be18bf3c2d877c9697832))


### Bug Fixes

* address review findings on the new resources ([9f2efd1](https://github.com/fluencelabs/terraform-provider-rauthy/commit/9f2efd1870d182968642cab00c37b141c9043673))
* make the lint config and int32 narrowing version-proof ([f4d461c](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f4d461cd0166c85e1041c8b4b1946cdfc0024b4c))

## [0.1.1](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.1.0...v0.1.1) (2026-08-28)


### Bug Fixes

* include the registry manifest in SHA256SUMS ([ce3005b](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ce3005b7fe44c14b8516c9e2331bc8befdd922ea))
* include the registry manifest in SHA256SUMS ([486d86e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/486d86e2fec2aaeff9d84e1e9849fe3be485b59b))

## 0.1.0 (2026-08-28)


### Features

* add provider skeleton and Rauthy API client ([1bcd8a4](https://github.com/fluencelabs/terraform-provider-rauthy/commit/1bcd8a4b6563f319f8b91e99fd39c8219b5bd5b6))
* add the rauthy_client resource ([b01c82d](https://github.com/fluencelabs/terraform-provider-rauthy/commit/b01c82d99a2bd68fcfc8ad57897ad90e4cd07331))
* Terraform provider for Rauthy OIDC clients ([c9c0e5c](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c9c0e5c338adf9da472f505184b43c35086a52f3))


### Bug Fixes

* cut 0.1.0 as the first release, not 1.0.0 ([b0cf3fd](https://github.com/fluencelabs/terraform-provider-rauthy/commit/b0cf3fde6b13705604ec82217a68e83fc3d3b064))
* cut 0.1.0 as the first release, not 1.0.0 ([d4b3f5c](https://github.com/fluencelabs/terraform-provider-rauthy/commit/d4b3f5cda8a464a452cf15fbeba83aef3422f98d))
