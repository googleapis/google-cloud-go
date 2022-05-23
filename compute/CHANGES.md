# Changes

### [1.6.1](https://github.com/googleapis/google-cloud-go/compare/compute/v1.6.0...compute/v1.6.1) (2022-04-21)


### Bug Fixes

* **compute:** revert proto3_optional, required removal on parent_id ([#714](https://github.com/googleapis/google-cloud-go/issues/714)) ([d4ea7dd](https://github.com/googleapis/google-cloud-go/commit/d4ea7dd68bf2b858481727afd8a8830e31a9fe55))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/compute/v1.5.0...compute/v1.6.0) (2022-04-14)


### Features

* **compute:** update compute API to revision 20220322 ([#710](https://github.com/googleapis/google-cloud-go/issues/710)) ([bb5da6b](https://github.com/googleapis/google-cloud-go/commit/bb5da6b3c34079a01d18b766b67f626cff18d849))


### Bug Fixes

* **compute:** remove proto3_optional from parent_id ([#712](https://github.com/googleapis/google-cloud-go/issues/712)) ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))
* **compute:** replace missing REQUIRED for parent_id ([#711](https://github.com/googleapis/google-cloud-go/issues/711)) ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/compute/v1.4.0...compute/v1.5.0) (2022-02-23)


### Features

* **compute:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/compute/v1.3.0...compute/v1.4.0) (2022-02-22)


### Features

* **compute:** update compute API to revision 20220112 ([#700](https://github.com/googleapis/google-cloud-go/issues/700)) ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))


### Bug Fixes

* **compute:** fix breaking changes in Compute API ([#701](https://github.com/googleapis/google-cloud-go/issues/701)) ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/compute/v1.2.0...compute/v1.3.0) (2022-02-14)


### Features

* **compute:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/compute/v1.1.0...compute/v1.2.0) (2022-02-03)


### Features

* **compute:** regen compute for LRO helpers ([#5431](https://www.github.com/googleapis/google-cloud-go/issues/5431)) ([95d3609](https://www.github.com/googleapis/google-cloud-go/commit/95d3609b7b9ec917e48b8dfbc18875dfed378c1b))


### Bug Fixes

* **compute/metadata:** init a HTTP client in OnGCE check ([#5439](https://www.github.com/googleapis/google-cloud-go/issues/5439)) ([76c6e40](https://www.github.com/googleapis/google-cloud-go/commit/76c6e40171b2f032913549c95396cd8d44fbd7f5)), refs [#5430](https://www.github.com/googleapis/google-cloud-go/issues/5430)

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/compute/v1.0.0...compute/v1.1.0) (2022-01-14)


### Features

* **compute:** remove BETA language on Compute V1 ([#697](https://www.github.com/googleapis/google-cloud-go/issues/697)) ([3bbe8c0](https://www.github.com/googleapis/google-cloud-go/commit/3bbe8c0c558c06ef5865bb79eb228b6da667ddb3))

## [1.0.0](https://www.github.com/googleapis/google-cloud-go/compare/compute/v0.1.0...compute/v1.0.0) (2022-01-11)

### Features

* **compute:** release 1.0.0 ([#5328](https://www.github.com/googleapis/google-cloud-go/issues/5328)) ([5437c12](https://www.github.com/googleapis/google-cloud-go/commit/5437c12945595325f7df098f707b2691cc8011be))

## v0.1.0

This is the first tag to carve out compute as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
