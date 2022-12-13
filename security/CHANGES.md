# Changes

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.9.0...security/v1.10.0) (2022-11-03)


### Features

* **security:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.8.0...security/v1.9.0) (2022-10-25)


### Features

* **security:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.7.0...security/v1.8.0) (2022-09-21)


### Features

* **security:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.6.0...security/v1.7.0) (2022-09-19)


### Features

* **security:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))


### Bug Fixes

* **security/publicca:** Add proto options for Ruby, PHP and C# API client libraries ([bc7a5f6](https://github.com/googleapis/google-cloud-go/commit/bc7a5f609994f73e26f72a78f0ff14aa75c1c227))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.5.0...security/v1.6.0) (2022-09-15)


### Features

* **security/privateca/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **security/publicca/apiv1beta1/publiccapb:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **security/publicca/apiv1beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.4.1...security/v1.5.0) (2022-09-09)


### Features

* **security/publicca:** Start generating apiv1beta1 ([#6642](https://github.com/googleapis/google-cloud-go/issues/6642)) ([778161b](https://github.com/googleapis/google-cloud-go/commit/778161b208819783618c5be8191960167bd67e1e))

## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/security/v1.4.0...security/v1.4.1) (2022-07-12)


### Bug Fixes

* **security/privateca:** publish v1beta1 LRO HTTP rules ([963efe2](https://github.com/googleapis/google-cloud-go/commit/963efe22cf67bc04fed09b5fa8f9cb20b9edf1a3))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.3.0...security/v1.4.0) (2022-05-24)


### Features

* **security/privateca:** Provide interfaces for location and IAM policy calls ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.2.1...security/v1.3.0) (2022-02-23)


### Features

* **security:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

### [1.2.1](https://github.com/googleapis/google-cloud-go/compare/security/v1.2.0...security/v1.2.1) (2022-02-22)


### Bug Fixes

* **security/privateca:** Add google-cloud-location as a dependency for the privateca client ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/security/v1.1.1...security/v1.2.0) (2022-02-14)


### Features

* **security:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

### [1.1.1](https://www.github.com/googleapis/google-cloud-go/compare/security/v1.1.0...security/v1.1.1) (2022-01-04)


### Bug Fixes

* **security/privateca:** include mixin protos as input for mixin rpcs ([479c2f9](https://www.github.com/googleapis/google-cloud-go/commit/479c2f90d556a106b25ebcdb1539d231488182da))
* **security/privateca:** repair service config to enable mixins ([83b941c](https://www.github.com/googleapis/google-cloud-go/commit/83b941c0983e44fdd18ceee8c6f3e91219d72ad1))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/security/v1.0.0...security/v1.1.0) (2021-10-11)

### Features

* **security/privateca:** add IAMPolicy & Locations mix-in support ([1a0720f](https://www.github.com/googleapis/google-cloud-go/commit/1a0720f2f33bb14617f5c6a524946a93209e1266))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out security as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
