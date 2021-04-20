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

// [START spanner_generated_spanner_Client_BatchReadOnlyTransaction]

package main

import (
	"context"
	"sync"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	var (
		client *spanner.Client
		txn    *spanner.BatchReadOnlyTransaction
		err    error
	)
	if client, err = spanner.NewClient(ctx, myDB); err != nil {
		// TODO: Handle error.
	}
	defer client.Close()
	if txn, err = client.BatchReadOnlyTransaction(ctx, spanner.StrongRead()); err != nil {
		// TODO: Handle error.
	}
	defer txn.Close()

	// Singer represents the elements in a row from the Singers table.
	type Singer struct {
		SingerID   int64
		FirstName  string
		LastName   string
		SingerInfo []byte
	}
	stmt := spanner.Statement{SQL: "SELECT * FROM Singers;"}
	partitions, err := txn.PartitionQuery(ctx, stmt, spanner.PartitionOptions{})
	if err != nil {
		// TODO: Handle error.
	}
	// Note: here we use multiple goroutines, but you should use separate
	// processes/machines.
	wg := sync.WaitGroup{}
	for i, p := range partitions {
		wg.Add(1)
		go func(i int, p *spanner.Partition) {
			defer wg.Done()
			iter := txn.Execute(ctx, p)
			defer iter.Stop()
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				} else if err != nil {
					// TODO: Handle error.
				}
				var s Singer
				if err := row.ToStruct(&s); err != nil {
					// TODO: Handle error.
				}
				_ = s // TODO: Process the row.
			}
		}(i, p)
	}
	wg.Wait()
}

// [END spanner_generated_spanner_Client_BatchReadOnlyTransaction]
