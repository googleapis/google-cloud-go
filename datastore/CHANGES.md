# Changes

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
