# Changes


## [1.4.0](https://github.com/googleapis/google-cloud-go/releases/tag/baremetalsolution%2Fv1.4.0) (2025-10-09)

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

## [1.3.6](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.5...baremetalsolution/v1.3.6) (2025-04-15)


### Bug Fixes

* **baremetalsolution:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.3.5](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.4...baremetalsolution/v1.3.5) (2025-03-13)


### Bug Fixes

* **baremetalsolution:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.3.4](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.3...baremetalsolution/v1.3.4) (2025-03-06)


### Bug Fixes

* **baremetalsolution:** Fix out-of-sync version.go ([28f0030](https://github.com/googleapis/google-cloud-go/commit/28f00304ebb13abfd0da2f45b9b79de093cca1ec))

## [1.3.3](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.2...baremetalsolution/v1.3.3) (2025-01-02)


### Bug Fixes

* **baremetalsolution:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [1.3.2](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.1...baremetalsolution/v1.3.2) (2024-10-23)


### Bug Fixes

* **baremetalsolution:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **baremetalsolution:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.3.0...baremetalsolution/v1.3.1) (2024-09-12)


### Bug Fixes

* **baremetalsolution:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.11...baremetalsolution/v1.3.0) (2024-08-20)


### Features

* **baremetalsolution:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.2.11](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.10...baremetalsolution/v1.2.11) (2024-08-08)


### Bug Fixes

* **baremetalsolution:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.2.10](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.9...baremetalsolution/v1.2.10) (2024-07-24)


### Bug Fixes

* **baremetalsolution:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.2.9](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.8...baremetalsolution/v1.2.9) (2024-07-10)


### Bug Fixes

* **baremetalsolution:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.2.8](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.7...baremetalsolution/v1.2.8) (2024-07-01)


### Bug Fixes

* **baremetalsolution:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.2.7](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.6...baremetalsolution/v1.2.7) (2024-06-26)


### Bug Fixes

* **baremetalsolution:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.2.6](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.5...baremetalsolution/v1.2.6) (2024-05-01)


### Bug Fixes

* **baremetalsolution:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.2.5](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.4...baremetalsolution/v1.2.5) (2024-03-14)


### Bug Fixes

* **baremetalsolution:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.2.4](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.3...baremetalsolution/v1.2.4) (2024-01-30)


### Bug Fixes

* **baremetalsolution:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.2.3](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.2...baremetalsolution/v1.2.3) (2023-11-01)


### Bug Fixes

* **baremetalsolution:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.2.2](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.1...baremetalsolution/v1.2.2) (2023-10-26)


### Bug Fixes

* **baremetalsolution:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.2.0...baremetalsolution/v1.2.1) (2023-10-12)


### Bug Fixes

* **baremetalsolution:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.1.1...baremetalsolution/v1.2.0) (2023-08-14)


### Features

* **baremetalsolution:** Several new resources and RPCs ([fcb41cc](https://github.com/googleapis/google-cloud-go/commit/fcb41cc1d2435452ee78314c1b0362e3f21ae637))

## [1.1.1](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.1.0...baremetalsolution/v1.1.1) (2023-06-20)


### Bug Fixes

* **baremetalsolution:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.0.1...baremetalsolution/v1.1.0) (2023-05-30)


### Features

* **baremetalsolution:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v1.0.0...baremetalsolution/v1.0.1) (2023-05-08)


### Bug Fixes

* **baremetalsolution:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v0.5.0...baremetalsolution/v1.0.0) (2023-04-04)


### Features

* **baremetalsolution:** Promote to GA ([597ea0f](https://github.com/googleapis/google-cloud-go/commit/597ea0fe09bcea04e884dffe78add850edb2120d))
* **baremetalsolution:** Promote to GA ([#7644](https://github.com/googleapis/google-cloud-go/issues/7644)) ([9e26142](https://github.com/googleapis/google-cloud-go/commit/9e261425af910ec56acee6ed842848995b9d0d65))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v0.4.0...baremetalsolution/v0.5.0) (2023-01-04)


### Features

* **baremetalsolution:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v0.3.0...baremetalsolution/v0.4.0) (2022-11-03)


### Features

* **baremetalsolution:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v0.2.0...baremetalsolution/v0.3.0) (2022-10-25)


### Features

* **baremetalsolution:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/baremetalsolution/v0.1.0...baremetalsolution/v0.2.0) (2022-06-21)


### Features

* **baremetalsolution:** add support for new API methods, The removed Snapshots methods were never officially released ([f9ae8d4](https://github.com/googleapis/google-cloud-go/commit/f9ae8d41d160908aabc544e780a7f90c4fceb667))

## 0.1.0 (2022-06-16)


### Features

* **baremetalsolution:** start generating apiv2 ([#6147](https://github.com/googleapis/google-cloud-go/issues/6147)) ([5dcbf2f](https://github.com/googleapis/google-cloud-go/commit/5dcbf2f859e2b99e5497d6ac45825a80799f32ab))
