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

// [START datastore_generated_datastore_Commit_Key]

package main

import (
	"context"
	"fmt"
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
	client, err := datastore.NewClient(ctx, "")
	if err != nil {
		// TODO: Handle error.
	}
	var pk1, pk2 *datastore.PendingKey
	// Create two posts in a single transaction.
	commit, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var err error
		pk1, err = tx.Put(datastore.IncompleteKey("Post", nil), &Post{Title: "Post 1", PublishedAt: time.Now()})
		if err != nil {
			return err
		}
		pk2, err = tx.Put(datastore.IncompleteKey("Post", nil), &Post{Title: "Post 2", PublishedAt: time.Now()})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// TODO: Handle error.
	}
	// Now pk1, pk2 are valid PendingKeys. Let's convert them into real keys
	// using the Commit object.
	k1 := commit.Key(pk1)
	k2 := commit.Key(pk2)
	fmt.Println(k1, k2)
}

// [END datastore_generated_datastore_Commit_Key]
