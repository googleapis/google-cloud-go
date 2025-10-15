# Changes

## [0.5.1](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.5.0...networkservices/v0.5.1) (2025-09-18)


### Bug Fixes

* **networkservices:** Upgrade gRPC service registration func ([a10ecc9](https://github.com/googleapis/google-cloud-go/commit/a10ecc9b3c22e320e9a32dedef7248b42465cd49))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.4.0...networkservices/v0.5.0) (2025-07-09)


### Features

* **networkservices:** Add isolation support to prevent cross-region overflow by adding a new field "isolation_config" to message "ServiceLbPolicy" ([98ba6f0](https://github.com/googleapis/google-cloud-go/commit/98ba6f06e69685bca510ca85c12124434f9ba1e8))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.3.0...networkservices/v0.4.0) (2025-06-25)


### Features

* **networkservices:** Add support for WasmPlugin resource in NetworkServices API ([#12485](https://github.com/googleapis/google-cloud-go/issues/12485)) ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))


### Documentation

* **networkservices:** A comment for enum `LoadBalancingScheme` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `authority` in message `.google.cloud.networkservices.v1.ExtensionChain` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `extensions` in message `.google.cloud.networkservices.v1.ExtensionChain` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `forwarding_rules` in message `.google.cloud.networkservices.v1.LbRouteExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `forwarding_rules` in message `.google.cloud.networkservices.v1.LbTrafficExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `load_balancing_scheme` in message `.google.cloud.networkservices.v1.LbRouteExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `load_balancing_scheme` in message `.google.cloud.networkservices.v1.LbTrafficExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `metadata` in message `.google.cloud.networkservices.v1.LbRouteExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `metadata` in message `.google.cloud.networkservices.v1.LbTrafficExtension` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `order_by` in message `.google.cloud.networkservices.v1.ListLbRouteExtensionsRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `order_by` in message `.google.cloud.networkservices.v1.ListLbTrafficExtensionsRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `parent` in message `.google.cloud.networkservices.v1.ListLbRouteExtensionsRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `parent` in message `.google.cloud.networkservices.v1.ListLbTrafficExtensionsRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.CreateLbRouteExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.CreateLbTrafficExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.DeleteLbRouteExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.DeleteLbTrafficExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.UpdateLbRouteExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `request_id` in message `.google.cloud.networkservices.v1.UpdateLbTrafficExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `service` in message `.google.cloud.networkservices.v1.ExtensionChain` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `supported_events` in message `.google.cloud.networkservices.v1.ExtensionChain` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `timeout` in message `.google.cloud.networkservices.v1.ExtensionChain` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `update_mask` in message `.google.cloud.networkservices.v1.UpdateLbRouteExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))
* **networkservices:** A comment for field `update_mask` in message `.google.cloud.networkservices.v1.UpdateLbTrafficExtensionRequest` is changed ([116a33a](https://github.com/googleapis/google-cloud-go/commit/116a33ab13c9fac6f6830dded55c24d38504707b))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.5...networkservices/v0.3.0) (2025-06-17)


### Features

* **networkservices:** Update NetworkServices protos ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))


### Documentation

* **networkservices:** A comment for field `address` in message `.google.cloud.networkservices.v1.TcpRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `fault_injection_policy` in message `.google.cloud.networkservices.v1.GrpcRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `matches` in message `.google.cloud.networkservices.v1.TlsRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `metadata_label_match_criteria` in message `.google.cloud.networkservices.v1.EndpointMatcher` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.DeleteServiceBindingRequest` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.EndpointPolicy` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.Gateway` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.GetServiceBindingRequest` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.GrpcRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.HttpRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.Mesh` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.ServiceBinding` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.TcpRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `name` in message `.google.cloud.networkservices.v1.TlsRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `parent` in message `.google.cloud.networkservices.v1.CreateServiceBindingRequest` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `parent` in message `.google.cloud.networkservices.v1.ListServiceBindingsRequest` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `ports` in message `.google.cloud.networkservices.v1.Gateway` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `scope` in message `.google.cloud.networkservices.v1.Gateway` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `service` in message `.google.cloud.networkservices.v1.ServiceBinding` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `sni_host` in message `.google.cloud.networkservices.v1.TlsRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for field `weight` in message `.google.cloud.networkservices.v1.TlsRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for message `GrpcRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for message `HttpRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for message `ServiceBinding` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))
* **networkservices:** A comment for message `TlsRoute` is changed ([9614487](https://github.com/googleapis/google-cloud-go/commit/96144875e01bfc8a59c2671c6eae87233710cef7))

## [0.2.5](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.4...networkservices/v0.2.5) (2025-04-15)


### Bug Fixes

* **networkservices:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [0.2.4](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.3...networkservices/v0.2.4) (2025-03-13)


### Bug Fixes

* **networkservices:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [0.2.3](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.2...networkservices/v0.2.3) (2025-01-02)


### Bug Fixes

* **networkservices:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [0.2.2](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.1...networkservices/v0.2.2) (2024-10-23)


### Bug Fixes

* **networkservices:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **networkservices:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.2.0...networkservices/v0.2.1) (2024-09-12)


### Bug Fixes

* **networkservices:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.6...networkservices/v0.2.0) (2024-08-20)


### Features

* **networkservices:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [0.1.6](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.5...networkservices/v0.1.6) (2024-08-08)


### Bug Fixes

* **networkservices:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [0.1.5](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.4...networkservices/v0.1.5) (2024-07-24)


### Bug Fixes

* **networkservices:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [0.1.4](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.3...networkservices/v0.1.4) (2024-07-10)


### Bug Fixes

* **networkservices:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.2...networkservices/v0.1.3) (2024-07-01)


### Bug Fixes

* **networkservices:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.1...networkservices/v0.1.2) (2024-06-26)


### Bug Fixes

* **networkservices:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/networkservices/v0.1.0...networkservices/v0.1.1) (2024-06-12)


### Documentation

* **networkservices:** Add a comment for the NetworkServices service ([134f567](https://github.com/googleapis/google-cloud-go/commit/134f567c18892d6050f60ae875a3de7738104da0))

## 0.1.0 (2024-06-05)


### Features

* **networkservices:** New client(s) ([#10314](https://github.com/googleapis/google-cloud-go/issues/10314)) ([ee4df98](https://github.com/googleapis/google-cloud-go/commit/ee4df98e7ff89c005ee345120fb53c85086a2461))

## Changes
