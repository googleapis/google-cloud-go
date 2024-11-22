# Changes


## [1.25.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.25.0...deploy/v1.25.1) (2024-11-21)


### Documentation

* **deploy:** Minor documentation updates ([1036734](https://github.com/googleapis/google-cloud-go/commit/1036734d387691f6264bd7a51c9e19567815a3d2))

## [1.25.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.24.0...deploy/v1.25.0) (2024-11-14)


### Features

* **deploy:** A new field `timed_promote_release_condition` is added to message `.google.cloud.deploy.v1.AutomationRuleCondition` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **deploy:** A new field `timed_promote_release_operation` is added to message `.google.cloud.deploy.v1.AutomationRun` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **deploy:** A new field `timed_promote_release_rule` is added to message `.google.cloud.deploy.v1.AutomationRule` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **deploy:** A new message `TimedPromoteReleaseCondition` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **deploy:** A new message `TimedPromoteReleaseOperation` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **deploy:** A new message `TimedPromoteReleaseRule` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))


### Documentation

* **deploy:** A comment for field `target_id` in message `.google.cloud.deploy.v1.AutomationRun` is changed ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.23.1...deploy/v1.24.0) (2024-11-06)


### Features

* **deploy:** Added new fields for the Automation Repair rule ([2c83297](https://github.com/googleapis/google-cloud-go/commit/2c83297a569117b0252b5b2edaecb09e4924d979))
* **deploy:** Added route destination related fields to Gateway service mesh message ([2c83297](https://github.com/googleapis/google-cloud-go/commit/2c83297a569117b0252b5b2edaecb09e4924d979))

## [1.23.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.23.0...deploy/v1.23.1) (2024-10-23)


### Bug Fixes

* **deploy:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **deploy:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.22.1...deploy/v1.23.0) (2024-10-09)


### Features

* **deploy:** Added support for deploy policies ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **deploy:** Minor documentation updates ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.22.0...deploy/v1.22.1) (2024-09-12)


### Bug Fixes

* **deploy:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.21.2...deploy/v1.22.0) (2024-08-20)


### Features

* **deploy:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.21.2](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.21.1...deploy/v1.21.2) (2024-08-13)


### Documentation

* **deploy:** Very minor documentation updates ([564c355](https://github.com/googleapis/google-cloud-go/commit/564c355c6dfbf5a1033a04c8f48135f5d937592b))

## [1.21.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.21.0...deploy/v1.21.1) (2024-08-08)


### Bug Fixes

* **deploy:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.20.0...deploy/v1.21.0) (2024-08-01)


### Features

* **deploy:** Add support for different Pod selector labels when doing canaries ([#10581](https://github.com/googleapis/google-cloud-go/issues/10581)) ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))


### Bug Fixes

* **deploy:** Make changes to an API that is disabled on the server, but whose client libraries were prematurely published ([5b4b0f7](https://github.com/googleapis/google-cloud-go/commit/5b4b0f7878276ab5709011778b1b4a6ffd30a60b))
* **deploy:** Remove an API that was mistakenly made public ([123c886](https://github.com/googleapis/google-cloud-go/commit/123c8861625142b1d58605c008355bc569a3b47b))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.19.3...deploy/v1.20.0) (2024-07-24)


### Features

* **deploy:** Added support for configuring a proxy_url to a Kubernetes server ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))
* **deploy:** Added support for deploy policies ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))
* **deploy:** Added support for new custom target type and deploy policy platform logs ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))


### Bug Fixes

* **deploy:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **deploy:** Small Cloud Deploy API documentation updates ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))
* **deploy:** Small corrections to Cloud Deploy API documentation ([1bb4c84](https://github.com/googleapis/google-cloud-go/commit/1bb4c846ec1ff37f394afb1684823ea76c18d16e))

## [1.19.3](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.19.2...deploy/v1.19.3) (2024-07-10)


### Bug Fixes

* **deploy:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.19.1...deploy/v1.19.2) (2024-07-01)


### Bug Fixes

* **deploy:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.19.0...deploy/v1.19.1) (2024-06-26)


### Bug Fixes

* **deploy:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.18.1...deploy/v1.19.0) (2024-05-16)


### Features

* **deploy:** Add Skaffold verbose support to Execution Environment properties ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.18.0...deploy/v1.18.1) (2024-05-08)


### Documentation

* **deploy:** Small corrections to Cloud Deploy API documentation ([ae42f23](https://github.com/googleapis/google-cloud-go/commit/ae42f23f586ad76b058066a66c1566e4fef23692))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.17.2...deploy/v1.18.0) (2024-05-01)


### Features

* **deploy:** Add Skaffold remote config support for GCB repos ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Bug Fixes

* **deploy:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.17.2](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.17.1...deploy/v1.17.2) (2024-03-14)


### Bug Fixes

* **deploy:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.17.0...deploy/v1.17.1) (2024-01-30)


### Bug Fixes

* **deploy:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.16.0...deploy/v1.17.0) (2024-01-08)


### Features

* **deploy:** Add stable cutback duration configuration to the k8s gateway service mesh deployment strategy. This allows configuring the amount of time to migrate traffic back to the original Service in the stable phase ([#9227](https://github.com/googleapis/google-cloud-go/issues/9227)) ([bd30055](https://github.com/googleapis/google-cloud-go/commit/bd3005532fbffa9894b11149e9693b7c33227d79))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.15.0...deploy/v1.16.0) (2023-12-07)


### Features

* **deploy:** Add custom target type support ([5132d0f](https://github.com/googleapis/google-cloud-go/commit/5132d0fea3a5ac902a2c9eee865241ed4509a5f4))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.14.2...deploy/v1.15.0) (2023-11-09)


### Features

* **deploy:** Add Automation API and Rollback API ([b44c4b3](https://github.com/googleapis/google-cloud-go/commit/b44c4b301a91e8d4d107be6056b49a8fbdac9003))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.14.1...deploy/v1.14.2) (2023-11-01)


### Bug Fixes

* **deploy:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.14.0...deploy/v1.14.1) (2023-10-26)


### Bug Fixes

* **deploy:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.13.1...deploy/v1.14.0) (2023-10-19)


### Features

* **deploy:** Added platform log RolloutUpdateEvent ([f3e2b05](https://github.com/googleapis/google-cloud-go/commit/f3e2b05129582f599fa9f53598f0cd7abe177493))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.13.0...deploy/v1.13.1) (2023-10-12)


### Bug Fixes

* **deploy:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.12.1...deploy/v1.13.0) (2023-07-27)


### Features

* **deploy:** Added support for predeploy and postdeploy actions ([#8337](https://github.com/googleapis/google-cloud-go/issues/8337)) ([c51d006](https://github.com/googleapis/google-cloud-go/commit/c51d0064faadd77f39843b40231efc248a7f675a))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.12.0...deploy/v1.12.1) (2023-07-24)


### Documentation

* **deploy:** Small documentation updates ([#8286](https://github.com/googleapis/google-cloud-go/issues/8286)) ([eca3c90](https://github.com/googleapis/google-cloud-go/commit/eca3c9070cd96a50fa857a6c016e35a98dbea5e7))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.11.0...deploy/v1.12.0) (2023-07-18)


### Features

* **deploy:** Added support routeUpdateWaitTime field for Deployment Strategies ([dda1f9d](https://github.com/googleapis/google-cloud-go/commit/dda1f9dc2f5b54dca15ae05302d8cac821fe8da1))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.10.1...deploy/v1.11.0) (2023-06-27)


### Features

* **deploy:** Add deploy parameters for cloud deploy ([94ea341](https://github.com/googleapis/google-cloud-go/commit/94ea3410e233db6040a7cb0a931948f1e3bb4c9a))

## [1.10.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.10.0...deploy/v1.10.1) (2023-06-20)


### Bug Fixes

* **deploy:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.9.0...deploy/v1.10.0) (2023-06-07)


### Features

* **deploy:** Add support for disabling Pod overprovisioning in the progressive deployment strategy configuration for a Kubernetes Target ([#8052](https://github.com/googleapis/google-cloud-go/issues/8052)) ([f2c3dd3](https://github.com/googleapis/google-cloud-go/commit/f2c3dd38fce43f15f4d3a4da5d621de79e174475))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.8.1...deploy/v1.9.0) (2023-05-30)


### Features

* **deploy:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.8.0...deploy/v1.8.1) (2023-05-08)


### Bug Fixes

* **deploy:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.7.0...deploy/v1.8.0) (2023-03-22)


### Features

* **deploy:** Added supported for Cloud Deploy Progressive Deployment Strategy ([c967961](https://github.com/googleapis/google-cloud-go/commit/c967961ed95750e173af0193ec8d0974471f43ff))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.6.0...deploy/v1.7.0) (2023-03-15)


### Features

* **deploy:** Update iam and longrunning deps ([91a1f78](https://github.com/googleapis/google-cloud-go/commit/91a1f784a109da70f63b96414bba8a9b4254cddd))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.5.0...deploy/v1.6.0) (2023-01-04)


### Features

* **deploy:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.4.0...deploy/v1.5.0) (2022-11-03)


### Features

* **deploy:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.3.0...deploy/v1.4.0) (2022-10-25)


### Features

* **deploy:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.2.1...deploy/v1.3.0) (2022-10-14)


### Features

* **deploy:** Publish new JobRun resource and associated methods for Google Cloud Deploy ([ce3f945](https://github.com/googleapis/google-cloud-go/commit/ce3f9458e511eca0910992763232abbcd64698f1))


### Bug Fixes

* **deploy:** Fix resource annotations for Cloud Deploy to use common resource name for locations ([ce3f945](https://github.com/googleapis/google-cloud-go/commit/ce3f9458e511eca0910992763232abbcd64698f1))

## [1.2.1](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.2.0...deploy/v1.2.1) (2022-07-12)


### Documentation

* **deploy:** Cloud Deploy API Platform Logging documentation ([6ffce1d](https://github.com/googleapis/google-cloud-go/commit/6ffce1dbf567758d23ac39aaf63dc17ced5e4db9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.1.0...deploy/v1.2.0) (2022-02-23)


### Features

* **deploy:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/deploy/v1.0.0...deploy/v1.1.0) (2022-02-14)


### Features

* **deploy:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.0.0](https://www.github.com/googleapis/google-cloud-go/compare/deploy/v0.1.0...deploy/v1.0.0) (2022-01-25)


### Features

* **deploy:** to v1 ([#5140](https://www.github.com/googleapis/google-cloud-go/issues/5140)) ([74c389e](https://www.github.com/googleapis/google-cloud-go/commit/74c389e26c1ce8b0ce9ede7b298c6a8a9d106094))

## v0.1.0

- feat(deploy): start generating clients
