# Changes

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.14.2...cloudbuild/v1.14.3) (2023-11-01)


### Bug Fixes

* **cloudbuild:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.14.1...cloudbuild/v1.14.2) (2023-10-26)


### Bug Fixes

* **cloudbuild:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.14.0...cloudbuild/v1.14.1) (2023-10-12)


### Bug Fixes

* **cloudbuild:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.13.0...cloudbuild/v1.14.0) (2023-08-08)


### Features

* **cloudbuild/apiv1:** Add update_mask to UpdateBuildTriggerRequest proto ([#8358](https://github.com/googleapis/google-cloud-go/issues/8358)) ([58b5851](https://github.com/googleapis/google-cloud-go/commit/58b5851b3f38aeeefcdb3507e29b9a02ccfb1bba))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.12.0...cloudbuild/v1.13.0) (2023-07-26)


### Features

* **cloudbuild/apiv1:** Add automap_substitutions flag to use substitutions as envs in Cloud Build ([327e101](https://github.com/googleapis/google-cloud-go/commit/327e10188a2e22dd7b7e6c12a8cf66729f65974c))
* **cloudbuild/apiv1:** Add git_file_source and git_repo_source to build_trigger ([7cb7f66](https://github.com/googleapis/google-cloud-go/commit/7cb7f66f0646617c27aa9a9b4fe38b9f368eb3bb))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.11.0...cloudbuild/v1.12.0) (2023-07-24)


### Features

* **cloudbuild/apiv1:** Add repositoryevent to buildtrigger ([eca3c90](https://github.com/googleapis/google-cloud-go/commit/eca3c9070cd96a50fa857a6c016e35a98dbea5e7))
* **cloudbuild/apiv1:** Add routing information in Cloud Build GRPC clients ([eca3c90](https://github.com/googleapis/google-cloud-go/commit/eca3c9070cd96a50fa857a6c016e35a98dbea5e7))
* **cloudbuild/apiv1:** Added e2-medium machine type ([eca3c90](https://github.com/googleapis/google-cloud-go/commit/eca3c9070cd96a50fa857a6c016e35a98dbea5e7))
* **cloudbuild/apiv1:** Update third party clodubuild.proto library to include git_source ([eca3c90](https://github.com/googleapis/google-cloud-go/commit/eca3c9070cd96a50fa857a6c016e35a98dbea5e7))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.10.1...cloudbuild/v1.11.0) (2023-07-10)


### Features

* **cloudbuild:** Add GitLabConfig and fetchGitRefs for Cloud Build Repositories ([a3ec3cf](https://github.com/googleapis/google-cloud-go/commit/a3ec3cf858c7d9154338ac4cd8a9a068dc7a7f4d))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.10.0...cloudbuild/v1.10.1) (2023-06-20)


### Bug Fixes

* **cloudbuild:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.9.1...cloudbuild/v1.10.0) (2023-05-30)


### Features

* **cloudbuild:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.9.0...cloudbuild/v1.9.1) (2023-05-08)


### Bug Fixes

* **cloudbuild:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.8.0...cloudbuild/v1.9.0) (2023-03-22)


### Features

* **cloudbuild/apiv1:** Add DefaultLogsBucketBehavior to BuildOptions ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))


### Bug Fixes

* **cloudbuild:** Change java package of Cloud Build v2 ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))
* **cloudbuild:** Change java package of Cloud Build v2 ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.7.0...cloudbuild/v1.8.0) (2023-03-15)


### Features

* **cloudbuild:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.6.0...cloudbuild/v1.7.0) (2023-03-01)


### Features

* **cloudbuild:** Start generating apiv2 ([#7505](https://github.com/googleapis/google-cloud-go/issues/7505)) ([6fb3398](https://github.com/googleapis/google-cloud-go/commit/6fb339836920ab4196390814b03636f93e7c3676))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/cloudbuild/v1.5.0...cloudbuild/v1.6.0) (2023-01-04)


### Features

* **cloudbuild:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

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
