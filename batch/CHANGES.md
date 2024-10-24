# Changelog


## [1.11.2](https://github.com/googleapis/google-cloud-go/compare/batch/v1.11.1...batch/v1.11.2) (2024-10-23)


### Bug Fixes

* **batch:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **batch:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.11.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.11.0...batch/v1.11.1) (2024-10-09)


### Documentation

* **batch:** Clarify Batch only supports global custom instance template now ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.10.0...batch/v1.11.0) (2024-09-12)


### Features

* **batch:** Promote block_project_ssh_keys support to batch v1 API ([2710d0f](https://github.com/googleapis/google-cloud-go/commit/2710d0f8c66c17f1ddb1d4cc287f7aeb701c0f72))


### Bug Fixes

* **batch:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.9.4...batch/v1.10.0) (2024-08-20)


### Features

* **batch:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))


### Documentation

* **batch:** Clarify tasks success criteria for background runnable ([#10702](https://github.com/googleapis/google-cloud-go/issues/10702)) ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.9.4](https://github.com/googleapis/google-cloud-go/compare/batch/v1.9.3...batch/v1.9.4) (2024-08-13)


### Documentation

* **batch:** Batch CentOS images and HPC CentOS images are EOS ([#10680](https://github.com/googleapis/google-cloud-go/issues/10680)) ([578ce14](https://github.com/googleapis/google-cloud-go/commit/578ce14068881a40d0c1d26d33b570973ec1a5bc))
* **batch:** Clarify required fields for Runnable.Container ([578ce14](https://github.com/googleapis/google-cloud-go/commit/578ce14068881a40d0c1d26d33b570973ec1a5bc))
* **batch:** Clarify required oneof fields for Runnable.Script ([578ce14](https://github.com/googleapis/google-cloud-go/commit/578ce14068881a40d0c1d26d33b570973ec1a5bc))
* **batch:** Clarify TaskSpec requires one or more runnables ([578ce14](https://github.com/googleapis/google-cloud-go/commit/578ce14068881a40d0c1d26d33b570973ec1a5bc))

## [1.9.3](https://github.com/googleapis/google-cloud-go/compare/batch/v1.9.2...batch/v1.9.3) (2024-08-08)


### Bug Fixes

* **batch:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))


### Documentation

* **batch:** Refine usage scope for fields `task_execution` and `task_state` in StatusEvent ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))

## [1.9.2](https://github.com/googleapis/google-cloud-go/compare/batch/v1.9.1...batch/v1.9.2) (2024-07-24)


### Bug Fixes

* **batch:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.9.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.9.0...batch/v1.9.1) (2024-07-10)


### Bug Fixes

* **batch:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.8...batch/v1.9.0) (2024-07-01)


### Features

* **batch:** Add a install_ops_agent field to InstancePolicyOrTemplate for Ops Agent support ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))


### Bug Fixes

* **batch:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))


### Documentation

* **batch:** Add instructions on how to configure cross-project pubsub publisher ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **batch:** Document default disk type ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **batch:** Update GCS description of volume.mount_options field ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **batch:** Update links in the description of volume.mount_options field ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))
* **batch:** Update list of volume.mount_options field ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))

## [1.8.8](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.7...batch/v1.8.8) (2024-06-26)


### Bug Fixes

* **batch:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.8.7](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.6...batch/v1.8.7) (2024-06-05)


### Documentation

* **batch:** Documentation improvements ([d5150d3](https://github.com/googleapis/google-cloud-go/commit/d5150d34eabac0218cbd16a9bbdaaaf019cf237d))

## [1.8.6](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.5...batch/v1.8.6) (2024-05-16)


### Documentation

* **batch:** Refine description for field `task_execution` ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))

## [1.8.5](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.4...batch/v1.8.5) (2024-05-01)


### Bug Fixes

* **batch:** Add internaloption.WithDefaultEndpointTemplate ([3b41408](https://github.com/googleapis/google-cloud-go/commit/3b414084450a5764a0248756e95e13383a645f90))
* **batch:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))


### Documentation

* **batch:** Update description on allowed_locations in LocationPolicy field ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **batch:** Update description on allowed_locations in LocationPolicy field ([#9777](https://github.com/googleapis/google-cloud-go/issues/9777)) ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))

## [1.8.4](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.3...batch/v1.8.4) (2024-04-15)


### Documentation

* **batch:** State one Resource Allowance per region per project limitation on v1alpha ([dbcdfd7](https://github.com/googleapis/google-cloud-go/commit/dbcdfd7843be16573b1683466852242a84891456))
* **batch:** Update comments on ServiceAccount email and scopes fields ([#9734](https://github.com/googleapis/google-cloud-go/issues/9734)) ([4d5a342](https://github.com/googleapis/google-cloud-go/commit/4d5a3429cec6816d50bdf284063dddf1971b79cf))

## [1.8.3](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.2...batch/v1.8.3) (2024-03-14)


### Bug Fixes

* **batch:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.8.2](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.1...batch/v1.8.2) (2024-03-04)


### Documentation

* **batch:** Update description of Job uid field ([d130d86](https://github.com/googleapis/google-cloud-go/commit/d130d861f55d137a2803340c2e11da3589669cb8))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.8.0...batch/v1.8.1) (2024-02-21)


### Documentation

* **batch:** Refine proto comment for run_as_non_root ([0195fe9](https://github.com/googleapis/google-cloud-go/commit/0195fe9292274ff9d86c71079a8e96ed2e5f9331))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.7.0...batch/v1.8.0) (2024-01-30)


### Features

* **batch:** Add `run_as_non_root` field to allow user's runnable be executed as non root ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))


### Bug Fixes

* **batch:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.6.3...batch/v1.7.0) (2023-11-27)


### Features

* **batch:** Add a CloudLoggingOption and use_generic_task_monitored_resource fields for users to opt out new batch monitored resource in cloud logging ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))

## [1.6.3](https://github.com/googleapis/google-cloud-go/compare/batch/v1.6.2...batch/v1.6.3) (2023-11-01)


### Bug Fixes

* **batch:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.6.2](https://github.com/googleapis/google-cloud-go/compare/batch/v1.6.1...batch/v1.6.2) (2023-10-31)


### Documentation

* **batch:** Update default max parallel tasks per job ([#8940](https://github.com/googleapis/google-cloud-go/issues/8940)) ([4d40180](https://github.com/googleapis/google-cloud-go/commit/4d40180da0557c2a2e9e2cb8b0509b429676bfc0))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.6.0...batch/v1.6.1) (2023-10-26)


### Bug Fixes

* **batch:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.5.1...batch/v1.6.0) (2023-10-17)


### Features

* **batch:** Expose display_name to batch v1 API ([e864fbc](https://github.com/googleapis/google-cloud-go/commit/e864fbcbc4f0a49dfdb04850b07451074c57edc8))

## [1.5.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.5.0...batch/v1.5.1) (2023-10-12)


### Bug Fixes

* **batch:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.4.1...batch/v1.5.0) (2023-10-04)


### Features

* **batch:** Add InstancePolicy.reservation field for restricting jobs to a specific reservation ([481127f](https://github.com/googleapis/google-cloud-go/commit/481127fb8271cab3a754e0e1820b32567e80524a))


### Documentation

* **batch:** Update batch PD interface support ([#8616](https://github.com/googleapis/google-cloud-go/issues/8616)) ([8729aa0](https://github.com/googleapis/google-cloud-go/commit/8729aa07f11e40482868d4dfe53c755dc49c3e43))

## [1.4.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.4.0...batch/v1.4.1) (2023-09-11)


### Documentation

* **batch:** Revert HTML formats in comments ([20725c8](https://github.com/googleapis/google-cloud-go/commit/20725c86c970ad24efa18c056fc3aa71dc3a4f03))
* **batch:** Update description on size_gb in disk field ([15be57b](https://github.com/googleapis/google-cloud-go/commit/15be57b9264a793494cedc3966034fa20f56d7c5))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.3.1...batch/v1.4.0) (2023-08-08)


### Features

* **batch:** Add comment to the unsupported order_by field of ListTasksRequest ([e3f8c89](https://github.com/googleapis/google-cloud-go/commit/e3f8c89429a207c05fee36d5d93efe76f9e29efe))
* **batch:** Clarify Batch API proto doc about pubsub notifications ([#8394](https://github.com/googleapis/google-cloud-go/issues/8394)) ([1639d62](https://github.com/googleapis/google-cloud-go/commit/1639d62202bc4b233ae83479cc1a539e083b67fe))

## [1.3.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.3.0...batch/v1.3.1) (2023-07-10)


### Documentation

* **batch:** Add image shortcut example for Batch HPC CentOS Image ([14b95d3](https://github.com/googleapis/google-cloud-go/commit/14b95d33753d0b391d0b49533e92b551e5dc3072))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.2.0...batch/v1.3.0) (2023-06-20)


### Features

* **batch:** Add support for scheduling_policy ([3382ef8](https://github.com/googleapis/google-cloud-go/commit/3382ef81b6bcefe1c7bfc14aa5ff9bbf25850966))


### Bug Fixes

* **batch:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.1.0...batch/v1.2.0) (2023-05-30)


### Features

* **batch:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/batch/v1.0.1...batch/v1.1.0) (2023-05-16)


### Features

* **batch:** Add support for placement policies ([#7943](https://github.com/googleapis/google-cloud-go/issues/7943)) ([7c2f642](https://github.com/googleapis/google-cloud-go/commit/7c2f642ac308fcdfcb41985aae425785afa27823))

## [1.0.1](https://github.com/googleapis/google-cloud-go/compare/batch/v1.0.0...batch/v1.0.1) (2023-05-08)


### Bug Fixes

* **batch:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.0.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.7.0...batch/v1.0.0) (2023-04-04)


### Features

* **batch:** Promote to GA ([597ea0f](https://github.com/googleapis/google-cloud-go/commit/597ea0fe09bcea04e884dffe78add850edb2120d))
* **batch:** Promote to GA ([#7645](https://github.com/googleapis/google-cloud-go/issues/7645)) ([307e5ad](https://github.com/googleapis/google-cloud-go/commit/307e5adfe93b9f0b66f2f4312f127bb74c102011))

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.6.0...batch/v0.7.0) (2023-02-14)


### Features

* **batch:** Support custom scopes for service account in v1 ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.5.0...batch/v0.6.0) (2023-01-04)


### Features

* **batch:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **batch:** Support secret and encrypted environment variables in v1 ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.4.1...batch/v0.5.0) (2022-12-01)


### Features

* **batch:** Adds named reservation to InstancePolicy ([4f0456e](https://github.com/googleapis/google-cloud-go/commit/4f0456eb3c8ed707774951c9418ffc2bf3fe5368))


### Documentation

* **batch:** fix minor docstring formatting ([7231644](https://github.com/googleapis/google-cloud-go/commit/7231644e71f05abc864924a0065b9ea22a489180))

## [0.4.1](https://github.com/googleapis/google-cloud-go/compare/batch/v0.4.0...batch/v0.4.1) (2022-11-16)


### Documentation

* **batch:** fix minor docstring formatting ([2b4957c](https://github.com/googleapis/google-cloud-go/commit/2b4957c7c348ecf5952e02f3602379fffaa758b4))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.3.0...batch/v0.4.0) (2022-11-03)


### Features

* **batch:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.2.1...batch/v0.3.0) (2022-10-25)


### Features

* **batch:** Enable install_gpu_drivers flag in v1 proto docs: Refine GPU drivers installation proto description docs: Refine comments for deprecated proto fields docs: Update the API comments about the device_name ([8b203b8](https://github.com/googleapis/google-cloud-go/commit/8b203b8aea4dada5c0846a515b14414cd8c58f78))
* **batch:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/batch/v0.2.0...batch/v0.2.1) (2022-09-08)


### Bug Fixes

* **batch:** mark service_account_email as deprecated docs: removing comment from a deprecated field ([e45ad9a](https://github.com/googleapis/google-cloud-go/commit/e45ad9af568c59151decc0dacedf137653b576dd))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/batch/v0.1.0...batch/v0.2.0) (2022-09-06)


### Features

* **batch:** environment variables, disk interfaces ([3bc37e2](https://github.com/googleapis/google-cloud-go/commit/3bc37e28626df5f7ec37b00c0c2f0bfb91c30495))

## 0.1.0 (2022-06-16)


### Features

* **batch:** start generating apiv1 ([#6145](https://github.com/googleapis/google-cloud-go/issues/6145)) ([41525fa](https://github.com/googleapis/google-cloud-go/commit/41525fab52da7e913f3593e89cef91c022898be3))
