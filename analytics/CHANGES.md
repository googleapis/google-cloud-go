# Changes

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
