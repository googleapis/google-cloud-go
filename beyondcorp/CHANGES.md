# Changes


## [1.2.0](https://github.com/googleapis/google-cloud-go/releases/tag/beyondcorp%2Fv1.2.0) (2025-10-09)

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

## [1.1.6](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.5...beyondcorp/v1.1.6) (2025-04-15)


### Bug Fixes

* **beyondcorp:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.1.5](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.4...beyondcorp/v1.1.5) (2025-03-13)


### Bug Fixes

* **beyondcorp:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.1.4](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.3...beyondcorp/v1.1.4) (2025-03-06)


### Bug Fixes

* **beyondcorp:** Fix out-of-sync version.go ([28f0030](https://github.com/googleapis/google-cloud-go/commit/28f00304ebb13abfd0da2f45b9b79de093cca1ec))

## [1.1.3](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.2...beyondcorp/v1.1.3) (2025-01-02)


### Bug Fixes

* **beyondcorp:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [1.1.2](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.1...beyondcorp/v1.1.2) (2024-10-23)


### Bug Fixes

* **beyondcorp:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **beyondcorp:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.1.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.1.0...beyondcorp/v1.1.1) (2024-09-12)


### Bug Fixes

* **beyondcorp:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.11...beyondcorp/v1.1.0) (2024-08-20)


### Features

* **beyondcorp:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.0.11](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.10...beyondcorp/v1.0.11) (2024-08-08)


### Bug Fixes

* **beyondcorp:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.0.10](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.9...beyondcorp/v1.0.10) (2024-07-24)


### Bug Fixes

* **beyondcorp:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.0.9](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.8...beyondcorp/v1.0.9) (2024-07-10)


### Bug Fixes

* **beyondcorp:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.0.8](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.7...beyondcorp/v1.0.8) (2024-07-01)


### Bug Fixes

* **beyondcorp:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.0.7](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.6...beyondcorp/v1.0.7) (2024-06-26)


### Bug Fixes

* **beyondcorp:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.0.6](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.5...beyondcorp/v1.0.6) (2024-05-01)


### Bug Fixes

* **beyondcorp:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.0.5](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.4...beyondcorp/v1.0.5) (2024-03-14)


### Bug Fixes

* **beyondcorp:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.0.4](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.3...beyondcorp/v1.0.4) (2024-01-30)


### Bug Fixes

* **beyondcorp:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.0.3](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.2...beyondcorp/v1.0.3) (2023-11-01)


### Bug Fixes

* **beyondcorp:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.0.2](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.1...beyondcorp/v1.0.2) (2023-10-26)


### Bug Fixes

* **beyondcorp:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v1.0.0...beyondcorp/v1.0.1) (2023-10-12)


### Bug Fixes

* **beyondcorp:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.6.1...beyondcorp/v1.0.0) (2023-07-18)


### Features

* **beyondcorp:** Promote to GA ([#8285](https://github.com/googleapis/google-cloud-go/issues/8285)) ([02413ca](https://github.com/googleapis/google-cloud-go/commit/02413cacf908ce35df8dde6ae75c954917564ae7))

## [0.6.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.6.0...beyondcorp/v0.6.1) (2023-06-20)


### Bug Fixes

* **beyondcorp:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.5.1...beyondcorp/v0.6.0) (2023-05-30)


### Features

* **beyondcorp:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [0.5.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.5.0...beyondcorp/v0.5.1) (2023-05-08)


### Bug Fixes

* **beyondcorp:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.4.0...beyondcorp/v0.5.0) (2023-03-15)


### Features

* **beyondcorp:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.3.1...beyondcorp/v0.4.0) (2023-01-04)


### Features

* **beyondcorp:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.3.0...beyondcorp/v0.3.1) (2022-12-01)


### Documentation

* **beyondcorp/appconnections:** fix minor docstring formatting ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.2.0...beyondcorp/v0.3.0) (2022-11-03)


### Features

* **beyondcorp:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.1.1...beyondcorp/v0.2.0) (2022-10-25)


### Features

* **beyondcorp:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/beyondcorp/v0.1.0...beyondcorp/v0.1.1) (2022-09-15)


### Documentation

* **beyondcorp/appconnections:** Fix the main client gem name listed in the readme ([6a0080a](https://github.com/googleapis/google-cloud-go/commit/6a0080ad69398c572d856886293e19c79cf0fc0e))

## 0.1.0 (2022-07-12)


### Features

* **beyondcorp/appconnections:** start generating apiv1 ([#6325](https://github.com/googleapis/google-cloud-go/issues/6325)) ([f29db2f](https://github.com/googleapis/google-cloud-go/commit/f29db2fc5bf1c839065fa35e15827bd227626dca))
* **beyondcorp/appconnectors:** start generating apiv1 ([7066fed](https://github.com/googleapis/google-cloud-go/commit/7066fedc31fa4c19e851477792bd8de8e50541ab))
* **beyondcorp/appgateways:** start generating apiv1 ([7066fed](https://github.com/googleapis/google-cloud-go/commit/7066fedc31fa4c19e851477792bd8de8e50541ab))
* **beyondcorp/clientconnectorservices:** start generating apiv1 ([7066fed](https://github.com/googleapis/google-cloud-go/commit/7066fedc31fa4c19e851477792bd8de8e50541ab))
* **beyondcorp/clientgateways:** start generating apiv1 ([7066fed](https://github.com/googleapis/google-cloud-go/commit/7066fedc31fa4c19e851477792bd8de8e50541ab))
