# Changes

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/aiplatform/v1.3.0...aiplatform/v1.4.0) (2022-02-14)


### Features

* **aiplatform:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.3.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.2.0...aiplatform/v1.3.0) (2022-02-03)


### Features

* **aiplatform:** add dedicated_resources to DeployedIndex message in aiplatform v1 index_endpoint.proto chore: sort imports ([6e56077](https://www.github.com/googleapis/google-cloud-go/commit/6e560776fd6e574320ce2dbad1f9eb9e22999185))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.1.1...aiplatform/v1.2.0) (2022-01-04)


### Features

* **aiplatform:** add enable_private_service_connect field to Endpoint feat: add id field to DeployedModel feat: add service_attachment field to PrivateEndpoints feat: add endpoint_id to CreateEndpointRequest and method signature to CreateEndpoint feat: add method signature to CreateFeatureStore, CreateEntityType, CreateFeature feat: add network and enable_private_service_connect to IndexEndpoint feat: add service_attachment to IndexPrivateEndpoints feat: add stratified_split field to training_pipeline InputDataConfig ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))
* **aiplatform:** Adds support for `google.protobuf.Value` pipeline parameters in the `parameter_values` field ([88a1cdb](https://www.github.com/googleapis/google-cloud-go/commit/88a1cdbef3cc337354a61bc9276725bfb9a686d8))
* **aiplatform:** Tensorboard v1 protos release feat:Exposing a field for v1 CustomJob-Tensorboard integration. ([90e2868](https://www.github.com/googleapis/google-cloud-go/commit/90e2868a3d220aa7f897438f4917013fda7a7c59))

### [1.1.1](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.1.0...aiplatform/v1.1.1) (2021-11-02)


### Bug Fixes

* **aiplatform:** Remove invalid resource annotations ([587bba5](https://www.github.com/googleapis/google-cloud-go/commit/587bba5ad792a92f252107aa38c6af50fb09fb58))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v1.0.0...aiplatform/v1.1.0) (2021-10-18)


### Features

* **aiplatform:** add featurestore service to aiplatform v1 feat: add metadata service to aiplatform v1 ([30794e7](https://www.github.com/googleapis/google-cloud-go/commit/30794e70050b55ff87d6a80d0b4075065e9d271d))
* **aiplatform:** add vizier service to aiplatform v1 BUILD.bazel ([12928d4](https://www.github.com/googleapis/google-cloud-go/commit/12928d47de771f4b23577062afe5cf551b347919))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/aiplatform/v0.1.0...aiplatform/v0.2.0) (2021-09-09)


### Features

* **aiplatform:** add Vizier service to aiplatform v1 ([33e4d89](https://www.github.com/googleapis/google-cloud-go/commit/33e4d895373dc8ec1dad13645ee5f342b2b15282))
* **aiplatform:** add XAI, model monitoring, and index services to aiplatform v1 ([e385b40](https://www.github.com/googleapis/google-cloud-go/commit/e385b40a1e2ecf81f5fd0910de5c37275951f86b))

## v0.1.0

This is the first tag to carve out aiplatform as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
