# Changes

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.15.1...contactcenterinsights/v1.16.0) (2024-11-19)


### Features

* **contactcenterinsights:** Add AnalysisRules resource and APIs ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** Add FeedbackLabel resource and APIs ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** Add Quality AI resources and APIs ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** Add QueryMetrics API ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))


### Documentation

* **contactcenterinsights:** A comment for field `custom_metadata_keys` in message `.google.cloud.contactcenterinsights.v1.IngestConversationsRequest` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for field `encryption_spec` in message `.google.cloud.contactcenterinsights.v1.InitializeEncryptionSpecRequest` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for field `kms_key` in message `.google.cloud.contactcenterinsights.v1.EncryptionSpec` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for field `labels` in message `.google.cloud.contactcenterinsights.v1.Conversation` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for field `metadata_json` in message `.google.cloud.contactcenterinsights.v1.Conversation` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for field `partial_errors` in message `.google.cloud.contactcenterinsights.v1.InitializeEncryptionSpecMetadata` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for message `EncryptionSpec` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **contactcenterinsights:** A comment for method `InitializeEncryptionSpec` in service `ContactCenterInsights` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))

## [1.15.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.15.0...contactcenterinsights/v1.15.1) (2024-10-23)


### Bug Fixes

* **contactcenterinsights:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **contactcenterinsights:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.14.1...contactcenterinsights/v1.15.0) (2024-10-09)


### Features

* **contactcenterinsights:** Add CMEK InitializeLroSpec ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **contactcenterinsights:** Add import / export IssueModel ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **contactcenterinsights:** Add metadata import to IngestConversations ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **contactcenterinsights:** Add sampling to IngestConversations ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **contactcenterinsights:** Add a comment for valid `order_by` values in ListConversations ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **contactcenterinsights:** Add a comment for valid `update_mask` values in UpdateConversation ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.14.0...contactcenterinsights/v1.14.1) (2024-09-12)


### Bug Fixes

* **contactcenterinsights:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.7...contactcenterinsights/v1.14.0) (2024-08-20)


### Features

* **contactcenterinsights:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.13.7](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.6...contactcenterinsights/v1.13.7) (2024-08-08)


### Bug Fixes

* **contactcenterinsights:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.13.6](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.5...contactcenterinsights/v1.13.6) (2024-07-24)


### Bug Fixes

* **contactcenterinsights:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.13.5](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.4...contactcenterinsights/v1.13.5) (2024-07-10)


### Bug Fixes

* **contactcenterinsights:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.13.4](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.3...contactcenterinsights/v1.13.4) (2024-07-01)


### Bug Fixes

* **contactcenterinsights:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.13.3](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.2...contactcenterinsights/v1.13.3) (2024-06-26)


### Bug Fixes

* **contactcenterinsights:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.13.2](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.1...contactcenterinsights/v1.13.2) (2024-05-01)


### Bug Fixes

* **contactcenterinsights:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.13.0...contactcenterinsights/v1.13.1) (2024-03-14)


### Bug Fixes

* **contactcenterinsights:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.12.1...contactcenterinsights/v1.13.0) (2024-01-30)


### Features

* **contactcenterinsights:** Add Conversation QualityMetadata ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))


### Bug Fixes

* **contactcenterinsights:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.12.0...contactcenterinsights/v1.12.1) (2023-11-27)


### Documentation

* **contactcenterinsights:** Update IngestConversations and BulkAnalyzeConversations comments ([#9028](https://github.com/googleapis/google-cloud-go/issues/9028)) ([2020edf](https://github.com/googleapis/google-cloud-go/commit/2020edff24e3ffe127248cf9a90c67593c303e18))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.11.3...contactcenterinsights/v1.12.0) (2023-11-09)


### Features

* **contactcenterinsights:** Launch BulkDelete API, and bulk audio import via the IngestConversations API ([#8964](https://github.com/googleapis/google-cloud-go/issues/8964)) ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [1.11.3](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.11.2...contactcenterinsights/v1.11.3) (2023-11-01)


### Bug Fixes

* **contactcenterinsights:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.11.1...contactcenterinsights/v1.11.2) (2023-10-26)


### Bug Fixes

* **contactcenterinsights:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.11.0...contactcenterinsights/v1.11.1) (2023-10-12)


### Bug Fixes

* **contactcenterinsights:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.10.0...contactcenterinsights/v1.11.0) (2023-10-04)


### Features

* **contactcenterinsights:** Add optional SpeechConfig to UploadConversationRequest ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.9.1...contactcenterinsights/v1.10.0) (2023-07-10)


### Features

* **contactcenterinsights:** Support topic model type V2 ([8ff13bf](https://github.com/googleapis/google-cloud-go/commit/8ff13bf87397ad524019268c1146e44f3c1cd0e6))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.9.0...contactcenterinsights/v1.9.1) (2023-06-20)


### Bug Fixes

* **contactcenterinsights:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.8.0...contactcenterinsights/v1.9.0) (2023-06-07)


### Features

* **contactcenterinsights:** Add the resource definition of a STT recognizer ([#8035](https://github.com/googleapis/google-cloud-go/issues/8035)) ([b119cd0](https://github.com/googleapis/google-cloud-go/commit/b119cd08924ce9b4b26c6343686a76137de7375d))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.7.1...contactcenterinsights/v1.8.0) (2023-05-30)


### Features

* **contactcenterinsights:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.7.0...contactcenterinsights/v1.7.1) (2023-05-08)


### Bug Fixes

* **contactcenterinsights:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.6.0...contactcenterinsights/v1.7.0) (2023-04-11)


### Features

* **contactcenterinsights:** Launch UploadConversation endpoint ([fc90e54](https://github.com/googleapis/google-cloud-go/commit/fc90e54b25bda6b339266e3e5388174339ed6a44))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.5.0...contactcenterinsights/v1.6.0) (2023-02-14)


### Features

* **contactcenterinsights:** Add IngestConversationsStats ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.4.0...contactcenterinsights/v1.5.0) (2023-01-04)


### Features

* **contactcenterinsights:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.3.0...contactcenterinsights/v1.4.0) (2022-11-03)


### Features

* **contactcenterinsights:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.2.3...contactcenterinsights/v1.3.0) (2022-10-25)


### Features

* **contactcenterinsights:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.2.3](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.2.2...contactcenterinsights/v1.2.3) (2022-08-23)


### Documentation

* **contactcenterinsights:** Updating comments chore: added LRO to API list ([7b01462](https://github.com/googleapis/google-cloud-go/commit/7b014626f07dd2974d6f72f926c2df1d25fa1f4a))

## [1.2.2](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.2.1...contactcenterinsights/v1.2.2) (2022-08-02)


### Documentation

* **contactcenterinsights:** Updating comments chore:remove LRO to API list ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.2.0...contactcenterinsights/v1.2.1) (2022-07-12)


### Documentation

* **contactcenterinsights:** Updating comments ([4134941](https://github.com/googleapis/google-cloud-go/commit/41349411e601f57dc6d9e246f1748fd86d17bb15))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.1.0...contactcenterinsights/v1.2.0) (2022-02-23)


### Features

* **contactcenterinsights:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v1.0.0...contactcenterinsights/v1.1.0) (2022-02-14)


### Features

* **contactcenterinsights:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.0.0](https://www.github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v0.4.0...contactcenterinsights/v1.0.0) (2022-01-25)


### Features

* **contactcenterinsights:** to v1 ([#5137](https://www.github.com/googleapis/google-cloud-go/issues/5137)) ([7618f2a](https://www.github.com/googleapis/google-cloud-go/commit/7618f2af910bc32e293c5b80f6d897adfd6f0ad5))

## [0.4.0](https://www.github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v0.3.0...contactcenterinsights/v0.4.0) (2022-01-04)


### Features

* **contactcenterinsights:** Add ability to update phrase matchers feat: Add issue model stats to time series feat: Add display name to issue model stats ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))
* **contactcenterinsights:** Add WriteDisposition to BigQuery Export API ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))
* **contactcenterinsights:** new feature flag disable_issue_modeling docs: fixed formatting issues in the reference documentation ([c8271d4](https://www.github.com/googleapis/google-cloud-go/commit/c8271d4b217a6e6924d9f87eac9468c4b5767ba7))
* **contactcenterinsights:** remove feature flag disable_issue_modeling ([c8271d4](https://www.github.com/googleapis/google-cloud-go/commit/c8271d4b217a6e6924d9f87eac9468c4b5767ba7))

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v0.2.0...contactcenterinsights/v0.3.0) (2021-10-18)

### Features

* **contactcenterinsights:** add metadata from dialogflow related to transcript segment feat: add sentiment data for transcript segment feat: add obfuscated if from dialogflow ([12928d4](https://www.github.com/googleapis/google-cloud-go/commit/12928d47de771f4b23577062afe5cf551b347919))
* **contactcenterinsights:** deprecate issue_matches docs: if conversation medium is unspecified, it will default to PHONE_CALL ([1a0720f](https://www.github.com/googleapis/google-cloud-go/commit/1a0720f2f33bb14617f5c6a524946a93209e1266))

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/contactcenterinsights/v0.1.0...contactcenterinsights/v0.2.0) (2021-09-21)

### Features

* **contactcenterinsights:** filter is used to filter conversations used for issue model training feat: update_time is used to indicate when the phrase matcher was updated ([cb3a823](https://www.github.com/googleapis/google-cloud-go/commit/cb3a8236252fe0e64abb6f98448c5bf9d085d448))

## v0.1.0

- feat(contactcenterinsights): start generating clients
