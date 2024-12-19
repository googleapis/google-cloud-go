# Changes

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.9.0...texttospeech/v1.10.0) (2024-11-06)


### Features

* **texttospeech:** Add multi-speaker markup, which allows generating dialogue between multiple speakers ([2c83297](https://github.com/googleapis/google-cloud-go/commit/2c83297a569117b0252b5b2edaecb09e4924d979))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.8.1...texttospeech/v1.9.0) (2024-10-23)


### Features

* **texttospeech:** Add brand voice lite, which lets you clone a voice with just 10 seconds of audio ([#11014](https://github.com/googleapis/google-cloud-go/issues/11014)) ([6e69d2e](https://github.com/googleapis/google-cloud-go/commit/6e69d2e85849002bad227ea5bebcde9199605bef))
* **texttospeech:** Add CustomPronunciationParams for upcoming feature work ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))
* **texttospeech:** Add low latency journey option to proto ([f0b05e2](https://github.com/googleapis/google-cloud-go/commit/f0b05e260435d5e8889b9a0ca0ab215fcde169ab))


### Bug Fixes

* **texttospeech:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **texttospeech:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [1.8.1](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.8.0...texttospeech/v1.8.1) (2024-09-12)


### Bug Fixes

* **texttospeech:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.12...texttospeech/v1.8.0) (2024-08-20)


### Features

* **texttospeech:** A new method `StreamingSynthesize` is added to service `TextToSpeech` ([#10687](https://github.com/googleapis/google-cloud-go/issues/10687)) ([b32cb37](https://github.com/googleapis/google-cloud-go/commit/b32cb378ab03f34c0670a8a204bd0ef3f71d48d4))
* **texttospeech:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))


### Documentation

* **texttospeech:** A comment for field `name` in message `.google.cloud.texttospeech.v1.VoiceSelectionParams` is changed ([b32cb37](https://github.com/googleapis/google-cloud-go/commit/b32cb378ab03f34c0670a8a204bd0ef3f71d48d4))
* **texttospeech:** Update Long Audio capabilities to include SSML ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.7.12](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.11...texttospeech/v1.7.12) (2024-08-08)


### Bug Fixes

* **texttospeech:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.7.11](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.10...texttospeech/v1.7.11) (2024-07-24)


### Bug Fixes

* **texttospeech:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.7.10](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.9...texttospeech/v1.7.10) (2024-07-10)


### Bug Fixes

* **texttospeech:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.7.9](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.8...texttospeech/v1.7.9) (2024-07-01)


### Bug Fixes

* **texttospeech:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.7.8](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.7...texttospeech/v1.7.8) (2024-06-26)


### Bug Fixes

* **texttospeech:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.7.7](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.6...texttospeech/v1.7.7) (2024-05-01)


### Bug Fixes

* **texttospeech:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.7.6](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.5...texttospeech/v1.7.6) (2024-03-14)


### Bug Fixes

* **texttospeech:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.7.5](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.4...texttospeech/v1.7.5) (2024-01-30)


### Bug Fixes

* **texttospeech:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.7.4](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.3...texttospeech/v1.7.4) (2023-11-01)


### Bug Fixes

* **texttospeech:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.7.3](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.2...texttospeech/v1.7.3) (2023-10-26)


### Bug Fixes

* **texttospeech:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.7.2](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.1...texttospeech/v1.7.2) (2023-10-12)


### Bug Fixes

* **texttospeech:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.7.1](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.7.0...texttospeech/v1.7.1) (2023-06-20)


### Bug Fixes

* **texttospeech:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.6.2...texttospeech/v1.7.0) (2023-05-30)


### Features

* **texttospeech:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.6.2](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.6.1...texttospeech/v1.6.2) (2023-05-10)


### Documentation

* **texttospeech:** Update documentation to require certain fields ([31c3766](https://github.com/googleapis/google-cloud-go/commit/31c3766c9c4cab411669c14fc1a30bd6d2e3f2dd))

## [1.6.1](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.6.0...texttospeech/v1.6.1) (2023-05-08)


### Bug Fixes

* **texttospeech:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.5.0...texttospeech/v1.6.0) (2023-01-04)


### Features

* **texttospeech:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.4.0...texttospeech/v1.5.0) (2022-11-03)


### Features

* **texttospeech:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.3.0...texttospeech/v1.4.0) (2022-10-25)


### Features

* **texttospeech:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.2.0...texttospeech/v1.3.0) (2022-02-23)


### Features

* **texttospeech:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/texttospeech/v1.1.0...texttospeech/v1.2.0) (2022-02-14)


### Features

* **texttospeech:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/texttospeech/v1.0.0...texttospeech/v1.1.0) (2022-01-04)


### Features

* **texttospeech:** update v1 proto ([90e2868](https://www.github.com/googleapis/google-cloud-go/commit/90e2868a3d220aa7f897438f4917013fda7a7c59))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out texttospeech as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
