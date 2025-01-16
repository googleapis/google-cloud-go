# Changes

## [1.8.2](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.8.1...pubsublite/v1.8.2) (2024-06-05)


### Bug Fixes

* **pubsublite:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))
* **pubsublite:** Fix int conversion ([9221c7f](https://github.com/googleapis/google-cloud-go/commit/9221c7fa12cef9d5fb7ddc92f41f1d6204971c7b))
* **pubsublite:** Fix regional endpoints ([#9362](https://github.com/googleapis/google-cloud-go/issues/9362)) ([14ce978](https://github.com/googleapis/google-cloud-go/commit/14ce9787f2a53ebdb25abea978367d7f8ae00188))
* **pubsublite:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.8.0...pubsublite/v1.8.1) (2023-05-11)


### Bug Fixes

* **pubsublite:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.7.1...pubsublite/v1.8.0) (2023-04-28)


### Features

* **pubsublite:** Expose functions to encode/decode message event times ([#7853](https://github.com/googleapis/google-cloud-go/issues/7853)) ([89bf810](https://github.com/googleapis/google-cloud-go/commit/89bf8106fdd12025abe3cc54484b7bcf084266a0))


### Documentation

* **pubsublite:** Associate examples with the correct function ([#7827](https://github.com/googleapis/google-cloud-go/issues/7827)) ([481b161](https://github.com/googleapis/google-cloud-go/commit/481b161d2fe5fbf2b6fb306b9153b2c174e75991))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.7.0...pubsublite/v1.7.1) (2023-04-21)


### Bug Fixes

* **pubsublite:** Enforce minimum publisher and subscriber timeout of 2 minutes ([#7746](https://github.com/googleapis/google-cloud-go/issues/7746)) ([89a9b0b](https://github.com/googleapis/google-cloud-go/commit/89a9b0b40fd2d4b1cf256b6c90a2108a62c44bed))


### Documentation

* **pubsublite:** Example for configuring earlier OAuth token refresh ([#7745](https://github.com/googleapis/google-cloud-go/issues/7745)) ([290d637](https://github.com/googleapis/google-cloud-go/commit/290d637e05401340677b77daa504ddc975fc0901))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.6.0...pubsublite/v1.7.0) (2023-03-20)


### Features

* **pubsublite:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **pubsublite:** API for publish idempotence ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))
* **pubsublite:** Publish idempotence ([#7390](https://github.com/googleapis/google-cloud-go/issues/7390)) ([8df979e](https://github.com/googleapis/google-cloud-go/commit/8df979eb7d9591290ec3c4427d11d817d8bc2e1a))
* **pubsublite:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.5.0...pubsublite/v1.6.0) (2022-12-10)


### Features

* **pubsublite:** Create/update export subscriptions ([#6885](https://github.com/googleapis/google-cloud-go/issues/6885)) ([5fa8555](https://github.com/googleapis/google-cloud-go/commit/5fa855545502ab01775d19cc7b42810beefd1d5f))
* **pubsublite:** rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))
* **pubsublite:** Unload idle partition publishers ([#7105](https://github.com/googleapis/google-cloud-go/issues/7105)) ([176f533](https://github.com/googleapis/google-cloud-go/commit/176f5331ff02dd9ae4eb706f299b31c903689298))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.4.1...pubsublite/v1.5.0) (2022-12-01)


### Features

* **pubsublite:** Add current state of export subscriptions to API ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **pubsublite:** Remove obsolete export subscription statuses field from API ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))
* **pubsublite:** Set finalizer for PublisherClient ([#7109](https://github.com/googleapis/google-cloud-go/issues/7109)) ([e648bd9](https://github.com/googleapis/google-cloud-go/commit/e648bd95ff5b33383440e18245106741292ac97a))
* **pubsublite:** start generating proto stubs ([cf89415](https://github.com/googleapis/google-cloud-go/commit/cf894154e451a32b431fef2af3781a0d2d8080ff))

## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.4.0...pubsublite/v1.4.1) (2022-10-18)


### Bug Fixes

* **pubsublite:** Close api clients when publisher clients have terminated ([#6867](https://github.com/googleapis/google-cloud-go/issues/6867)) ([5cb5662](https://github.com/googleapis/google-cloud-go/commit/5cb5662ff28153e6764e54ef7245f000f0379e5a))


### Documentation

* **pubsublite:** Update publisher and subscriber client usage ([#6864](https://github.com/googleapis/google-cloud-go/issues/6864)) ([f9eb454](https://github.com/googleapis/google-cloud-go/commit/f9eb45439b0fa6e9cffdbd06eac0c540e3ed8db6))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.3.2...pubsublite/v1.4.0) (2022-10-17)


### Features

* **pubsublite:** Add export config protos to API ([41ab4ec](https://github.com/googleapis/google-cloud-go/commit/41ab4ec00552931b12f61a9fcb27b36a7c0b5d77))


### Bug Fixes

* **pubsublite:** set pubsublite back to grpc-only instead of grpc+rest ([199b725](https://github.com/googleapis/google-cloud-go/commit/199b7250f474b1a6f53dcf0aac0c2966f4987b68))

## [1.3.2](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.3.1...pubsublite/v1.3.2) (2022-06-15)


### Bug Fixes

* **pubsublite:** fixed version in stream headers ([#6181](https://github.com/googleapis/google-cloud-go/issues/6181)) ([25a2ae3](https://github.com/googleapis/google-cloud-go/commit/25a2ae384b6383b9ac73d600e7e9f34d10979e31))

### [1.3.1](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.3.0...pubsublite/v1.3.1) (2022-04-27)


### Features

* **pubsublite:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))


### Bug Fixes

* **pubsublite:** retry Cancelled stream errors ([#5943](https://github.com/googleapis/google-cloud-go/issues/5943)) ([bbee3d5](https://github.com/googleapis/google-cloud-go/commit/bbee3d54dc754f424abe48a5a4eed7eac13770b6))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/pubsublite/v1.2.2...pubsublite/v1.3.0) (2022-02-15)


### Features

* **pubsublite:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))


### Bug Fixes

* **pubsublite:** use pubsublite internal version ([#5618](https://github.com/googleapis/google-cloud-go/issues/5618)) ([0156450](https://github.com/googleapis/google-cloud-go/commit/01564507f57829e7b68f5db3d94acc1ca2ddc90a))

### [1.2.2](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v1.2.1...pubsublite/v1.2.2) (2022-02-08)


### Features

* **pubsublite:** add C++ rules for Pub/Sub Lite ([3e7185c](https://www.github.com/googleapis/google-cloud-go/commit/3e7185c241d97ee342f132ae04bc93bb79a8e897))


### Bug Fixes

* **pubsublite:** mitigate gRPC stream connection issues ([#5382](https://www.github.com/googleapis/google-cloud-go/issues/5382)) ([8763ef3](https://www.github.com/googleapis/google-cloud-go/commit/8763ef3d2da18e8fed6e350aef76d26a135246c2))


### Documentation

* **pubsublite:** update comments for regional topics ([#5202](https://www.github.com/googleapis/google-cloud-go/issues/5202)) ([7805468](https://www.github.com/googleapis/google-cloud-go/commit/78054682770af3b35cb0002c3c34006ec36590ef))

### [1.2.1](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v1.2.0...pubsublite/v1.2.1) (2021-10-26)


### Bug Fixes

* **pubsublite:** disable grpc stream retries ([#5019](https://www.github.com/googleapis/google-cloud-go/issues/5019)) ([74f9c11](https://www.github.com/googleapis/google-cloud-go/commit/74f9c112eadb83fea7b759f37ddb8ced9317f238))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v1.1.0...pubsublite/v1.2.0) (2021-10-01)


### Features

* **pubsublite:** notify subscriber clients on partition reassignment ([#4777](https://www.github.com/googleapis/google-cloud-go/issues/4777)) ([393b0a3](https://www.github.com/googleapis/google-cloud-go/commit/393b0a39bf917a5bade854dddeb278aa95f9d3f0))
* **pubsublite:** support reservations in AdminClient ([#4294](https://www.github.com/googleapis/google-cloud-go/issues/4294)) ([65b0f88](https://www.github.com/googleapis/google-cloud-go/commit/65b0f88a78d8833bcaaf8fc59401ec0a1527db1d))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v1.0.0...pubsublite/v1.1.0) (2021-08-09)


### Features

* **pubsublite:** support seek subscription in AdminClient ([#4316](https://www.github.com/googleapis/google-cloud-go/issues/4316)) ([2dea319](https://www.github.com/googleapis/google-cloud-go/commit/2dea3196a73764bd10842a3da5d0fa29ae84e101))


### Bug Fixes

* **pubsublite:** set a default grpc connection pool size of 8 ([#4462](https://www.github.com/googleapis/google-cloud-go/issues/4462)) ([b7ce742](https://www.github.com/googleapis/google-cloud-go/commit/b7ce742db1acdd18b5a597ebb2a2111953c0942a))

## [1.0.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.10.2...pubsublite/v1.0.0) (2021-07-08)


### Bug Fixes

* **pubsublite:** hide CreateSubscriptionOption.apply ([#4344](https://www.github.com/googleapis/google-cloud-go/issues/4344)) ([f31fac6](https://www.github.com/googleapis/google-cloud-go/commit/f31fac6c2674a1bb9180a75ae7dbeda55721482d))
* **pubsublite:** lower gRPC keepalive timeouts ([#4378](https://www.github.com/googleapis/google-cloud-go/issues/4378)) ([35d98c8](https://www.github.com/googleapis/google-cloud-go/commit/35d98c8cad3a71400c2b47218a0fb9c80154e613))


### Documentation

* **pubsublite:** promote pubsublite to stable ([#4301](https://www.github.com/googleapis/google-cloud-go/issues/4301)) ([c841d7f](https://www.github.com/googleapis/google-cloud-go/commit/c841d7feb48fc66e90ec7e63f35002712d5e6dbf))

### [0.10.2](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.10.1...pubsublite/v0.10.2) (2021-06-29)


### Bug Fixes

* **pubsublite:** ensure timeout settings are respected ([#4329](https://www.github.com/googleapis/google-cloud-go/issues/4329)) ([e75262c](https://www.github.com/googleapis/google-cloud-go/commit/e75262cf5eba845271965eab3c28c0a23bec14c4))
* **pubsublite:** wire user context to api clients ([#4318](https://www.github.com/googleapis/google-cloud-go/issues/4318)) ([ae34396](https://www.github.com/googleapis/google-cloud-go/commit/ae34396b1a2a970a0d871cd5496527294f3310d4))

### [0.10.1](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.10.0...pubsublite/v0.10.1) (2021-06-22)


### Bug Fixes

* **pubsublite:** fixes for background partition count updates ([#4293](https://www.github.com/googleapis/google-cloud-go/issues/4293)) ([634847b](https://www.github.com/googleapis/google-cloud-go/commit/634847b7499fb58575e3e5001dd8e6da0661fccd))
* **pubsublite:** make SubscriberClient.Receive identical to pubsub ([#4281](https://www.github.com/googleapis/google-cloud-go/issues/4281)) ([5b5d0f7](https://www.github.com/googleapis/google-cloud-go/commit/5b5d0f782b224f324dcfa13cc4145ee33a395d09))

## [0.10.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.9.1...pubsublite/v0.10.0) (2021-06-15)


### Features

* **pubsublite:** support out of band seeks ([#4208](https://www.github.com/googleapis/google-cloud-go/issues/4208)) ([1432e67](https://www.github.com/googleapis/google-cloud-go/commit/1432e678d5510f6a60b5319e7c70b0c15229b88c))


### Bug Fixes

* **pubsublite:** ack assignment after removed subscribers have terminated ([#4217](https://www.github.com/googleapis/google-cloud-go/issues/4217)) ([0ad3f16](https://www.github.com/googleapis/google-cloud-go/commit/0ad3f168b8525033e6926882059cb0b430d1f350))

### [0.9.1](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.9.0...pubsublite/v0.9.1) (2021-06-10)


### Bug Fixes

* **pubsublite:** ensure api clients are closed when startup fails ([#4239](https://www.github.com/googleapis/google-cloud-go/issues/4239)) ([55025a1](https://www.github.com/googleapis/google-cloud-go/commit/55025a1c6abe0ef4e57dd31347265aab3b78bdf8))

## [0.9.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.8.0...pubsublite/v0.9.0) (2021-06-08)


### Features

* **pubsublite:** Add initial_cursor field to InitialSubscribeRequest ([6f9c8b0](https://www.github.com/googleapis/google-cloud-go/commit/6f9c8b0a5d6e4509f056a146cb586f310f3336a9))
* **pubsublite:** Add Pub/Sub Lite Reservation APIs ([18375e5](https://www.github.com/googleapis/google-cloud-go/commit/18375e50e8f16e63506129b8927a7b62f85e407b))
* **pubsublite:** ComputeTimeCursor RPC for Pub/Sub Lite ([d089dda](https://www.github.com/googleapis/google-cloud-go/commit/d089dda0089acb9aaef9b3da40b219476af9fc06))
* **pubsublite:** detect stream reset signal ([#4144](https://www.github.com/googleapis/google-cloud-go/issues/4144)) ([ff5f8c9](https://www.github.com/googleapis/google-cloud-go/commit/ff5f8c989cba2751dcc77745483ef3828e6df78c))
* **pubsublite:** flush and reset committer ([#4143](https://www.github.com/googleapis/google-cloud-go/issues/4143)) ([0ecd732](https://www.github.com/googleapis/google-cloud-go/commit/0ecd732e3f57928e7999ae4e78871be070c184d9))


### Bug Fixes

* **pubsublite:** prevent subscriber flow control token races ([#4060](https://www.github.com/googleapis/google-cloud-go/issues/4060)) ([dc0103b](https://www.github.com/googleapis/google-cloud-go/commit/dc0103baeaf168474b9e163f0aa5f7555170ffc4))

## [0.8.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.7.0...pubsublite/v0.8.0) (2021-03-25)


### Features

* **pubsublite:** add skip_backlog field to allow subscriptions to be created at HEAD ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **pubsublite:** adding ability to create subscriptions at head ([#3790](https://www.github.com/googleapis/google-cloud-go/issues/3790)) ([bc083b6](https://www.github.com/googleapis/google-cloud-go/commit/bc083b66972b1c4329c18da9529c76b79ef56c50))


### Bug Fixes

* **pubsublite:** ackTracker should discard new acks after committer terminates ([#3827](https://www.github.com/googleapis/google-cloud-go/issues/3827)) ([bc49753](https://www.github.com/googleapis/google-cloud-go/commit/bc497531a9918f2e3bc9f1895ddd49011427e388))
* **pubsublite:** fix committer races ([#3810](https://www.github.com/googleapis/google-cloud-go/issues/3810)) ([d8689f1](https://www.github.com/googleapis/google-cloud-go/commit/d8689f1d32be83f9bbbacb9dd24ce085d81d79e8))
* **pubsublite:** improve handling of backend unavailability ([#3846](https://www.github.com/googleapis/google-cloud-go/issues/3846)) ([db31457](https://www.github.com/googleapis/google-cloud-go/commit/db31457cebdcd1c6370953e0360acd227567496d))
* **pubsublite:** increase default timeouts for publish and subscribe stream connections ([#3821](https://www.github.com/googleapis/google-cloud-go/issues/3821)) ([df28999](https://www.github.com/googleapis/google-cloud-go/commit/df28999076fa91939038c06a706fc63811b20932))
* **pubsublite:** remove publish error translation ([#3843](https://www.github.com/googleapis/google-cloud-go/issues/3843)) ([d8d8f68](https://www.github.com/googleapis/google-cloud-go/commit/d8d8f68e8a70e2353048578f5d22fa1cd2ca6482))

## [0.7.0](https://www.github.com/googleapis/google-cloud-go/compare/v0.6.0...v0.7.0) (2021-02-18)

The status of this library is now **BETA**.

### Features

* **pubsublite:** allow increasing the number of topic partitions ([#3647](https://www.github.com/googleapis/google-cloud-go/issues/3647)) ([1f85fdc](https://www.github.com/googleapis/google-cloud-go/commit/1f85fdca9f4317fab0f18b8bd9fcc8c65ab690e9))


### Bug Fixes

* **pubsublite:** change pubsub.Message.ID to an encoded publish.Metadata ([#3662](https://www.github.com/googleapis/google-cloud-go/issues/3662)) ([6b2807f](https://www.github.com/googleapis/google-cloud-go/commit/6b2807f1e13dc38eb79833f8d2766f27d4003434))
* **pubsublite:** rebatch messages upon new publish stream ([#3694](https://www.github.com/googleapis/google-cloud-go/issues/3694)) ([0da3578](https://www.github.com/googleapis/google-cloud-go/commit/0da3578c8f007f71291cdc93d43f98acbe1dbb37))
* **pubsublite:** rename publish.Metadata to pscompat.MessageMetadata ([#3672](https://www.github.com/googleapis/google-cloud-go/issues/3672)) ([6a8d4c5](https://www.github.com/googleapis/google-cloud-go/commit/6a8d4c515eb957d05e280e02e8cea9a89bdcbb1e))

## [0.6.0](https://www.github.com/googleapis/google-cloud-go/compare/v0.5.0...v0.6.0) (2021-01-28)


### ⚠ API Changes

* **pubsublite:** add separate publisher and subscriber client constructors with settings ([#3528](https://www.github.com/googleapis/google-cloud-go/issues/3528)) ([98637e0](https://www.github.com/googleapis/google-cloud-go/commit/98637e089776292232bb7c039844680627ddade1))
* **pubsublite:** rename package ps to pscompat ([#3569](https://www.github.com/googleapis/google-cloud-go/issues/3569)) ([9d8fd2b](https://www.github.com/googleapis/google-cloud-go/commit/9d8fd2b5e6999657bcf324878732da801b805591))
* **pubsublite:** rename AdminClient TopicPartitions to TopicPartitionCount ([#3565](https://www.github.com/googleapis/google-cloud-go/issues/3565)) ([86a4de7](https://www.github.com/googleapis/google-cloud-go/commit/86a4de757bc2eed97577aba7fd51b5f5540e097e))
* **pubsublite:** use strings for resource paths ([#3559](https://www.github.com/googleapis/google-cloud-go/issues/3559)) ([c18ed25](https://www.github.com/googleapis/google-cloud-go/commit/c18ed25900ba41e0b6b98a89cec8615df6a1146c))

### Bug Fixes

* **pubsublite:** close clients after publisher and subscriber have terminated ([#3512](https://www.github.com/googleapis/google-cloud-go/issues/3512)) ([72d2aff](https://www.github.com/googleapis/google-cloud-go/commit/72d2affb957cea7b6a223b108d0fe67c5635b25c))
* **pubsublite:** ignore outstanding acks for unassigned partition subscribers ([#3597](https://www.github.com/googleapis/google-cloud-go/issues/3597)) ([eb91f1f](https://www.github.com/googleapis/google-cloud-go/commit/eb91f1f3c96f4c868e523f3c43f8c22b10ad4de4))

## [0.5.0](https://www.github.com/googleapis/google-cloud-go/compare/v0.4.0...v0.5.0) (2021-01-07)


### Features

* **pubsublite:** add client library metadata to headers ([#3458](https://www.github.com/googleapis/google-cloud-go/issues/3458)) ([8226811](https://www.github.com/googleapis/google-cloud-go/commit/822681105bc13f1e1f0784c4557faf849c1110b4))
* **pubsublite:** publisher client ([#3303](https://www.github.com/googleapis/google-cloud-go/issues/3303)) ([1648ea0](https://www.github.com/googleapis/google-cloud-go/commit/1648ea06bbb08c3452f79551a9d45147379f13e4))
* **pubsublite:** settings and message transforms for Cloud Pub/Sub shim ([#3281](https://www.github.com/googleapis/google-cloud-go/issues/3281)) ([74923c2](https://www.github.com/googleapis/google-cloud-go/commit/74923c27efd7936b3e18cd8ccb72882a40c7ff42))
* **pubsublite:** subscriber client ([#3442](https://www.github.com/googleapis/google-cloud-go/issues/3442)) ([221bfba](https://www.github.com/googleapis/google-cloud-go/commit/221bfbae54107486ab9060b950081faa27489d1c))


### Bug Fixes

* **pubsublite:** return an error if no topic or subscription fields were updated ([#3502](https://www.github.com/googleapis/google-cloud-go/issues/3502)) ([a875969](https://www.github.com/googleapis/google-cloud-go/commit/a87596942d39fbfe47427c007e4029bd9be2ca0e))

## [0.4.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.3.0...v0.4.0) (2020-12-09)


### Features

pubsublite/internal/wire implementation:

* **pubsublite:** assigning subscriber ([#3238](https://www.github.com/googleapis/google-cloud-go/issues/3238)) ([d1c03da](https://www.github.com/googleapis/google-cloud-go/commit/d1c03dae383f5a175e4237d5f46dc1bdc2cd33f0))
* **pubsublite:** committer ([#3198](https://www.github.com/googleapis/google-cloud-go/issues/3198)) ([ecc706b](https://www.github.com/googleapis/google-cloud-go/commit/ecc706b03079c6521a31e1066b00677aaf51e7dd))
* **pubsublite:** receive settings ([#3195](https://www.github.com/googleapis/google-cloud-go/issues/3195)) ([bd837fc](https://www.github.com/googleapis/google-cloud-go/commit/bd837fc9aad4181b8aa574e41341000755875eca))
* **pubsublite:** routing publisher ([#3277](https://www.github.com/googleapis/google-cloud-go/issues/3277)) ([88e5466](https://www.github.com/googleapis/google-cloud-go/commit/88e546600c7d4f7570530aa72355f51f44187890))
* **pubsublite:** single and multi partition subscribers ([#3221](https://www.github.com/googleapis/google-cloud-go/issues/3221)) ([299b803](https://www.github.com/googleapis/google-cloud-go/commit/299b803aaee9a0dc0b2ec8c81fac66341045b8b2))
* **pubsublite:** single partition publisher ([#3225](https://www.github.com/googleapis/google-cloud-go/issues/3225)) ([4982eeb](https://www.github.com/googleapis/google-cloud-go/commit/4982eeb32ebe85de211ae09d13fdaf6140d9e115))


### Bug Fixes

* **pubsublite:** fixed return value of AdminClient.TopicSubscriptions ([#3220](https://www.github.com/googleapis/google-cloud-go/issues/3220)) ([f37f118](https://www.github.com/googleapis/google-cloud-go/commit/f37f118c87d4d0a77a554515a430ae06e5852294))

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/pubsublite/v0.2.0...v0.3.0) (2020-11-10)


### Features

* **pubsublite:** Added Pub/Sub Lite clients and routing headers ([#3105](https://www.github.com/googleapis/google-cloud-go/issues/3105)) ([98668fa](https://www.github.com/googleapis/google-cloud-go/commit/98668fa5457d26ed34debee708614f027020e5bc))
* **pubsublite:** Flow controller and offset tracker for the subscriber ([#3132](https://www.github.com/googleapis/google-cloud-go/issues/3132)) ([5899bdd](https://www.github.com/googleapis/google-cloud-go/commit/5899bdd7d6d5eac96e42e1baa1bd5e905e767a17))
* **pubsublite:** Mock server and utils for unit tests ([#3092](https://www.github.com/googleapis/google-cloud-go/issues/3092)) ([586592e](https://www.github.com/googleapis/google-cloud-go/commit/586592ef5875667e65e19e3662fe532b26293172))
* **pubsublite:** Move internal implementation details to internal/wire subpackage ([#3123](https://www.github.com/googleapis/google-cloud-go/issues/3123)) ([ed3fd1a](https://www.github.com/googleapis/google-cloud-go/commit/ed3fd1aed7dbc9396aecc70622ccfd302bbb4265))
* **pubsublite:** Periodic background task ([#3152](https://www.github.com/googleapis/google-cloud-go/issues/3152)) ([58c12cc](https://www.github.com/googleapis/google-cloud-go/commit/58c12ccba01cfe3b320e2e83d7ca1145f1e310d7))
* **pubsublite:** Test utils for streams ([#3153](https://www.github.com/googleapis/google-cloud-go/issues/3153)) ([5bb2b02](https://www.github.com/googleapis/google-cloud-go/commit/5bb2b0218d355bc558b03f24db1a0786a3489cac))
* **pubsublite:** Trackers for acks and commit cursor ([#3137](https://www.github.com/googleapis/google-cloud-go/issues/3137)) ([26599a0](https://www.github.com/googleapis/google-cloud-go/commit/26599a0995d9b108bbaaceca775457ffc331dcb2))

## v0.2.0

- Features
  - feat(pubsublite): Types for resource paths and topic/subscription configs (#3026)
  - feat(pubsublite): Pub/Sub Lite admin client (#3036)

## v0.1.0

This is the first tag to carve out pubsublite as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.

