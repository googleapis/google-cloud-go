# Changes

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.4.0...cloudbuild/v1.5.0) (2022-11-09)


### Features

* **cloudbuild/apiv1:** Add allow_failure, exit_code, and allow_exit_code to BuildStep message ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **cloudbuild/apiv1:** Integration of Cloud Build with Artifact Registry ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.3.0...cloudbuild/v1.4.0) (2022-11-03)


### Features

* **cloudbuild:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.2.0...cloudbuild/v1.3.0) (2022-10-25)


### Features

* **cloudbuild:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.1.0...cloudbuild/v1.2.0) (2022-02-23)


### Features

* **cloudbuild:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.0.0...cloudbuild/v1.1.0) (2022-02-14)


### Features

* **cloudbuild:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/cloudbuild/v0.1.0...cloudbuild/v0.2.0) (2021-08-30)


### Features

* **cloudbuild/apiv1:** Add ability to configure BuildTriggers to create Builds that require approval before executing and ApproveBuild API to approve or reject pending Builds ([d4c3340](https://www.github.com/googleapis/google-cloud-go/commit/d4c3340bfc8b6793d6d2c8a3ed8ccdb472e1efd3))
* **cloudbuild/apiv1:** add script field to BuildStep message ([b9226eb](https://www.github.com/googleapis/google-cloud-go/commit/b9226eb0b34473cb6f920c2526ad0d6dacb03f3c))
* **cloudbuild/apiv1:** Update cloudbuild proto with the service_account for BYOSA Triggers. ([b9226eb](https://www.github.com/googleapis/google-cloud-go/commit/b9226eb0b34473cb6f920c2526ad0d6dacb03f3c))

## v0.1.0

This is the first tag to carve out cloudbuild as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
