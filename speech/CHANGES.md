# Changes

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.9.0...speech/v1.10.0) (2022-12-15)


### Features

* **speech:** Added new fields to facilitate debugging * Added new field to Speech response proto, to give more information to indicate whether, or not, Biasing was applied (eg. did Biasing application timed out). * Added request_id to Speech response protos. ([bf75547](https://github.com/googleapis/google-cloud-go/commit/bf75547278ef342c79b958e886925da553b0bcc2))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.8.0...speech/v1.9.0) (2022-11-03)


### Features

* **speech:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.7.0...speech/v1.8.0) (2022-10-25)


### Features

* **speech:** Start generating apiv2 ([#6891](https://github.com/googleapis/google-cloud-go/issues/6891)) ([1c7e02f](https://github.com/googleapis/google-cloud-go/commit/1c7e02f6871d3fbd5475a549405ba5b94fd28100))
* **speech:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.6.0...speech/v1.7.0) (2022-09-21)


### Features

* **speech:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.5.0...speech/v1.6.0) (2022-09-19)


### Features

* **speech:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.4.0...speech/v1.5.0) (2022-06-29)


### Features

* **speech:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.3.1...speech/v1.4.0) (2022-05-24)


### Features

* **speech:** Add adaptation proto for v1 api ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

### [1.3.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.3.0...speech/v1.3.1) (2022-04-14)


### Bug Fixes

* **speech:** use full link in comment to fix JSDoc broken link ([19a9ef2](https://github.com/googleapis/google-cloud-go/commit/19a9ef2d9b8d77d3bc3e4c11c7f1f3e47700edd4))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.2.0...speech/v1.3.0) (2022-02-23)


### Features

* **speech:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.1.0...speech/v1.2.0) (2022-02-14)


### Features

* **speech:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/speech/v1.0.0...speech/v1.1.0) (2022-01-04)


### Features

* **speech:** add result_end_time to SpeechRecognitionResult ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))
* **speech:** added alternative_language_codes to RecognitionConfig feat: WEBM_OPUS codec feat: SpeechAdaptation configuration feat: word confidence feat: spoken punctuation and spoken emojis feat: hint boost in SpeechContext ([a2c0bef](https://www.github.com/googleapis/google-cloud-go/commit/a2c0bef551489c9f1d0d12b973d3bf095354841e))

## 1.0.0

Stabilize GA surface.

## [0.2.0](https://www.github.com/googleapis/google-cloud-go/compare/speech/v0.1.0...speech/v0.2.0) (2021-08-30)


### Features

* **speech:** Add transcript normalization ([b31646d](https://www.github.com/googleapis/google-cloud-go/commit/b31646d1e12037731df4b5c0ba9f60b6434d7b9b))

## v0.1.0

This is the first tag to carve out speech as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
