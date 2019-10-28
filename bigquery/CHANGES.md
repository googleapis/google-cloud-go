# Changes

## v1.2.0

* Adds support for scripting feature, which includes script statistics
  and the ability to list jobs run as part of a script query.

* Updates default endpoint for BigQuery from www.googleapis.com
  to bigquery.googleapis.com.

## v1.1.0

* Added support for specifying default `EncryptionConfig` settings on the
  dataset.

* Added support for `EncyptionConfig` as part of an ML model.

* Added `Relax()` to make all fields within a `Schema` nullable.

* Added a `UseAvroLogicalTypes` option when defining an avro extract job.

## v1.0.1

This patch release is a small fix to the go.mod to point to the post-carve out
cloud.google.com/go.

## v1.0.0

This is the first tag to carve out bigquery as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
