# Changes


## [1.23.2](https://github.com/googleapis/google-cloud-go/compare/video/v1.23.1...video/v1.23.2) (2024-10-23)


### Bug Fixes

* **video:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **video:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [1.23.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.23.0...video/v1.23.1) (2024-09-12)


### Bug Fixes

* **video:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.22.1...video/v1.23.0) (2024-08-20)


### Features

* **video:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.22.0...video/v1.22.1) (2024-08-08)


### Bug Fixes

* **video:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.21.3...video/v1.22.0) (2024-07-24)


### Features

* **video/livestream:** Added Clip resource for performing clip cutting jobs ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **video/livestream:** Added RetentionConfig for enabling retention of output media segments ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))
* **video/livestream:** Added StaticOverlay for embedding images the whole duration of the live stream ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))


### Bug Fixes

* **video:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))


### Documentation

* **video/livestream:** Clarify the format of key/id fields ([eb63f0d](https://github.com/googleapis/google-cloud-go/commit/eb63f0d4f42a06581e1425f99c2a03d52d6cb404))

## [1.21.3](https://github.com/googleapis/google-cloud-go/compare/video/v1.21.2...video/v1.21.3) (2024-07-10)


### Bug Fixes

* **video:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.21.2](https://github.com/googleapis/google-cloud-go/compare/video/v1.21.1...video/v1.21.2) (2024-07-01)


### Bug Fixes

* **video:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.21.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.21.0...video/v1.21.1) (2024-06-26)


### Bug Fixes

* **video:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.6...video/v1.21.0) (2024-05-29)


### Features

* **video/stitcher:** Add apis for Create, Read, Update, Delete for VODConfigs ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **video/stitcher:** Added adtracking to Livesession ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **video/stitcher:** Added fetchoptions with custom headers for Live and VODConfigs ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **video/stitcher:** Added targetting parameter support to Livesession ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **video/stitcher:** Added token config for MediaCdnKey ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))
* **video/stitcher:** Allowed usage for VODConfigs in VODSession ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))


### Documentation

* **video/stitcher:** Added apis for Create, Read, Update, Delete For VODConfigs. Added vodConfig usage in VODSession. Added TokenConfig for MediaCdnKey. Added targeting_parameter and ad_tracking for Livesession. Added FetchOptions for Live and VOD configs. ([3df3c04](https://github.com/googleapis/google-cloud-go/commit/3df3c04f0dffad3fa2fe272eb7b2c263801b9ada))

## [1.20.6](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.5...video/v1.20.6) (2024-05-01)


### Bug Fixes

* **video:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.20.5](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.4...video/v1.20.5) (2024-03-14)


### Bug Fixes

* **video:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.20.4](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.3...video/v1.20.4) (2024-01-30)


### Bug Fixes

* **video:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.20.3](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.2...video/v1.20.3) (2023-11-01)


### Bug Fixes

* **video:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.20.2](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.1...video/v1.20.2) (2023-10-26)


### Bug Fixes

* **video:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.20.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.20.0...video/v1.20.1) (2023-10-12)


### Bug Fixes

* **video:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.19.0...video/v1.20.0) (2023-09-12)


### Features

* **video/stitcher:** Refactor RPCs to use LRO ([#8561](https://github.com/googleapis/google-cloud-go/issues/8561)) ([aaebe09](https://github.com/googleapis/google-cloud-go/commit/aaebe097413bd38a969476253e951b7f5274cbbf))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.18.0...video/v1.19.0) (2023-07-24)


### Features

* **video/livestream:** Added support for slate events which allow users to create and insert a slate into a live stream to replace the main live stream content ([432864c](https://github.com/googleapis/google-cloud-go/commit/432864c7fc0bb551a5017b423bbd5f76c3357dc3))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.17.1...video/v1.18.0) (2023-07-18)


### Features

* **video/transcoder:** Added support for segment template manifest generation with DASH ([#8242](https://github.com/googleapis/google-cloud-go/issues/8242)) ([adb982e](https://github.com/googleapis/google-cloud-go/commit/adb982ed500c5011e477c00baefad504b0a00210))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.17.0...video/v1.17.1) (2023-06-20)


### Bug Fixes

* **video:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.16.1...video/v1.17.0) (2023-05-30)


### Features

* **video:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.16.1](https://github.com/googleapis/google-cloud-go/compare/video/v1.16.0...video/v1.16.1) (2023-05-08)


### Bug Fixes

* **video:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.15.0...video/v1.16.0) (2023-04-25)


### Features

* **video/transcoder:** Add support for batch processing mode ([c298dcc](https://github.com/googleapis/google-cloud-go/commit/c298dcc14e73fbb5648945b84c23087cafc8179c))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.14.0...video/v1.15.0) (2023-04-04)


### Features

* **video/stitcher:** LRO changes for CdnKey and Slate methods, VodSession changes for ad tracking, and LiveSession changes for live config ([597ea0f](https://github.com/googleapis/google-cloud-go/commit/597ea0fe09bcea04e884dffe78add850edb2120d))
* **video/stitcher:** Remove default_ad_break_duration from LiveConfig ([226764d](https://github.com/googleapis/google-cloud-go/commit/226764d72f9d5714fbc6c1852189b81746e38f72))
* **video/stitcher:** Update LRO metadata type to google.cloud.common.OperationMetadata ([226764d](https://github.com/googleapis/google-cloud-go/commit/226764d72f9d5714fbc6c1852189b81746e38f72))


### Bug Fixes

* **video/stitcher:** Roll back the changes that update of LRO metadata type to google.cloud.common.OperationMetadata ([#7651](https://github.com/googleapis/google-cloud-go/issues/7651)) ([226764d](https://github.com/googleapis/google-cloud-go/commit/226764d72f9d5714fbc6c1852189b81746e38f72))
* **video/stitcher:** Stop generation and rewind stitcher client ([#7659](https://github.com/googleapis/google-cloud-go/issues/7659)) ([3c4d7cf](https://github.com/googleapis/google-cloud-go/commit/3c4d7cf0edd12e840438d4079dd2b8ff4c18de27))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.13.0...video/v1.14.0) (2023-03-22)


### Features

* **video/livestream:** Added TimecodeConfig for specifying the source of timecode used in media workflow synchronization ([c967961](https://github.com/googleapis/google-cloud-go/commit/c967961ed95750e173af0193ec8d0974471f43ff))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.12.0...video/v1.13.0) (2023-03-01)


### Features

* **video/transcoder:** Specifying language code and display name for text and audio streams is now supported ([#7489](https://github.com/googleapis/google-cloud-go/issues/7489)) ([8b895d2](https://github.com/googleapis/google-cloud-go/commit/8b895d245f0e4ebf6bbd3825a1078778b2e6de28))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.11.0...video/v1.12.0) (2023-01-04)


### Features

* **video:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.10.0...video/v1.11.0) (2022-12-01)


### Features

* **video/transcoder:** add PreprocessingConfig.deinterlace docs: minor documentation changes ([7c8cbcf](https://github.com/googleapis/google-cloud-go/commit/7c8cbcf769ed8a33eb6c7da96c789667fb733156))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.9.0...video/v1.10.0) (2022-11-09)


### Features

* **video/stitcher:** Add support for Media CDN ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.8.0...video/v1.9.0) (2022-11-03)


### Features

* **video:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.7.0...video/v1.8.0) (2022-10-25)


### Features

* **video:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.6.0...video/v1.7.0) (2022-06-29)


### Features

* **video/livestream:** add C++ library rules for the Live Stream API ([199b725](https://github.com/googleapis/google-cloud-go/commit/199b7250f474b1a6f53dcf0aac0c2966f4987b68))
* **video/livestream:** release as GA ([5be6d33](https://github.com/googleapis/google-cloud-go/commit/5be6d33a57cc57ecfe5c34a0b1f6e3e0dd4b51fa))
* **video/stitcher:** release as GA ([5be6d33](https://github.com/googleapis/google-cloud-go/commit/5be6d33a57cc57ecfe5c34a0b1f6e3e0dd4b51fa))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.5.0...video/v1.6.0) (2022-06-17)


### Features

* **video/transcoder:** add support for user labels for job and job template ([c84e111](https://github.com/googleapis/google-cloud-go/commit/c84e111db5d3f57f4e8fbb5dfff0219d052435a0))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.4.0...video/v1.5.0) (2022-06-16)


### Features

* **video/stitcher:** add asset_id and stream_id fields to VodSession and LiveSession responses fix: remove COMPLETE_POD stitching option ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.3.0...video/v1.4.0) (2022-03-14)


### Features

* **video/stitcher:** start generating apiv1 ([#5720](https://github.com/googleapis/google-cloud-go/issues/5720)) ([4ca8fea](https://github.com/googleapis/google-cloud-go/commit/4ca8fea35c1c5f3e2675c666e238bb1dc3561d52))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.2.0...video/v1.3.0) (2022-02-23)


### Features

* **video:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/video/v1.1.0...video/v1.2.0) (2022-02-11)


### Features

* **video/transcoder:** remove apiv1beta1, turned down ([#5467](https://github.com/googleapis/google-cloud-go/issues/5467)) ([917d8d2](https://github.com/googleapis/google-cloud-go/commit/917d8d255906ea505cb674302870e031e5eff517))
* **video:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/video/v1.0.1...video/v1.1.0) (2022-01-25)


### Features

* **video:** start generating livestream apiv1 ([#5404](https://www.github.com/googleapis/google-cloud-go/issues/5404)) ([2b6770d](https://www.github.com/googleapis/google-cloud-go/commit/2b6770d762897c84e653973e989d95c0371b89ad))

### [1.0.1](https://www.github.com/googleapis/google-cloud-go/compare/video/v1.0.0...video/v1.0.1) (2021-10-18)

### Bug Fixes

* **video/transcoder:** update nodejs package name to video-transcoder ([30794e7](https://www.github.com/googleapis/google-cloud-go/commit/30794e70050b55ff87d6a80d0b4075065e9d271d))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out video as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
