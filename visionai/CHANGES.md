# Changelog

## [0.4.2](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.4.1...visionai/v0.4.2) (2024-10-23)


### Bug Fixes

* **visionai:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **visionai:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [0.4.1](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.4.0...visionai/v0.4.1) (2024-09-12)


### Bug Fixes

* **visionai:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.3.0...visionai/v0.4.0) (2024-08-27)


### Features

* **visionai:** Add BatchOperationStatus to import metadata ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))
* **visionai:** Request client libraries for new languages ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))


### Documentation

* **visionai:** A comment for enum value `FAILED` in enum `State` is changed ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))
* **visionai:** A comment for enum value `IN_PROGRESS` in enum `State` is changed ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))
* **visionai:** A comment for enum value `SUCCEEDED` in enum `State` is changed ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))
* **visionai:** A comment for field `relevance` in message `.google.cloud.visionai.v1.SearchResultItem` is changed ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))
* **visionai:** A comment for method `ClipAsset` in service `Warehouse` is changed ([2b2c673](https://github.com/googleapis/google-cloud-go/commit/2b2c673ade81b686fa579b49e557d51853aa370a))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.5...visionai/v0.3.0) (2024-08-20)


### Features

* **visionai:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.2.5](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.4...visionai/v0.2.5) (2024-08-08)


### Bug Fixes

* **visionai:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.2.4](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.3...visionai/v0.2.4) (2024-07-24)


### Bug Fixes

* **visionai:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.2.3](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.2...visionai/v0.2.3) (2024-07-10)


### Bug Fixes

* **visionai:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.1...visionai/v0.2.2) (2024-07-01)


### Bug Fixes

* **visionai:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.2.0...visionai/v0.2.1) (2024-06-26)


### Bug Fixes

* **visionai:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.1.2...visionai/v0.2.0) (2024-05-16)


### Features

* **visionai:** Expose confidence score filter ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))


### Bug Fixes

* **visionai:** Changed proto3 optional flag of an existing field `granularity` in message `.google.cloud.visionai.v1.DataSchemaDetails` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **visionai:** Changed proto3 optional flag of an existing field `search_strategy_type` in message `.google.cloud.visionai.v1.DataSchemaDetails` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **visionai:** Changed proto3 optional flag of an existing field `type` in message `.google.cloud.visionai.v1.DataSchemaDetails` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))


### Documentation

* **visionai:** A comment for field `filter` in message `.google.cloud.visionai.v1.ListAnnotationsRequest` is changed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **visionai:** A comment for field `hypernym` in message `.google.cloud.visionai.v1.SearchHypernym` is changed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **visionai:** A comment for field `hyponyms` in message `.google.cloud.visionai.v1.SearchHypernym` is changed ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.1.1...visionai/v0.1.2) (2024-05-01)


### Bug Fixes

* **visionai:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/visionai/v0.1.0...visionai/v0.1.1) (2024-03-14)


### Bug Fixes

* **visionai:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## 0.1.0 (2024-01-30)


### Features

* **visionai:** New clients ([#9333](https://github.com/googleapis/google-cloud-go/issues/9333)) ([4315cdf](https://github.com/googleapis/google-cloud-go/commit/4315cdf6bfdcd9ed6e9137254451eabbc5cb420b))

## Changes
