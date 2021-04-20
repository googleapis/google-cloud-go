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

// [START datastore_generated_datastore_Client_Mutate]

package main

import (
	"context"
	"time"

	"cloud.google.com/go/datastore"
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

	key1 := datastore.NameKey("Post", "post1", nil)
	key2 := datastore.NameKey("Post", "post2", nil)
	key3 := datastore.NameKey("Post", "post3", nil)
	key4 := datastore.NameKey("Post", "post4", nil)

	_, err = client.Mutate(ctx,
		datastore.NewInsert(key1, Post{Title: "Post 1"}),
		datastore.NewUpsert(key2, Post{Title: "Post 2"}),
		datastore.NewUpdate(key3, Post{Title: "Post 3"}),
		datastore.NewDelete(key4))
	if err != nil {
		// TODO: Handle error.
	}
}

// [END datastore_generated_datastore_Client_Mutate]
