# Changelog

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/memorystore/v0.2.2...memorystore/v0.3.0) (2025-05-06)


### Features

* **memorystore:** A new field `async_instance_endpoints_deletion_enabled` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `automated_backup_config` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `backup_collection` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `cross_instance_replication_config` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `gcs_source` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `maintenance_policy` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `maintenance_schedule` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `managed_backup_source` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `ondemand_maintenance` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `port` is added to message `.google.cloud.memorystore.v1.PscConnection` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `psc_attachment_details` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `target_engine_version` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new field `target_node_type` is added to message `.google.cloud.memorystore.v1.Instance` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `AutomatedBackupConfig` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `Backup` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `BackupCollection` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `BackupFile` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `BackupInstanceRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `CrossInstanceReplicationConfig` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `DeleteBackupRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ExportBackupRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `GcsBackupSource` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `GetBackupCollectionRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `GetBackupRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ListBackupCollectionsRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ListBackupCollectionsResponse` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ListBackupsRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ListBackupsResponse` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `MaintenancePolicy` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `MaintenanceSchedule` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `ManagedBackupSource` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `PscAttachmentDetail` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `RescheduleMaintenanceRequest` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new message `WeeklyMaintenanceWindow` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `BackupInstance` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `DeleteBackup` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `ExportBackup` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `GetBackup` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `GetBackupCollection` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `ListBackupCollections` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `ListBackups` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new method `RescheduleMaintenance` is added to service `Memorystore` ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new resource_definition `cloudkms.googleapis.com/CryptoKey` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new resource_definition `memorystore.googleapis.com/Backup` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A new resource_definition `memorystore.googleapis.com/BackupCollection` is added ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))


### Bug Fixes

* **memorystore:** Changed field behavior for an existing field `psc_connection_id` in message `.google.cloud.memorystore.v1.PscConnection` ([#12095](https://github.com/googleapis/google-cloud-go/issues/12095)) ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))


### Documentation

* **memorystore:** A comment for field `discovery_endpoints` in message `.google.cloud.memorystore.v1.Instance` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `engine_version` in message `.google.cloud.memorystore.v1.Instance` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `node_type` in message `.google.cloud.memorystore.v1.Instance` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `port` in message `.google.cloud.memorystore.v1.PscAutoConnection` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `psc_auto_connection` in message `.google.cloud.memorystore.v1.Instance` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `psc_auto_connections` in message `.google.cloud.memorystore.v1.Instance` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))
* **memorystore:** A comment for field `psc_connection_id` in message `.google.cloud.memorystore.v1.PscConnection` is changed ([2f22244](https://github.com/googleapis/google-cloud-go/commit/2f2224464c132fbcf84e82cc4c3fabb21f07e858))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/memorystore/v0.2.1...memorystore/v0.2.2) (2025-04-15)


### Bug Fixes

* **memorystore:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/memorystore/v0.2.0...memorystore/v0.2.1) (2025-03-13)


### Bug Fixes

* **memorystore:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/memorystore/v0.1.1...memorystore/v0.2.0) (2025-02-12)


### Features

* **memorystore:** Add Instance.Mode.CLUSTER_DISABLED value, and deprecate STANDALONE ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **memorystore:** Add Instance.Mode.CLUSTER_DISABLED value, and deprecate STANDALONE ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))


### Documentation

* **memorystore:** A comment for enum value `STANDALONE` in enum `Mode` is changed ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))
* **memorystore:** A comment for enum value `STANDALONE` in enum `Mode` is changed ([90140b1](https://github.com/googleapis/google-cloud-go/commit/90140b17da6378fa87d4bec0d404c18a78d6b02a))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/memorystore/v0.1.0...memorystore/v0.1.1) (2025-01-02)


### Bug Fixes

* **memorystore:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## 0.1.0 (2024-12-18)


### Features

* **memorystore:** New clients ([#11310](https://github.com/googleapis/google-cloud-go/issues/11310)) ([1946e3d](https://github.com/googleapis/google-cloud-go/commit/1946e3de6c3afb7ed51ac641bddcbe027916df46))

## Changes
