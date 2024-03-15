# Changelog



## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.10.0...alloydb/v1.10.1) (2024-03-14)


### Bug Fixes

* **alloydb:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.9.0...alloydb/v1.10.0) (2024-03-04)


### Features

* **alloydb:** Support for obtaining the public IP address of an Instance ([d130d86](https://github.com/googleapis/google-cloud-go/commit/d130d861f55d137a2803340c2e11da3589669cb8))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.8.1...alloydb/v1.9.0) (2024-02-26)


### Features

* **alloydb:** Support for obtaining the public IP address of an Instance ([#9470](https://github.com/googleapis/google-cloud-go/issues/9470)) ([c1f4bc8](https://github.com/googleapis/google-cloud-go/commit/c1f4bc8c087aa6ba7421426d5ab6b0404527db6c))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.8.0...alloydb/v1.8.1) (2024-01-30)


### Bug Fixes

* **alloydb:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.7.0...alloydb/v1.8.0) (2024-01-03)


### Features

* **alloydb:** Added PSC config, PSC interface config, PSC instance config ([#9184](https://github.com/googleapis/google-cloud-go/issues/9184)) ([36fec33](https://github.com/googleapis/google-cloud-go/commit/36fec33b8225f3e7df552839290ddbfefb222816))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.6.3...alloydb/v1.7.0) (2023-11-09)


### Features

* **alloydb:** Add new field in `GenerateClientCertificate` v1 API to allow AlloyDB connectors request client certs with metadata exchange support ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [1.6.3](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.6.2...alloydb/v1.6.3) (2023-11-01)


### Bug Fixes

* **alloydb:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.6.2](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.6.1...alloydb/v1.6.2) (2023-10-26)


### Bug Fixes

* **alloydb:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.6.0...alloydb/v1.6.1) (2023-10-12)


### Bug Fixes

* **alloydb:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.5.0...alloydb/v1.6.0) (2023-10-04)


### Features

* **alloydb/connectors:** Start generating apiv1 ([#8648](https://github.com/googleapis/google-cloud-go/issues/8648)) ([c68448e](https://github.com/googleapis/google-cloud-go/commit/c68448eb1787d56dbd91920f376cc9dad2cb163e))
* **alloydb:** Add support to generate client certificate and get connection info for auth proxy in AlloyDB v1 ([e9ae601](https://github.com/googleapis/google-cloud-go/commit/e9ae6018983ae09781740e4ff939e6e365863dbb))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.4.0...alloydb/v1.5.0) (2023-09-20)


### Features

* **alloydb:** Added enum value for PG15 ([2f3bb44](https://github.com/googleapis/google-cloud-go/commit/2f3bb443e9fa6968d20806f86b391dad85970afc))
* **alloydb:** Added enum value for PG15 ([2f3bb44](https://github.com/googleapis/google-cloud-go/commit/2f3bb443e9fa6968d20806f86b391dad85970afc))
* **alloydb:** Changed description for recovery_window_days in ContinuousBackupConfig ([2f3bb44](https://github.com/googleapis/google-cloud-go/commit/2f3bb443e9fa6968d20806f86b391dad85970afc))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.3.0...alloydb/v1.4.0) (2023-07-31)


### Features

* **alloydb:** Generate connector types ([#8357](https://github.com/googleapis/google-cloud-go/issues/8357)) ([f777e68](https://github.com/googleapis/google-cloud-go/commit/f777e6884b7ac63a0dafef56b5d9f8ae923fe073))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.2.1...alloydb/v1.3.0) (2023-07-18)


### Features

* **alloydb:** Add metadata exchange support for AlloyDB connectors ([#8255](https://github.com/googleapis/google-cloud-go/issues/8255)) ([22a908b](https://github.com/googleapis/google-cloud-go/commit/22a908b0bd26f131c6033ec3fc48eaa2d2cd0c0e))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.2.0...alloydb/v1.2.1) (2023-06-20)


### Bug Fixes

* **alloydb:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/alloydb-v1.1.0...alloydb/v1.2.0) (2023-06-13)


### Features

* **alloydb:** Added ClusterView supporting more granular view of continuous backups ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))
* **alloydb:** Added ClusterView supporting more granular view of continuous backups ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))
* **alloydb:** Added new SSL modes ALLOW_UNENCRYPTED_AND_ENCRYPTED, ENCRYPTED_ONLY ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.0.1...alloydb/v1.1.0) (2023-05-30)


### Features

* **alloydb:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v1.0.0...alloydb/v1.0.1) (2023-05-08)


### Bug Fixes

* **alloydb:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v0.2.1...alloydb/v1.0.0) (2023-04-25)


### Features

* **alloydb:** Promote to GA ([#7769](https://github.com/googleapis/google-cloud-go/issues/7769)) ([c6fc46c](https://github.com/googleapis/google-cloud-go/commit/c6fc46c296b37700b7dafed4c95022515c616bbc))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/alloydb/v0.2.0...alloydb/v0.2.1) (2023-04-04)


### Documentation

* **alloydb:** Minor formatting in description of AvailabilityType ([7aa546e](https://github.com/googleapis/google-cloud-go/commit/7aa546ebf19b9d8e7aaef5438525a4df97a1aa98))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/alloydb/v0.1.0...alloydb/v0.2.0) (2023-03-15)


### Features

* **alloydb:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## 0.1.0 (2023-03-01)


### Features

* **alloydb:** Start generating apiv1, apiv1beta, apiv1alpha ([#7503](https://github.com/googleapis/google-cloud-go/issues/7503)) ([25e8426](https://github.com/googleapis/google-cloud-go/commit/25e842659ef5c3941717827459e6524f024e5a26))

## Changes
