// Copyright 2014 Google Inc. All Rights Reserved.
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

package datastore

import (
	"errors"

	pb "google.golang.org/cloud/internal/datastore"
)

// ErrConcurrentTransaction is returned when a transaction is rolled back due
// to a conflict with a concurrent transaction.
var ErrConcurrentTransaction = errors.New("datastore: concurrent transaction")

func runOnce(c *Client, f func(*Client) error) error {
	// Begin the transaction.
	t := &Client{projectId: c.projectId, BasePath: c.BasePath, c: c.c, namespace: c.namespace}
	req := &pb.BeginTransactionRequest{}
	resp := &pb.BeginTransactionResponse{}

	if err := t.call("BeginTransaction", req, resp); err != nil {
		return err
	}

	t.transaction = resp.Transaction

	// Call f, rolling back the transaction if f returns a non-nil error, or panics.
	// The panic is not recovered.
	defer func() {
		if t.transactionFinished {
			return
		}
		t.transactionFinished = true
		// Ignore the error return value, since we are already returning a non-nil
		// error (or we're panicking).
		c.call("Rollback", &pb.RollbackRequest{Transaction: t.transaction}, &pb.RollbackResponse{})
	}()
	if err := f(t); err != nil {
		return err
	}
	t.transactionFinished = true

	// Commit the transaction.
	err := c.call("Commit", &pb.CommitRequest{Transaction: t.transaction}, &pb.CommitResponse{})
	return err
}

// RunInTransaction runs f in a transaction. It calls f with a transaction
// context tc that f should use for all App Engine operations.
//
// If f returns nil, RunInTransaction attempts to commit the transaction,
// returning nil if it succeeds. If the commit fails due to a conflicting
// transaction, RunInTransaction retries f, each time with a new transaction
// context. It gives up and returns ErrConcurrentTransaction after three
// failed attempts.
//
// If f returns non-nil, then any datastore changes will not be applied and
// RunInTransaction returns that same error. The function f is not retried.
//
// Note that when f returns, the transaction is not yet committed. Calling code
// must be careful not to assume that any of f's changes have been committed
// until RunInTransaction returns nil.
//
// Nested transactions are not supported; c may not be a transaction context.
func RunInTransaction(c *Client, f func(tc *Client) error) error {
	if c.transaction != nil {
		return errors.New("datastore: nested transactions are not supported")
	}
	for i := 0; i < 3; i++ {
		if err := runOnce(c, f); err != ErrConcurrentTransaction {
			return err
		}
	}
	return ErrConcurrentTransaction
}
