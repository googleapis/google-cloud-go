# Changes

## [1.15.0](https://github.com/googleapis/google-cloud-go/releases/tag/automl%2Fv1.15.0) (2025-10-09)

### Features

* A new field 'enforced_retention_end_time' in message 'google.cloud.netapp.v1.Backup' is added ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A new field `catalog_publishing_enabled` is added to message `.google.cloud.dataplex.v1.DataQualitySpec` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `catalog_publishing_status` is added to message `.google.cloud.dataplex.v1.DataQualityResult` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `catalog_publishing_status` is added to message `.google.cloud.dataplex.v1.DataScanEvent` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `created_entry_links` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `deleted_entry_links` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `dimensions` is added to message `.google.cloud.dataplex.v1.DataQualityColumnResult` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `entry_link_types` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `entry_link` is added to message `.google.cloud.dataplex.v1.ImportItem` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `glossaries` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `passed` is added to message `.google.cloud.dataplex.v1.DataQualityColumnResult` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `project` is added to message `.google.cloud.dataplex.v1.DataDiscoverySpec` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `referenced_entry_scopes` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new field `unchanged_entry_links` is added to message `.google.cloud.dataplex.v1.MetadataJob` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message 'google.cloud.netapp.v1.BackupRetentionPolicy' is added in 'google.cloud.netapp.v1.BackupVault' ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A new message `CreateEntryLinkRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `CreateGlossaryCategoryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `CreateGlossaryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `CreateGlossaryTermRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `DataScanCatalogPublishingStatus` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `DeleteEntryLinkRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `DeleteGlossaryCategoryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `DeleteGlossaryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `DeleteGlossaryTermRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `EntryLink` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GetEntryLinkRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GetGlossaryCategoryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GetGlossaryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GetGlossaryTermRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GlossaryCategory` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `GlossaryTerm` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `Glossary` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossariesRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossariesResponse` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossaryCategoriesRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossaryCategoriesResponse` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossaryTermsRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `ListGlossaryTermsResponse` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `UpdateGlossaryCategoryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `UpdateGlossaryRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new message `UpdateGlossaryTermRequest` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new method `CreateEntryLink` is added to service `CatalogService` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new method `DeleteEntryLink` is added to service `CatalogService` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new method `GetEntryLink` is added to service `CatalogService` ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new method_signature `parent,online_return_policy` is added to method `CreateOnlineReturnPolicy` in service `OnlineReturnPolicyService` ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A new resource_definition `dataplex.googleapis.com/EntryLink` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new resource_definition `dataplex.googleapis.com/GlossaryCategory` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new resource_definition `dataplex.googleapis.com/GlossaryTerm` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new resource_definition `dataplex.googleapis.com/Glossary` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A new service `BusinessGlossaryService` is added ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* API for extending the time to live (TTL) of a Migrating VM ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Add OmnichannelSetingsService, LfpProvidersService and GbpAccountsService PiperOrigin-RevId: 759329567 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Add PublicKeyFormat enums XWING_RAW_BYTES (used for KEM_XWING) and DER ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Add full lifecycle management for API Operations within API Versions (Create, Update, Delete) ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Add new CSQL API for supporting Cluster creation from Cloud SQL ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Add new GetIamPolicy, SetIamPolicy, and TestIamPermissions RPCs ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Add new fields and enums to resources to support richer metadata, including source tracking (SourceMetadata), plugin configurations (AuthConfig, ConfigVariable), new attributes, and additional deployment details ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Add new fields to support observability configurations, machine types and PSC related configs ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Add new methods for exporting, importing and upgrade Cluster operations ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Added `ranking_expression_backed` and `rank_signals` fields related to the Custom Ranking feature ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Added client library for capacity planner services ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Adding eTag field to AutokeyConfig ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Azure as a source ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* CMEK support ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Cutover forecast ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Enable Deletion of ApiHub Instances via the Provisioning service ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Enhance list filtering options across various resources (APIs, Versions, Specs, Operations, Deployments) with support for user-defined attributes ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Image Import ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Introduce new services for data collection (ApiHubCollect) and curation (ApiHubCurate) ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Machine Image Import ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Make CMEK configuration optional for ApiHub Instances, defaulting to GMEK ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Migration warnings in addition to errors ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Multiple additional supported target details ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* New fields 'custom_performance_enabled', 'total_throughput_mibps', 'total_iops' in message 'google.cloud.netapp.v1.StoragePool' are added ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* OS capabilities detection ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* PSC support for custom weights deploy ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Significantly expand Plugin and Plugin Instance management capabilities, including creation, execution, and lifecycle control ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Support KEY_ENCAPSULATION purpose and quantum-safe algorithms ML_KEM_768, ML_KEM_1024 and KEM_XWING ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Support adding a workflow action to execute a Data Preparation node ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Sync AlloyDB API changes from HEAD to stable ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Tuning Checkpoints API ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Tuning Checkpoints API PiperOrigin-RevId: 757844206 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Tuning PreTunedModel API field ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Update Compute Engine v1 API to revision 20250511 (#1047) (#12396) ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* Update Compute Engine v1beta API to revision 20250511 (#1041) (#12298) ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* Update Compute Engine v1beta API to revision 20250902 Source-Link: https://togithub.com/googleapis/googleapis/commit/efa74ff3562449aa87877fc7ce63fdb05fec6917 ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* VM disk migration ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* add ANN feature for RagManagedDb PiperOrigin-RevId: 757834804 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* add GCE to DeploymentPlatform enum PiperOrigin-RevId: 805463393 ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* add LocationSupport,Domain,DocumentFallbackLocation ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* add encryption_spec to Model Monitoring public preview API PiperOrigin-RevId: 759653857 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* add new change_stream.proto PiperOrigin-RevId: 766241102 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add scenarios AUTO/NONE to autotuning config PiperOrigin-RevId: 766437023 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add throughput_mode to UpdateDatabaseDdlRequest to be used by Spanner Migration Tool. See https (#12287) ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* adding thoughts_token_count to prediction service PiperOrigin-RevId: 759720969 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* adding thoughts_token_count to v1beta1 client library PiperOrigin-RevId: 759721742 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* introduce DataTransfer APIs ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* new field `additional_properties` is added to message `.google.cloud.aiplatform.v1.Schema` PiperOrigin-RevId: 757829708 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* new field `additional_properties` is added to message `.google.cloud.aiplatform.v1beta1.Schema` PiperOrigin-RevId: 757839731 ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* release the conversational search public SDK ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* release the latest conversational search public SDK ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))

### Bug Fixes

* Changed field behavior for an existing field `key` in message `.google.cloud.vmmigration.v1.AwsSourceDetails` to `required` to protect from incorrect input ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Changed field behavior for an existing field `project` in message `.google.cloud.vmmigration.v1.TargetProject` to `required` to protect from incorrect input ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Changed field behavior for an existing field `value` in message `.google.cloud.vmmigration.v1.AwsSourceDetails` to `required` to protect from incorrect input ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* upgrade gRPC service registration func An update to Go gRPC Protobuf generation will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go service files. This change should affect very few client library users. See release notes advisories in https://togithub.com/googleapis/google-cloud-go/pull/11025. ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))

### Documentation

* A comment for enum `Mode` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `ABORTED` in enum `State` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `CONVERSATIONAL_FILTER_ONLY` in enum `Mode` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `CREATED` in enum `State` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `DESTROYED` in enum `CryptoKeyVersionState` is changed PiperOrigin-RevId: 759163334 ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A comment for enum value `DISABLED` in enum `Mode` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `ENABLED` in enum `Mode` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `FULL` in enum `SyncMode` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `TASK_CONFIG` in enum `ExecutionTrigger` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for enum value `TASK_CONFIG` in enum `Trigger` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `accept_defective_only` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `accept_exchange` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `alternate_use_permission` in message `.google.cloud.dataplex.v1.AspectType` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `alternate_use_permission` in message `.google.cloud.dataplex.v1.EntryType` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `aspect_keys` in message `.google.cloud.dataplex.v1.ImportItem` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `average_length` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `average` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `count` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `create_time` in message `.google.cloud.dataplex.v1.Entry` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `database_flags` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `dedicated_endpoint_disabled` in message `.google.cloud.aiplatform.v1beta1.DeployRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `dimension` in message `.google.cloud.dataplex.v1.DataQualityRule` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `distinct_ratio` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `encryption_config` in message `.google.cloud.alloydb.v1.AutomatedBackupPolicy` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `encryption_config` in message `.google.cloud.alloydb.v1.ContinuousBackupConfig` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `end` in message `.google.cloud.dataplex.v1.ScannedData` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `entity` in message `.google.cloud.dataplex.v1.DataSource` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `field` in message `.google.cloud.dataplex.v1.ScannedData` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `fields` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `id` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `image_version` in message `.google.cloud.dataplex.v1.Environment` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `ip` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `max_length` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `max` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `min_length` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `min` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `mode` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.DataQualityDimension` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.DataScanJob` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.DataScan` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.DeleteDataScanRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.GetDataScanJobRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.GetDataScanRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.RunDataScanRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `name` in message `.google.cloud.dataplex.v1.SearchEntriesRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `null_ratio` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `online_return_policy` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `order_by` in message `.google.cloud.dataplex.v1.SearchEntriesRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `output_path` in message `.google.cloud.dataplex.v1.MetadataJob` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.CreateDataScanRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.CreateEntryGroupRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.CreateLakeRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.ListDataScanJobsRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.ListDataScansRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.ListDataTaxonomiesRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.cloud.dataplex.v1.ListLakesRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `parent` in message `.google.shopping.merchant.accounts.v1beta.CreateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `process_refund_days` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `profile` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `quartiles` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `query` in message `.google.cloud.dataplex.v1.SearchEntriesRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `ranking_expression` in messages `.google.cloud.discoveryengine.v1alpha.SearchRequest` and `.google.cloud.discoveryengine.v1beta.SearchRequest` is changed to support the Custom Ranking use case ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `ratio` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `requested_cancellation` in message `.google.cloud.alloydb.v1.OperationMetadata` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `resource` in message `.google.cloud.dataplex.v1.DataSource` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `results_table` in message `.google.cloud.dataplex.v1.DataProfileSpec` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `return_label_source` in message `.google.shopping.merchant.accounts.v1beta.OnlineReturnPolicy` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `row_count` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `row_filter` in message `.google.cloud.dataplex.v1.DataProfileSpec` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `rule` in message `.google.cloud.dataplex.v1.GenerateDataQualityRulesResponse` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `scanned_data` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `standard_deviation` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `start` in message `.google.cloud.dataplex.v1.ScannedData` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `state` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `top_n_values` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `transfer_bytes` in message `.google.cloud.netapp.v1.TransferStats` is changed ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* A comment for field `type` in message `.google.cloud.dataplex.v1.AspectType` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `type` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `update_mask` in message `.google.cloud.dataplex.v1.ImportItem` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `update_mask` in message `.google.shopping.merchant.accounts.v1beta.UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for field `update_time` in message `.google.cloud.dataplex.v1.Entry` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `use_metadata_exchange` in message `.google.cloud.alloydb.v1.GenerateClientCertificateRequest` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `user_managed` in message `.google.cloud.dataplex.v1.Schema` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `user_query_types` in message `.google.cloud.retail.v2alpha.ConversationalSearchResponse` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `user` in message `.google.cloud.alloydb.v1.ExecuteSqlRequest` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for field `value` in message `.google.cloud.dataplex.v1.DataProfileResult` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for field `zone_id` in message `.google.cloud.alloydb.v1.Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for message `AspectType` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for message `DeleteAspectTypeRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for message `DeleteEntryTypeRequest` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for message `Instance` is changed ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* A comment for message `UpdateOnlineReturnPolicyRequest` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for method `CreateMetadataJob` in service `CatalogService` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for method `DeleteOnlineReturnPolicy` in service `OnlineReturnPolicyService` is changed ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* A comment for service `CatalogService` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for service `CmekService` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* A comment for service `ContentService` is changed ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Annotate all names with IDENTIFIER ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* Remove comments for a non public feature (#12243) ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* Update field descriptions, comments, and links in existing services ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Updated comments and descriptions to improve clarity ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))
* Updated the formatting in some comments in multiple services ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Updating docs for total_size field in KMS List APIs ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* Use backticks around `username` in documentation for `Actor.email` ([cb8b66c](https://github.com/googleapis/google-cloud-go/commit/cb8b66cdbff925aaecb59703523cdf364b554eb6))
* fix links and typos ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* minor doc revision ([d73f912](https://github.com/googleapis/google-cloud-go/commit/d73f9123be77bb3278f48d510cd0fb22feb605bc))

## [1.14.7](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.6...automl/v1.14.7) (2025-04-15)


### Bug Fixes

* **automl:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.14.6](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.5...automl/v1.14.6) (2025-03-13)


### Bug Fixes

* **automl:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.14.5](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.4...automl/v1.14.5) (2025-03-06)


### Bug Fixes

* **automl:** Fix out-of-sync version.go ([28f0030](https://github.com/googleapis/google-cloud-go/commit/28f00304ebb13abfd0da2f45b9b79de093cca1ec))

## [1.14.4](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.3...automl/v1.14.4) (2025-01-02)


### Bug Fixes

* **automl:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.2...automl/v1.14.3) (2024-12-11)


### Documentation

* **automl:** Update io.proto to use markdown headings instead of HTML, remove some unused HTML from markdown ([#11234](https://github.com/googleapis/google-cloud-go/issues/11234)) ([0b3ab19](https://github.com/googleapis/google-cloud-go/commit/0b3ab19d38a5726b9e9db91fde933c4d7fe72fc8))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.1...automl/v1.14.2) (2024-10-23)


### Bug Fixes

* **automl:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **automl:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/automl/v1.14.0...automl/v1.14.1) (2024-09-12)


### Bug Fixes

* **automl:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.12...automl/v1.14.0) (2024-08-20)


### Features

* **automl:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.13.12](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.11...automl/v1.13.12) (2024-08-08)


### Bug Fixes

* **automl:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.13.11](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.10...automl/v1.13.11) (2024-07-24)


### Bug Fixes

* **automl:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.13.10](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.9...automl/v1.13.10) (2024-07-10)


### Bug Fixes

* **automl:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.13.9](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.8...automl/v1.13.9) (2024-07-01)


### Bug Fixes

* **automl:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.13.8](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.7...automl/v1.13.8) (2024-06-26)


### Bug Fixes

* **automl:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.13.7](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.6...automl/v1.13.7) (2024-05-01)


### Bug Fixes

* **automl:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.13.6](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.5...automl/v1.13.6) (2024-03-14)


### Bug Fixes

* **automl:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.13.5](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.4...automl/v1.13.5) (2024-01-30)


### Bug Fixes

* **automl:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.13.4](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.3...automl/v1.13.4) (2023-11-01)


### Bug Fixes

* **automl:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.13.3](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.2...automl/v1.13.3) (2023-10-26)


### Bug Fixes

* **automl:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.13.2](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.1...automl/v1.13.2) (2023-10-12)


### Bug Fixes

* **automl:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/automl/v1.13.0...automl/v1.13.1) (2023-06-20)


### Bug Fixes

* **automl:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.12.1...automl/v1.13.0) (2023-05-30)


### Features

* **automl:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/automl/v1.12.0...automl/v1.12.1) (2023-05-08)


### Bug Fixes

* **automl:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/automl-v1.11.0...automl/v1.12.0) (2023-01-26)


### Features

* **automl:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **automl:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **automl:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **automl:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **automl:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **automl:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/automl-v1.10.0...automl/v1.11.0) (2023-01-26)


### Features

* **automl:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **automl:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **automl:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **automl:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **automl:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **automl:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.9.0...automl/v1.10.0) (2023-01-26)


### Features

* **automl:** Enable REST transport in C# ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.8.0...automl/v1.9.0) (2023-01-04)


### Features

* **automl:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.7.0...automl/v1.8.0) (2022-11-03)


### Features

* **automl:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.6.0...automl/v1.7.0) (2022-10-25)


### Features

* **automl:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.5.0...automl/v1.6.0) (2022-09-21)


### Features

* **automl:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.4.0...automl/v1.5.0) (2022-09-19)


### Features

* **automl:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.3.0...automl/v1.4.0) (2022-06-29)


### Features

* **automl:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.2.0...automl/v1.3.0) (2022-02-23)


### Features

* **automl:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/automl/v1.1.0...automl/v1.2.0) (2022-02-11)


### Features

* **automl:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))


### Bug Fixes

* **automl:** proto field markdown comment for the display_name field in annotation_payload.proto to point the correct public v1/ version fix: Add back java_multiple_files option to the text_sentiment.proto to match with the previous published version of text_sentiment proto ([5255bbb](https://github.com/googleapis/google-cloud-go/commit/5255bbbb92c45453e9446e03f1d4acb14ac07f12))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/automl/v1.0.0...automl/v1.1.0) (2022-01-14)


### Features

* **automl:** publish updated protos for cloud/automl/v1 service fix!: One of the fields now have field_behavior as REQUIRED in cloud/automl/v1 service definition. ([3bbe8c0](https://www.github.com/googleapis/google-cloud-go/commit/3bbe8c0c558c06ef5865bb79eb228b6da667ddb3))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out automl as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
