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

// [START storage_generated_storage_ObjectHandle_Delete]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// To delete multiple objects in a bucket, list them with an
	// ObjectIterator, then Delete them.

	// If you are using this package on the App Engine Flex runtime,
	// you can init a bucket client with your app's default bucket name.
	// See http://godoc.org/google.golang.org/appengine/file#DefaultBucketName.
	bucket := client.Bucket("my-bucket")
	it := bucket.Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err != nil && err != iterator.Done {
			// TODO: Handle error.
		}
		if err == iterator.Done {
			break
		}
		if err := bucket.Object(objAttrs.Name).Delete(ctx); err != nil {
			// TODO: Handle error.
		}
	}
	fmt.Println("deleted all object items in the bucket specified.")
}

// [END storage_generated_storage_ObjectHandle_Delete]
