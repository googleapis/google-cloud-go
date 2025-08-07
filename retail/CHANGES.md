# Changes

## [1.24.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.23.0...retail/v1.24.0) (2025-07-31)


### Features

* **retail:** Add new fields in conversational search public SDK ([#12602](https://github.com/googleapis/google-cloud-go/issues/12602)) ([83f894e](https://github.com/googleapis/google-cloud-go/commit/83f894e372ae66b96d8d9d4379fa0ea18547fe72))

## [1.23.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.22.0...retail/v1.23.0) (2025-07-23)


### Features

* **retail:** Add experiment_id in the SearchRequest proto ([eeb4b1f](https://github.com/googleapis/google-cloud-go/commit/eeb4b1fe8eb83b73ec31b0bd46e3704bdc0212c3))

## [1.22.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.21.0...retail/v1.22.0) (2025-06-25)


### Features

* **retail:** Add a user_attributes field in SearchRequest that can be used for personalization ([#12494](https://github.com/googleapis/google-cloud-go/issues/12494)) ([2d66d4f](https://github.com/googleapis/google-cloud-go/commit/2d66d4f5b26a488b015be82733b242ce611c0fe3))

## [1.21.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.20.0...retail/v1.21.0) (2025-05-29)


### Features

* **retail:** Add a model_scores field in SearchResponse.results to expose model quality signals ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))
* **retail:** Add a user_attributes field in SearchRequest that can be used for personalization ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))
* **retail:** Data_source_id replaces primary_feed_id in MerchantCenterFeedFilter ([8189e33](https://github.com/googleapis/google-cloud-go/commit/8189e3313ed62b99cc238c421ae9acfa32aaf9af))

## [1.20.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.19.4...retail/v1.20.0) (2025-04-30)


### Features

* **retail:** Add availability field to Localnventory ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **retail:** Add conversational search API ([73bd9a8](https://github.com/googleapis/google-cloud-go/commit/73bd9a8de74a3c71e81888cab98dcc489ba3302b))
* **retail:** Add language_code, region_code and place_id to SearchRequest ([73bd9a8](https://github.com/googleapis/google-cloud-go/commit/73bd9a8de74a3c71e81888cab98dcc489ba3302b))
* **retail:** Add language_code, region_code and place_id to SearchRequest ([#12069](https://github.com/googleapis/google-cloud-go/issues/12069)) ([8f6067c](https://github.com/googleapis/google-cloud-go/commit/8f6067c000833caa338a8cab15ec6fd0ea39f849))
* **retail:** Add new fields including language_code, region_code and place_id to SearchRequest. ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **retail:** Add pin_control_metadata to SearchResponse ([8f6067c](https://github.com/googleapis/google-cloud-go/commit/8f6067c000833caa338a8cab15ec6fd0ea39f849))
* **retail:** Add pin_control_metadata to SearchResponse ([73bd9a8](https://github.com/googleapis/google-cloud-go/commit/73bd9a8de74a3c71e81888cab98dcc489ba3302b))
* **retail:** Add pin_control_metadata to SearchResponse. ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))


### Bug Fixes

* **retail:** An existing field `llm_embedding_config` is removed from message `.google.cloud.retail.v2alpha.Model` ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))
* **retail:** An existing message `LlmEmbeddingConfig` is removed. ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))


### Documentation

* **retail:** Keep the API doc up-to-date with recent changes ([8f6067c](https://github.com/googleapis/google-cloud-go/commit/8f6067c000833caa338a8cab15ec6fd0ea39f849))
* **retail:** Keep the API doc up-to-date with recent changes ([73bd9a8](https://github.com/googleapis/google-cloud-go/commit/73bd9a8de74a3c71e81888cab98dcc489ba3302b))
* **retail:** Keep the API doc up-to-date with recent changes ([a95a0bf](https://github.com/googleapis/google-cloud-go/commit/a95a0bf4172b8a227955a0353fd9c845f4502411))

## [1.19.4](https://github.com/googleapis/google-cloud-go/compare/retail/v1.19.3...retail/v1.19.4) (2025-04-15)


### Bug Fixes

* **retail:** Update google.golang.org/api to 0.229.0 ([3319672](https://github.com/googleapis/google-cloud-go/commit/3319672f3dba84a7150772ccb5433e02dab7e201))

## [1.19.3](https://github.com/googleapis/google-cloud-go/compare/retail/v1.19.2...retail/v1.19.3) (2025-03-13)


### Bug Fixes

* **retail:** Update golang.org/x/net to 0.37.0 ([1144978](https://github.com/googleapis/google-cloud-go/commit/11449782c7fb4896bf8b8b9cde8e7441c84fb2fd))

## [1.19.2](https://github.com/googleapis/google-cloud-go/compare/retail/v1.19.1...retail/v1.19.2) (2025-01-02)


### Bug Fixes

* **retail:** Update golang.org/x/net to v0.33.0 ([e9b0b69](https://github.com/googleapis/google-cloud-go/commit/e9b0b69644ea5b276cacff0a707e8a5e87efafc9))

## [1.19.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.19.0...retail/v1.19.1) (2024-10-23)


### Bug Fixes

* **retail:** Update google.golang.org/api to v0.203.0 ([8bb87d5](https://github.com/googleapis/google-cloud-go/commit/8bb87d56af1cba736e0fe243979723e747e5e11e))
* **retail:** WARNING: On approximately Dec 1, 2024, an update to Protobuf will change service registration function signatures to use an interface instead of a concrete type in generated .pb.go files. This change is expected to affect very few if any users of this client library. For more information, see https://togithub.com/googleapis/google-cloud-go/issues/11020. ([2b8ca4b](https://github.com/googleapis/google-cloud-go/commit/2b8ca4b4127ce3025c7a21cc7247510e07cc5625))

## [1.19.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.18.1...retail/v1.19.0) (2024-10-09)


### Features

* **retail:** Add conversational search ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Add conversational search ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Add conversational search ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Add tile navigation ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Add tile navigation ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Add tile navigation ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))


### Documentation

* **retail:** Keep the API doc up-to-date with recent changes ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Keep the API doc up-to-date with recent changes ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))
* **retail:** Keep the API doc up-to-date with recent changes ([78d8513](https://github.com/googleapis/google-cloud-go/commit/78d8513f7e31c6ef118bdfc784049b8c7f1e3249))

## [1.18.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.18.0...retail/v1.18.1) (2024-09-12)


### Bug Fixes

* **retail:** Bump dependencies ([2ddeb15](https://github.com/googleapis/google-cloud-go/commit/2ddeb1544a53188a7592046b98913982f1b0cf04))

## [1.18.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.5...retail/v1.18.0) (2024-08-20)


### Features

* **retail:** Add support for Go 1.23 iterators ([84461c0](https://github.com/googleapis/google-cloud-go/commit/84461c0ba464ec2f951987ba60030e37c8a8fc18))

## [1.17.5](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.4...retail/v1.17.5) (2024-08-08)


### Bug Fixes

* **retail:** Update google.golang.org/api to v0.191.0 ([5b32644](https://github.com/googleapis/google-cloud-go/commit/5b32644eb82eb6bd6021f80b4fad471c60fb9d73))

## [1.17.4](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.3...retail/v1.17.4) (2024-07-24)


### Bug Fixes

* **retail:** Update dependencies ([257c40b](https://github.com/googleapis/google-cloud-go/commit/257c40bd6d7e59730017cf32bda8823d7a232758))

## [1.17.3](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.2...retail/v1.17.3) (2024-07-10)


### Bug Fixes

* **retail:** Bump google.golang.org/grpc@v1.64.1 ([8ecc4e9](https://github.com/googleapis/google-cloud-go/commit/8ecc4e9622e5bbe9b90384d5848ab816027226c5))

## [1.17.2](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.1...retail/v1.17.2) (2024-07-01)


### Bug Fixes

* **retail:** Bump google.golang.org/api@v0.187.0 ([8fa9e39](https://github.com/googleapis/google-cloud-go/commit/8fa9e398e512fd8533fd49060371e61b5725a85b))

## [1.17.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.17.0...retail/v1.17.1) (2024-06-26)


### Bug Fixes

* **retail:** Enable new auth lib ([b95805f](https://github.com/googleapis/google-cloud-go/commit/b95805f4c87d3e8d10ea23bd7a2d68d7a4157568))

## [1.17.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.16.2...retail/v1.17.0) (2024-06-10)


### Features

* **retail:** Add branch and project APIs to alpha ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Add page_categories to control condition ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Add product purge API ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Allow to skip denylist postfiltering in recommendations ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Support attribute suggestion in autocomplete ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Support frequent bought together model config ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))
* **retail:** Support merged facets ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))


### Documentation

* **retail:** Keep the API doc up-to-date with recent changes ([4c102b7](https://github.com/googleapis/google-cloud-go/commit/4c102b732826222a1b1648bf51d3df7e9f97d1f5))

## [1.16.2](https://github.com/googleapis/google-cloud-go/compare/retail/v1.16.1...retail/v1.16.2) (2024-05-01)


### Bug Fixes

* **retail:** Bump x/net to v0.24.0 ([ba31ed5](https://github.com/googleapis/google-cloud-go/commit/ba31ed5fda2c9664f2e1cf972469295e63deb5b4))

## [1.16.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.16.0...retail/v1.16.1) (2024-03-14)


### Bug Fixes

* **retail:** Update protobuf dep to v1.33.0 ([30b038d](https://github.com/googleapis/google-cloud-go/commit/30b038d8cac0b8cd5dd4761c87f3f298760dd33a))

## [1.16.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.15.1...retail/v1.16.0) (2024-02-09)


### Features

* **retail:** Add analytics service ([f049c97](https://github.com/googleapis/google-cloud-go/commit/f049c9751415f9fc4c81c1839a8371782cfc016c))

## [1.15.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.15.0...retail/v1.15.1) (2024-01-30)


### Bug Fixes

* **retail:** Enable universe domain resolution options ([fd1d569](https://github.com/googleapis/google-cloud-go/commit/fd1d56930fa8a747be35a224611f4797b8aeb698))

## [1.15.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.14.4...retail/v1.15.0) (2024-01-22)


### Features

* **retail:** Add analytics service ([af2f8b4](https://github.com/googleapis/google-cloud-go/commit/af2f8b4f3401c0b12dadb2c504aa0f902aee76de))
* **retail:** Add analytics service ([#9272](https://github.com/googleapis/google-cloud-go/issues/9272)) ([af2f8b4](https://github.com/googleapis/google-cloud-go/commit/af2f8b4f3401c0b12dadb2c504aa0f902aee76de))

## [1.14.4](https://github.com/googleapis/google-cloud-go/compare/retail/v1.14.3...retail/v1.14.4) (2023-11-01)


### Bug Fixes

* **retail:** Bump google.golang.org/api to v0.149.0 ([8d2ab9f](https://github.com/googleapis/google-cloud-go/commit/8d2ab9f320a86c1c0fab90513fc05861561d0880))

## [1.14.3](https://github.com/googleapis/google-cloud-go/compare/retail/v1.14.2...retail/v1.14.3) (2023-10-26)


### Bug Fixes

* **retail:** Update grpc-go to v1.59.0 ([81a97b0](https://github.com/googleapis/google-cloud-go/commit/81a97b06cb28b25432e4ece595c55a9857e960b7))

## [1.14.2](https://github.com/googleapis/google-cloud-go/compare/retail/v1.14.1...retail/v1.14.2) (2023-10-12)


### Bug Fixes

* **retail:** Update golang.org/x/net to v0.17.0 ([174da47](https://github.com/googleapis/google-cloud-go/commit/174da47254fefb12921bbfc65b7829a453af6f5d))

## [1.14.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.14.0...retail/v1.14.1) (2023-06-20)


### Bug Fixes

* **retail:** REST query UpdateMask bug ([df52820](https://github.com/googleapis/google-cloud-go/commit/df52820b0e7721954809a8aa8700b93c5662dc9b))

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.13.1...retail/v1.14.0) (2023-05-30)


### Features

* **retail:** Update all direct dependencies ([b340d03](https://github.com/googleapis/google-cloud-go/commit/b340d030f2b52a4ce48846ce63984b28583abde6))

## [1.13.1](https://github.com/googleapis/google-cloud-go/compare/retail/v1.13.0...retail/v1.13.1) (2023-05-08)


### Bug Fixes

* **retail:** Update grpc to v1.55.0 ([1147ce0](https://github.com/googleapis/google-cloud-go/commit/1147ce02a990276ca4f8ab7a1ab65c14da4450ef))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.12.0...retail/v1.13.0) (2023-04-11)


### Features

* **retail:** Add merchant center link service ([23c974a](https://github.com/googleapis/google-cloud-go/commit/23c974a019693e6453c1342cad172df77f86974e))
* **retail:** Add model service ([#7700](https://github.com/googleapis/google-cloud-go/issues/7700)) ([fc90e54](https://github.com/googleapis/google-cloud-go/commit/fc90e54b25bda6b339266e3e5388174339ed6a44))
* **retail:** Support per-entity search and autocomplete ([fc90e54](https://github.com/googleapis/google-cloud-go/commit/fc90e54b25bda6b339266e3e5388174339ed6a44))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.11.0...retail/v1.12.0) (2023-01-04)


### Features

* **retail:** Add REST client ([06a54a1](https://github.com/googleapis/google-cloud-go/commit/06a54a16a5866cce966547c51e203b9e09a25bc0))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.10.0...retail/v1.11.0) (2022-11-03)


### Features

* **retail:** rewrite signatures in terms of new location ([3c4b2b3](https://github.com/googleapis/google-cloud-go/commit/3c4b2b34565795537aac1661e6af2442437e34ad))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.9.0...retail/v1.10.0) (2022-10-25)


### Features

* **retail:** start generating stubs dir ([de2d180](https://github.com/googleapis/google-cloud-go/commit/de2d18066dc613b72f6f8db93ca60146dabcfdcc))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.8.0...retail/v1.9.0) (2022-09-21)


### Features

* **retail:** rewrite signatures in terms of new types for betas ([9f303f9](https://github.com/googleapis/google-cloud-go/commit/9f303f9efc2e919a9a6bd828f3cdb1fcb3b8b390))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.7.0...retail/v1.8.0) (2022-09-19)


### Features

* **retail:** start generating proto message types ([563f546](https://github.com/googleapis/google-cloud-go/commit/563f546262e68102644db64134d1071fc8caa383))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.6.0...retail/v1.7.0) (2022-09-15)


### Features

* **retail/apiv2alpha:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))
* **retail/apiv2beta:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.5.0...retail/v1.6.0) (2022-09-06)


### Features

* **retail:** release Control and ServingConfig serivces to v2 version feat: release AttributesConfig APIs to v2 version feat: release CompletionConfig APIs to v2 version feat: add local inventories info to the Product resource docs: Improved documentation for Fullfillment and Inventory API in ProductService docs: minor documentation format and typo fixes ([204b856](https://github.com/googleapis/google-cloud-go/commit/204b85632f2556ab2c74020250850b53f6a405ff))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.4.0...retail/v1.5.0) (2022-08-02)


### Features

* **retail:** new model service to manage recommendation models feat: support case insensitive match on search facets feat: allow disabling spell check in search requests feat: allow adding labels in search requests feat: allow returning min/max values on search numeric facets feat: allow using serving configs as an alias of placements feat: allow enabling recommendation filtering on custom attributes feat: return output BigQuery table on product / event export response feat: allow skiping default branch protection when doing product full import docs: keep the API doc up-to-date with recent changes ([338d9c3](https://github.com/googleapis/google-cloud-go/commit/338d9c38b9c7f1b5e75493a2e3437c50785c561c))
* **retail:** support case insensitive match on search facets feat: allow disabling spell check in search requests feat: allow adding labels in search requests feat: allow returning min/max values on search numeric facets feat: allow using serving configs as an alias of placements feat: allow enabling recommendation filtering on custom attributes feat: return output BigQuery table on product / event export response docs: keep the API doc up-to-date with recent changes ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))
* **retail:** support case insensitive match on search facets feat: allow to return min/max values on search numeric facets feat: allow to use serving configs as an alias of placements docs: keep the API doc up-to-date with recent changes ([338d9c3](https://github.com/googleapis/google-cloud-go/commit/338d9c38b9c7f1b5e75493a2e3437c50785c561c))

## [1.4.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.3.0...retail/v1.4.0) (2022-06-01)


### Features

* **retail:** allow users to disable spell check in search requests feat: allow users to add labels in search requests docs: deprecate indexable/searchable on the product level custom attributes docs: keep the API doc up-to-date with recent changes ([5ed25c5](https://github.com/googleapis/google-cloud-go/commit/5ed25c5e2e40c7602802c35d61742631b619ed3c))

## [1.3.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.2.0...retail/v1.3.0) (2022-05-24)


### Features

* **retail:** start generating apiv2alpha and apiv2beta ([#6073](https://github.com/googleapis/google-cloud-go/issues/6073)) ([ec90f7b](https://github.com/googleapis/google-cloud-go/commit/ec90f7b224c67a02eb293224916c019054f5749d))

## [1.2.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.1.0...retail/v1.2.0) (2022-02-23)


### Features

* **retail:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.1.0](https://github.com/googleapis/google-cloud-go/compare/retail/v1.0.0...retail/v1.1.0) (2022-02-14)


### Features

* **retail:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out retail as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
