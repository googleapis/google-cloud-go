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

// [START storage_generated_storage_Client_ListHMACKeys_showDeletedKeys]

package main

import (
	"context"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}

	iter := client.ListHMACKeys(ctx, "project-id", storage.ShowDeletedHMACKeys())
	for {
		key, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: handle error.
		}
		_ = key // TODO: Use the key.
	}
}

// [END storage_generated_storage_Client_ListHMACKeys_showDeletedKeys]
