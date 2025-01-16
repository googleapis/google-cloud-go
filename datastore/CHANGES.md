# Changes

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.19.0...datastore/v1.20.0) (2024-10-29)


### Features

* **datastore:** Add FindNearest API to the stable branch ([#10980](https://github.com/googleapis/google-cloud-go/issues/10980)) ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **datastore:** Support for field update operators in the Datastore API and resolution strategies when there is a conflict at write time ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Bug Fixes

* **datastore:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))
* **datastore:** Do not delay on final transaction attempt ([#10824](https://github.com/googleapis/google-cloud-go/issues/10824)) ([0d732cc](https://github.com/googleapis/google-cloud-go/commit/0d732cc87b432589f156c96d430e13c792dceeeb))
* **datastore:** Remove namespace from Key.String() ([40229e6](https://github.com/googleapis/google-cloud-go/commit/40229e65f574d63e2f31e5b15ec61d565db59fef))
* **datastore:** Remove namespace from Key.String() ([#10684](https://github.com/googleapis/google-cloud-go/issues/10684)) ([#10823](https://github.com/googleapis/google-cloud-go/issues/10823)) ([40229e6](https://github.com/googleapis/google-cloud-go/commit/40229e65f574d63e2f31e5b15ec61d565db59fef))
* **datastore:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **datastore:** Use local retryer in transactions ([#11050](https://github.com/googleapis/google-cloud-go/issues/11050)) ([3ef61a2](https://github.com/googleapis/google-cloud-go/commit/3ef61a29f10d34355d44830b5d473cacd6cf2be5))
* **datastore:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.18.0...datastore/v1.19.0) (2024-08-22)


### Features

* **datastore:** Reference new protos ([#10724](https://github.com/googleapis/google-cloud-go/issues/10724)) ([d8887df](https://github.com/googleapis/google-cloud-go/commit/d8887df4a12fe56606515c116fe1db12a6549aa1))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.17.1...datastore/v1.18.0) (2024-08-21)


### Features

* **datastore:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))
* **datastore:** Start generating datastorepb protos ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))


### Bug Fixes

* **datastore:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))
* **datastore:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))
* **datastore:** Ignore field mismatch errors ([#8694](https://github.com/googleapis/google-cloud-go/issues/8694)) ([6625d12](https://github.com/googleapis/google-cloud-go/commit/6625d12c3135587f188cc8f801beb9ae5a0c7515))
* **datastore:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))
* **datastore:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.17.0...datastore/v1.17.1) (2024-06-10)


### Bug Fixes

* **datastore:** Regenerate protos in new namespace ([#10158](https://github.com/googleapis/google-cloud-go/issues/10158)) ([8875511](https://github.com/googleapis/google-cloud-go/commit/8875511ca0c640d1260248f51c7b88e55136cdc6)), refs [#10155](https://github.com/googleapis/google-cloud-go/issues/10155)
* **datastore:** Update retry transaction logic to be inline with Spanner ([#10349](https://github.com/googleapis/google-cloud-go/issues/10349)) ([5929a6e](https://github.com/googleapis/google-cloud-go/commit/5929a6e67891b425d33128155af5cc76ecfc87a1))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.16.0...datastore/v1.17.0) (2024-05-08)


### Features

* **datastore:** Add query profiling ([#9200](https://github.com/googleapis/google-cloud-go/issues/9200)) ([b0235bb](https://github.com/googleapis/google-cloud-go/commit/b0235bb3828cea64b87cac3d1d84e42f1b2744ab))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.15.0...datastore/v1.16.0) (2024-04-29)


### Features

* **datastore:** Adding BeginLater and transaction state ([#8984](https://github.com/googleapis/google-cloud-go/issues/8984)) ([5f8e21f](https://github.com/googleapis/google-cloud-go/commit/5f8e21f84f0febd54e7ee6092ae6b88b269b0fc8))
* **datastore:** Adding BeginLater transaction option ([#8972](https://github.com/googleapis/google-cloud-go/issues/8972)) ([4067f4e](https://github.com/googleapis/google-cloud-go/commit/4067f4eabbba9e5e3265754339709fb273b73e4c))
* **datastore:** Adding reserve IDs support ([#9027](https://github.com/googleapis/google-cloud-go/issues/9027)) ([2d66de0](https://github.com/googleapis/google-cloud-go/commit/2d66de0c3004ca09d643c373d40e9e0f9e0f1aa5))
* **datastore:** Configure both mTLS and TLS endpoints for Datastore client ([#9653](https://github.com/googleapis/google-cloud-go/issues/9653)) ([38bd793](https://github.com/googleapis/google-cloud-go/commit/38bd7933f450a87886331429632fcb498a045675))
* **datastore:** Respect DATASTORE_EMULATOR_HOST setting ([#9789](https://github.com/googleapis/google-cloud-go/issues/9789)) ([7259373](https://github.com/googleapis/google-cloud-go/commit/7259373371e74cb4f7041f9397ba01dd9878b00b))


### Bug Fixes

* **datastore:** Add explicit sleep before read time use ([#9080](https://github.com/googleapis/google-cloud-go/issues/9080)) ([0538be4](https://github.com/googleapis/google-cloud-go/commit/0538be457518f7b86ffcabcbad35496f053f38cc))
* **datastore:** Adding tracing to run method ([#9602](https://github.com/googleapis/google-cloud-go/issues/9602)) ([a5e197c](https://github.com/googleapis/google-cloud-go/commit/a5e197c78cb112836768f012c7ee4535dec8b9f5))
* **datastore:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))
* **datastore:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))
* **datastore:** Prevent panic on GetMulti failure ([#9656](https://github.com/googleapis/google-cloud-go/issues/9656)) ([55845ad](https://github.com/googleapis/google-cloud-go/commit/55845ad9215191f1889b10d10fa72c9a42815d41))
* **datastore:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.14.0...datastore/v1.15.0) (2023-10-06)


### Features

* **datastore:** Adding dynamic routing header ([#8364](https://github.com/googleapis/google-cloud-go/issues/8364)) ([d235a42](https://github.com/googleapis/google-cloud-go/commit/d235a427a4e8d84466599cad4a68539a7a57a5db))


### Bug Fixes

* **datastore:** Allow saving nested byte slice ([#8540](https://github.com/googleapis/google-cloud-go/issues/8540)) ([8e53787](https://github.com/googleapis/google-cloud-go/commit/8e53787eac6f724ea4282533349abef3cbaffefe))
* **datastore:** Handle loading nil values ([#8544](https://github.com/googleapis/google-cloud-go/issues/8544)) ([25dbb9c](https://github.com/googleapis/google-cloud-go/commit/25dbb9cf1041d9e19edecd5c48b698b6f81f2d20))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.13.0...datastore/v1.14.0) (2023-08-22)


### Features

* **datastore:** SUM and AVG aggregations ([#8307](https://github.com/googleapis/google-cloud-go/issues/8307)) ([a9fff18](https://github.com/googleapis/google-cloud-go/commit/a9fff181e4ea8281ad907e7b2e0d90e70013a4de))
* **datastore:** Support aggregation query in transaction ([#8439](https://github.com/googleapis/google-cloud-go/issues/8439)) ([37681ff](https://github.com/googleapis/google-cloud-go/commit/37681ff291c0ccf4c908be55b97639c04b9dec48))


### Bug Fixes

* **datastore:** Correcting string representation of Key ([#8363](https://github.com/googleapis/google-cloud-go/issues/8363)) ([4cb1211](https://github.com/googleapis/google-cloud-go/commit/4cb12110ba229dfbe21568eb06c243bdffd1fee7))
* **datastore:** Fix NoIndex for array property ([#7674](https://github.com/googleapis/google-cloud-go/issues/7674)) ([01951e6](https://github.com/googleapis/google-cloud-go/commit/01951e64f3955dc337172a30d78e2f92f65becb2))


### Documentation

* **datastore/admin:** Specify limit for `properties` in `Index` message in Datastore Admin API ([b890425](https://github.com/googleapis/google-cloud-go/commit/b8904253a0f8424ea4548469e5feef321bd7396a))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.12.1...datastore/v1.13.0) (2023-07-26)


### Features

* **datastore:** Multi DB support ([#8276](https://github.com/googleapis/google-cloud-go/issues/8276)) ([e4d07a0](https://github.com/googleapis/google-cloud-go/commit/e4d07a0dddeab7fe635840f506daf01ceb18c067))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.12.0...datastore/v1.12.1) (2023-07-07)


### Bug Fixes

* **datastore:** Return error from RunAggregationQuery ([#8222](https://github.com/googleapis/google-cloud-go/issues/8222)) ([a9b67cf](https://github.com/googleapis/google-cloud-go/commit/a9b67cfc95b567d29358501ec7c5883b1f90bd3e))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.11.0...datastore/v1.12.0) (2023-06-27)


### Features

* **datastore:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))


### Bug Fixes

* **datastore:** Change aggregation result to return generic value ([#8167](https://github.com/googleapis/google-cloud-go/issues/8167)) ([9d3d17b](https://github.com/googleapis/google-cloud-go/commit/9d3d17bee90d010dab99a5a0f1610a777e55cc78))
* **datastore:** Handling nil slices in save and query ([#8043](https://github.com/googleapis/google-cloud-go/issues/8043)) ([36f01e9](https://github.com/googleapis/google-cloud-go/commit/36f01e99f75f4f07ae10991c52f45115b8180b45))
* **datastore:** PKG:datastore TYPE:datastoreClient FUNC:RunAggregationQuery ([#7803](https://github.com/googleapis/google-cloud-go/issues/7803)) ([1f050ea](https://github.com/googleapis/google-cloud-go/commit/1f050ea92782e7ec1ecb67fe134a89347a613351))
* **datastore:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))
* **datastore:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.10.0...datastore/v1.11.0) (2023-04-04)


### Features

* **datastore:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **datastore:** EntityFilter for AND/OR queries ([#7589](https://github.com/googleapis/google-cloud-go/issues/7589)) ([81f7c87](https://github.com/googleapis/google-cloud-go/commit/81f7c876d377b5a2dadf38bc811e5c71338a4b78))
* **datastore:** Return Get, GetMulti, Put and PutMulti errors with enhanced details ([#7061](https://github.com/googleapis/google-cloud-go/issues/7061)) ([c82b63a](https://github.com/googleapis/google-cloud-go/commit/c82b63ae9e2f24fff6f8c428c2444df679fed479))
* **datastore:** Rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))
* **datastore:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))


### Bug Fixes

* **datastore:** Adds nil check to AggregationQuery ([#7376](https://github.com/googleapis/google-cloud-go/issues/7376)) ([c43b9ed](https://github.com/googleapis/google-cloud-go/commit/c43b9ed31e8af07c1e8bcfa5db15ad3a83c96c50))


### Documentation

* **datastore/admin:** Reference the correct main client gem name ([1fb0c5e](https://github.com/googleapis/google-cloud-go/commit/1fb0c5e105dcae3a30b2e5b10ee47b84cbef8295))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.9.0...datastore/v1.10.0) (2022-11-29)


### Features

* **datastore:** start generating proto stubs ([eed371e](https://github.com/googleapis/google-cloud-go/commit/eed371e9b1639c81663c6858db119fb87a126454))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.8.0...datastore/v1.9.0) (2022-10-26)


### Features

* **datastore:** Adds COUNT aggregation query ([#6714](https://github.com/googleapis/google-cloud-go/issues/6714)) ([27363ca](https://github.com/googleapis/google-cloud-go/commit/27363ca581e3ae38d3eff0174727429838fcb4ac))
* **datastore:** Adds snapshot reads ([#6755](https://github.com/googleapis/google-cloud-go/issues/6755)) ([9240741](https://github.com/googleapis/google-cloud-go/commit/924074139a086aec7f12572d05909ee0b54e21f5))


### Documentation

* **datastore:** Adds emulator instructions ([#6928](https://github.com/googleapis/google-cloud-go/issues/6928)) ([553456a](https://github.com/googleapis/google-cloud-go/commit/553456a469662e8e14de13b55b4193740b21ff96))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/datastore-v1.7.0...datastore/v1.8.0) (2022-06-21)


### Features

* **datastore:** add better version metadata to calls ([d1ad921](https://github.com/googleapis/google-cloud-go/commit/d1ad921d0322e7ce728ca9d255a3cf0437d26add))
* **datastore:** adds in, not-in, and != query operators ([#6017](https://github.com/googleapis/google-cloud-go/issues/6017)) ([e926fb4](https://github.com/googleapis/google-cloud-go/commit/e926fb479c5ad9695ce50c1ee4a773a8330c6e66))
* **datastore:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/datastore/v1.6.0...datastore/v1.7.0) (2022-05-09)


### Features

* **datastore/admin:** define Datastore -> Firestore in Datastore mode migration long running operation metadata ([d9a0634](https://github.com/googleapis/google-cloud-go/commit/d9a0634042265f8c247e7dcbd8b85323a83c7235))
* **datastore:** add better version metadata to calls ([d1ad921](https://github.com/googleapis/google-cloud-go/commit/d1ad921d0322e7ce728ca9d255a3cf0437d26add))
* **datastore:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.6.0](https://www.github.com/googleapis/google-cloud-go/compare/datastore/v1.5.0...datastore/v1.6.0) (2021-09-17)


### Features

* **datastore/admin:** Publish message definitions for new Cloud Datastore migration logging steps. ([528ffc9](https://www.github.com/googleapis/google-cloud-go/commit/528ffc9bd63090129a8b1355cd31273f8c23e34c))


### Bug Fixes

* **datastore:** Initialize commit sentinel to avoid cross use of commits ([#4599](https://www.github.com/googleapis/google-cloud-go/issues/4599)) ([fcf13b0](https://www.github.com/googleapis/google-cloud-go/commit/fcf13b0abad4f837d4f4f53fad6c55eba1a0fe56))

## [1.5.0](https://www.github.com/googleapis/google-cloud-go/compare/v1.4.0...v1.5.0) (2021-03-01)


### Features

* **datastore/admin:** Added methods for creating and deleting composite indexes feat: Populated php_namespace ([529925b](https://www.github.com/googleapis/google-cloud-go/commit/529925ba79f4d3191ef80a13e566d86210fe4d25))
* **datastore/admin:** Publish message definitions for Cloud Datastore migration logging. ([529925b](https://www.github.com/googleapis/google-cloud-go/commit/529925ba79f4d3191ef80a13e566d86210fe4d25))

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/datastore/v1.3.0...v1.4.0) (2021-01-15)


### Features

* **datastore:** add opencensus tracing/stats support ([#2804](https://www.github.com/googleapis/google-cloud-go/issues/2804)) ([5e6c350](https://www.github.com/googleapis/google-cloud-go/commit/5e6c350b2ac94787934380e930af2cb2094fa8f1))
* **datastore:** support civil package types save ([#3202](https://www.github.com/googleapis/google-cloud-go/issues/3202)) ([9cc1a66](https://www.github.com/googleapis/google-cloud-go/commit/9cc1a66e22ecd8dcad1235c290f05b92edff5aa0))


### Bug Fixes

* **datastore:** Ensure the datastore time is returned as UTC ([#3521](https://www.github.com/googleapis/google-cloud-go/issues/3521)) ([0e659e2](https://www.github.com/googleapis/google-cloud-go/commit/0e659e28da503b9520c83eb136df6e54d6c6daf7))
* **datastore:** increase deferred key iter limit ([#2878](https://www.github.com/googleapis/google-cloud-go/issues/2878)) ([7f1057a](https://www.github.com/googleapis/google-cloud-go/commit/7f1057a30d3b8691a22c85255bb41d31d42c6f9c))
* **datastore:** loading civil types in non UTC location is incorrect ([#3376](https://www.github.com/googleapis/google-cloud-go/issues/3376)) ([9ac287d](https://www.github.com/googleapis/google-cloud-go/commit/9ac287d2abfb6bdcdceabb67fa0d93fb7b0dd863))

## v1.3.0
- Fix saving behavior for non-struct custom types which implement
  `PropertyLoadSaver` and for nil interface types.
- Support `DetectProjectID` when using the emulator.

## v1.2.0
- Adds Datastore Admin API.
- Documentation updates.

## v1.1.0

- DEADLINE_EXCEEDED is now not retried.
- RunInTransaction now panics more explicitly on a nil TransactionOption.
- PropertyLoadSaver now tries to Load as much as possible (e.g., Key), even if an error is returned.
- Client now uses transport/grpc.DialPool rather than Dial.
  - Connection pooling now does not use the deprecated (and soon to be removed) gRPC load balancer API.
- Doc updates
  - Iterator is unsafe for concurrent use.
  - Mutation docs now describe atomicity and gRPC error codes more explicitly.
  - Cursor example now correctly uses "DecodeCursor" rather than "NewCursor"

## v1.0.0

This is the first tag to carve out datastore as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
