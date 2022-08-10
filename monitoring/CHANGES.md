# Changes

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/monitoring/v1.5.0...monitoring/v1.6.0) (2022-08-09)


### Features

* **monitoring/apiv3:** Added support for evaluating missing data in AlertPolicy ([83d8e8d](https://github.com/googleapis/google-cloud-go/commit/83d8e8dde9d8601db20096fb869b50c7abf1ba7e))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/monitoring/v1.4.0...monitoring/v1.5.0) (2022-04-14)


### Features

* **monitoring/dashboard:** Sync public protos with latests public api state. This adds support for collapsible groups, filters, labels, drilldowns, logs panels and tables ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/monitoring/v1.3.0...monitoring/v1.4.0) (2022-02-23)


### Features

* **monitoring:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/monitoring/v1.2.0...monitoring/v1.3.0) (2022-02-14)


### Features

* **monitoring:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/monitoring/v1.1.0...monitoring/v1.2.0) (2022-01-04)


### Features

* **monitoring/dashboard:** Added support for auto-close configurations ([90e2868](https://www.github.com/googleapis/google-cloud-go/commit/90e2868a3d220aa7f897438f4917013fda7a7c59))
* **monitoring/metricsscope:** promote apiv1 to GA ([#5135](https://www.github.com/googleapis/google-cloud-go/issues/5135)) ([33c0f63](https://www.github.com/googleapis/google-cloud-go/commit/33c0f63e0e0ce69d9ef6e57b04d1b8cc10ed2b78))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/monitoring/v1.0.0...monitoring/v1.1.0) (2021-10-18)

### Features

* **monitoring/apiv3:** add CreateServiceTimeSeries RPC ([9e41088](https://www.github.com/googleapis/google-cloud-go/commit/9e41088bb395fbae0e757738277d5c95fa2749c8))

### Bug Fixes

* **monitoring/apiv3:** Reintroduce deprecated field/enum for backward compatibility docs: Use absolute link targets in comments ([45fd259](https://www.github.com/googleapis/google-cloud-go/commit/45fd2594d99ef70c776df26866f0a3b537e7e69e))

## 1.0.0

Stabilize GA surface.

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/monitoring/v0.2.0...monitoring/v0.3.0) (2021-09-21)

### Features

* **monitoring/metricsscope:** start generating apiv1 ([8d45b5d](https://www.github.com/googleapis/google-cloud-go/commit/8d45b5d802b5da2e82f9f8fbe00c01b0c54a6b33))

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/monitoring/v0.1.0...monitoring/v0.2.0) (2021-08-30)

### Features

* **monitoring/dashboard:** Added support for logs-based alerts: https://cloud.google.com/logging/docs/alerting/log-based-alerts feat: Added support for user-defined labels on cloud monitoring's Service and ServiceLevelObjective objects fix!: mark required fields in QueryTimeSeriesRequest as required ([b9226eb](https://www.github.com/googleapis/google-cloud-go/commit/b9226eb0b34473cb6f920c2526ad0d6dacb03f3c))

## v0.1.0

This is the first tag to carve out monitoring as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
