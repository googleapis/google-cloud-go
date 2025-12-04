# Changes

## [0.3.0](https://github.com/googleapis/google-cloud-go/releases/tag/geminidataanalytics%2Fv0.3.0) (2025-12-04)

### Features

* Adding DatasourceOptions to provide configuration options for datasources ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding a DeleteConversation RPC to allow for the deletion of conversations ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding a GlossaryTerm message to allow users to provide definitions for domain-specific terms ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding a new SchemaRelationship message to define relationships between table schema ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding a new TextType PROGRESS to provide informational messages about an agent&#39;s progress for supporting more granular Agent RAG tools ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding an ExampleQueries message to surface derived and authored example queries ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding client_managed_resource_context to allow clients to manage their own conversation and agent resources ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding struct_schema to Datasource to support flexible schemas, particularly for Looker datasources ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))
* Adding support for LookerQuery within the DataQuery message for retrieving data from Looker explores ([185951b](https://github.com/googleapis/google-cloud-go/commit/185951b3bea9fb942979e81ce248ccdebb40d94b))

## [0.2.1](https://github.com/googleapis/google-cloud-go/compare/geminidataanalytics/v0.2.0...geminidataanalytics/v0.2.1) (2025-09-18)


### Bug Fixes

* **geminidataanalytics:** Upgrade gRPC service registration func ([a10ecc9](https://github.com/googleapis/google-cloud-go/commit/a10ecc9b3c22e320e9a32dedef7248b42465cd49))

## [0.2.0](https://github.com/googleapis/google-cloud-go/compare/geminidataanalytics/v0.1.0...geminidataanalytics/v0.2.0) (2025-09-04)


### Features

* **geminidataanalytics:** A new enum `DataFilterType` is added ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `description` is added to message `.google.cloud.geminidataanalytics.v1alpha.Schema` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `example_queries` is added to message `.google.cloud.geminidataanalytics.v1alpha.Context` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `filters` is added to message `.google.cloud.geminidataanalytics.v1alpha.Schema` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `schema` is added to message `.google.cloud.geminidataanalytics.v1alpha.BigQueryTableReference` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `synonyms` is added to message `.google.cloud.geminidataanalytics.v1alpha.Field` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `synonyms` is added to message `.google.cloud.geminidataanalytics.v1alpha.Schema` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `tags` is added to message `.google.cloud.geminidataanalytics.v1alpha.Field` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `tags` is added to message `.google.cloud.geminidataanalytics.v1alpha.Schema` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new field `value_format` is added to message `.google.cloud.geminidataanalytics.v1alpha.Field` ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new message `DataFilter` is added ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))
* **geminidataanalytics:** A new message `ExampleQuery` is added ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))


### Bug Fixes

* **geminidataanalytics:** An existing service `ContextRetrievalService` is removed ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))


### Documentation

* **geminidataanalytics:** Many comment updates ([7e241f3](https://github.com/googleapis/google-cloud-go/commit/7e241f3c17e44e83f858ac142ebedc916330651e))

## 0.1.0 (2025-08-18)


### Features

* **geminidataanalytics:** New client ([#12729](https://github.com/googleapis/google-cloud-go/issues/12729)) ([1bc6c98](https://github.com/googleapis/google-cloud-go/commit/1bc6c98c371418b05cbe13a95a601e08d1c97014))

## Changes
