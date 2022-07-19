// Copyright 2022 Google LLC
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

package managedwriter

import (
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc/status"
)

// StorageErrorFromError attempts to extract a storage error if it is present.
func StorageErrorFromError(err error) *storagepb.StorageError {
	status, ok := status.FromError(err)
	if !ok {
		return nil
	}
	for _, detail := range status.Details() {
		if se, ok := detail.(*storagepb.StorageError); ok {
			return se
		}
	}
	return nil
}
