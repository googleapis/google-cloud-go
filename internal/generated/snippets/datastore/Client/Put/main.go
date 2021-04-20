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

// [START datastore_generated_datastore_Client_Put]

package main

import (
	"context"
	"time"

	"cloud.google.com/go/datastore"
)

func main() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	type Article struct {
		Title       string
		Description string
		Body        string `datastore:",noindex"`
		Author      *datastore.Key
		PublishedAt time.Time
	}
	newKey := datastore.IncompleteKey("Article", nil)
	_, err = client.Put(ctx, newKey, &Article{
		Title:       "The title of the article",
		Description: "The description of the article...",
		Body:        "...",
		Author:      datastore.NameKey("Author", "jbd", nil),
		PublishedAt: time.Now(),
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// [END datastore_generated_datastore_Client_Put]
