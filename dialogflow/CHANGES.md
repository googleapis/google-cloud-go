# Changes

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
