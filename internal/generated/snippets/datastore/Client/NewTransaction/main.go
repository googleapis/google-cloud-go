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

// [START datastore_generated_datastore_Client_NewTransaction]

package main

import (
	"context"

	"cloud.google.com/go/datastore"
)

func main() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	const retries = 3

	// Increment a counter.
	// See https://cloud.google.com/appengine/articles/sharding_counters for
	// a more scalable solution.
	type Counter struct {
		Count int
	}

	key := datastore.NameKey("counter", "CounterA", nil)
	var tx *datastore.Transaction
	for i := 0; i < retries; i++ {
		tx, err = client.NewTransaction(ctx)
		if err != nil {
			break
		}

		var c Counter
		if err = tx.Get(key, &c); err != nil && err != datastore.ErrNoSuchEntity {
			break
		}
		c.Count++
		if _, err = tx.Put(key, &c); err != nil {
			break
		}

		// Attempt to commit the transaction. If there's a conflict, try again.
		if _, err = tx.Commit(); err != datastore.ErrConcurrentTransaction {
			break
		}
	}
	if err != nil {
		// TODO: Handle error.
	}
}

// [END datastore_generated_datastore_Client_NewTransaction]
