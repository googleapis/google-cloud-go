# Changelog


## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.4.0...netapp/v1.4.1) (2024-10-23)


### Bug Fixes

* **netapp:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **netapp:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.3.1...netapp/v1.4.0) (2024-09-19)


### Features

* **netapp:** A new field 'allow_auto_tiering' in message 'google.cloud.netapp.v1.StoragePool' is added ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))
* **netapp:** A new field 'cold_tier_size_gib' in message 'google.cloud.netapp.v1.Volume' is added ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))
* **netapp:** A new message 'google.cloud.netapp.v1.SwitchActiveReplicaZoneRequest' is added ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))
* **netapp:** A new rpc 'SwitchActiveReplicaZone' is added to service 'google.cloud.netapp.v1.NetApp' ([37866ce](https://github.com/googleapis/google-cloud-go/commit/37866ce67a286a3eed1b92f53bdac2ae8f1c63ed))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.3.0...netapp/v1.3.1) (2024-09-12)


### Bug Fixes

* **netapp:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.2.1...netapp/v1.3.0) (2024-08-20)


### Features

* **netapp:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.2.0...netapp/v1.2.1) (2024-08-08)


### Bug Fixes

* **netapp:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.1.4...netapp/v1.2.0) (2024-08-01)


### Features

* **netapp:** A new field `administrators` is added to message `.google.cloud.netapp.v1.ActiveDirectory` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `large_capacity` is added to message `.google.cloud.netapp.v1.Volume` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `multiple_endpoints` is added to message `.google.cloud.netapp.v1.Volume` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `replica_zone` is added to message `.google.cloud.netapp.v1.StoragePool` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `replica_zone` is added to message `.google.cloud.netapp.v1.Volume` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `zone` is added to message `.google.cloud.netapp.v1.StoragePool` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A new field `zone` is added to message `.google.cloud.netapp.v1.Volume` ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))


### Documentation

* **netapp:** A comment for enum value `TRANSFERRING` in enum `MirrorState` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `active_directory_id` in message `.google.cloud.netapp.v1.CreateActiveDirectoryRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `backup_id` in message `.google.cloud.netapp.v1.CreateBackupRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `backup_policy_id` in message `.google.cloud.netapp.v1.CreateBackupPolicyRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `backup_vault_id` in message `.google.cloud.netapp.v1.CreateBackupVaultRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `kms_config_id` in message `.google.cloud.netapp.v1.CreateKmsConfigRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `replication_id` in message `.google.cloud.netapp.v1.CreateReplicationRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `snapshot_id` in message `.google.cloud.netapp.v1.CreateSnapshotRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `storage_pool_id` in message `.google.cloud.netapp.v1.CreateStoragePoolRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `total_transfer_duration` in message `.google.cloud.netapp.v1.TransferStats` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `transfer_bytes` in message `.google.cloud.netapp.v1.TransferStats` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))
* **netapp:** A comment for field `volume_id` in message `.google.cloud.netapp.v1.CreateVolumeRequest` is changed ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))

## [1.1.4](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.1.3...netapp/v1.1.4) (2024-07-24)


### Bug Fixes

* **netapp:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.1.3](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.1.2...netapp/v1.1.3) (2024-07-10)


### Bug Fixes

* **netapp:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.1.2](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.1.1...netapp/v1.1.2) (2024-07-01)


### Bug Fixes

* **netapp:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.1.1](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.1.0...netapp/v1.1.1) (2024-06-26)


### Bug Fixes

* **netapp:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v1.0.0...netapp/v1.1.0) (2024-05-22)


### Features

* **netapp:** Add a new Service Level FLEX ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))
* **netapp:** Add backup chain bytes to BackupConfig in Volume ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))
* **netapp:** Add Location metadata support ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))
* **netapp:** Add Tiering Policy to Volume ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.8...netapp/v1.0.0) (2024-05-16)


### Features

* **netapp:** Promote client to GA ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))


### Miscellaneous Chores

* **netapp:** Promote to GA v1.0.0 ([#10210](https://github.com/googleapis/google-cloud-go/issues/10210)) ([fc2fb6b](https://github.com/googleapis/google-cloud-go/commit/fc2fb6b2650a0b850913d4c11b4cff8416f0f02c))

## [0.2.8](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.7...netapp/v0.2.8) (2024-05-01)


### Bug Fixes

* **netapp:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.2.7](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.6...netapp/v0.2.7) (2024-03-25)


### Documentation

* **netapp:** Rephrase comment on psa_range ([1ef5b19](https://github.com/googleapis/google-cloud-go/commit/1ef5b1917bb9a1271c3fb152413ec0e74163164d))

## [0.2.6](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.5...netapp/v0.2.6) (2024-03-14)


### Bug Fixes

* **netapp:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.2.5](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.4...netapp/v0.2.5) (2024-03-07)


### Documentation

* **netapp:** Mark optional fields explicitly in Storage Pool ([#9513](https://github.com/googleapis/google-cloud-go/issues/9513)) ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))

## [0.2.4](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.3...netapp/v0.2.4) (2024-01-30)


### Bug Fixes

* **netapp:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.2.3](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.2...netapp/v0.2.3) (2023-11-01)


### Bug Fixes

* **netapp:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.1...netapp/v0.2.2) (2023-10-26)


### Bug Fixes

* **netapp:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.2.0...netapp/v0.2.1) (2023-10-12)


### Bug Fixes

* **netapp:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/netapp/v0.1.0...netapp/v0.2.0) (2023-08-08)


### Features

* **netapp:** Add RestrictedAction to Volume ([#8378](https://github.com/googleapis/google-cloud-go/issues/8378)) ([b9e56d2](https://github.com/googleapis/google-cloud-go/commit/b9e56d2fdb6c770d58964e1e23148a712f74b9ad))

## 0.1.0 (2023-07-31)


### Features

* **netapp:** Start generating apiv1 ([#8353](https://github.com/googleapis/google-cloud-go/issues/8353)) ([f609b3c](https://github.com/googleapis/google-cloud-go/commit/f609b3cf831fb89c45386f81d0047560120cb3f4))

## Changes
