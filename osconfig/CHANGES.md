# Changes

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.9.0...osconfig/v1.10.0) (2022-11-03)


### Features

* **osconfig:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.8.0...osconfig/v1.9.0) (2022-10-25)


### Features

* **osconfig:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.7.0...osconfig/v1.8.0) (2022-09-21)


### Features

* **osconfig:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.6.0...osconfig/v1.7.0) (2022-09-19)


### Features

* **osconfig:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.5.0...osconfig/v1.6.0) (2022-06-29)


### Features

* **osconfig:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.4.0...osconfig/v1.5.0) (2022-02-23)


### Features

* **osconfig:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.3.0...osconfig/v1.4.0) (2022-02-22)


### Features

* **osconfig/agentendpoint:** Add field to PatchConfig message:   - mig_instances_allowed fix: Add NONE Interpreter enum value that should be used over INTERPRETER_UNSPECIFIED in ExecStepConfig message ([7d6b0e5](https://github.com/googleapis/google-cloud-go/commit/7d6b0e5891b50cccdf77cd17ddd3644f31ef6dfc))
* **osconfig/agentendpoint:** Add fields to RegisterAgentRequest:   - supported_capabilities   - os_long_name   - os_short_name   - os_version   - os_architecture feat: Add field to PatchConfig message:   - mig_instances_allowed fix: Add NONE Interpreter enum value that should be used over INTERPRETER_UNSPECIFIED in ExecStepConfig message ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))


### Bug Fixes

* **osconfig/agentendpoint:** Fix description of an interpreter field, validate if the field is not unspecified ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))
* **osconfig/agentendpoint:** update third_party protos to the most actual version: - Add item that is affected by vulnerability - Add GetOsPolicyAssignmentReport and analogous List rpc method - Add Inventory to InstanceFilter - Add existing os_policy_assignment_reports.proto fixing the build - Mark methods as deprecated ([4a223de](https://github.com/googleapis/google-cloud-go/commit/4a223de8eab072d95818c761e41fb3f3f6ac728c))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.2.0...osconfig/v1.3.0) (2022-02-14)


### Features

* **osconfig:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))
* **osconfig:** Update osconfig v1 protos ([61f23b2](https://github.com/googleapis/google-cloud-go/commit/61f23b2167dbe9e3e031db12ccf46b7eac639fa3))
* **osconfig:** Update v1beta protos with recently added features. PatchRollout proto, mig_instances_allowed field to PatchConfig, UpdatePatchDeployment RPC,PausePatchDeployment and ResumePatchDeployment pair of RPCs ([61f23b2](https://github.com/googleapis/google-cloud-go/commit/61f23b2167dbe9e3e031db12ccf46b7eac639fa3))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/osconfig/v1.1.0...osconfig/v1.2.0) (2022-01-04)


### Features

* **osconfig:** OSConfig: add OS policy assignment rpcs ([83b941c](https://www.github.com/googleapis/google-cloud-go/commit/83b941c0983e44fdd18ceee8c6f3e91219d72ad1))
* **osconfig:** Update OSConfig API ([e33350c](https://www.github.com/googleapis/google-cloud-go/commit/e33350cfcabcddcda1a90069383d39c68deb977a))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/osconfig/v1.0.0...osconfig/v1.1.0) (2021-11-02)


### Features

* **osconfig:** Update osconfig v1 and v1alpha RecurringSchedule.Frequency with DAILY frequency ([59e548a](https://www.github.com/googleapis/google-cloud-go/commit/59e548acc249c7bddd9c884c2af35d582a408c4d))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/osconfig/v0.1.0...osconfig/v0.2.0) (2021-09-11)

### Features

* **osconfig:** add OSConfigZonalService API Committer: [@jaiminsh](https://www.github.com/jaiminsh) ([d9ce9d0](https://www.github.com/googleapis/google-cloud-go/commit/d9ce9d0ee64f59c4e07ce4752bfd721051a95ac7))
* **osconfig:** Update osconfig v1 and v1alpha with WindowsApplication ([bf4378b](https://www.github.com/googleapis/google-cloud-go/commit/bf4378b5b859f7b835946891dbfebfee31c4b123))

## v0.1.0

This is the first tag to carve out osconfig as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
