# Changes

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.23.0...datacatalog/v1.24.0) (2024-11-21)


### Features

* **datacatalog:** A new enum `CatalogUIExperience` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new enum `TagTemplateMigration` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new field `dataplex_transfer_status` is added to message `.google.cloud.datacatalog.v1.Tag` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new field `transferred_to_dataplex` is added to message `.google.cloud.datacatalog.v1.EntryGroup` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new message `MigrationConfig` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new message `OrganizationConfig` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new message `RetrieveConfigRequest` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new message `RetrieveEffectiveConfigRequest` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new message `SetConfigRequest` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new method `RetrieveConfig` is added to service `DataCatalog` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new method `RetrieveEffectiveConfig` is added to service `DataCatalog` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new method `SetConfig` is added to service `DataCatalog` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datacatalog:** A new value `TRANSFERRED` is added to enum `DataplexTransferStatus` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))


### Documentation

* **datacatalog:** A comment for message `EntryGroup` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.22.2...datacatalog/v1.23.0) (2024-11-14)


### Features

* **datacatalog:** A new enum `DataplexTransferStatus` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new field `dataplex_transfer_status` is added to message `.google.cloud.datacatalog.v1.TagTemplate` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new field `feature_online_store_spec` is added to message `.google.cloud.datacatalog.v1.Entry` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new message `FeatureOnlineStoreSpec` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `CUSTOM_TEXT_EMBEDDING` is added to enum `ModelSourceType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `FEATURE_GROUP` is added to enum `EntryType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `FEATURE_ONLINE_STORE` is added to enum `EntryType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `FEATURE_VIEW` is added to enum `EntryType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `GENIE` is added to enum `ModelSourceType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A new value `MARKETPLACE` is added to enum `ModelSourceType` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))


### Documentation

* **datacatalog:** A comment for field `name` in message `.google.cloud.datacatalog.v1.Entry` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A comment for field `name` in message `.google.cloud.datacatalog.v1.EntryGroup` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A comment for field `name` in message `.google.cloud.datacatalog.v1.Tag` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A comment for field `name` in message `.google.cloud.datacatalog.v1.TagTemplate` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **datacatalog:** A comment for field `name` in message `.google.cloud.datacatalog.v1.TagTemplateField` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))

## [1.22.2](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.22.1...datacatalog/v1.22.2) (2024-10-23)


### Bug Fixes

* **datacatalog:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **datacatalog:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.22.0...datacatalog/v1.22.1) (2024-09-12)


### Bug Fixes

* **datacatalog:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.21.1...datacatalog/v1.22.0) (2024-08-20)


### Features

* **datacatalog:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.21.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.21.0...datacatalog/v1.21.1) (2024-08-08)


### Bug Fixes

* **datacatalog:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.5...datacatalog/v1.21.0) (2024-08-01)


### Features

* **datacatalog:** Add DataplexTransferStatus enum and field to TagTemplate ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))


### Documentation

* **datacatalog:** Mark DataplexTransferStatus.MIGRATED as deprecated ([#10621](https://github.com/googleapis/google-cloud-go/issues/10621)) ([6b51942](https://github.com/googleapis/google-cloud-go/commit/6b519428182e8b17ff30fa09e0e3c18716269f1c))
* **datacatalog:** Update field comments for updated IDENTIFIER field behavior ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))

## [1.20.5](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.4...datacatalog/v1.20.5) (2024-07-24)


### Bug Fixes

* **datacatalog:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.20.4](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.3...datacatalog/v1.20.4) (2024-07-10)


### Bug Fixes

* **datacatalog:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.20.3](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.2...datacatalog/v1.20.3) (2024-07-01)


### Bug Fixes

* **datacatalog:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.20.2](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.1...datacatalog/v1.20.2) (2024-06-26)


### Bug Fixes

* **datacatalog:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.20.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.20.0...datacatalog/v1.20.1) (2024-05-01)


### Bug Fixes

* **datacatalog:** Add internaloption.WithDefaultEndpointTemplate ([3b41408](https://github.com/googleapis/google-cloud-go/commit/3b414084450a5764a0248756e95e13383a645f90))
* **datacatalog:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.19.3...datacatalog/v1.20.0) (2024-03-14)


### Features

* **datacatalog:** Add RANGE type to Data Catalog ([#9573](https://github.com/googleapis/google-cloud-go/issues/9573)) ([25c3f2d](https://github.com/googleapis/google-cloud-go/commit/25c3f2dfcf1e720df82b3ee236d8e6a1fe888318))


### Bug Fixes

* **datacatalog:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.19.3](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.19.2...datacatalog/v1.19.3) (2024-01-30)


### Bug Fixes

* **datacatalog:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.19.1...datacatalog/v1.19.2) (2024-01-11)


### Bug Fixes

* **datacatalog:** Change field behavior of the property "name" to IDENTIFIER ([c3f1174](https://github.com/googleapis/google-cloud-go/commit/c3f1174dc29d1c00d514a69590bd83f9b08a60d1))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.19.0...datacatalog/v1.19.1) (2024-01-08)


### Documentation

* **datacatalog:** Change field behavior of the property "name" to IDENTIFIER ([bd30055](https://github.com/googleapis/google-cloud-go/commit/bd3005532fbffa9894b11149e9693b7c33227d79))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.18.3...datacatalog/v1.19.0) (2023-11-09)


### Features

* **datacatalog/lineage:** Add open lineage support ([#8974](https://github.com/googleapis/google-cloud-go/issues/8974)) ([1a16cbf](https://github.com/googleapis/google-cloud-go/commit/1a16cbf260bb673e07a05e1014868b236e510499))

## [1.18.3](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.18.2...datacatalog/v1.18.3) (2023-11-01)


### Bug Fixes

* **datacatalog:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.18.2](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.18.1...datacatalog/v1.18.2) (2023-10-26)


### Bug Fixes

* **datacatalog:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.18.0...datacatalog/v1.18.1) (2023-10-12)


### Bug Fixes

* **datacatalog:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.17.1...datacatalog/v1.18.0) (2023-10-04)


### Features

* **datacatalog:** Enable Vertex AI Ingestion on DataPlex ([e9ae601](https://github.com/googleapis/google-cloud-go/commit/e9ae6018983ae09781740e4ff939e6e365863dbb))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.17.0...datacatalog/v1.17.1) (2023-09-11)


### Documentation

* **datacatalog:** Fix typo ([20725c8](https://github.com/googleapis/google-cloud-go/commit/20725c86c970ad24efa18c056fc3aa71dc3a4f03))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.16.0...datacatalog/v1.17.0) (2023-08-08)


### Features

* **datacatalog:** Add support for admin_search in SearchCatalog() API method ([4b68747](https://github.com/googleapis/google-cloud-go/commit/4b6874762ca3e5ebef76f72496753650cdf39523))


### Documentation

* **datacatalog:** Minor formatting ([e3f8c89](https://github.com/googleapis/google-cloud-go/commit/e3f8c89429a207c05fee36d5d93efe76f9e29efe))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.15.0...datacatalog/v1.16.0) (2023-07-18)


### Features

* **datacatalog/lineage:** Promote to GA ([#8265](https://github.com/googleapis/google-cloud-go/issues/8265)) ([130c571](https://github.com/googleapis/google-cloud-go/commit/130c5713dcbac7f670cb92ea113dd53d8029c960))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.14.1...datacatalog/v1.15.0) (2023-07-10)


### Features

* **datacatalog:** Added rpc RenameTagTemplateFieldEnumValue ([#8205](https://github.com/googleapis/google-cloud-go/issues/8205)) ([a0eb675](https://github.com/googleapis/google-cloud-go/commit/a0eb67567e00362f62c1a8186c1c0dbfec8ffcda))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.14.0...datacatalog/v1.14.1) (2023-06-20)


### Bug Fixes

* **datacatalog:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.13.1...datacatalog/v1.14.0) (2023-05-30)


### Features

* **datacatalog:** Add support for entries associated with Spanner and ClougBigTable ([#7992](https://github.com/googleapis/google-cloud-go/issues/7992)) ([ebae64d](https://github.com/googleapis/google-cloud-go/commit/ebae64d53397ec5dfe851f098754eaa1f5df7cb1))
* **datacatalog:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.13.0...datacatalog/v1.13.1) (2023-05-08)


### Bug Fixes

* **datacatalog:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.12.0...datacatalog/v1.13.0) (2023-03-15)


### Features

* **datacatalog:** Add support for new ImportEntries() API, including format of the dump ([8775cae](https://github.com/googleapis/google-cloud-go/commit/8775cae47a9efb358ce34240853a1b09c7f6dc62))
* **datacatalog:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog-v1.11.0...datacatalog/v1.12.0) (2023-01-26)


### Features

* **datacatalog/apiv1beta1:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **datacatalog/lineage:** Start generating apiv1 ([#7245](https://github.com/googleapis/google-cloud-go/issues/7245)) ([d7a53c3](https://github.com/googleapis/google-cloud-go/commit/d7a53c3a8ca8f8434d7f41f7a55effa9366e0461))
* **datacatalog:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **datacatalog:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **datacatalog:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **datacatalog:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **datacatalog:** Start generating REST transport for apiv1 ([#7246](https://github.com/googleapis/google-cloud-go/issues/7246)) ([1b90131](https://github.com/googleapis/google-cloud-go/commit/1b9013192c1e82c7ef4a5e42273bcc1ac2a57223))
* **datacatalog:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **datacatalog/lineage:** Fixed formatting for several literal expressions ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))
* **datacatalog:** Documentation updates chore: cleanup; annotations updates; adding missing imports ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog-v1.10.1...datacatalog/v1.11.0) (2023-01-26)


### Features

* **datacatalog/apiv1beta1:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **datacatalog/lineage:** Start generating apiv1 ([#7245](https://github.com/googleapis/google-cloud-go/issues/7245)) ([d7a53c3](https://github.com/googleapis/google-cloud-go/commit/d7a53c3a8ca8f8434d7f41f7a55effa9366e0461))
* **datacatalog:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **datacatalog:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **datacatalog:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **datacatalog:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **datacatalog:** Start generating REST transport for apiv1 ([#7246](https://github.com/googleapis/google-cloud-go/issues/7246)) ([1b90131](https://github.com/googleapis/google-cloud-go/commit/1b9013192c1e82c7ef4a5e42273bcc1ac2a57223))
* **datacatalog:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **datacatalog/lineage:** Fixed formatting for several literal expressions ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))
* **datacatalog:** Documentation updates chore: cleanup; annotations updates; adding missing imports ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.10.0...datacatalog/v1.10.1) (2023-01-26)


### Documentation

* **datacatalog/lineage:** Fixed formatting for several literal expressions ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.9.0...datacatalog/v1.10.0) (2023-01-18)


### Features

* **datacatalog/lineage:** Start generating apiv1 ([#7245](https://github.com/googleapis/google-cloud-go/issues/7245)) ([d7a53c3](https://github.com/googleapis/google-cloud-go/commit/d7a53c3a8ca8f8434d7f41f7a55effa9366e0461))
* **datacatalog:** Start generating REST transport for apiv1 ([#7246](https://github.com/googleapis/google-cloud-go/issues/7246)) ([1b90131](https://github.com/googleapis/google-cloud-go/commit/1b9013192c1e82c7ef4a5e42273bcc1ac2a57223))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.8.1...datacatalog/v1.9.0) (2023-01-04)


### Features

* **datacatalog:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.8.0...datacatalog/v1.8.1) (2022-11-09)


### Documentation

* **datacatalog:** documentation updates chore: cleanup; annotations updates; adding missing imports ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.7.0...datacatalog/v1.8.0) (2022-11-03)


### Features

* **datacatalog:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.6.0...datacatalog/v1.7.0) (2022-10-25)


### Features

* **datacatalog:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.5.0...datacatalog/v1.6.0) (2022-09-21)


### Features

* **datacatalog:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.4.0...datacatalog/v1.5.0) (2022-09-19)


### Features

* **datacatalog:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.3.1...datacatalog/v1.4.0) (2022-09-15)


### Features

* **datacatalog/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.3.0...datacatalog/v1.3.1) (2022-07-12)


### Documentation

* **datacatalog:** update taxonomy display_name comment feat: added Dataplex specific fields ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.2.0...datacatalog/v1.3.0) (2022-02-23)


### Features

* **datacatalog:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/datacatalog/v1.1.0...datacatalog/v1.2.0) (2022-02-11)


### Features

* **datacatalog:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))
* **datacatalog:** Add methods and messages related to starring feature feat: Add methods and messages related to business context feature docs: Updates copyright message ([61f23b2](https://github.com/googleapis/google-cloud-go/commit/61f23b2167dbe9e3e031db12ccf46b7eac639fa3))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/datacatalog/v1.0.0...datacatalog/v1.1.0) (2022-01-04)


### Features

* **datacatalog:** Added BigQueryDateShardedSpec.latest_shard_resource field feat: Added SearchCatalogResult.display_name field feat: Added SearchCatalogResult.description field ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out datacatalog as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
