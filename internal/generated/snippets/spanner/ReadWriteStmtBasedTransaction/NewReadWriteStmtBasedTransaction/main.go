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

// [START spanner_generated_spanner_NewReadWriteStmtBasedTransaction]

package main

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const myDB = "projects/my-project/instances/my-instance/database/my-db"

func main() {
	ctx := context.Background()
	client, err := spanner.NewClient(ctx, myDB)
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

	f := func(tx *spanner.ReadWriteStmtBasedTransaction) error {
		var balance int64
		row, err := tx.ReadRow(ctx, "Accounts", spanner.Key{"alice"}, []string{"balance"})
		if err != nil {
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
		return tx.BufferWrite([]*spanner.Mutation{m})
	}

	for {
		tx, err := spanner.NewReadWriteStmtBasedTransaction(ctx, client)
		if err != nil {
			// TODO: Handle error.
			break
		}
		err = f(tx)
		if err != nil && status.Code(err) != codes.Aborted {
			tx.Rollback(ctx)
			// TODO: Handle error.
			break
		} else if err == nil {
			_, err = tx.Commit(ctx)
			if err == nil {
				break
			} else if status.Code(err) != codes.Aborted {
				// TODO: Handle error.
				break
			}
		}
		// Set a default sleep time if the server delay is absent.
		delay := 10 * time.Millisecond
		if serverDelay, hasServerDelay := spanner.ExtractRetryDelay(err); hasServerDelay {
			delay = serverDelay
		}
		time.Sleep(delay)
	}
}

// [END spanner_generated_spanner_NewReadWriteStmtBasedTransaction]
