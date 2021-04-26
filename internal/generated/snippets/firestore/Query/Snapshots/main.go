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

// [START firestore_generated_firestore_Query_Snapshots]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	q := client.Collection("States").
		Where("pop", ">", 10).
		OrderBy("pop", firestore.Desc).
		Limit(10)
	qsnapIter := q.Snapshots(ctx)
	// Listen forever for changes to the query's results.
	for {
		qsnap, err := qsnapIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Printf("At %s there were %d results.\n", qsnap.ReadTime, qsnap.Size)
		_ = qsnap.Documents // TODO: Iterate over the results if desired.
		_ = qsnap.Changes   // TODO: Use the list of incremental changes if desired.
	}
}

// [END firestore_generated_firestore_Query_Snapshots]
