# Changes

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.14.1...osconfig/v1.14.2) (2024-10-23)


### Bug Fixes

* **osconfig:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **osconfig:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.14.0...osconfig/v1.14.1) (2024-09-12)


### Bug Fixes

* **osconfig:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.13.3...osconfig/v1.14.0) (2024-08-20)


### Features

* **osconfig:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.13.3](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.13.2...osconfig/v1.13.3) (2024-08-08)


### Bug Fixes

* **osconfig:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.13.2](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.13.1...osconfig/v1.13.2) (2024-07-24)


### Bug Fixes

* **osconfig:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.13.0...osconfig/v1.13.1) (2024-07-10)


### Bug Fixes

* **osconfig:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.8...osconfig/v1.13.0) (2024-07-01)


### Features

* **osconfig/agentendpoint:** Add data about source of the package to VersionedPackage ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))


### Bug Fixes

* **osconfig:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))


### Documentation

* **osconfig/agentendpoint:** A comment for enum `Interpreter` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for enum value `INTERPRETER_UNSPECIFIED` in enum `Interpreter` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for enum value `NONE` in enum `Interpreter` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for enum value `POWERSHELL` in enum `Interpreter` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for enum value `SHELL` in enum `Interpreter` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `archive_type` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `components` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `desired_state` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `exit_code` in message `.google.cloud.osconfig.agentendpoint.v1.ExecStepTaskOutput` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `id` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `id` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `inventory_checksum` in message `.google.cloud.osconfig.agentendpoint.v1.ReportInventoryRequest` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `inventory` in message `.google.cloud.osconfig.agentendpoint.v1.ReportInventoryRequest` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `task_type` in message `.google.cloud.osconfig.agentendpoint.v1.ReportTaskProgressRequest` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `uri` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))
* **osconfig/agentendpoint:** A comment for field `validate` in message `.google.cloud.osconfig.agentendpoint.v1.OSPolicy` is changed ([6a9c12a](https://github.com/googleapis/google-cloud-go/commit/6a9c12a395245d8500c267437c2dfa897049a719))

## [1.12.8](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.7...osconfig/v1.12.8) (2024-06-26)


### Bug Fixes

* **osconfig:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.12.7](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.6...osconfig/v1.12.7) (2024-05-01)


### Bug Fixes

* **osconfig:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.12.6](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.5...osconfig/v1.12.6) (2024-03-14)


### Bug Fixes

* **osconfig:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.12.5](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.4...osconfig/v1.12.5) (2024-01-30)


### Bug Fixes

* **osconfig:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.12.4](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.3...osconfig/v1.12.4) (2023-11-01)


### Bug Fixes

* **osconfig:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.12.3](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.2...osconfig/v1.12.3) (2023-10-26)


### Bug Fixes

* **osconfig:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.12.2](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.1...osconfig/v1.12.2) (2023-10-12)


### Bug Fixes

* **osconfig:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.12.0...osconfig/v1.12.1) (2023-06-20)


### Bug Fixes

* **osconfig:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.11.1...osconfig/v1.12.0) (2023-05-30)


### Features

* **osconfig:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.11.0...osconfig/v1.11.1) (2023-05-08)


### Bug Fixes

* **osconfig:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/osconfig/v1.10.0...osconfig/v1.11.0) (2023-01-04)


### Features

* **osconfig:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

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
