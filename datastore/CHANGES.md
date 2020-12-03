# Changes

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/datastore/v1.3.0...v1.4.0) (2020-12-03)


### Features

* **datastore:** add opencensus tracing/stats support ([#2804](https://www.github.com/googleapis/google-cloud-go/issues/2804)) ([5e6c350](https://www.github.com/googleapis/google-cloud-go/commit/5e6c350b2ac94787934380e930af2cb2094fa8f1))
* **datastore:** support civil package types save ([#3202](https://www.github.com/googleapis/google-cloud-go/issues/3202)) ([9cc1a66](https://www.github.com/googleapis/google-cloud-go/commit/9cc1a66e22ecd8dcad1235c290f05b92edff5aa0))


### Bug Fixes

* **datastore:** increase deferred key iter limit ([#2878](https://www.github.com/googleapis/google-cloud-go/issues/2878)) ([7f1057a](https://www.github.com/googleapis/google-cloud-go/commit/7f1057a30d3b8691a22c85255bb41d31d42c6f9c))

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
