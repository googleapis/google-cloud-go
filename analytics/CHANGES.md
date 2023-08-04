# Changes

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
