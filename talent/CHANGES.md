# Changes


## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/talent/v1.4.0...talent/v1.4.1) (2022-11-09)


### Documentation

* **talent:** marking keyword_searchable_job_custom_attributes on the company object as deprecated ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **talent:** marking keyword_searchable_job_custom_attributes on the company object as deprecated ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/talent/v1.3.0...talent/v1.4.0) (2022-11-03)


### Features

* **talent:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/talent/v1.2.0...talent/v1.3.0) (2022-10-25)


### Features

* **talent:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/talent/v1.1.0...talent/v1.2.0) (2022-09-21)


### Features

* **talent:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/talent/v1.0.0...talent/v1.1.0) (2022-09-19)


### Features

* **talent:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.9.0...talent/v1.0.0) (2022-07-12)


### Features

* **talent:** promote to GA ([#6293](https://github.com/googleapis/google-cloud-go/issues/6293)) ([c0a0c20](https://github.com/googleapis/google-cloud-go/commit/c0a0c2078c0cf0e7859130e1104d4fd8f04d8b01))

## [0.9.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.8.0...talent/v0.9.0) (2022-06-29)


### Features

* **talent:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [0.8.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.7.0...talent/v0.8.0) (2022-06-16)


### ⚠ BREAKING CHANGES

* **talent:** remove Application and Profile services and and related protos, enums, and messages

### Bug Fixes

* **talent:** remove Application and Profile services and and related protos, enums, and messages ([4134941](https://github.com/googleapis/google-cloud-go/commit/41349411e601f57dc6d9e246f1748fd86d17bb15))


### Miscellaneous Chores

* **talent:** release v0.8.0 ([#6203](https://github.com/googleapis/google-cloud-go/issues/6203)) ([dee0ec2](https://github.com/googleapis/google-cloud-go/commit/dee0ec28c7d01bde3850fa0356ffd9fa9d595ddb))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.6.0...talent/v0.7.0) (2022-06-07)


### Features

* **talent:** add methods for Long Running Operations service ([1a0b09a](https://github.com/googleapis/google-cloud-go/commit/1a0b09a991d210fd562674aae1d2df854a0e15f9))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.5.0...talent/v0.6.0) (2022-06-01)


### Features

* **talent:** Add a new operator on companyDisplayNames filter to further support fuzzy match by treating input value as a multi word token feat: Add a new option TELECOMMUTE_JOBS_EXCLUDED under enum TelecommutePreference to completely filter out the telecommute jobs in response docs: Deprecate option TELECOMMUTE_EXCLUDED under enum TelecommutePreference ([46c16f1](https://github.com/googleapis/google-cloud-go/commit/46c16f1fdc7181d2fefadc8fd6a9e0b9cb226cac))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.4.0...talent/v0.5.0) (2022-02-23)


### Features

* **talent:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/talent/v0.3.1...talent/v0.4.0) (2022-02-14)


### Features

* **talent:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

### [0.3.1](https://www.github.com/googleapis/google-cloud-go/compare/talent/v0.3.0...talent/v0.3.1) (2022-01-14)


### Bug Fixes

* **talent:** add ancillary service bindings to service_yaml ([3bbe8c0](https://www.github.com/googleapis/google-cloud-go/commit/3bbe8c0c558c06ef5865bb79eb228b6da667ddb3))

## [0.3.0](https://www.github.com/googleapis/google-cloud-go/compare/talent/v0.2.0...talent/v0.3.0) (2021-09-15)


### Features

* **talent:** Added a new `KeywordMatchMode` field to support more keyword matching options feat: Added more `DiversificationLevel` configuration options ([8ffed36](https://www.github.com/googleapis/google-cloud-go/commit/8ffed36c9db818a24073cf865f626d29afd01716))

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/talent/v0.1.0...talent/v0.2.0) (2021-08-30)


### Features

* **talent:** Add new commute methods in Search APIs feat: Add new histogram type 'publish_time_in_day' feat: Support filtering by requisitionId is ListJobs API ([d4c3340](https://www.github.com/googleapis/google-cloud-go/commit/d4c3340bfc8b6793d6d2c8a3ed8ccdb472e1efd3))

## v0.1.0

This is the first tag to carve out talent as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
