// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package managedwriter provides an EXPERIMENTAL thick client around the BigQuery storage API's BigQueryWriteClient.
More information about this new write client may also be found in the public documentation: https://cloud.google.com/bigquery/docs/write-api

It is EXPERIMENTAL and subject to change or removal without notice.  This library is in a pre-alpha
state, and breaking changes are frequent.

Currently, this client targets the BigQueryWriteClient present in the v1 endpoint, and is intended as a more
feature-rich successor to the classic BigQuery streaming interface, which is presented as the Inserter abstraction
in cloud.google.com/go/bigquery, and the tabledata.insertAll method if you're more familiar with the BigQuery v2 REST
methods.

Appending data is accomplished through the use of streams.  There are four stream types, each targetting slightly different
use cases and needs.  See the StreamType documentation for more details about each stream type.

This API uses serialized protocol buffer messages for appending data to streams.  For users who don't have predefined protocol
buffer messages for sending data, the cloud.google.com/go/bigquery/storage/managedwriter/adapt subpackage includes functionality
for defining protocol buffer messages dynamically using table schema information, which enables users to do things like using
protojson to convert json text into a protocol buffer.
*/
package managedwriter
