# Changes


## [2.19.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.18.0...recaptchaenterprise/v2.19.0) (2024-11-14)


### Features

* **recaptchaenterprise:** A new enum `Challenge` is added ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))
* **recaptchaenterprise:** A new field `challenge` is added to message `.google.cloud.recaptchaenterprise.v1.RiskAnalysis` ([f329c4c](https://github.com/googleapis/google-cloud-go/commit/f329c4c7782fc5f52751235d969bb8de11616ec3))

## [2.18.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.17.3...recaptchaenterprise/v2.18.0) (2024-11-06)


### Features

* **recaptchaenterprise:** Enable Akamai web application firewall ([248026e](https://github.com/googleapis/google-cloud-go/commit/248026e7fad6ef6433f36c0205a9fe9e08b5545e))
* **recaptchaenterprise:** Support for ListIpOverrides and RemoveIpOverride ([#11054](https://github.com/googleapis/google-cloud-go/issues/11054)) ([248026e](https://github.com/googleapis/google-cloud-go/commit/248026e7fad6ef6433f36c0205a9fe9e08b5545e))


### Documentation

* **recaptchaenterprise:** Minor updates to reference documentation ([248026e](https://github.com/googleapis/google-cloud-go/commit/248026e7fad6ef6433f36c0205a9fe9e08b5545e))

## [2.17.3](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.17.2...recaptchaenterprise/v2.17.3) (2024-10-23)


### Bug Fixes

* **recaptchaenterprise:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **recaptchaenterprise:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [2.17.2](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.17.1...recaptchaenterprise/v2.17.2) (2024-10-09)


### Documentation

* **recaptchaenterprise:** Minor wording and branding adjustments ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [2.17.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.17.0...recaptchaenterprise/v2.17.1) (2024-09-12)


### Bug Fixes

* **recaptchaenterprise/v2:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))


### Documentation

* **recaptchaenterprise:** Fix CreateAssessmentRequest comment ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))
* **recaptchaenterprise:** Minor doc fixes ([#10778](https://github.com/googleapis/google-cloud-go/issues/10778)) ([b3ea577](https://github.com/googleapis/google-cloud-go/commit/b3ea5776b171fc60b4e96035d56d35dbd7505f3b))
* **recaptchaenterprise:** Update API title in docs overview ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))

## [2.17.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.16.0...recaptchaenterprise/v2.17.0) (2024-08-27)


### Features

* **recaptchaenterprise:** Add AssessmentEnvironment for CreateAssessement to explicitly describe the environment of the assessment ([#10777](https://github.com/googleapis/google-cloud-go/issues/10777)) ([78c4aca](https://github.com/googleapis/google-cloud-go/commit/78c4aca925e40b4cf80fb4912e63b4623d392778))

## [2.16.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.15.0...recaptchaenterprise/v2.16.0) (2024-08-23)


### Features

* **recaptchaenterprise:** Add `express_settings` to `Key` ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))
* **recaptchaenterprise:** Add AddIpOverride RPC ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))


### Documentation

* **recaptchaenterprise:** Clarify `Event.express` field ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))
* **recaptchaenterprise:** Fix billing, quota, and usecase links ([946a5fc](https://github.com/googleapis/google-cloud-go/commit/946a5fcfeb85e22b1d8e995cda6b18b745459656))

## [2.15.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.14.3...recaptchaenterprise/v2.15.0) (2024-08-20)


### Features

* **recaptchaenterprise:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [2.14.3](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.14.2...recaptchaenterprise/v2.14.3) (2024-08-08)


### Bug Fixes

* **recaptchaenterprise:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [2.14.2](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.14.1...recaptchaenterprise/v2.14.2) (2024-07-24)


### Bug Fixes

* **recaptchaenterprise/v2:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [2.14.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.14.0...recaptchaenterprise/v2.14.1) (2024-07-10)


### Bug Fixes

* **recaptchaenterprise/v2:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [2.14.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.13.1...recaptchaenterprise/v2.14.0) (2024-07-01)


### Features

* **recaptchaenterprise:** Added SMS Toll Fraud assessment ([eec7a3b](https://github.com/googleapis/google-cloud-go/commit/eec7a3b5c00fc18076f410ddc4910cdcc61c702c))


### Bug Fixes

* **recaptchaenterprise/v2:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [2.13.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.13.0...recaptchaenterprise/v2.13.1) (2024-06-26)


### Bug Fixes

* **recaptchaenterprise:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [2.13.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.12.0...recaptchaenterprise/v2.13.0) (2024-05-01)


### Features

* **recaptchaenterprise:** Add Fraud Prevention settings field ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **recaptchaenterprise:** Add Fraud Prevention settings field ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Bug Fixes

* **recaptchaenterprise:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))


### Documentation

* **recaptchaenterprise:** Fixed the description of ListFirewallPoliciesResponse ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))

## [2.12.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.11.1...recaptchaenterprise/v2.12.0) (2024-03-25)


### Features

* **recaptchaenterprise:** Existing resource_reference option of the field name is removed from message `google.cloud.recaptchaenterprise.v1.RelatedAccountGroupMemberShip` ([94f9463](https://github.com/googleapis/google-cloud-go/commit/94f9463f890ed886622ee65edfbc4b5ecdfa97f8))

## [2.11.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.11.0...recaptchaenterprise/v2.11.1) (2024-03-14)


### Bug Fixes

* **recaptchaenterprise:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [2.11.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.10.0...recaptchaenterprise/v2.11.0) (2024-03-07)


### Features

* **recaptchaenterprise:** Add include_recaptcha_script for as a new action in firewall policies ([a74cbbe](https://github.com/googleapis/google-cloud-go/commit/a74cbbee6be0c02e0280f115119596da458aa707))

## [2.10.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.9.2...recaptchaenterprise/v2.10.0) (2024-02-21)


### Features

* **recaptchaenterprise:** Add an API method for reordering firewall policies ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))

## [2.9.2](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.9.1...recaptchaenterprise/v2.9.2) (2024-01-30)


### Bug Fixes

* **recaptchaenterprise:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [2.9.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.9.0...recaptchaenterprise/v2.9.1) (2024-01-22)


### Documentation

* **recaptchaenterprise:** Update comment for `AccountVerificationInfo.username` ([00b9900](https://github.com/googleapis/google-cloud-go/commit/00b990061592a20a181e61faa6964b45205b76a7))

## [2.9.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.8.4...recaptchaenterprise/v2.9.0) (2023-11-27)


### Features

* **recaptchaenterprise:** Added AnnotateAssessmentRequest.account_id ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))


### Documentation

* **recaptchaenterprise:** Minor comments updates ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))

## [2.8.4](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.8.3...recaptchaenterprise/v2.8.4) (2023-11-09)


### Bug Fixes

* **recaptchaenterprise:** Added required annotations ([ba23673](https://github.com/googleapis/google-cloud-go/commit/ba23673da7707c31292e4aa29d65b7ac1446d4a6))

## [2.8.3](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.8.2...recaptchaenterprise/v2.8.3) (2023-11-01)


### Bug Fixes

* **recaptchaenterprise:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [2.8.2](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.8.1...recaptchaenterprise/v2.8.2) (2023-10-26)


### Bug Fixes

* **recaptchaenterprise:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [2.8.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.8.0...recaptchaenterprise/v2.8.1) (2023-10-12)


### Bug Fixes

* **recaptchaenterprise:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [2.8.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.7.2...recaptchaenterprise/v2.8.0) (2023-10-04)


### Features

* **recaptchaenterprise:** Added FraudPreventionAssessment.behavioral_trust_verdict ([481127f](https://github.com/googleapis/google-cloud-go/commit/481127fb8271cab3a754e0e1820b32567e80524a))
* **recaptchaenterprise:** FirewallPolicy CRUD API ([#8635](https://github.com/googleapis/google-cloud-go/issues/8635)) ([481127f](https://github.com/googleapis/google-cloud-go/commit/481127fb8271cab3a754e0e1820b32567e80524a))

## [2.7.2](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise-v2.7.1...recaptchaenterprise/v2.7.2) (2023-06-20)


### Bug Fixes

* **recaptchaenterprise:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [2.7.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.7.0...recaptchaenterprise/v2.7.1) (2023-05-08)


### Bug Fixes

* **recaptchaenterprise/v2:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))
* **recaptchaenterprise:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [2.7.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.6.0...recaptchaenterprise/v2.7.0) (2023-03-22)


### Features

* **recaptchaenterprise/v2:** Add reCAPTCHA Enterprise Fraud Prevention API ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))
* **recaptchaenterprise/v2:** Add reCAPTCHA Enterprise Fraud Prevention API ([00fff3a](https://github.com/googleapis/google-cloud-go/commit/00fff3a58bed31274ab39af575876dab91d708c9))

## [2.6.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.5.0...recaptchaenterprise/v2.6.0) (2023-01-04)


### Features

* **recaptchaenterprise/v2:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [2.5.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.4.0...recaptchaenterprise/v2.5.0) (2022-11-03)


### Features

* **recaptchaenterprise/v2:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [2.4.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.3.0...recaptchaenterprise/v2.4.0) (2022-10-25)


### Features

* **recaptchaenterprise/v2:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [2.3.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.2.0...recaptchaenterprise/v2.3.0) (2022-10-14)


### Features

* **recaptchaenterprise/v2:** add RetrieveLegacySecretKey method feat: add annotation reasons REFUND, REFUND_FRAUD, TRANSACTION_ACCEPTED, TRANSACTION_DECLINED and SOCIAL_SPAM ([de4e16a](https://github.com/googleapis/google-cloud-go/commit/de4e16a498354ea7271f5b396f7cb2bb430052aa))

## [2.2.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.1.0...recaptchaenterprise/v2.2.0) (2022-09-21)


### Features

* **recaptchaenterprise:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [2.1.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.0.1...recaptchaenterprise/v2.1.0) (2022-09-20)


### Features

* **recaptchaenterprise/v2:** start generating apiv1beta1 ([4aa2f48](https://github.com/googleapis/google-cloud-go/commit/4aa2f48eeb2b37124b207d3567f2b66f567797a8))

## [2.0.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v2.0.0...recaptchaenterprise/v2.0.1) (2022-06-16)


### Bug Fixes

* **recaptchaenterprise/v2:** set the right field number for reCAPTCHA private password leak ([5e46068](https://github.com/googleapis/google-cloud-go/commit/5e46068329153daf5aa590a6415d4764f1ab2b90))

## [2.0.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v1.3.1...recaptchaenterprise/v2.0.0) (2022-05-24)


### ⚠ BREAKING CHANGES

* **recaptchaenterprise/v2:** parent changed to project in googleapis/go-genproto#808

### Features

* **recaptchaenterprise/v2:** Release breaking changes as v2 module ([#6062](https://github.com/googleapis/google-cloud-go/issues/6062)) ([1266896](https://github.com/googleapis/google-cloud-go/commit/1266896827d1b788931f348c399ef1fb6fd33ef7))

### [1.3.1](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v1.3.0...recaptchaenterprise/v1.3.1) (2022-05-03)


### Bug Fixes

* **recaptchaenterprise:** remove key management API feat: introduced Reason, PasswordLeakVerification, AccountDefenderAssessment ([380529e](https://github.com/googleapis/google-cloud-go/commit/380529ef939c7019458b2dda2b789770376aff19))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v1.2.0...recaptchaenterprise/v1.3.0) (2022-02-23)


### Features

* **recaptchaenterprise:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v1.1.0...recaptchaenterprise/v1.2.0) (2022-02-14)


### Features

* **recaptchaenterprise:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v1.0.0...recaptchaenterprise/v1.1.0) (2022-01-04)


### Features

* **recaptchaenterprise:** add new reCAPTCHA Enterprise fraud annotations ([3dd34a2](https://www.github.com/googleapis/google-cloud-go/commit/3dd34a262edbff63b9aece8faddc2ff0d98ce42a))
* **recaptchaenterprise:** add reCAPTCHA Enterprise account defender API methods ([88a1cdb](https://www.github.com/googleapis/google-cloud-go/commit/88a1cdbef3cc337354a61bc9276725bfb9a686d8))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/recaptchaenterprise/v0.1.0...recaptchaenterprise/v0.2.0) (2021-09-18)


### Features

* **recaptchaenterprise:** add GetMetrics and MigrateKey methods to reCAPTCHA enterprise API ([829f15a](https://www.github.com/googleapis/google-cloud-go/commit/829f15a01da2a564a05ee980b994c56d9fad9c95))

## v0.1.0

This is the first tag to carve out recaptchaenterprise as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
