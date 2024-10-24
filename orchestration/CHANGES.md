# Changes


## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.11.0...orchestration/v1.11.1) (2024-10-23)


### Bug Fixes

* **orchestration:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **orchestration:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.10.1...orchestration/v1.11.0) (2024-09-19)


### Features

* **orchestration/airflow/service:** A new field `airflow_metadata_retention_config` is added to message `.google.cloud.orchestration.airflow.service.v1.DataRetentionConfig` ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A new field `satisfies_pzi` is added to message `.google.cloud.orchestration.airflow.service.v1.Environment` ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A new message `AirflowMetadataRetentionPolicyConfig` is added ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A new message `CheckUpgradeRequest` is added ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A new method `CheckUpgrade` is added to service `Environments` ([#10854](https://github.com/googleapis/google-cloud-go/issues/10854)) ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))


### Documentation

* **orchestration/airflow/service:** A comment for field `maintenance_window` in message `.google.cloud.orchestration.airflow.service.v1.EnvironmentConfig` is changed ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A comment for field `storage_mode` in message `.google.cloud.orchestration.airflow.service.v1.TaskLogsRetentionConfig` is changed ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))
* **orchestration/airflow/service:** A comment for message `WorkloadsConfig` is changed ([c56f8ac](https://github.com/googleapis/google-cloud-go/commit/c56f8ac77567ab52f374fdd02364a2c0a3df879c))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.10.0...orchestration/v1.10.1) (2024-09-12)


### Bug Fixes

* **orchestration:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.7...orchestration/v1.10.0) (2024-08-20)


### Features

* **orchestration:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.9.7](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.6...orchestration/v1.9.7) (2024-08-08)


### Bug Fixes

* **orchestration:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.9.6](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.5...orchestration/v1.9.6) (2024-07-24)


### Bug Fixes

* **orchestration:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.9.5](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.4...orchestration/v1.9.5) (2024-07-10)


### Bug Fixes

* **orchestration:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.9.4](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.3...orchestration/v1.9.4) (2024-07-01)


### Bug Fixes

* **orchestration:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.9.3](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.2...orchestration/v1.9.3) (2024-06-26)


### Bug Fixes

* **orchestration:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.9.2](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.1...orchestration/v1.9.2) (2024-05-01)


### Bug Fixes

* **orchestration:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.9.0...orchestration/v1.9.1) (2024-03-14)


### Bug Fixes

* **orchestration:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.5...orchestration/v1.9.0) (2024-02-21)


### Features

* **orchestration/airflow/service:** Added ListWorkloads RPC ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))

## [1.8.5](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.4...orchestration/v1.8.5) (2024-01-30)


### Bug Fixes

* **orchestration:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.8.4](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.3...orchestration/v1.8.4) (2023-11-01)


### Bug Fixes

* **orchestration:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.8.3](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.2...orchestration/v1.8.3) (2023-10-26)


### Bug Fixes

* **orchestration:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.8.2](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.1...orchestration/v1.8.2) (2023-10-12)


### Bug Fixes

* **orchestration:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.8.0...orchestration/v1.8.1) (2023-06-20)


### Bug Fixes

* **orchestration:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.7.0...orchestration/v1.8.0) (2023-06-13)


### Features

* **orchestration/airflow/service:** Added RPCs StopAirflowCommand, ExecuteAirflowCommand, PollAirflowCommand, DatabaseFailover, FetchDatabaseProperties ([#8081](https://github.com/googleapis/google-cloud-go/issues/8081)) ([3abdfa1](https://github.com/googleapis/google-cloud-go/commit/3abdfa14dd56cf773c477f289a7f888e20bbbd9a))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.6.1...orchestration/v1.7.0) (2023-05-30)


### Features

* **orchestration:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.6.0...orchestration/v1.6.1) (2023-05-08)


### Bug Fixes

* **orchestration:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.5.0...orchestration/v1.6.0) (2023-01-04)


### Features

* **orchestration:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.4.0...orchestration/v1.5.0) (2022-12-15)


### Features

* **orchestration/airflow/service:** Added LoadSnapshot, SaveSnapshot RPCs feat: added fields maintenance_window, workloads_config, environment_size, master_authorized_networks_config, recovery_config to EnvironmentConfig feat: added field scheduler_count to SoftwareConfig feat: added field enable_ip_masq_agent to NodeConfig feat: added fields cloud_composer_network_ipv4_cidr_block, cloud_composer_network_ipv4_reserved_range, enable_privately_used_public_ips, cloud_composer_connection_subnetwork, networking_config to PrivateEnvironmentConfig ([7357077](https://github.com/googleapis/google-cloud-go/commit/735707796d81d7f6f32fc3415800c512fe62297e))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.3.0...orchestration/v1.4.0) (2022-11-03)


### Features

* **orchestration:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.2.0...orchestration/v1.3.0) (2022-10-25)


### Features

* **orchestration:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.1.0...orchestration/v1.2.0) (2022-02-23)


### Features

* **orchestration:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/orchestration/v1.0.0...orchestration/v1.1.0) (2022-02-14)


### Features

* **orchestration:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.0.0](https://www.github.com/googleapis/google-cloud-go/compare/orchestration/v0.1.0...orchestration/v1.0.0) (2022-01-25)


### Features

* **orchestration/airflow/service:** to v1 ([#5138](https://www.github.com/googleapis/google-cloud-go/issues/5138)) ([5fda0bc](https://www.github.com/googleapis/google-cloud-go/commit/5fda0bccc5b68a5bc00c71bad6b032bd0708ae96))

## v0.1.0

- feat(orchestration): start generating clients
