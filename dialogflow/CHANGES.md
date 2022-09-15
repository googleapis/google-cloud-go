# Changes

## [1.14.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.13.0...dialogflow/v1.14.0) (2022-09-15)


### Features

* **dialogflow/apiv2beta1:** add REST transport ([f7b0822](https://github.com/googleapis/google-cloud-go/commit/f7b082212b1e46ff2f4126b52d49618785c2e8ca))

## [1.13.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.12.1...dialogflow/v1.13.0) (2022-09-06)


### Features

* **dialogflow:** start generating apiv2beta1 ([#6601](https://github.com/googleapis/google-cloud-go/issues/6601)) ([6f8b1eb](https://github.com/googleapis/google-cloud-go/commit/6f8b1eb205740568be20c9d1094860812aa27cb1))

## [1.12.1](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.12.0...dialogflow/v1.12.1) (2022-08-02)


### Documentation

* **dialogflow:** added an explicit note that DetectIntentRequest's text input is limited by 256 characters ([1d6fbcc](https://github.com/googleapis/google-cloud-go/commit/1d6fbcc6406e2063201ef5a98de560bf32f7fb73))

## [1.12.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.11.0...dialogflow/v1.12.0) (2022-07-12)


### Features

* **dialogflow:** deprecated the filter field and add resource_definition docs: add more meaningful comments ([8a1ad06](https://github.com/googleapis/google-cloud-go/commit/8a1ad06572a65afa91a0a77a85b849e766876671))


### Documentation

* **dialogflow/cx:** clarify descriptions of the AdvancedSettings and WebhookRequest data types ([1732e43](https://github.com/googleapis/google-cloud-go/commit/1732e4334c84019d93775d861be5c0008e3f5245))
* **dialogflow/cx:** improve comments for protos ([963efe2](https://github.com/googleapis/google-cloud-go/commit/963efe22cf67bc04fed09b5fa8f9cb20b9edf1a3))

## [1.11.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.10.0...dialogflow/v1.11.0) (2022-06-29)


### Features

* **dialogflow:** start generating REST client for beta clients ([25b7775](https://github.com/googleapis/google-cloud-go/commit/25b77757c1e6f372e03bf99ab7461264bba48d26))

## [1.10.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.9.0...dialogflow/v1.10.0) (2022-06-16)


### Features

* **dialogflow/cx:** added webhook_config ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))
* **dialogflow/cx:** added webhook_config ([90489b1](https://github.com/googleapis/google-cloud-go/commit/90489b10fd7da4cfafe326e00d1f4d81570147f7))

## [1.9.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.8.1...dialogflow/v1.9.0) (2022-05-24)


### Features

* **dialogflow/cx:** add audio export settings to security settings docs: update the doc on diagnostic info ([6ef576e](https://github.com/googleapis/google-cloud-go/commit/6ef576e2d821d079e7b940cd5d49fe3ca64a7ba2))

### [1.8.1](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.8.0...dialogflow/v1.8.1) (2022-04-20)


### Bug Fixes

* **dialogflow:** correct broken ConversationModelEvaluation resource pattern ([689cad9](https://github.com/googleapis/google-cloud-go/commit/689cad94fdcf54cebd22aecfcdad4d8b44f58df9))

## [1.8.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.7.0...dialogflow/v1.8.0) (2022-04-06)


### Features

* **dialogflow/cx:** added support for locking an agent for changes feat: added data format specification for export agent ([81c4c91](https://github.com/googleapis/google-cloud-go/commit/81c4c9116178711059772f41bbf76d423ffebc95))
* **dialogflow/cx:** added support for locking an agent for changes feat: added data format specification for export agent ([81c4c91](https://github.com/googleapis/google-cloud-go/commit/81c4c9116178711059772f41bbf76d423ffebc95))

## [1.7.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.6.0...dialogflow/v1.7.0) (2022-03-14)


### Features

* **dialogflow:** added ConversationModel resource and its APIs feat: added ConversationDataset resource and its APIs feat: added SetSuggestionFeatureConfig and ClearSuggestionFeatureConfig APIs for ConversationProfile feat: added new knowledge type of Document content feat: added states of Document feat: added metadata for the Knowledge operation docs: updated copyright ([96c9d7e](https://github.com/googleapis/google-cloud-go/commit/96c9d7ee74af075fdd17b02233ac92fba6d89988))

## [1.6.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.5.0...dialogflow/v1.6.0) (2022-02-23)


### Features

* **dialogflow:** set versionClient to module version ([55f0d92](https://github.com/googleapis/google-cloud-go/commit/55f0d92bf112f14b024b4ab0076c9875a17423c9))

## [1.5.0](https://github.com/googleapis/google-cloud-go/compare/dialogflow/v1.4.0...dialogflow/v1.5.0) (2022-02-14)


### Features

* **dialogflow:** add file for tracking version ([17b36ea](https://github.com/googleapis/google-cloud-go/commit/17b36ead42a96b1a01105122074e65164357519e))

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/dialogflow/v1.3.0...dialogflow/v1.4.0) (2022-02-03)


### Features

* **dialogflow:** added conversation process config, ImportDocument and SuggestSmartReplies API ([f560b1e](https://www.github.com/googleapis/google-cloud-go/commit/f560b1ed0263956ef84fbf2fbf34bdc66dbc0a88))

## [1.3.0](https://www.github.com/googleapis/google-cloud-go/compare/dialogflow/v1.2.0...dialogflow/v1.3.0) (2022-01-04)


### Features

* **dialogflow/cx:** added `TelephonyTransferCall` in response message ([fe27098](https://www.github.com/googleapis/google-cloud-go/commit/fe27098e5d429911428821ded57384353e699774))
* **dialogflow/cx:** added support for comparing between versions docs: clarified security settings API reference ([83b941c](https://www.github.com/googleapis/google-cloud-go/commit/83b941c0983e44fdd18ceee8c6f3e91219d72ad1))
* **dialogflow/cx:** added the display name of the current page in webhook requests ([e0833b2](https://www.github.com/googleapis/google-cloud-go/commit/e0833b2853834ba79fd20ca2ae9c613d585dd2a5))
* **dialogflow/cx:** added the display name of the current page in webhook requests ([e0833b2](https://www.github.com/googleapis/google-cloud-go/commit/e0833b2853834ba79fd20ca2ae9c613d585dd2a5))
* **dialogflow/cx:** allow setting custom CA for generic webhooks and release CompareVersions API docs: clarify DLP template reader usage ([90e2868](https://www.github.com/googleapis/google-cloud-go/commit/90e2868a3d220aa7f897438f4917013fda7a7c59))
* **dialogflow:** added export documentation method feat: added filter in list documentations request feat: added option to import custom metadata from Google Cloud Storage in reload document request feat: added option to apply partial update to the smart messaging allowlist in reload document request feat: added filter in list knowledge bases request ([5444809](https://www.github.com/googleapis/google-cloud-go/commit/5444809e0b7cf9f5416645ea2df6fec96f8b9023))
* **dialogflow:** added support to configure security settings, language code and time zone on conversation profile ([1f5aa78](https://www.github.com/googleapis/google-cloud-go/commit/1f5aa78a4d6633871651c89a6d9c48e3409fecc5))
* **dialogflow:** removed OPTIONAL for speech model variant docs: added more docs for speech model variant and improved docs format for participant ([5444809](https://www.github.com/googleapis/google-cloud-go/commit/5444809e0b7cf9f5416645ea2df6fec96f8b9023))
* **dialogflow:** support document metadata filter in article suggestion and smart reply model in human agent assistant ([e33350c](https://www.github.com/googleapis/google-cloud-go/commit/e33350cfcabcddcda1a90069383d39c68deb977a))

## [1.2.0](https://www.github.com/googleapis/google-cloud-go/compare/dialogflow/v1.1.0...dialogflow/v1.2.0) (2021-11-02)


### Features

* **dialogflow/cx:** added API for changelogs docs: clarified semantic of the streaming APIs ([587bba5](https://www.github.com/googleapis/google-cloud-go/commit/587bba5ad792a92f252107aa38c6af50fb09fb58))
* **dialogflow/cx:** added API for changelogs docs: clarified semantic of the streaming APIs ([587bba5](https://www.github.com/googleapis/google-cloud-go/commit/587bba5ad792a92f252107aa38c6af50fb09fb58))

## [1.1.0](https://www.github.com/googleapis/google-cloud-go/compare/dialogflow/v1.0.0...dialogflow/v1.1.0) (2021-10-18)


### Features

* **dialogflow/cx:** added support for Deployments with ListDeployments and GetDeployment apis feat: added support for DeployFlow api under Environments feat: added support for TestCasesConfig under Environment docs: added long running operation explanation for several apis fix!: marked resource name of security setting as not-required ([8c5c6cf](https://www.github.com/googleapis/google-cloud-go/commit/8c5c6cf9df046b67998a8608d05595bd9e34feb0))

## 1.0.0

Stabilize GA surface.

## v0.1.0

This is the first tag to carve out dialogflow as its own module. See
[Add a module to a multi-module repository](https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository).
