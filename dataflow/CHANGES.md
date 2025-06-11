# Changes

## [0.11.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.6...dataflow/v0.11.0) (2025-05-06)


### Features

* **dataflow:** A new enum `StreamingMode` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `bugs` is added to message `.google.dataflow.v1beta3.SdkVersion` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `data_sampling` is added to message `.google.dataflow.v1beta3.DebugOptions` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `default_streaming_mode` is added to message `.google.dataflow.v1beta3.TemplateMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `default_value` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `disk_size_gb` is added to message `.google.dataflow.v1beta3.RuntimeEnvironment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `dynamic_destinations` is added to message `.google.dataflow.v1beta3.PubsubLocation` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `enable_launcher_vm_serial_port_logging` is added to message `.google.dataflow.v1beta3.FlexTemplateRuntimeEnvironment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `enum_options` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `group_name` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `hidden_ui` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `image_repository_cert_path` is added to message `.google.dataflow.v1beta3.ContainerSpec` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `image_repository_password_secret_id` is added to message `.google.dataflow.v1beta3.ContainerSpec` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `image_repository_username_secret_id` is added to message `.google.dataflow.v1beta3.ContainerSpec` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `name` is added to message `.google.dataflow.v1beta3.ListJobsRequest` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `parent_name` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `parent_trigger_values` is added to message `.google.dataflow.v1beta3.ParameterMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `runtime_updatable_params` is added to message `.google.dataflow.v1beta3.Job` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `satisfies_pzi` is added to message `.google.dataflow.v1beta3.Job` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `service_resources` is added to message `.google.dataflow.v1beta3.Job` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `step_names_hash` is added to message `.google.dataflow.v1beta3.PipelineDescription` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `straggler_info` is added to message `.google.dataflow.v1beta3.WorkItemDetails` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `straggler_summary` is added to message `.google.dataflow.v1beta3.StageSummary` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `streaming_mode` is added to message `.google.dataflow.v1beta3.Environment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `streaming_mode` is added to message `.google.dataflow.v1beta3.FlexTemplateRuntimeEnvironment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `streaming_mode` is added to message `.google.dataflow.v1beta3.RuntimeEnvironment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `streaming` is added to message `.google.dataflow.v1beta3.TemplateMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `supports_at_least_once` is added to message `.google.dataflow.v1beta3.TemplateMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `supports_exactly_once` is added to message `.google.dataflow.v1beta3.TemplateMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `trie` is added to message `.google.dataflow.v1beta3.MetricUpdate` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `update_mask` is added to message `.google.dataflow.v1beta3.UpdateJobRequest` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `use_streaming_engine_resource_based_billing` is added to message `.google.dataflow.v1beta3.Environment` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new field `user_display_properties` is added to message `.google.dataflow.v1beta3.JobMetadata` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `DataSamplingConfig` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `HotKeyDebuggingInfo` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `ParameterMetadataEnumOption` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `RuntimeUpdatableParams` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `SdkBug` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `ServiceResources` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `Straggler` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `StragglerInfo` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `StragglerSummary` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new message `StreamingStragglerInfo` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new method_signature `job,update_mask` is added to method `UpdateJob` in service `JobsV1Beta3` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `BIGQUERY_TABLE` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `BOOLEAN` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `ENUM` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `GO` is added to enum `Language` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `JAVASCRIPT_UDF_FILE` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `KAFKA_READ_TOPIC` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `KAFKA_TOPIC` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `KAFKA_WRITE_TOPIC` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `KMS_KEY_NAME` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `MACHINE_TYPE` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `NUMBER` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `SERVICE_ACCOUNT` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `WORKER_REGION` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A new value `WORKER_ZONE` is added to enum `ParameterType` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))


### Bug Fixes

* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** An existing oauth_scope `https ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))


### Documentation

* **dataflow:** A comment for enum `JobState` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for enum `WorkerIPAddressConfiguration` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for enum value `JOB_VIEW_ALL` in enum `JobView` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `additional_experiments` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `additional_user_labels` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `bypass_temp_dir_validation` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `capabilities` in message `.google.dataflow.v1beta3.SdkHarnessContainerImage` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `current_state` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `dataset` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `debug_options` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `dump_heap_on_oom` in message `.google.dataflow.v1beta3.FlexTemplateRuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `dynamic_template` in message `.google.dataflow.v1beta3.LaunchTemplateRequest` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `enable_hot_key_logging` in message `.google.dataflow.v1beta3.DebugOptions` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `enable_streaming_engine` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `environment` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `flex_resource_scheduling_goal` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `gcs_path` in message `.google.dataflow.v1beta3.DynamicTemplateLaunchParams` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `gcs_path` in message `.google.dataflow.v1beta3.LaunchTemplateRequest` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `id` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `ip_configuration` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `job_name` in message `.google.dataflow.v1beta3.LaunchTemplateParameters` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `kms_key_name` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `launch_parameters` in message `.google.dataflow.v1beta3.LaunchTemplateRequest` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `location` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `machine_type` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `max_workers` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `name` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `network` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `num_workers` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `project_id` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `requested_state` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `save_heap_dumps_to_gcs_path` in message `.google.dataflow.v1beta3.FlexTemplateRuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `service_account_email` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `service_account_email` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `service_kms_key_name` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `service_options` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `set` in message `.google.dataflow.v1beta3.MetricUpdate` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `subnetwork` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `temp_location` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `transform_name_mapping` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `type` in message `.google.dataflow.v1beta3.Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `worker_region` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `worker_region` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `worker_zone` in message `.google.dataflow.v1beta3.Environment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `worker_zone` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for field `zone` in message `.google.dataflow.v1beta3.RuntimeEnvironment` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `DynamicTemplateLaunchParams` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `Job` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `JobExecutionStageInfo` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `JobMetrics` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `LaunchTemplateParameters` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `MetricUpdate` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `SdkHarnessContainerImage` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for message `Step` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `AggregatedListJobs` in service `JobsV1Beta3` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `CreateJob` in service `JobsV1Beta3` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `CreateJobFromTemplate` in service `TemplatesService` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `GetTemplate` in service `TemplatesService` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `LaunchTemplate` in service `TemplatesService` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for method `ListJobs` in service `JobsV1Beta3` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **dataflow:** A comment for service `FlexTemplatesService` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))

## [0.10.6](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.5...dataflow/v0.10.6) (2025-04-15)


### Bug Fixes

* **dataflow:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.10.5](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.4...dataflow/v0.10.5) (2025-03-13)


### Bug Fixes

* **dataflow:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.10.4](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.3...dataflow/v0.10.4) (2025-03-06)


### Bug Fixes

* **dataflow:** Fix out-of-sync version.go ([28f0030](https://github.com/googleapis/google-cloud-go/commit/28f00304ebb13abfd0da2f45b9b79de093cca1ec))

## [0.10.3](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.2...dataflow/v0.10.3) (2025-01-02)


### Bug Fixes

* **dataflow:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.10.2](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.1...dataflow/v0.10.2) (2024-10-23)


### Bug Fixes

* **dataflow:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **dataflow:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.10.1](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.10.0...dataflow/v0.10.1) (2024-09-12)


### Bug Fixes

* **dataflow:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.10.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.12...dataflow/v0.10.0) (2024-08-20)


### Features

* **dataflow:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.9.12](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.11...dataflow/v0.9.12) (2024-08-08)


### Bug Fixes

* **dataflow:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.9.11](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.10...dataflow/v0.9.11) (2024-07-24)


### Bug Fixes

* **dataflow:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.9.10](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.9...dataflow/v0.9.10) (2024-07-10)


### Bug Fixes

* **dataflow:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.9.9](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.8...dataflow/v0.9.9) (2024-07-01)


### Bug Fixes

* **dataflow:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.9.8](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.7...dataflow/v0.9.8) (2024-06-26)


### Bug Fixes

* **dataflow:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.9.7](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.6...dataflow/v0.9.7) (2024-05-01)


### Bug Fixes

* **dataflow:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.9.6](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.5...dataflow/v0.9.6) (2024-03-14)


### Bug Fixes

* **dataflow:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.9.5](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.4...dataflow/v0.9.5) (2024-01-30)


### Bug Fixes

* **dataflow:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.9.4](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.3...dataflow/v0.9.4) (2023-11-01)


### Bug Fixes

* **dataflow:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.9.3](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.2...dataflow/v0.9.3) (2023-10-26)


### Bug Fixes

* **dataflow:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.9.2](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.1...dataflow/v0.9.2) (2023-10-12)


### Bug Fixes

* **dataflow:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [0.9.1](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.9.0...dataflow/v0.9.1) (2023-06-20)


### Bug Fixes

* **dataflow:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.8.1...dataflow/v0.9.0) (2023-05-30)


### Features

* **dataflow:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [0.8.1](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.8.0...dataflow/v0.8.1) (2023-05-08)


### Bug Fixes

* **dataflow:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.7.0...dataflow/v0.8.0) (2023-01-04)


### Features

* **dataflow:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.6.0...dataflow/v0.7.0) (2022-09-21)


### Features

* **dataflow:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.5.1...dataflow/v0.6.0) (2022-09-19)


### Features

* **dataflow:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [0.5.1](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.5.0...dataflow/v0.5.1) (2022-07-12)


### Documentation

* **dataflow:** corrected the Dataflow job name regex ([1732e43](https://github.com/googleapis/google-cloud-go/commit/1732e4334c84019d93775d861be5c0008e3f5245))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.4.0...dataflow/v0.5.0) (2022-06-29)


### Features

* **dataflow:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.3.0...dataflow/v0.4.0) (2022-03-28)


### Features

* **dataflow:** Add the ability to plumb environment capabilities through v1beta3 protos. ([b01c037](https://github.com/googleapis/google-cloud-go/commit/b01c03783d84cb7a3eba4f69d49d3fb7be1b6353))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.2.0...dataflow/v0.3.0) (2022-02-23)


### Features

* **dataflow:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/dataflow/v0.1.0...dataflow/v0.2.0) (2022-02-14)


### Features

* **dataflow:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## v0.1.0

This is the first tag to carve out dataflow as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
