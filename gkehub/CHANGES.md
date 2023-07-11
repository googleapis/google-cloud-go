# Changes

## [0.14.1](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.14.0...gkehub/v0.14.1) (2023-06-20)


### Bug Fixes

* **gkehub:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [0.14.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.13.1...gkehub/v0.14.0) (2023-05-30)


### Features

* **gkehub:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [0.13.1](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.13.0...gkehub/v0.13.1) (2023-05-08)


### Bug Fixes

* **gkehub:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [0.13.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.12.0...gkehub/v0.13.0) (2023-04-25)


### Features

* **gkehub:** Add `monitoring_config` field ([#7806](https://github.com/googleapis/google-cloud-go/issues/7806)) ([e1e8ba9](https://github.com/googleapis/google-cloud-go/commit/e1e8ba9f4d427c52c4a2bc949479055824124ba0))

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.11.0...gkehub/v0.12.0) (2023-03-15)


### Features

* **gkehub:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.10.0...gkehub/v0.11.0) (2023-01-04)


### Features

* **gkehub:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.9.0...gkehub/v0.10.0) (2022-09-21)


### Features

* **gkehub:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.8.0...gkehub/v0.9.0) (2022-09-19)


### Features

* **gkehub:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.7.0...gkehub/v0.8.0) (2022-06-29)


### Features

* **gkehub:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.6.0...gkehub/v0.7.0) (2022-06-16)


### Features

* **gkehub:** add ClusterType field in MembershipEndpoint.OnPremCluster ([5e46068](https://github.com/googleapis/google-cloud-go/commit/5e46068329153daf5aa590a6415d4764f1ab2b90))
* **gkehub:** Added support for locations and iam_policy clients ([4134941](https://github.com/googleapis/google-cloud-go/commit/41349411e601f57dc6d9e246f1748fd86d17bb15))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.5.0...gkehub/v0.6.0) (2022-06-01)


### Features

* **gkehub:** add EdgeCluster as a new membershipEndpoint type feat: add ApplianceCluster as a new membershipEndpoint type feat: add c++ rules in BUILD file doc: add API annotations doc: minor changes on code and doc format ([02cbe4b](https://github.com/googleapis/google-cloud-go/commit/02cbe4bec42b3389d64d1e78396b3f6a8e4976ba))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.4.0...gkehub/v0.5.0) (2022-03-14)


### Features

* **gkehub:** added support for k8s_version field docs: k8s_version field is not part of resource_options struct ([a4f8273](https://github.com/googleapis/google-cloud-go/commit/a4f8273697a888473689db9b887298c74e0aebf3))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.3.0...gkehub/v0.4.0) (2022-02-23)


### Features

* **gkehub:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/gkehub/v0.2.0...gkehub/v0.3.0) (2022-02-14)


### Features

* **gkehub:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/gkehub/v0.1.0...gkehub/v0.2.0) (2021-08-30)


### Features

* **gkehub:** Add request_id under `DeleteMembershipRequest` and `UpdateMembershipRequest` ([b9226eb](https://www.github.com/googleapis/google-cloud-go/commit/b9226eb0b34473cb6f920c2526ad0d6dacb03f3c))

## v0.1.0

This is the first tag to carve out gkehub as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
