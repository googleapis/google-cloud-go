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

// [START storage_generated_storage_ObjectHandle_NewRangeReader]

package main

import (
	"context"
	"fmt"
	"io/ioutil"

	"cloud.google.com/go/storage"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Read only the first 64K.
	rc, err := client.Bucket("bucketname").Object("filename1").NewRangeReader(ctx, 0, 64*1024)
	if err != nil {
		// TODO: handle error.
	}
	defer rc.Close()

	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Printf("first 64K of file contents:\n%s\n", slurp)
}

// [END storage_generated_storage_ObjectHandle_NewRangeReader]
