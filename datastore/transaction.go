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
	"reflect"

	"code.google.com/p/goprotobuf/proto"

	pb "google.golang.org/cloud/internal/datastore"
)

var typeOfCommitRequest = reflect.TypeOf(pb.CommitRequest{})

// ErrConcurrentTransaction is returned when a transaction is rolled back due
// to a conflict with a concurrent transaction.
var ErrConcurrentTransaction = errors.New("datastore: concurrent transaction")

type transaction struct {
	Client
	transaction []byte
	finished    bool
}

func (t *transaction) Call(method string, in, out proto.Message) error {
	if t.finished {
		return errors.New("datastore: transaction context has expired")
	}

	v := reflect.ValueOf(in)
	rf := v.FieldByName("ReadOptions")
	if rf.IsValid() {
		readOpts, ok := rf.Interface().(*pb.ReadOptions)
		if ok {
			if readOpts == nil {
				if rf.CanSet() {
					rf.Set(reflect.ValueOf(&pb.ReadOptions{Transaction: t.transaction}))
				}
			} else {
				readOpts.Transaction = t.transaction
			}
		}
	}

	if v.Type() == typeOfCommitRequest {
		commitReq := in.(*pb.CommitRequest)
		commitReq.Transaction = t.transaction
		commitReq.Mode = pb.CommitRequest_TRANSACTIONAL.Enum()
	}

	return t.Client.Call(method, in, out)
}

func runOnce(c Client, f func(Client) error, opts *TransactionOptions) error {
	// Begin the transaction.
	t := &transaction{Client: c}
	req := &pb.BeginTransactionRequest{}
	resp := &pb.BeginTransactionResponse{}
	if opts != nil && opts.XG {
		//TODO(PSG-Luna): Support XG queries
		return errors.New("XG queries are not supported yet")
	}
	if err := t.Client.Call("BeginTransaction", req, resp); err != nil {
		return err
	}

	t.transaction = resp.GetTransaction()

	// Call f, rolling back the transaction if f returns a non-nil error, or panics.
	// The panic is not recovered.
	defer func() {
		if t.finished {
			return
		}
		t.finished = true
		// Ignore the error return value, since we are already returning a non-nil
		// error (or we're panicking).
		c.Call("Rollback", &pb.RollbackRequest{Transaction: t.transaction}, &pb.RollbackResponse{})
	}()
	if err := f(t); err != nil {
		return err
	}
	t.finished = true

	// Commit the transaction.
	err := c.Call("Commit", &pb.CommitRequest{Transaction: t.transaction}, &pb.CommitResponse{})
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
func RunInTransaction(c Client, f func(tc Client) error, opts *TransactionOptions) error {
	if _, ok := c.(*transaction); ok {
		return errors.New("datastore: nested transactions are not supported")
	}
	for i := 0; i < 3; i++ {
		if err := runOnce(c, f, opts); err != ErrConcurrentTransaction {
			return err
		}
	}
	return ErrConcurrentTransaction
}

// TransactionOptions are the options for running a transaction.
type TransactionOptions struct {
	// XG is whether the transaction can cross multiple entity groups. In
	// comparison, a single group transaction is one where all datastore keys
	// used have the same root key. Note that cross group transactions do not
	// have the same behavior as single group transactions. In particular, it
	// is much more likely to see partially applied transactions in different
	// entity groups, in global queries.
	// It is valid to set XG to true even if the transaction is within a
	// single entity group.
	XG bool
}
