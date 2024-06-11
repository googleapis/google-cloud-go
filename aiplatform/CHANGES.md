# Changes


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
