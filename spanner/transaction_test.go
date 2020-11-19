/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	gstatus "google.golang.org/grpc/status"
)

// Single can only be used once.
func TestSingle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn := client.Single()
	defer txn.Close()
	_, _, e := txn.acquire(ctx)
	if e != nil {
		t.Fatalf("Acquire for single use, got %v, want nil.", e)
	}
	_, _, e = txn.acquire(ctx)
	if wantErr := errTxClosed(); !testEqual(e, wantErr) {
		t.Fatalf("Second acquire for single use, got %v, want %v.", e, wantErr)
	}

	// Only one BatchCreateSessionsRequest is sent.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{&sppb.BatchCreateSessionsRequest{}}); err != nil {
		t.Fatal(err)
	}
}

// Re-using ReadOnlyTransaction: can recover from acquire failure.
func TestReadOnlyTransaction_RecoverFromFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	// First request will fail.
	errUsr := gstatus.Error(codes.Unknown, "error")
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction,
		SimulatedExecutionTime{
			Errors: []error{errUsr},
		})

	_, _, e := txn.acquire(ctx)
	if wantErr := ToSpannerError(errUsr); !testEqual(e, wantErr) {
		t.Fatalf("Acquire for multi use, got %v, want %v.", e, wantErr)
	}
	_, _, e = txn.acquire(ctx)
	if e != nil {
		t.Fatalf("Acquire for multi use, got %v, want nil.", e)
	}
}

// ReadOnlyTransaction: can not be used after close.
func TestReadOnlyTransaction_UseAfterClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn := client.ReadOnlyTransaction()
	txn.Close()

	_, _, e := txn.acquire(ctx)
	if wantErr := errTxClosed(); !testEqual(e, wantErr) {
		t.Fatalf("Second acquire for multi use, got %v, want %v.", e, wantErr)
	}
}

// ReadOnlyTransaction: can be acquired concurrently.
func TestReadOnlyTransaction_Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	server.TestSpanner.Freeze()
	var (
		sh1 *sessionHandle
		sh2 *sessionHandle
		ts1 *sppb.TransactionSelector
		ts2 *sppb.TransactionSelector
		wg  = sync.WaitGroup{}
	)
	acquire := func(sh **sessionHandle, ts **sppb.TransactionSelector) {
		defer wg.Done()
		var e error
		*sh, *ts, e = txn.acquire(ctx)
		if e != nil {
			t.Errorf("Concurrent acquire for multiuse, got %v, expect nil.", e)
		}
	}
	wg.Add(2)
	go acquire(&sh1, &ts1)
	go acquire(&sh2, &ts2)

	// TODO(deklerk): Get rid of this.
	<-time.After(100 * time.Millisecond)

	server.TestSpanner.Unfreeze()
	wg.Wait()
	if sh1.session.id != sh2.session.id {
		t.Fatalf("Expected acquire to get same session handle, got %v and %v.", sh1, sh2)
	}
	if !testEqual(ts1, ts2) {
		t.Fatalf("Expected acquire to get same transaction selector, got %v and %v.", ts1, ts2)
	}
}

func TestApply_Single(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, e := client.Apply(ctx, ms, ApplyAtLeastOnce()); e != nil {
		t.Fatalf("applyAtLeastOnce retry on abort, got %v, want nil.", e)
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.CommitRequest{},
	}); err != nil {
		t.Fatal(err)
	}
}

// Transaction retries on abort.
func TestApply_RetryOnAbort(t *testing.T) {
	ctx := context.Background()
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	// First commit will fail, and the retry will begin a new transaction.
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{newAbortedErrorWithMinimalRetryDelay()},
		})

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId"}, []interface{}{int64(1)}),
	}

	if _, e := client.Apply(ctx, ms); e != nil {
		t.Fatalf("ReadWriteTransaction retry on abort, got %v, want nil.", e)
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // First commit fails.
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // Second commit succeeds.
	}); err != nil {
		t.Fatal(err)
	}
}

// Tests that SessionNotFound errors are retried.
func TestTransaction_SessionNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	serverErr := newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction,
		SimulatedExecutionTime{
			Errors: []error{serverErr, serverErr, serverErr},
		})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{serverErr},
		})

	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	var wantErr error
	if _, _, got := txn.acquire(ctx); !testEqual(wantErr, got) {
		t.Fatalf("Expect acquire to succeed, got %v, want %v.", got, wantErr)
	}

	// The server error should lead to a retry of the BeginTransaction call and
	// a valid session handle to be returned that will be used by the following
	// requests. Note that calling txn.Query(...) does not actually send the
	// query to the (mock) server. That is done at the first call to
	// RowIterator.Next. The following statement only verifies that the
	// transaction is in a valid state and received a valid session handle.
	if got := txn.Query(ctx, NewStatement("SELECT 1")); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Query to succeed, got %v, want %v.", got.err, wantErr)
	}

	if got := txn.Read(ctx, "Users", KeySets(Key{"alice"}, Key{"bob"}), []string{"name", "email"}); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Read to succeed, got %v, want %v.", got.err, wantErr)
	}

	wantErr = ToSpannerError(newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"))
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	_, got := client.Apply(ctx, ms, ApplyAtLeastOnce())
	if !testEqual(wantErr, got) {
		t.Fatalf("Expect Apply to fail\nGot:  %v\nWant: %v\n", got, wantErr)
	}
}

// When an error is returned from the closure sent into ReadWriteTransaction, it
// kicks off a rollback.
func TestReadWriteTransaction_ErrorReturned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	want := errors.New("an error")
	_, got := client.ReadWriteTransaction(ctx, func(context.Context, *ReadWriteTransaction) error {
		return want
	})
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.RollbackRequest{}}, requests); err != nil {
		// TODO: remove this once the session pool maintainer has been changed
		// so that is doesn't delete sessions already during the first
		// maintenance window.
		// If we failed to get 3, it might have because - due to timing - we got
		// a fourth request. If this request is DeleteSession, that's OK and
		// expected.
		if err := compareRequests([]interface{}{
			&sppb.BatchCreateSessionsRequest{},
			&sppb.BeginTransactionRequest{},
			&sppb.RollbackRequest{},
			&sppb.DeleteSessionRequest{}}, requests); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBatchDML_WithMultipleDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		if _, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
			return err
		}
		if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}, {SQL: UpdateBarSetFoo}}); err != nil {
			return err
		}
		if _, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
			return err
		}
		_, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := gotReqs[2].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[5].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// When an Aborted error happens during a commit, it does not kick off a
// rollback.
func TestReadWriteStmtBasedTransaction_CommitAbortedErrorReturned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.Aborted, "Transaction aborted")},
		})

	txn, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatalf("got an error: %v", err)
	}
	_, err = txn.Commit(ctx)
	if status.Code(err) != codes.Aborted || !strings.Contains(err.Error(), "Transaction aborted") {
		t.Fatalf("got an incorrect error: %v", err)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		// TODO: remove this once the session pool maintainer has been changed
		// so that is doesn't delete sessions already during the first
		// maintenance window.
		// If we failed to get 4, it might have because - due to timing - we got
		// a fourth request. If this request is DeleteSession, that's OK and
		// expected.
		if err := compareRequests([]interface{}{
			&sppb.BatchCreateSessionsRequest{},
			&sppb.BeginTransactionRequest{},
			&sppb.CommitRequest{},
			&sppb.DeleteSessionRequest{}}, requests); err != nil {
			t.Fatal(err)
		}
	}
}

// When a non-aborted error happens during a commit, it kicks off a rollback.
func TestReadWriteStmtBasedTransaction_CommitNonAbortedErrorReturned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.NotFound, "Session not found")},
		})

	txn, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatalf("got an error: %v", err)
	}
	_, err = txn.Commit(ctx)
	if status.Code(err) != codes.NotFound || !strings.Contains(err.Error(), "Session not found") {
		t.Fatalf("got an incorrect error: %v", err)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.RollbackRequest{}}, requests); err != nil {
		// TODO: remove this once the session pool maintainer has been changed
		// so that is doesn't delete sessions already during the first
		// maintenance window.
		// If we failed to get 4, it might have because - due to timing - we got
		// a fourth request. If this request is DeleteSession, that's OK and
		// expected.
		if err := compareRequests([]interface{}{
			&sppb.BatchCreateSessionsRequest{},
			&sppb.BeginTransactionRequest{},
			&sppb.CommitRequest{},
			&sppb.RollbackRequest{},
			&sppb.DeleteSessionRequest{}}, requests); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadWriteStmtBasedTransaction(t *testing.T) {
	t.Parallel()

	rowCount, attempts, err := testReadWriteStmtBasedTransaction(t, make(map[string]SimulatedExecutionTime))
	if err != nil {
		t.Fatalf("transaction failed to commit: %v", err)
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	if g, w := attempts, 1; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func TestReadWriteStmtBasedTransaction_CommitAborted(t *testing.T) {
	t.Parallel()
	rowCount, attempts, err := testReadWriteStmtBasedTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	})
	if err != nil {
		t.Fatalf("transaction failed to commit: %v", err)
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func testReadWriteStmtBasedTransaction(t *testing.T, executionTimes map[string]SimulatedExecutionTime) (rowCount int64, attempts int, err error) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for method, exec := range executionTimes {
		server.TestSpanner.PutExecutionTime(method, exec)
	}
	ctx := context.Background()

	f := func(tx *ReadWriteStmtBasedTransaction) (int64, error) {
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		rowCount := int64(0)
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return 0, err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return 0, err
			}
			rowCount++
		}
		return rowCount, nil
	}

	for {
		attempts++
		tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
		if err != nil {
			return 0, attempts, fmt.Errorf("failed to begin a transaction: %v", err)
		}
		rowCount, err = f(tx)
		if err != nil && status.Code(err) != codes.Aborted {
			tx.Rollback(ctx)
			return 0, attempts, err
		} else if err == nil {
			_, err = tx.Commit(ctx)
			if err == nil {
				break
			} else if status.Code(err) != codes.Aborted {
				return 0, attempts, err
			}
		}
		// Set a default sleep time if the server delay is absent.
		delay := 10 * time.Millisecond
		if serverDelay, hasServerDelay := ExtractRetryDelay(err); hasServerDelay {
			delay = serverDelay
		}
		time.Sleep(delay)
	}

	return rowCount, attempts, err
}

func TestBatchDML_StatementBased_WithMultipleDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if _, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}, {SQL: UpdateBarSetFoo}}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	_, err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := gotReqs[2].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[5].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// shouldHaveReceived asserts that exactly expectedRequests were present in
// the server's ReceivedRequests channel. It only looks at type, not contents.
//
// Note: this in-place modifies serverClientMock by popping items off the
// ReceivedRequests channel.
func shouldHaveReceived(server InMemSpannerServer, want []interface{}) ([]interface{}, error) {
	got := drainRequestsFromServer(server)
	return got, compareRequests(want, got)
}

// Compares expected requests (want) with actual requests (got).
func compareRequests(want []interface{}, got []interface{}) error {
	if len(got) != len(want) {
		var gotMsg string
		for _, r := range got {
			gotMsg += fmt.Sprintf("%v: %+v]\n", reflect.TypeOf(r), r)
		}

		var wantMsg string
		for _, r := range want {
			wantMsg += fmt.Sprintf("%v: %+v]\n", reflect.TypeOf(r), r)
		}

		return fmt.Errorf("got %d requests, want %d requests:\ngot:\n%s\nwant:\n%s", len(got), len(want), gotMsg, wantMsg)
	}

	for i, want := range want {
		if reflect.TypeOf(got[i]) != reflect.TypeOf(want) {
			return fmt.Errorf("request %d: got %+v, want %+v", i, reflect.TypeOf(got[i]), reflect.TypeOf(want))
		}
	}
	return nil
}

func drainRequestsFromServer(server InMemSpannerServer) []interface{} {
	var reqs []interface{}
loop:
	for {
		select {
		case req := <-server.ReceivedRequests():
			reqs = append(reqs, req)
		default:
			break loop
		}
	}
	return reqs
}

func newAbortedErrorWithMinimalRetryDelay() error {
	st := gstatus.New(codes.Aborted, "Transaction has been aborted")
	retry := &errdetails.RetryInfo{
		RetryDelay: ptypes.DurationProto(time.Nanosecond),
	}
	st, _ = st.WithDetails(retry)
	return st.Err()
}
