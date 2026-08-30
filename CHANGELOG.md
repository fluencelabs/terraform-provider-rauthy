# Changelog

## [0.3.0](https://github.com/fluencelabs/terraform-provider-rauthy/compare/v0.2.0...v0.3.0) (2026-08-30)


### Features

* add rauthy_api_key for managing Rauthy's own admin API keys ([5835a07](https://github.com/fluencelabs/terraform-provider-rauthy/commit/5835a079e16250b7fe9e2e1c9f85b87c387ae09c))
* add rauthy_blacklist_ip ([88b0b81](https://github.com/fluencelabs/terraform-provider-rauthy/commit/88b0b81dda6f3dee5f4c614cf04e037d2a732852))
* add the rauthy_api_key resource ([5a8085e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/5a8085e9cca09f7473575a476da05dffeed2b9d4))
* add the rauthy_auth_provider resource and data sources ([552ca26](https://github.com/fluencelabs/terraform-provider-rauthy/commit/552ca26fedb99243da1b379288a9341ffcb9f108))
* add the rauthy_auth_provider resource and data sources ([5cb979e](https://github.com/fluencelabs/terraform-provider-rauthy/commit/5cb979e9d411edcf521800e2094b7bcb241d409a))
* add the rauthy_blacklist_ip resource ([7085aee](https://github.com/fluencelabs/terraform-provider-rauthy/commit/7085aeec4e73914946ef5597269c510066e9eb76))
* add the rauthy_client data source ([f3db5b3](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f3db5b30d926b07134f20214c8234a1b59681887))
* add the rauthy_client data source ([e57da38](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e57da3822f73d48f03b6826f2edbc452fbe8ef01))
* add the rauthy_client_logo and rauthy_client_favicon resources ([2715f13](https://github.com/fluencelabs/terraform-provider-rauthy/commit/2715f130699e72886a6404799021e03f1fe028d6))
* add the rauthy_scope resource and data source ([de0271c](https://github.com/fluencelabs/terraform-provider-rauthy/commit/de0271c76ceb3c163c03e6e96b157801c9f8ae08))
* add the rauthy_scope resource and data source ([e59d494](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e59d494cf59eb41ce0ab92dc3676eafdbc5439f7))
* add the rauthy_user resource and data source ([554dabc](https://github.com/fluencelabs/terraform-provider-rauthy/commit/554dabc832a882e576b600652bfec8ad78d43d67))
* add the rauthy_user resource and data source ([272e2c1](https://github.com/fluencelabs/terraform-provider-rauthy/commit/272e2c1ad6ab6d0732fd744715006ba7bb4a18d0))
* add the rauthy_user_attribute resource and data source ([105cd73](https://github.com/fluencelabs/terraform-provider-rauthy/commit/105cd737c5a1c1237b947d474f7fd11662d6a5fb))
* keep supplied secrets out of Terraform state with write-only attributes ([88f8e03](https://github.com/fluencelabs/terraform-provider-rauthy/commit/88f8e03d1f8ac8f3ec67e14a721744e0aea22e14))
* manage per-client logo and favicon ([c1e076f](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c1e076f62e914599694d6df4397639b305f3a305))
* manage Rauthy's PAM subsystem ([23a0dd6](https://github.com/fluencelabs/terraform-provider-rauthy/commit/23a0dd65e26d59bdfa5ed552c6aad2f7e0b9e889))
* manage Rauthy's PAM subsystem ([c38da8d](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c38da8dd9a51b4d49110b32ccb2d3b9d9e7f00c8))
* retry transient failures in the API client ([ffb4597](https://github.com/fluencelabs/terraform-provider-rauthy/commit/ffb459778c6d9258cd1ea0d0d5f68cedfcaac532))
* run the acceptance tests against a live Rauthy in CI ([4ee81d8](https://github.com/fluencelabs/terraform-provider-rauthy/commit/4ee81d83c74c79d9d2b79b72f5a615dd4b6357ac))
* run the acceptance tests against a live Rauthy in CI ([b3f3bf6](https://github.com/fluencelabs/terraform-provider-rauthy/commit/b3f3bf6821dae73eb838b77cadb28ac5318c9ee1))
* take secrets out of state with write-only attributes ([346db71](https://github.com/fluencelabs/terraform-provider-rauthy/commit/346db71fd9ef35af2220d0af7f2cd5c9cc10c9c9))


### Bug Fixes

* keep an empty attribute mapping distinct from an absent one ([e9355b5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/e9355b58e168967318257367d8145f88bdd89f4e))
* make rauthy_user work against a live Rauthy ([f708891](https://github.com/fluencelabs/terraform-provider-rauthy/commit/f708891267dbb224b63d60f1e7179ee235f7cca4))
* make the acceptance job unable to pass while testing nothing ([561e6b5](https://github.com/fluencelabs/terraform-provider-rauthy/commit/561e6b5f1d2e0d9533854062de353dacf2d56b87))
* rauthy_password_policy could not be refreshed or imported ([29719eb](https://github.com/fluencelabs/terraform-provider-rauthy/commit/29719eb617c913d24c9c5f5f6fb057d11a692ee7))
* rauthy_password_policy could not be refreshed or imported ([c43e243](https://github.com/fluencelabs/terraform-provider-rauthy/commit/c43e243590bfedcde03e016a189243a252a770d0))

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
