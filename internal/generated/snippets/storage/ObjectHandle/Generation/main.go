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

// [START storage_generated_storage_ObjectHandle_Generation]

package main

import (
	"context"
	"io"
	"os"

	"cloud.google.com/go/storage"
)

var gen int64

func main() {
	// Read an object's contents from generation gen, regardless of the
	// current generation of the object.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	rc, err := obj.Generation(gen).NewReader(ctx)
	if err != nil {
		// TODO: handle error.
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		// TODO: handle error.
	}
}

// [END storage_generated_storage_ObjectHandle_Generation]
