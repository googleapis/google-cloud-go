# Changelog

## 1.0.0 (2026-06-16)


### ⚠ BREAKING CHANGES

* **all:** The HAS operator on timestamp fields now only accepts the wildcard "*" for presence checks (e.g. "create_time:*"). Using timestamp values with HAS (e.g. "create_time:\"2022-...\"") is no longer supported.

### Features

* Enable open telemetry attrs ([#14426](https://github.com/googleapis/google-cloud-go/issues/14426)) ([74eab64](https://github.com/googleapis/google-cloud-go/commit/74eab64d1b4e22d8c79b0de4e5fc9a36bc4c6c19))
* Update API sources and regenerate ([#14581](https://github.com/googleapis/google-cloud-go/issues/14581)) ([df96b2e](https://github.com/googleapis/google-cloud-go/commit/df96b2ecb3930d6fb2e6e542e11521ee8e9d5935))
* Update image to ([09bb990](https://github.com/googleapis/google-cloud-go/commit/09bb990b634df7bc5a927d50f76a2d5be650ab82))


### Bug Fixes

* **bigquery:** Migrate usages of proto.Clone to proto.CloneOf ([#14463](https://github.com/googleapis/google-cloud-go/issues/14463)) ([8089b04](https://github.com/googleapis/google-cloud-go/commit/8089b04532058462915018ab54ca06a977a325c3))


### Miscellaneous Chores

* **all:** Update deps (main) ([#14180](https://github.com/googleapis/google-cloud-go/issues/14180)) ([14b9656](https://github.com/googleapis/google-cloud-go/commit/14b965686dde28d25ce4ad0ca0056c04f5bd235a))

## Changes
