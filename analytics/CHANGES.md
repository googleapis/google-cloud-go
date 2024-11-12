# Changes

## [0.25.2](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.25.1...analytics/v0.25.2) (2024-10-23)


### Bug Fixes

* **analytics:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **analytics:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.25.1](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.25.0...analytics/v0.25.1) (2024-09-12)


### Bug Fixes

* **analytics:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.25.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.24.0...analytics/v0.25.0) (2024-08-20)


### Features

* **analytics:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.24.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.6...analytics/v0.24.0) (2024-08-08)


### Features

* **analytics/admin:** Add `CreateBigQueryLink`, `UpdateBigQueryLink`, and `DeleteBigQueryLink` methods ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add `GetEventEditRule`, `CreateEventEditRule`, `ListEventEditRules`, `UpdateEventEditRule`, `DeleteEventEditRule`, and `ReorderEventEditRules` methods to the Admin API v1 alpha ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add `GetKeyEvent`, `CreateKeyEvent`, `ListKeyEvents`, `UpdateKeyEvent`, and `DeleteKeyEvent` methods ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add the `BIGQUERY_LINK` option to the `ChangeHistoryResourceType` enum ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add the `create_time` field to the `Audience` resource ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add the `dataset_location` field to the `BigQueryLink` resource ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add the `gmp_organization` field to the `Account` resource ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Add the `primary` field to the `ChannelGroup` resource ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Mark `GetConversionEvent`, `CreateConversionEvent`, `ListConversionEvents`, `UpdateConversionEvent`, and `DeleteConversionEvent` methods as deprecated ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))


### Bug Fixes

* **analytics/admin:** Rename custom method `CreateSubpropertyRequest` to `ProvisionSubpropertyRequest` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))


### Documentation

* **analytics/admin:** Add deprecation comment to `GetConversionEvent`, `CreateConversionEvent`, `ListConversionEvents`, `UpdateConversionEvent`, and `DeleteConversionEvent` methods ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Improve comment formatting of `account` and `property` fields in `SearchChangeHistoryEventsRequest` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Improve comment formatting of the `name` field in `DeleteFirebaseLinkRequest`, `GetGlobalSiteTagRequest`, and `GetDataSharingSettingsRequest` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **analytics/admin:** Improve comment formatting of the `parent` field in `CreateFirebaseLinkRequest` and `ListFirebaseLinksRequest` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))

## [0.23.6](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.5...analytics/v0.23.6) (2024-07-24)


### Bug Fixes

* **analytics:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.23.5](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.4...analytics/v0.23.5) (2024-07-10)


### Bug Fixes

* **analytics:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.23.4](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.3...analytics/v0.23.4) (2024-07-01)


### Bug Fixes

* **analytics:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.23.3](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.2...analytics/v0.23.3) (2024-06-26)


### Bug Fixes

* **analytics:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.23.2](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.1...analytics/v0.23.2) (2024-05-01)


### Bug Fixes

* **analytics:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.23.1](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.23.0...analytics/v0.23.1) (2024-03-14)


### Bug Fixes

* **analytics:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.23.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.22.0...analytics/v0.23.0) (2024-01-30)


### Features

* **analytics/admin:** Add `GetCalculatedMetric`, `CreateCalculatedMetric`, `ListCalculatedMetrics`, `UpdateCalculatedMetric`, `DeleteCalculatedMetric` methods to the Admin API v1alpha ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))


### Bug Fixes

* **analytics:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.22.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.6...analytics/v0.22.0) (2024-01-03)


### Features

* **analytics/admin:** Add `GetSubpropertyEventFilter`, `ListSubpropertyEventFilters` methods to the Admin API v1 alpha ([69c49f2](https://github.com/googleapis/google-cloud-go/commit/69c49f2537af8064e7b18e4845c3b2fbd502f141))

## [0.21.6](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.5...analytics/v0.21.6) (2023-11-01)


### Bug Fixes

* **analytics:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.21.5](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.4...analytics/v0.21.5) (2023-10-26)


### Bug Fixes

* **analytics:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.21.4](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.3...analytics/v0.21.4) (2023-10-12)


### Bug Fixes

* **analytics:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [0.21.3](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.2...analytics/v0.21.3) (2023-07-27)


### Bug Fixes

* **analytics/admin:** Update the `ReportingAttributionModel` enum to rename `CROSS_CHANNEL_DATA_DRIVEN` to `PAID_AND_ORGANIC_CHANNELS_DATA_DRIVEN`, `CROSS_CHANNEL_LAST_CLICK` to `PAID_AND_ORGANIC_CHANNELS_LAST_CLICK`, `CROSS_CHANNEL_FIRST_CLICK` to `... ([#8330](https://github.com/googleapis/google-cloud-go/issues/8330)) ([f7939e0](https://github.com/googleapis/google-cloud-go/commit/f7939e093159a40d8be0ca4a60284b5bad524ae5))

## [0.21.2](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.21.1...analytics/v0.21.2) (2023-06-27)


### Documentation

* **analytics/admin:** Announce the deprecation of first-click, linear, time-decay and position-based attribution models ([94ea341](https://github.com/googleapis/google-cloud-go/commit/94ea3410e233db6040a7cb0a931948f1e3bb4c9a))

## [0.21.1](https://github.com/googleapis/google-cloud-go/compare/analytics-v0.21.0...analytics/v0.21.1) (2023-06-20)


### Bug Fixes

* **analytics:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [0.21.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.20.0...analytics/v0.21.0) (2023-05-30)


### Features

* **analytics:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [0.20.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.19.1...analytics/v0.20.0) (2023-05-16)


### Features

* **analytics/admin:** Add `GetAdSenseLink`, `CreateAdSenseLink`, `DeleteAdSenseLink`, `ListAdSenseLinks` methods to the Admin API v1alpha ([8c479ac](https://github.com/googleapis/google-cloud-go/commit/8c479acd5ea710629b4b562a4654bc369e828c16))

## [0.19.1](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.19.0...analytics/v0.19.1) (2023-05-08)


### Bug Fixes

* **analytics:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [0.19.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.18.0...analytics/v0.19.0) (2023-03-22)


### Features

* **analytics/admin:** Support REST transport ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))

## [0.18.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.17.0...analytics/v0.18.0) (2023-03-01)


### Features

* **analytics/admin:** Add `CreateAccessBinding`, `GetAccessBinding`, `UpdateAccessBinding`, `DeleteAccessBinding`, `ListAccessBindings`, `BatchCreateAccessBindings`, `BatchGetAccessBindings`, `BatchUpdateAccessBindings`, `BatchDeleteAccessBindings` methods to the Admin API v1alpha ([aeb6fec](https://github.com/googleapis/google-cloud-go/commit/aeb6fecc7fd3f088ff461a0c068ceb9a7ae7b2a3))

## [0.17.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.16.0...analytics/v0.17.0) (2023-02-14)


### Features

* **analytics/admin:** Add `GetSearchAds360Link`, `ListSearchAds360Links`, `CreateSearchAds360Link`, `DeleteSearchAds360Link`, `UpdateSearchAds360Link` methods to the Admin API v1alpha feat: add `SetAutomatedGa4ConfigurationOptOut`, `FetchAutomatedGa4ConfigurationOptOut` methods to the Admin API v1alpha feat: add `GetBigQueryLink`, `ListBigQueryLinks` methods to the Admin API v1alpha feat: add `tokens_per_project_per_hour` field to `AccessQuota` type feat: add `EXPANDED_DATA_SET`, `CHANNEL_GROUP` values to `ChangeHistoryResourceType` enum feat: add `search_ads_360_link`, `expanded_data_set`, `bigquery_link` values to ChangeHistoryResource.resource oneof field feat: add `BigQueryLink`, `SearchAds360Link` resource types to the Admin API v1alpha fix!: remove `LESS_THAN_OR_EQUAL`, `GREATER_THAN_OR_EQUAL` values from NumericFilter.Operation enum fix!: remove `PARTIAL_REGEXP` value from StringFilter.MatchType enum ([2fef56f](https://github.com/googleapis/google-cloud-go/commit/2fef56f75a63dc4ff6e0eea56c7b26d4831c8e27))

## [0.16.0](https://github.com/googleapis/google-cloud-go/compare/analytics-v0.15.0...analytics/v0.16.0) (2023-01-26)


### Features

* **analytics/admin:** Add `GetAudience`, 'ListAudience', 'CreateAudience', 'UpdateAudience', 'ArchiveAudience' methods to the Admin API v1alpha feat: add `GetAttributionSettings`, `UpdateAttributionSettings` methods to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))
* **analytics/admin:** Add `RunAccessReport` method to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))
* **analytics/admin:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **analytics/admin:** Enable REST transport support for Python analytics-admin, media-translation and dataflow clients ([ec1a190](https://github.com/googleapis/google-cloud-go/commit/ec1a190abbc4436fcaeaa1421c7d9df624042752))
* **analytics:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **analytics:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **analytics:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))


### Bug Fixes

* **analytics/admin:** Add py_test targets ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [0.15.0](https://github.com/googleapis/google-cloud-go/compare/analytics-v0.14.0...analytics/v0.15.0) (2023-01-26)


### Features

* **analytics/admin:** Add `GetAudience`, 'ListAudience', 'CreateAudience', 'UpdateAudience', 'ArchiveAudience' methods to the Admin API v1alpha feat: add `GetAttributionSettings`, `UpdateAttributionSettings` methods to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))
* **analytics/admin:** Add `RunAccessReport` method to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))
* **analytics/admin:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **analytics/admin:** Enable REST transport support for Python analytics-admin, media-translation and dataflow clients ([ec1a190](https://github.com/googleapis/google-cloud-go/commit/ec1a190abbc4436fcaeaa1421c7d9df624042752))
* **analytics:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **analytics:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **analytics:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))


### Bug Fixes

* **analytics/admin:** Add py_test targets ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [0.14.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.13.0...analytics/v0.14.0) (2023-01-26)


### Features

* **analytics/admin:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [0.13.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.12.0...analytics/v0.13.0) (2023-01-04)


### Features

* **analytics:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.12.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.11.0...analytics/v0.12.0) (2022-09-21)


### Features

* **analytics:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.10.0...analytics/v0.11.0) (2022-09-19)


### Features

* **analytics:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.9.0...analytics/v0.10.0) (2022-09-15)


### Features

* **analytics/admin:** Enable REST transport support for Python analytics-admin, media-translation and dataflow clients ([ec1a190](https://github.com/googleapis/google-cloud-go/commit/ec1a190abbc4436fcaeaa1421c7d9df624042752))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.8.1...analytics/v0.9.0) (2022-08-09)


### Features

* **analytics/admin:** add `GetAudience`, 'ListAudience', 'CreateAudience', 'UpdateAudience', 'ArchiveAudience' methods to the Admin API v1alpha feat: add `GetAttributionSettings`, `UpdateAttributionSettings` methods to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))
* **analytics/admin:** add `RunAccessReport` method to the Admin API v1alpha ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))

## [0.8.1](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.8.0...analytics/v0.8.1) (2022-08-02)


### Bug Fixes

* **analytics/admin:** Add py_test targets ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.7.0...analytics/v0.8.0) (2022-06-29)


### Features

* **analytics/admin:** Enable REST transport for most of Java and Go clients ([f01bf32](https://github.com/googleapis/google-cloud-go/commit/f01bf32d7f4aa2c59db6bfdcc574ce2470bc61bb))
* **analytics:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.6.1...analytics/v0.7.0) (2022-06-16)


### Features

* **analytics/admin:** Add Java REST transport to analytics, servicecontrol, servicemanagement, serviceusage and langauge APIs ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))

### [0.6.1](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.6.0...analytics/v0.6.1) (2022-05-24)


### Bug Fixes

* **analytics/admin:** CustomDimension and CustomMetric resource configuration in Analytics Admin API ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.5.0...analytics/v0.6.0) (2022-03-14)


### Features

* **analytics/admin:** remove `WebDataStream`, `IosAppDataStream`, `AndroidAppDataStream` resources and corresponding operations, as they are replaced by the `DataStream` resource feat: add `restricted_metric_type` field to the `CustomMetric` resource feat!: move the `GlobalSiteTag` resource from the property level to the data stream level ([3f17f9f](https://github.com/googleapis/google-cloud-go/commit/3f17f9fb741bc426800ca68f29de66fbc8751df1))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.4.0...analytics/v0.5.0) (2022-02-23)


### Features

* **analytics:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/analytics/v0.3.0...analytics/v0.4.0) (2022-02-14)


### Features

* **analytics:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/analytics/v0.2.0...analytics/v0.3.0) (2022-01-04)


### Features

* **analytics/admin:** add the `AcknowledgeUserDataCollection` operation which acknowledges the terms of user data collection for the specified property feat: add the new resource type `DataStream`, which is planned to eventually replace `WebDataStream`, `IosAppDataStream`, `AndroidAppDataStream` resources fix!: remove `GetEnhancedMeasurementSettings`, `UpdateEnhancedMeasurementSettingsRequest`, `UpdateEnhancedMeasurementSettingsRequest` operations from the API feat: add `CreateDataStream`, `DeleteDataStream`, `UpdateDataStream`, `ListDataStreams` operations to support the new `DataStream` resource feat: add `DISPLAY_VIDEO_360_ADVERTISER_LINK`,  `DISPLAY_VIDEO_360_ADVERTISER_LINK_PROPOSAL` fields to `ChangeHistoryResourceType` enum feat: add the `account` field to the `Property` type docs: update the documentation with a new list of valid values for `UserLink.direct_roles` field ([5444809](https://www.github.com/googleapis/google-cloud-go/commit/5444809e0b7cf9f5416645ea2df6fec96f8b9023))

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/analytics/v0.1.0...analytics/v0.2.0) (2021-08-30)


### Features

* **analytics/admin:** add `GetDataRetentionSettings`, `UpdateDataRetentionSettings` methods to the API ([8467899](https://www.github.com/googleapis/google-cloud-go/commit/8467899ab6ebf0328c543bfb5fbcddeb2f53a082))

## v0.1.0

This is the first tag to carve out analytics as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
