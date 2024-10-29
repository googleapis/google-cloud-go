# Changes


## [1.35.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.34.0...documentai/v1.35.0) (2024-10-23)


### Features

* **documentai:** Add RESOURCE_EXHAUSTED to retryable status codes for ProcessDocument method ([6e69d2e](https://github.com/googleapis/google-cloud-go/commit/6e69d2e85849002bad227ea5bebcde9199605bef))
* **documentai:** Added an url for issue reporting and api short name ([6e69d2e](https://github.com/googleapis/google-cloud-go/commit/6e69d2e85849002bad227ea5bebcde9199605bef))
* **documentai:** Updated the exponential backoff settings for the Document AI ProcessDocument and BatchProcessDocuments methods ([6e69d2e](https://github.com/googleapis/google-cloud-go/commit/6e69d2e85849002bad227ea5bebcde9199605bef))


### Bug Fixes

* **documentai:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **documentai:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))

## [1.34.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.33.0...documentai/v1.34.0) (2024-09-12)


### Features

* **documentai:** Add API fields for the descriptions of entity type and property in the document schema ([2d5a9f9](https://github.com/googleapis/google-cloud-go/commit/2d5a9f9ea9a31e341f9a380ae50a650d48c29e99))


### Bug Fixes

* **documentai:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.33.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.32.0...documentai/v1.33.0) (2024-08-20)


### Features

* **documentai:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.32.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.31.0...documentai/v1.32.0) (2024-08-08)


### Features

* **documentai:** A new field `gen_ai_model_info` is added to message `.google.cloud.documentai.v1.ProcessorVersion` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))
* **documentai:** A new field `imageless_mode` is added to message `.google.cloud.documentai.v1.ProcessRequest` ([649c075](https://github.com/googleapis/google-cloud-go/commit/649c075d5310e2fac64a0b65ec445e7caef42cb0))


### Bug Fixes

* **documentai:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.31.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.5...documentai/v1.31.0) (2024-08-01)


### Features

* **documentai:** A new field `imageless_mode` is added to message `.google.cloud.documentai.v1.ProcessRequest` ([#10615](https://github.com/googleapis/google-cloud-go/issues/10615)) ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))


### Documentation

* **documentai:** Keep the API doc up-to-date with recent changes ([97fa560](https://github.com/googleapis/google-cloud-go/commit/97fa56008a30857fc6d835517fc2d9a2959b19a5))

## [1.30.5](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.4...documentai/v1.30.5) (2024-07-24)


### Bug Fixes

* **documentai:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.30.4](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.3...documentai/v1.30.4) (2024-07-10)


### Bug Fixes

* **documentai:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.30.3](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.2...documentai/v1.30.3) (2024-07-01)


### Bug Fixes

* **documentai:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.30.2](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.1...documentai/v1.30.2) (2024-06-26)


### Bug Fixes

* **documentai:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.30.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.30.0...documentai/v1.30.1) (2024-06-20)


### Documentation

* **documentai:** Update the comment to add a note about `documentai.processors.create` permission ([4fa4308](https://github.com/googleapis/google-cloud-go/commit/4fa43082511e153044084c1e6736553de41a9894))

## [1.30.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.29.0...documentai/v1.30.0) (2024-06-05)


### Features

* **documentai:** Make Layout Parser generally available in V1 ([#10286](https://github.com/googleapis/google-cloud-go/issues/10286)) ([92dc381](https://github.com/googleapis/google-cloud-go/commit/92dc381da281197567a2c9eb8dc941292000a3da))

## [1.29.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.28.1...documentai/v1.29.0) (2024-05-29)


### Features

* **documentai:** Make Layout Parser generally available in V1 ([#10277](https://github.com/googleapis/google-cloud-go/issues/10277)) ([dafecc9](https://github.com/googleapis/google-cloud-go/commit/dafecc9f28a6b028889c8cefb352e50f60563a4e))

## [1.28.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.28.0...documentai/v1.28.1) (2024-05-16)


### Documentation

* **documentai:** Clarify the unavailability of some features ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **documentai:** Updated comments ([292e812](https://github.com/googleapis/google-cloud-go/commit/292e81231b957ae7ac243b47b8926564cee35920))

## [1.28.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.27.0...documentai/v1.28.0) (2024-05-01)


### Features

* **documentai:** A new message `FoundationModelTuningOptions` is added ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))
* **documentai:** Support Chunk header and footer in Doc AI external proto ([1d757c6](https://github.com/googleapis/google-cloud-go/commit/1d757c66478963d6cbbef13fee939632c742759c))


### Bug Fixes

* **documentai:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.27.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.26.1...documentai/v1.27.0) (2024-04-15)


### Features

* **documentai:** Support a new Layout Processor in Document AI ([2cdc40a](https://github.com/googleapis/google-cloud-go/commit/2cdc40a0b4288f5ab5f2b2b8f5c1d6453a9c81ec))

## [1.26.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.26.0...documentai/v1.26.1) (2024-03-14)


### Bug Fixes

* **documentai:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))


### Documentation

* **documentai:** A comment for field `processor_version_source` in message `.google.cloud.documentai.v1beta3.ImportProcessorVersionRequest` is changed ([25c3f2d](https://github.com/googleapis/google-cloud-go/commit/25c3f2dfcf1e720df82b3ee236d8e6a1fe888318))

## [1.26.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.25.0...documentai/v1.26.0) (2024-02-21)


### Features

* **documentai:** A new field `schema_override` is added to message `ProcessOptions` ([#9400](https://github.com/googleapis/google-cloud-go/issues/9400)) ([a86aa8e](https://github.com/googleapis/google-cloud-go/commit/a86aa8e962b77d152ee6cdd433ad94967150ef21))
* **documentai:** A new message FoundationModelTuningOptions is added ([7e6c208](https://github.com/googleapis/google-cloud-go/commit/7e6c208c5d97d3f6e2f7fd7aca09b8ae98dc0bf2))

## [1.25.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.24.0...documentai/v1.25.0) (2024-02-09)


### Features

* **documentai:** Expose model_type in v1 processor, so that user can see the model_type after get or list processor version ([2fcf55c](https://github.com/googleapis/google-cloud-go/commit/2fcf55ccb24749cf5387e707b0212bca722f2e96))

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.8...documentai/v1.24.0) (2024-02-06)


### Features

* **documentai:** Add model_type in v1beta3 processor proto ([#9355](https://github.com/googleapis/google-cloud-go/issues/9355)) ([05e9e1f](https://github.com/googleapis/google-cloud-go/commit/05e9e1f53f2a0c8b3aaadc1811338ca3e682f245))

## [1.23.8](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.7...documentai/v1.23.8) (2024-01-30)


### Bug Fixes

* **documentai:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.23.7](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.6...documentai/v1.23.7) (2023-12-13)


### Documentation

* **documentai:** Clarify Properties documentation ([757c1b0](https://github.com/googleapis/google-cloud-go/commit/757c1b0dcca95058aedd091d1e89d5d14f2fbc1c))

## [1.23.6](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.5...documentai/v1.23.6) (2023-11-27)


### Documentation

* **documentai:** Update comment for ProcessOptions.page_range ([63ffff2](https://github.com/googleapis/google-cloud-go/commit/63ffff2a994d991304ba1ef93cab847fa7cd39e4))

## [1.23.5](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.4...documentai/v1.23.5) (2023-11-01)


### Bug Fixes

* **documentai:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))


### Documentation

* **documentai:** Updated comments ([24e410e](https://github.com/googleapis/google-cloud-go/commit/24e410efbb6add2d33ecfb6ad98b67dc8894e578))

## [1.23.4](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.3...documentai/v1.23.4) (2023-10-26)


### Bug Fixes

* **documentai:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.23.3](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.2...documentai/v1.23.3) (2023-10-17)


### Documentation

* **documentai:** Minor clarification on fields related to page range ([#8734](https://github.com/googleapis/google-cloud-go/issues/8734)) ([e864fbc](https://github.com/googleapis/google-cloud-go/commit/e864fbcbc4f0a49dfdb04850b07451074c57edc8))

## [1.23.2](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.1...documentai/v1.23.2) (2023-10-12)


### Bug Fixes

* **documentai:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.23.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.23.0...documentai/v1.23.1) (2023-10-12)


### Documentation

* **documentai:** Minor comment update ([9c502c2](https://github.com/googleapis/google-cloud-go/commit/9c502c2cf66b15c253e53747e08da77a21549cc2))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.22.1...documentai/v1.23.0) (2023-10-04)


### Features

* **documentai:** Added `SummaryOptions` to `ProcessOptions` for the Summarizer processor ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))
* **documentai:** Added field Processor.processor_version_aliases ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))
* **documentai:** Make `page_range` field public ([#8602](https://github.com/googleapis/google-cloud-go/issues/8602)) ([02a899c](https://github.com/googleapis/google-cloud-go/commit/02a899c95eb9660128506cf94525c5a75bedb308))

## [1.22.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.22.0...documentai/v1.22.1) (2023-09-12)


### Documentation

* **documentai:** Update client libraries for Enterprise OCR add-ons ([#8549](https://github.com/googleapis/google-cloud-go/issues/8549)) ([0449518](https://github.com/googleapis/google-cloud-go/commit/0449518f8396cc0280c0f3303c103edcee34016b))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.21.0...documentai/v1.22.0) (2023-07-26)


### Features

* **documentai:** Exposed Import PV external_processor_version_source to v1beta3 public ([#8323](https://github.com/googleapis/google-cloud-go/issues/8323)) ([08b151a](https://github.com/googleapis/google-cloud-go/commit/08b151a3fd9b614b6696e99d065ecda339ed00ff))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.20.0...documentai/v1.21.0) (2023-07-18)


### Features

* **documentai:** Removed id field from Document message ([4a5651c](https://github.com/googleapis/google-cloud-go/commit/4a5651caa472882fe4c7f6be400f782f60f6f258))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.19.0...documentai/v1.20.0) (2023-06-20)


### Features

* **documentai:** Add StyleInfo to document.proto ([b726d41](https://github.com/googleapis/google-cloud-go/commit/b726d413166faa8c84c0a09c6019ff50f3249b9d))
* **documentai:** Add StyleInfo to document.proto ([b726d41](https://github.com/googleapis/google-cloud-go/commit/b726d413166faa8c84c0a09c6019ff50f3249b9d))


### Bug Fixes

* **documentai:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.18.1...documentai/v1.19.0) (2023-05-30)


### Features

* **documentai:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.18.0...documentai/v1.18.1) (2023-05-08)


### Bug Fixes

* **documentai:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.17.0...documentai/v1.18.0) (2023-03-22)


### Features

* **documentai:** Add ImportProcessorVersion in v1beta3 ([c967961](https://github.com/googleapis/google-cloud-go/commit/c967961ed95750e173af0193ec8d0974471f43ff))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.16.0...documentai/v1.17.0) (2023-03-15)


### Features

* **documentai:** Added hints.language_hints field in OcrConfig ([#7522](https://github.com/googleapis/google-cloud-go/issues/7522)) ([b2c40c3](https://github.com/googleapis/google-cloud-go/commit/b2c40c3df916691b82f1b384eac5bc953960960a))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.15.0...documentai/v1.16.0) (2023-02-22)


### Features

* **documentai:** ROLLBACK ([#7439](https://github.com/googleapis/google-cloud-go/issues/7439)) ([932ddc8](https://github.com/googleapis/google-cloud-go/commit/932ddc87ed3889bd5b132d4c2307b1017c3ef3a2))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.8.0...documentai/v1.15.0) (2023-02-14)


### ⚠ BREAKING CHANGES

* **documentai:** The TrainProcessorVersion parent was incorrectly annotated.

### Features

* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **documentai:** Added advanced_ocr_options field in OcrConfig ([45c70e3](https://github.com/googleapis/google-cloud-go/commit/45c70e31e12ae5bb9ad9644648eb154ff5c033df))
* **documentai:** Added EvaluationReference to evaluation.proto ([#7290](https://github.com/googleapis/google-cloud-go/issues/7290)) ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))
* **documentai:** Added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **documentai:** Added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **documentai:** Added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **documentai:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Miscellaneous Chores

* **documentai:** Release 1.15.0 ([#7426](https://github.com/googleapis/google-cloud-go/issues/7426)) ([672d8c2](https://github.com/googleapis/google-cloud-go/commit/672d8c20f7cbce9fbd9b2d5e29cfb803f1e51d2d))
* **documentai:** Release 1.8.0 ([#7423](https://github.com/googleapis/google-cloud-go/issues/7423)) ([a10f592](https://github.com/googleapis/google-cloud-go/commit/a10f592f85641153832d713551e0246d9b5a1174))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.7.0...documentai/v1.8.0) (2023-02-14)


### Features

* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **documentai:** Added advanced_ocr_options field in OcrConfig ([45c70e3](https://github.com/googleapis/google-cloud-go/commit/45c70e31e12ae5bb9ad9644648eb154ff5c033df))
* **documentai:** Added EvaluationReference to evaluation.proto ([#7290](https://github.com/googleapis/google-cloud-go/issues/7290)) ([4623db8](https://github.com/googleapis/google-cloud-go/commit/4623db86fb70305278f6740999ecaee674506052))
* **documentai:** Added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **documentai:** Added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **documentai:** Added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **documentai:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **documentai:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Miscellaneous Chores

* **documentai:** Release 1.8.0 ([#7423](https://github.com/googleapis/google-cloud-go/issues/7423)) ([a10f592](https://github.com/googleapis/google-cloud-go/commit/a10f592f85641153832d713551e0246d9b5a1174))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.6.0...documentai/v1.7.0) (2023-01-31)


### Features

* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **documentai:** Added advanced_ocr_options field in OcrConfig ([45c70e3](https://github.com/googleapis/google-cloud-go/commit/45c70e31e12ae5bb9ad9644648eb154ff5c033df))
* **documentai:** Added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **documentai:** Added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **documentai:** Added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **documentai:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **documentai:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **documentai:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.5.0...documentai/v1.6.0) (2023-01-26)


### Features

* **documentai/apiv1beta3:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **documentai:** Added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **documentai:** Added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **documentai:** Added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **documentai:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **documentai:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **documentai:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/documentai-v1.15.0...documentai/v1.5.0) (2023-01-26)


### ⚠ BREAKING CHANGES

* **documentai:** Changed the name field for ProcessRequest and BatchProcessorRequest to accept * so the name field can accept Processor and ProcessorVersion.

### Features

* **documentai/apiv1beta3:** Add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))
* **documentai:** Added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))
* **documentai:** Added field_mask to ProcessRequest object in document_processor_service.proto feat: Added parent_ids to Revision object in document.proto feat: Added integer_values, float_values and non_present to Entity object in document.proto feat: Added corrected_key_text, correct_value_text to FormField object in document.proto feat: Added OperationMetadata resource feat!: Added Processor Management and Processor Version support to v1 library ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))
* **documentai:** Added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))
* **documentai:** Added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))
* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))
* **documentai:** Rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))
* **documentai:** Start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))
* **documentai:** Start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))


### Documentation

* **documentai:** Fix minor docstring formatting ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))


### Miscellaneous Chores

* **documentai:** Release v1.5.0 ([#6522](https://github.com/googleapis/google-cloud-go/issues/6522)) ([4169a66](https://github.com/googleapis/google-cloud-go/commit/4169a66d15e99a14d3a59fd5d0e9a8f4509f0643))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.14.0...documentai/v1.15.0) (2023-01-26)


### Features

* **documentai:** Exposed GetProcessorType to v1 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))
* **documentai:** Exposed GetProcessorType to v1beta3 ([447afdd](https://github.com/googleapis/google-cloud-go/commit/447afddf34d59c599cabe5415b4f9265b228bb9a))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.13.0...documentai/v1.14.0) (2023-01-04)


### Features

* **documentai:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.12.0...documentai/v1.13.0) (2022-12-01)


### Features

* **documentai:** added field_mask field in DocumentOutputConfig.GcsOutputConfig in document_io.proto ([2a0b1ae](https://github.com/googleapis/google-cloud-go/commit/2a0b1aeb1683222e6aa5c876cb945845c00cef79))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.11.0...documentai/v1.12.0) (2022-11-16)


### Features

* **documentai:** added TrainProcessorVersion, EvaluateProcessorVersion, GetEvaluation, and ListEvaluations v1beta3 APIs feat: added evaluation.proto feat: added document_schema field in ProcessorVersion processor.proto feat: added image_quality_scores field in Document.Page in document.proto feat: added font_family field in Document.Style in document.proto ([ac0c5c2](https://github.com/googleapis/google-cloud-go/commit/ac0c5c21221e8d055e6b8b1c473600c58e306b00))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.10.0...documentai/v1.11.0) (2022-11-09)


### Features

* **documentai:** added font_family to document.proto feat: added ImageQualityScores message to document.proto feat: added PropertyMetadata and EntityTypeMetadata to document_schema.proto ([9c5d6c8](https://github.com/googleapis/google-cloud-go/commit/9c5d6c857b9deece4663d37fc6c834fd758b98ca))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.9.0...documentai/v1.10.0) (2022-11-03)


### Features

* **documentai:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.8.0...documentai/v1.9.0) (2022-10-25)


### Features

* **documentai:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.7.0...documentai/v1.8.0) (2022-09-21)


### Features

* **documentai:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.6.0...documentai/v1.7.0) (2022-09-19)


### Features

* **documentai:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.5.0...documentai/v1.6.0) (2022-09-15)


### Features

* **documentai/apiv1beta3:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.4.0...documentai/v1.5.0) (2022-08-18)


### ⚠ BREAKING CHANGES

* **documentai:** Changed the name field for ProcessRequest and BatchProcessorRequest to accept * so the name field can accept Processor and ProcessorVersion.

### Features

* **documentai:** Added field_mask to ProcessRequest object in document_processor_service.proto feat: Added parent_ids to Revision object in document.proto feat: Added integer_values, float_values and non_present to Entity object in document.proto feat: Added corrected_key_text, correct_value_text to FormField object in document.proto feat: Added OperationMetadata resource feat!: Added Processor Management and Processor Version support to v1 library ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))


### Documentation

* **documentai:** fix minor docstring formatting ([370e23e](https://github.com/googleapis/google-cloud-go/commit/370e23eaa342a7055a8d8b6f8fe9420f83afe43e))


### Miscellaneous Chores

* **documentai:** release v1.5.0 ([#6522](https://github.com/googleapis/google-cloud-go/issues/6522)) ([4169a66](https://github.com/googleapis/google-cloud-go/commit/4169a66d15e99a14d3a59fd5d0e9a8f4509f0643))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.3.0...documentai/v1.4.0) (2022-02-23)


### Features

* **documentai:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.2.0...documentai/v1.3.0) (2022-02-22)


### Features

* **documentai:** add `symbols` field, and auto-format comments ([f9fe0f2](https://github.com/googleapis/google-cloud-go/commit/f9fe0f2bf152c3855d3c6a2c54f9b7adba54f626))
* **documentai:** add `symbols` field, and auto-format comments ([f9fe0f2](https://github.com/googleapis/google-cloud-go/commit/f9fe0f2bf152c3855d3c6a2c54f9b7adba54f626))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/documentai/v1.1.0...documentai/v1.2.0) (2022-02-11)


### Features

* **documentai:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))
* **documentai:** add question_id field in ReviewDocumentOperationMetadata ([2fae584](https://github.com/googleapis/google-cloud-go/commit/2fae584d01fad2f693b165a95c18d4fb8bf062bf))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/documentai/v1.0.1...documentai/v1.1.0) (2022-02-03)


### Features

* **documentai:** add question_id field in ReviewDocumentOperationMetadata ([6e56077](https://www.github.com/googleapis/google-cloud-go/commit/6e560776fd6e574320ce2dbad1f9eb9e22999185))

### [1.0.1](https://www.github.com/googleapis/google-cloud-go/compare/documentai/v1.0.0...documentai/v1.0.1) (2022-01-13)


### Bug Fixes

* **documentai:** add ancillary service bindings to service_yaml ([3bbe8c0](https://www.github.com/googleapis/google-cloud-go/commit/3bbe8c0c558c06ef5865bb79eb228b6da667ddb3))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out documentai as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
