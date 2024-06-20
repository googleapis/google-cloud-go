# Changes

## [2.4.2](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.4.1...dataproc/v2.4.2) (2024-05-01)


### Bug Fixes

* **dataproc:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [2.4.1](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.4.0...dataproc/v2.4.1) (2024-03-14)


### Bug Fixes

* **dataproc:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [2.4.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.3.0...dataproc/v2.4.0) (2024-01-30)


### Features

* **dataproc:** Add session and session_template controllers ([4d56af1](https://github.com/googleapis/google-cloud-go/commit/4d56af183d42ff12862c0c35226e767ed8763118))


### Bug Fixes

* **dataproc:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [2.3.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.2.3...dataproc/v2.3.0) (2023-11-09)


### Features

* **dataproc:** Support required_registration_fraction for secondary workers ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [2.2.3](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.2.2...dataproc/v2.2.3) (2023-11-01)


### Bug Fixes

* **dataproc:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [2.2.2](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.2.1...dataproc/v2.2.2) (2023-10-26)


### Bug Fixes

* **dataproc:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [2.2.1](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.2.0...dataproc/v2.2.1) (2023-10-12)


### Bug Fixes

* **dataproc:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [2.2.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.1.0...dataproc/v2.2.0) (2023-09-20)


### Features

* **dataproc:** Add optional parameters (tarball_gcs_dir, diagnosis_interval, jobs, yarn_application_ids) in DiagnoseClusterRequest ([2f3bb44](https://github.com/googleapis/google-cloud-go/commit/2f3bb443e9fa6968d20806f86b391dad85970afc))

## [2.1.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.0.2...dataproc/v2.1.0) (2023-09-11)


### Features

* **dataproc:** Support min_num_instances for primary worker and InstanceFlexibilityPolicy for secondary worker ([20725c8](https://github.com/googleapis/google-cloud-go/commit/20725c86c970ad24efa18c056fc3aa71dc3a4f03))

## [2.0.2](https://github.com/googleapis/google-cloud-go/compare/dataproc/v2.0.1...dataproc/v2.0.2) (2023-08-08)


### Documentation

* **dataproc:** Minor formatting ([b4349cc](https://github.com/googleapis/google-cloud-go/commit/b4349cc507870ff8629bbc07de578b63bb889626))

## [2.0.1](https://github.com/googleapis/google-cloud-go/compare/dataproc-v2.0.0...dataproc/v2.0.1) (2023-06-20)


### Bug Fixes

* **dataproc:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [2.0.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.12.0...dataproc/v2.0.0) (2023-04-25)


### ⚠ BREAKING CHANGES

* **dataproc:** update go_package to v2 in google.cloud.dataproc.v1
* **dataproc:** add support for new Dataproc features ([#7479](https://github.com/googleapis/google-cloud-go/issues/7479))

### Features

* **dataproc:** Add support for new Dataproc features ([#7479](https://github.com/googleapis/google-cloud-go/issues/7479)) ([0862303](https://github.com/googleapis/google-cloud-go/commit/0862303712d874f879053527d0ab183b514d0b7d))
* **dataproc:** Update go_package to v2 in google.cloud.dataproc.v1 ([87a67b4](https://github.com/googleapis/google-cloud-go/commit/87a67b44b2c7ffc3cea986b255614ea0d21aa6fc))
* **dataproc:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/dataproc-v1.11.0...dataproc/v1.12.0) (2023-01-26)


### Features

* **dataproc:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **dataproc:** Add SPOT to Preemptibility enum ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **dataproc:** Add support for Dataproc metric configuration ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **dataproc:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **dataproc:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/dataproc-v1.10.0...dataproc/v1.11.0) (2023-01-26)


### Features

* **dataproc:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **dataproc:** Add SPOT to Preemptibility enum ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **dataproc:** Add support for Dataproc metric configuration ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **dataproc:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **dataproc:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.9.0...dataproc/v1.10.0) (2023-01-26)


### Features

* **dataproc:** Add SPOT to Preemptibility enum ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.8.0...dataproc/v1.9.0) (2023-01-04)


### Features

* **dataproc:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.7.0...dataproc/v1.8.0) (2022-11-03)


### Features

* **dataproc:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.6.0...dataproc/v1.7.0) (2022-10-25)


### Features

* **dataproc:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.5.0...dataproc/v1.6.0) (2022-09-28)


### Features

* **dataproc:** add support for Dataproc metric configuration ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.4.0...dataproc/v1.5.0) (2022-02-23)


### Features

* **dataproc:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.3.0...dataproc/v1.4.0) (2022-02-22)


### Features

* **dataproc:** add support for Virtual Dataproc cluster running on GKE cluster ([7d6b0e5](https://github.com/googleapis/google-cloud-go/commit/7d6b0e5891b50cccdf77cd17ddd3644f31ef6dfc))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/dataproc/v1.2.0...dataproc/v1.3.0) (2022-02-14)


### Features

* **dataproc:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/dataproc/v1.1.0...dataproc/v1.2.0) (2021-11-02)


### Features

* **dataproc:** Add support for dataproc BatchController service ([8519b94](https://www.github.com/googleapis/google-cloud-go/commit/8519b948fee5dc82d39300c4d96e92c85fe78fe6))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/dataproc/v1.0.0...dataproc/v1.1.0) (2021-10-18)


### Features

* **dataproc:** add Dataproc Serverless for Spark Batches API ([30794e7](https://www.github.com/googleapis/google-cloud-go/commit/30794e70050b55ff87d6a80d0b4075065e9d271d))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/dataproc/v0.1.0...dataproc/v0.2.0) (2021-08-30)


### Features

* **dataproc:** remove apiv1beta2 client ([#4682](https://www.github.com/googleapis/google-cloud-go/issues/4682)) ([2248554](https://www.github.com/googleapis/google-cloud-go/commit/22485541affb1251604df292670a20e794111d3e))

## v0.1.0

This is the first tag to carve out dataproc as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
