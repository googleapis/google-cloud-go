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

// [START datastore_generated_datastore_Client_RunInTransaction]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/datastore"
)

func main() {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Increment a counter.
	// See https://cloud.google.com/appengine/articles/sharding_counters for
	// a more scalable solution.
	type Counter struct {
		Count int
	}

	var count int
	key := datastore.NameKey("Counter", "singleton", nil)
	_, err = client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var x Counter
		if err := tx.Get(key, &x); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		x.Count++
		if _, err := tx.Put(key, &x); err != nil {
			return err
		}
		count = x.Count
		return nil
	})
	if err != nil {
		// TODO: Handle error.
	}
	// The value of count is only valid once the transaction is successful
	// (RunInTransaction has returned nil).
	fmt.Printf("Count=%d\n", count)
}

// [END datastore_generated_datastore_Client_RunInTransaction]
