# Changes

## [2.0.0](https://www.github.com/googleapis/google-cloud-go/compare/firestore-v1.5.0...firestore-v2.0.0) (2021-05-04)


### ⚠ BREAKING CHANGES

* **all:** This is a breaking change in dialogflow

### Features

* **appengine:** added vm_liveness, search_api_available, network_settings, service_account, build_env_variables, kms_key_reference to v1 API ([fd04a55](https://www.github.com/googleapis/google-cloud-go/commit/fd04a552213f99619c714b5858548f61f4948493))
* **assuredworkloads:** Add 'resource_settings' field to provide custom properties (ids) for the provisioned projects. ([ab4824a](https://www.github.com/googleapis/google-cloud-go/commit/ab4824a7914864228e59b244d6382de862139524))
* **assuredworkloads:** add HIPAA and HITRUST compliance regimes ([ab4824a](https://www.github.com/googleapis/google-cloud-go/commit/ab4824a7914864228e59b244d6382de862139524))
* **bigquery/storage:** add a Arrow compression options (Only LZ4 for now). feat: Return schema on first ReadRowsResponse. doc: clarify limit on filter string. ([2b02a03](https://www.github.com/googleapis/google-cloud-go/commit/2b02a03ff9f78884da5a8e7b64a336014c61bde7))
* **bigquery/storage:** deprecate bigquery storage v1alpha2 API ([9cc6d2c](https://www.github.com/googleapis/google-cloud-go/commit/9cc6d2cce96235b0a144c1c6b48eff496f9e5fa7))
* **bigquery/storage:** new JSON type through BigQuery Write ([9029071](https://www.github.com/googleapis/google-cloud-go/commit/90290710158cf63de918c2d790df48f55a23adc5))
* **bigquery/storage:** updates for v1beta2 storage API - Updated comments on BatchCommitWriteStreams - Added new support Bigquery types BIGNUMERIC and INTERVAL to TableSchema - Added read rows schema in ReadRowsResponse - Misc comment updates ([48b4e59](https://www.github.com/googleapis/google-cloud-go/commit/48b4e596206cef879194d2888186d603a6f51292))
* **billing/budgets:** Added support for configurable budget time period. fix: Updated some documentation links. ([83b1b3b](https://www.github.com/googleapis/google-cloud-go/commit/83b1b3b648c6d9225f07f00e8c0cdabc3d1fc1ab))
* **billing/budgets:** Added support for configurable budget time period. fix: Updated some documentation links. ([83b1b3b](https://www.github.com/googleapis/google-cloud-go/commit/83b1b3b648c6d9225f07f00e8c0cdabc3d1fc1ab))
* **channel:** addition of billing_account field on Plan. docs: clarification that valid address lines are required for all customers. ([d4246aa](https://www.github.com/googleapis/google-cloud-go/commit/d4246aad4da3c3ef12350385f229bb908e3fb215))
* **cloudbuild/apiv1:** Add fields for Pub/Sub triggers ([8b4adbf](https://www.github.com/googleapis/google-cloud-go/commit/8b4adbf9815e1ec229dfbcfb9189d3ea63112e1b))
* **datacatalog:** Policy Tag Manager v1 API service feat: new RenameTagTemplateFieldEnumValue API feat: adding fully_qualified_name in lookup and search feat: added DATAPROC_METASTORE integrated system along with new entry types: DATABASE and SERVICE docs: Documentation improvements ([2b02a03](https://www.github.com/googleapis/google-cloud-go/commit/2b02a03ff9f78884da5a8e7b64a336014c61bde7))
* **dataproc:** update the Dataproc V1 API client library ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **datastore/admin:** Added methods for creating and deleting composite indexes feat: Populated php_namespace ([529925b](https://www.github.com/googleapis/google-cloud-go/commit/529925ba79f4d3191ef80a13e566d86210fe4d25))
* **datastore/admin:** Publish message definitions for Cloud Datastore migration logging. ([529925b](https://www.github.com/googleapis/google-cloud-go/commit/529925ba79f4d3191ef80a13e566d86210fe4d25))
* **dialogflow/cx:** added fallback option when restoring an agent docs: clarified experiment length ([cd70aa9](https://www.github.com/googleapis/google-cloud-go/commit/cd70aa9cc1a5dccfe4e49d2d6ca6db2119553c86))
* **dialogflow/cx:** allow to disable webhook invocation per request ([d4246aa](https://www.github.com/googleapis/google-cloud-go/commit/d4246aad4da3c3ef12350385f229bb908e3fb215))
* **dialogflow/cx:** allow to disable webhook invocation per request ([44c6bf9](https://www.github.com/googleapis/google-cloud-go/commit/44c6bf986f39a3c9fddf46788ae63bfbb3739441))
* **dialogflow/cx:** include original user query in WebhookRequest; add GetTextCaseresult API. doc: clarify resource format for session response. ([a0b1f6f](https://www.github.com/googleapis/google-cloud-go/commit/a0b1f6faae77d014fdee166ab018ddcd6f846ab4))
* **dialogflow/cx:** include original user query in WebhookRequest; add GetTextCaseresult API. doc: clarify resource format for session response. ([b5b4da6](https://www.github.com/googleapis/google-cloud-go/commit/b5b4da6952922440d03051f629f3166f731dfaa3))
* **dialogflow/cx:** support setting current_page to resume sessions; expose transition_route_groups in flows and language_code in webhook ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **dialogflow/cx:** support setting current_page to resume sessions; expose transition_route_groups in flows and language_code in webhook ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **dialogflow:** Add CCAI API ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **dialogflow:** added more Environment RPCs feat: added Versions service feat: added Fulfillment service feat: added TextToSpeechSettings. feat: added location in some resource patterns. ([4f73dc1](https://www.github.com/googleapis/google-cloud-go/commit/4f73dc19c2e05ad6133a8eac3d62ddb522314540))
* **dialogflow:** expose MP3_64_KBPS and MULAW for output audio encodings. ([b5b4da6](https://www.github.com/googleapis/google-cloud-go/commit/b5b4da6952922440d03051f629f3166f731dfaa3))
* **documentai:** add confidence field to the PageAnchor.PageRef in document.proto. ([d089dda](https://www.github.com/googleapis/google-cloud-go/commit/d089dda0089acb9aaef9b3da40b219476af9fc06))
* **documentai:** add confidence field to the PageAnchor.PageRef in document.proto. ([07fdcd1](https://www.github.com/googleapis/google-cloud-go/commit/07fdcd12499eac26f9b5fae01d6c1282c3e02b7c))
* **documentai:** add EVAL_SKIPPED value to the Provenance.OperationType enum in document.proto. ([cb43066](https://www.github.com/googleapis/google-cloud-go/commit/cb4306683926843f6e977f207fa6070bb9242a61))
* **documentai:** remove the translation fields in document.proto. ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **documentai:** Update documentai/v1beta3 protos: add support for boolean normalized value ([529925b](https://www.github.com/googleapis/google-cloud-go/commit/529925ba79f4d3191ef80a13e566d86210fe4d25))
* **kms:** Add maxAttempts to retry policy for KMS gRPC service config feat: Add Bazel exports_files entry for KMS gRPC service config ([fd04a55](https://www.github.com/googleapis/google-cloud-go/commit/fd04a552213f99619c714b5858548f61f4948493))
* **metastore:** added support for release channels when creating service ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **metastore:** Publish Dataproc Metastore v1alpha API ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **pubsublite:** add skip_backlog field to allow subscriptions to be created at HEAD ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **pubsublite:** ComputeTimeCursor RPC for Pub/Sub Lite ([d089dda](https://www.github.com/googleapis/google-cloud-go/commit/d089dda0089acb9aaef9b3da40b219476af9fc06))
* **secretmanager:** added topic field to Secret ([f1323b1](https://www.github.com/googleapis/google-cloud-go/commit/f1323b10a3c7cc1d215730cefd3062064ef54c01))
* **secretmanager:** Rotation for Secrets ([2b02a03](https://www.github.com/googleapis/google-cloud-go/commit/2b02a03ff9f78884da5a8e7b64a336014c61bde7))
* **spanner/admin/database:** add `progress` field to `UpdateDatabaseDdlMetadata` ([9029071](https://www.github.com/googleapis/google-cloud-go/commit/90290710158cf63de918c2d790df48f55a23adc5))
* **spanner/admin/database:** add tagging request options ([2b02a03](https://www.github.com/googleapis/google-cloud-go/commit/2b02a03ff9f78884da5a8e7b64a336014c61bde7))
* **spanner:** add `optimizer_statistics_package` field in `QueryOptions` ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **spanner:** add RPC Priority request options ([b5b4da6](https://www.github.com/googleapis/google-cloud-go/commit/b5b4da6952922440d03051f629f3166f731dfaa3))
* **speech:** add webm opus support. ([d089dda](https://www.github.com/googleapis/google-cloud-go/commit/d089dda0089acb9aaef9b3da40b219476af9fc06))
* **speech:** Support for spoken punctuation and spoken emojis. ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **speech:** Support output transcript to GCS for LongRunningRecognize. ([fd04a55](https://www.github.com/googleapis/google-cloud-go/commit/fd04a552213f99619c714b5858548f61f4948493))
* **speech:** Support output transcript to GCS for LongRunningRecognize. ([cd70aa9](https://www.github.com/googleapis/google-cloud-go/commit/cd70aa9cc1a5dccfe4e49d2d6ca6db2119553c86))
* **speech:** Support output transcript to GCS for LongRunningRecognize. ([35a8706](https://www.github.com/googleapis/google-cloud-go/commit/35a870662df8bf63c4ec10a0233d1d7a708007ee))


### Bug Fixes

* **analytics/admin:** add `https://www.googleapis.com/auth/analytics.edit` OAuth2 scope to the list of acceptable scopes for all read only methods of the Admin API docs: update the documentation of the `update_mask` field used by Update() methods ([f1323b1](https://www.github.com/googleapis/google-cloud-go/commit/f1323b10a3c7cc1d215730cefd3062064ef54c01))
* **apigateway:** Provide resource definitions for service management and IAM resources ([18c88c4](https://www.github.com/googleapis/google-cloud-go/commit/18c88c437bd1741eaf5bf5911b9da6f6ea7cd75d))
* **binaryauthorization:** add Java options to Binaryauthorization protos ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **firestore:** retry RESOURCE_EXHAUSTED errors docs: various documentation improvements ([9a459d5](https://www.github.com/googleapis/google-cloud-go/commit/9a459d5d149b9c3b02a35d4245d164b899ff09b3))
* **functions:** Fix service namespace in grpc_service_config. ([7811a34](https://www.github.com/googleapis/google-cloud-go/commit/7811a34ef64d722480c640810251bb3a0d65d495))
* **metastore:** increase metastore lro polling timeouts ([83b1b3b](https://www.github.com/googleapis/google-cloud-go/commit/83b1b3b648c6d9225f07f00e8c0cdabc3d1fc1ab))


### Miscellaneous Chores

* **all:** auto-regenerate gapics ([#3837](https://www.github.com/googleapis/google-cloud-go/issues/3837)) ([ab4824a](https://www.github.com/googleapis/google-cloud-go/commit/ab4824a7914864228e59b244d6382de862139524))

## [1.5.0](https://www.github.com/googleapis/google-cloud-go/compare/v1.4.0...v1.5.0) (2021-02-24)


### Features

* **firestore:** add opencensus tracing support  ([#2942](https://www.github.com/googleapis/google-cloud-go/issues/2942)) ([257f322](https://www.github.com/googleapis/google-cloud-go/commit/257f322e68b75765bd316ccefed5461d4df538a0))


### Bug Fixes

* **firestore:** address a missing branch in watch.stop() error remapping ([#3643](https://www.github.com/googleapis/google-cloud-go/issues/3643)) ([89ad55d](https://www.github.com/googleapis/google-cloud-go/commit/89ad55d72f79995a68f9c2ed1cd9b5ba50009d6d))

## [1.4.0](https://www.github.com/googleapis/google-cloud-go/compare/firestore/v1.3.0...v1.4.0) (2020-12-03)


### Features

* **firestore:** support "!=" and "not-in" query operators ([#3207](https://www.github.com/googleapis/google-cloud-go/issues/3207)) ([5c44019](https://www.github.com/googleapis/google-cloud-go/commit/5c440192105fe3e9b5dd1b584118b309113935e3)), closes [/firebase.google.com/support/release-notes/js#version_7210_-_september_17_2020](https://www.github.com/googleapis//firebase.google.com/support/release-notes/js/issues/version_7210_-_september_17_2020)

## v1.3.0

- Add support for LimitToLast feature for queries. This allows
  a query to return the final N results. See docs
  [here](https://firebase.google.com/docs/reference/js/firebase.database.Query#limittolast).
- Add support for FieldTransformMinimum and FieldTransformMaximum.
- Add exported SetGoogleClientInfo method.
- Various updates to autogenerated clients.

## v1.2.0

- Deprecate v1beta1 client.
- Fix serverTimestamp docs.
- Add missing operators to query docs.
- Make document IDs 20 alpha-numeric characters. Previously, there could be more
  than 20 non-alphanumeric characters, which broke some users. See
  https://github.com/googleapis/google-cloud-go/issues/1715.
- Various updates to autogenerated clients.

## v1.1.1

- Fix bug in CollectionGroup query validation.

## v1.1.0

- Add support for `in` and `array-contains-any` query operators.

## v1.0.0

This is the first tag to carve out firestore as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
