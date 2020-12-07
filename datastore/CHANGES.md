# Changes

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/datastore/v1.3.0...v1.4.0) (2020-12-07)


### Features

* **all:** auto-regenerate gapics , refs [#3374](https://www.github.com/googleapis/google-cloud-go/issues/3374) [#3335](https://www.github.com/googleapis/google-cloud-go/issues/3335) [#3294](https://www.github.com/googleapis/google-cloud-go/issues/3294) [#3250](https://www.github.com/googleapis/google-cloud-go/issues/3250) [#3229](https://www.github.com/googleapis/google-cloud-go/issues/3229) [#3211](https://www.github.com/googleapis/google-cloud-go/issues/3211) [#3217](https://www.github.com/googleapis/google-cloud-go/issues/3217) [#3212](https://www.github.com/googleapis/google-cloud-go/issues/3212) [#3209](https://www.github.com/googleapis/google-cloud-go/issues/3209) [#3206](https://www.github.com/googleapis/google-cloud-go/issues/3206) [#3199](https://www.github.com/googleapis/google-cloud-go/issues/3199) [#3177](https://www.github.com/googleapis/google-cloud-go/issues/3177) [#3164](https://www.github.com/googleapis/google-cloud-go/issues/3164) [#3149](https://www.github.com/googleapis/google-cloud-go/issues/3149) [#3142](https://www.github.com/googleapis/google-cloud-go/issues/3142) [#3136](https://www.github.com/googleapis/google-cloud-go/issues/3136) [#3130](https://www.github.com/googleapis/google-cloud-go/issues/3130) [#3121](https://www.github.com/googleapis/google-cloud-go/issues/3121) [#3119](https://www.github.com/googleapis/google-cloud-go/issues/3119) [#3115](https://www.github.com/googleapis/google-cloud-go/issues/3115) [#3106](https://www.github.com/googleapis/google-cloud-go/issues/3106) [#3102](https://www.github.com/googleapis/google-cloud-go/issues/3102) [#3083](https://www.github.com/googleapis/google-cloud-go/issues/3083) [#3073](https://www.github.com/googleapis/google-cloud-go/issues/3073) [#3057](https://www.github.com/googleapis/google-cloud-go/issues/3057) [#3044](https://www.github.com/googleapis/google-cloud-go/issues/3044) [#3047](https://www.github.com/googleapis/google-cloud-go/issues/3047) [#3035](https://www.github.com/googleapis/google-cloud-go/issues/3035) [#3025](https://www.github.com/googleapis/google-cloud-go/issues/3025) [#3010](https://www.github.com/googleapis/google-cloud-go/issues/3010) [#3005](https://www.github.com/googleapis/google-cloud-go/issues/3005) [#2993](https://www.github.com/googleapis/google-cloud-go/issues/2993) [#2989](https://www.github.com/googleapis/google-cloud-go/issues/2989) [#2981](https://www.github.com/googleapis/google-cloud-go/issues/2981) [#2976](https://www.github.com/googleapis/google-cloud-go/issues/2976) [#2968](https://www.github.com/googleapis/google-cloud-go/issues/2968) [#2958](https://www.github.com/googleapis/google-cloud-go/issues/2958) [#2952](https://www.github.com/googleapis/google-cloud-go/issues/2952) [#2944](https://www.github.com/googleapis/google-cloud-go/issues/2944) [#2935](https://www.github.com/googleapis/google-cloud-go/issues/2935) [#2933](https://www.github.com/googleapis/google-cloud-go/issues/2933) [#2919](https://www.github.com/googleapis/google-cloud-go/issues/2919) [#2913](https://www.github.com/googleapis/google-cloud-go/issues/2913) [#2910](https://www.github.com/googleapis/google-cloud-go/issues/2910) [#2899](https://www.github.com/googleapis/google-cloud-go/issues/2899) [#2897](https://www.github.com/googleapis/google-cloud-go/issues/2897) [#2886](https://www.github.com/googleapis/google-cloud-go/issues/2886) [#2877](https://www.github.com/googleapis/google-cloud-go/issues/2877)
* **datastore:** add opencensus tracing/stats support ([#2804](https://www.github.com/googleapis/google-cloud-go/issues/2804)) ([5e6c350](https://www.github.com/googleapis/google-cloud-go/commit/5e6c350b2ac94787934380e930af2cb2094fa8f1))
* **datastore:** support civil package types save ([#3202](https://www.github.com/googleapis/google-cloud-go/issues/3202)) ([9cc1a66](https://www.github.com/googleapis/google-cloud-go/commit/9cc1a66e22ecd8dcad1235c290f05b92edff5aa0))


### Bug Fixes

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
