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

// [START spanner_generated_spanner_Client_ReadWriteTransaction]

package main

import (
	"context"
	"errors"

	"cloud.google.com/go/spanner"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var balance int64
		row, err := txn.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
			// This function will be called again if this is an IsAborted error.
			return err
		}
		if err := row.Column(0, &balance); err != nil {
			return err
		}

		if balance <= 10 {
			return errors.New("insufficient funds in account")
		}
		balance -= 10
		m := spanner.Update("Accounts", []string{"user", "balance"}, []interface{}{"alice", balance})
		// The buffered mutation will be committed. If the commit fails with an
		// IsAborted error, this function will be called again.
		return txn.BufferWrite([]*spanner.Mutation{m})
	})
	if err != nil {
		// TODO: Handle error.
	}
}

// [END spanner_generated_spanner_Client_ReadWriteTransaction]
