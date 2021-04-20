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

// [START storage_generated_storage_ObjectHandle_CopierFrom_rotateEncryptionKeys]

package main

import (
	"context"

	"cloud.google.com/go/storage"
)

var key1, key2 []byte

func main() {
	// To rotate the encryption key on an object, copy it onto itself.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("bucketname").Object("obj")
	// Assume obj is encrypted with key1, and we want to change to key2.
	_, err = obj.Key(key2).CopierFrom(obj.Key(key1)).Run(ctx)
	if err != nil {
		// TODO: handle error.
	}
}

// [END storage_generated_storage_ObjectHandle_CopierFrom_rotateEncryptionKeys]
