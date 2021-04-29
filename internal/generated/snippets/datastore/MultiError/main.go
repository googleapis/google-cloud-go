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

// [START datastore_generated_datastore_MultiError]

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

	keys := []*datastore.Key{
		datastore.NameKey("bad-key", "bad-key", nil),
	}
	posts := make([]Post, 1)
	if err := client.GetMulti(ctx, keys, posts); err != nil {
		if merr, ok := err.(datastore.MultiError); ok {
			for _, err := range merr {
				// TODO: Handle error.
				_ = err
			}
		} else {
			// TODO: Handle error.
		}
	}
}

// [END datastore_generated_datastore_MultiError]
