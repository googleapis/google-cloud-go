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

// Package managedwriter will be a thick client around the storage API's BigQueryWriteClient.
//
// It is EXPERIMENTAL and subject to change or removal without notice.
//
// Currently, the BigQueryWriteClient this library targets is exposed in the storage v1beta2 endpoint, and is
// a successor to the streaming interface.  API method tabledata.insertAll is the primary backend method, and
// the Inserter abstraction is the equivalent to this in the cloud.google.com/go/bigquery package.
package managedwriter
