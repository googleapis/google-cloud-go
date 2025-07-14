# Changelog

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.3.3...developerconnect/v0.4.0) (2025-06-25)


### Features

* **developerconnect:** A new enum `google.cloud.developerconnect.v1.SystemProvider` is added ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new field `bitbucket_cloud_config` is added to message `google.cloud.developerconnect.v1.Connection` ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new field `bitbucket_data_center_config` is added to message `google.cloud.developerconnect.v1.Connection` ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new field `oauth_start_uri` is added to message `google.cloud.developerconnect.v1.AccountConnector` ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new field `provider_oauth_config` is added to message `google.cloud.developerconnect.v1.AccountConnector` ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new message `google.cloud.developerconnect.v1.AccountConnector` is added ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new message `google.cloud.developerconnect.v1.GitProxyConfig` is added ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A new message `google.cloud.developerconnect.v1.User` is added ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** Add DCI insights config proto ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))


### Documentation

* **developerconnect:** A comment for field `uid` in message `.google.cloud.developerconnect.v1.Connection` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **developerconnect:** A comment for field `uid` in message `.google.cloud.developerconnect.v1.GitRepositoryLink` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))

## [0.3.3](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.3.2...developerconnect/v0.3.3) (2025-04-15)


### Bug Fixes

* **developerconnect:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.3.2](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.3.1...developerconnect/v0.3.2) (2025-03-13)


### Bug Fixes

* **developerconnect:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.3.0...developerconnect/v0.3.1) (2025-01-02)


### Bug Fixes

* **developerconnect:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.2.2...developerconnect/v0.3.0) (2024-11-19)


### Features

* **developerconnect:** A new field `crypto_key_config` is added to message `.google.cloud.developerconnect.v1.Connection` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new field `github_enterprise_config` is added to message `.google.cloud.developerconnect.v1.Connection` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new field `gitlab_config` is added to message `.google.cloud.developerconnect.v1.Connection` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new field `gitlab_enterprise_config` is added to message `.google.cloud.developerconnect.v1.Connection` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new field `webhook_id` is added to message `.google.cloud.developerconnect.v1.GitRepositoryLink` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `CryptoKeyConfig` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `GitHubEnterpriseConfig` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `GitLabConfig` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `GitLabEnterpriseConfig` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `ServiceDirectoryConfig` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new message `UserCredential` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new resource_definition `cloudkms.googleapis.com/CryptoKey` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **developerconnect:** A new resource_definition `servicedirectory.googleapis.com/Service` is added ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))


### Documentation

* **developerconnect:** A comment for field `requested_cancellation` in message `.google.cloud.developerconnect.v1.OperationMetadata` is changed ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.2.1...developerconnect/v0.2.2) (2024-10-23)


### Bug Fixes

* **developerconnect:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **developerconnect:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.2.0...developerconnect/v0.2.1) (2024-09-12)


### Bug Fixes

* **developerconnect:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.1.4...developerconnect/v0.2.0) (2024-08-20)


### Features

* **developerconnect:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.1.4](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.1.3...developerconnect/v0.1.4) (2024-08-08)


### Bug Fixes

* **developerconnect:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.1.2...developerconnect/v0.1.3) (2024-07-24)


### Bug Fixes

* **developerconnect:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.1.1...developerconnect/v0.1.2) (2024-07-10)


### Bug Fixes

* **developerconnect:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/developerconnect/v0.1.0...developerconnect/v0.1.1) (2024-07-01)


### Bug Fixes

* **developerconnect:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## 0.1.0 (2024-06-26)


### Bug Fixes

* **developerconnect:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## Changes
