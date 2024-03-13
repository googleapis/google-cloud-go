# Changes

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.14.0...firestore/v1.15.0) (2024-03-05)


### Features

* **firestore/apiv1:** Add DeleteDatabase API and delete protection ([#9185](https://github.com/googleapis/google-cloud-go/issues/9185)) ([ec9b526](https://github.com/googleapis/google-cloud-go/commit/ec9b5268627734c40efd15353cf4bc83a837ff3a))
* **firestore/apiv1:** Expose Firestore PITR fields in Database to stable ([5132d0f](https://github.com/googleapis/google-cloud-go/commit/5132d0fea3a5ac902a2c9eee865241ed4509a5f4))
* **firestore:** Add new types QueryMode, QueryPlan, ResultSetStats ([82054d0](https://github.com/googleapis/google-cloud-go/commit/82054d0e6905358e48517cbc8ea844dfb624082c))


### Bug Fixes

* **firestore:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))
* **firestore:** Correct the cursors when LimitToLast is used ([#9413](https://github.com/googleapis/google-cloud-go/issues/9413)) ([2090651](https://github.com/googleapis/google-cloud-go/commit/2090651b4a7a1dc3be5af4e7ac4607fbc3ffccac))
* **firestore:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))
* **firestore:** Remove types QueryMode, QueryPlan, ResultSetStats ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))
* **firestore:** Return status code from bulkwriter results ([#9030](https://github.com/googleapis/google-cloud-go/issues/9030)) ([e8223c6](https://github.com/googleapis/google-cloud-go/commit/e8223c6ee544237b54b351e421b7092dc3b237a6))
* **firestore:** Update grpc-go to v1.56.3 ([343cea8](https://github.com/googleapis/google-cloud-go/commit/343cea8c43b1e31ae21ad50ad31d3b0b60143f8c))
* **firestore:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))


### Documentation

* **firestore/apiv1:** Fix formatting due to unclosed backtick ([0500c7a](https://github.com/googleapis/google-cloud-go/commit/0500c7a7f9a9e8629a091558fa258ca7c5028474))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.13.0...firestore/v1.14.0) (2023-10-19)


### Features

* **firestore:** SUM and AVG aggregations ([#8293](https://github.com/googleapis/google-cloud-go/issues/8293)) ([011f9ff](https://github.com/googleapis/google-cloud-go/commit/011f9ff083bebad5c30443b3b0fd9df68579a65b))


### Bug Fixes

* **firestore:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.12.0...firestore/v1.13.0) (2023-09-18)


### Features

* **firestore:** Support for multiple databases ([#5331](https://github.com/googleapis/google-cloud-go/issues/5331)) ([94d4b1b](https://github.com/googleapis/google-cloud-go/commit/94d4b1b58d2c8f3dac18e7efb0be641b6311c775))


### Bug Fixes

* **firestore:** Compare full resource path when docs ordered by __name__ ([#8409](https://github.com/googleapis/google-cloud-go/issues/8409)) ([5ef93de](https://github.com/googleapis/google-cloud-go/commit/5ef93de226b854bdf6277b7f906b86755a07d229))
* **firestore:** Correcting EndBefore with LimitToLast behaviour ([#8370](https://github.com/googleapis/google-cloud-go/issues/8370)) ([350f7ad](https://github.com/googleapis/google-cloud-go/commit/350f7adb2a087811a70f1c05bf71014022aefeb4))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.11.0...firestore/v1.12.0) (2023-08-01)


### Features

* **firestore:** Publish proto definitions for SUM/AVG in Firestore ([e3f8c89](https://github.com/googleapis/google-cloud-go/commit/e3f8c89429a207c05fee36d5d93efe76f9e29efe))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.10.0...firestore/v1.11.0) (2023-06-26)


### Features

* **firestore:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))


### Bug Fixes

* **firestore:** Cleanup integration test resources ([#8057](https://github.com/googleapis/google-cloud-go/issues/8057)) ([210584d](https://github.com/googleapis/google-cloud-go/commit/210584df494e9627dd13c24138fcbebe85048647))
* **firestore:** Do not trace iterator.Done error ([#8082](https://github.com/googleapis/google-cloud-go/issues/8082)) ([5f24d17](https://github.com/googleapis/google-cloud-go/commit/5f24d173db35358d241de186953cd094dae312c9)), refs [#7711](https://github.com/googleapis/google-cloud-go/issues/7711)
* **firestore:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.9.0...firestore/v1.10.0) (2023-05-22)


### Features

* **firestore:** Add `OR` query support docs: Improve the API documentation for the `Firestore.ListDocuments` RPC docs: Minor documentation formatting and cleanup ([aeb6fec](https://github.com/googleapis/google-cloud-go/commit/aeb6fecc7fd3f088ff461a0c068ceb9a7ae7b2a3))
* **firestore:** Add bloom filter related proto fields PiperOrigin-RevId: 529511263 ([31c3766](https://github.com/googleapis/google-cloud-go/commit/31c3766c9c4cab411669c14fc1a30bd6d2e3f2dd))
* **firestore:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **firestore:** Added support for REST transport ([aeb6fec](https://github.com/googleapis/google-cloud-go/commit/aeb6fecc7fd3f088ff461a0c068ceb9a7ae7b2a3))
* **firestore:** EntityFilter for AND/OR queries ([#7757](https://github.com/googleapis/google-cloud-go/issues/7757)) ([ae37793](https://github.com/googleapis/google-cloud-go/commit/ae377932de20d99f31766ca1cccd2d1cfa18a1c0))
* **firestore:** Rewrite signatures and type in terms of new location ([620e6d8](https://github.com/googleapis/google-cloud-go/commit/620e6d828ad8641663ae351bfccfe46281e817ad))
* **firestore:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))


### Bug Fixes

* **firestore:** Enable rest_numeric_enums for PHP client ([2fef56f](https://github.com/googleapis/google-cloud-go/commit/2fef56f75a63dc4ff6e0eea56c7b26d4831c8e27))
* **firestore:** Replace usage of transform with update_transform in batch write  ([#7864](https://github.com/googleapis/google-cloud-go/issues/7864)) ([949e4d8](https://github.com/googleapis/google-cloud-go/commit/949e4d8001040e78f2ad9b1e5cbf5b9113d8f3ef))
* **firestore:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.8.0...firestore/v1.9.0) (2022-11-29)


### Features

* **firestore:** start generating proto stubs ([eed371e](https://github.com/googleapis/google-cloud-go/commit/eed371e9b1639c81663c6858db119fb87a126454))


### Documentation

* **firestore:** Adds emulator snippet ([#6926](https://github.com/googleapis/google-cloud-go/issues/6926)) ([456afab](https://github.com/googleapis/google-cloud-go/commit/456afab76f078ef58b7e5b3409acc6b3f71c5b79))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.7.0...firestore/v1.8.0) (2022-10-17)


### Features

* **firestore:** Adds COUNT aggregation query ([#6692](https://github.com/googleapis/google-cloud-go/issues/6692)) ([31ac692](https://github.com/googleapis/google-cloud-go/commit/31ac692d925065981a695266d1e4e22e5374725e))
* **firestore:** Adds snapshot reads impl. ([#6718](https://github.com/googleapis/google-cloud-go/issues/6718)) ([43cc5bc](https://github.com/googleapis/google-cloud-go/commit/43cc5bc068d2f3abdde6c65beaac349218fc1a02))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/firestore/v1.6.1...firestore/v1.7.0) (2022-10-06)


### Features

* **firestore/apiv1:** add firestore aggregation query apis to the stable googleapis branch ([ec1a190](https://github.com/googleapis/google-cloud-go/commit/ec1a190abbc4436fcaeaa1421c7d9df624042752))
* **firestore:** Adds Bulkwriter support to Firestore client ([#5946](https://github.com/googleapis/google-cloud-go/issues/5946)) ([20b6c1b](https://github.com/googleapis/google-cloud-go/commit/20b6c1bbbc28311f4388e163cd9358d1ac0e94d4))
* **firestore:** expose read_time fields in Firestore PartitionQuery and ListCollectionIds, currently only available in private preview ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))

### [1.6.1](https://www.github.com/googleapis/google-cloud-go/compare/firestore/v1.6.0...firestore/v1.6.1) (2021-10-29)


### Bug Fixes

* **firestore:** prefer exact matches when reflecting fields ([#4908](https://www.github.com/googleapis/google-cloud-go/issues/4908)) ([d3d9420](https://www.github.com/googleapis/google-cloud-go/commit/d3d94205995ad910bd277f1f930cef4ac86c8040))

## [1.6.0](https://www.github.com/googleapis/google-cloud-go/compare/firestore/v1.5.0...firestore/v1.6.0) (2021-09-09)


### Features

* **firestore:** Add support for PartitionQuery ([#4206](https://www.github.com/googleapis/google-cloud-go/issues/4206)) ([b34783a](https://www.github.com/googleapis/google-cloud-go/commit/b34783a4d7a8c88204e0f44bd411795d8267d811))
* **firestore:** Support DocumentRefs in OrderBy, Add Query.Serialize, Query.Deserialize for cross machine serialization ([#4347](https://www.github.com/googleapis/google-cloud-go/issues/4347)) ([a0f7a02](https://www.github.com/googleapis/google-cloud-go/commit/a0f7a02bd8db90fa2297c6e84658868901ef9566))


### Bug Fixes

* **firestore:** correct an issue with returning empty paritions from GetPartionedQueries ([#4346](https://www.github.com/googleapis/google-cloud-go/issues/4346)) ([b2a6171](https://www.github.com/googleapis/google-cloud-go/commit/b2a61719b3caf43b095fc290b23de245a2135512))
* **firestore:** remove excessive spans on iterator ([#4163](https://www.github.com/googleapis/google-cloud-go/issues/4163)) ([812ef1f](https://www.github.com/googleapis/google-cloud-go/commit/812ef1ffdce2e87570660b58f0e725ad51f68546))
* **firestore:** retry RESOURCE_EXHAUSTED errors docs: various documentation improvements ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))

## [1.5.0](https://www.github.com/googleapis/google-cloud-go/compare/v1.4.0...v1.5.0) (2021-02-24)


### Features

* **firestore:** add opencensus tracing support  ([#2942](https://www.github.com/googleapis/google-cloud-go/issues/2942)) ([257f322](https://www.github.com/googleapis/google-cloud-go/commit/257f322e68b75765bd316ccefed5461d4df538a0))


### Bug Fixes

* **firestore:** address a missing branch in watch.stop() error remapping ([#3643](https://www.github.com/googleapis/google-cloud-go/issues/3643)) ([89ad55d](https://www.github.com/googleapis/google-cloud-go/commit/89ad55d72f79995a68f9c2ed1cd9b5ba50009d6d))

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/firestore/v1.3.0...v1.4.0) (2020-12-03)


### Features

* **firestore:** support "!=" and "not-in" query operators ([#3207](https://www.github.com/googleapis/google-cloud-go/issues/3207)) ([5c44019](https://www.github.com/googleapis/google-cloud-go/commit/5c440192105fe3e9b5dd1b584118b309113935e3)), closes [/firebase.google.com/support/release-notes/js#version_7210_-_september_17_2020](https://www.github.com/googleapis//firebase.google.com/support/release-notes/js/issues/version_7210_-_september_17_2020)

## v1.3.0

- Add support for LimitToLast feature for queries. This allows
  a query to return the final N results. See docs
  [here](https://firebase.google.com/docs/reference/js/firebase.database.Query#limittolast).
- Add support for FieldTransformMinimum and FieldTransformMaximum.
- Add exported SetGoogleClientInfo method.
- Various updates to autogenerated clients.

## v1.2.0

- Deprecate v1beta1 client.
- Fix serverTimestamp docs.
- Add missing operators to query docs.
- Make document IDs 20 alpha-numeric characters. Previously, there could be more
  than 20 non-alphanumeric characters, which broke some users. See
  https://github.com/googleapis/google-cloud-go/issues/1715.
- Various updates to autogenerated clients.

## v1.1.1

- Fix bug in CollectionGroup query validation.

## v1.1.0

- Add support for `in` and `array-contains-any` query operators.

## v1.0.0

This is the first tag to carve out firestore as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
