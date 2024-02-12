# Changes

## [1.14.7](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.6...artifactregistry/v1.14.7) (2024-01-30)


### Bug Fixes

* **artifactregistry:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.14.6](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.5...artifactregistry/v1.14.6) (2023-11-01)


### Bug Fixes

* **artifactregistry:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.14.5](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.4...artifactregistry/v1.14.5) (2023-10-31)


### Documentation

* **artifactregistry:** Use code font for resource name references ([ffb0dda](https://github.com/googleapis/google-cloud-go/commit/ffb0ddabf3d9822ba8120cabaf25515fd32e9615))

## [1.14.4](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.3...artifactregistry/v1.14.4) (2023-10-26)


### Bug Fixes

* **artifactregistry:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.2...artifactregistry/v1.14.3) (2023-10-12)


### Bug Fixes

* **artifactregistry:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.1...artifactregistry/v1.14.2) (2023-10-04)


### Bug Fixes

* **artifactregistry:** Make repository and repository_id in CreateRepository required ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.14.0...artifactregistry/v1.14.1) (2023-06-20)


### Bug Fixes

* **artifactregistry:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.13.1...artifactregistry/v1.14.0) (2023-05-30)


### Features

* **artifactregistry:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.13.0...artifactregistry/v1.13.1) (2023-05-08)


### Bug Fixes

* **artifactregistry:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.12.0...artifactregistry/v1.13.0) (2023-04-04)


### Features

* **artifactregistry:** Promote to GA ([#7647](https://github.com/googleapis/google-cloud-go/issues/7647)) ([9334a1d](https://github.com/googleapis/google-cloud-go/commit/9334a1db35f9edc1700ca125191d3240cb9b3415))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.11.2...artifactregistry/v1.12.0) (2023-03-15)


### Features

* **artifactregistry:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.11.1...artifactregistry/v1.11.2) (2023-03-01)


### Bug Fixes

* **artifactregistry:** Remove unintentionally published proto ([aeb6fec](https://github.com/googleapis/google-cloud-go/commit/aeb6fecc7fd3f088ff461a0c068ceb9a7ae7b2a3))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.11.0...artifactregistry/v1.11.1) (2023-02-16)


### Bug Fixes

* **artifactregistry:** Remove unintentionally published proto ([#7434](https://github.com/googleapis/google-cloud-go/issues/7434)) ([d42b989](https://github.com/googleapis/google-cloud-go/commit/d42b98943fe71795747e386879ae3b72f6f32a36))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.10.0...artifactregistry/v1.11.0) (2023-02-14)


### Features

* **artifactregistry:** Add format-specific resources `MavenArtifact`, `NpmPackage`, `KfpArtifact` and `PythonPackage` feat: add `order_by` to `ListDockerImages` feat: add an API to get and update VPCSC config feat: add `BatchDeleteVersionMetadata` to return version that failed to delete fix!: make `GetFileRequest.name` and `ListFilesRequest.parent` required fix: make `Package` a resource fix: deprecate `REDIRECTION_FROM_GCR_IO_FINALIZED` ([2fef56f](https://github.com/googleapis/google-cloud-go/commit/2fef56f75a63dc4ff6e0eea56c7b26d4831c8e27))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.9.0...artifactregistry/v1.10.0) (2023-01-04)


### Features

* **artifactregistry:** Add location methods ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **artifactregistry:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.8.0...artifactregistry/v1.9.0) (2022-11-03)


### Features

* **artifactregistry:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.7.0...artifactregistry/v1.8.0) (2022-10-25)


### Features

* **artifactregistry:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.6.0...artifactregistry/v1.7.0) (2022-09-21)


### Features

* **artifactregistry:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.5.0...artifactregistry/v1.6.0) (2022-09-19)


### Features

* **artifactregistry:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.4.0...artifactregistry/v1.5.0) (2022-09-15)


### Features

* **artifactregistry/apiv1beta2:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.3.0...artifactregistry/v1.4.0) (2022-07-26)


### Features

* **artifactregistry:** start generating apiv1 ([#6403](https://github.com/googleapis/google-cloud-go/issues/6403)) ([045b544](https://github.com/googleapis/google-cloud-go/commit/045b544619f6199acefe454b015bc6b30d595bf3))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.2.0...artifactregistry/v1.3.0) (2022-02-23)


### Features

* **artifactregistry:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.1.0...artifactregistry/v1.2.0) (2022-02-14)


### Features

* **artifactregistry:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.0.2...artifactregistry/v1.1.0) (2022-02-03)


### Features

* **artifactregistry:** add APIs for importing and uploading Apt and Yum artifacts feat: add version policy support for Maven repositories feat: add order_by support for listing versions fix!: mark a few resource name fields as required ([f560b1e](https://www.github.com/googleapis/google-cloud-go/commit/f560b1ed0263956ef84fbf2fbf34bdc66dbc0a88))

### [1.0.2](https://www.github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.0.1...artifactregistry/v1.0.2) (2022-01-13)


### Bug Fixes

* **artifactregistry:** add missing HTTP rules to service config ([3bbe8c0](https://www.github.com/googleapis/google-cloud-go/commit/3bbe8c0c558c06ef5865bb79eb228b6da667ddb3))

### [1.0.1](https://www.github.com/googleapis/google-cloud-go/compare/artifactregistry/v1.0.0...artifactregistry/v1.0.1) (2022-01-04)


### Bug Fixes

* **artifactregistry:** fix resource pattern ID segment name ([5444809](https://www.github.com/googleapis/google-cloud-go/commit/5444809e0b7cf9f5416645ea2df6fec96f8b9023))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out artifactregistry as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
