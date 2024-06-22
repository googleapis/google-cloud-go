# Changelog

## [0.7.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.6.0...ai/v0.7.0) (2024-06-12)


### Features

* **ai/generativelanguage/apiv1beta:** Add SetGoogleClientInfo for all clients ([#10272](https://github.com/googleapis/google-cloud-go/issues/10272)) ([0dee490](https://github.com/googleapis/google-cloud-go/commit/0dee49034889f59160bd1beb8d5573fe002eb56a))
* **ai/generativelanguage:** Add cached_content_token_count to CountTokensResponse ([fc9e895](https://github.com/googleapis/google-cloud-go/commit/fc9e895c460d6911edbe0b47d8fc689cf76a4a58))
* **ai/generativelanguage:** Add cached_content_token_count to generative_service's UsageMetadata ([fc9e895](https://github.com/googleapis/google-cloud-go/commit/fc9e895c460d6911edbe0b47d8fc689cf76a4a58))
* **ai/generativelanguage:** Add content caching ([fc9e895](https://github.com/googleapis/google-cloud-go/commit/fc9e895c460d6911edbe0b47d8fc689cf76a4a58))


### Documentation

* **ai/generativelanguage:** Small fixes ([fc9e895](https://github.com/googleapis/google-cloud-go/commit/fc9e895c460d6911edbe0b47d8fc689cf76a4a58))

## [0.6.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.5.0...ai/v0.6.0) (2024-05-29)


### Features

* **ai/generativelanguage:** Add generate_content_request to CountTokensRequest ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **ai/generativelanguage:** Add usage metadata to GenerateContentResponse ([#10179](https://github.com/googleapis/google-cloud-go/issues/10179)) ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **ai/generativelanguage:** Add video metadata to files API ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))
* **ai/generativelanguage:** Update timeouts ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))
* **ai/generativelanguage:** Update timeouts for generate content ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))


### Documentation

* **ai/generativelanguage:** Minor updates ([5238dbc](https://github.com/googleapis/google-cloud-go/commit/5238dbc48971a7295127be0f415280248608c6be))
* **ai/generativelanguage:** Minor updates ([652ba8f](https://github.com/googleapis/google-cloud-go/commit/652ba8fa79d4d23b4267fd201acf5ca692228959))

## [0.5.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.4.1...ai/v0.5.0) (2024-05-09)


### Features

* **ai/generativelanguage:** Add FileState to File ([3e25053](https://github.com/googleapis/google-cloud-go/commit/3e250530567ee81ed4f51a3856c5940dbec35289))

## [0.4.1](https://github.com/googleapis/google-cloud-go/compare/ai/v0.4.0...ai/v0.4.1) (2024-05-01)


### Bug Fixes

* **ai:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [0.4.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.3.4...ai/v0.4.0) (2024-04-15)


### Features

* **ai/generativelanguage:** Add question_answering and fact_verification task types for AQA ([#9745](https://github.com/googleapis/google-cloud-go/issues/9745)) ([cca3f47](https://github.com/googleapis/google-cloud-go/commit/cca3f47c895e7cac07d7d48ab3c4850b265a710f))
* **ai/generativelanguage:** Add rest binding for tuned models ([8892943](https://github.com/googleapis/google-cloud-go/commit/8892943b169060f8ba7be227cd65680696c494a0))
* **ai/generativelanguage:** Add system instructions ([dd7c8e5](https://github.com/googleapis/google-cloud-go/commit/dd7c8e5a206ca6fab7d05e2591a36ea706e5e9f1))

## [0.3.4](https://github.com/googleapis/google-cloud-go/compare/ai/v0.3.3...ai/v0.3.4) (2024-03-19)


### Bug Fixes

* **ai/generativelanguage:** Make learning rate a one-of ([a3bb7c0](https://github.com/googleapis/google-cloud-go/commit/a3bb7c07ba570f26c6eb073ab3275487784547d0))

## [0.3.3](https://github.com/googleapis/google-cloud-go/compare/ai/v0.3.2...ai/v0.3.3) (2024-03-14)


### Bug Fixes

* **ai:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [0.3.2](https://github.com/googleapis/google-cloud-go/compare/ai/v0.3.1...ai/v0.3.2) (2024-01-30)


### Bug Fixes

* **ai/generativelanguage:** Fix content.proto's Schema - `type` should be required ([97d62c7](https://github.com/googleapis/google-cloud-go/commit/97d62c7a6a305c47670ea9c147edc444f4bf8620))
* **ai:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [0.3.1](https://github.com/googleapis/google-cloud-go/compare/ai/v0.3.0...ai/v0.3.1) (2024-01-22)


### Documentation

* **ai/generativelanguage:** Fixed minor documentation typos for field `function_declarations` in message `.google.ai.generativelanguage.v1beta.Tool` ([af2f8b4](https://github.com/googleapis/google-cloud-go/commit/af2f8b4f3401c0b12dadb2c504aa0f902aee76de))

## [0.3.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.2.0...ai/v0.3.0) (2023-12-13)


### Features

* **ai:** Expose ability to set headers ([#9154](https://github.com/googleapis/google-cloud-go/issues/9154)) ([40f2d6a](https://github.com/googleapis/google-cloud-go/commit/40f2d6aadffb43f4661badf83274c84f9908f7c1))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/ai/v0.1.4...ai/v0.2.0) (2023-12-11)


### Features

* **ai/generativelanguage:** Add v1beta, adds GenerativeService and RetrievalService ([29effe6](https://github.com/googleapis/google-cloud-go/commit/29effe600e16f24a127a1422ec04263c4f7a600a))
* **ai:** New clients ([#9126](https://github.com/googleapis/google-cloud-go/issues/9126)) ([c09249e](https://github.com/googleapis/google-cloud-go/commit/c09249e16b01da2b441337416115af7931892aaa))

## [0.1.4](https://github.com/googleapis/google-cloud-go/compare/ai/v0.1.3...ai/v0.1.4) (2023-11-01)


### Bug Fixes

* **ai:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [0.1.3](https://github.com/googleapis/google-cloud-go/compare/ai/v0.1.2...ai/v0.1.3) (2023-10-26)


### Bug Fixes

* **ai:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [0.1.2](https://github.com/googleapis/google-cloud-go/compare/ai/v0.1.1...ai/v0.1.2) (2023-10-12)


### Bug Fixes

* **ai:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [0.1.1](https://github.com/googleapis/google-cloud-go/compare/ai/v0.1.0...ai/v0.1.1) (2023-07-24)


### Documentation

* **ai:** Fix README.md title ([#8289](https://github.com/googleapis/google-cloud-go/issues/8289)) ([ece5895](https://github.com/googleapis/google-cloud-go/commit/ece5895abd1d26f802eaea470e15ea5ce8476ce5))

## 0.1.0 (2023-07-10)


### Features

* **ai/generativelanguage:** Start generating apiv1beta2 ([#8229](https://github.com/googleapis/google-cloud-go/issues/8229)) ([837f325](https://github.com/googleapis/google-cloud-go/commit/837f32596518d8154f43da1c70f57d1515e2ea8c))

## Changes
