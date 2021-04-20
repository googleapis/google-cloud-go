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

// [START storage_generated_storage_Copier_Run_progress]

package main

import (
	"context"
	"log"

	"cloud.google.com/go/storage"
)

func main() {
	// Display progress across multiple rewrite RPCs.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	src := client.Bucket("bucketname").Object("file1")
	dst := client.Bucket("another-bucketname").Object("file2")

	copier := dst.CopierFrom(src)
	copier.ProgressFunc = func(copiedBytes, totalBytes uint64) {
		log.Printf("copy %.1f%% done", float64(copiedBytes)/float64(totalBytes)*100)
	}
	if _, err := copier.Run(ctx); err != nil {
		// TODO: handle error.
	}
}

// [END storage_generated_storage_Copier_Run_progress]
