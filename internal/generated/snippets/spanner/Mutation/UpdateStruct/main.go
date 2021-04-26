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

// [START spanner_generated_spanner_UpdateStruct]

package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	type account struct {
		User    string `spanner:"user"`
		Balance int64  `spanner:"balance"`
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			return err
		}
		var balance int64
		if err := row.Column(0, &balance); err != nil {
			return err
		}
		m, err := spanner.UpdateStruct("Accounts", account{
			User:    "alice",
			Balance: balance + 10,
		})
		if err != nil {
			return err
		}
		return txn.BufferWrite([]*spanner.Mutation{m})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// [END spanner_generated_spanner_UpdateStruct]
