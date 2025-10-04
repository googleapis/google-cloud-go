// Copyright 2017 Google LLC
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

package firestore

import (
	"context"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRunTransaction(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}

	beginReq := &pb.BeginTransactionRequest{Database: db}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}
	commitReq := &pb.CommitRequest{Database: db, Transaction: tid}
	// Empty transaction.
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp})
	err := c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	// Transaction with read and write.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:            c.path(),
			Documents:           []string{db + "/documents/C/a"},
			ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{Transaction: tid},
		}, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: aDoc},
				ReadTime: aTimestamp2,
			},
		})
	aDoc2 := &pb.Document{
		Name:   aDoc.Name,
		Fields: map[string]*pb.Value{"count": intval(2)},
	}
	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{Update: aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{Exists: true},
				},
			}},
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)
	var commitResponse CommitResponse
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		doc, err := tx.Get(docref)
		if err != nil {
			return err
		}
		count, err := doc.DataAt("count")
		if err != nil {
			return err
		}
		return tx.Update(docref, []Update{{Path: "count", Value: count.(int64) + 1}})
	}, WithCommitResponseTo(&commitResponse))
	if err != nil {
		t.Fatal(err)
	}

	// validate commit time
	commitTime := commitResponse.CommitTime()
	if commitTime != aTimestamp3.AsTime() {
		t.Fatalf("commit time %v should equal %v", commitTime, aTimestamp3)
	}

	// Query
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(
		&pb.RunQueryRequest{
			Parent: db + "/documents",
			QueryType: &pb.RunQueryRequest_StructuredQuery{
				StructuredQuery: &pb.StructuredQuery{
					From: []*pb.StructuredQuery_CollectionSelector{{CollectionId: "C"}},
				},
			},
			ConsistencySelector: &pb.RunQueryRequest_Transaction{Transaction: tid},
		},
		[]interface{}{},
	)
	srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp3})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		it := tx.Documents(c.Collection("C"))
		defer it.Stop()
		_, err := it.Next()
		if err != iterator.Done {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Retry entire transaction.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(commitReq, status.Errorf(codes.Aborted, ""))
	rollbackReq := &pb.RollbackRequest{Database: db, Transaction: tid}
	srv.addRPC(rollbackReq, &emptypb.Empty{})
	tid2 := []byte{2} // Use a new transaction ID for the response to the retry BeginTransaction
	beginReqRetry := &pb.BeginTransactionRequest{
		Database: db,
		Options: &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadWrite_{
				ReadWrite: &pb.TransactionOptions_ReadWrite{RetryTransaction: tid}, // Retries with the previous transaction's ID
			},
		},
	}
	beginRes2 := &pb.BeginTransactionResponse{Transaction: tid2} // New transaction ID for the successful attempt
	srv.addRPC(beginReqRetry, beginRes2)

	// Attempt 2: Commit succeeds with the new transaction ID (tid2).
	commitReq2 := &pb.CommitRequest{Database: db, Transaction: tid2}
	srv.addRPC(commitReq2, &pb.CommitResponse{CommitTime: aTimestamp})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionErrors(t *testing.T) {
	ctx := context.Background()
	const db = "projects/projectID/databases/(default)"
	c, srv, cleanup := newMock(t)
	defer cleanup()

	var (
		tid        = []byte{1}
		unknownErr = status.Errorf(codes.Unknown, "so sad")
		abortedErr = status.Errorf(codes.Aborted, "not so sad.retryable")

		beginRetryReq = &pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					ReadWrite: &pb.TransactionOptions_ReadWrite{RetryTransaction: tid},
				},
			},
		}
		beginReq = &pb.BeginTransactionRequest{
			Database: db,
		}
		beginRes = &pb.BeginTransactionResponse{Transaction: tid}
		get      = func(_ context.Context, tx *Transaction) error {
			_, err := tx.Get(c.Doc("C/a"))
			return err
		}
		getReq = &pb.BatchGetDocumentsRequest{
			Database:            c.path(),
			Documents:           []string{db + "/documents/C/a"},
			ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{Transaction: tid},
		}
		getRes = []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result: &pb.BatchGetDocumentsResponse_Found{Found: &pb.Document{
					Name:       "projects/projectID/databases/(default)/documents/C/a",
					CreateTime: aTimestamp,
					UpdateTime: aTimestamp2,
				}},
				ReadTime: aTimestamp2,
			},
		}
		rollbackReq = &pb.RollbackRequest{Database: db, Transaction: tid}
		commitReq   = &pb.CommitRequest{Database: db, Transaction: tid}
		commitRes   = &pb.CommitResponse{CommitTime: aTimestamp}
	)

	t.Run("BeginTransaction has a permanent error", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, unknownErr)
		err := c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
		if status.Code(err) != codes.Unknown {
			t.Errorf("got <%v>, want Unknown", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Get has a permanent error", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(getReq, unknownErr)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, get)
		if status.Code(err) != codes.Unknown {
			t.Errorf("got <%v>, want Unknown", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Get has a permanent error, but the rollback fails", func(t *testing.T) {
		// We still return Get's error.
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(getReq, unknownErr)
		srv.addRPC(rollbackReq, status.Errorf(codes.FailedPrecondition, ""))
		err := c.RunTransaction(ctx, get)
		if status.Code(err) != codes.Unknown {
			t.Errorf("got <%v>, want Unknown", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Commit has a permanent error", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(getReq, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result: &pb.BatchGetDocumentsResponse_Found{Found: &pb.Document{
					Name:       "projects/projectID/databases/(default)/documents/C/a",
					CreateTime: aTimestamp,
					UpdateTime: aTimestamp2,
				}},
				ReadTime: aTimestamp2,
			},
		})
		srv.addRPC(commitReq, unknownErr)
		srv.addRPC(rollbackReq, &emptypb.Empty{})

		err := c.RunTransaction(ctx, get)
		if status.Code(err) != codes.Unknown {
			t.Errorf("got <%v>, want Unknown", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Read after write", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
			if err := tx.Delete(c.Doc("C/a")); err != nil {
				return err
			}
			if _, err := tx.Get(c.Doc("C/a")); err != nil {
				return err
			}
			return nil
		})
		if err != errReadAfterWrite {
			t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Read after write, with query", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
			if err := tx.Delete(c.Doc("C/a")); err != nil {
				return err
			}
			it := tx.Documents(c.Collection("C").Select("x"))
			defer it.Stop()
			if _, err := it.Next(); err != iterator.Done {
				return err
			}
			return nil
		})
		if err != errReadAfterWrite {
			t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Read after write, with query and GetAll", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
			if err := tx.Delete(c.Doc("C/a")); err != nil {
				return err
			}
			_, err := tx.Documents(c.Collection("C").Select("x")).GetAll()
			return err
		})
		if err != errReadAfterWrite {
			t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Read after write fails even if the user ignores the read's error", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
			if err := tx.Delete(c.Doc("C/a")); err != nil {
				return err
			}
			if _, err := tx.Get(c.Doc("C/a")); err != nil {
				return err
			}
			return nil
		})
		if err != errReadAfterWrite {
			t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Write in read-only transaction", func(t *testing.T) {
		srv.reset()
		srv.addRPC(
			&pb.BeginTransactionRequest{
				Database: db,
				Options: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{}},
				},
			},
			beginRes,
		)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
			return tx.Delete(c.Doc("C/a"))
		}, ReadOnly)
		if err != errWriteReadOnly {
			t.Errorf("got <%v>, want <%v>", err, errWriteReadOnly)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Too many retries", func(t *testing.T) {
		// Use tid = 1 for the first attempt.
		// Use tid = 2 for the second attempt.
		tid1 := []byte{1}
		tid2 := []byte{2}
		beginRes2 := &pb.BeginTransactionResponse{Transaction: tid2}

		srv.reset()

		// Attempt 1 (Fails)
		srv.addRPC(beginReq, beginRes)                          // 1. BeginTransaction (tid1)
		srv.addRPC(commitReq, status.Errorf(codes.Aborted, "")) // 2. Commit (tid1) fails (Aborted)
		srv.addRPC(rollbackReq, &emptypb.Empty{})               // 3. Rollback (tid1)

		// Attempt 2 (Fails)
		beginReqRetry := &pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					ReadWrite: &pb.TransactionOptions_ReadWrite{RetryTransaction: tid1}, // Retries with previous ID (tid1)
				},
			},
		}
		// The retry BeginTransaction should return a new ID (tid2)
		srv.addRPC(beginReqRetry, beginRes2) // 4. BeginTransaction (tid2)

		commitReq2 := &pb.CommitRequest{Database: db, Transaction: tid2} // New commit request with tid2
		srv.addRPC(commitReq2, status.Errorf(codes.Aborted, ""))         // 5. Commit (tid2) fails (Aborted)

		// Final Rollback on Aborted error when MaxAttempts is reached
		rollbackReq2 := &pb.RollbackRequest{Database: db, Transaction: tid2}
		srv.addRPC(rollbackReq2, &emptypb.Empty{}) // 6. Rollback (tid2)

		err := c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil },
			MaxAttempts(2))
		if status.Code(err) != codes.Aborted {
			t.Errorf("got <%v>, want Aborted", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Nested transaction", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		err := c.RunTransaction(ctx, func(ctx context.Context, tx *Transaction) error {
			return c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
		})
		if got, want := err, errNestedTransaction; got != want {
			t.Errorf("got <%v>, want <%v>", got, want)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})

	t.Run("Get has a retryable error", func(t *testing.T) {
		srv.reset()
		srv.addRPC(beginReq, beginRes)
		srv.addRPC(getReq, abortedErr)
		srv.addRPC(rollbackReq, &emptypb.Empty{})
		srv.addRPC(beginRetryReq, beginRes)
		srv.addRPC(getReq, getRes)
		srv.addRPC(commitReq, commitRes)
		err := c.RunTransaction(ctx, get)
		if err != nil {
			t.Errorf("got <%v>, want nil", err)
		}
		if !srv.isEmpty() {
			t.Errorf("Expected %+v requests but not received. srv.reqItems: %+v", len(srv.reqItems), srv.reqItems)
		}
	})
}

func TestTransactionGetAll(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const dbPath = "projects/projectID/databases/(default)"
	tid := []byte{1}
	beginReq := &pb.BeginTransactionRequest{Database: dbPath}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}
	srv.addRPC(beginReq, beginRes)
	req := &pb.BatchGetDocumentsRequest{
		Database: dbPath,
		Documents: []string{
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/b",
			dbPath + "/documents/C/c",
		},
		ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{Transaction: tid},
	}
	err := c.RunTransaction(context.Background(), func(_ context.Context, tx *Transaction) error {
		testGetAll(t, c, srv, dbPath,
			func(drs []*DocumentRef) ([]*DocumentSnapshot, error) { return tx.GetAll(drs) },
			req)
		commitReq := &pb.CommitRequest{Database: dbPath, Transaction: tid}
		srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Each retry attempt has the same amount of commit writes.
func TestRunTransaction_Retries(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}

	// Attempt 1: Begin
	srv.addRPC(
		&pb.BeginTransactionRequest{Database: db},
		&pb.BeginTransactionResponse{Transaction: tid},
	)

	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	aDoc2 := &pb.Document{
		Name:   aDoc.Name,
		Fields: map[string]*pb.Value{"count": intval(7)},
	}

	// Attempt 1: Commit (Fails)
	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{Update: aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{Exists: true},
				},
			}},
		},
		status.Errorf(codes.Aborted, "something failed! please retry me!"),
	)

	rollbackReq := &pb.RollbackRequest{Database: db, Transaction: tid}
	srv.addRPC(rollbackReq, &emptypb.Empty{})

	// Attempt 2: Begin (Retry)
	srv.addRPC(
		&pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					ReadWrite: &pb.TransactionOptions_ReadWrite{RetryTransaction: tid},
				},
			},
		},
		&pb.BeginTransactionResponse{Transaction: tid},
	)

	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{Update: aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{Exists: true},
				},
			}},
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)

	err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		return tx.Update(docref, []Update{{Path: "count", Value: 7}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Non-transactional operations are allowed in transactions (although
// discouraged).
func TestRunTransaction_NonTransactionalOp(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}

	beginReq := &pb.BeginTransactionRequest{Database: db}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}

	srv.reset()
	srv.addRPC(beginReq, beginRes)
	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:  c.path(),
			Documents: []string{db + "/documents/C/a"},
		}, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: aDoc},
				ReadTime: aTimestamp2,
			},
		})
	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)

	if err := c.RunTransaction(ctx, func(ctx2 context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		if _, err := c.GetAll(ctx2, []*DocumentRef{docref}); err != nil {
			t.Fatal(err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTransaction_WithReadOptions(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const db = "projects/projectID/databases/(default)"
	tm := time.Date(2021, time.February, 20, 0, 0, 0, 0, time.UTC)
	ts := &timestamppb.Timestamp{Nanos: int32(tm.UnixNano())}
	tid := []byte{1}

	beginReq := &pb.BeginTransactionRequest{Database: db}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}

	srv.reset()
	srv.addRPC(beginReq, beginRes)

	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
		},
		&pb.CommitResponse{CommitTime: ts},
	)

	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
		},
		&pb.CommitResponse{CommitTime: ts},
	)

	if err := c.RunTransaction(ctx, func(ctx2 context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		tx.WithReadOptions(ReadTime(tm)).Get(docref)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
