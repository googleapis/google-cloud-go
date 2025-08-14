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
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	gstatus "google.golang.org/grpc/status"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
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
	expectedReqs := []interface{}{&sppb.BatchCreateSessionsRequest{}}
	if isMultiplexEnabled {
		expectedReqs = []interface{}{&sppb.CreateSessionRequest{}}
	}
	if _, err := shouldHaveReceived(server.TestSpanner, expectedReqs); err != nil {
		t.Fatal(err)
	}
}

// Re-using ReadOnlyTransaction: can recover from acquire failure.
func TestReadOnlyTransaction_RecoverFromFailure(t *testing.T) {

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
	requests := drainRequestsFromServer(server.TestSpanner)
	expectedReqs := []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.CommitRequest{},
	}
	if isMultiplexEnabled {
		expectedReqs = []interface{}{
			&sppb.CreateSessionRequest{},
			&sppb.CommitRequest{},
		}
	}
	if err := compareRequests(expectedReqs, requests); err != nil {
		t.Fatal(err)
	}
	for _, s := range requests {
		switch req := s.(type) {
		case *sppb.CommitRequest:
			// Validate the session is multiplexed
			if !testEqual(isMultiplexEnabled, strings.Contains(req.Session, "multiplexed")) {
				t.Errorf("TestApply_Single expected multiplexed session to be used, got: %v", req.Session)
			}
		}
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
		&sppb.BatchCreateSessionsRequest{}}, requests); err != nil {
		// TODO: remove this once the session pool maintainer has been changed
		// so that is doesn't delete sessions already during the first
		// maintenance window.
		// If we failed to get 3, it might have because - due to timing - we got
		// a fourth request. If this request is DeleteSession, that's OK and
		// expected.
		if err := compareRequests([]interface{}{
			&sppb.BatchCreateSessionsRequest{},
			&sppb.RollbackRequest{}}, requests); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClient_ReadWriteTransaction_UnimplementedErrorWithMultiplexedSessionSwitchesToRegular(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:                     1,
			MaxOpened:                     1,
			enableMultiplexSession:        true,
			enableMultiplexedSessionForRW: true,
		},
	})
	defer teardown()

	for _, sumulatdError := range []error{
		status.Error(codes.Unimplemented, "other Unimplemented error which should not turn off multiplexed session"),
		status.Error(codes.Unimplemented, "Transaction type read_write not supported with multiplexed sessions")} {
		server.TestSpanner.PutExecutionTime(
			MethodExecuteStreamingSql,
			SimulatedExecutionTime{Errors: []error{sumulatdError}},
		)
		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
			defer iter.Stop()
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
		requests := drainRequestsFromServer(server.TestSpanner)
		foundMultiplexedSession := false
		for _, req := range requests {
			if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
				if strings.Contains(sqlReq.Session, "multiplexed") {
					foundMultiplexedSession = true
					break
				}
			}
		}

		// Assert that the error is an Unimplemented error.
		if status.Code(err) != codes.Unimplemented {
			t.Fatalf("Expected Unimplemented error, got: %v", err)
		}
		if !foundMultiplexedSession {
			t.Fatalf("Expected first transaction to use a multiplexed session, but it did not")
		}
		server.TestSpanner.Reset()
	}

	// Attempt a second read-write transaction.
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, NewStatement(SelectFooFromBar))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Unexpected error in second transaction: %v", err)
	}

	// Check that the second transaction used a regular session.
	requests := drainRequestsFromServer(server.TestSpanner)
	foundMultiplexedSession := false
	for _, req := range requests {
		if sqlReq, ok := req.(*sppb.CommitRequest); ok {
			if strings.Contains(sqlReq.Session, "multiplexed") {
				foundMultiplexedSession = true
				break
			}
		}
	}

	if foundMultiplexedSession {
		t.Fatalf("Expected second transaction to use a regular session, but it did not")
	}
}

func TestReadWriteTransaction_PrecommitToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:                     1,
			MaxOpened:                     1,
			enableMultiplexSession:        true,
			enableMultiplexedSessionForRW: true,
		},
	})
	defer teardown()

	type testCase struct {
		name                   string
		query                  bool
		update                 bool
		batchUpdate            bool
		mutationsOnly          bool
		expectedPrecommitToken string
		expectedSequenceNumber int32
	}

	testCases := []testCase{
		{"Only Query", true, false, false, false, "PartialResultSetPrecommitToken", 3},
		{"Query and Update", true, true, false, false, "ResultSetPrecommitToken", 4},
		{"Query, Update, and Batch Update", true, true, true, false, "ExecuteBatchDmlResponsePrecommitToken", 5},
		{"Only Mutations", false, false, false, true, "TransactionPrecommitToken", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				if tc.mutationsOnly {
					ms := []*Mutation{
						Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}),
						Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}),
					}
					if err := tx.BufferWrite(ms); err != nil {
						return err
					}
				}

				if tc.query {
					iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
					defer iter.Stop()
					for {
						_, err := iter.Next()
						if err == iterator.Done {
							break
						}
						if err != nil {
							return err
						}
					}
				}

				if tc.update {
					if _, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
						return err
					}
				}

				if tc.batchUpdate {
					if _, err := tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}}); err != nil {
						return err
					}
				}

				return nil
			})
			if err != nil {
				t.Fatalf("%s failed: %v", tc.name, err)
			}

			requests := drainRequestsFromServer(server.TestSpanner)
			var commitReq *sppb.CommitRequest
			for _, req := range requests {
				if c, ok := req.(*sppb.CommitRequest); ok {
					commitReq = c
				}
				if b, ok := req.(*sppb.BeginTransactionRequest); ok {
					if !strings.Contains(b.GetSession(), "multiplexed") {
						t.Errorf("Expected session to be multiplexed")
					}
					if b.MutationKey == nil {
						t.Fatalf("Expected BeginTransaction request to contain a mutation key")
					}
				}

			}
			if !strings.Contains(commitReq.GetSession(), "multiplexed") {
				t.Errorf("Expected session to be multiplexed")
			}
			if commitReq.PrecommitToken == nil || len(commitReq.PrecommitToken.GetPrecommitToken()) == 0 {
				t.Fatalf("Expected commit request to contain a valid precommitToken, got: %v", commitReq.PrecommitToken)
			}
			// Validate that the precommit token contains the test argument.
			if !strings.Contains(string(commitReq.PrecommitToken.GetPrecommitToken()), tc.expectedPrecommitToken) {
				t.Fatalf("Precommit token does not contain the expected test argument")
			}
			// Validate that the sequence number is as expected.
			if got, want := commitReq.PrecommitToken.GetSeqNum(), tc.expectedSequenceNumber; got != want {
				t.Fatalf("Precommit token sequence number mismatch: got %d, want %d", got, want)
			}
		})
	}
}

func TestCommitWithMultiplexedSessionRetry(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:                     1,
			MaxOpened:                     1,
			enableMultiplexSession:        true,
			enableMultiplexedSessionForRW: true,
		},
	})
	defer teardown()

	// newCommitResponseWithPrecommitToken creates a simulated response with a PrecommitToken
	newCommitResponseWithPrecommitToken := func() *sppb.CommitResponse {
		precommitToken := &sppb.MultiplexedSessionPrecommitToken{
			PrecommitToken: []byte("commit-retry-precommit-token"),
			SeqNum:         4,
		}

		// Create a CommitResponse with the PrecommitToken
		return &sppb.CommitResponse{
			MultiplexedSessionRetry: &sppb.CommitResponse_PrecommitToken{PrecommitToken: precommitToken},
		}
	}

	// Simulate a commit response with a MultiplexedSessionRetry
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Responses: []interface{}{newCommitResponseWithPrecommitToken()},
		})

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		ms := []*Mutation{
			Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}),
			Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}),
		}
		if err := tx.BufferWrite(ms); err != nil {
			return err
		}

		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify that the commit was retried
	requests := drainRequestsFromServer(server.TestSpanner)
	commitCount := 0
	for _, req := range requests {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			if !strings.Contains(commitReq.GetSession(), "multiplexed") {
				t.Errorf("Expected session to be multiplexed")
			}
			commitCount++
			if commitCount == 1 {
				// Validate that the first commit had mutations set
				if len(commitReq.Mutations) == 0 {
					t.Fatalf("Expected first commit to have mutations set")
				}
				if commitReq.PrecommitToken == nil || !strings.Contains(string(commitReq.PrecommitToken.PrecommitToken), "ResultSetPrecommitToken") {
					t.Fatalf("Expected first commit to have precommit token 'ResultSetPrecommitToken', got: %v", commitReq.PrecommitToken)
				}
			} else if commitCount == 2 {
				// Validate that the second commit attempt had mutations un-set
				if len(commitReq.Mutations) != 0 {
					t.Fatalf("Expected second commit to have no mutations set")
				}
				// Validate that the second commit had the precommit token set
				if commitReq.PrecommitToken == nil || string(commitReq.PrecommitToken.PrecommitToken) != "commit-retry-precommit-token" {
					t.Fatalf("Expected second commit to have precommit token 'commit-retry-precommit-token', got: %v", commitReq.PrecommitToken)
				}
			}
		}
	}
	if commitCount != 2 {
		t.Fatalf("Expected 2 commit attempts, got %d", commitCount)
	}
}

func TestClient_ReadWriteTransaction_PreviousTransactionID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := SessionPoolConfig{
		MinOpened:                     1,
		MaxOpened:                     1,
		enableMultiplexSession:        true,
		enableMultiplexedSessionForRW: true,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig:    cfg,
	})
	defer teardown()

	// First ExecuteSql will fail with InvalidArgument to force explicit BeginTransaction
	invalidSQL := "Update FOO Set BAR=1"
	server.TestSpanner.PutStatementResult(
		invalidSQL,
		&StatementResult{
			Type: StatementResultError,
			Err:  status.Error(codes.InvalidArgument, "Invalid update"),
		},
	)

	// First Commit will fail with Aborted to force transaction retry
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})

	var attempts int
	expectedAttempts := 4
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		if attempts == 1 || attempts == 3 {
			// Replace the aborted result with a real result to prevent the
			// transaction from aborting indefinitely.
			server.TestSpanner.PutStatementResult(
				invalidSQL,
				&StatementResult{
					Type:        StatementResultUpdateCount,
					UpdateCount: 3,
				},
			)
		}
		if attempts == 2 {
			server.TestSpanner.PutStatementResult(
				invalidSQL,
				&StatementResult{
					Type: StatementResultError,
					Err:  status.Error(codes.InvalidArgument, "Invalid update"),
				},
			)
		}
		attempts++
		// First attempt: Inline begin fails due to invalid update
		// Second attempt: Explicit begin succeeds but commit aborts
		// Third attempt: Inline begin fails, previousTx set to txn2
		// Fourth attempt: Explicit begin succeeds with previousTx=txn2
		_, err := tx.Update(ctx, NewStatement(invalidSQL))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != expectedAttempts {
		t.Fatalf("got %d attempts, want %d", attempts, expectedAttempts)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	var txID2 []byte
	var foundPrevTxID bool

	// Verify the sequence of requests and transaction IDs
	for i, req := range requests {
		switch r := req.(type) {
		case *sppb.ExecuteSqlRequest:
			// First and third attempts use inline begin
			if i == 1 || i == 5 {
				if _, ok := r.Transaction.GetSelector().(*sppb.TransactionSelector_Begin); !ok {
					t.Errorf("Request %d: got %T, want Begin", i, r.Transaction.GetSelector())
				}
			}
			if txID2 == nil && r.Transaction.GetId() != nil {
				txID2 = r.Transaction.GetId()
			}
		case *sppb.BeginTransactionRequest:
			if i == 7 {
				opts := r.Options.GetReadWrite()
				if opts == nil {
					t.Fatal("Request 7: missing ReadWrite options")
				}
				if !testEqual(opts.MultiplexedSessionPreviousTransactionId, txID2) {
					t.Errorf("Request 7: got prev txID %v, want %v",
						opts.MultiplexedSessionPreviousTransactionId, txID2)
				}
				foundPrevTxID = true
			}
		}
	}

	if !foundPrevTxID {
		t.Error("Did not find BeginTransaction request with previous transaction ID")
	}

	// Verify the complete sequence of requests
	wantRequests := []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},       // Attempt 1: Inline begin fails
		&sppb.BeginTransactionRequest{}, // Attempt 2: Explicit begin
		&sppb.ExecuteSqlRequest{},       // Attempt 2: Update succeeds
		&sppb.CommitRequest{},           // Attempt 2: Commit aborts
		&sppb.ExecuteSqlRequest{},       // Attempt 3: Inline begin fails
		&sppb.BeginTransactionRequest{}, // Attempt 4: Explicit begin with prev txID
		&sppb.ExecuteSqlRequest{},       // Attempt 4: Update succeeds
		&sppb.CommitRequest{},           // Attempt 4: Commit succeeds
	}
	if err := compareRequestsWithConfig(wantRequests, requests, &cfg); err != nil {
		t.Fatal(err)
	}
}

func TestMutationOnlyCaseAborted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Define mutations to apply
	mutations := []*Mutation{
		Insert("FOO", []string{"ID", "NAME"}, []interface{}{int64(1), "Bar"}),
	}

	// Define a function to verify requests
	verifyRequests := func(server *MockedSpannerInMemTestServer) {
		var numBeginReq, numCommitReq int
		// Verify that for mutation-only case, a mutation key is set in BeginTransactionRequest
		requests := drainRequestsFromServer(server.TestSpanner)
		for _, req := range requests {
			if beginReq, ok := req.(*sppb.BeginTransactionRequest); ok {
				if beginReq.GetMutationKey() == nil {
					t.Fatalf("Expected mutation key with insert operation")
				}
				if !strings.Contains(beginReq.GetSession(), "multiplexed") {
					t.Errorf("Expected session to be multiplexed")
				}
				numBeginReq++
			}
			if commitReq, ok := req.(*sppb.CommitRequest); ok {
				if commitReq.GetPrecommitToken() == nil || !strings.Contains(string(commitReq.GetPrecommitToken().PrecommitToken), "TransactionPrecommitToken") {
					t.Errorf("Expected precommit token 'TransactionPrecommitToken', got %v", commitReq.GetPrecommitToken())
				}
				if !strings.Contains(commitReq.GetSession(), "multiplexed") {
					t.Errorf("Expected session to be multiplexed")
				}
				numCommitReq++
			}
		}
		if numBeginReq != 2 || numCommitReq != 2 {
			t.Fatalf("Expected 2 BeginTransactionRequests and 2 CommitRequests, got %d and %d", numBeginReq, numCommitReq)
		}
	}

	// Test both ReadWriteTransaction and client.Apply
	for _, method := range []string{"ReadWriteTransaction", "Apply"} {
		t.Run(method, func(t *testing.T) {
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
				DisableNativeMetrics: true,
				SessionPoolConfig: SessionPoolConfig{
					MinOpened:                     1,
					MaxOpened:                     1,
					enableMultiplexSession:        true,
					enableMultiplexedSessionForRW: true,
				},
			})
			defer teardown()

			// Simulate an aborted transaction on the first commit attempt
			server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
				SimulatedExecutionTime{
					Errors: []error{status.Errorf(codes.Aborted, "Transaction aborted")},
				})
			switch method {
			case "ReadWriteTransaction":
				_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					if err := tx.BufferWrite(mutations); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					t.Fatalf("ReadWriteTransaction failed: %v", err)
				}
			case "Apply":
				_, err := client.Apply(ctx, mutations)
				if err != nil {
					t.Fatalf("Apply failed: %v", err)
				}
			}

			// Verify requests for the current method
			verifyRequests(server)
		})
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
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if got, want := gotReqs[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
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

func TestReadOnlyTransaction_BeginTransactionOption(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	ctx := context.Background()
	for _, beginTransactionOption := range []BeginTransactionOption{DefaultBeginTransaction, ExplicitBeginTransaction, InlinedBeginTransaction} {
		tx := client.ReadOnlyTransaction().WithBeginTransactionOption(beginTransactionOption)
		for range 2 {
			iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
			if err := consumeIterator(iter); err != nil {
				t.Fatalf("failed to execute query: %v", err)
			}
		}
		tx.Close()

		requests := drainRequestsFromServer(server.TestSpanner)
		if beginTransactionOption == InlinedBeginTransaction {
			if err := compareRequests([]interface{}{
				&sppb.ExecuteSqlRequest{},
				&sppb.ExecuteSqlRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := compareRequests([]interface{}{
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.ExecuteSqlRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReadOnlyTransaction_ConcurrentQueries(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	numQueries := 10
	ctx := context.Background()
	for _, beginTransactionOption := range []BeginTransactionOption{DefaultBeginTransaction, ExplicitBeginTransaction, InlinedBeginTransaction} {
		tx := client.ReadOnlyTransaction().WithBeginTransactionOption(beginTransactionOption)
		wg := sync.WaitGroup{}
		errs := make([]error, 0, numQueries)
		mu := sync.Mutex{}
		for range numQueries {
			wg.Add(1)
			go func() {
				defer wg.Done()
				iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
				if err := consumeIterator(iter); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if len(errs) > 0 {
			t.Fatalf("got errors: %v", errs)
		}
		tx.Close()

		requests := drainRequestsFromServer(server.TestSpanner)
		if g, w := countRequests(requests, reflect.TypeOf(&sppb.ExecuteSqlRequest{})), numQueries; g != w {
			t.Fatalf("request count mismatch\n Got: %v\nWant: %v", g, w)
		}
		if beginTransactionOption == InlinedBeginTransaction {
			if g, w := len(requests), numQueries; g != w {
				t.Fatalf("total request count mismatch\n Got: %v\nWant: %v", g, w)
			}
			for n, req := range requests {
				request, ok := req.(*sppb.ExecuteSqlRequest)
				if !ok {
					t.Fatalf("got a non-ExecuteSqlRequest: %T", req)
				}
				begin := request.Transaction.GetBegin()
				if n == 0 && begin == nil {
					t.Fatal("first request does not include a BeginTransaction option")
				} else if n > 0 && begin != nil {
					t.Fatalf("request %d contains a BeginTransaction option: %v", n, begin)
				}
			}
		} else {
			// The transaction used an explicit BeginTransaction RPC.
			if g, w := len(requests), numQueries+1; g != w {
				t.Fatalf("total request count mismatch\n Got: %v\nWant: %v", g, w)
			}
			for n, req := range requests {
				if n == 0 {
					_, ok := req.(*sppb.BeginTransactionRequest)
					if !ok {
						t.Fatalf("got a non-BeginTransactionRequest: %T", req)
					}
				} else {
					request := req.(*sppb.ExecuteSqlRequest)
					begin := request.Transaction.GetBegin()
					if begin != nil {
						t.Fatalf("request %d contains a BeginTransaction option: %v", n, begin)
					}
				}
			}
		}
	}
}

func TestReadWriteTransaction_BeginTransactionOption(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	ctx := context.Background()
	for _, beginTransactionOption := range []BeginTransactionOption{DefaultBeginTransaction, ExplicitBeginTransaction, InlinedBeginTransaction} {
		if _, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			if _, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
				return err
			}
			return nil
		}, TransactionOptions{BeginTransactionOption: beginTransactionOption}); err != nil {
			t.Fatalf("transaction failed: %v", err)
		}
		requests := drainRequestsFromServer(server.TestSpanner)
		if beginTransactionOption == ExplicitBeginTransaction {
			if err := compareRequests([]interface{}{
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.CommitRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := compareRequests([]interface{}{
				&sppb.ExecuteSqlRequest{},
				&sppb.CommitRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReadWriteStmtBasedTransaction(t *testing.T) {
	t.Parallel()

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		rowCount, attempts, err := testReadWriteStmtBasedTransaction(t, beginTransactionOption, make(map[string]SimulatedExecutionTime))
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
}

func TestReadWriteStmtBasedTransaction_CommitAborted(t *testing.T) {
	t.Parallel()

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		rowCount, attempts, err := testReadWriteStmtBasedTransaction(t, beginTransactionOption, map[string]SimulatedExecutionTime{
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
}

func TestReadWriteStmtBasedTransaction_QueryAborted(t *testing.T) {
	t.Parallel()

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		rowCount, attempts, err := testReadWriteStmtBasedTransaction(t, beginTransactionOption, map[string]SimulatedExecutionTime{
			MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
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
}

func TestReadWriteTransaction_QueryFailed(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	// Add a sticky error to the server.
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{KeepError: true, Errors: []error{status.Error(codes.InvalidArgument, "Missing value for query parameter")}})

	ctx := context.Background()
	attempt := 0
	if _, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempt++
		iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
		err := consumeIterator(iter)
		// Queries return the original errors, and not the 'internal' error that triggers a retry of the transaction.
		if g, w := ErrCode(err), codes.InvalidArgument; g != w {
			t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
		}
		_, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if attempt == 1 {
			return err
		}
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	// We expect the following requests:
	// 1. ExecuteSqlRequest with inline-begin. This fails and a retry will be triggered when the update function is
	//    called, but before that request is sent to Spanner
	// 2. BeginTransactionRequest for the retry.
	// 3. ExecuteSqlRequest for the query.
	// 4. ExecuteSqlRequest for the update.
	// 5. CommitRequest.
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
}

func TestReadWriteStmtBasedTransaction_QueryFailed(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	// Add a 'sticky' error to the server
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{KeepError: true, Errors: []error{status.Error(codes.InvalidArgument, "Missing value for query parameter")}})

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		ctx := context.Background()
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(ctx, client, TransactionOptions{BeginTransactionOption: beginTransactionOption})
		if err != nil {
			t.Fatal(err)
		}

		// Execute a query that will return an error as the first statement in the transaction.
		// Queries always return the actual error, but if the transaction used inlined-begin, then the transaction is
		// marked as aborted and any following statement or commit will fail with an Aborted error.
		// If the transaction used explicit begin, then the transaction is not marked as aborted.
		iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
		err = consumeIterator(iter)
		if g, w := ErrCode(err), codes.InvalidArgument; g != w {
			t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
		}
		_, err = tx.Commit(ctx)

		if beginTransactionOption == InlinedBeginTransaction {
			// The transaction used inline-begin, but the first statement failed. The transaction is then marked as
			// aborted by the client library, and that error pops up on the second statement (or commit).
			if g, w := ErrCode(err), codes.Aborted; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			// Reset and retry the transaction.
			tx, err = tx.ResetForRetry(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// The transaction should now use an explicit BeginTransaction and the query should still return the actual error.
			iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
			err := consumeIterator(iter)
			if g, w := ErrCode(err), codes.InvalidArgument; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			// The commit should now succeed, even though there are no successful statements in the transaction.
			if _, err := tx.Commit(ctx); err != nil {
				t.Fatalf("commit failed: %v", err)
			}
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.ExecuteSqlRequest{},
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.CommitRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		} else {
			// The transaction used an explicit BeginTransaction, so the commit should have succeeded the first time.
			if err != nil {
				t.Fatalf("commit failed: %v", err)
			}
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.CommitRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReadWriteStmtBasedTransaction_UpdateAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		server.TestSpanner.PutExecutionTime(
			MethodExecuteSql,
			SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

		ctx := context.Background()
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(ctx, client, TransactionOptions{BeginTransactionOption: beginTransactionOption})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if g, w := ErrCode(err), codes.Aborted; g != w {
			t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
		}
		tx, err = tx.ResetForRetry(ctx)
		if err != nil {
			t.Fatal(err)
		}
		c, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			t.Fatal(err)
		}
		if g, w := c, int64(UpdateBarSetFooRowCount); g != w {
			t.Fatalf("update count mismatch\n Got: %v\nWant: %v", g, w)
		}
		if _, err := tx.Commit(ctx); err != nil {
			t.Fatalf("failed to commit: %v", err)
		}
	}
}

func waitForMinSessions(t *testing.T, client *Client, server *MockedSpannerInMemTestServer) {
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.idleList.Len() < int(sp.MinOpened) {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})
	drainRequestsFromServer(server.TestSpanner)
}

func TestReadWriteStmtBasedTransaction_UpdateFailed(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	// Add a 'sticky' error to the server
	server.TestSpanner.PutExecutionTime(
		MethodExecuteSql,
		SimulatedExecutionTime{KeepError: true, Errors: []error{status.Error(codes.AlreadyExists, "This row already exists")}})

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		ctx := context.Background()
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(ctx, client, TransactionOptions{BeginTransactionOption: beginTransactionOption})
		if err != nil {
			t.Fatal(err)
		}

		// Execute an update statement that will return an error as the first statement in the transaction.
		// The returned error depends on the BeginTransactionOption that has been chosen:
		// 1. ExplicitBegin: The actual error is returned.
		// 2. InlinedBegin: An Aborted error is returned. This indicates that the user should retry the transaction.
		_, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})

		if beginTransactionOption == InlinedBeginTransaction {
			if g, w := ErrCode(err), codes.Aborted; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			// Reset and retry the transaction.
			tx, err = tx.ResetForRetry(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// The transaction should now use an explicit BeginTransaction and the update should now return the actual error.
			_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
			if g, w := ErrCode(err), codes.AlreadyExists; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			tx.Rollback(ctx)
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.ExecuteSqlRequest{},
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.RollbackRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		} else {
			// The transaction used an explicit BeginTransaction, so the update should return the actual error the first time.
			if g, w := ErrCode(err), codes.AlreadyExists; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			tx.Rollback(ctx)
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteSqlRequest{},
				&sppb.RollbackRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReadWriteStmtBasedTransaction_BatchUpdateAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteBatchDml,
		SimulatedExecutionTime{Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}})

	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
	if g, w := ErrCode(err), codes.Aborted; g != w {
		t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
	}
	tx, err = tx.ResetForRetry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := c, []int64{UpdateBarSetFooRowCount}; !reflect.DeepEqual(g, w) {
		t.Fatalf("update count mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestReadWriteStmtBasedTransaction_BatchUpdateFailed(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
	defer teardown()
	// Wait for the session pool to have been initialized, so we don't have to worry about that in the checks below.
	waitForMinSessions(t, client, server)

	// Add a 'sticky' error to the server
	server.TestSpanner.PutExecutionTime(
		MethodExecuteBatchDml,
		SimulatedExecutionTime{KeepError: true, Errors: []error{status.Error(codes.AlreadyExists, "This row already exists")}})

	for _, beginTransactionOption := range []BeginTransactionOption{ExplicitBeginTransaction, InlinedBeginTransaction} {
		ctx := context.Background()
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(ctx, client, TransactionOptions{BeginTransactionOption: beginTransactionOption})
		if err != nil {
			t.Fatal(err)
		}

		// Execute a BatchUpdate that will return an error as the first statement in the transaction.
		// The returned error depends on the BeginTransactionOption that has been chosen:
		// 1. ExplicitBegin: The actual error is returned.
		// 2. InlinedBegin: An Aborted error is returned. This indicates that the user should retry the transaction.
		_, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})

		if beginTransactionOption == InlinedBeginTransaction {
			if g, w := ErrCode(err), codes.Aborted; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			// Reset and retry the transaction.
			tx, err = tx.ResetForRetry(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// The transaction should now use an explicit BeginTransaction and the update should now return the actual error.
			_, err = tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
			if g, w := ErrCode(err), codes.AlreadyExists; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			tx.Rollback(ctx)
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.ExecuteBatchDmlRequest{},
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteBatchDmlRequest{},
				&sppb.RollbackRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		} else {
			// The transaction used an explicit BeginTransaction, so the update should return the actual error the first time.
			if g, w := ErrCode(err), codes.AlreadyExists; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			tx.Rollback(ctx)
			requests := drainRequestsFromServer(server.TestSpanner)
			if err := compareRequests([]interface{}{
				&sppb.BeginTransactionRequest{},
				&sppb.ExecuteBatchDmlRequest{},
				&sppb.RollbackRequest{}}, requests); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func testReadWriteStmtBasedTransaction(t *testing.T, beginTransactionOption BeginTransactionOption, executionTimes map[string]SimulatedExecutionTime) (rowCount int64, attempts int, err error) {
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: SessionPoolConfig{
			// Use a session pool with size 1 to ensure that there are no session leaks.
			MinOpened: 1,
			MaxOpened: 1,
		},
	})
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

	var tx *ReadWriteStmtBasedTransaction
	for {
		attempts++
		if attempts > 1 {
			tx, err = tx.ResetForRetry(ctx)
		} else {
			tx, err = NewReadWriteStmtBasedTransactionWithOptions(ctx, client, TransactionOptions{TransactionTag: "test", BeginTransactionOption: beginTransactionOption})
		}
		if err != nil {
			return 0, attempts, fmt.Errorf("failed to begin a transaction: %v", err)
		}
		if g, w := tx.options.TransactionTag, "test"; g != w {
			t.Errorf("transaction tag mismatch\n Got: %v\nWant: %v", g, w)
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

func TestReadWriteStmtBasedTransactionWithOptions(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
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

	var resp CommitResponse
	for {
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(
			ctx,
			client,
			TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}},
		)
		if err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}

		_, err = f(tx)
		if err != nil && status.Code(err) != codes.Aborted {
			tx.Rollback(ctx)
			break
		} else if err == nil {
			resp, err = tx.CommitWithReturnResp(ctx)
			if err != nil {
				t.Fatalf("failed to CommitWithReturnResp: %v", err)
			}
			break
		}
		// Set a default sleep time if the server delay is absent.
		delay := 10 * time.Millisecond
		if serverDelay, hasServerDelay := ExtractRetryDelay(err); hasServerDelay {
			delay = serverDelay
		}
		time.Sleep(delay)
	}
	if got, want := resp.CommitStats.MutationCount, int64(1); got != want {
		t.Fatalf("Mismatch mutation count - got: %d, want: %d", got, want)
	}
}

// Verify that requests in a ReadWriteStmtBasedTransaction uses multiplexed sessions when enabled
func TestReadWriteStmtBasedTransaction_UsesMultiplexedSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := SessionPoolConfig{
		// Avoid regular session creation
		MinOpened: 0,
		MaxOpened: 0,
		// Enabled multiplexed sessions
		enableMultiplexSession:        true,
		enableMultiplexedSessionForRW: true,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig:    cfg,
		DisableNativeMetrics: true,
	})
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
		&sppb.CreateSessionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	for _, req := range requests {
		if c, ok := req.(*sppb.CommitRequest); ok {
			if !strings.Contains(c.GetSession(), "multiplexed") {
				t.Errorf("Expected session to be multiplexed")
			}
		}
		if b, ok := req.(*sppb.BeginTransactionRequest); ok {
			if !strings.Contains(b.GetSession(), "multiplexed") {
				t.Errorf("Expected session to be multiplexed")
			}
		}
	}
}

// Verify that in ReadWriteStmtBasedTransaction when a transaction using a multiplexed session fails with ABORTED then during retry the previousTransactionID is passed
func TestReadWriteStmtBasedTransaction_UsesPreviousTransactionIDForMultiplexedSession_OnAbort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := SessionPoolConfig{
		// Avoid regular session creation
		MinOpened: 0,
		MaxOpened: 0,
		// Enabled multiplexed sessions
		enableMultiplexSession:        true,
		enableMultiplexedSessionForRW: true,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig:    cfg,
		DisableNativeMetrics: true,
	})
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
	// Fetch the transaction ID of the first attempt
	previousTransactionID := txn.tx
	txn, err = txn.ResetForRetry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = txn.Commit(ctx)
	if err != nil {
		t.Fatalf("expect commit to succeed but failed with error: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	beginTransactionRequestCount := 0
	for _, req := range requests {
		if b, ok := req.(*sppb.BeginTransactionRequest); ok {
			beginTransactionRequestCount++
			// The first BeginTransactionRequest will not have any previousTransactionID.
			// Check if the second BeginTransactionRequest sets the previousTransactionID to correct value.
			if beginTransactionRequestCount == 2 {
				if !strings.Contains(b.GetSession(), "multiplexed") {
					t.Errorf("Expected session to be multiplexed")
				}
				opts := b.Options.GetReadWrite()
				if opts == nil {
					t.Fatal("missing ReadWrite options")
				}
				if !bytes.Equal(opts.MultiplexedSessionPreviousTransactionId, previousTransactionID) {
					t.Errorf("BeginTransactionRequest during retry: got prev txID %v, want %v",
						opts.MultiplexedSessionPreviousTransactionId, previousTransactionID)
				}
			}
		}
	}
}

// Verify that in ReadWriteStmtBasedTransaction, commit request has precommit token set when using multiplexed sessions
func TestReadWriteStmtBasedTransaction_SetsPrecommitToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := SessionPoolConfig{
		// Avoid regular session creation
		MinOpened: 0,
		MaxOpened: 0,
		// Enabled multiplexed sessions
		enableMultiplexSession:        true,
		enableMultiplexedSessionForRW: true,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig:    cfg,
		DisableNativeMetrics: true,
	})
	defer teardown()

	txn, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatalf("got an error: %v", err)
	}
	c, err := txn.Update(ctx, Statement{SQL: UpdateBarSetFoo})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := c, int64(UpdateBarSetFooRowCount); g != w {
		t.Fatalf("update count mismatch\n Got: %v\nWant: %v", g, w)
	}
	_, err = txn.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			if commitReq.GetPrecommitToken() == nil || !strings.Contains(string(commitReq.GetPrecommitToken().PrecommitToken), "ResultSetPrecommitToken") {
				t.Errorf("Expected precommit token 'ResultSetPrecommitToken', got %v", commitReq.GetPrecommitToken())
			}
			if !strings.Contains(commitReq.GetSession(), "multiplexed") {
				t.Errorf("Expected session to be multiplexed")
			}
		}
	}
}

func TestBatchDML_StatementBased_WithMultipleDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

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
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if got, want := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[5+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestPriorityInQueryOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(
		t, ClientConfig{DisableNativeMetrics: true, QueryOptions: QueryOptions{Priority: sppb.RequestOptions_PRIORITY_LOW}},
		[]option.ClientOption{},
	)
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	var iter *RowIterator
	iter = tx.txReadOnly.Query(ctx, NewStatement("SELECT 1"))
	err = iter.Do(func(r *Row) error { return nil })
	if status.Code(err) != codes.Internal {
		t.Fatalf("got unexpected error %v, expected Internal \"No result found for SELECT 1\"", err)
	}
	iter = tx.txReadOnly.QueryWithOptions(ctx, NewStatement("SELECT 1"),
		QueryOptions{Priority: sppb.RequestOptions_PRIORITY_MEDIUM})
	err = iter.Do(func(r *Row) error { return nil })
	if status.Code(err) != codes.Internal {
		t.Fatalf("got unexpected error %v, expected Internal \"No result found for SELECT 1\"", err)
	}
	iter = tx.txReadOnly.QueryWithStats(ctx, NewStatement("SELECT 1"))
	err = iter.Do(func(r *Row) error { return nil })
	if status.Code(err) != codes.Internal {
		t.Fatalf("got unexpected error %v, expected Internal \"No result found for SELECT 1\"", err)
	}
	_, err = tx.txReadOnly.AnalyzeQuery(ctx, NewStatement("SELECT 1"))
	if status.Code(err) != codes.Internal {
		t.Fatalf("got unexpected error %v, expected Internal \"No result found for SELECT 1\"", err)
	}
	if _, err = tx.Update(ctx, Statement{SQL: UpdateBarSetFoo}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = tx.UpdateWithOptions(ctx, Statement{SQL: UpdateBarSetFoo}, QueryOptions{Priority: sppb.RequestOptions_PRIORITY_MEDIUM}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if got, want := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_LOW; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_MEDIUM; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_LOW; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[5+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_LOW; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[6+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_LOW; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[7+muxCreateBuffer].(*sppb.ExecuteSqlRequest).RequestOptions.Priority, sppb.RequestOptions_PRIORITY_MEDIUM; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestLastStatement_Update(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.Update(context.Background(), NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		if _, err := tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), QueryOptions{LastStatement: true}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(executeRequests); i++ {
		if g, w := executeRequests[i].(*sppb.ExecuteSqlRequest).LastStatement, i == len(executeRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func TestLastStatement_Query(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		for i := 0; i < 2; i++ {
			iter := tx.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), QueryOptions{LastStatement: i == 1})
			for {
				_, err := iter.Next()
				if errors.Is(err, iterator.Done) {
					break
				}
				if err != nil {
					return err
				}
			}
			iter.Stop()
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(executeRequests); i++ {
		if g, w := executeRequests[i].(*sppb.ExecuteSqlRequest).LastStatement, i == len(executeRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func TestLastStatement_BatchUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.BatchUpdate(context.Background(), []Statement{NewStatement(UpdateBarSetFoo)}); err != nil {
			return err
		}
		if _, err := tx.BatchUpdateWithOptions(context.Background(), []Statement{NewStatement(UpdateBarSetFoo)}, QueryOptions{LastStatement: true}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	batchRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteBatchDmlRequest{}))
	if g, w := len(batchRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(batchRequests); i++ {
		if g, w := batchRequests[i].(*sppb.ExecuteBatchDmlRequest).LastStatements, i == len(batchRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func TestLastStatement_StmtBasedTransaction_Update(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(context.Background(), client)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if _, err := tx.Update(context.Background(), NewStatement(UpdateBarSetFoo)); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	if _, err := tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), QueryOptions{LastStatement: true}); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(executeRequests); i++ {
		if g, w := executeRequests[i].(*sppb.ExecuteSqlRequest).LastStatement, i == len(executeRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func TestLastStatement_StmtBasedTx_Query(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(context.Background(), client)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	for i := 0; i < 2; i++ {
		iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), QueryOptions{LastStatement: i == 1})
		for {
			_, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				t.Fatalf("failed to query: %v", err)
			}
		}
		iter.Stop()
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(executeRequests); i++ {
		if g, w := executeRequests[i].(*sppb.ExecuteSqlRequest).LastStatement, i == len(executeRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
	}
}

func TestLastStatement_StmtBasedTx_BatchUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true})
	defer teardown()

	tx, err := NewReadWriteStmtBasedTransaction(context.Background(), client)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if _, err := tx.BatchUpdate(context.Background(), []Statement{NewStatement(UpdateBarSetFoo)}); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	if _, err := tx.BatchUpdateWithOptions(context.Background(), []Statement{NewStatement(UpdateBarSetFoo)}, QueryOptions{LastStatement: true}); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	batchRequests := requestsOfType(requests, reflect.TypeOf(&sppb.ExecuteBatchDmlRequest{}))
	if g, w := len(batchRequests), 2; g != w {
		t.Fatalf("num requests mismatch\n Got: %v\nWant: %v", g, w)
	}
	for i := 0; i < len(batchRequests); i++ {
		if g, w := batchRequests[i].(*sppb.ExecuteBatchDmlRequest).LastStatements, i == len(batchRequests)-1; g != w {
			t.Fatalf("%d: last statement mismatch\n Got: %v\nWant: %v", i, g, w)
		}
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
	return compareRequestsWithConfig(want, got, nil)
}

func compareRequestsWithConfig(want []interface{}, got []interface{}, config *SessionPoolConfig) error {
	if reflect.TypeOf(want[0]) != reflect.TypeOf(&sppb.BatchCreateSessionsRequest{}) {
		sessReq := 0
		for i := 0; i < len(want); i++ {
			if reflect.TypeOf(want[i]) == reflect.TypeOf(&sppb.BatchCreateSessionsRequest{}) {
				sessReq = i
				break
			}
		}
		want[0], want[sessReq] = want[sessReq], want[0]
	}
	if isMultiplexEnabled || (config != nil && config.enableMultiplexSession) {
		if reflect.TypeOf(want[0]) != reflect.TypeOf(&sppb.CreateSessionRequest{}) {
			want = append([]interface{}{&sppb.CreateSessionRequest{}}, want...)
		}
		if reflect.TypeOf(got[0]) == reflect.TypeOf(&sppb.BatchCreateSessionsRequest{}) {
			muxSessionIndex := 0
			for i := 0; i < len(got); i++ {
				if reflect.TypeOf(got[i]) == reflect.TypeOf(&sppb.CreateSessionRequest{}) {
					muxSessionIndex = i
					break
				}
			}
			got[0], got[muxSessionIndex] = got[muxSessionIndex], got[0]
		}
	}
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

func requestsOfType(requests []interface{}, t reflect.Type) []interface{} {
	res := make([]interface{}, 0)
	for _, req := range requests {
		if reflect.TypeOf(req) == t {
			res = append(res, req)
		}
	}
	return res
}

func newAbortedErrorWithMinimalRetryDelay() error {
	st := gstatus.New(codes.Aborted, "Transaction has been aborted")
	retry := &errdetails.RetryInfo{
		RetryDelay: durationpb.New(time.Nanosecond),
	}
	st, _ = st.WithDetails(retry)
	return st.Err()
}
