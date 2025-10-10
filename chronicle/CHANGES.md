# Changes

## [0.2.0](https://github.com/googleapis/google-cloud-go/releases/tag/chronicle%2Fv0.2.0) (2025-10-10)

### Features

* Update Compute Engine v1 API to revision 20250511 (#1047) (#12396) ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add new change_stream.proto PiperOrigin-RevId: 766241102 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))
* add scenarios AUTO/NONE to autotuning config PiperOrigin-RevId: 766437023 ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))

### Bug Fixes

* upgrade gRPC service registration func An update to Go gRPC Protobuf generation will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go service files. This change should affect very few client library users. See release notes advisories in https://togithub.com/googleapis/google-cloud-go/pull/11025. ([40b60a4](https://github.com/googleapis/google-cloud-go/commit/40b60a4b268040ca3debd71ebcbcd126b5d58eaa))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/chronicle/v0.1.0...chronicle/v0.1.1) (2025-05-21)


### Bug Fixes

* **chronicle:** A new packaging option `com.google.cloud.chronicle.v1` for `java_package` is added ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))
* **chronicle:** An existing packaging option `google.cloud.chronicle.v1` for `java_package` is removed ([2aaada3](https://github.com/googleapis/google-cloud-go/commit/2aaada3fb7a9d3eaacec3351019e225c4038646b))

## 0.1.0 (2025-04-30)


### Features

* **chronicle:** New clients ([#12081](https://github.com/googleapis/google-cloud-go/issues/12081)) ([8f38e18](https://github.com/googleapis/google-cloud-go/commit/8f38e18f030f54756e5d556e5c3cbc6146bfe149))

## Changes
