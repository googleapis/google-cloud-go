# Changes

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
