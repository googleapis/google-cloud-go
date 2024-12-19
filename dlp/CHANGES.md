# Changes

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.19.0...dlp/v1.20.0) (2024-10-23)


### Features

* **dlp:** Discovery of BigQuery snapshots ([70d82fe](https://github.com/googleapis/google-cloud-go/commit/70d82fe93f60f1075298a077ce1616f9ae7e13fe))


### Bug Fixes

* **dlp:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **dlp:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))


### Documentation

* **dlp:** Documentation revisions for data profiles ([70d82fe](https://github.com/googleapis/google-cloud-go/commit/70d82fe93f60f1075298a077ce1616f9ae7e13fe))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.18.1...dlp/v1.19.0) (2024-09-19)


### Features

* **dlp:** Action for publishing data profiles to SecOps (formelly known as Chronicle) ([#10884](https://github.com/googleapis/google-cloud-go/issues/10884)) ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))
* **dlp:** Action for publishing data profiles to Security Command Center ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))
* **dlp:** Discovery configs for AWS S3 buckets ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))


### Documentation

* **dlp:** Small improvements and clarifications ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.18.0...dlp/v1.18.1) (2024-09-12)


### Bug Fixes

* **dlp:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.17.0...dlp/v1.18.0) (2024-08-20)


### Features

* **dlp:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))
* **dlp:** File store data profiles can now be filtered by type and storage location ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))
* **dlp:** Inspect template modified cadence discovery config for Cloud SQL ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))


### Documentation

* **dlp:** Small improvements ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.16.0...dlp/v1.17.0) (2024-08-08)


### Features

* **dlp:** Add the TagResources API ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))


### Bug Fixes

* **dlp:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.15.0...dlp/v1.16.0) (2024-08-01)


### Features

* **dlp:** Add refresh frequency for data profiling ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **dlp:** GRPC config for get, list, and delete FileStoreDataProfiles ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **dlp:** Org-level connection bindings ([#10601](https://github.com/googleapis/google-cloud-go/issues/10601)) ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))


### Documentation

* **dlp:** Replace HTML tags with CommonMark notation ([#10613](https://github.com/googleapis/google-cloud-go/issues/10613)) ([d949cc0](https://github.com/googleapis/google-cloud-go/commit/d949cc0e5d44af62154d9d5fd393f25a852f93ed))
* **dlp:** Small improvements ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.14.3...dlp/v1.15.0) (2024-07-24)


### Features

* **dlp:** Add Cloud Storage discovery support ([#10527](https://github.com/googleapis/google-cloud-go/issues/10527)) ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))


### Bug Fixes

* **dlp:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **dlp:** Updated method documentation ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.14.2...dlp/v1.14.3) (2024-07-10)


### Bug Fixes

* **dlp:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.14.1...dlp/v1.14.2) (2024-07-01)


### Bug Fixes

* **dlp:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.14.0...dlp/v1.14.1) (2024-06-26)


### Bug Fixes

* **dlp:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.13.0...dlp/v1.14.0) (2024-05-29)


### Features

* **dlp:** Add secrets discovery support ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))


### Documentation

* **dlp:** Updated method documentation ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.12.2...dlp/v1.13.0) (2024-05-08)


### Features

* **dlp:** Add RPCs for deleting TableDataProfiles ([3e25053](https://github.com/googleapis/google-cloud-go/commit/3e250530567ee81ed4f51a3856c5940dbec35289))

## [1.12.2](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.12.1...dlp/v1.12.2) (2024-05-01)


### Bug Fixes

* **dlp:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.12.0...dlp/v1.12.1) (2024-03-14)


### Bug Fixes

* **dlp:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.11.2...dlp/v1.12.0) (2024-03-07)


### Features

* **dlp:** Add RPCs for getting and listing project, table, and column data profiles ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.11.1...dlp/v1.11.2) (2024-01-30)


### Bug Fixes

* **dlp:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.11.0...dlp/v1.11.1) (2023-11-01)


### Bug Fixes

* **dlp:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.10.3...dlp/v1.11.0) (2023-10-31)


### Features

* **dlp:** Introduce Discovery API protos and methods ([ffb0dda](https://github.com/googleapis/google-cloud-go/commit/ffb0ddabf3d9822ba8120cabaf25515fd32e9615))

## [1.10.3](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.10.2...dlp/v1.10.3) (2023-10-26)


### Bug Fixes

* **dlp:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.10.2](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.10.1...dlp/v1.10.2) (2023-10-12)


### Bug Fixes

* **dlp:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.10.0...dlp/v1.10.1) (2023-06-20)


### Bug Fixes

* **dlp:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.9.1...dlp/v1.10.0) (2023-05-30)


### Features

* **dlp:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.9.0...dlp/v1.9.1) (2023-05-08)


### Bug Fixes

* **dlp:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.8.0...dlp/v1.9.0) (2023-01-04)


### Features

* **dlp:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.7.0...dlp/v1.8.0) (2022-11-16)


### Features

* **dlp:** ExcludeByHotword added as an ExclusionRule type, NEW_ZEALAND added as a LocationCategory value ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.6.0...dlp/v1.7.0) (2022-11-03)


### Features

* **dlp:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.5.1...dlp/v1.6.0) (2022-10-25)


### Features

* **dlp:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.5.1](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.5.0...dlp/v1.5.1) (2022-10-14)


### Bug Fixes

* **dlp:** deprecate extra field to avoid confusion ([8388f87](https://github.com/googleapis/google-cloud-go/commit/8388f877b5682c96e9476863ca761b975cbe4131))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.4.0...dlp/v1.5.0) (2022-09-15)


### Features

* **dlp:** add Deidentify action ([6a0080a](https://github.com/googleapis/google-cloud-go/commit/6a0080ad69398c572d856886293e19c79cf0fc0e))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.3.0...dlp/v1.4.0) (2022-04-06)


### Features

* **dlp:** add DataProfilePubSubMessage supporting pub/sub integration ([57896d1](https://github.com/googleapis/google-cloud-go/commit/57896d1491c04fa53d3f3e2344ef10c3d91c4b65))
* **dlp:** new Bytes and File types: POWERPOINT and EXCEL ([81c4c91](https://github.com/googleapis/google-cloud-go/commit/81c4c9116178711059772f41bbf76d423ffebc95))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.2.0...dlp/v1.3.0) (2022-02-23)


### Features

* **dlp:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/dlp/v1.1.0...dlp/v1.2.0) (2022-02-14)


### Features

* **dlp:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/dlp/v1.0.0...dlp/v1.1.0) (2022-01-04)


### Features

* **dlp:** added deidentify replacement dictionaries feat: added field for BigQuery inspect template inclusion lists feat: added field to support infotype versioning ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out dlp as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
