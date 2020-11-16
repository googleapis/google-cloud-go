# Changes

### [1.1.2](https://www.github.com/googleapis/google-cloud-go/compare/logging/v1.1.1...v1.1.2) (2020-11-09)


### Bug Fixes

* **logging:** allow X-Cloud-Trace-Context fields to be optional ([#3062](https://www.github.com/googleapis/google-cloud-go/issues/3062)) ([7ff03cf](https://www.github.com/googleapis/google-cloud-go/commit/7ff03cf9a544e753de5b034e18339ecf517d2193))
* **logging:** do not panic in library code ([#3076](https://www.github.com/googleapis/google-cloud-go/issues/3076)) ([529be97](https://www.github.com/googleapis/google-cloud-go/commit/529be977f766443f49cb8914e17ba07c93841e84)), closes [#1862](https://www.github.com/googleapis/google-cloud-go/issues/1862)

## v1.1.1

- Rebrand "Stackdriver Logging" to "Cloud Logging".

## v1.1.0

- Support unmarshalling stringified Severity.
- Add exported SetGoogleClientInfo wrappers to manual file.
- Support no payload.
- Update "Grouping Logs by Request" docs.
- Add auto-detection of monitored resources on GAE Standard.

## v1.0.0

This is the first tag to carve out logging as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
