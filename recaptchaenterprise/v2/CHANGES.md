# Changes

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
