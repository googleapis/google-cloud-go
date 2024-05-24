# Changes


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
