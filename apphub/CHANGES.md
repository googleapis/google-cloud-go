# Changes

## [0.4.0](https://github.com/googleapis/google-cloud-go/releases/tag/apphub%2Fv0.4.0) (2025-10-09)

### Features

* A new field 'enforced_retention_end_time' in message 'google.cloud.netapp.v1.Backup' is added ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A new message 'google.cloud.netapp.v1.BackupRetentionPolicy' is added in 'google.cloud.netapp.v1.BackupVault' ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A new method_signature `parent,online_return_policy` is added to method `CreateOnlineReturnPolicy` in service `OnlineReturnPolicyService` ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* Add OmnichannelSetingsService, LfpProvidersService and GbpAccountsService PiperOrigin-RevId: 759329567 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Add new CSQL API for supporting Cluster creation from Cloud SQL ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Add new fields to support observability configurations, machine types and PSC related configs ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Add new methods for exporting, importing and upgrade Cluster operations ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Adding eTag field to AutokeyConfig ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* New fields 'custom_performance_enabled', 'total_throughput_mibps', 'total_iops' in message 'google.cloud.netapp.v1.StoragePool' are added ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Support adding a workflow action to execute a Data Preparation node ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Sync AlloyDB API changes from HEAD to stable ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Tuning Checkpoints API ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Tuning Checkpoints API PiperOrigin-RevId: 757844206 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Update Compute Engine v1 API to revision 20250511 (#1047) (#12396) ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* Update Compute Engine v1beta API to revision 20250511 (#1041) (#12298) ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* add ANN feature for RagManagedDb PiperOrigin-RevId: 757834804 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* add encryption_spec to Model Monitoring public preview API PiperOrigin-RevId: 759653857 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* add new change_stream.proto PiperOrigin-RevId: 766241102 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add scenarios AUTO/NONE to autotuning config PiperOrigin-RevId: 766437023 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add throughput_mode to UpdateDatabaseDdlRequest to be used by Spanner Migration Tool. See https (#12287) ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* adding thoughts_token_count to prediction service PiperOrigin-RevId: 759720969 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* adding thoughts_token_count to v1beta1 client library PiperOrigin-RevId: 759721742 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* new field `additional_properties` is added to message `.google.cloud.aiplatform.v1.Schema` PiperOrigin-RevId: 757829708 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* new field `additional_properties` is added to message `.google.cloud.aiplatform.v1beta1.Schema` PiperOrigin-RevId: 757839731 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))

### Bug Fixes

* upgrade gRPC service registration func An update to Go gRPC Protobuf generation will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go service files. This change should affect very few client library users. See release notes advisories in https://togithub.com/googleapis/google-cloud-go/pull/11025. ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))

### Documentation

* A comment for enum value `DESTROYED` in enum `CryptoKeyVersionState` is changed PiperOrigin-RevId: 759163334 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A comment for field `accept_defective_only` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `accept_exchange` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `database_flags` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `encryption_config` in message `.google.cloud.alloydb.v1.AutomatedBackupPolicy` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `encryption_config` in message `.google.cloud.alloydb.v1.ContinuousBackupConfig` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `id` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `ip` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `parent` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `process_refund_days` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `requested_cancellation` in message `.google.cloud.alloydb.v1.OperationMetadata` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `return_label_source` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `state` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `transfer_bytes` in message `.google.cloud.netapp.v1.TransferStats` is changed ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A comment for field `update_mask` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `use_metadata_exchange` in message `.google.cloud.alloydb.v1.GenerateClientCertificateRequest` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `user` in message `.google.cloud.alloydb.v1.ExecuteSqlRequest` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `zone_id` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for message `Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for message `UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for method `DeleteOnlineReturnPolicy` in service `OnlineReturnPolicyService` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* Annotate all names with IDENTIFIER ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* Remove comments for a non public feature (#12243) ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Updated the formatting in some comments in multiple services ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Updating docs for total_size field in KMS List APIs ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Use backticks around `username` in documentation for `Actor.email` ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* fix links and typos ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.3.0...apphub/v0.3.1) (2025-04-15)


### Bug Fixes

* **apphub:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.2.4...apphub/v0.3.0) (2025-03-27)


### Features

* **apphub:** Add enum `Type.GLOBAL` ([a21d596](https://github.com/googleapis/google-cloud-go/commit/a21d5965fa3f4322da9563425350ba1079279d5a))


### Documentation

* **apphub:** Misc comment updates, formatting ([a21d596](https://github.com/googleapis/google-cloud-go/commit/a21d5965fa3f4322da9563425350ba1079279d5a))

## [0.2.4](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.2.3...apphub/v0.2.4) (2025-03-13)


### Bug Fixes

* **apphub:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.2.3](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.2.2...apphub/v0.2.3) (2025-01-02)


### Bug Fixes

* **apphub:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.2.1...apphub/v0.2.2) (2024-10-23)


### Bug Fixes

* **apphub:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **apphub:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.2.0...apphub/v0.2.1) (2024-09-12)


### Bug Fixes

* **apphub:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.6...apphub/v0.2.0) (2024-08-20)


### Features

* **apphub:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.1.6](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.5...apphub/v0.1.6) (2024-08-08)


### Bug Fixes

* **apphub:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.1.5](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.4...apphub/v0.1.5) (2024-07-24)


### Bug Fixes

* **apphub:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.1.4](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.3...apphub/v0.1.4) (2024-07-10)


### Bug Fixes

* **apphub:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.2...apphub/v0.1.3) (2024-07-01)


### Bug Fixes

* **apphub:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.1...apphub/v0.1.2) (2024-06-26)


### Bug Fixes

* **apphub:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/apphub/v0.1.0...apphub/v0.1.1) (2024-05-01)


### Bug Fixes

* **apphub:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## 0.1.0 (2024-03-12)


### Features

* **apphub:** New client(s) ([#9564](https://github.com/googleapis/google-cloud-go/issues/9564)) ([b8cf40b](https://github.com/googleapis/google-cloud-go/commit/b8cf40bcf6bd27744ce7f8f970896e68f4313f93))

## Changes
