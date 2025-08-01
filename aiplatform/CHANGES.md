# Changes


## [1.96.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.95.0...aiplatform/v1.96.0) (2025-07-31)


### Features

* **aiplatform:** Add `DeploymentStage` for CreateEndpointOperationMetadata and DeployModelOperationMetadata ([9d29fa9](https://github.com/googleapis/google-cloud-go/commit/9d29fa96abaac05868fa4ed1bc986244e9f561d8))
* **aiplatform:** Add `DeploymentStage` for CreateEndpointOperationMetadata and DeployModelOperationMetadata ([9d29fa9](https://github.com/googleapis/google-cloud-go/commit/9d29fa96abaac05868fa4ed1bc986244e9f561d8))
* **aiplatform:** Add enable_datapoint_upsert_logging to google.cloud.aiplatform.v1.DeployedIndex ([9d29fa9](https://github.com/googleapis/google-cloud-go/commit/9d29fa96abaac05868fa4ed1bc986244e9f561d8))
* **aiplatform:** Add enable_datapoint_upsert_logging to google.cloud.aiplatform.v1.DeployedIndex ([c574e28](https://github.com/googleapis/google-cloud-go/commit/c574e287f49cc1c3b069b35d95b98da2bc9b948f))
* **aiplatform:** Added the ability to use the Model Armor service for content sanitization ([#12623](https://github.com/googleapis/google-cloud-go/issues/12623)) ([768079e](https://github.com/googleapis/google-cloud-go/commit/768079ef67c0fa68e57f60249cc6dbe774631df1))
* **aiplatform:** Adds DWS and spot VM feature support to custom batch predictions 2.0 ([83f894e](https://github.com/googleapis/google-cloud-go/commit/83f894e372ae66b96d8d9d4379fa0ea18547fe72))


### Documentation

* **aiplatform:** Update MutateDeployedModel documentation ([9d29fa9](https://github.com/googleapis/google-cloud-go/commit/9d29fa96abaac05868fa4ed1bc986244e9f561d8))
* **aiplatform:** Update MutateDeployedModel documentation ([9d29fa9](https://github.com/googleapis/google-cloud-go/commit/9d29fa96abaac05868fa4ed1bc986244e9f561d8))

## [1.95.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.94.0...aiplatform/v1.95.0) (2025-07-23)


### Features

* **aiplatform:** Add service_account to Reasoning Engine public protos ([eeb4b1f](https://github.com/googleapis/google-cloud-go/commit/eeb4b1fe8eb83b73ec31b0bd46e3704bdc0212c3))
* **aiplatform:** Add service_account to Reasoning Engine public protos ([eeb4b1f](https://github.com/googleapis/google-cloud-go/commit/eeb4b1fe8eb83b73ec31b0bd46e3704bdc0212c3))


### Bug Fixes

* **aiplatform:** Remove gemini_template_config and request_column_name fields from DatasetService.AssessData and DatasetService.AssembleData ([ac4970b](https://github.com/googleapis/google-cloud-go/commit/ac4970b5a6318dbfcdca7da5ee256852ca49ea23))

## [1.94.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.93.0...aiplatform/v1.94.0) (2025-07-16)


### Features

* **aiplatform:** Add Aggregation Output in EvaluateDataset Get Operation Response ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** Add API for Managed OSS Fine Tuning ([#12552](https://github.com/googleapis/google-cloud-go/issues/12552)) ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))
* **aiplatform:** Add flexstart option to v1beta1 ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** Some comments changes in machine_resources.proto to v1beta1 ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** Vertex AI Model Garden custom model deploy Public Preview ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))


### Documentation

* **aiplatform:** A comment for field `boot_disk_type` in message `.google.cloud.aiplatform.v1beta1.DiskSpec` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `machine_spec` in message `.google.cloud.aiplatform.v1beta1.DedicatedResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `max_replica_count` in message `.google.cloud.aiplatform.v1beta1.AutomaticResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `max_replica_count` in message `.google.cloud.aiplatform.v1beta1.DedicatedResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `min_replica_count` in message `.google.cloud.aiplatform.v1beta1.AutomaticResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `min_replica_count` in message `.google.cloud.aiplatform.v1beta1.DedicatedResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field `required_replica_count` in message `.google.cloud.aiplatform.v1beta1.DedicatedResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** A comment for field learning_rate_multiplier in message .google.cloud.aiplatform.v1beta1.SupervisedHyperParameters is changed ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))
* **aiplatform:** A comment for field model in message .google.cloud.aiplatform.v1beta1.TunedModel is changed ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))
* **aiplatform:** A comment for field training_dataset_uri in message .google.cloud.aiplatform.v1beta1.SupervisedTuningSpec is changed ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))
* **aiplatform:** A comment for field validation_dataset_uri in message .google.cloud.aiplatform.v1beta1.SupervisedTuningSpec is changed ([622edbb](https://github.com/googleapis/google-cloud-go/commit/622edbbcb248142f545a717bf6aaaa5c91845a43))
* **aiplatform:** A comment for message `DedicatedResources` is changed ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))
* **aiplatform:** Add constraints for AggregationMetric enum and default value for flip_enabled field in AutoraterConfig ([8d76df5](https://github.com/googleapis/google-cloud-go/commit/8d76df5771277c582d5d074adf4753fbcfe26673))

## [1.93.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.92.0...aiplatform/v1.93.0) (2025-07-09)


### Features

* **aiplatform:** Add computer use support to tools ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Add computer use support to tools ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Add invoke_route_prefix to ModelContainerSpec in aiplatform v1 models.proto ([#12502](https://github.com/googleapis/google-cloud-go/issues/12502)) ([8f25e1c](https://github.com/googleapis/google-cloud-go/commit/8f25e1cf971c01402c88df505ab3bdcce2c543d6))
* **aiplatform:** Add message ColabImage, add field colab_image to NotebookSoftwareConfig ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Add message ColabImage, add field colab_image to NotebookSoftwareConfig ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Allow user input for schedule_resource_name in NotebookExecutionJob ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Allow user input for schedule_resource_name in NotebookExecutionJob ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Expose task_unique_name in pipeline task details for pipeline rerun ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** Expose task_unique_name in pipeline task details for pipeline rerun ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))


### Documentation

* **aiplatform:** A comment for enum value BEING_STARTED in enum NotebookRuntime.RuntimeState is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** A comment for enum value BEING_STARTED in enum NotebookRuntime.RuntimeState is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** A comment for message NotebookRuntime is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** A comment for message NotebookRuntime is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** A comment for message NotebookSoftwareConfig is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))
* **aiplatform:** A comment for message NotebookSoftwareConfig is changed ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))

## [1.92.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.91.0...aiplatform/v1.92.0) (2025-06-25)


### Features

* **aiplatform:** Add GenAiAdvancedFeaturesConfig to endpoint.proto ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))
* **aiplatform:** Add invoke_route_prefix to ModelContainerSpec in aiplatform v1beta1 models.proto ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))
* **aiplatform:** Add Model Garden deploy OSS model API ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))
* **aiplatform:** Add PSCAutomationConfig to PrivateServiceConnectConfig in service_networking.proto ([#12467](https://github.com/googleapis/google-cloud-go/issues/12467)) ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))
* **aiplatform:** Reasoning Engine v1beta1 subresource updates ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))


### Documentation

* **aiplatform:** Clarify that the names for sessions and session_events are no longer required. ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))
* **aiplatform:** Update dedicateEndpointDns documentation ([e720182](https://github.com/googleapis/google-cloud-go/commit/e720182b5704cac4ae9871785a87e3a94d446bc2))

## [1.91.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.90.0...aiplatform/v1.91.0) (2025-06-17)


### Features

* **aiplatform:** Add dns_peering_configs to PscInterfaceConfig ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Add dns_peering_configs to PscInterfaceConfig ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **aiplatform:** Add DnsPeeringConfig in service_networking.proto ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Add DnsPeeringConfig in service_networking.proto ([#12421](https://github.com/googleapis/google-cloud-go/issues/12421)) ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **aiplatform:** Add EncryptionSpec field for RagCorpus CMEK feature to v1 ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **aiplatform:** Add RagEngineConfig update/get APIs to v1 ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Add Scaled tier for RagEngineConfig to v1beta, equivalent to Enterprise ([#12460](https://github.com/googleapis/google-cloud-go/issues/12460)) ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Add Unprovisioned tier to RagEngineConfig in v1beta1 that can disable RagEngine service and delete all data within the service ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Add Unprovisioned tier to RagEngineConfig to disable RagEngine service and delete all data within the service. ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))
* **aiplatform:** Expose UrlContextMetadata API to v1 ([feb078b](https://github.com/googleapis/google-cloud-go/commit/feb078b04ab541dd3bdceb2ac1f24938bb0354a3))
* **aiplatform:** Expose UrlContextMetadata API to v1beta1 ([feb078b](https://github.com/googleapis/google-cloud-go/commit/feb078b04ab541dd3bdceb2ac1f24938bb0354a3))
* **aiplatform:** Introduce RagFileMetadataConfig for importing metadata to Rag ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))


### Documentation

* **aiplatform:** Enterprise tier in RagEngineConfig, use Scaled tier instead. ([a013575](https://github.com/googleapis/google-cloud-go/commit/a01357592a61b05c19ee2c520ed59f02504c371a))

## [1.90.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.89.0...aiplatform/v1.90.0) (2025-06-04)


### Features

* **aiplatform:** A new field `include_thoughts` is added to message `.google.cloud.aiplatform.v1.GenerationConfig.ThinkingConfig` ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** A new field `thought_signature` is added to message `.google.cloud.aiplatform.v1.Part` ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** A new field `thought` is added to message `.google.cloud.aiplatform.v1.Part` ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Add json schema support to structured output and function declaration ([4ee690e](https://github.com/googleapis/google-cloud-go/commit/4ee690e07fddac5d742561bf39cd3b610de7d80a))
* **aiplatform:** Add json schema support to structured output and function declaration ([#12382](https://github.com/googleapis/google-cloud-go/issues/12382)) ([4ee690e](https://github.com/googleapis/google-cloud-go/commit/4ee690e07fddac5d742561bf39cd3b610de7d80a))
* **aiplatform:** Add network_attachment to PscInterfaceConfig ([#12356](https://github.com/googleapis/google-cloud-go/issues/12356)) ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Add psc_interface_config to CustomJobSpec ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Add psc_interface_config to PersistentResource ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Add psc_interface_config to PipelineJob ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Expose URL Context API to v1 ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))
* **aiplatform:** Expose URL Context API to v1beta1 ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))


### Bug Fixes

* **aiplatform:** Fix: upgrade gRPC service registration func ([6a871e0](https://github.com/googleapis/google-cloud-go/commit/6a871e0f6924980da4fec78405bfe0736522afa8))


### Documentation

* **aiplatform:** Allow field `thought` to be set as input ([394ef95](https://github.com/googleapis/google-cloud-go/commit/394ef958d4cbb29fedb6b331581ce1390c65ccb6))

## [1.89.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.88.0...aiplatform/v1.89.0) (2025-05-29)


### Features

* **aiplatform:** A new field `include_thoughts` is added to message `.google.cloud.aiplatform.v1.Part` ([#12349](https://github.com/googleapis/google-cloud-go/issues/12349)) ([49769e0](https://github.com/googleapis/google-cloud-go/commit/49769e084bbcb3116f7ff4c7498e189b81c06798))
* **aiplatform:** Add ImportIndex to IndexService ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))
* **aiplatform:** Introduce RAG as context/memory store for Gemini Live API ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))


### Documentation

* **aiplatform:** A comment for field `global_max_embedding_requests_per_min` in message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` is updated. ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))
* **aiplatform:** A comment for message `RagFileParsingConfig` is updated. ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))

## [1.88.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.87.0...aiplatform/v1.88.0) (2025-05-21)


### Features

* **aiplatform:** Add checkpoint ID to endpoint proto ([d863442](https://github.com/googleapis/google-cloud-go/commit/d863442bc040d09b370aecc40792631df479b1fe))
* **aiplatform:** Add checkpoint ID to endpoint proto ([#12282](https://github.com/googleapis/google-cloud-go/issues/12282)) ([70c11ad](https://github.com/googleapis/google-cloud-go/commit/70c11ad152a463a268421050e3395017f6335090))
* **aiplatform:** Add encryption_spec to Model Monitoring public preview API ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* **aiplatform:** Add VertexAISearch.max_results, filter, data_store_specs options ([62155da](https://github.com/googleapis/google-cloud-go/commit/62155dae7958ebd50140a630a38003f4f74d68bc))
* **aiplatform:** Add VertexAISearch.max_results, filter, data_store_specs options ([#12297](https://github.com/googleapis/google-cloud-go/issues/12297)) ([62155da](https://github.com/googleapis/google-cloud-go/commit/62155dae7958ebd50140a630a38003f4f74d68bc))
* **aiplatform:** Adding thoughts_token_count to prediction service ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))
* **aiplatform:** Adding thoughts_token_count to v1beta1 client library ([2a9d8ee](https://github.com/googleapis/google-cloud-go/commit/2a9d8eec71a7e6803eb534287c8d2f64903dcddd))

## [1.87.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.86.0...aiplatform/v1.87.0) (2025-05-13)


### Features

* **aiplatform:** A new value `NVIDIA_B200 & NVIDIA_H200_141GB` is added to enum `AcceleratorType` ([#12216](https://github.com/googleapis/google-cloud-go/issues/12216)) ([f2b31a3](https://github.com/googleapis/google-cloud-go/commit/f2b31a31eb97db32ca7f19d5bb4f4c9cba73a806))
* **aiplatform:** A new value `NVIDIA_B200 & NVIDIA_H200_141GB` is added to enum `AcceleratorType` ([#12232](https://github.com/googleapis/google-cloud-go/issues/12232)) ([909dcbd](https://github.com/googleapis/google-cloud-go/commit/909dcbdba2685cdf6c4727be3357918be99bc847))
* **aiplatform:** Add ANN feature for RagManagedDb ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* **aiplatform:** Add EncryptionSpec for RagCorpus CMEK feature ([909dcbd](https://github.com/googleapis/google-cloud-go/commit/909dcbdba2685cdf6c4727be3357918be99bc847))
* **aiplatform:** New field `additional_properties` is added to message `.google.cloud.aiplatform.v1.Schema` ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* **aiplatform:** New field `additional_properties` is added to message `.google.cloud.aiplatform.v1beta1.Schema` ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* **aiplatform:** Tuning Checkpoints API ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* **aiplatform:** Tuning Checkpoints API ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))


### Documentation

* **aiplatform:** Fix links and typos ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))
* **aiplatform:** Remove comments for a non public feature ([#12243](https://github.com/googleapis/google-cloud-go/issues/12243)) ([037b55c](https://github.com/googleapis/google-cloud-go/commit/037b55cf453e23451b59ee04077ca599e3ffe031))

## [1.86.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.85.0...aiplatform/v1.86.0) (2025-05-06)


### Features

* **aiplatform:** A new field `system_labels` is added to message `google.cloud.aiplatform.v1beta1.DeployRequest` ([83ae06c](https://github.com/googleapis/google-cloud-go/commit/83ae06c3ec7d190e38856ba4cfd8a13f08356b4d))
* **aiplatform:** Expose llm parser to public v1 proto to prepare for GA ([#12089](https://github.com/googleapis/google-cloud-go/issues/12089)) ([83ae06c](https://github.com/googleapis/google-cloud-go/commit/83ae06c3ec7d190e38856ba4cfd8a13f08356b4d))


### Documentation

* **aiplatform:** Update an outdated URL ([83ae06c](https://github.com/googleapis/google-cloud-go/commit/83ae06c3ec7d190e38856ba4cfd8a13f08356b4d))

## [1.85.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.84.0...aiplatform/v1.85.0) (2025-04-30)


### Features

* **aiplatform:** Adding ThinkingConfig to v1 client library ([#12058](https://github.com/googleapis/google-cloud-go/issues/12058)) ([0fe3b60](https://github.com/googleapis/google-cloud-go/commit/0fe3b60aa3b1530361012fecb13a00567ca27094))
* **aiplatform:** Adding ThinkingConfig to v1beta1 client library ([0fe3b60](https://github.com/googleapis/google-cloud-go/commit/0fe3b60aa3b1530361012fecb13a00567ca27094))
* **aiplatform:** Allow customers to set encryption_spec for context caching ([4c53c42](https://github.com/googleapis/google-cloud-go/commit/4c53c4273a17a39667d962ffa74e308b663270e9))
* **aiplatform:** Deprecate election category HARM_CATEGORY_CIVIC_INTEGRITY ([19c60f9](https://github.com/googleapis/google-cloud-go/commit/19c60f9ac0489ad408b4a8672c5bf091022eda15))
* **aiplatform:** Deprecate election category HARM_CATEGORY_CIVIC_INTEGRITY ([19c60f9](https://github.com/googleapis/google-cloud-go/commit/19c60f9ac0489ad408b4a8672c5bf091022eda15))
* **aiplatform:** Model Registry Model Checkpoint API ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **aiplatform:** New fields `ref` and `defs` are added to message `.google.cloud.aiplatform.v1.Schema` ([4c53c42](https://github.com/googleapis/google-cloud-go/commit/4c53c4273a17a39667d962ffa74e308b663270e9))
* **aiplatform:** New fields `ref` and `defs` are added to message `.google.cloud.aiplatform.v1beta1.Schema` ([4c53c42](https://github.com/googleapis/google-cloud-go/commit/4c53c4273a17a39667d962ffa74e308b663270e9))


### Documentation

* **aiplatform:** Deprecate election category HARM_CATEGORY_CIVIC_INTEGRITY ([4c53c42](https://github.com/googleapis/google-cloud-go/commit/4c53c4273a17a39667d962ffa74e308b663270e9))
* **aiplatform:** Deprecate election category HARM_CATEGORY_CIVIC_INTEGRITY ([4c53c42](https://github.com/googleapis/google-cloud-go/commit/4c53c4273a17a39667d962ffa74e308b663270e9))

## [1.84.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.83.0...aiplatform/v1.84.0) (2025-04-22)


### Features

* **aiplatform:** Deprecated EventActions.transfer_to_agent and replaced with EventActions.transfer_agent ([2551567](https://github.com/googleapis/google-cloud-go/commit/25515675379c6f0ff57cc18565293971d65d1bf2))
* **aiplatform:** Model Registry Model Checkpoint API ([fe831f9](https://github.com/googleapis/google-cloud-go/commit/fe831f9b125baf2cf5774ad892361df2d655814a))


### Bug Fixes

* **aiplatform:** Removed support for session resource paths that do not include reasoning engine ([#12023](https://github.com/googleapis/google-cloud-go/issues/12023)) ([2551567](https://github.com/googleapis/google-cloud-go/commit/25515675379c6f0ff57cc18565293971d65d1bf2))

## [1.83.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.82.1...aiplatform/v1.83.0) (2025-04-15)


### Features

* **aiplatform:** Add Model Garden EULA(End User License Agreement) related APIs ([#11982](https://github.com/googleapis/google-cloud-go/issues/11982)) ([43bc515](https://github.com/googleapis/google-cloud-go/commit/43bc51591e4ffe7efc76449bb00e3747cda2c944))

## [1.82.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.82.0...aiplatform/v1.82.1) (2025-04-15)


### Bug Fixes

* **aiplatform:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.82.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.81.0...aiplatform/v1.82.0) (2025-04-15)


### Features

* **aiplatform:** Add FeatureViewDirectWrite API ([dfdf404](https://github.com/googleapis/google-cloud-go/commit/dfdf404138728724aa6305c5c465ecc6fe5b1264))
* **aiplatform:** Add Gen AI logging public preview API ([dfdf404](https://github.com/googleapis/google-cloud-go/commit/dfdf404138728724aa6305c5c465ecc6fe5b1264))
* **aiplatform:** Add global quota config to vertex rag engine api ([#11949](https://github.com/googleapis/google-cloud-go/issues/11949)) ([dfdf404](https://github.com/googleapis/google-cloud-go/commit/dfdf404138728724aa6305c5c465ecc6fe5b1264))
* **aiplatform:** Add rag_managed_db_config to RagEngineConfig for specifying Basic or Enterprise RagManagedDb tiers ([8a2171a](https://github.com/googleapis/google-cloud-go/commit/8a2171a42cca078228fe27bd287a8ba6cad30e70))
* **aiplatform:** Add RagEngineConfig to specify RAG project-level config ([8a2171a](https://github.com/googleapis/google-cloud-go/commit/8a2171a42cca078228fe27bd287a8ba6cad30e70))
* **aiplatform:** Add UpdateRagEngineConfig rpc ([8a2171a](https://github.com/googleapis/google-cloud-go/commit/8a2171a42cca078228fe27bd287a8ba6cad30e70))

## [1.81.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.80.0...aiplatform/v1.81.0) (2025-03-31)


### Features

* **aiplatform:** Add page spans in retrieved contexts from Vertex RAG Engine in aiplatform v1 ([#11917](https://github.com/googleapis/google-cloud-go/issues/11917)) ([28632c6](https://github.com/googleapis/google-cloud-go/commit/28632c6b7c8f0f9250c2dd6ab86d8cc19de84522))
* **aiplatform:** Add page spans in retrieved contexts from Vertex RAG Engine in aiplatform v1beta1 ([f437f08](https://github.com/googleapis/google-cloud-go/commit/f437f0871a88abbeb918ce7364d0299a513cc311))


### Documentation

* **aiplatform:** A comment for field `model_name` in message `.google.cloud.aiplatform.v1beta1.RagFileParsingConfig` is changed ([f437f08](https://github.com/googleapis/google-cloud-go/commit/f437f0871a88abbeb918ce7364d0299a513cc311))
* **aiplatform:** A comment for field `rag_files_count` in message `.google.cloud.aiplatform.v1beta1.RagCorpus` is changed ([f437f08](https://github.com/googleapis/google-cloud-go/commit/f437f0871a88abbeb918ce7364d0299a513cc311))

## [1.80.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.79.0...aiplatform/v1.80.0) (2025-03-27)


### Features

* **aiplatform:** Add batch prediction assessments to multimodal dataset RPCs ([#11906](https://github.com/googleapis/google-cloud-go/issues/11906)) ([12465b5](https://github.com/googleapis/google-cloud-go/commit/12465b5f3f70d49b19ee5e24dae0f731a24b894d))
* **aiplatform:** Add example, example_store, and example_store_service protos ([76309ee](https://github.com/googleapis/google-cloud-go/commit/76309eee261b1f8a39b79d18a4e69e31b60a1969))
* **aiplatform:** Add session.proto and session_service.proto ([a21d596](https://github.com/googleapis/google-cloud-go/commit/a21d5965fa3f4322da9563425350ba1079279d5a))
* **aiplatform:** Add support for Vertex AI Search engine ([#11912](https://github.com/googleapis/google-cloud-go/issues/11912)) ([76309ee](https://github.com/googleapis/google-cloud-go/commit/76309eee261b1f8a39b79d18a4e69e31b60a1969))
* **aiplatform:** Enable force deletion in ReasoningEngine ([a21d596](https://github.com/googleapis/google-cloud-go/commit/a21d5965fa3f4322da9563425350ba1079279d5a))

## [1.79.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.78.0...aiplatform/v1.79.0) (2025-03-25)


### Features

* **aiplatform:** Add a ExportPublisherModel API ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))
* **aiplatform:** Add AssessData and AssembleData RPCs to DatasetService ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))
* **aiplatform:** Add import result bq sink to the import files API ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))
* **aiplatform:** Add import result gcs sink to the import files API ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))
* **aiplatform:** Add model_config field for model selection preference ([57a62c0](https://github.com/googleapis/google-cloud-go/commit/57a62c05a11b71b4c505061eb4b9469186adeda5))
* **aiplatform:** Enable force deletion in ReasoningEngine v1beta1 ([#11901](https://github.com/googleapis/google-cloud-go/issues/11901)) ([57a62c0](https://github.com/googleapis/google-cloud-go/commit/57a62c05a11b71b4c505061eb4b9469186adeda5))
* **aiplatform:** Update multimodal evaluation (content_map_instance), rubric generation (rubric_based_instance, etc) and raw_output(raw_output, custom_output, etc) proto change in online eval API ([#11876](https://github.com/googleapis/google-cloud-go/issues/11876)) ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))


### Documentation

* **aiplatform:** A comment for field `autorater_config` in message `.google.cloud.aiplatform.v1beta1.EvaluateDatasetRequest` is changed ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))
* **aiplatform:** A comment for field `gcs_source` in message `.google.cloud.aiplatform.v1beta1.EvaluationDataset` is changed ([427f448](https://github.com/googleapis/google-cloud-go/commit/427f448d9a1a32a2a55a695e9e3a915fcc71ae19))

## [1.78.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.77.0...aiplatform/v1.78.0) (2025-03-19)


### Features

* **aiplatform:** Add env variables and agent framework to ReasoningEngineSpec ([c2ee207](https://github.com/googleapis/google-cloud-go/commit/c2ee207621b2bb5fad8e6821892eae0041f469cd))
* **aiplatform:** Add env variables and agent framework to ReasoningEngineSpec in v1beta1 ([#11867](https://github.com/googleapis/google-cloud-go/issues/11867)) ([6618039](https://github.com/googleapis/google-cloud-go/commit/66180390ae6c87906a37b069c431e092010c8a28))
* **aiplatform:** Add VertexAISearch.engine option ([671eed9](https://github.com/googleapis/google-cloud-go/commit/671eed979bfdbf199c4c3787d4f18bca1d5883f4))


### Documentation

* **aiplatform:** Add `deployment_spec` and `agent_framework` field to `ReasoningEngineSpec`. ([c2ee207](https://github.com/googleapis/google-cloud-go/commit/c2ee207621b2bb5fad8e6821892eae0041f469cd))
* **aiplatform:** Add `deployment_spec` and `agent_framework` field to `ReasoningEngineSpec`. ([6618039](https://github.com/googleapis/google-cloud-go/commit/66180390ae6c87906a37b069c431e092010c8a28))
* **aiplatform:** Update comment for `package_spec` from required to optional in `ReasoningEngineSpec`. ([c2ee207](https://github.com/googleapis/google-cloud-go/commit/c2ee207621b2bb5fad8e6821892eae0041f469cd))
* **aiplatform:** Update comment for `package_spec` from required to optional in `ReasoningEngineSpec`. ([6618039](https://github.com/googleapis/google-cloud-go/commit/66180390ae6c87906a37b069c431e092010c8a28))

## [1.77.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.76.0...aiplatform/v1.77.0) (2025-03-13)


### Features

* **aiplatform:** Add function_call.id and function_response.id ([#11820](https://github.com/googleapis/google-cloud-go/issues/11820)) ([1c7cc4b](https://github.com/googleapis/google-cloud-go/commit/1c7cc4b06d24a1af6c4df23f0417880b9629a3a7))


### Bug Fixes

* **aiplatform:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.76.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.75.0...aiplatform/v1.76.0) (2025-03-12)


### Features

* **aiplatform:** Add Layout Parser to RAG v1 API ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **aiplatform:** Add multihost_gpu_node_count to Vertex SDK for multihost GPU support ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **aiplatform:** Add reranker config to RAG v1 API ([12bfa98](https://github.com/googleapis/google-cloud-go/commit/12bfa984f87099dbfbd5abf3436e440e62b04bad))
* **aiplatform:** Allowing users to choose whether to use the hf model cache ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **aiplatform:** Allowing users to choose whether to use the hf model cache ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **aiplatform:** Allowing users to specify the version id of the Model Garden model ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))
* **aiplatform:** Allowing users to specify the version id of the Model Garden model ([dd0d1d7](https://github.com/googleapis/google-cloud-go/commit/dd0d1d7b41884c9fc9b5fe808139cccd29e1e486))

## [1.75.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.74.0...aiplatform/v1.75.0) (2025-03-06)


### Features

* **aiplatform:** A new field `include_equivalent_model_garden_model_deployment_configs` is added to message `.google.cloud.aiplatform.v1beta1.GetPublisherModelRequest` ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **aiplatform:** Add EnterpriseWebSearch tool option ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **aiplatform:** Add VertexAISearch.engine option ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))


### Bug Fixes

* **aiplatform:** An existing google.api.http annotation `http_uri` is changed for method `DeployPublisherModel` in service `ModelGardenService` ([#11670](https://github.com/googleapis/google-cloud-go/issues/11670)) ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))
* **aiplatform:** Remove VertexAISearch.engine option ([#11681](https://github.com/googleapis/google-cloud-go/issues/11681)) ([60dc167](https://github.com/googleapis/google-cloud-go/commit/60dc167a3e9c2876fe55a4f50bd7e0682f953d67))


### Documentation

* **aiplatform:** A comment for field `model` in message `.google.cloud.aiplatform.v1beta1.DeployPublisherModelRequest` is changed ([3f23a91](https://github.com/googleapis/google-cloud-go/commit/3f23a9176f29a0a69b9d57b16f44b72eb3096d0c))

## [1.74.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.73.0...aiplatform/v1.74.0) (2025-02-26)


### Features

* **aiplatform:** Add Model Garden deploy API ([2c4fb44](https://github.com/googleapis/google-cloud-go/commit/2c4fb448a2207a6d9988ec3a7646ea6cbb6f65f9))

## [1.73.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.72.0...aiplatform/v1.73.0) (2025-02-12)


### Features

* **aiplatform:** A new field `create_time` is added to message `.google.cloud.aiplatform.v1.GenerateContentResponse` ([93b6495](https://github.com/googleapis/google-cloud-go/commit/93b649580863dc8121c69263749064660a83e095))
* **aiplatform:** A new field `create_time` is added to message `.google.cloud.aiplatform.v1.GenerateContentResponse` ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **aiplatform:** A new field `response_id` is added to message `.google.cloud.aiplatform.v1.GenerateContentResponse` ([93b6495](https://github.com/googleapis/google-cloud-go/commit/93b649580863dc8121c69263749064660a83e095))
* **aiplatform:** A new field `response_id` is added to message `.google.cloud.aiplatform.v1.GenerateContentResponse` ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **aiplatform:** Add additional Probe options to v1 model.proto ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **aiplatform:** Add Notebooks Runtime Software Configuration ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **aiplatform:** Add Notebooks Runtime Software Configuration ([#11566](https://github.com/googleapis/google-cloud-go/issues/11566)) ([7e10021](https://github.com/googleapis/google-cloud-go/commit/7e100215974a28e556e9b801b4922a0e97bd98c1))
* **aiplatform:** Add RolloutOptions to DeployedModel in v1beta1 endpoint.proto, add additional Probe options in v1beta1 model.proto ([#11574](https://github.com/googleapis/google-cloud-go/issues/11574)) ([1715e30](https://github.com/googleapis/google-cloud-go/commit/1715e30f6d9a383330e697672583a654562aae13))
* **aiplatform:** EvaluateDataset API v1beta1 initial release ([7e10021](https://github.com/googleapis/google-cloud-go/commit/7e100215974a28e556e9b801b4922a0e97bd98c1))


### Documentation

* **aiplatform:** A comment for field `filter` in message `.google.cloud.aiplatform.v1.ListNotebookRuntimesRequest` is changed ([7e10021](https://github.com/googleapis/google-cloud-go/commit/7e100215974a28e556e9b801b4922a0e97bd98c1))
* **aiplatform:** A comment for field `filter` in message `.google.cloud.aiplatform.v1.ListNotebookRuntimeTemplatesRequest` is changed ([7e10021](https://github.com/googleapis/google-cloud-go/commit/7e100215974a28e556e9b801b4922a0e97bd98c1))
* **aiplatform:** A comment for field `filter` in message `.google.cloud.aiplatform.v1beta1.ListNotebookRuntimesRequest` is changed ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **aiplatform:** A comment for field `filter` in message `.google.cloud.aiplatform.v1beta1.ListNotebookRuntimeTemplatesRequest` is changed ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))

## [1.72.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.71.0...aiplatform/v1.72.0) (2025-02-05)


### Features

* **aiplatform:** Add rag_files_count to RagCorpus to count number of associated files ([678944b](https://github.com/googleapis/google-cloud-go/commit/678944b30e389781687209caf3e3b9d35739a6f0))

## [1.71.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.70.0...aiplatform/v1.71.0) (2025-01-30)


### Features

* **aiplatform:** Add Context Cache to v1 ([973e3d2](https://github.com/googleapis/google-cloud-go/commit/973e3d267844d251f5bfc33f473b853ac288b959))
* **aiplatform:** Add machine_spec, data_persistent_disk_spec, network_spec, euc_config, shielded_vm_config to `.google.cloud.aiplatform.v1beta1.NotebookRuntime` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Add machine_spec, data_persistent_disk_spec, network_spec, euc_config, shielded_vm_config to message `.google.cloud.aiplatform.v1.NotebookRuntime` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Add optimized config in v1 API ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **aiplatform:** Add per-modality token count break downs for GenAI APIs ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))
* **aiplatform:** Add per-modality token count break downs for GenAI APIs ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))
* **aiplatform:** Add retrieval_config to ToolConfig v1 ([973e3d2](https://github.com/googleapis/google-cloud-go/commit/973e3d267844d251f5bfc33f473b853ac288b959))
* **aiplatform:** Add retrieval_config to ToolConfig v1beta1 ([973e3d2](https://github.com/googleapis/google-cloud-go/commit/973e3d267844d251f5bfc33f473b853ac288b959))
* **aiplatform:** Add speculative decoding spec to DeployedModel proto ([#11469](https://github.com/googleapis/google-cloud-go/issues/11469)) ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Enable FeatureView Service Account in v1 API version ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))
* **aiplatform:** Enable UpdateFeatureMonitor in v1beta1 API version ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))
* **aiplatform:** Expose code execution tool API to v1 ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))
* **aiplatform:** Model Registry Checkpoint API ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **aiplatform:** Model Registry Checkpoint API ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))
* **aiplatform:** Reasoning Engine v1 GAPIC release ([#11456](https://github.com/googleapis/google-cloud-go/issues/11456)) ([aa54375](https://github.com/googleapis/google-cloud-go/commit/aa54375c195b1bf8653de26400f342438a8d6f85))
* **aiplatform:** Remove autorater config related visibility v1beta1 ([90edd74](https://github.com/googleapis/google-cloud-go/commit/90edd74d13b9dd737134a75d5b18a064a8ee656a))


### Documentation

* **aiplatform:** Deprecate `is_default` in message `.google.cloud.aiplatform.v1.NotebookRuntimeTemplate` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Deprecate `is_default` in message `.google.cloud.aiplatform.v1beta1.NotebookRuntimeTemplate` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Deprecate `service_account` in message `.google.cloud.aiplatform.v1.NotebookRuntime` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Deprecate `service_account` in message `.google.cloud.aiplatform.v1.NotebookRuntimeTemplate` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Deprecate `service_account` in message `.google.cloud.aiplatform.v1beta1.NotebookRuntime` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Deprecate `service_account` in message `.google.cloud.aiplatform.v1beta1.NotebookRuntimeTemplate` ([a694e11](https://github.com/googleapis/google-cloud-go/commit/a694e1152fc75307da6ca8dcfff26cae9189f29c))
* **aiplatform:** Update comments for NumericFilter and Operator ([de5ca9d](https://github.com/googleapis/google-cloud-go/commit/de5ca9d636e15ca22c6487c690aeaf815630d129))

## [1.70.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.69.0...aiplatform/v1.70.0) (2025-01-08)


### Features

* **aiplatform:** A new field `list_all_versions` to `ListPublisherModelsRequest` ([38385d4](https://github.com/googleapis/google-cloud-go/commit/38385d441ba43e7bf6166ee5507a70e77c0b01f5))
* **aiplatform:** A new value `NVIDIA_H100_MEGA_80GB` is added to enum `AcceleratorType` ([38385d4](https://github.com/googleapis/google-cloud-go/commit/38385d441ba43e7bf6166ee5507a70e77c0b01f5))
* **aiplatform:** A new value `NVIDIA_H100_MEGA_80GB` is added to enum `AcceleratorType` ([38385d4](https://github.com/googleapis/google-cloud-go/commit/38385d441ba43e7bf6166ee5507a70e77c0b01f5))
* **aiplatform:** Add a `nfs_mounts` to RaySpec in PersistentResource API ([#11122](https://github.com/googleapis/google-cloud-go/issues/11122)) ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** Add a new thought field in content proto ([4254053](https://github.com/googleapis/google-cloud-go/commit/42540530e44e5f331e66e0777c4aabf449f5fd90))
* **aiplatform:** Add a v1 UpdateEndpointLongRunning API ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **aiplatform:** Add CustomEnvironmentSpec to NotebookExecutionJob ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **aiplatform:** Add CustomEnvironmentSpec to NotebookExecutionJob ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))
* **aiplatform:** Add fast_tryout_enabled to FasterDeploymentConfig v1 proto ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** Add GenerationConfig.MediaResolution ([46fc993](https://github.com/googleapis/google-cloud-go/commit/46fc993a3195203a230e2831bee456baaa9f7b1c))
* **aiplatform:** Add GenerationConfig.Modality ([46fc993](https://github.com/googleapis/google-cloud-go/commit/46fc993a3195203a230e2831bee456baaa9f7b1c))
* **aiplatform:** Add GenerationConfig.SpeechConfig ([46fc993](https://github.com/googleapis/google-cloud-go/commit/46fc993a3195203a230e2831bee456baaa9f7b1c))
* **aiplatform:** Add LLM parser proto to API ([1f49a23](https://github.com/googleapis/google-cloud-go/commit/1f49a23270a3614ead812524d94a87c39b403e76))
* **aiplatform:** Add Model Garden deploy API ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **aiplatform:** Add new `RequiredReplicaCount` field to DedicatedResources in MachineResources ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Add new `RequiredReplicaCount` field to DedicatedResources in MachineResources ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Add new `Status` field to DeployedModel in Endpoint ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Add new `Status` field to DeployedModel in Endpoint ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Add Tool.GoogleSearch ([#11266](https://github.com/googleapis/google-cloud-go/issues/11266)) ([46fc993](https://github.com/googleapis/google-cloud-go/commit/46fc993a3195203a230e2831bee456baaa9f7b1c))
* **aiplatform:** Add Vertex RAG service proto to v1 ([bf3fe5b](https://github.com/googleapis/google-cloud-go/commit/bf3fe5be3262c5f91f92d4850c833e56fe11be16))
* **aiplatform:** Add workbench_runtime and kernel_name to NotebookExecutionJob ([57fdec7](https://github.com/googleapis/google-cloud-go/commit/57fdec7ce3792753c419298b9e526c4889f4101d))
* **aiplatform:** Add workbench_runtime and kernel_name to NotebookExecutionJob ([57fdec7](https://github.com/googleapis/google-cloud-go/commit/57fdec7ce3792753c419298b9e526c4889f4101d))
* **aiplatform:** COMET added to evaluation service proto ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** Enable FeatureGroup Service Account and IAM methods ([46fc993](https://github.com/googleapis/google-cloud-go/commit/46fc993a3195203a230e2831bee456baaa9f7b1c))
* **aiplatform:** Introduce HybridSearch and Ranking configuration for RAG ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Introduce VertexAiSearch integration for RAG ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Support streaming and multi class methods in Reasoning Engine v1beta1 API ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Trajectory eval metrics added to evaluation service proto ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))


### Bug Fixes

* **aiplatform:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))


### Documentation

* **aiplatform:** A comment for field `annotation_schema_uri` in message `.google.cloud.aiplatform.v1.ExportDataConfig` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `api_key_config` in message `.google.cloud.aiplatform.v1beta1.JiraSource` is changed ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `attributions` in message `.google.cloud.aiplatform.v1.Explanation` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `bool_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `bytes_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `class_method` in message `.google.cloud.aiplatform.v1beta1.StreamQueryReasoningEngineRequest` is changed (from steam_query to stream_query) ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `data_stats` in message `.google.cloud.aiplatform.v1.Model` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `deployed_index` in message `.google.cloud.aiplatform.v1.MutateDeployedIndexRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `double_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `enable_logging` in message `.google.cloud.aiplatform.v1.ModelMonitoringAlertConfig` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `encryption_spec` in message `.google.cloud.aiplatform.v1.NotebookExecutionJob` is changed ([57fdec7](https://github.com/googleapis/google-cloud-go/commit/57fdec7ce3792753c419298b9e526c4889f4101d))
* **aiplatform:** A comment for field `encryption_spec` in message `.google.cloud.aiplatform.v1beta1.NotebookExecutionJob` is changed ([57fdec7](https://github.com/googleapis/google-cloud-go/commit/57fdec7ce3792753c419298b9e526c4889f4101d))
* **aiplatform:** A comment for field `float_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `int_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `int64_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `labels` in message `.google.cloud.aiplatform.v1beta1.PublisherModel` is changed ([8ebcc6d](https://github.com/googleapis/google-cloud-go/commit/8ebcc6d276fc881c3914b5a7af3265a04e718e45))
* **aiplatform:** A comment for field `next_page_token` in message `.google.cloud.aiplatform.v1.ListNotebookExecutionJobsResponse` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `page_token` in message `.google.cloud.aiplatform.v1.ListFeatureGroupsRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `page_token` in message `.google.cloud.aiplatform.v1.ListNotebookExecutionJobsRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `page_token` in message `.google.cloud.aiplatform.v1.ListPersistentResourcesRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `page_token` in message `.google.cloud.aiplatform.v1.ListTuningJobsRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `partial_failure_bigquery_sink` in message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` is changed ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `partial_failure_gcs_sink` in message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` is changed ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `predictions` in message `.google.cloud.aiplatform.v1.EvaluatedAnnotation` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `rag_file_parsing_config` in message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` is changed ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `request` in message `.google.cloud.aiplatform.v1.BatchMigrateResourcesOperationMetadata` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `saved_query_id` in message `.google.cloud.aiplatform.v1.ExportDataConfig` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `source_uri` in message `.google.cloud.aiplatform.v1beta1.RagContexts` is changed ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** A comment for field `string_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `uint_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for field `uint64_val` in message `.google.cloud.aiplatform.v1.Tensor` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for message `DeleteEntityTypeRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for message `DeleteFeatureViewRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for message `ListPersistentResourcesRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for message `StreamingReadFeatureValuesRequest` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** A comment for method `ResumeSchedule` in service `ScheduleService` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **aiplatform:** Added support for multiple `class_methods` in QueryReasoningEngine ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Added support for StreamQueryReasoningEngine ([8dedb87](https://github.com/googleapis/google-cloud-go/commit/8dedb878c070cc1e92d62bb9b32358425e3ceffb))
* **aiplatform:** Fixed typo for field `use_strict_string_match` in message `.google.cloud.aiplatform.v1beta1.ToolParameterKVMatchSpec` ([c1e936d](https://github.com/googleapis/google-cloud-go/commit/c1e936df6527933f5e7c31be0f95aa46ff2c0e61))

## [1.69.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.68.0...aiplatform/v1.69.0) (2024-11-12)


### Features

* **aiplatform:** A new enum `NotebookExecutionJobView` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new enum `Strategy` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new enum `Strategy` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `any_of` is added to message `.google.cloud.aiplatform.v1.Schema` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `any_of` is added to message `.google.cloud.aiplatform.v1beta1.Schema` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `avg_logprobs` is added to message `.google.cloud.aiplatform.v1.Candidate` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `avg_logprobs` is added to message `.google.cloud.aiplatform.v1beta1.Candidate` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `billable_sum` is added to message `.google.cloud.aiplatform.v1.SupervisedTuningDatasetDistribution` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `billable_sum` is added to message `.google.cloud.aiplatform.v1beta1.SupervisedTuningDatasetDistribution` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `dedicated_endpoint_dns` is added to message `.google.cloud.aiplatform.v1.Endpoint` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `dedicated_endpoint_dns` is added to message `.google.cloud.aiplatform.v1beta1.Endpoint` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `dedicated_endpoint_enabled` is added to message `.google.cloud.aiplatform.v1.Endpoint` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `dedicated_endpoint_enabled` is added to message `.google.cloud.aiplatform.v1beta1.Endpoint` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `display_name` is added to message `.google.cloud.aiplatform.v1beta1.CachedContent` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `encryption_spec` is added to message `.google.cloud.aiplatform.v1.NotebookExecutionJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `encryption_spec` is added to message `.google.cloud.aiplatform.v1beta1.NotebookExecutionJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `generation_config` is added to message `.google.cloud.aiplatform.v1.CountTokensRequest` ([#10880](https://github.com/googleapis/google-cloud-go/issues/10880)) ([0b3c268](https://github.com/googleapis/google-cloud-go/commit/0b3c268c564ffe0d87b0efc716f08afaf064b4cc))
* **aiplatform:** A new field `generation_config` is added to message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` ([2f0aec8](https://github.com/googleapis/google-cloud-go/commit/2f0aec894179304d234be6c792d82cf4336b6d0a))
* **aiplatform:** A new field `grounding_chunks` is added to message `.google.cloud.aiplatform.v1.GroundingMetadata` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `grounding_supports` is added to message `.google.cloud.aiplatform.v1.GroundingMetadata` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `hugging_face_token` is added to message `.google.cloud.aiplatform.v1.GetPublisherModelRequest` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `hugging_face_token` is added to message `.google.cloud.aiplatform.v1beta1.GetPublisherModelRequest` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `is_hugging_face_model` is added to message `.google.cloud.aiplatform.v1.GetPublisherModelRequest` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `is_hugging_face_model` is added to message `.google.cloud.aiplatform.v1beta1.GetPublisherModelRequest` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `jira_source` is added to message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `jira_source` is added to message `.google.cloud.aiplatform.v1beta1.RagFile` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `labels` is added to message `.google.cloud.aiplatform.v1.GenerateContentRequest` ([0b3c268](https://github.com/googleapis/google-cloud-go/commit/0b3c268c564ffe0d87b0efc716f08afaf064b4cc))
* **aiplatform:** A new field `labels` is added to message `.google.cloud.aiplatform.v1beta1.GenerateContentRequest` ([2f0aec8](https://github.com/googleapis/google-cloud-go/commit/2f0aec894179304d234be6c792d82cf4336b6d0a))
* **aiplatform:** A new field `logprbs` is added to message `.google.cloud.aiplatform.v1.GenerationConfig` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `logprbs` is added to message `.google.cloud.aiplatform.v1beta1.GenerationConfig` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `logprobs_result` is added to message `.google.cloud.aiplatform.v1.Candidate` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `logprobs_result` is added to message `.google.cloud.aiplatform.v1beta1.Candidate` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `model_version` is added to message `.google.cloud.aiplatform.v1.GenerateContentResponse` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `model_version` is added to message `.google.cloud.aiplatform.v1beta1.GenerateContentResponse` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `numeric_filters` is added to message `.google.cloud.aiplatform.v1.NearestNeighborQuery` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `numeric_filters` is added to message `.google.cloud.aiplatform.v1beta1.NearestNeighborQuery` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `property_ordering` is added to message `.google.cloud.aiplatform.v1.Schema` ([#10868](https://github.com/googleapis/google-cloud-go/issues/10868)) ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))
* **aiplatform:** A new field `property_ordering` is added to message `.google.cloud.aiplatform.v1beta1.Schema` ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))
* **aiplatform:** A new field `psc_interface_config` is added to message `.google.cloud.aiplatform.v1beta1.PersistentResource` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `ray_logs_spec` is added to message `.google.cloud.aiplatform.v1.RaySpec` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `ray_logs_spec` is added to message `.google.cloud.aiplatform.v1beta1.RaySpec` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `response_logprbs` is added to message `.google.cloud.aiplatform.v1.GenerationConfig` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `response_logprbs` is added to message `.google.cloud.aiplatform.v1beta1.GenerationConfig` ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** A new field `routing_config` is added to message `.google.cloud.aiplatform.v1.GenerationConfig` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `routing_config` is added to message `.google.cloud.aiplatform.v1beta1.GenerationConfig` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `sample_request` is added to message `.google.cloud.aiplatform.v1.PublisherModel` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `sample_request` is added to message `.google.cloud.aiplatform.v1beta1.PublisherModel` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.BatchPredictionJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.CustomJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.DataItem` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.Dataset` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.DatasetVersion` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.DeploymentResourcePool` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.Endpoint` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.EntityType` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.FeatureOnlineStore` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.Featurestore` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.FeatureView` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.FeatureViewSync` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.HyperparameterTuningJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.Index` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.IndexEndpoint` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.ModelDeploymentMonitoringJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1.NasJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.CustomJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.Endpoint` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.EntityType` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.FeatureOnlineStore` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.Featurestore` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.FeatureView` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.FeatureViewSync` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.HyperparameterTuningJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.Index` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.IndexEndpoint` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.ModelDeploymentMonitoringJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.ModelMonitor` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.NasJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzi` is added to message `.google.cloud.aiplatform.v1beta1.PipelineJob` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.BatchPredictionJob` ([#10663](https://github.com/googleapis/google-cloud-go/issues/10663)) ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.CustomJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.DataItem` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.Dataset` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.DatasetVersion` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.DeploymentResourcePool` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.Endpoint` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.EntityType` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.FeatureOnlineStore` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.Featurestore` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.FeatureView` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.FeatureViewSync` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.HyperparameterTuningJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.Index` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.IndexEndpoint` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.ModelDeploymentMonitoringJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1.NasJob` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.CustomJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.Endpoint` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.EntityType` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.FeatureOnlineStore` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.Featurestore` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.FeatureView` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.FeatureViewSync` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.HyperparameterTuningJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.Index` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.IndexEndpoint` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.ModelDeploymentMonitoringJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.ModelMonitor` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.NasJob` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `satisfies_pzs` is added to message `.google.cloud.aiplatform.v1beta1.PipelineJob` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `score` is added to message `.google.cloud.aiplatform.v1.Candidate` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `score` is added to message `.google.cloud.aiplatform.v1beta1.Candidate` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `seed` is added to message `.google.cloud.aiplatform.v1.GenerationConfig` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `seed` is added to message `.google.cloud.aiplatform.v1beta1.GenerationConfig` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `service_attachment` is added to message `.google.cloud.aiplatform.v1.PrivateServiceConnectConfig` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `service_attachment` is added to message `.google.cloud.aiplatform.v1beta1.PrivateServiceConnectConfig` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `slack_source` is added to message `.google.cloud.aiplatform.v1beta1.ImportRagFilesConfig` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `slack_source` is added to message `.google.cloud.aiplatform.v1beta1.RagFile` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `strategy` is added to message `.google.cloud.aiplatform.v1.Scheduling` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `strategy` is added to message `.google.cloud.aiplatform.v1beta1.Scheduling` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `system_instruction` is added to message `.google.cloud.aiplatform.v1.CountTokensRequest` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `system_instruction` is added to message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `time_series` is added to message `.google.cloud.aiplatform.v1.FeatureGroup` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `time_series` is added to message `.google.cloud.aiplatform.v1beta1.FeatureGroup` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `tools` is added to message `.google.cloud.aiplatform.v1.CountTokensRequest` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `tools` is added to message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `total_billable_token_count` is added to message `.google.cloud.aiplatform.v1.SupervisedTuningDataStats` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new field `total_billable_token_count` is added to message `.google.cloud.aiplatform.v1beta1.SupervisedTuningDataStats` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new field `total_truncated_example_count` is added to message `.google.cloud.aiplatform.v1.SupervisedTuningDataStats` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `total_truncated_example_count` is added to message `.google.cloud.aiplatform.v1beta1.SupervisedTuningDataStats` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new field `truncated_example_indices` is added to message `.google.cloud.aiplatform.v1.SupervisedTuningDataStats` ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new field `truncated_example_indices` is added to message `.google.cloud.aiplatform.v1beta1.SupervisedTuningDataStats` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new message `ApiAuth` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `CreateNotebookExecutionJobOperationMetadata` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `CreateNotebookExecutionJobRequest` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `DeleteNotebookExecutionJobRequest` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `GetNotebookExecutionJobRequest` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `GroundingChunk` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `GroundingSupport` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `JiraSource` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `ListNotebookExecutionJobsRequest` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `ListNotebookExecutionJobsResponse` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `NotebookExecutionJob` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `NumericFilter` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `NumericFilter` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `PscInterfaceConfig` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `RayLogsSpec` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `RayLogsSpec` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `RoutingConfig` is added ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new message `RoutingConfig` is added ([#10653](https://github.com/googleapis/google-cloud-go/issues/10653)) ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new message `Segment` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new message `SlackSource` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new message `TimeSeries` is added ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new message `TimeSeries` is added ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A new method `CreateNotebookExecutionJob` is added to service `NotebookService` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new method `DeleteNotebookExecutionJob` is added to service `NotebookService` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new method `GetNotebookExecutionJob` is added to service `NotebookService` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new method `ListNotebookExecutionJobs` is added to service `NotebookService` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new resource_definition `aiplatform.googleapis.com/NotebookExecutionJob` is added ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new resource_definition `compute.googleapis.com/NetworkAttachment` is added ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A new resource_definition `compute.googleapis.com/NetworkAttachment` is added ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A new value `ADAPTER_SIZE_THIRTY_TWO` is added to enum `AdapterSize` ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A new value `ADAPTER_SIZE_THIRTY_TWO` is added to enum `AdapterSize` ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** Add `text` field for Grounding metadata support chunk output ([6e69d2e](https://github.com/googleapis/google-cloud-go/commit/6e69d2e85849002bad227ea5bebcde9199605bef))
* **aiplatform:** Add a dynamic retrieval API ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add a dynamic retrieval API ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add ApiKeyConfig field to ApiAuth ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** Add BatchCreateFeatures rpc to feature_registry_service.proto ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **aiplatform:** Add BYOSA field to tuning_job ([380e7d2](https://github.com/googleapis/google-cloud-go/commit/380e7d23e69b22ab46cc6e3be58902accee2f26a))
* **aiplatform:** Add BYOSA field to tuning_job ([380e7d2](https://github.com/googleapis/google-cloud-go/commit/380e7d23e69b22ab46cc6e3be58902accee2f26a))
* **aiplatform:** Add CIVIC_INTEGRITY category to SafetySettings for prediction service ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))
* **aiplatform:** Add CIVIC_INTEGRITY category to SafetySettings for prediction service ([fdb4ea9](https://github.com/googleapis/google-cloud-go/commit/fdb4ea99189657880e5f0e0dce16bef1c3aa0d2f))
* **aiplatform:** Add client_id to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add client_secret to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add code execution tool API ([#11060](https://github.com/googleapis/google-cloud-go/issues/11060)) ([f307078](https://github.com/googleapis/google-cloud-go/commit/f307078bc14758671e43ac401efb855089b27752))
* **aiplatform:** Add continuous sync option in feature_view.proto ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add deploy_metadata to PublisherModel.Deploy v1 ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Add deploy_metadata to PublisherModel.Deploy v1beta1 ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Add drive_id to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add drive_name to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add enable_secure_private_service_connect in service attachment ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add encryption_spec to TuningJob ([d6c543c](https://github.com/googleapis/google-cloud-go/commit/d6c543c3969016c63e158a862fc173dff60fb8d9))
* **aiplatform:** Add enum value MALFORMED_FUNCTION_CALL to `.google.cloud.aiplatform.v1beta1.content.Candidate.FinishReason` ([#10400](https://github.com/googleapis/google-cloud-go/issues/10400)) ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** Add evaluation service proto to v1 ([24616cc](https://github.com/googleapis/google-cloud-go/commit/24616cc136a1ba49a551aad6f76d4d2e062d267c))
* **aiplatform:** Add fast_tryout_enabled to FasterDeploymentConfig message in aiplatform v1beta1 endpoint.proto ([#11042](https://github.com/googleapis/google-cloud-go/issues/11042)) ([2c83297](https://github.com/googleapis/google-cloud-go/commit/2c83297a569117b0252b5b2edaecb09e4924d979))
* **aiplatform:** Add Feature Monitoring API to Feature Store ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))
* **aiplatform:** Add fields grounding_chunks and grounding_supports to GroundingMetadata ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** Add file_id to SharePointSource ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **aiplatform:** Add FLEX_START to Scheduling.strategy ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add FLEX_START to Scheduling.strategy ([#10816](https://github.com/googleapis/google-cloud-go/issues/10816)) ([da8737e](https://github.com/googleapis/google-cloud-go/commit/da8737e2d955a1ca481e9df82c062dd075f86ad9))
* **aiplatform:** Add MALFORMED_FUNCTION_CALL to FinishReason ([d6c543c](https://github.com/googleapis/google-cloud-go/commit/d6c543c3969016c63e158a862fc173dff60fb8d9))
* **aiplatform:** Add max_wait_duration to Scheduling ([b3ea577](https://github.com/googleapis/google-cloud-go/commit/b3ea5776b171fc60b4e96035d56d35dbd7505f3b))
* **aiplatform:** Add max_wait_duration to Scheduling ([b3ea577](https://github.com/googleapis/google-cloud-go/commit/b3ea5776b171fc60b4e96035d56d35dbd7505f3b))
* **aiplatform:** Add model and contents fields to ComputeTokensRequest v1 ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **aiplatform:** Add model and contents fields to ComputeTokensRequest v1beta1 ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **aiplatform:** Add more configurability to feature_group.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add more configurability to feature_group.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add new `PipelineTaskRerunConfig` field to `pipeline_job.proto` ([#10876](https://github.com/googleapis/google-cloud-go/issues/10876)) ([2f0aec8](https://github.com/googleapis/google-cloud-go/commit/2f0aec894179304d234be6c792d82cf4336b6d0a))
* **aiplatform:** Add new `PscInterfaceConfig` field to `pipeline_job.proto` ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add new PscInterfaceConfig field to custom_job.proto ([#11024](https://github.com/googleapis/google-cloud-go/issues/11024)) ([706ecb2](https://github.com/googleapis/google-cloud-go/commit/706ecb2c813da3109035b986a642ca891a33847f))
* **aiplatform:** Add OFF to HarmBlockThreshold ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add OFF to HarmBlockThreshold ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add OptimizedConfig for feature_view ([f072178](https://github.com/googleapis/google-cloud-go/commit/f072178f6fd90537a5782395f4229e4c8b30af7e))
* **aiplatform:** Add partial_failure_bigquery_sink to ImportRagFilesConfig ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add partial_failure_gcs_sink tp ImportRagFilesConfig ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add partner_model_tuning_spec to TuningJob ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add Pinecone and Vector Search integration for Vertex RAG ([b9dfce5](https://github.com/googleapis/google-cloud-go/commit/b9dfce5e509d0c795e89c66b7f6a6bb356e3a172))
* **aiplatform:** Add pointwise and pairwise metrics to evaluation service ([#10627](https://github.com/googleapis/google-cloud-go/issues/10627)) ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Add preflight_validations to PipelineJob ([d6c543c](https://github.com/googleapis/google-cloud-go/commit/d6c543c3969016c63e158a862fc173dff60fb8d9))
* **aiplatform:** Add private_service_connect_config and service_attachment fields to DedicatedServingEndpoint v1 ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Add psc_automation_configs to DeployIndex v1 ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Add psc_automation_configs to DeployIndex v1beta1 ([0b3c268](https://github.com/googleapis/google-cloud-go/commit/0b3c268c564ffe0d87b0efc716f08afaf064b4cc))
* **aiplatform:** Add ragSource to feature_view.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add ragSource to feature_view.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add reservation affinity proto ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Add reservation affinity proto ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Add role field to TokensInfo v1 ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **aiplatform:** Add role field to TokensInfo v1beta1 ([3b15f9d](https://github.com/googleapis/google-cloud-go/commit/3b15f9db9e0ee3bff3d8d5aafc82cdc2a31d60fc))
* **aiplatform:** Add satisfies_pzs and satisfies_pzi fields to Model v1 ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Add satisfies_pzs and satisfies_pzi fields to Model v1beta1 ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Add satisfies_pzs and satisfies_pzi fields to Tensorboard v1 ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Add satisfies_pzs and satisfies_pzi fields to Tensorboard v1beta1 ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Add share_point_sources to ImportRagFilesConfig ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add share_point_sources to RagFile ([ba22f7b](https://github.com/googleapis/google-cloud-go/commit/ba22f7b5b8f21a39685017d2d8522456ce528c4c))
* **aiplatform:** Add share_point_sources to SharePointSources ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add sharepoint_folder_id to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add sharepoint_folder_path to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add sharepoint_site_name to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add spot field to Vertex Prediction's Dedicated Resources and Custom Training's Scheduling Strategy ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Add spot field to Vertex Prediction's Dedicated Resources and Custom Training's Scheduling Strategy ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Add StopNotebookRuntime method ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))
* **aiplatform:** Add StopNotebookRuntime method ([f307078](https://github.com/googleapis/google-cloud-go/commit/f307078bc14758671e43ac401efb855089b27752))
* **aiplatform:** Add streamRawPredict rpc to prediction service ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))
* **aiplatform:** Add sync watermark to feature_view_sync.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add sync watermark to feature_view_sync.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add system labels field to model garden deployments ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))
* **aiplatform:** Add tenant_id to SharePointSource ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Add text field in Segment ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** Add TunedModelRef and RebaseTunedModel Api for Vertex GenAiTuningService ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** Add TunedModelRef and RebaseTunedModel Api for Vertex GenAiTuningService ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** Add UpdateDeploymentResourcePool method to DeploymentResourcePoolService v1 ([#10454](https://github.com/googleapis/google-cloud-go/issues/10454)) ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Add UpdateDeploymentResourcePool method to DeploymentResourcePoolService v1beta1 ([#10475](https://github.com/googleapis/google-cloud-go/issues/10475)) ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Add UpdateEndpointLongRunning API in v1beta1 version ([f307078](https://github.com/googleapis/google-cloud-go/commit/f307078bc14758671e43ac401efb855089b27752))
* **aiplatform:** Add UpdateRagCorpus API for Vertex RAG ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** Add use_effective_order field to BleuSpec v1beta1 ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Add v1 NotebookExecutionJob to Schedule ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** Add Vector DB config for Vertex RAG (Weaviate + FeatureStore) ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** Added support for specifying function response type in `FunctionDeclaration` ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))
* **aiplatform:** Allow v1 api calls for some dataset_service, llm_utility_service, and prediction_service apis without project and location ([#10643](https://github.com/googleapis/google-cloud-go/issues/10643)) ([24616cc](https://github.com/googleapis/google-cloud-go/commit/24616cc136a1ba49a551aad6f76d4d2e062d267c))
* **aiplatform:** Allow v1beta1 api calls for some dataset_service, llm_utility_service, and prediction_service apis without project and location ([24616cc](https://github.com/googleapis/google-cloud-go/commit/24616cc136a1ba49a551aad6f76d4d2e062d267c))
* **aiplatform:** Enable rest_numeric_enums for aiplatform v1 and v1beta1 ([#10524](https://github.com/googleapis/google-cloud-go/issues/10524)) ([f46b747](https://github.com/googleapis/google-cloud-go/commit/f46b747fe2b0f89085ee5745652db7a46b049074))
* **aiplatform:** Expose `RuntimeArtifact` proto in `ui_pipeline_spec.proto` ([2f0aec8](https://github.com/googleapis/google-cloud-go/commit/2f0aec894179304d234be6c792d82cf4336b6d0a))
* **aiplatform:** Introduce DefaultRuntime to PipelineJob ([70d82fe](https://github.com/googleapis/google-cloud-go/commit/70d82fe93f60f1075298a077ce1616f9ae7e13fe))
* **aiplatform:** MetricX added to evaluation service proto ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** Release advanced parsing options for rag files ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** Returns usage metadata for context caching ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))


### Bug Fixes

* **aiplatform:** An existing field `disable_attribution` is removed from message `.google.cloud.aiplatform.v1beta1.GoogleSearchRetrieval` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** An existing field `grounding_attributions` is removed from message `.google.cloud.aiplatform.v1beta1.GroundingMetadata` ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** An existing message `GroundingAttribution` is removed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** Annotate PipelineJob and PipelineTaskRerunConfig fields as optional ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **aiplatform:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))
* **aiplatform:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))
* **aiplatform:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))
* **aiplatform:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))
* **aiplatform:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))
* **aiplatform:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **aiplatform:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))


### Documentation

* **aiplatform:** A comment for enum `Strategy` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum `Strategy` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `AUTO` in enum `Mode` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `AUTO` in enum `Mode` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `BLOCKLIST` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `BLOCKLIST` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `MAX_TOKENS` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `MAX_TOKENS` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `OTHER` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `OTHER` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `PROHIBITED_CONTENT` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `PROHIBITED_CONTENT` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `RECITATION` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `RECITATION` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `SAFETY` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `SAFETY` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `SPII` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `SPII` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `STOP` in enum `FinishReason` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `STOP` in enum `FinishReason` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for enum value `STRATEGY_UNSPECIFIED` in enum `Strategy` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for enum value `STRATEGY_UNSPECIFIED` in enum `Strategy` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for field `contents` in message `.google.cloud.aiplatform.v1.CountTokensRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `contents` in message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `disable_attribution` in message `.google.cloud.aiplatform.v1.Retrieval` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `disable_attribution` in message `.google.cloud.aiplatform.v1beta1.Retrieval` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `distance` in message `.google.cloud.aiplatform.v1beta1.RagContexts` is changed ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** A comment for field `distance` in message `.google.cloud.aiplatform.v1beta1.RagContexts` is changed ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))
* **aiplatform:** A comment for field `distibution` in message `.google.cloud.aiplatform.v1beta1.model_monitoring_stats.ModelMonitoringStatsDataPoint` is changed. ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** A comment for field `instances` in message `.google.cloud.aiplatform.v1.CountTokensRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `instances` in message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `language_code` in message `.google.cloud.aiplatform.v1.GetPublisherModelRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `language_code` in message `.google.cloud.aiplatform.v1beta1.GetPublisherModelRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `language_code` in message `.google.cloud.aiplatform.v1beta1.ListPublisherModelsRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `model` in message `.google.cloud.aiplatform.v1.CountTokensRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `model` in message `.google.cloud.aiplatform.v1.GenerateContentRequest` is changed ([0f25bf2](https://github.com/googleapis/google-cloud-go/commit/0f25bf264db64724af359f9e9c83c0acb8947dd5))
* **aiplatform:** A comment for field `model` in message `.google.cloud.aiplatform.v1beta1.CountTokensRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `name` in message `.google.cloud.aiplatform.v1.Dataset` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `name` in message `.google.cloud.aiplatform.v1.DatasetVersion` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `name` in message `.google.cloud.aiplatform.v1beta1.cached_content.CachedContent` is changed ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** A comment for field `name` in message `.google.cloud.aiplatform.v1beta1.Dataset` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `name` in message `.google.cloud.aiplatform.v1beta1.DatasetVersion` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `partner_model_tuning_spec` in message `.google.cloud.aiplatform.v1beta1.TuningJob` is changed ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **aiplatform:** A comment for field `restart_job_on_worker_restart` in message `.google.cloud.aiplatform.v1.Scheduling` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for field `source` in message `.google.cloud.aiplatform.v1beta1.tool.Retrieval` is added. ([2003148](https://github.com/googleapis/google-cloud-go/commit/2003148b71a734afd5c31a0106e5204414ece6e9))
* **aiplatform:** A comment for field `timeout` in message `.google.cloud.aiplatform.v1.Scheduling` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1.UpdateFeatureGroupRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1.UpdateFeatureOnlineStoreRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1.UpdateFeatureRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1.UpdateFeatureViewRequest` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1.UpdateFeatureViewRequest` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1beta1.UpdateFeatureGroupRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1beta1.UpdateFeatureOnlineStoreRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1beta1.UpdateFeatureRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1beta1.UpdateFeatureViewRequest` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for field `vertex_prediction_endpoint` in message `.google.cloud.aiplatform.v1beta1.RagEmbeddingModelConfig` is changed ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))
* **aiplatform:** A comment for message `GetDatasetRequest` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for message `GetDatasetVersionRequest` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for message `TrialContext` is changed ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))
* **aiplatform:** A comment for message `TrialContext` is changed ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **aiplatform:** A comment for method `ListAnnotations` in service `DatasetService` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** A comment for method `RebaseTunedModel` in service `GenAiTuningService` is changed ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **aiplatform:** Fix typo in feature_online_store_admin_service.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Fix typo in feature_online_store_admin_service.proto ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **aiplatform:** Limit comment `SupervisedTuningSpec` for 1p tuning ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** Limit comment `SupervisedTuningSpec` for 1p tuning ([7250d71](https://github.com/googleapis/google-cloud-go/commit/7250d714a638dcd5df3fbe0e91c5f1250c3f80f9))
* **aiplatform:** Update comments of AutoscalingSpec v1 ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **aiplatform:** Update comments of AutoscalingSpec v1beta1 ([b15d840](https://github.com/googleapis/google-cloud-go/commit/b15d8401d3ea7f2add1d0d93bc26ceb2edd9b9fc))
* **aiplatform:** Update feature creation message commentary ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))
* **aiplatform:** Update the description for the deprecated GPU (K80) ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Update the description for the deprecated GPU (K80) ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **aiplatform:** Updated the maximum number of function declarations from 64 to 128 ([abf9cba](https://github.com/googleapis/google-cloud-go/commit/abf9cba74a78c0a909fa43e934f33bf0f59e83c1))

## [1.68.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.67.0...aiplatform/v1.68.0) (2024-06-11)


### Features

* **aiplatform/apiv1beta1:** Add methods to info.go ([#10288](https://github.com/googleapis/google-cloud-go/issues/10288)) ([882fe5c](https://github.com/googleapis/google-cloud-go/commit/882fe5c8ebc3afe800da2125c09595761e1a5e87))
* **aiplatform:** A new field `search_entry_point` is added to message `.google.cloud.aiplatform.v1beta1.GroundingMetadata` ([ae42f23](https://github.com/googleapis/google-cloud-go/commit/ae42f23f586ad76b058066a66c1566e4fef23692))
* **aiplatform:** A new value `TPU_V5_LITEPOD` is added to enum `AcceleratorType` ([#10074](https://github.com/googleapis/google-cloud-go/issues/10074)) ([7656129](https://github.com/googleapis/google-cloud-go/commit/7656129e1cffbfb788d849f3b35c28c7ac69054f))
* **aiplatform:** Add cached_content to GenerationContentRequest ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add ChatCompletions to PredictionService ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add dataplex_config to MetadataStore ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add dataplex_config to MetadataStore ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add direct_notebook_source to NotebookExecutionJob ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add direct_notebook_source to NotebookExecutionJob ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec to FeatureOnlineStore ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec to FeatureOnlineStore ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec to NotebookRuntimeTemplate ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec to NotebookRuntimeTemplate ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec, service_account, disable_container_logging to DeploymentResourcePool ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add encryption_spec, service_account, disable_container_logging to DeploymentResourcePool ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add idle_shutdown_config, encryption_spec, satisfies_pzs, satisfies_pzi to NotebookRuntime ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add idle_shutdown_config, encryption_spec, satisfies_pzs, satisfies_pzi to NotebookRuntime ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add INVALID_SPARSE_DIMENSIONS, INVALID_SPARSE_EMBEDDING, INVALID_EMBEDDING to NearestNeighborSearchOperationMetadata.RecordError ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add INVALID_SPARSE_DIMENSIONS, INVALID_SPARSE_EMBEDDING, INVALID_EMBEDDING to NearestNeighborSearchOperationMetadata.RecordError ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add max_embedding_requests_per_min to ImportRagFilesConfig ([d5150d3](https://github.com/googleapis/google-cloud-go/commit/d5150d34eabac0218cbd16a9bbdaaaf019cf237d))
* **aiplatform:** Add model_monitor resource and APIs to public v1beta1 client library ([#9755](https://github.com/googleapis/google-cloud-go/issues/9755)) ([8892943](https://github.com/googleapis/google-cloud-go/commit/8892943b169060f8ba7be227cd65680696c494a0))
* **aiplatform:** Add model_reference to Dataset ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add model_reference to Dataset ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add model_reference to DatasetVersion ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add model_reference to DatasetVersion ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add more fields in FindNeighborsRequest.Query ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add more fields in FindNeighborsRequest.Query ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add new GenAiCacheService and CachedContent ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add NotebookExecutionJob resource and APIs to public v1beta1 client library ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **aiplatform:** Add progress_percentage to ImportRagFilesOperationMetadata ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add rag_embedding_model_config to RagCorpus ([d5150d3](https://github.com/googleapis/google-cloud-go/commit/d5150d34eabac0218cbd16a9bbdaaaf019cf237d))
* **aiplatform:** Add RaySpec to PersistentResource ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_distance to FindNeighborsResponse.Neighbor ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_distance to FindNeighborsResponse.Neighbor ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_embedding to IndexDatapoint ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_embedding to IndexDatapoint ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_vectors_count to IndexStats ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add sparse_vectors_count to IndexStats ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add struct_value to FeatureValue ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add struct_value to FeatureValue ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add tool_config to GenerateContentRequest ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add UpdateNotebookRuntimeTemplate to NotebookService ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add UpdateNotebookRuntimeTemplate to NotebookService ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add UpdateReasoningEngine to ReasoningEngineService ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add valid_sparse_record_count, invalid_sparse_record_count to NearestNeighborSearchOperationMetadata.ContentValidationStats ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add valid_sparse_record_count, invalid_sparse_record_count to NearestNeighborSearchOperationMetadata.ContentValidationStats ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add ValueType.STRUCT to Feature ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Add ValueType.STRUCT to Feature ([#10282](https://github.com/googleapis/google-cloud-go/issues/10282)) ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Added the new `GenerationConfig.response_schema` field ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **aiplatform:** Added the v1beta1 version of the GenAI Tuning Service ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))


### Bug Fixes

* **aiplatform:** An existing field `app_id` is renamed to `engine_id` in message `.google.cloud.aiplatform.v1beta1.RuntimeConfig` ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** An existing field `disable_attribution` is removed from message `.google.cloud.aiplatform.v1beta1.GoogleSearchRetrieval` ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** An existing field `grounding_attributions` is removed from message `.google.cloud.aiplatform.v1beta1.GroundingMetadata` ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** An existing message `GroundingAttribution` is removed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** An existing message `Segment` is removed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))
* **aiplatform:** Delete the deprecated field for model monitor ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Documentation

* **aiplatform:** A comment for enum value `EMBEDDING_SIZE_MISMATCH` in enum `RecordErrorType` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for enum value `EMBEDDING_SIZE_MISMATCH` in enum `RecordErrorType` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `create_notebook_execution_job_request` in message `.google.cloud.aiplatform.v1beta1.Schedule` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `description` in message `.google.cloud.aiplatform.v1beta1.ExtensionManifest` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `exec` in message `.google.cloud.aiplatform.v1beta1.Probe` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `exec` in message `.google.cloud.aiplatform.v1beta1.Probe` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `feature_vector` in message `.google.cloud.aiplatform.v1beta1.IndexDatapoint` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `feature_vector` in message `.google.cloud.aiplatform.v1beta1.IndexDatapoint` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `INVALID_EMBEDDING` in message `NearestNeighborSearchOperationMetadata.RecordError` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `serving_config_name` in message `.google.cloud.aiplatform.v1beta1.RuntimeConfig` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `update_mask` in message `.google.cloud.aiplatform.v1beta1.UpdateExtensionRequest` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `vectors_count` in message `.google.cloud.aiplatform.v1beta1.IndexStats` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))
* **aiplatform:** A comment for field `vectors_count` in message `.google.cloud.aiplatform.v1beta1.IndexStats` is changed ([fac63c3](https://github.com/googleapis/google-cloud-go/commit/fac63c33a1c8452516fd78d841780c524ad2f730))

## [1.67.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.66.0...aiplatform/v1.67.0) (2024-04-08)


### Features

* **aiplatform:** A new field `presence_penalty` is added to message `.google.cloud.aiplatform.v1.GenerationConfig` ([dd7c8e5](https://github.com/googleapis/google-cloud-go/commit/dd7c8e5a206ca6fab7d05e2591a36ea706e5e9f1))
* **aiplatform:** Add NotebookRuntime resource and APIs to public v1 client library ([dd7c8e5](https://github.com/googleapis/google-cloud-go/commit/dd7c8e5a206ca6fab7d05e2591a36ea706e5e9f1))
* **aiplatform:** Add NotebookRuntime resource and APIs to public v1beta1 client library ([dd7c8e5](https://github.com/googleapis/google-cloud-go/commit/dd7c8e5a206ca6fab7d05e2591a36ea706e5e9f1))
* **aiplatform:** Add Persistent Resource reboot api call to v1beta1 ([#9680](https://github.com/googleapis/google-cloud-go/issues/9680)) ([e7342a3](https://github.com/googleapis/google-cloud-go/commit/e7342a3794aec038d6fdc195da4f8df23b1eeca1))
* **aiplatform:** GenAiTuningService aiplatform v1 initial release ([#9679](https://github.com/googleapis/google-cloud-go/issues/9679)) ([543a58d](https://github.com/googleapis/google-cloud-go/commit/543a58dc0df2ff0aa384ffec41c9ab45a893a714))


### Bug Fixes

* **aiplatform:** An existing field `response_recall_input` is removed from message `.google.cloud.aiplatform.v1beta1.EvaluateInstancesRequest` ([#9672](https://github.com/googleapis/google-cloud-go/issues/9672)) ([dd7c8e5](https://github.com/googleapis/google-cloud-go/commit/dd7c8e5a206ca6fab7d05e2591a36ea706e5e9f1))

## [1.66.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.65.0...aiplatform/v1.66.0) (2024-03-27)


### Features

* **aiplatform:** Add Vertex AI extension registry and execution related API and services to v1beta1 client ([4834425](https://github.com/googleapis/google-cloud-go/commit/48344254a5d21ec51ffee275c78a15c9345dc09c))
* **aiplatform:** Evaluation Service aiplatform v1beta1 initial release ([f8ff971](https://github.com/googleapis/google-cloud-go/commit/f8ff971366999aefb5eb5189c6c9e2bd76a05d9e))

## [1.65.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.64.0...aiplatform/v1.65.0) (2024-03-25)


### Features

* **aiplatform:** Add function_calling_config to ToolConfig ([cddd528](https://github.com/googleapis/google-cloud-go/commit/cddd528a02edae10dde8ba2529922565ef27c418))
* **aiplatform:** Add Optimized feature store proto ([#9635](https://github.com/googleapis/google-cloud-go/issues/9635)) ([94f9463](https://github.com/googleapis/google-cloud-go/commit/94f9463f890ed886622ee65edfbc4b5ecdfa97f8))
* **aiplatform:** Reasoning Engine v1beta1 GAPIC release ([1ef5b19](https://github.com/googleapis/google-cloud-go/commit/1ef5b1917bb9a1271c3fb152413ec0e74163164d))


### Documentation

* **aiplatform:** Update the description for reasoning engine ([1ef5b19](https://github.com/googleapis/google-cloud-go/commit/1ef5b1917bb9a1271c3fb152413ec0e74163164d))

## [1.64.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.63.0...aiplatform/v1.64.0) (2024-03-14)


### Features

* **aiplatform:** Add v1beta1 StreamingFetchFeatureValues API ([#9568](https://github.com/googleapis/google-cloud-go/issues/9568)) ([05f58cc](https://github.com/googleapis/google-cloud-go/commit/05f58ccce530d8a3ab404356929352002d5156ba))

## [1.63.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.62.2...aiplatform/v1.63.0) (2024-03-12)


### Features

* **aiplatform:** A new value `NVIDIA_H100_80GB` is added to enum `AcceleratorType` ([ccfe599](https://github.com/googleapis/google-cloud-go/commit/ccfe59970fac372e07202d26c520e36e0b3b9598))

## [1.62.2](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.62.1...aiplatform/v1.62.2) (2024-03-07)


### Bug Fixes

* **aiplatform:** An existing field `preflight_validations` is removed from message `.google.cloud.aiplatform.v1beta1.CreatePipelineJobRequest` ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))

## [1.62.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.62.0...aiplatform/v1.62.1) (2024-03-04)


### Documentation

* **aiplatform:** Update docs for FeatureView Service Agents ([d130d86](https://github.com/googleapis/google-cloud-go/commit/d130d861f55d137a2803340c2e11da3589669cb8))

## [1.62.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.61.0...aiplatform/v1.62.0) (2024-02-26)


### Features

* **aiplatform:** Add `point_of_contact` to `Feature` message ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))
* **aiplatform:** Add CompositeKey message and composite_key field to FeatureViewDataKey ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))
* **aiplatform:** Add CompositeKey message and composite_key field to FeatureViewDataKey ([#9452](https://github.com/googleapis/google-cloud-go/issues/9452)) ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))
* **aiplatform:** Add point_of_contact to feature ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))
* **aiplatform:** Add RayMetricSpec to persistent resource ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))
* **aiplatform:** Enable FeatureView Service Agents ([3814ee3](https://github.com/googleapis/google-cloud-go/commit/3814ee3f27724ad0d02688ad86030b83e0a72fd4))

## [1.61.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.60.0...aiplatform/v1.61.0) (2024-02-21)


### Features

* **aiplatform:** Add Grounding feature to PredictionService.GenerateContent ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))


### Bug Fixes

* **aiplatform:** Remove field `max_wait_duration` from message Scheduling ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))

## [1.60.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.59.0...aiplatform/v1.60.0) (2024-02-09)


### Features

* **aiplatform:** Add SearchNearestEntities rpc to FeatureOnlineStoreService in aiplatform v1 ([#9385](https://github.com/googleapis/google-cloud-go/issues/9385)) ([46a5050](https://github.com/googleapis/google-cloud-go/commit/46a50502f033ff0afe2f17b5f1e9812a956e190e))


### Bug Fixes

* **aiplatform:** Remove field `max_wait_duration` from message Scheduling ([#9387](https://github.com/googleapis/google-cloud-go/issues/9387)) ([f049c97](https://github.com/googleapis/google-cloud-go/commit/f049c9751415f9fc4c81c1839a8371782cfc016c))

## [1.59.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.58.2...aiplatform/v1.59.0) (2024-02-06)


### Features

* **aiplatform:** Add generateContent Unary API for aiplatform_v1 ([05e9e1f](https://github.com/googleapis/google-cloud-go/commit/05e9e1f53f2a0c8b3aaadc1811338ca3e682f245))
* **aiplatform:** Add generateContent Unary API for aiplatform_v1beta1 ([05e9e1f](https://github.com/googleapis/google-cloud-go/commit/05e9e1f53f2a0c8b3aaadc1811338ca3e682f245))

## [1.58.2](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.58.1...aiplatform/v1.58.2) (2024-01-30)


### Bug Fixes

* **aiplatform:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))


### Documentation

* **aiplatform:** Add comments for FeatureOnlineStoreService and ModelMonitoringAlertConfig ([#9326](https://github.com/googleapis/google-cloud-go/issues/9326)) ([4d56af1](https://github.com/googleapis/google-cloud-go/commit/4d56af183d42ff12862c0c35226e767ed8763118))

## [1.58.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.58.0...aiplatform/v1.58.1) (2024-01-22)


### Bug Fixes

* **aiplatform:** Fix rpc tensorboard_service.proto definitions for BatchCreateTensorboardTimeSeries and BatchReadTensorboardTimeSeriesData ([04ce84d](https://github.com/googleapis/google-cloud-go/commit/04ce84d23e734bbbb84e65bbf840d5ea294a2384))
* **aiplatform:** Fix rpc tensorboard_service.proto definitions for BatchCreateTensorboardTimeSeries and BatchReadTensorboardTimeSeriesData ([#9247](https://github.com/googleapis/google-cloud-go/issues/9247)) ([04ce84d](https://github.com/googleapis/google-cloud-go/commit/04ce84d23e734bbbb84e65bbf840d5ea294a2384))

## [1.58.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.57.0...aiplatform/v1.58.0) (2023-12-13)


### Features

* **aiplatform:** Expose ability to set headers ([#9150](https://github.com/googleapis/google-cloud-go/issues/9150)) ([2007541](https://github.com/googleapis/google-cloud-go/commit/20075417dfd3e7ba47f77586d5ec366fa68285a2))

## [1.57.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.56.0...aiplatform/v1.57.0) (2023-12-11)


### Features

* **aiplatform:** Add Content ([29effe6](https://github.com/googleapis/google-cloud-go/commit/29effe600e16f24a127a1422ec04263c4f7a600a))
* **aiplatform:** Add Content ([29effe6](https://github.com/googleapis/google-cloud-go/commit/29effe600e16f24a127a1422ec04263c4f7a600a))

## [1.56.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.55.0...aiplatform/v1.56.0) (2023-12-07)


### Features

* **aiplatform:** Add data_stats to Model ([5132d0f](https://github.com/googleapis/google-cloud-go/commit/5132d0fea3a5ac902a2c9eee865241ed4509a5f4))
* **aiplatform:** Add grpc_ports to UploadModel ModelContainerSpec ([#9059](https://github.com/googleapis/google-cloud-go/issues/9059)) ([0685da5](https://github.com/googleapis/google-cloud-go/commit/0685da5391aea1dca7ebb52e1d3392a9b6fc06c2))

## [1.55.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.54.0...aiplatform/v1.55.0) (2023-11-27)


### Features

* **aiplatform:** Add grpc_ports to UploadModel ModelContainerSpec ([2020edf](https://github.com/googleapis/google-cloud-go/commit/2020edff24e3ffe127248cf9a90c67593c303e18))

## [1.54.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.53.0...aiplatform/v1.54.0) (2023-11-16)


### Features

* **aiplatform:** Add ComputeTokens and CountTokens API ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))
* **aiplatform:** Add ComputeTokens API ([#8999](https://github.com/googleapis/google-cloud-go/issues/8999)) ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))
* **aiplatform:** Add deployment_timeout to UploadModel ModelContainerSpec ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))
* **aiplatform:** Add deployment_timeout to UploadModel ModelContainerSpec ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))
* **aiplatform:** Add protected_artifact_location_id to CustomJob ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))
* **aiplatform:** Add protected_artifact_location_id to CustomJob ([f2b5cbb](https://github.com/googleapis/google-cloud-go/commit/f2b5cbb35da5a4eca937a1441b6a0f1b147e072b))

## [1.53.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.52.0...aiplatform/v1.53.0) (2023-11-09)


### Features

* **aiplatform:** Add Optimized to FeatureOnlineStore ([1a16cbf](https://github.com/googleapis/google-cloud-go/commit/1a16cbf260bb673e07a05e1014868b236e510499))


### Bug Fixes

* **aiplatform:** Change CreateFeature metadata ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [1.52.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.51.2...aiplatform/v1.52.0) (2023-11-01)


### Features

* **aiplatform:** Adding new fields for concurrent explanations ([24e410e](https://github.com/googleapis/google-cloud-go/commit/24e410efbb6add2d33ecfb6ad98b67dc8894e578))


### Bug Fixes

* **aiplatform:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.51.2](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.51.1...aiplatform/v1.51.2) (2023-10-26)


### Bug Fixes

* **aiplatform:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.51.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.51.0...aiplatform/v1.51.1) (2023-10-12)


### Bug Fixes

* **aiplatform:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.51.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.50.0...aiplatform/v1.51.0) (2023-10-04)


### Features

* **aiplatform:** Add DatasetVersion and dataset version RPCs to DatasetService ([481127f](https://github.com/googleapis/google-cloud-go/commit/481127fb8271cab3a754e0e1820b32567e80524a))
* **aiplatform:** Add DatasetVersion and dataset version RPCs to DatasetService ([481127f](https://github.com/googleapis/google-cloud-go/commit/481127fb8271cab3a754e0e1820b32567e80524a))
* **aiplatform:** Add dedicated_serving_endpoint ([57fc1a6](https://github.com/googleapis/google-cloud-go/commit/57fc1a6de326456eb68ef25f7a305df6636ed386))
* **aiplatform:** Add feature.proto ([e9ae601](https://github.com/googleapis/google-cloud-go/commit/e9ae6018983ae09781740e4ff939e6e365863dbb))

## [1.50.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.49.0...aiplatform/v1.50.0) (2023-09-11)


### Features

* **aiplatform:** Add encryption_spec to index.proto and index_endpoint.proto ([ac10224](https://github.com/googleapis/google-cloud-go/commit/ac102249403e6c1604bff7c537343645c950ae13))
* **aiplatform:** Add encryption_spec to index.proto and index_endpoint.proto ([ac10224](https://github.com/googleapis/google-cloud-go/commit/ac102249403e6c1604bff7c537343645c950ae13))
* **aiplatform:** Add UpdatePersistentResourceRequest and add resource_pool_images and head_node_resource_pool_id to RaySpec ([fbfaf21](https://github.com/googleapis/google-cloud-go/commit/fbfaf21c15ae8a07ab39c6036cf0cee700b5627c))

## [1.49.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.48.0...aiplatform/v1.49.0) (2023-08-17)


### Features

* **aiplatform:** Add NVIDIA_H100_80GB and TPU_V5_LITEPOD to AcceleratorType ([b3dbdde](https://github.com/googleapis/google-cloud-go/commit/b3dbdde48ddfa215c3c3bb110e0051fd8158f451))
* **aiplatform:** Update field_behavior for `name` to be IMMUTABLE instead of OUTPUT_ONLY in Context, ModelMonitor, Schedule, DeploymentResourcePool ([b3dbdde](https://github.com/googleapis/google-cloud-go/commit/b3dbdde48ddfa215c3c3bb110e0051fd8158f451))

## [1.48.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.47.0...aiplatform/v1.48.0) (2023-07-31)


### Features

* **aiplatform:** Add `PredictionService.ServerStreamingPredict` method ([b890425](https://github.com/googleapis/google-cloud-go/commit/b8904253a0f8424ea4548469e5feef321bd7396a))
* **aiplatform:** Add RaySepc to ResourceRuntimeSpec, and add ResourceRuntime to PersistentResource ([b890425](https://github.com/googleapis/google-cloud-go/commit/b8904253a0f8424ea4548469e5feef321bd7396a))

## [1.47.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.46.0...aiplatform/v1.47.0) (2023-07-26)


### Features

* **aiplatform:** ScheduleService (schedule_service.proto) creates and manages Schedule resources to launch scheduled pipelines runs ([7cb7f66](https://github.com/googleapis/google-cloud-go/commit/7cb7f66f0646617c27aa9a9b4fe38b9f368eb3bb))

## [1.46.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.45.0...aiplatform/v1.46.0) (2023-07-18)


### Features

* **aiplatform:** Add data_item_count to Dataset ([22a908b](https://github.com/googleapis/google-cloud-go/commit/22a908b0bd26f131c6033ec3fc48eaa2d2cd0c0e))
* **aiplatform:** Add data_item_count to Dataset ([#8249](https://github.com/googleapis/google-cloud-go/issues/8249)) ([244b14e](https://github.com/googleapis/google-cloud-go/commit/244b14e4fe424100a6ff2b05637375fafe084673))

## [1.45.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.44.0...aiplatform/v1.45.0) (2023-06-20)


### Features

* **aiplatform:** Add bias_configs to ModelEvaluation ([b726d41](https://github.com/googleapis/google-cloud-go/commit/b726d413166faa8c84c0a09c6019ff50f3249b9d))
* **aiplatform:** Add UpdateExplanationDataset to aiplatform ([#8118](https://github.com/googleapis/google-cloud-go/issues/8118)) ([b726d41](https://github.com/googleapis/google-cloud-go/commit/b726d413166faa8c84c0a09c6019ff50f3249b9d))


### Bug Fixes

* **aiplatform:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.44.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform-v1.43.0...aiplatform/v1.44.0) (2023-06-07)


### Features

* **aiplatform:** Add blocking_operation_ids to ImportFeatureValuesOperationMetadata ([#8036](https://github.com/googleapis/google-cloud-go/issues/8036)) ([4f98e1a](https://github.com/googleapis/google-cloud-go/commit/4f98e1a919d61fb396fe95197a7b17376184d966))
* **aiplatform:** Add NVIDIA_A100_80GB to AcceleratorType ([4f98e1a](https://github.com/googleapis/google-cloud-go/commit/4f98e1a919d61fb396fe95197a7b17376184d966))
* **aiplatform:** Support for Model Garden -- A single place to search, discover, and interact with a wide variety of foundation models from Google and Google partners, available on Vertex AI ([#8026](https://github.com/googleapis/google-cloud-go/issues/8026)) ([75cf009](https://github.com/googleapis/google-cloud-go/commit/75cf009802303edb9c157b3cb686f8bc6786c62b))

## [1.43.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.42.0...aiplatform/v1.43.0) (2023-05-30)


### Features

* **aiplatform:** Add match service in aiplatform v1 ([2b3e7d9](https://github.com/googleapis/google-cloud-go/commit/2b3e7d9af7d2f500e736e3db77487127cb44ca23))
* **aiplatform:** Add updateSchedule method to ScheduleService ([2b3e7d9](https://github.com/googleapis/google-cloud-go/commit/2b3e7d9af7d2f500e736e3db77487127cb44ca23))

## [1.42.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.41.0...aiplatform/v1.42.0) (2023-05-16)


### Features

* **aiplatform:** Add examples to ExplanationParameters in aiplatform v1 explanation.proto ([7c2f642](https://github.com/googleapis/google-cloud-go/commit/7c2f642ac308fcdfcb41985aae425785afa27823))

## [1.41.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.40.1...aiplatform/v1.41.0) (2023-05-10)


### Features

* **aiplatform:** Add example_gcs_source to Examples in aiplatform v1beta1 explanation.proto PiperOrigin-RevId: 529739833 ([31c3766](https://github.com/googleapis/google-cloud-go/commit/31c3766c9c4cab411669c14fc1a30bd6d2e3f2dd))

## [1.40.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.40.0...aiplatform/v1.40.1) (2023-05-08)


### Bug Fixes

* **aiplatform:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.40.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.39.0...aiplatform/v1.40.0) (2023-05-01)


### Features

* **aiplatform:** Support for Model Garden -- A single place to search, discover, and interact with a wide variety of foundation models from Google and Google partners, available on Vertex AI ([#7849](https://github.com/googleapis/google-cloud-go/issues/7849)) ([ac00efc](https://github.com/googleapis/google-cloud-go/commit/ac00efcab5d7e2292d5b7cc60dd1196a1f8279a4))

## [1.39.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.38.0...aiplatform/v1.39.0) (2023-04-25)


### Features

* **aiplatform:** Add is_default to Tensorboard in aiplatform v1 tensorboard.proto and v1beta1 tensorboard.proto ([87a67b4](https://github.com/googleapis/google-cloud-go/commit/87a67b44b2c7ffc3cea986b255614ea0d21aa6fc))

## [1.38.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.37.0...aiplatform/v1.38.0) (2023-04-11)


### Features

* **aiplatform:** Add notification_channels in aiplatform v1beta1 model_monitoring.proto ([#7719](https://github.com/googleapis/google-cloud-go/issues/7719)) ([23c974a](https://github.com/googleapis/google-cloud-go/commit/23c974a019693e6453c1342cad172df77f86974e))

## [1.37.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.36.1...aiplatform/v1.37.0) (2023-04-04)


### Features

* **aiplatform:** Add public_endpoint_enabled and publid_endpoint_domain_name to IndexEndpoint ([c893c15](https://github.com/googleapis/google-cloud-go/commit/c893c158f1e6d03b0cde45dda2059c0e2aa9ead1))
* **aiplatform:** Add public_endpoint_enabled and publid_endpoint_domain_name to IndexEndpoint ([c893c15](https://github.com/googleapis/google-cloud-go/commit/c893c158f1e6d03b0cde45dda2059c0e2aa9ead1))
* **aiplatform:** ScheduleService (schedule_service.proto) creates and manages Schedule resources to launch scheduled pipelines runs ([597ea0f](https://github.com/googleapis/google-cloud-go/commit/597ea0fe09bcea04e884dffe78add850edb2120d))

## [1.36.1](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.36.0...aiplatform/v1.36.1) (2023-03-22)


### Bug Fixes

* **aiplatform:** Remove large_model_reference from Model in aiplatform v1beta1 model.proto ([#7582](https://github.com/googleapis/google-cloud-go/issues/7582)) ([4497130](https://github.com/googleapis/google-cloud-go/commit/44971302a2a4bd0eee6c50524b630bad41b2cca4))

## [1.36.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.35.0...aiplatform/v1.36.0) (2023-03-15)


### Features

* **aiplatform:** Add disable_container_logging to BatchPredictionJob in aiplatform v1,v1beta1 batch_prediction_job.proto ([8c98464](https://github.com/googleapis/google-cloud-go/commit/8c9846414f57620db198bad863cca38529d39e9e))
* **aiplatform:** Add evaluated_annotation.proto to aiplatform v1beta1 ([8c98464](https://github.com/googleapis/google-cloud-go/commit/8c9846414f57620db198bad863cca38529d39e9e))
* **aiplatform:** Add split to ExportDataConfig in aiplatform v1 dataset.proto ([8c98464](https://github.com/googleapis/google-cloud-go/commit/8c9846414f57620db198bad863cca38529d39e9e))

## [1.35.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.34.0...aiplatform/v1.35.0) (2023-02-22)


### Features

* **aiplatform:** Add match service in aiplatform v1beta1 match_service.proto ([932ddc8](https://github.com/googleapis/google-cloud-go/commit/932ddc87ed3889bd5b132d4c2307b1017c3ef3a2))

## [1.34.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.33.0...aiplatform/v1.34.0) (2023-02-14)


### Features

* **aiplatform:** Add disable_explanations to DeployedModel in aiplatform v1beta1 endpoint.proto ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))
* **aiplatform:** Add service_networking.proto to aiplatform v1 ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))

## [1.33.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform-v1.32.0...aiplatform/v1.33.0) (2023-01-26)


### Features

* **aiplatform/apiv1beta1:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **aiplatform:** Add annotation_labels to ImportDataConfig in aiplatform v1 dataset.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **aiplatform:** Add deleteFeatureValues in aiplatform v1beta1 featurestore_service.proto ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))
* **aiplatform:** Add DeploymentResourcePool in aiplatform v1beta1 deployment_resource_pool.proto feat: add DeploymentResourcePoolService in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add SHARED_RESOURCES to DeploymentResourcesType in aiplatform v1beta1 model.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Add enable_dashboard_access in aiplatform v1 and v1beta1 custom_job.proto ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **aiplatform:** Add instance_config to batch_prediction_job in aiplatform v1beta1 batch_prediction_job.proto ([2b4957c](https://github.com/googleapis/google-cloud-go/commit/2b4957c7c348ecf5952e02f3602379fffaa758b4))
* **aiplatform:** Add instance_config to BatchPredictionJob in aiplatform v1 batch_prediction_job.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add metadata_artifact to Dataset in aiplatform v1 dataset.proto feat: add WriteFeatureValues rpc to FeaturestoreOnlineServingService in aiplatform v1 featurestore_online_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** Add metadata_artifact to Dataset in aiplatform v1beta1 dataset.proto feat: add offline_storage_ttl_days to EntityType in aiplatform v1beta1 entity_type.proto feat: add online_storage_ttl_days to Featurestore in aiplatform v1beta1 featurestore.proto feat: add source_uris to ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** Add model_monitoring_stats_anomalies,model_monitoring_status to BatchPredictionJob in aiplatform v1beta1 batch_prediction_job.proto ([e45ad9a](https://github.com/googleapis/google-cloud-go/commit/e45ad9af568c59151decc0dacedf137653b576dd))
* **aiplatform:** Add model_source_info to Model in aiplatform v1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add model_source_info to Model in aiplatform v1beta1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add NVIDIA_A100_80GB to AcceleratorType in aiplatform v1beta1 accelerator_type.proto feat: add annotation_labels to ImportDataConfig in aiplatform v1beta1 dataset.proto feat: add total_deployed_model_count and total_endpoint_count to QueryDeployedModelsResponse in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1beta1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1beta1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1beta1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1beta1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **aiplatform:** Add read_mask to ListPipelineJobsRequest in aiplatform v1 pipeline_service feat: add input_artifacts to PipelineJob.runtime_config in aiplatform v1 pipeline_job ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))
* **aiplatform:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1beta1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1beta1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1beta1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1beta1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add service_account to batch_prediction_job in aiplatform v1 batch_prediction_job.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **aiplatform:** Add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1 metadata_service.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1beta1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1beta1 metadata_service.proto feat: add InputArtifact to RuntimeConfig in aiplatform v1beta1 pipeline_job.proto feat: add read_mask to ListPipelineJobsRequest in aiplatform v1beta1 pipeline_service.proto feat: add TransferLearningConfig in aiplatform v1beta1 study.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))
* **aiplatform:** Add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1beta1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))
* **aiplatform:** Add WriteFeatureValues in aiplatform v1beta1 featurestore_online_service.proto ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))
* **aiplatform:** Making network arg optional in aiplatform v1 custom_job.proto feat: added SHARED_RESOURCES enum to aiplatform v1 model.proto docs: doc edits to aiplatform v1 dataset_service.proto, job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Making network arg optional in aiplatform v1beta1 custom_job.proto feat: DeploymentResourcePool and DeployementResourcePoolService added to aiplatform v1beta1 model.proto (cl/463147866) docs: doc edits to aiplatform v1beta1 job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Rewrite beta methods in terms of new stub location ([#6735](https://github.com/googleapis/google-cloud-go/issues/6735)) ([095cafd](https://github.com/googleapis/google-cloud-go/commit/095cafd432fc9e7d3f761e616fd20e732890d5e4))
* **aiplatform:** Rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))
* **aiplatform:** Start generating stubs dir ([5d0b405](https://github.com/googleapis/google-cloud-go/commit/5d0b405033f55023825ef90e5c539f1bcf2ddedb))
* **aiplatform:** Start generating stubs for beta ([#6723](https://github.com/googleapis/google-cloud-go/issues/6723)) ([71f5ab9](https://github.com/googleapis/google-cloud-go/commit/71f5ab946fd3e529fd65a66ea0bfe8f3bb5dc8e9))

## [1.32.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform-v1.31.0...aiplatform/v1.32.0) (2023-01-26)


### Features

* **aiplatform/apiv1beta1:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **aiplatform:** Add annotation_labels to ImportDataConfig in aiplatform v1 dataset.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **aiplatform:** Add deleteFeatureValues in aiplatform v1beta1 featurestore_service.proto ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))
* **aiplatform:** Add DeploymentResourcePool in aiplatform v1beta1 deployment_resource_pool.proto feat: add DeploymentResourcePoolService in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add SHARED_RESOURCES to DeploymentResourcesType in aiplatform v1beta1 model.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Add enable_dashboard_access in aiplatform v1 and v1beta1 custom_job.proto ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **aiplatform:** Add instance_config to batch_prediction_job in aiplatform v1beta1 batch_prediction_job.proto ([2b4957c](https://github.com/googleapis/google-cloud-go/commit/2b4957c7c348ecf5952e02f3602379fffaa758b4))
* **aiplatform:** Add instance_config to BatchPredictionJob in aiplatform v1 batch_prediction_job.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add metadata_artifact to Dataset in aiplatform v1 dataset.proto feat: add WriteFeatureValues rpc to FeaturestoreOnlineServingService in aiplatform v1 featurestore_online_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** Add metadata_artifact to Dataset in aiplatform v1beta1 dataset.proto feat: add offline_storage_ttl_days to EntityType in aiplatform v1beta1 entity_type.proto feat: add online_storage_ttl_days to Featurestore in aiplatform v1beta1 featurestore.proto feat: add source_uris to ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** Add model_monitoring_stats_anomalies,model_monitoring_status to BatchPredictionJob in aiplatform v1beta1 batch_prediction_job.proto ([e45ad9a](https://github.com/googleapis/google-cloud-go/commit/e45ad9af568c59151decc0dacedf137653b576dd))
* **aiplatform:** Add model_source_info to Model in aiplatform v1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add model_source_info to Model in aiplatform v1beta1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add NVIDIA_A100_80GB to AcceleratorType in aiplatform v1beta1 accelerator_type.proto feat: add annotation_labels to ImportDataConfig in aiplatform v1beta1 dataset.proto feat: add total_deployed_model_count and total_endpoint_count to QueryDeployedModelsResponse in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1beta1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1beta1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1beta1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1beta1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **aiplatform:** Add read_mask to ListPipelineJobsRequest in aiplatform v1 pipeline_service feat: add input_artifacts to PipelineJob.runtime_config in aiplatform v1 pipeline_job ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))
* **aiplatform:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1beta1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1beta1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1beta1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1beta1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add service_account to batch_prediction_job in aiplatform v1 batch_prediction_job.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **aiplatform:** Add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1 metadata_service.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1beta1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1beta1 metadata_service.proto feat: add InputArtifact to RuntimeConfig in aiplatform v1beta1 pipeline_job.proto feat: add read_mask to ListPipelineJobsRequest in aiplatform v1beta1 pipeline_service.proto feat: add TransferLearningConfig in aiplatform v1beta1 study.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** Add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))
* **aiplatform:** Add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1beta1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))
* **aiplatform:** Add WriteFeatureValues in aiplatform v1beta1 featurestore_online_service.proto ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))
* **aiplatform:** Making network arg optional in aiplatform v1 custom_job.proto feat: added SHARED_RESOURCES enum to aiplatform v1 model.proto docs: doc edits to aiplatform v1 dataset_service.proto, job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Making network arg optional in aiplatform v1beta1 custom_job.proto feat: DeploymentResourcePool and DeployementResourcePoolService added to aiplatform v1beta1 model.proto (cl/463147866) docs: doc edits to aiplatform v1beta1 job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** Rewrite beta methods in terms of new stub location ([#6735](https://github.com/googleapis/google-cloud-go/issues/6735)) ([095cafd](https://github.com/googleapis/google-cloud-go/commit/095cafd432fc9e7d3f761e616fd20e732890d5e4))
* **aiplatform:** Rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))
* **aiplatform:** Start generating stubs dir ([5d0b405](https://github.com/googleapis/google-cloud-go/commit/5d0b405033f55023825ef90e5c539f1bcf2ddedb))
* **aiplatform:** Start generating stubs for beta ([#6723](https://github.com/googleapis/google-cloud-go/issues/6723)) ([71f5ab9](https://github.com/googleapis/google-cloud-go/commit/71f5ab946fd3e529fd65a66ea0bfe8f3bb5dc8e9))

## [1.31.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.30.0...aiplatform/v1.31.0) (2023-01-26)


### Features

* **aiplatform:** Add enable_dashboard_access in aiplatform v1 and v1beta1 custom_job.proto ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [1.30.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.29.0...aiplatform/v1.30.0) (2023-01-18)


### Features

* **aiplatform:** Add instance_config to BatchPredictionJob in aiplatform v1 batch_prediction_job.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))
* **aiplatform:** Add saved_queries to Dataset in aiplatform v1beta1 dataset.proto feat: add order_by to ListModelVersionRequest in aiplatform v1beta1 model_service.proto feat: add update_all_stopped_trials to ConvexAutomatedStoppingSpec in aiplatform v1beta1 study.proto feat: add ReadTensorboardUsage rpc in aiplatform v1beta1 tensorboard_service.proto ([ee41485](https://github.com/googleapis/google-cloud-go/commit/ee41485860bcbbd09ce4e28ee6ddca81a5f17211))

## [1.29.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.28.0...aiplatform/v1.29.0) (2023-01-04)


### Features

* **aiplatform:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.28.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.27.0...aiplatform/v1.28.0) (2022-12-05)


### Features

* **aiplatform:** rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))

## [1.27.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.26.0...aiplatform/v1.27.0) (2022-12-01)


### Features

* **aiplatform:** add metadata_artifact to Dataset in aiplatform v1 dataset.proto feat: add WriteFeatureValues rpc to FeaturestoreOnlineServingService in aiplatform v1 featurestore_online_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** add metadata_artifact to Dataset in aiplatform v1beta1 dataset.proto feat: add offline_storage_ttl_days to EntityType in aiplatform v1beta1 entity_type.proto feat: add online_storage_ttl_days to Featurestore in aiplatform v1beta1 featurestore.proto feat: add source_uris to ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **aiplatform:** start generating stubs dir ([5d0b405](https://github.com/googleapis/google-cloud-go/commit/5d0b405033f55023825ef90e5c539f1bcf2ddedb))

## [1.26.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.25.0...aiplatform/v1.26.0) (2022-11-16)


### Features

* **aiplatform:** add instance_config to batch_prediction_job in aiplatform v1beta1 batch_prediction_job.proto ([2b4957c](https://github.com/googleapis/google-cloud-go/commit/2b4957c7c348ecf5952e02f3602379fffaa758b4))
* **aiplatform:** add service_account to batch_prediction_job in aiplatform v1 batch_prediction_job.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))

## [1.25.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.24.0...aiplatform/v1.25.0) (2022-11-09)


### Features

* **aiplatform:** add annotation_labels to ImportDataConfig in aiplatform v1 dataset.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **aiplatform:** add NVIDIA_A100_80GB to AcceleratorType in aiplatform v1beta1 accelerator_type.proto feat: add annotation_labels to ImportDataConfig in aiplatform v1beta1 dataset.proto feat: add total_deployed_model_count and total_endpoint_count to QueryDeployedModelsResponse in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add start_time to BatchReadFeatureValuesRequest in aiplatform v1beta1 featurestore_service.proto feat: add metadata_artifact to Model in aiplatform v1beta1 model.proto feat: add failed_main_jobs and failed_pre_caching_check_jobs to ContainerDetail in aiplatform v1beta1 pipeline_job.proto feat: add persist_ml_use_assignment to InputDataConfig in aiplatform v1beta1 training_pipeline.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.23.0...aiplatform/v1.24.0) (2022-09-28)


### Features

* **aiplatform:** add model_source_info to Model in aiplatform v1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** add model_source_info to Model in aiplatform v1beta1 model.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1 metadata_service.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))
* **aiplatform:** add timestamp_outside_retention_rows_count to ImportFeatureValuesResponse and ImportFeatureValuesOperationMetadata in aiplatform v1beta1 featurestore_service.proto feat: add RemoveContextChildren rpc to aiplatform v1beta1 metadata_service.proto feat: add order_by to ListArtifactsRequest, ListContextsRequest, and ListExecutionsRequest in aiplatform v1beta1 metadata_service.proto feat: add InputArtifact to RuntimeConfig in aiplatform v1beta1 pipeline_job.proto feat: add read_mask to ListPipelineJobsRequest in aiplatform v1beta1 pipeline_service.proto feat: add TransferLearningConfig in aiplatform v1beta1 study.proto ([52dddd1](https://github.com/googleapis/google-cloud-go/commit/52dddd1ed89fbe77e1859311c3b993a77a82bfc7))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.22.0...aiplatform/v1.23.0) (2022-09-26)


### Features

* **aiplatform:** Rewrite beta methods in terms of new stub location ([#6735](https://github.com/googleapis/google-cloud-go/issues/6735)) ([095cafd](https://github.com/googleapis/google-cloud-go/commit/095cafd432fc9e7d3f761e616fd20e732890d5e4))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.21.0...aiplatform/v1.22.0) (2022-09-22)


### Features

* **aiplatform:** Start generating stubs for beta ([#6723](https://github.com/googleapis/google-cloud-go/issues/6723)) ([71f5ab9](https://github.com/googleapis/google-cloud-go/commit/71f5ab946fd3e529fd65a66ea0bfe8f3bb5dc8e9))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.20.0...aiplatform/v1.21.0) (2022-09-19)


### Features

* **aiplatform:** add deleteFeatureValues in aiplatform v1beta1 featurestore_service.proto ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.19.0...aiplatform/v1.20.0) (2022-09-15)


### Features

* **aiplatform/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.18.0...aiplatform/v1.19.0) (2022-09-08)


### Features

* **aiplatform:** add model_monitoring_stats_anomalies,model_monitoring_status to BatchPredictionJob in aiplatform v1beta1 batch_prediction_job.proto ([e45ad9a](https://github.com/googleapis/google-cloud-go/commit/e45ad9af568c59151decc0dacedf137653b576dd))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.17.0...aiplatform/v1.18.0) (2022-09-06)


### Features

* **aiplatform:** add read_mask to ListPipelineJobsRequest in aiplatform v1 pipeline_service feat: add input_artifacts to PipelineJob.runtime_config in aiplatform v1 pipeline_job ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))
* **aiplatform:** add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))
* **aiplatform:** add UpsertDatapoints and RemoveDatapoints rpcs to IndexService in aiplatform v1beta1 index_service.proto ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.16.0...aiplatform/v1.17.0) (2022-08-18)


### Features

* **aiplatform:** add WriteFeatureValues in aiplatform v1beta1 featurestore_online_service.proto ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.15.0...aiplatform/v1.16.0) (2022-08-02)


### Features

* **aiplatform:** add DeploymentResourcePool in aiplatform v1beta1 deployment_resource_pool.proto feat: add DeploymentResourcePoolService in aiplatform v1beta1 deployment_resource_pool_service.proto feat: add SHARED_RESOURCES to DeploymentResourcesType in aiplatform v1beta1 model.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** making network arg optional in aiplatform v1 custom_job.proto feat: added SHARED_RESOURCES enum to aiplatform v1 model.proto docs: doc edits to aiplatform v1 dataset_service.proto, job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **aiplatform:** making network arg optional in aiplatform v1beta1 custom_job.proto feat: DeploymentResourcePool and DeployementResourcePoolService added to aiplatform v1beta1 model.proto (cl/463147866) docs: doc edits to aiplatform v1beta1 job_service.proto, model_service.proto, pipeline_service.proto, saved_query.proto, study.proto, types.proto ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.14.0...aiplatform/v1.15.0) (2022-07-26)


### Features

* **aiplatform:** add a DeploymentResourcePool API resource_definition feat: add shared_resources for supported prediction_resources ([8a8ba85](https://github.com/googleapis/google-cloud-go/commit/8a8ba85311f85701c97fd7c10f1d88b738ce423f))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.13.0...aiplatform/v1.14.0) (2022-06-29)


### Features

* **aiplatform:** add BatchImportModelEvaluationSlices API in aiplatform v1 model_service.proto ([f01bf32](https://github.com/googleapis/google-cloud-go/commit/f01bf32d7f4aa2c59db6bfdcc574ce2470bc61bb))
* **aiplatform:** add BatchImportModelEvaluationSlices API in aiplatform v1beta1 model_service.proto ([f01bf32](https://github.com/googleapis/google-cloud-go/commit/f01bf32d7f4aa2c59db6bfdcc574ce2470bc61bb))
* **aiplatform:** add ListSavedQueries rpc to aiplatform v1 dataset_service.proto feat: add saved_query.proto to aiplatform v1 feat: add saved_query_id to InputDataConfig in aiplatform v1 training_pipeline.proto ([350e276](https://github.com/googleapis/google-cloud-go/commit/350e276a5b17483e7347a82f2e195f6619782bec))
* **aiplatform:** add ListSavedQueries rpc to aiplatform v1beta1 dataset_service.proto feat: add saved_query.proto to aiplatform v1beta1 feat: add saved_query_id to InputDataConfig in aiplatform v1beta1 training_pipeline.proto ([350e276](https://github.com/googleapis/google-cloud-go/commit/350e276a5b17483e7347a82f2e195f6619782bec))
* **aiplatform:** add model_monitoring_config to BatchPredictionJob in aiplatform v1beta1 batch_prediction_job.proto ([5fe3b1d](https://github.com/googleapis/google-cloud-go/commit/5fe3b1d946db991aebdfd279f6f3b06b8baec205))
* **aiplatform:** add model_version_id to BatchPredictionJob in aiplatform v1 batch_prediction_job.proto ([f01bf32](https://github.com/googleapis/google-cloud-go/commit/f01bf32d7f4aa2c59db6bfdcc574ce2470bc61bb))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.12.0...aiplatform/v1.13.0) (2022-06-17)


### Features

* **aiplatform:** add model_version_id to UploadModelResponse in aiplatform v1 model_service.proto ([c84e111](https://github.com/googleapis/google-cloud-go/commit/c84e111db5d3f57f4e8fbb5dfff0219d052435a0))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.11.0...aiplatform/v1.12.0) (2022-06-16)


### Features

* **aiplatform:** add default_skew_threshold to TrainingPredictionSkewDetectionConfig in aiplatform v1beta1, v1 model_monitoring.proto ([5e46068](https://github.com/googleapis/google-cloud-go/commit/5e46068329153daf5aa590a6415d4764f1ab2b90))
* **aiplatform:** add env to ContainerSpec in aiplatform v1beta1 custom_job.proto ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))
* **aiplatform:** add monitor_window to ModelDeploymentMonitoringScheduleConfig proto in aiplatform v1/v1beta1 model_deployment_monitoring_job.proto ([4134941](https://github.com/googleapis/google-cloud-go/commit/41349411e601f57dc6d9e246f1748fd86d17bb15))
* **aiplatform:** add successful_forecast_point_count to CompletionStats in aiplatform v1 completion_stats.proto feat: add neighbors to Explanation in aiplatform v1 explanation.proto feat: add examples_override to ExplanationSpecOverride in aiplatform v1 explanation.proto feat: add version_id, version_aliases, version_create_time, version_update_time, and version_description to aiplatform v1 model.proto feat: add ModelVersion CRUD methods in aiplatform v1 model_service.proto feat: add model_id and parent_model to TrainingPipeline in aiplatform v1 training_pipeline.proto ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))
* **aiplatform:** Include the location and iam_policy mixin clients ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.10.0...aiplatform/v1.11.0) (2022-06-01)


### Features

* **aiplatform:** add failure_policy to PipelineJob in aiplatform v1 & v1beta1 pipeline_job.proto ([46c16f1](https://github.com/googleapis/google-cloud-go/commit/46c16f1fdc7181d2fefadc8fd6a9e0b9cb226cac))
* **aiplatform:** add IAM policy to aiplatform_v1beta1.yaml feat: add preset configuration for example-based explanations in aiplatform v1beta1 explanation.proto feat: add latent_space_source to ExplanationMetadata in aiplatform v1beta1 explanation_metadata.proto feat: add successful_forecast_point_count to CompletionStats in completion_stats.proto ([46c16f1](https://github.com/googleapis/google-cloud-go/commit/46c16f1fdc7181d2fefadc8fd6a9e0b9cb226cac))
* **aiplatform:** add latent_space_source to ExplanationMetadata in aiplatform v1 explanation_metadata.proto feat: add scaling to OnlineServingConfig in aiplatform v1 featurestore.proto feat: add template_metadata to PipelineJob in aiplatform v1 pipeline_job.proto ([46c16f1](https://github.com/googleapis/google-cloud-go/commit/46c16f1fdc7181d2fefadc8fd6a9e0b9cb226cac))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.9.0...aiplatform/v1.10.0) (2022-05-24)


### Features

* **aiplatform:** add display_name and metadata to ModelEvaluation in aiplatform model_evaluation.proto ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))
* **aiplatform:** add Examples to Explanation related messages in aiplatform v1beta1 explanation.proto ([da99e5f](https://github.com/googleapis/google-cloud-go/commit/da99e5f7905367388d967aab12b4949bb4b250ff))
* **aiplatform:** add template_metadata to PipelineJob in aiplatform v1beta1 pipeline_job.proto ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.8.0...aiplatform/v1.9.0) (2022-04-20)


### ⚠ BREAKING CHANGES

* **aiplatform:** (php) remove several `REQUIRED` field annotations in featurestore.proto, metadata.proto, and pipeline_job.proto
* **aiplatform:** (php) remove several `REQUIRED` field annotations in featurestore.proto, metadata.proto, and pipeline_job.proto

### Features

* **aiplatform:** add reserved_ip_ranges to CustomJobSpec in aiplatform v1 custom_job.proto feat: add nfs_mounts to WorkPoolSpec in aiplatform v1 custom_job.proto feat: add JOB_STATE_UPDATING to JobState in aiplatform v1 job_state.proto feat: add MfsMount in aiplatform v1 machine_resources.proto feat: add ConvexAutomatedStoppingSpec to StudySpec in aiplatform v1 study.proto ([e71a99d](https://github.com/googleapis/google-cloud-go/commit/e71a99d3edc21c937aa9d7bfd61288b0073a5275))
* **aiplatform:** rename Similarity to Examples, and similarity to examples in ExplanationParameters in aiplatform v1beta1 explanation.proto feat: add reserved_ip_ranges to CustomJobSpec in aiplatform v1beta1 custom_job.proto feat: add nfs_mounts to WorkPoolSpec in aiplatform v1beta1 custom_job.proto feat: add PredictRequestResponseLoggingConfig to aiplatform v1beta1 endpoint.proto feat: add model_version_id to DeployedModel in aiplatform v1beta1 endpoint.proto feat: add JOB_STATE_UPDATING to JobState in aiplatform v1beta1 job_state.proto feat: add MfsMount in aiplatform v1beta1 machine_resources.proto feat: add version_id to Model in aiplatform v1beta1 model.proto feat: add LatestMonitoringPipelineMetadata to ModelDeploymentMonitoringJob in aiplatform v1beta1 model_deployment_monitoring_job.proto feat: add ListModelVersion, DeleteModelVersion, and MergeVersionAliases rpcs to aiplatform v1beta1 model_service.proto feat: add model_version_id to UploadModelRequest and UploadModelResponse in aiplatform v1beta1 model_service.proto feat: add model_version_id to PredictResponse in aiplatform v1beta1 prediction_service.proto feat: add ConvexAutomatedStoppingSpec to StudySpec in aiplatform v1beta1 study.proto feat: add model_id and parent_model to TrainingPipeline in aiplatform v1beta1 training_pipeline.proto ([e71a99d](https://github.com/googleapis/google-cloud-go/commit/e71a99d3edc21c937aa9d7bfd61288b0073a5275))


### Miscellaneous Chores

* **aiplatform:** release 1.9.0 ([#5921](https://github.com/googleapis/google-cloud-go/issues/5921)) ([a1a59ce](https://github.com/googleapis/google-cloud-go/commit/a1a59ce55a289f88a46508dfccf52ce5517a9c8b))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.7.0...aiplatform/v1.8.0) (2022-04-06)


### Features

* **aiplatform:** add ImportModelEvaluation in aiplatform v1 model_service.proto feat: add data_item_schema_uri, annotation_schema_uri, explanation_specs to ModelEvaluationExplanationSpec in aiplatform v1 model_evaluation.proto ([21a3cce](https://github.com/googleapis/google-cloud-go/commit/21a3cced42fe30abd4457b377ec78640e80babc8))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.6.0...aiplatform/v1.7.0) (2022-03-28)


### Features

* **aiplatform:** add `service_account` to `BatchPredictionJob` in aiplatform `v1beta1` `batch_prediction_job.proto` ([b01c037](https://github.com/googleapis/google-cloud-go/commit/b01c03783d84cb7a3eba4f69d49d3fb7be1b6353))
* **aiplatform:** add monitoring_config to EntityType in aiplatform v1 entity_type.proto feat: add disable_monitoring to Feature in aiplatform v1 feature.proto feat: add monitoring_stats_anomalies to Feature in aiplatform v1 feature.proto feat: add staleness_days to SnapshotAnalysis in aiplatform v1 featurestore_monitoring.proto feat: add import_features_analysis to FeaturestoreMonitoringConfig in aiplatform v1 featurestore_monitoring.proto feat: add numerical_threshold_config to FeaturestoreMonitoringConfig in aiplatform v1 featurestore_monitoring.proto feat: add categorical_threshold_config to FeaturestoreMonitoringConfig in aiplatform v1 featurestore_monitoring.proto feat: add objective to MonitoringStatsSpec in aiplatform v1 featurestore_service.proto ([c19b7a2](https://github.com/googleapis/google-cloud-go/commit/c19b7a2e49c032dddd7b3de7bad671f481d5f16c))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.5.0...aiplatform/v1.6.0) (2022-03-14)


### Features

* **aiplatform:** start generating apiv1beta1 ([#5738](https://github.com/googleapis/google-cloud-go/issues/5738)) ([a213bff](https://github.com/googleapis/google-cloud-go/commit/a213bff65e4e47912f94ab5cb1426dbb142fa493)), refs [#5737](https://github.com/googleapis/google-cloud-go/issues/5737)

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.4.0...aiplatform/v1.5.0) (2022-02-23)


### Features

* **aiplatform:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.3.0...aiplatform/v1.4.0) (2022-02-14)


### Features

* **aiplatform:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.3.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.2.0...aiplatform/v1.3.0) (2022-02-03)


### Features

* **aiplatform:** add dedicated_resources to DeployedIndex message in aiplatform v1 index_endpoint.proto chore: sort imports ([6e56077](https://www.github.com/googleapis/google-cloud-go/commit/6e560776fd6e574320ce2dbad1f9eb9e22999185))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.1.1...aiplatform/v1.2.0) (2022-01-04)


### Features

* **aiplatform:** add enable_private_service_connect field to Endpoint feat: add id field to DeployedModel feat: add service_attachment field to PrivateEndpoints feat: add endpoint_id to CreateEndpointRequest and method signature to CreateEndpoint feat: add method signature to CreateFeatureStore, CreateEntityType, CreateFeature feat: add network and enable_private_service_connect to IndexEndpoint feat: add service_attachment to IndexPrivateEndpoints feat: add stratified_split field to training_pipeline InputDataConfig ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))
* **aiplatform:** Adds support for `google.protobuf.Value` pipeline parameters in the `parameter_values` field ([88a1cdb](https://www.github.com/googleapis/google-cloud-go/commit/88a1cdbef3cc337354a61bc9276725bfb9a686d8))
* **aiplatform:** Tensorboard v1 protos release feat:Exposing a field for v1 CustomJob-Tensorboard integration. ([90e2868](https://www.github.com/googleapis/google-cloud-go/commit/90e2868a3d220aa7f897438f4917013fda7a7c59))

### [1.1.1](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.1.0...aiplatform/v1.1.1) (2021-11-02)


### Bug Fixes

* **aiplatform:** Remove invalid resource annotations ([587bba5](https://www.github.com/googleapis/google-cloud-go/commit/587bba5ad792a92f252107aa38c6af50fb09fb58))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.0.0...aiplatform/v1.1.0) (2021-10-18)


### Features

* **aiplatform:** add featurestore service to aiplatform v1 feat: add metadata service to aiplatform v1 ([30794e7](https://www.github.com/googleapis/google-cloud-go/commit/30794e70050b55ff87d6a80d0b4075065e9d271d))
* **aiplatform:** add vizier service to aiplatform v1 BUILD.bazel ([12928d4](https://www.github.com/googleapis/google-cloud-go/commit/12928d47de771f4b23577062afe5cf551b347919))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v0.1.0...aiplatform/v0.2.0) (2021-09-09)


### Features

* **aiplatform:** add Vizier service to aiplatform v1 ([33e4d89](https://www.github.com/googleapis/google-cloud-go/commit/33e4d895373dc8ec1dad13645ee5f342b2b15282))
* **aiplatform:** add XAI, model monitoring, and index services to aiplatform v1 ([e385b40](https://www.github.com/googleapis/google-cloud-go/commit/e385b40a1e2ecf81f5fd0910de5c37275951f86b))

## v0.1.0

This is the first tag to carve out aiplatform as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
