# Changes


## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.19.2...dataplex/v1.20.0) (2024-11-19)


### Features

* **dataplex:** A new enum `TableType` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** A new field `datascan_id` is added to message `.google.cloud.dataplex.v1.DiscoveryEvent` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** A new field `suspended` is added to DataScans ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** A new field `table` is added to message `.google.cloud.dataplex.v1.DiscoveryEvent` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** A new message `TableDetails` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a DATA_DISCOVERY enum type in DataScanEvent ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a DataDiscoveryAppliedConfigs message ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a TABLE_DELETED field in DiscoveryEvent ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a TABLE_IGNORED field in DiscoveryEvent ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a TABLE_PUBLISHED field in DiscoveryEvent ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add a TABLE_UPDATED field in DiscoveryEvent ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add an Issue field to DiscoveryEvent.ActionDetails to output the action message in Cloud Logs ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add annotations in CreateMetadataJob, GetMetadataJob, ListMetaDataJobs and CancelMetadataJob for cloud audit logging ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add data_version field to AspectSource ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add new Data Discovery scan type in Datascan ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Expose create time in DataScanJobAPI ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Expose create time to customers ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Release metadata export in private preview ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Release MetadataJob APIs and related resources in GA ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Update Go Bigtable import path ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Update Go Datastore import path ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))


### Documentation

* **dataplex:** A comment for message `DataScanEvent` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add comment for field `status` in message `.google.cloud.dataplex.v1.MetadataJob` per https ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add comment for field `type` in message `.google.cloud.dataplex.v1.MetadataJob` per https ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add Identifier for `name` in message `.google.cloud.dataplex.v1.MetadataJob` per https ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add info about schema changes for BigQuery metadata in Dataplex Catalog ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Add link to fully qualified names documentation ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Correct API documentation ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Correct the dimensions for data quality rules ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Dataplex Tasks do not support Dataplex Content path as a direct input anymore ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **dataplex:** Scrub descriptions for standalone discovery scans ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))

## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.19.1...dataplex/v1.19.2) (2024-10-23)


### Bug Fixes

* **dataplex:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **dataplex:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.19.0...dataplex/v1.19.1) (2024-09-12)


### Bug Fixes

* **dataplex:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.18.3...dataplex/v1.19.0) (2024-08-20)


### Features

* **dataplex:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.18.3](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.18.2...dataplex/v1.18.3) (2024-08-08)


### Bug Fixes

* **dataplex:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.18.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.18.1...dataplex/v1.18.2) (2024-07-24)


### Bug Fixes

* **dataplex:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.18.0...dataplex/v1.18.1) (2024-07-10)


### Bug Fixes

* **dataplex:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.17.0...dataplex/v1.18.0) (2024-07-01)


### Features

* **dataplex:** Expose data scan execution create time to customers ([#10438](https://github.com/googleapis/google-cloud-go/issues/10438)) ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))


### Bug Fixes

* **dataplex:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.16.1...dataplex/v1.17.0) (2024-06-26)


### Features

* **dataplex:** Exposing EntrySource.location field that contains location of a resource in the source system ([d6c543c](https://github.com/googleapis/google-cloud-go/commit/d6c543c3969016c63e158a862fc173dff60fb8d9))


### Bug Fixes

* **dataplex:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))


### Documentation

* **dataplex:** Scrub descriptions for GenerateDataQualityRules ([d6c543c](https://github.com/googleapis/google-cloud-go/commit/d6c543c3969016c63e158a862fc173dff60fb8d9))

## [1.16.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.16.0...dataplex/v1.16.1) (2024-06-18)


### Documentation

* **dataplex:** Clarify DataQualityRule.sql_assertion descriptions ([abac5c6](https://github.com/googleapis/google-cloud-go/commit/abac5c6eec859477c6d390b116ea8954213ba585))
* **dataplex:** Fix links to RuleType proto references ([abac5c6](https://github.com/googleapis/google-cloud-go/commit/abac5c6eec859477c6d390b116ea8954213ba585))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.15.1...dataplex/v1.16.0) (2024-05-08)


### Features

* **dataplex:** Updated client libraries for Dataplex Catalog ([a4a8fbc](https://github.com/googleapis/google-cloud-go/commit/a4a8fbcf561346eec1dc32987b10174f102bb46a))

## [1.15.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.15.0...dataplex/v1.15.1) (2024-05-01)


### Bug Fixes

* **dataplex:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.14.3...dataplex/v1.15.0) (2024-03-19)


### Features

* **dataplex:** Added client side library for the followings ([#9592](https://github.com/googleapis/google-cloud-go/issues/9592)) ([a3bb7c0](https://github.com/googleapis/google-cloud-go/commit/a3bb7c07ba570f26c6eb073ab3275487784547d0))

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.14.2...dataplex/v1.14.3) (2024-03-14)


### Bug Fixes

* **dataplex:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.14.1...dataplex/v1.14.2) (2024-02-06)


### Documentation

* **dataplex:** Fix typo in comment ([e60a6ba](https://github.com/googleapis/google-cloud-go/commit/e60a6ba01acf2ef2e8d12e23ed5c6e876edeb1b7))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.14.0...dataplex/v1.14.1) (2024-01-30)


### Bug Fixes

* **dataplex:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.13.0...dataplex/v1.14.0) (2024-01-03)


### Features

* **dataplex:** Added enum value EventType.GOVERNANCE_RULE_PROCESSING ([902d842](https://github.com/googleapis/google-cloud-go/commit/902d84299b5073543ade684aa311b791bed3a999))


### Documentation

* **dataplex:** Fix the comment for `ignore_null` field to clarify its applicability on data quality rules ([cbe96af](https://github.com/googleapis/google-cloud-go/commit/cbe96af778ec9152b528714281de9e534f01c237))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.12.0...dataplex/v1.13.0) (2023-12-07)


### Features

* **dataplex:** Add data quality score to DataQualityResult ([5132d0f](https://github.com/googleapis/google-cloud-go/commit/5132d0fea3a5ac902a2c9eee865241ed4509a5f4))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.11.2...dataplex/v1.12.0) (2023-11-27)


### Features

* **dataplex:** Added DataQualityResult.score, dimension_score, column_score ([2020edf](https://github.com/googleapis/google-cloud-go/commit/2020edff24e3ffe127248cf9a90c67593c303e18))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.11.1...dataplex/v1.11.2) (2023-11-09)


### Documentation

* **dataplex:** Updated comments for `DataQualityResult.dimensions` field ([ba23673](https://github.com/googleapis/google-cloud-go/commit/ba23673da7707c31292e4aa29d65b7ac1446d4a6))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.11.0...dataplex/v1.11.1) (2023-11-01)


### Bug Fixes

* **dataplex:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.10.2...dataplex/v1.11.0) (2023-10-31)


### Features

* **dataplex:** DataQualityDimension is now part of the DataQualityDimensionResult message ([4d40180](https://github.com/googleapis/google-cloud-go/commit/4d40180da0557c2a2e9e2cb8b0509b429676bfc0))

## [1.10.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.10.1...dataplex/v1.10.2) (2023-10-26)


### Bug Fixes

* **dataplex:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.10.0...dataplex/v1.10.1) (2023-10-12)


### Bug Fixes

* **dataplex:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.9.1...dataplex/v1.10.0) (2023-10-12)


### Features

* **dataplex:** DataQualityDimension is now part of the DataQualityDimensionResult message ([#8663](https://github.com/googleapis/google-cloud-go/issues/8663)) ([a811f4c](https://github.com/googleapis/google-cloud-go/commit/a811f4c49f0c3c769467239d866d4267a9ba2b44))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.9.0...dataplex/v1.9.1) (2023-08-08)


### Bug Fixes

* **dataplex:** Remove unused annotation in results_table ([#8382](https://github.com/googleapis/google-cloud-go/issues/8382)) ([1390cbd](https://github.com/googleapis/google-cloud-go/commit/1390cbd0deaab849c24e4b8f11589d18d81177c6))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.8.1...dataplex/v1.9.0) (2023-07-26)


### Features

* **dataplex:** New service DataTaxonomyService and related messages ([#8320](https://github.com/googleapis/google-cloud-go/issues/8320)) ([cdee2d9](https://github.com/googleapis/google-cloud-go/commit/cdee2d918015c9b0a53aa8283085214d9a11c77c))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.8.0...dataplex/v1.8.1) (2023-06-20)


### Bug Fixes

* **dataplex:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.7.1...dataplex/v1.8.0) (2023-05-30)


### Features

* **dataplex:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.7.0...dataplex/v1.7.1) (2023-05-08)


### Bug Fixes

* **dataplex:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.6.0...dataplex/v1.7.0) (2023-05-03)


### Features

* **dataplex:** Added new Dataplex APIs and new features for existing APIs (e.g. DataScans) ([d5d1fe9](https://github.com/googleapis/google-cloud-go/commit/d5d1fe96c9cf3cc3bb0e05fb75297a68bbbd8e41))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.5.2...dataplex/v1.6.0) (2023-03-15)


### Features

* **dataplex:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.5.2](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.5.1...dataplex/v1.5.2) (2023-02-14)


### Documentation

* **dataplex:** Improvements to DataScan API documentation ([f1c3ec7](https://github.com/googleapis/google-cloud-go/commit/f1c3ec753259c5c5d083f1f06960f77327b7ca61))

## [1.5.1](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.5.0...dataplex/v1.5.1) (2023-01-10)


### Documentation

* **dataplex:** Fix minor docstring formatting ([3115df4](https://github.com/googleapis/google-cloud-go/commit/3115df407cd4876d58c79e726308e9f229ceb6ed))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.4.0...dataplex/v1.5.0) (2023-01-04)


### Features

* **dataplex:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.3.0...dataplex/v1.4.0) (2022-11-03)


### Features

* **dataplex:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.2.0...dataplex/v1.3.0) (2022-10-25)


### Features

* **dataplex:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.1.0...dataplex/v1.2.0) (2022-10-14)


### Features

* **dataplex:** Add support for notebook tasks ([de4e16a](https://github.com/googleapis/google-cloud-go/commit/de4e16a498354ea7271f5b396f7cb2bb430052aa))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v1.0.0...dataplex/v1.1.0) (2022-07-19)


### Features

* **dataplex:** Add IAM support for Explore content APIs feat: Add support for custom container for Task feat: Add support for cross project for Task feat: Add support for custom encryption key to be used for encrypt data on the PDs associated with the VMs in your Dataproc cluster for Task feat: Add support for Latest job in Task resource feat: User mode filter in Explore list sessions API feat: Support logging sampled file paths per partition to Cloud logging for Discovery event ([8b17366](https://github.com/googleapis/google-cloud-go/commit/8b17366c46bbd8a0b2adf39ec3b058eb83192933))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v0.4.0...dataplex/v1.0.0) (2022-06-29)


### Features

* **dataplex:** release 1.0.0 ([7678be5](https://github.com/googleapis/google-cloud-go/commit/7678be543d9130dcd8fc4147608a10b70faef44e))


### Miscellaneous Chores

* **dataplex:** release 1.0.0 ([b165e15](https://github.com/googleapis/google-cloud-go/commit/b165e15cac74ea7b5c011b35b3f92e349e99759e))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v0.3.0...dataplex/v0.4.0) (2022-02-23)


### Features

* **dataplex:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v0.2.0...dataplex/v0.3.0) (2022-02-22)


### Features

* **dataplex:** added client side library for the followings: 1. Content APIs. 2. Create|Update|Delete Metadata APIs (e.g. Entity and/or Partition). ([7d6b0e5](https://github.com/googleapis/google-cloud-go/commit/7d6b0e5891b50cccdf77cd17ddd3644f31ef6dfc))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/dataplex/v0.1.0...dataplex/v0.2.0) (2022-02-14)


### Features

* **dataplex:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## 0.1.0 (2022-01-28)

### Features

* **dataplex:** start generating apiv1 ([#5409](https://www.github.com/googleapis/google-cloud-go/issues/5409)) ([2a2d572](https://www.github.com/googleapis/google-cloud-go/commit/2a2d572743e71d5381f6a67467782fe6416d855c))

## v0.1.0

- feat(dataplex): start generating clients
