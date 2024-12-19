# Changes


## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/functions/v1.19.1...functions/v1.19.2) (2024-10-23)


### Bug Fixes

* **functions:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **functions:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/functions/v1.19.0...functions/v1.19.1) (2024-09-12)


### Bug Fixes

* **functions:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.18.0...functions/v1.19.0) (2024-08-20)


### Features

* **functions:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.17.0...functions/v1.18.0) (2024-08-13)


### Features

* **functions:** Additional field on the output that specified whether the deployment supports Physical Zone Separation. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Generate upload URL now supports for specifying the GCF generation that the generated upload url will be used for. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** ListRuntimes response now includes deprecation and decommissioning dates. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Optional field for binary authorization policy. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Optional field for deploying a source from a GitHub repository. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Optional field for specifying a revision on GetFunction. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Optional field for specifying a service account to use for the build. This helps navigate the change of historical default on new projects. For more details, see https ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))
* **functions:** Optional fields for setting up automatic base image updates. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))


### Documentation

* **functions:** Refined description in several fields. ([6593c0d](https://github.com/googleapis/google-cloud-go/commit/6593c0d62d48751c857bce3d3f858127467a4489))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.6...functions/v1.17.0) (2024-08-08)


### Features

* **functions:** Added `build_service_account` field to CloudFunction ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Additional field on the output that specified whether the deployment supports Physical Zone Separation. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Generate upload URL now supports for specifying the GCF generation that the generated upload url will be used for. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** ListRuntimes response now includes deprecation and decommissioning dates. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Optional field for binary authorization policy. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Optional field for deploying a source from a GitHub repository. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Optional field for specifying a revision on GetFunction. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Optional field for specifying a service account to use for the build. This helps navigate the change of historical default on new projects. For more details, see https ([#10650](https://github.com/googleapis/google-cloud-go/issues/10650)) ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Optional fields for setting up automatic base image updates. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))


### Bug Fixes

* **functions:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))


### Documentation

* **functions:** A comment for field `automatic_update_policy` in message `.google.cloud.functions.v1.CloudFunction` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** A comment for field `docker_repository` in message `.google.cloud.functions.v1.CloudFunction` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** A comment for field `on_deploy_update_policy` in message `.google.cloud.functions.v1.CloudFunction` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** A comment for field `runtime_version` in message `.google.cloud.functions.v1.CloudFunction` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** A comment for field `url` in message `.google.cloud.functions.v1.HttpsTrigger` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** A comment for field `url` in message `.google.cloud.functions.v1.SourceRepository` is changed ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))
* **functions:** Refined description in several fields. ([9a5144e](https://github.com/googleapis/google-cloud-go/commit/9a5144e7d30c6f058b13fdf3fd9436904e77dff0))

## [1.16.6](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.5...functions/v1.16.6) (2024-07-24)


### Bug Fixes

* **functions:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.16.5](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.4...functions/v1.16.5) (2024-07-10)


### Bug Fixes

* **functions:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.16.4](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.3...functions/v1.16.4) (2024-07-01)


### Bug Fixes

* **functions:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.16.3](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.2...functions/v1.16.3) (2024-06-26)


### Bug Fixes

* **functions:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.16.2](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.1...functions/v1.16.2) (2024-05-01)


### Bug Fixes

* **functions:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.16.1](https://github.com/googleapis/google-cloud-go/compare/functions/v1.16.0...functions/v1.16.1) (2024-03-14)


### Bug Fixes

* **functions:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.15.4...functions/v1.16.0) (2024-01-30)


### Features

* **functions:** Updated description for `docker_registry` to reflect transition to Artifact Registry ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))


### Bug Fixes

* **functions:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.15.4](https://github.com/googleapis/google-cloud-go/compare/functions/v1.15.3...functions/v1.15.4) (2023-11-01)


### Bug Fixes

* **functions:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.15.3](https://github.com/googleapis/google-cloud-go/compare/functions/v1.15.2...functions/v1.15.3) (2023-10-26)


### Bug Fixes

* **functions:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.15.2](https://github.com/googleapis/google-cloud-go/compare/functions/v1.15.1...functions/v1.15.2) (2023-10-12)


### Bug Fixes

* **functions:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.15.1](https://github.com/googleapis/google-cloud-go/compare/functions/v1.15.0...functions/v1.15.1) (2023-06-20)


### Bug Fixes

* **functions:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.14.0...functions/v1.15.0) (2023-05-30)


### Features

* **functions:** ListFunctions now include metadata which indicates whether a function is a `GEN_1` or `GEN_2` function ([ca94e27](https://github.com/googleapis/google-cloud-go/commit/ca94e2724f9e2610b46aefd0a3b5ddc06102e91b))
* **functions:** ListFunctions now include metadata which indicates whether a function is a `GEN_1` or `GEN_2` function ([#7984](https://github.com/googleapis/google-cloud-go/issues/7984)) ([ca94e27](https://github.com/googleapis/google-cloud-go/commit/ca94e2724f9e2610b46aefd0a3b5ddc06102e91b))
* **functions:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.13.1...functions/v1.14.0) (2023-05-16)


### Features

* **functions:** Added helper methods for long running operations, IAM, and locations ([31421d5](https://github.com/googleapis/google-cloud-go/commit/31421d52c3bf3b7baa235fb6cb18bb8a786398df))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/functions/v1.13.0...functions/v1.13.1) (2023-05-08)


### Bug Fixes

* **functions:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.12.0...functions/v1.13.0) (2023-04-04)


### Features

* **functions:** Promote v2 to GA ([#7642](https://github.com/googleapis/google-cloud-go/issues/7642)) ([e68abb2](https://github.com/googleapis/google-cloud-go/commit/e68abb2236a4f653ec3723ae2f83e8ccf2dff8ae))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.11.0...functions/v1.12.0) (2023-03-22)


### Features

* **functions:** Add `available_cpu ` field ([499b489](https://github.com/googleapis/google-cloud-go/commit/499b489d8d6bc8db203c864db97f1462bbeff3d2))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.10.0...functions/v1.11.0) (2023-03-15)


### Features

* **functions:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.9.0...functions/v1.10.0) (2023-01-04)


### Features

* **functions:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.8.0...functions/v1.9.0) (2022-11-03)


### Features

* **functions:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.7.0...functions/v1.8.0) (2022-10-25)


### Features

* **functions:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))
* **functions:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.6.0...functions/v1.7.0) (2022-09-21)


### Features

* **functions:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.5.0...functions/v1.6.0) (2022-09-19)


### Features

* **functions:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))


### Documentation

* **functions:** Update metadata.Resource docs ([#6660](https://github.com/googleapis/google-cloud-go/issues/6660)) ([ad01de9](https://github.com/googleapis/google-cloud-go/commit/ad01de9aa1fd2fcc087cab5e43ee2e2853c55bb3)), refs [#6612](https://github.com/googleapis/google-cloud-go/issues/6612)

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.4.0...functions/v1.5.0) (2022-07-12)


### Features

* **functions:** add apiv2beta REST client ([#6291](https://github.com/googleapis/google-cloud-go/issues/6291)) ([f53363a](https://github.com/googleapis/google-cloud-go/commit/f53363a8d52960721206932bd5d838df7db8418f))
* **functions:** start generating apiv2 ([#6323](https://github.com/googleapis/google-cloud-go/issues/6323)) ([49f9549](https://github.com/googleapis/google-cloud-go/commit/49f95499f87a38e4917f8cc1f3ec435d6614d2c2))
* **functions:** start generating apiv2beta ([#6280](https://github.com/googleapis/google-cloud-go/issues/6280)) ([86c8bff](https://github.com/googleapis/google-cloud-go/commit/86c8bff34ce27b8090f567c8714c1237cbd490d1))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.3.0...functions/v1.4.0) (2022-06-16)


### Features

* **functions:** added support for CMEK docs: clarified wording around quota usage ([4134941](https://github.com/googleapis/google-cloud-go/commit/41349411e601f57dc6d9e246f1748fd86d17bb15))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.2.0...functions/v1.3.0) (2022-02-23)


### Features

* **functions:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/functions/v1.1.0...functions/v1.2.0) (2022-02-14)


### Features

* **functions:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/functions/v1.0.0...functions/v1.1.0) (2022-01-04)


### Features

* **functions:** Secret Manager integration fields 'secret_environment_variables' and 'secret_volumes' added feat: CMEK integration fields 'kms_key_name' and 'docker_repository' added ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/functions/v0.1.0...functions/v0.2.0) (2021-09-16)


### Features

* **functions:** add SecurityLevel option on HttpsTrigger ([8ffed36](https://www.github.com/googleapis/google-cloud-go/commit/8ffed36c9db818a24073cf865f626d29afd01716))

## v0.1.0

This is the first tag to carve out functions as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
