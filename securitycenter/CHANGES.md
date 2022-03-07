# Changes

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.3.0...securitycenter/v1.4.0) (2022-02-23)


### Features

* **securitycenter:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/securitycenter/v1.2.0...securitycenter/v1.3.0) (2022-02-14)


### Features

* **securitycenter:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/securitycenter/v1.1.0...securitycenter/v1.2.0) (2022-01-04)


### Features

* **securitycenter:** Added a new API method UpdateExternalSystem, which enables updating a finding w/ external system metadata. External systems are a child resource under finding, and are housed on the finding itself, and can also be filtered on in Notifications, the ListFindings and GroupFindings API ([c8271d4](https://www.github.com/googleapis/google-cloud-go/commit/c8271d4b217a6e6924d9f87eac9468c4b5767ba7))
* **securitycenter:** Added mute related APIs, proto messages and fields ([3e7185c](https://www.github.com/googleapis/google-cloud-go/commit/3e7185c241d97ee342f132ae04bc93bb79a8e897))
* **securitycenter:** Added resource type and display_name field to the FindingResult, and supported them in the filter for ListFindings and GroupFindings. Also added display_name to the resource which is surfaced in NotificationMessage ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/securitycenter/v1.0.0...securitycenter/v1.1.0) (2021-10-11)


### Features

* **securitycenter:** Added vulnerability field to the finding feat: Added type field to the resource which is surfaced in NotificationMessage ([090cc3a](https://www.github.com/googleapis/google-cloud-go/commit/090cc3ae0f8747a14cc904fc6d429e2f5379bb03))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out securitycenter as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
