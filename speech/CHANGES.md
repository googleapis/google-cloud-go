# Changes

## [1.20.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.20.0...speech/v1.20.1) (2023-11-01)


### Bug Fixes

* **speech:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.19.2...speech/v1.20.0) (2023-10-31)


### Features

* **speech:** Add transcript normalization + m4a audio format support ([4d40180](https://github.com/googleapis/google-cloud-go/commit/4d40180da0557c2a2e9e2cb8b0509b429676bfc0))

## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/speech/v1.19.1...speech/v1.19.2) (2023-10-26)


### Bug Fixes

* **speech:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.19.0...speech/v1.19.1) (2023-10-12)


### Bug Fixes

* **speech:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.18.0...speech/v1.19.0) (2023-07-18)


### Features

* **speech:** Promote to GA ([#8268](https://github.com/googleapis/google-cloud-go/issues/8268)) ([d9bb34f](https://github.com/googleapis/google-cloud-go/commit/d9bb34f1f83db94c4e07824b2158ff3c994821d8))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.17.1...speech/v1.18.0) (2023-07-10)


### Features

* **speech:** Add `model` and `language_codes` fields in `RecognitionConfig` message + enable default `_` recognizer ([#8204](https://github.com/googleapis/google-cloud-go/issues/8204)) ([f41d56f](https://github.com/googleapis/google-cloud-go/commit/f41d56f2f5b1fa3d47be48874fece70136382a45))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.17.0...speech/v1.17.1) (2023-06-20)


### Bug Fixes

* **speech:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.16.0...speech/v1.17.0) (2023-05-30)


### Features

* **speech:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.15.1...speech/v1.16.0) (2023-05-16)


### Features

* **speech:** Add processing strategy to batch recognition requests ([#7900](https://github.com/googleapis/google-cloud-go/issues/7900)) ([31421d5](https://github.com/googleapis/google-cloud-go/commit/31421d52c3bf3b7baa235fb6cb18bb8a786398df))

## [1.15.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.15.0...speech/v1.15.1) (2023-05-08)


### Bug Fixes

* **speech:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.14.1...speech/v1.15.0) (2023-03-22)


### Features

* **speech:** Add support for BatchRecognize ([c967961](https://github.com/googleapis/google-cloud-go/commit/c967961ed95750e173af0193ec8d0974471f43ff))


### Documentation

* **speech:** Fix the resource name format in comment for CreatePhraseSetRequest ([c967961](https://github.com/googleapis/google-cloud-go/commit/c967961ed95750e173af0193ec8d0974471f43ff))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.14.0...speech/v1.14.1) (2023-02-14)


### Documentation

* **speech:** Clarified boost usage ([f1c3ec7](https://github.com/googleapis/google-cloud-go/commit/f1c3ec753259c5c5d083f1f06960f77327b7ca61))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/speech-v1.13.0...speech/v1.14.0) (2023-01-26)


### Features

* **speech:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **speech:** Added ABNF Grammars field in Speech Adaptation     * Added a new field to Speech Adaptation to specify ABNF grammar       definitions ([3115df4](https://github.com/googleapis/google-cloud-go/commit/3115df407cd4876d58c79e726308e9f229ceb6ed))
* **speech:** Added new fields to facilitate debugging * Added new field to Speech response proto, to give more information to indicate whether, or not, Biasing was applied (eg. did Biasing application timed out). * Added request_id to Speech response protos. ([bf75547](https://github.com/googleapis/google-cloud-go/commit/bf75547278ef342c79b958e886925da553b0bcc2))
* **speech:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **speech:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **speech:** Start generating apiv2 ([#6891](https://github.com/googleapis/google-cloud-go/issues/6891)) ([1c7e02f](https://github.com/googleapis/google-cloud-go/commit/1c7e02f6871d3fbd5475a549405ba5b94fd28100))
* **speech:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **speech:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **speech:** Clarify boost usage in Reference ([2359f92](https://github.com/googleapis/google-cloud-go/commit/2359f92ed3109415a3aed8d1feb15d1f360f3cd7))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/speech-v1.12.1...speech/v1.13.0) (2023-01-26)


### Features

* **speech:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **speech:** Added ABNF Grammars field in Speech Adaptation     * Added a new field to Speech Adaptation to specify ABNF grammar       definitions ([3115df4](https://github.com/googleapis/google-cloud-go/commit/3115df407cd4876d58c79e726308e9f229ceb6ed))
* **speech:** Added new fields to facilitate debugging * Added new field to Speech response proto, to give more information to indicate whether, or not, Biasing was applied (eg. did Biasing application timed out). * Added request_id to Speech response protos. ([bf75547](https://github.com/googleapis/google-cloud-go/commit/bf75547278ef342c79b958e886925da553b0bcc2))
* **speech:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **speech:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **speech:** Start generating apiv2 ([#6891](https://github.com/googleapis/google-cloud-go/issues/6891)) ([1c7e02f](https://github.com/googleapis/google-cloud-go/commit/1c7e02f6871d3fbd5475a549405ba5b94fd28100))
* **speech:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **speech:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **speech:** Clarify boost usage in Reference ([2359f92](https://github.com/googleapis/google-cloud-go/commit/2359f92ed3109415a3aed8d1feb15d1f360f3cd7))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/speech/v1.12.0...speech/v1.12.1) (2023-01-26)


### Documentation

* **speech:** Clarify boost usage in Reference ([2359f92](https://github.com/googleapis/google-cloud-go/commit/2359f92ed3109415a3aed8d1feb15d1f360f3cd7))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.11.0...speech/v1.12.0) (2023-01-10)


### Features

* **speech:** Added ABNF Grammars field in Speech Adaptation     * Added a new field to Speech Adaptation to specify ABNF grammar       definitions ([3115df4](https://github.com/googleapis/google-cloud-go/commit/3115df407cd4876d58c79e726308e9f229ceb6ed))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/speech/v1.10.0...speech/v1.11.0) (2023-01-04)


### Features

* **speech:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

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
