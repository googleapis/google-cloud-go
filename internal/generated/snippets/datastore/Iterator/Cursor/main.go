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

// [START datastore_generated_datastore_Iterator_Cursor]

package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

type Post struct {
	Title       string
	PublishedAt time.Time
	Comments    int
}

func main() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	it := client.Run(ctx, datastore.NewQuery("Post"))
	for {
		var p Post
		_, err := it.Next(&p)
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(p)
		cursor, err := it.Cursor()
		if err != nil {
			// TODO: Handle error.
		}
		// When printed, a cursor will display as a string that can be passed
		// to datastore.DecodeCursor.
		fmt.Printf("to resume with this post, use cursor %s\n", cursor)
	}
}

// [END datastore_generated_datastore_Iterator_Cursor]
