# Changes

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
