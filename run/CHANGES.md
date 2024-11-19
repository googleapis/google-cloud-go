# Changelog


## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.7.0...run/v1.8.0) (2024-11-19)


### Features

* **run:** Support manual instance count in Cloud Run for manual scaling feature ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.6.1...run/v1.7.0) (2024-11-14)


### Features

* **run:** Add EncryptionKeyRevocationAction and shutdown duration configuration to Services ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))


### Documentation

* **run:** Fixed formatting of some documentation ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.6.0...run/v1.6.1) (2024-10-23)


### Bug Fixes

* **run:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **run:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.5.1...run/v1.6.0) (2024-10-09)


### Features

* **run:** Add Builds API ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **run:** Add GPU configuration to Services ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **run:** Add INGRESS_TRAFFIC_NONE to Services ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **run:** Add Service Mesh configuration to Services ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **run:** Add ServiceScaling to Services ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **run:** Fixed formatting of some documentation ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [1.5.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.5.0...run/v1.5.1) (2024-09-12)


### Bug Fixes

* **run:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.4.1...run/v1.5.0) (2024-08-20)


### Features

* **run:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.4.0...run/v1.4.1) (2024-08-08)


### Bug Fixes

* **run:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.10...run/v1.4.0) (2024-07-24)


### Features

* **run:** Add Job ExecutionReference.completion_status to show status of the most recent execution ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **run:** Add Job start_execution_token and run_execution_token to execute jobs immediately on creation ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **run:** Support update_mask in Cloud Run UpdateService ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))


### Bug Fixes

* **run:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **run:** Clarify optional fields in Cloud Run requests ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))

## [1.3.10](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.9...run/v1.3.10) (2024-07-10)


### Bug Fixes

* **run:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.3.9](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.8...run/v1.3.9) (2024-07-01)


### Bug Fixes

* **run:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.3.8](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.7...run/v1.3.8) (2024-06-26)


### Bug Fixes

* **run:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.3.7](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.6...run/v1.3.7) (2024-05-01)


### Bug Fixes

* **run:** Add internaloption.WithDefaultEndpointTemplate ([3b41408](https://github.com/googleapis/google-cloud-go/commit/3b414084450a5764a0248756e95e13383a645f90))
* **run:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.3.6](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.5...run/v1.3.6) (2024-03-14)


### Bug Fixes

* **run:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.3.5](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.4...run/v1.3.5) (2024-03-07)


### Documentation

* **run:** Clarify some defaults and required or optional values ([#9505](https://github.com/googleapis/google-cloud-go/issues/9505)) ([1cf28f6](https://github.com/googleapis/google-cloud-go/commit/1cf28f61b26d52a9e2303c52e9aba7a0cdfbe7eb))

## [1.3.4](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.3...run/v1.3.4) (2024-01-30)


### Bug Fixes

* **run:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.3.3](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.2...run/v1.3.3) (2023-11-01)


### Bug Fixes

* **run:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.3.2](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.1...run/v1.3.2) (2023-10-26)


### Bug Fixes

* **run:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.3.0...run/v1.3.1) (2023-10-12)


### Bug Fixes

* **run:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.2.0...run/v1.3.0) (2023-10-04)


### Features

* **run:** Adds support for cancel Execution ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))


### Bug Fixes

* **run:** Removes accidentally exposed field service.traffic_tags_cleanup_threshold in Cloud Run Service ([57fc1a6](https://github.com/googleapis/google-cloud-go/commit/57fc1a6de326456eb68ef25f7a305df6636ed386))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.1.1...run/v1.2.0) (2023-07-10)


### Features

* **run:** Adds support for custom audiences ([#8227](https://github.com/googleapis/google-cloud-go/issues/8227)) ([7732b8c](https://github.com/googleapis/google-cloud-go/commit/7732b8c2c19aef0fad4a7bae6d4bd7354018cfc4))

## [1.1.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.1.0...run/v1.1.1) (2023-06-20)


### Bug Fixes

* **run:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/run/v1.0.1...run/v1.1.0) (2023-05-30)


### Features

* **run:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/run/v1.0.0...run/v1.0.1) (2023-05-08)


### Bug Fixes

* **run:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.9.0...run/v1.0.0) (2023-04-04)


### Features

* **run:** Promote to GA ([#7617](https://github.com/googleapis/google-cloud-go/issues/7617)) ([4cb997e](https://github.com/googleapis/google-cloud-go/commit/4cb997e9805872d8084432f209c629e40dc55cf7))
* **run:** Promote to GA ([#7641](https://github.com/googleapis/google-cloud-go/issues/7641)) ([a1280ef](https://github.com/googleapis/google-cloud-go/commit/a1280ef3f8627b52492ae8c25e64197451b8807c))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.8.0...run/v0.9.0) (2023-03-15)


### Features

* **run:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/run-v0.7.0...run/v0.8.0) (2023-01-26)


### Features

* **run:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **run:** Adding support for encryption_key_revocation_action and encryption_key_shutdown_duration for RevisionTemplate and ExecutionTemplate docs: Documentation improvements, including clarification that v1 labels/annotations are rejected in v2 API ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))
* **run:** Adds Cloud Run Jobs v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **run:** Adds gRPC probe support to Cloud Run v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **run:** Adds Startup and Liveness probes to Cloud Run v2 API client libraries ([8b203b8](https://github.com/googleapis/google-cloud-go/commit/8b203b8aea4dada5c0846a515b14414cd8c58f78))
* **run:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **run:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **run:** Fix the main client gem name listed in the readme ([a679a5a](https://github.com/googleapis/google-cloud-go/commit/a679a5a9b1ea60cb155eb6c8be4afcc43d3b121f))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/run-v0.6.0...run/v0.7.0) (2023-01-26)


### Features

* **run:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **run:** Adding support for encryption_key_revocation_action and encryption_key_shutdown_duration for RevisionTemplate and ExecutionTemplate docs: Documentation improvements, including clarification that v1 labels/annotations are rejected in v2 API ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))
* **run:** Adds Cloud Run Jobs v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **run:** Adds gRPC probe support to Cloud Run v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **run:** Adds Startup and Liveness probes to Cloud Run v2 API client libraries ([8b203b8](https://github.com/googleapis/google-cloud-go/commit/8b203b8aea4dada5c0846a515b14414cd8c58f78))
* **run:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **run:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **run:** Fix the main client gem name listed in the readme ([a679a5a](https://github.com/googleapis/google-cloud-go/commit/a679a5a9b1ea60cb155eb6c8be4afcc43d3b121f))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.5.0...run/v0.6.0) (2023-01-26)


### Features

* **run:** Adding support for encryption_key_revocation_action and encryption_key_shutdown_duration for RevisionTemplate and ExecutionTemplate docs: Documentation improvements, including clarification that v1 labels/annotations are rejected in v2 API ([19e9d03](https://github.com/googleapis/google-cloud-go/commit/19e9d033c263e889d32b74c4c853c440ce136d68))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.4.0...run/v0.5.0) (2023-01-04)


### Features

* **run:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.3.0...run/v0.4.0) (2022-11-09)


### Features

* **run:** Adds Cloud Run Jobs v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **run:** Adds gRPC probe support to Cloud Run v2 API client libraries ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.2.0...run/v0.3.0) (2022-11-03)


### Features

* **run:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/run/v0.1.2...run/v0.2.0) (2022-10-25)


### Features

* **run:** Adds Startup and Liveness probes to Cloud Run v2 API client libraries ([8b203b8](https://github.com/googleapis/google-cloud-go/commit/8b203b8aea4dada5c0846a515b14414cd8c58f78))
* **run:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/run/v0.1.1...run/v0.1.2) (2022-09-15)


### Documentation

* **run:** Fix the main client gem name listed in the readme ([a679a5a](https://github.com/googleapis/google-cloud-go/commit/a679a5a9b1ea60cb155eb6c8be4afcc43d3b121f))

### [0.1.1](https://github.com/googleapis/google-cloud-go/compare/run/v0.1.0...run/v0.1.1) (2022-05-24)


### Bug Fixes

* **run:** Updates pre-release Cloud Run v2 Preview client libraries to work with the latest API revision ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

## 0.1.0 (2022-04-06)


### Features

* **run:** start generating apiv2 ([#5825](https://github.com/googleapis/google-cloud-go/issues/5825)) ([2602a20](https://github.com/googleapis/google-cloud-go/commit/2602a20ca8eba1ba2b1e59bb27a7b44132d63032))
