# Changes


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
