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

// [START storage_generated_storage_ObjectHandle_Attrs_withConditions]

package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	// Read the object.
	objAttrs1, err := obj.Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Do something else for a while.
	time.Sleep(5 * time.Minute)
	// Now read the same contents, even if the object has been written since the last read.
	objAttrs2, err := obj.Generation(objAttrs1.Generation).Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(objAttrs1, objAttrs2)
}

// [END storage_generated_storage_ObjectHandle_Attrs_withConditions]
