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

// [START spanner_generated_spanner_KeySets]

package main

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	// Get some rows from the Accounts table using a secondary index. In this case we get all users who are in Georgia.
	iter := client.Single().ReadUsingIndex(context.Background(), "Accounts", "idx_state", spanner.Key{"GA"}, []string{"state"})

	// Create a empty KeySet by calling the KeySets function with no parameters.
	ks := spanner.KeySets()

	// Loop the results of a previous query iterator.
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			// TODO: Handle error.
		}
		var id string
		err = row.ColumnByName("User", &id)
		if err != nil {
			// TODO: Handle error.
		}
		ks = spanner.KeySets(spanner.KeySets(spanner.Key{id}, ks))
	}

	_ = ks //TODO: Go use the KeySet in another query.

}

// [END spanner_generated_spanner_KeySets]
