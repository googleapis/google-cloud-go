# Changelog

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
