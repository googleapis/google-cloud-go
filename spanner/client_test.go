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
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	itestutil "cloud.google.com/go/internal/testutil"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupMockedTestServer(t *testing.T) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	return setupMockedTestServerWithConfig(t, ClientConfig{})
}

func setupMockedTestServerWithConfig(t *testing.T, config ClientConfig) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	return setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{})
}

func setupMockedTestServerWithConfigAndClientOptions(t *testing.T, config ClientConfig, clientOptions []option.ClientOption) (server *MockedSpannerInMemTestServer, client *Client, teardown func()) {
	grpcHeaderChecker := &itestutil.HeadersEnforcer{
		OnFailure: t.Fatalf,
		Checkers: []*itestutil.HeaderChecker{
			{
				Key: "x-goog-api-client",
				ValuesValidator: func(token ...string) error {
					if len(token) != 1 {
						return spannerErrorf(codes.Internal, "unexpected number of api client token headers: %v", len(token))
					}
					if !strings.HasPrefix(token[0], "gl-go/") {
						return spannerErrorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					return nil
				},
			},
		},
	}
	clientOptions = append(clientOptions, grpcHeaderChecker.CallOptions()...)
	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	opts = append(opts, clientOptions...)
	ctx := context.Background()
	var formattedDatabase = fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	client, err := NewClientWithConfig(ctx, formattedDatabase, config, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return server, client, func() {
		client.Close()
		serverTeardown()
	}
}

// Test validDatabaseName()
func TestValidDatabaseName(t *testing.T) {
	validDbURI := "projects/spanner-cloud-test/instances/foo/databases/foodb"
	invalidDbUris := []string{
		// Completely wrong DB URI.
		"foobarDB",
		// Project ID contains "/".
		"projects/spanner-cloud/test/instances/foo/databases/foodb",
		// No instance ID.
		"projects/spanner-cloud-test/instances//databases/foodb",
	}
	if err := validDatabaseName(validDbURI); err != nil {
		t.Errorf("validateDatabaseName(%q) = %v, want nil", validDbURI, err)
	}
	for _, d := range invalidDbUris {
		if err, wantErr := validDatabaseName(d), "should conform to pattern"; !strings.Contains(err.Error(), wantErr) {
			t.Errorf("validateDatabaseName(%q) = %q, want error pattern %q", validDbURI, err, wantErr)
		}
	}
}

func TestReadOnlyTransactionClose(t *testing.T) {
	// Closing a ReadOnlyTransaction shouldn't panic.
	c := &Client{}
	tx := c.ReadOnlyTransaction()
	tx.Close()
}

func TestClient_Single(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Single_Unavailable(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, status.Error(codes.Unavailable, "Temporary unavailable"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Single_InvalidArgument(t *testing.T) {
	t.Parallel()
	err := testSingleQuery(t, status.Error(codes.InvalidArgument, "Invalid argument"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got unexpected exception %v, expected InvalidArgument", err)
	}
}

func TestClient_Single_RetryableErrorOnPartialResultSet(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	// Add two errors that will be returned by the mock server when the client
	// is trying to fetch a partial result set. Both errors are retryable.
	// The errors are not 'sticky' on the mocked server, i.e. once the error
	// has been returned once, the next call for the same partial result set
	// will succeed.

	// When the client is fetching the partial result set with resume token 2,
	// the mock server will respond with an internal error with the message
	// 'stream terminated by RST_STREAM'. The client will retry the call to get
	// this partial result set.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         spannerErrorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// When the client is fetching the partial result set with resume token 3,
	// the mock server will respond with a 'Unavailable' error. The client will
	// retry the call to get this partial result set.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         spannerErrorf(codes.Unavailable, "server is unavailable"),
		},
	)
	ctx := context.Background()
	if err := executeSingerQuery(ctx, client.Single()); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Single_NonRetryableErrorOnPartialResultSet(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	// Add two errors that will be returned by the mock server when the client
	// is trying to fetch a partial result set. The first error is retryable,
	// the second is not.

	// This error will automatically be retried.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         spannerErrorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// 'Session not found' is not retryable and the error will be returned to
	// the user.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         spannerErrorf(codes.NotFound, "Session not found"),
		},
	)
	ctx := context.Background()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.NotFound)
	}
}

func TestClient_Single_DeadlineExceeded_NoErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 50 * time.Millisecond,
		})
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Millisecond))
	defer cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.DeadlineExceeded)
	}
}

func TestClient_Single_DeadlineExceeded_WithErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         spannerErrorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken:   EncodeResumeToken(3),
			Err:           spannerErrorf(codes.Unavailable, "server is unavailable"),
			ExecutionTime: 50 * time.Millisecond,
		},
	)
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(25*time.Millisecond))
	defer cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("got unexpected error %v, expected DeadlineExceeded", err)
	}
}

func TestClient_Single_ContextCanceled_noDeclaredServerErrors(t *testing.T) {
	t.Parallel()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.Canceled {
		t.Fatalf("got unexpected error %v, expected Canceled", err)
	}
}

func TestClient_Single_ContextCanceled_withDeclaredServerErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         spannerErrorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         spannerErrorf(codes.Unavailable, "server is unavailable"),
		},
	)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	f := func(rowCount int64) error {
		if rowCount == 2 {
			cancel()
		}
		return nil
	}
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	err := executeSingerQueryWithRowFunc(ctx, client.Single(), f)
	if status.Code(err) != codes.Canceled {
		t.Fatalf("got unexpected error %v, expected Canceled", err)
	}
}

func testSingleQuery(t *testing.T, serverError error) error {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	if serverError != nil {
		server.TestSpanner.SetError(serverError)
	}
	return executeSingerQuery(ctx, client.Single())
}

func executeSingerQuery(ctx context.Context, tx *ReadOnlyTransaction) error {
	return executeSingerQueryWithRowFunc(ctx, tx, nil)
}

func executeSingerQueryWithRowFunc(ctx context.Context, tx *ReadOnlyTransaction, f func(rowCount int64) error) error {
	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		var singerID, albumID int64
		var albumTitle string
		if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
			return err
		}
		rowCount++
		if f != nil {
			if err := f(rowCount); err != nil {
				return err
			}
		}
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		return spannerErrorf(codes.Internal, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	return nil
}

func createSimulatedExecutionTimeWithTwoUnavailableErrors(method string) map[string]SimulatedExecutionTime {
	errors := make([]error, 2)
	errors[0] = status.Error(codes.Unavailable, "Temporary unavailable")
	errors[1] = status.Error(codes.Unavailable, "Temporary unavailable")
	executionTimes := make(map[string]SimulatedExecutionTime)
	executionTimes[method] = SimulatedExecutionTime{
		Errors: errors,
	}
	return executionTimes
}

func TestClient_ReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, make(map[string]SimulatedExecutionTime)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnSessionCreate(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodCreateSession)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodBeginTransaction)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(t, createSimulatedExecutionTimeWithTwoUnavailableErrors(MethodExecuteStreamingSql)); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnCreateSessionAndBeginTransaction(t *testing.T) {
	t.Parallel()
	exec := map[string]SimulatedExecutionTime{
		MethodCreateSession:    {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
		MethodBeginTransaction: {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
	}
	if err := testReadOnlyTransaction(t, exec); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_UnavailableOnCreateSessionAndInvalidArgumentOnBeginTransaction(t *testing.T) {
	t.Parallel()
	exec := map[string]SimulatedExecutionTime{
		MethodCreateSession:    {Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")}},
		MethodBeginTransaction: {Errors: []error{status.Error(codes.InvalidArgument, "Invalid argument")}},
	}
	if err := testReadOnlyTransaction(t, exec); err == nil {
		t.Fatalf("Missing expected exception")
	} else if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Got unexpected exception: %v", err)
	}
}

func testReadOnlyTransaction(t *testing.T, executionTimes map[string]SimulatedExecutionTime) error {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for method, exec := range executionTimes {
		server.TestSpanner.PutExecutionTime(method, exec)
	}
	tx := client.ReadOnlyTransaction()
	defer tx.Close()
	ctx := context.Background()
	return executeSingerQuery(ctx, tx)
}

func TestClient_ReadWriteTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, make(map[string]SimulatedExecutionTime), 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionCommitAborted(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionExecuteStreamingSqlAborted(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Aborted, "Transaction aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginAndAbortOnCommit(t *testing.T) {
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction:  {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodCommitTransaction: {Errors: []error{status.Error(codes.Aborted, "Aborted")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_UnavailableOnBeginAndExecuteStreamingSqlAndTwiceAbortOnCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction:    {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Unavailable, "Unavailable")}},
		MethodCommitTransaction:   {Errors: []error{status.Error(codes.Aborted, "Aborted"), status.Error(codes.Aborted, "Aborted")}},
	}, 3); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_AbortedOnExecuteStreamingSqlAndCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{status.Error(codes.Aborted, "Aborted")}},
		MethodCommitTransaction:   {Errors: []error{status.Error(codes.Aborted, "Aborted"), status.Error(codes.Aborted, "Aborted")}},
	}, 4); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionCommitAbortedAndUnavailable(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {
			Errors: []error{
				status.Error(codes.Aborted, "Transaction aborted"),
				status.Error(codes.Unavailable, "Unavailable"),
			},
		},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransactionCommitAlreadyExists(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{status.Error(codes.AlreadyExists, "A row with this key already exists")}},
	}, 1); err != nil {
		if status.Code(err) != codes.AlreadyExists {
			t.Fatalf("Got unexpected error %v, expected %v", err, codes.AlreadyExists)
		}
	} else {
		t.Fatalf("Missing expected exception")
	}
}

func testReadWriteTransaction(t *testing.T, executionTimes map[string]SimulatedExecutionTime, expectedAttempts int) error {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for method, exec := range executionTimes {
		server.TestSpanner.PutExecutionTime(method, exec)
	}
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		rowCount := int64(0)
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
			rowCount++
		}
		if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
			return spannerErrorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if expectedAttempts != attempts {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, expectedAttempts)
	}
	return nil
}

func TestClient_ApplyAtLeastOnce(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadWriteTransaction_ErrUnexpectedEOF(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			var singerID, albumID int64
			var albumTitle string
			if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
				return err
			}
		}
		return io.ErrUnexpectedEOF
	})
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("Missing expected error %v, got %v", io.ErrUnexpectedEOF, err)
	}
	if attempts != 1 {
		t.Fatalf("unexpected number of attempts: %d, expected %d", attempts, 1)
	}
}
