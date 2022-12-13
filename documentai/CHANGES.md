# Changes


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
