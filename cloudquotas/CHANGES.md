# Changelog



## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.1.2...cloudquotas/v1.2.0) (2024-11-14)


### Features

* **cloudquotas:** A new value `NOT_ENOUGH_USAGE_HISTORY` is added to enum `IneligibilityReason` ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))
* **cloudquotas:** A new value `NOT_SUPPORTED` is added to enum `IneligibilityReason` ([e85151d](https://github.com/googleapis/google-cloud-go/commit/e85151ddc5f70174f951265106d5a114191c5f53))

## [1.1.2](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.1.1...cloudquotas/v1.1.2) (2024-10-23)


### Bug Fixes

* **cloudquotas:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **cloudquotas:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.1.1](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.1.0...cloudquotas/v1.1.1) (2024-09-12)


### Bug Fixes

* **cloudquotas:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.0.4...cloudquotas/v1.1.0) (2024-08-20)


### Features

* **cloudquotas:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.0.4](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.0.3...cloudquotas/v1.0.4) (2024-08-08)


### Bug Fixes

* **cloudquotas:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.0.3](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.0.2...cloudquotas/v1.0.3) (2024-07-24)


### Bug Fixes

* **cloudquotas:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.0.2](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.0.1...cloudquotas/v1.0.2) (2024-07-10)


### Bug Fixes

* **cloudquotas:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v1.0.0...cloudquotas/v1.0.1) (2024-07-01)


### Bug Fixes

* **cloudquotas:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.2.1...cloudquotas/v1.0.0) (2024-06-26)


### Miscellaneous Chores

* **cloudquotas:** Release v1.0.0 ([#10444](https://github.com/googleapis/google-cloud-go/issues/10444)) ([4f2cc2b](https://github.com/googleapis/google-cloud-go/commit/4f2cc2b6925486fc5d0c1d16be82604b8c889659))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.2.0...cloudquotas/v0.2.1) (2024-05-01)


### Bug Fixes

* **cloudquotas:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))


### Documentation

* **cloudquotas:** Update contact_email doc to not check permission of the email account ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.1.3...cloudquotas/v0.2.0) (2024-04-15)


### Features

* **cloudquotas:** Add `rollout_info` field to `QuotaDetails` message ([2cdc40a](https://github.com/googleapis/google-cloud-go/commit/2cdc40a0b4288f5ab5f2b2b8f5c1d6453a9c81ec))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.1.2...cloudquotas/v0.1.3) (2024-03-27)


### Documentation

* **cloudquotas:** Update comment of `contact_email` to make it optional as opposed to required ([4834425](https://github.com/googleapis/google-cloud-go/commit/48344254a5d21ec51ffee275c78a15c9345dc09c))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.1.1...cloudquotas/v0.1.2) (2024-03-14)


### Bug Fixes

* **cloudquotas:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))


### Documentation

* **cloudquotas:** A comment for field `filter` in message `.google.api.cloudquotas.v1.ListQuotaPreferencesRequest` is changed ([05f58cc](https://github.com/googleapis/google-cloud-go/commit/05f58ccce530d8a3ab404356929352002d5156ba))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/cloudquotas/v0.1.0...cloudquotas/v0.1.1) (2024-01-30)


### Bug Fixes

* **cloudquotas:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## 0.1.0 (2024-01-08)


### Features

* **cloudquotas:** New clients ([#9222](https://github.com/googleapis/google-cloud-go/issues/9222)) ([57e2d7b](https://github.com/googleapis/google-cloud-go/commit/57e2d7bd2730b4acd18eac0e3a18e682b51c3e03))

## Changes
