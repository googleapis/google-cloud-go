# Changes


## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.11.2...datastream/v1.12.0) (2024-11-21)


### Features

* **datastream:** A new field `append_only` is added to message `.google.cloud.datastream.v1.BigQueryDestinationConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `binary_log_parser` is added to message `.google.cloud.datastream.v1.OracleSourceConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `binary_log_position` is added to message `.google.cloud.datastream.v1.MysqlSourceConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `gtid` is added to message `.google.cloud.datastream.v1.MysqlSourceConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `last_recovery_time` is added to message `.google.cloud.datastream.v1.Stream` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `log_miner` is added to message `.google.cloud.datastream.v1.OracleSourceConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `merge` is added to message `.google.cloud.datastream.v1.BigQueryDestinationConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `oracle_asm_config` is added to message `.google.cloud.datastream.v1.OracleProfile` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `oracle_ssl_config` is added to message `.google.cloud.datastream.v1.OracleProfile` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `secret_manager_stored_password` is added to message `.google.cloud.datastream.v1.OracleProfile` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_excluded_objects` is added to message `.google.cloud.datastream.v1.Stream` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_identifier` is added to message `.google.cloud.datastream.v1.SourceObjectIdentifier` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_profile` is added to message `.google.cloud.datastream.v1.ConnectionProfile` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_rdbms` is added to message `.google.cloud.datastream.v1.DiscoverConnectionProfileRequest` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_rdbms` is added to message `.google.cloud.datastream.v1.DiscoverConnectionProfileResponse` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new field `sql_server_source_config` is added to message `.google.cloud.datastream.v1.SourceConfig` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `AppendOnly` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `BinaryLogParser` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `BinaryLogPosition` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `CdcStrategy` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `Gtid` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `LogMiner` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `Merge` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `MysqlLogPosition` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `OracleAsmConfig` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `OracleScnPosition` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `OracleSslConfig` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `RunStreamRequest` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerChangeTables` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerColumn` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerLsnPosition` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerObjectIdentifier` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerProfile` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerRdbms` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerSchema` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerSourceConfig` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerTable` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new message `SqlServerTransactionLogs` is added ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new method `RunStream` is added to service `Datastream` ([#11153](https://github.com/googleapis/google-cloud-go/issues/11153)) ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A new value `WARNING` is added to enum `State` ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))


### Documentation

* **datastream:** A comment for field `dataset_id` in message `.google.cloud.datastream.v1.BigQueryDestinationConfig` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `password` in message `.google.cloud.datastream.v1.MysqlProfile` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `password` in message `.google.cloud.datastream.v1.OracleProfile` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `password` in message `.google.cloud.datastream.v1.PostgresqlProfile` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `requested_cancellation` in message `.google.cloud.datastream.v1.OperationMetadata` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `state` in message `.google.cloud.datastream.v1.BackfillJob` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `state` in message `.google.cloud.datastream.v1.Validation` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for field `stream_large_objects` in message `.google.cloud.datastream.v1.OracleSourceConfig` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for message `MysqlProfile` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))
* **datastream:** A comment for message `OracleProfile` is changed ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))

## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.11.1...datastream/v1.11.2) (2024-10-23)


### Bug Fixes

* **datastream:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **datastream:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.11.0...datastream/v1.11.1) (2024-09-12)


### Bug Fixes

* **datastream:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.11...datastream/v1.11.0) (2024-08-20)


### Features

* **datastream:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.10.11](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.10...datastream/v1.10.11) (2024-08-08)


### Bug Fixes

* **datastream:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.10.10](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.9...datastream/v1.10.10) (2024-07-24)


### Bug Fixes

* **datastream:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.10.9](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.8...datastream/v1.10.9) (2024-07-10)


### Bug Fixes

* **datastream:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.10.8](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.7...datastream/v1.10.8) (2024-07-01)


### Bug Fixes

* **datastream:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.10.7](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.6...datastream/v1.10.7) (2024-06-26)


### Bug Fixes

* **datastream:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.10.6](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.5...datastream/v1.10.6) (2024-05-01)


### Bug Fixes

* **datastream:** Add internaloption.WithDefaultEndpointTemplate ([3b41408](https://github.com/googleapis/google-cloud-go/commit/3b414084450a5764a0248756e95e13383a645f90))
* **datastream:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.10.5](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.4...datastream/v1.10.5) (2024-03-14)


### Bug Fixes

* **datastream:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.10.4](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.3...datastream/v1.10.4) (2024-01-30)


### Bug Fixes

* **datastream:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.10.3](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.2...datastream/v1.10.3) (2023-11-01)


### Bug Fixes

* **datastream:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.10.2](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.1...datastream/v1.10.2) (2023-10-26)


### Bug Fixes

* **datastream:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.10.0...datastream/v1.10.1) (2023-10-12)


### Bug Fixes

* **datastream:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.9.1...datastream/v1.10.0) (2023-07-26)


### Features

* **datastream:** Add precision and scale to MysqlColumn ([7cb7f66](https://github.com/googleapis/google-cloud-go/commit/7cb7f66f0646617c27aa9a9b4fe38b9f368eb3bb))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.9.0...datastream/v1.9.1) (2023-06-20)


### Bug Fixes

* **datastream:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.8.0...datastream/v1.9.0) (2023-05-30)


### Features

* **datastream:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.7.1...datastream/v1.8.0) (2023-05-10)


### Features

* **datastream:** Max concurrent backfill tasks ([31c3766](https://github.com/googleapis/google-cloud-go/commit/31c3766c9c4cab411669c14fc1a30bd6d2e3f2dd))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.7.0...datastream/v1.7.1) (2023-05-08)


### Bug Fixes

* **datastream:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.6.0...datastream/v1.7.0) (2023-03-15)


### Features

* **datastream:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.5.0...datastream/v1.6.0) (2023-01-04)


### Features

* **datastream:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.4.0...datastream/v1.5.0) (2022-11-03)


### Features

* **datastream:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.3.0...datastream/v1.4.0) (2022-10-25)


### Features

* **datastream:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.2.0...datastream/v1.3.0) (2022-09-21)


### Features

* **datastream:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.1.0...datastream/v1.2.0) (2022-09-19)


### Features

* **datastream:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v1.0.0...datastream/v1.1.0) (2022-09-06)


### Features

* **datastream:** added support for BigQuery destination and PostgreSQL source types ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v0.5.0...datastream/v1.0.0) (2022-06-29)


### Features

* **datastream:** release 1.0.0 ([7678be5](https://github.com/googleapis/google-cloud-go/commit/7678be543d9130dcd8fc4147608a10b70faef44e))
* **datastream:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))


### Miscellaneous Chores

* **datastream:** release 1.0.0 ([53f7cbd](https://github.com/googleapis/google-cloud-go/commit/53f7cbdd253e4ac224fa7d8ed3fa378e0dc8c97e))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v0.4.0...datastream/v0.5.0) (2022-05-24)


### Features

* **datastream:** Include the location mixin client ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v0.3.0...datastream/v0.4.0) (2022-05-09)


### Features

* **datastream:** start generating apiv1 ([#6003](https://github.com/googleapis/google-cloud-go/issues/6003)) ([0c9e0f9](https://github.com/googleapis/google-cloud-go/commit/0c9e0f9d4c7ddfe020b61f0cf8540246c4c9695e)), refs [#5958](https://github.com/googleapis/google-cloud-go/issues/5958)

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v0.2.0...datastream/v0.3.0) (2022-02-23)


### Features

* **datastream:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/datastream/v0.1.1...datastream/v0.2.0) (2022-02-14)


### Features

* **datastream:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

### [0.1.1](https://www.github.com/googleapis/google-cloud-go/compare/datastream/v0.1.0...datastream/v0.1.1) (2021-08-30)


### Bug Fixes

* **datastream:** Change a few resource pattern variables from camelCase to snake_case ([bf4378b](https://www.github.com/googleapis/google-cloud-go/commit/bf4378b5b859f7b835946891dbfebfee31c4b123))

## v0.1.0

This is the first tag to carve out datastream as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
