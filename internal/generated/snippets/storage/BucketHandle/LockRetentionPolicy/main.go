// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// [START storage_generated_storage_BucketHandle_LockRetentionPolicy]

package main

import (
	"context"

	"cloud.google.com/go/storage"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	attrs, err := b.Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Note that locking the bucket without first attaching a RetentionPolicy
	// that's at least 1 day is a no-op
	err = b.If(storage.BucketConditions{MetagenerationMatch: attrs.MetaGeneration}).LockRetentionPolicy(ctx)
	if err != nil {
		// TODO: handle err
	}
}

// [END storage_generated_storage_BucketHandle_LockRetentionPolicy]
