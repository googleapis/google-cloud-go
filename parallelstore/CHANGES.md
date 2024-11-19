# Changelog

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.7.1...parallelstore/v0.8.0) (2024-11-06)


### Features

* **parallelstore:** New client(s) ([#11055](https://github.com/googleapis/google-cloud-go/issues/11055)) ([93d3f92](https://github.com/googleapis/google-cloud-go/commit/93d3f928afa6f5137b5f180dbdf6862896d43572))

## [0.7.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.7.0...parallelstore/v0.7.1) (2024-10-23)


### Bug Fixes

* **parallelstore:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **parallelstore:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.6.1...parallelstore/v0.7.0) (2024-10-09)


### Features

* **parallelstore:** Add UPGRADING state to Parallelstore state ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **parallelstore:** Cleanup of Parallelstore API descriptions ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **parallelstore:** Minor documentation formatting fix for Parallelstore ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **parallelstore:** Minor documentation formatting fix for Parallelstore ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [0.6.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.6.0...parallelstore/v0.6.1) (2024-09-12)


### Bug Fixes

* **parallelstore:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.5.1...parallelstore/v0.6.0) (2024-08-20)


### Features

* **parallelstore:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.5.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.5.0...parallelstore/v0.5.1) (2024-08-08)


### Bug Fixes

* **parallelstore:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.4.1...parallelstore/v0.5.0) (2024-08-01)


### Features

* **parallelstore:** Add file_stripe_level and directory_stripe_level fields to Instance ([#10622](https://github.com/googleapis/google-cloud-go/issues/10622)) ([2fef238](https://github.com/googleapis/google-cloud-go/commit/2fef23856e4c0738fd49d5d2aa98342a32202489))

## [0.4.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.4.0...parallelstore/v0.4.1) (2024-07-24)


### Bug Fixes

* **parallelstore:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.3.2...parallelstore/v0.4.0) (2024-07-10)


### Features

* **parallelstore:** Add iam.googleapis.com/ServiceAccount resource definition ([b660d68](https://github.com/googleapis/google-cloud-go/commit/b660d6870658fe6881883785bcebaea0929fec0a))
* **parallelstore:** Adding Import/Export BYOSA to the import Data request ([b660d68](https://github.com/googleapis/google-cloud-go/commit/b660d6870658fe6881883785bcebaea0929fec0a))


### Bug Fixes

* **parallelstore:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.3.2](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.3.1...parallelstore/v0.3.2) (2024-07-01)


### Bug Fixes

* **parallelstore:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.3.0...parallelstore/v0.3.1) (2024-06-26)


### Bug Fixes

* **parallelstore:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.2.0...parallelstore/v0.3.0) (2024-05-16)


### Features

* **parallelstore:** A new field `api_version` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `api_version` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `create_time` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `create_time` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `destination_gcs_bucket` is added to message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `destination_parallelstore` is added to message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `end_time` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `end_time` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `requested_cancellation` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `requested_cancellation` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `source_gcs_bucket` is added to message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `source_parallelstore` is added to message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `status_message` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `status_message` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `target` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `target` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `verb` is added to message `.google.cloud.parallelstore.v1beta.ExportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A new field `verb` is added to message `.google.cloud.parallelstore.v1beta.ImportDataMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))


### Bug Fixes

* **parallelstore:** An existing field `create_time` is removed from message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** An existing field `destination_path` is renamed to `destination_parallelstore` in message `.google.cloud.parallelstore.v1beta.ImportDataRequest` ([e4543f8](https://github.com/googleapis/google-cloud-go/commit/e4543f87bbad42eb37f501a4571128c3a426780b))
* **parallelstore:** An existing field `destination` is removed from message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** An existing field `end_time` is removed from message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** An existing field `source_gcs_uri` is renamed to `source_gcs_bucket` in message `.google.cloud.parallelstore.v1beta.ImportDataRequest` ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))
* **parallelstore:** An existing field `source` is removed from message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))


### Documentation

* **parallelstore:** A comment for field `counters` in message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` is changed ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **parallelstore:** A comment for field `transfer_type` in message `.google.cloud.parallelstore.v1beta.TransferOperationMetadata` is changed ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.1.1...parallelstore/v0.2.0) (2024-05-01)


### Features

* **parallelstore:** Add ImportData and ExportData RPCs ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Bug Fixes

* **parallelstore:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/parallelstore/v0.1.0...parallelstore/v0.1.1) (2024-03-14)


### Bug Fixes

* **parallelstore:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## 0.1.0 (2024-02-21)


### Features

* **parallelstore:** Introducing Parallelstore API v1beta ([7e6c208](https://github.com/googleapis/google-cloud-go/commit/7e6c208c5d97d3f6e2f7fd7aca09b8ae98dc0bf2))
* **parallelstore:** New client(s) ([#9434](https://github.com/googleapis/google-cloud-go/issues/9434)) ([3410b19](https://github.com/googleapis/google-cloud-go/commit/3410b190796edbf73f439494abcbeb204ed5e395))

## Changes
