// Copyright 2024 Google LLC
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

package dataflux_test

import (
	"context"
	"log"

	"cloud.google.com/go/storage"
	"cloud.google.com/go/storage/dataflux"
	"google.golang.org/api/iterator"
)

func ExampleNextBatch_batch() {
	ctx := context.Background()
	// Pass in any client opts or set retry policy here.
	client, err := storage.NewClient(ctx)
	if err != nil {
		// handle error
	}

	// Create dataflux fast-list input and provide desired options,
	//  including number of workers, batch size, query to filer objects, etc.
	in := &dataflux.ListerInput{
		BucketName: "mybucket",
		// Optionally specify params to apply to lister.
		Parallelism:          100,
		BatchSize:            500000,
		Query:                storage.Query{},
		SkipDirectoryObjects: false,
	}

	// Create Lister with desired options, including number of workers,
	// part size, per operation timeout, etc.
	df, close := dataflux.NewLister(client, in)
	defer close()

	var numOfObjects int

	for {
		objects, err := df.NextBatch(ctx)
		if err != nil {
			// handle error
		}

		if err == iterator.Done {
			numOfObjects += len(objects)
			// No more objects in the bucket to list.
			break
		}
		if err != nil {
			// handle error
		}
		numOfObjects += len(objects)
	}
	log.Printf("listing %d objects in bucket %q is complete.", numOfObjects, in.BucketName)
}
