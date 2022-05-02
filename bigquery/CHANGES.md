# Changes

## [1.31.0](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.30.2...bigquery/v1.31.0) (2022-04-12)


### Features

* **bigquery/storage:** Deprecate format specific `row_count` field in Read API ([57896d1](https://github.com/googleapis/google-cloud-go/commit/57896d1491c04fa53d3f3e2344ef10c3d91c4b65))
* **bigquery:** enhance SchemaFromJSON ([#5877](https://github.com/googleapis/google-cloud-go/issues/5877)) ([16289f0](https://github.com/googleapis/google-cloud-go/commit/16289f086ae15ea18c70d387b542796e099d4a09))
* **bigquery:** support table cloning ([#5672](https://github.com/googleapis/google-cloud-go/issues/5672)) ([74c120a](https://github.com/googleapis/google-cloud-go/commit/74c120a81d2181d9809e5d3c0462bd859297d073))

### [1.30.2](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.30.1...bigquery/v1.30.2) (2022-03-30)


### Bug Fixes

* **bigquery/storage/managedwriter/adapt:** fix enum append ([#5819](https://github.com/googleapis/google-cloud-go/issues/5819)) ([9eeaf0f](https://github.com/googleapis/google-cloud-go/commit/9eeaf0fe9de6e9583a6994e49f95ad524bc9e68e))

### [1.30.1](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.30.0...bigquery/v1.30.1) (2022-03-30)


### Bug Fixes

* **bigquery/storage/managedwriter:** correct enum processing in NormalizeDescriptor ([#5811](https://github.com/googleapis/google-cloud-go/issues/5811)) ([52cf48e](https://github.com/googleapis/google-cloud-go/commit/52cf48edff487352c2755de86e2ea069b1b29617))
* **bigquery:** improve retry for table create ([#5807](https://github.com/googleapis/google-cloud-go/issues/5807)) ([f27d1dc](https://github.com/googleapis/google-cloud-go/commit/f27d1dc43acbd437f517c05d65c992644f3f3111))

## [1.30.0](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.29.0...bigquery/v1.30.0) (2022-03-16)


### Features

* **bigquery:** support authorized datasets ([#5666](https://github.com/googleapis/google-cloud-go/issues/5666)) ([859048e](https://github.com/googleapis/google-cloud-go/commit/859048e491dd840c9aea218fa670ed2fb46d78e2))


### Bug Fixes

* **bigquery:** Query.Read fails with dry-run queries ([#5753](https://github.com/googleapis/google-cloud-go/issues/5753)) ([e279584](https://github.com/googleapis/google-cloud-go/commit/e279584727e2a496b3d566ed6f6683715a594a6d))

## [1.29.0](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.28.0...bigquery/v1.29.0) (2022-03-02)


### Features

* **bigquery/storage/managedwriter/adapt:** handle oneof normalization ([#5670](https://github.com/googleapis/google-cloud-go/issues/5670)) ([c7f54d8](https://github.com/googleapis/google-cloud-go/commit/c7f54d81baa34ce0f31bbe0af1cb03c2598e5e74))
* **bigquery/storage/managedwriter:** minor ease-of-use improvements ([#5660](https://github.com/googleapis/google-cloud-go/issues/5660)) ([d253c24](https://github.com/googleapis/google-cloud-go/commit/d253c24fd61f181971056ba00749efd69b3ae691))
* **bigquery/storage:** add trace_id for Read API ([080adb0](https://github.com/googleapis/google-cloud-go/commit/080adb0b855289ddbd86ac9e5e6eb236673f6884))
* **bigquery:** add job timeout support ([#5707](https://github.com/googleapis/google-cloud-go/issues/5707)) ([868363c](https://github.com/googleapis/google-cloud-go/commit/868363cbc68c655d4c1f8959280cf1acba5073a7))
* **bigquery:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))


### Bug Fixes

* **bigquery/storage:** remove bigquery.readonly auth scope ([5af548b](https://github.com/googleapis/google-cloud-go/commit/5af548bee4ffde279727b2e1ad9b072925106a74))

## [1.28.0](https://github.com/googleapis/google-cloud-go/compare/bigquery/v1.27.0...bigquery/v1.28.0) (2022-02-14)


### Features

* **bigquery/datatransfer:** add owner email to TransferConfig message feat: allow customer to enroll a datasource programmatically docs: improvements to various message and field descriptions ([f560b1e](https://github.com/googleapis/google-cloud-go/commit/f560b1ed0263956ef84fbf2fbf34bdc66dbc0a88))
* **bigquery:** add better version metadata to calls ([d1ad921](https://github.com/googleapis/google-cloud-go/commit/d1ad921d0322e7ce728ca9d255a3cf0437d26add))


### Bug Fixes

* **bigquery/storage/managedwriter:** address possible panic due to flow ([#5436](https://github.com/googleapis/google-cloud-go/issues/5436)) ([50c6e38](https://github.com/googleapis/google-cloud-go/commit/50c6e38c2798b3d4f2a9560239753ecd04502273))
* **bigquery/storage/managedwriter:** append improvements ([#5465](https://github.com/googleapis/google-cloud-go/issues/5465)) ([aa167bd](https://github.com/googleapis/google-cloud-go/commit/aa167bd5e57facb0f0d6834ab65805956e4ef08c))

## [1.27.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.26.0...bigquery/v1.27.0) (2022-01-24)


### Features

* **bigquery:** augment retry predicate ([#5387](https://www.github.com/googleapis/google-cloud-go/issues/5387)) ([f9608d4](https://www.github.com/googleapis/google-cloud-go/commit/f9608d4622c56362b2ed0a5845b8fe27f81995aa))
* **bigquery:** support null marker for csv in external data config ([#5287](https://www.github.com/googleapis/google-cloud-go/issues/5287)) ([132904a](https://www.github.com/googleapis/google-cloud-go/commit/132904a061809ba7117c51e8a8000f1adac34e48))

## [1.26.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.25.0...bigquery/v1.26.0) (2022-01-04)


### Features

* **bigquery/reservation:** increase the logical timeout (retry deadline) to 5 minutes ([5444809](https://www.github.com/googleapis/google-cloud-go/commit/5444809e0b7cf9f5416645ea2df6fec96f8b9023))
* **bigquery/storage/managedwriter:** support schema change notification ([#5253](https://www.github.com/googleapis/google-cloud-go/issues/5253)) ([70e40db](https://www.github.com/googleapis/google-cloud-go/commit/70e40db88bc016f228a425da1e278fc76dbf2e36))
* **bigquery/storage:** add write_mode support for BigQuery Storage Write API v1 ([615b42b](https://www.github.com/googleapis/google-cloud-go/commit/615b42bbb549b6fd3e8b1ba751bc109f79a5575b))

## [1.25.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.24.0...bigquery/v1.25.0) (2021-12-02)


### ⚠ BREAKING CHANGES

* **bigquery/storage/managedwriter:** changes function signatures to add variadic call options

### Features

* **bigquery/storage/managedwriter:** extend managedstream to support call options ([#5078](https://www.github.com/googleapis/google-cloud-go/issues/5078)) ([fbc2717](https://www.github.com/googleapis/google-cloud-go/commit/fbc2717ec84b1c5557873efaa732c047da66c1e6))
* **bigquery/storage/managedwriter:** improve method parity in managedwriter ([#5007](https://www.github.com/googleapis/google-cloud-go/issues/5007)) ([a2af4de](https://www.github.com/googleapis/google-cloud-go/commit/a2af4de215a42848368ec3081263d34782032caa))
* **bigquery/storage/managedwriter:** support variadic appends ([#5102](https://www.github.com/googleapis/google-cloud-go/issues/5102)) ([014b314](https://www.github.com/googleapis/google-cloud-go/commit/014b314b2db70147a26120a1d54a6bc7142d5665))
* **bigquery:** add BI Engine information to query statistics ([#5081](https://www.github.com/googleapis/google-cloud-go/issues/5081)) ([b78c89b](https://www.github.com/googleapis/google-cloud-go/commit/b78c89b18a81ce155441554cb5455600168eb8fd))
* **bigquery:** add support for AvroOptions in external data config ([#4945](https://www.github.com/googleapis/google-cloud-go/issues/4945)) ([8844e40](https://www.github.com/googleapis/google-cloud-go/commit/8844e40b7c2a7347e174587ea2cf438a6da9e16f))
* **bigquery:** allow construction of jobs from other projects ([#5048](https://www.github.com/googleapis/google-cloud-go/issues/5048)) ([6d07eca](https://www.github.com/googleapis/google-cloud-go/commit/6d07eca680362807f6dd870ba9df8c26256601ab))
* **bigquery:** expose identifiers using a variety of formats ([#5017](https://www.github.com/googleapis/google-cloud-go/issues/5017)) ([c9cd984](https://www.github.com/googleapis/google-cloud-go/commit/c9cd9846b6707d236648d33d44434e64eced9cdd))


### Bug Fixes

* **bigquery/migration:** correct python namespace for migration API Committer: [@shollyman](https://www.github.com/shollyman) ([8c5c6cf](https://www.github.com/googleapis/google-cloud-go/commit/8c5c6cf9df046b67998a8608d05595bd9e34feb0))
* **bigquery/storage/managedwriter:** correctly copy request ([#5122](https://www.github.com/googleapis/google-cloud-go/issues/5122)) ([cd43a5c](https://www.github.com/googleapis/google-cloud-go/commit/cd43a5cde5e4e388266f3773f206ead90d666261))
* **bigquery:** address one other callsite for the job construction feature ([#5059](https://www.github.com/googleapis/google-cloud-go/issues/5059)) ([98779eb](https://www.github.com/googleapis/google-cloud-go/commit/98779eba0f1f95b195aa6194210208767c169f5e))


### Miscellaneous Chores

* **bigquery:** release 1.25.0 ([#5128](https://www.github.com/googleapis/google-cloud-go/issues/5128)) ([f58a9f7](https://www.github.com/googleapis/google-cloud-go/commit/f58a9f7b88e2ce6101cb4bd3c85c267a688a1a1d))
* **bigquery:** release 1.25.0 ([#5177](https://www.github.com/googleapis/google-cloud-go/issues/5177)) ([359f5b1](https://www.github.com/googleapis/google-cloud-go/commit/359f5b1ca118ff6f92603da083eb943b672ed779))

## [1.24.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.23.0...bigquery/v1.24.0) (2021-09-27)


### Features

* **bigquery/migration:** Add PAUSED state to Subtask and add task details protos ([bddab08](https://www.github.com/googleapis/google-cloud-go/commit/bddab08dfd0b9a0a79b113a46a0dd84dba1f3d3b))


### Bug Fixes

* **bigquery/storage:** add missing read api retry setting on SplitReadStream ([797a9bd](https://www.github.com/googleapis/google-cloud-go/commit/797a9bdcb68c0c3ff7eef04cd3a3a0747937975b))

## [1.23.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.22.0...bigquery/v1.23.0) (2021-09-23)


### Features

* **bigquery/reservation:**
  * Deprecated SearchAssignments in favor of SearchAllAssignments
  * feat: Reservation objects now contain a creation time and an update time
  * feat: Added commitment_start_time to capacity commitments
  * feat: Force deleting capacity commitments is allowed while reservations with active assignments exist
  * feat: ML_EXTERNAL job type is supported
  * feat: Optional id can be passed into CreateCapacityCommitment and CreateAssignment
  * docs: Clarified docs for None assignments
  * fix!: Fixed pattern for BiReservation object BREAKING_CHANGE: Changed from `bireservation` to `biReservation`
  * ([d9ce9d0](https://www.github.com/googleapis/google-cloud-go/commit/d9ce9d0ee64f59c4e07ce4752bfd721051a95ac7))
* **bigquery/storage/managedwriter:** BREAKING CHANGE: changeAppendRows behavior ([#4729](https://github.com/googleapis/google-cloud-go/pull/4729))
* **bigquery/storage:** add BigQuery Storage Write API v1 ([e52c204](https://www.github.com/googleapis/google-cloud-go/commit/e52c2042a2b7cdd7dd799a561421f32fecc5d1d2))
* **bigquery/storage:** migrate managedwriter to v1 write from v1beta2 ([#4788](https://github.com/googleapis/google-cloud-go/pull/4788))
* **bigquery:** add session and connection support ([#4754](https://www.github.com/googleapis/google-cloud-go/issues/4754)) ([e846dfd](https://www.github.com/googleapis/google-cloud-go/commit/e846dfdefbba88320088667525e5fdd966c80c4b))
* **bigquery:** expose the query source of a rowiterator via SourceJob() ([#4748](https://github.com/googleapis/google-cloud-go/pull/4748))

## [1.22.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.21.0...bigquery/v1.22.0) (2021-08-30)


### Features

* **bigquery/storage/managedwriter/adapt:** add NormalizeDescriptor ([#4681](https://www.github.com/googleapis/google-cloud-go/issues/4681)) ([c54aa74](https://www.github.com/googleapis/google-cloud-go/commit/c54aa74f7a0574cbbe3f65dc90b96cf5a0b1aa88))
* **bigquery/storage/managedwriter:** more metrics instrumentation ([#4690](https://www.github.com/googleapis/google-cloud-go/issues/4690)) ([9505384](https://www.github.com/googleapis/google-cloud-go/commit/9505384b2c771d7d0c95f7786744bdf76174c706))

## [1.21.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.20.1...bigquery/v1.21.0) (2021-08-16)


### Features

* **bigquery/storage/managedwriter:** add project autodetection ([#4605](https://www.github.com/googleapis/google-cloud-go/issues/4605)) ([d8cc9be](https://www.github.com/googleapis/google-cloud-go/commit/d8cc9be6f0314f585f708638834abfc209799724))
* **bigquery/storage/managedwriter:** improve protobuf support ([#4589](https://www.github.com/googleapis/google-cloud-go/issues/4589)) ([a455082](https://www.github.com/googleapis/google-cloud-go/commit/a45508272a730e0ad81021695d2d8564e7c81631))
* **bigquery/storage/managedwriter:** more instrumentation support ([#4601](https://www.github.com/googleapis/google-cloud-go/issues/4601)) ([ff488c8](https://www.github.com/googleapis/google-cloud-go/commit/ff488c86b9c1a1f02397bb579905fa049e59ac05))
* **bigquery:** switch to centralized project autodetect logic ([#4625](https://www.github.com/googleapis/google-cloud-go/issues/4625)) ([18ff070](https://www.github.com/googleapis/google-cloud-go/commit/18ff070b8baa3ed7d324ca9ea00dcd66d7742340))


### Bug Fixes

* **bigquery/storage/managedwriter:** support non-default regions ([#4566](https://www.github.com/googleapis/google-cloud-go/issues/4566)) ([68418f9](https://www.github.com/googleapis/google-cloud-go/commit/68418f9e340def179eb5556aea433c0d07000b79))

### [1.20.1](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.20.0...bigquery/v1.20.1) (2021-08-06)


### Bug Fixes

* **bigquery/storage/managedwriter:** fix flowcontroller double-release ([#4555](https://www.github.com/googleapis/google-cloud-go/issues/4555)) ([67facd9](https://www.github.com/googleapis/google-cloud-go/commit/67facd9697e931e193f3cd8e188f1dd819ba31eb))

## [1.20.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.19.0...bigquery/v1.20.0) (2021-07-30)


### Features

* **bigquery/connection:** add cloud spanner connection support ([458f15b](https://www.github.com/googleapis/google-cloud-go/commit/458f15bb6f1193ce83dbfc7a82c3f2a672f52c06))
* **bigquery/storage/managedwriter/adapt:** add schema -> proto support ([#4375](https://www.github.com/googleapis/google-cloud-go/issues/4375)) ([4ff6243](https://www.github.com/googleapis/google-cloud-go/commit/4ff62433f58c1c92976a66e890b7d5394198f77b))
* **bigquery/storage/managedwriter:** add append stream plumbing ([#4452](https://www.github.com/googleapis/google-cloud-go/issues/4452)) ([b085384](https://www.github.com/googleapis/google-cloud-go/commit/b0853846a34a32ca45deb92a3cc6ab843473acd8))
* **bigquery/storage/managedwriter:** add base client ([#4422](https://www.github.com/googleapis/google-cloud-go/issues/4422)) ([4f7193b](https://www.github.com/googleapis/google-cloud-go/commit/4f7193b74f4b1954cf7b664d61b5cc9805337e84))
* **bigquery/storage/managedwriter:** add flow controller ([#4404](https://www.github.com/googleapis/google-cloud-go/issues/4404)) ([9dc78e0](https://www.github.com/googleapis/google-cloud-go/commit/9dc78e073b5f69037c6328460554c4354fcee11f))
* **bigquery/storage/managedwriter:** add opencensus instrumentation ([#4512](https://www.github.com/googleapis/google-cloud-go/issues/4512)) ([73b6f5e](https://www.github.com/googleapis/google-cloud-go/commit/73b6f5e012d0b89d36850cb986fd7e288bf1e3c5))
* **bigquery/storage/managedwriter:** add state tracking ([#4407](https://www.github.com/googleapis/google-cloud-go/issues/4407)) ([4638e17](https://www.github.com/googleapis/google-cloud-go/commit/4638e17dacd1fa76f9976f44974c4037fe4358dc))
* **bigquery/storage/managedwriter:** naming and doc improvements ([#4508](https://www.github.com/googleapis/google-cloud-go/issues/4508)) ([663c899](https://www.github.com/googleapis/google-cloud-go/commit/663c899c3b8aa751527d24f541d964f2ba91a233))
* **bigquery/storage/managedwriter:** wire in flow controller ([#4501](https://www.github.com/googleapis/google-cloud-go/issues/4501)) ([40571fa](https://www.github.com/googleapis/google-cloud-go/commit/40571fa0e3b5ab326fd592a6907061c2304893aa))
* **bigquery:** add more dml statistics to query statistics ([#4405](https://www.github.com/googleapis/google-cloud-go/issues/4405)) ([99d5728](https://www.github.com/googleapis/google-cloud-go/commit/99d57282f6668de91390ad29a888a89209689f39))
* **bigquery:** support decimalTargetType prioritization ([#4343](https://www.github.com/googleapis/google-cloud-go/issues/4343)) ([95a27f7](https://www.github.com/googleapis/google-cloud-go/commit/95a27f711a1c7dfdaa16ae5d3c52644769b6fc39))
* **bigquery:** support multistatement transaction statistics in jobs ([#4485](https://www.github.com/googleapis/google-cloud-go/issues/4485)) ([4565eb7](https://www.github.com/googleapis/google-cloud-go/commit/4565eb7fe730eade294fb3baa85bd255df008bfa))


### Bug Fixes

* **bigquery/storage/managedwriter:** fix double-close error, add tests ([#4502](https://www.github.com/googleapis/google-cloud-go/issues/4502)) ([c6cf659](https://www.github.com/googleapis/google-cloud-go/commit/c6cf6590a41368885b7399c993c47dc965862558))

## [1.19.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.18.0...bigquery/v1.19.0) (2021-06-29)


### Features

* **bigquery/storage:** Add ZSTD compression as an option for Arrow. ([770db30](https://www.github.com/googleapis/google-cloud-go/commit/770db3083270d485d265362fe5a4b2a1b23619ff))
* **bigquery/storage:** remove alpha client ([#4100](https://www.github.com/googleapis/google-cloud-go/issues/4100)) ([a2d137d](https://www.github.com/googleapis/google-cloud-go/commit/a2d137d233e7a401976fbe1fd8ff81145dda515d)), refs [#4098](https://www.github.com/googleapis/google-cloud-go/issues/4098)
* **bigquery:** add support for parameterized types ([#4103](https://www.github.com/googleapis/google-cloud-go/issues/4103)) ([a2330e4](https://www.github.com/googleapis/google-cloud-go/commit/a2330e4d66c0a1832fb3b9e23a33c006c9345c28))
* **bigquery:** add support for snapshot/restore ([#4112](https://www.github.com/googleapis/google-cloud-go/issues/4112)) ([4c12b42](https://www.github.com/googleapis/google-cloud-go/commit/4c12b424eec06c7d87244eaa922995bbe6e46e7e))
* **bigquery:** add support for user defined TVF ([#4043](https://www.github.com/googleapis/google-cloud-go/issues/4043)) ([37607b4](https://www.github.com/googleapis/google-cloud-go/commit/37607b4afbc4c42baa4a931a9a86cddcc6d885ca))
* **bigquery:** enable project autodetection, expose project ids further ([#4312](https://www.github.com/googleapis/google-cloud-go/issues/4312)) ([267787e](https://www.github.com/googleapis/google-cloud-go/commit/267787eb245d9307cf78304c1ce34bdfb2aaf5ab))
* **bigquery:** support job deletion ([#3935](https://www.github.com/googleapis/google-cloud-go/issues/3935)) ([363ba03](https://www.github.com/googleapis/google-cloud-go/commit/363ba03e1c3c813749a65ff3c050877ce4f60016))
* **bigquery:** support nullable params and geography params ([#4225](https://www.github.com/googleapis/google-cloud-go/issues/4225)) ([43755d3](https://www.github.com/googleapis/google-cloud-go/commit/43755d38b5d928222127cc6be26183d6bfbb1cb4))


### Bug Fixes

* **bigquery:** minor rename to feature that's not yet in a release ([#4320](https://www.github.com/googleapis/google-cloud-go/issues/4320)) ([ef8d138](https://www.github.com/googleapis/google-cloud-go/commit/ef8d1386149cff28ae6258ab167789bae6af6407))
* **bigquery:** update streaming insert error test ([#4321](https://www.github.com/googleapis/google-cloud-go/issues/4321)) ([12f3042](https://www.github.com/googleapis/google-cloud-go/commit/12f3042716d51fb0d7a23071d00a20f9751bac91))

## [1.18.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.17.0...bigquery/v1.18.0) (2021-05-06)


### Features

* **bigquery/storage:** new JSON type through BigQuery Write ([9029071](https://www.github.com/googleapis/google-cloud-go/commit/90290710158cf63de918c2d790df48f55a23adc5))
* **bigquery:** augment retry predicate to support additional errors ([#4046](https://www.github.com/googleapis/google-cloud-go/issues/4046)) ([d4af6f7](https://www.github.com/googleapis/google-cloud-go/commit/d4af6f7707b3c5ee12cde53c7485a9b743034119))
* **bigquery:** expose ParquetOptions for loads and external tables ([#4016](https://www.github.com/googleapis/google-cloud-go/issues/4016)) ([f9c4ccb](https://www.github.com/googleapis/google-cloud-go/commit/f9c4ccb6efb271c421edf3f67d5249b1cfb0ecb2))
* **bigquery:** support mutable clustering configuration ([#3950](https://www.github.com/googleapis/google-cloud-go/issues/3950)) ([0ab30da](https://www.github.com/googleapis/google-cloud-go/commit/0ab30dadc43ae85354dc12a4130ecfcc56273882))

## [1.17.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.15.0...bigquery/v1.17.0) (2021-04-08)


### Features

* **bigquery/storage:** add a Arrow compression options (Only LZ4 for now). feat: Return schema on first ReadRowsResponse. doc: clarify limit on filter string. ([2b02a03](https://www.github.com/googleapis/google-cloud-go/commit/2b02a03ff9f78884da5a8e7b64a336014c61bde7))
* **bigquery/storage:** deprecate bigquery storage v1alpha2 API ([9cc6d2c](https://www.github.com/googleapis/google-cloud-go/commit/9cc6d2cce96235b0a144c1c6b48eff496f9e5fa7))
* **bigquery/storage:** updates for v1beta2 storage API - Updated comments on BatchCommitWriteStreams - Added new support Bigquery types BIGNUMERIC and INTERVAL to TableSchema - Added read rows schema in ReadRowsResponse - Misc comment updates ([48b4e59](https://www.github.com/googleapis/google-cloud-go/commit/48b4e596206cef879194d2888186d603a6f51292))
* **bigquery:** export HivePartitioningOptions in load job configurations ([#3877](https://www.github.com/googleapis/google-cloud-go/issues/3877)) ([7c759be](https://www.github.com/googleapis/google-cloud-go/commit/7c759be074ce1f6b8ccce88c86dbe49bd38fd6b5))
* **bigquery:** support type alias names for numeric/bignumeric schemas. ([#3760](https://www.github.com/googleapis/google-cloud-go/issues/3760)) ([2ee6bf4](https://www.github.com/googleapis/google-cloud-go/commit/2ee6bf451524fc1f9735634320a55ca0b07d3d8b))

## v1.16.0

- Updates to various dependencies.

## [1.15.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.14.0...v1.15.0) (2021-01-14)


### Features

* **bigquery:** add reservation usage stats to query statistics ([#3403](https://www.github.com/googleapis/google-cloud-go/issues/3403)) ([112bcde](https://www.github.com/googleapis/google-cloud-go/commit/112bcdeb7cee1b44f337d3e5398a0d0820e93162))
* **bigquery:** add support for allowing Javascript UDFs to indicate determinism ([#3534](https://www.github.com/googleapis/google-cloud-go/issues/3534)) ([2f417a3](https://www.github.com/googleapis/google-cloud-go/commit/2f417a39d93402fbb1e5e3001645019782d7d656)), refs [#3533](https://www.github.com/googleapis/google-cloud-go/issues/3533)


### Bug Fixes

* **bigquery:** address possible panic due to offset checking in handleInsertErrors  ([#3524](https://www.github.com/googleapis/google-cloud-go/issues/3524)) ([5288511](https://www.github.com/googleapis/google-cloud-go/commit/52885115af3e95cdfd1ec784837fb1df7fe01446)), refs [#3519](https://www.github.com/googleapis/google-cloud-go/issues/3519)

## [1.14.0](https://www.github.com/googleapis/google-cloud-go/compare/bigquery/v1.13.0...v1.14.0) (2020-12-04)


### Features

* **bigquery:** add support for bignumeric ([#2779](https://www.github.com/googleapis/google-cloud-go/issues/2779)) ([ea3cde5](https://www.github.com/googleapis/google-cloud-go/commit/ea3cde55ad3d8d843bce8d023747cf69552850b5))
* **bigquery:** expose hive partitioning options ([#3240](https://www.github.com/googleapis/google-cloud-go/issues/3240)) ([fa77efa](https://www.github.com/googleapis/google-cloud-go/commit/fa77efa1a1880ff89307d54cc7e9e8c09430e4e2))

## v1.13.0

* Support retries for specific http2 transport race.
* Remove unused datasource client from bigquery/datatransfer.
* Adds support for authorized User Defined Functions (UDFs).
* Documentation improvements.
* Various updates to autogenerated clients.


## v1.12.0

* Adds additional retry support for table deletion.
* Various updates to autogenerated clients.

## v1.11.2

* Addresses issue with consuming query results using an iterator.Pager

## v1.11.1

* Addresses issue with optimized query path changes, released
  in v1.11.0

## v1.11.0

* Add support for optimized query path.
* Documentation improvements.
* Fix issue related to the ReturnType of a bigquery Routine.
* Various updates to autogenerated clients.

## v1.10.0

* Support for Infinity/-Infinity/NaN values in NullFloat64.
* Updates to RowIterator to address issues related to retrieving query
  results without explicit destination table references.
* Various updates to autogenerated clients.

## v1.9.0

* SchemaFromJSON will now accept alias type names (e.g. INT64 vs INTEGER, STRUCT vs RECORD).
* Support for IAM on table resources.
* Various updates to autogenerated clients.

## v1.8.0

* Add support for hourly time partitioning.
* Various updates to autogenerated clients.

## v1.7.0

* Add support for extracting BQML models to cloud storage.
* Add support for specifying projected fields when ingesting
  datastore backups.
* Fix issue related to defining a range partitioning range
  using default values.
* Add bigquery/reservation/v1 API.
* Various updates to autogenerated clients.

## v1.6.0

* Add support for materialized views.
* Add support for policy tags (column ACLs).
* Add bigquery/connection/v1beta1 API.
* Documentation improvements.
* Various updates to autogenerated clients.

## v1.5.0

* Add v1 endpoint for bigquerystorage API.
* Improved error message in bigquery.PutMultiError.
* Various updates to autogenerated clients.

## v1.4.0

* Add v1beta2, v1alpha2 endpoints for bigquerystorage API.

* Location is now reported as part of TableMetadata.

## v1.3.0

* Add Description field for Routine entities.

* Add support for iamMember entities on dataset ACLs.

* Address issue when constructing a Pager from a RowIterator
  that referenced a result with zero result rows.

* Add support for integer range partitioning, which affects
  table creation directly and via query/load jobs.

* Add opt-out support for streaming inserts via experimental
  `NoDedupeID` sentinel.

## v1.2.0

* Adds support for scripting feature, which includes script statistics
  and the ability to list jobs run as part of a script query.

* Updates default endpoint for BigQuery from www.googleapis.com
  to bigquery.googleapis.com.

## v1.1.0

* Added support for specifying default `EncryptionConfig` settings on the
  dataset.

* Added support for `EncyptionConfig` as part of an ML model.

* Added `Relax()` to make all fields within a `Schema` nullable.

* Added a `UseAvroLogicalTypes` option when defining an avro extract job.

## v1.0.1

This patch release is a small fix to the go.mod to point to the post-carve out
cloud.google.com/go.

## v1.0.0

This is the first tag to carve out bigquery as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
