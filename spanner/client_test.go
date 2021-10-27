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
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	itestutil "cloud.google.com/go/internal/testutil"
	vkit "cloud.google.com/go/spanner/apiv1"
	. "cloud.google.com/go/spanner/internal/testutil"
	structpb "github.com/golang/protobuf/ptypes/struct"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
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
						return status.Errorf(codes.Internal, "unexpected number of api client token headers: %v", len(token))
					}
					if !strings.HasPrefix(token[0], "gl-go/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
					}
					if !strings.Contains(token[0], "gccl/") {
						return status.Errorf(codes.Internal, "unexpected api client token: %v", token[0])
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
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
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
		t.Fatalf("got: %v, want: %v", err, codes.InvalidArgument)
	}
}

func TestClient_Single_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_Single_Read_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodStreamingRead,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	iter := client.Single().Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_Single_ReadRow_SessionNotFound(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodStreamingRead,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	row, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
	if err != nil {
		t.Fatalf("Unexpected error for read row: %v", err)
	}
	if row == nil {
		t.Fatal("ReadRow did not return a row")
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
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// When the client is fetching the partial result set with resume token 3,
	// the mock server will respond with a 'Unavailable' error. The client will
	// retry the call to get this partial result set.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
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
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	// 'Session not found' is not retryable and the error will be returned to
	// the user.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"),
		},
	)
	ctx := context.Background()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.NotFound)
	}
}

func TestClient_Single_NonRetryableInternalErrors(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "grpc: error while marshaling: string field contains invalid UTF-8"),
		},
	)
	ctx := context.Background()
	err := executeSingerQuery(ctx, client.Single())
	if status.Code(err) != codes.Internal {
		t.Fatalf("Error mismatch:\ngot: %v\nwant: %v", err, codes.Internal)
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
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken:   EncodeResumeToken(3),
			Err:           status.Errorf(codes.Unavailable, "server is unavailable"),
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
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(3),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
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

func TestClient_Single_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: tt.client})
			defer teardown()

			var iter *RowIterator
			if tt.query.Options == nil {
				iter = client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			} else {
				iter = client.Single().QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
			}
			testQueryOptions(t, iter, server.TestSpanner, tt.want)
		})
	}
}

func TestClient_ReturnDatabaseName(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	got := client.DatabaseName()
	want := "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"
	if got != want {
		t.Fatalf("Incorrect database name returned, got: %s, want: %s", got, want)
	}
}

func testQueryOptions(t *testing.T, iter *RowIterator, server InMemSpannerServer, qo QueryOptions) {
	defer iter.Stop()

	_, err := iter.Next()
	if err != nil {
		t.Fatalf("Failed to read from the iterator: %v", err)
	}

	checkReqsForQueryOptions(t, server, qo)
}

func checkReqsForQueryOptions(t *testing.T, server InMemSpannerServer, qo QueryOptions) {
	reqs := drainRequestsFromServer(server)
	sqlReqs := []*sppb.ExecuteSqlRequest{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}

	if got, want := len(sqlReqs), 1; got != want {
		t.Fatalf("Length mismatch, got %v, want %v", got, want)
	}

	reqQueryOptions := sqlReqs[0].QueryOptions
	if got, want := reqQueryOptions.OptimizerVersion, qo.Options.OptimizerVersion; got != want {
		t.Fatalf("Optimizer version mismatch, got %v, want %v", got, want)
	}
	if got, want := reqQueryOptions.OptimizerStatisticsPackage, qo.Options.OptimizerStatisticsPackage; got != want {
		t.Fatalf("Optimizer statistics package mismatch, got %v, want %v", got, want)
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
		return status.Errorf(codes.Internal, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
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

func TestClient_ReadOnlyTransaction_SessionNotFoundOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	// Session not found is not retryable for a query on a multi-use read-only
	// transaction, as we would need to start a new transaction on a new
	// session.
	err := testReadOnlyTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	})
	want := ToSpannerError(newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"))
	if err == nil {
		t.Fatalf("missing expected error\nGot: nil\nWant: %v", want)
	}
	if status.Code(err) != status.Code(want) || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("error mismatch\nGot: %v\nWant: %v", err, want)
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

func TestClient_ReadOnlyTransaction_SessionNotFoundOnBeginTransaction(t *testing.T) {
	t.Parallel()
	if err := testReadOnlyTransaction(
		t,
		map[string]SimulatedExecutionTime{
			MethodBeginTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
		},
	); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_SessionNotFoundOnBeginTransaction_WithMaxOneSession(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(
		t,
		ClientConfig{
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: 0,
				MaxOpened: 1,
			},
		})
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodBeginTransaction,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	tx := client.ReadOnlyTransaction()
	defer tx.Close()
	ctx := context.Background()
	if err := executeSingerQuery(ctx, tx); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadOnlyTransaction_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: tt.client})
			defer teardown()

			tx := client.ReadOnlyTransaction()
			defer tx.Close()

			var iter *RowIterator
			if tt.query.Options == nil {
				iter = tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
			} else {
				iter = tx.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
			}
			testQueryOptions(t, iter, server.TestSpanner, tt.want)
		})
	}
}

func setQueryOptionsEnvVars(opts *sppb.ExecuteSqlRequest_QueryOptions) func() {
	os.Setenv("SPANNER_OPTIMIZER_VERSION", opts.OptimizerVersion)
	os.Setenv("SPANNER_OPTIMIZER_STATISTICS_PACKAGE", opts.OptimizerStatisticsPackage)
	return func() {
		defer os.Setenv("SPANNER_OPTIMIZER_VERSION", "")
		defer os.Setenv("SPANNER_OPTIMIZER_STATISTICS_PACKAGE", "")
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

func TestClient_ReadWriteTransaction_SessionNotFoundOnCommit(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodCommitTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnBeginTransaction(t *testing.T) {
	t.Parallel()
	// We expect only 1 attempt, as the 'Session not found' error is already
	//handled in the session pool where the session is prepared.
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodBeginTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnBeginTransactionWithEmptySessionPool(t *testing.T) {
	t.Parallel()
	// There will be no prepared sessions in the pool, so the error will occur
	// when the transaction tries to get a session from the pool. This will
	// also be handled by the session pool, so the transaction itself does not
	// need to retry, hence the expectedAttempts == 1.
	if err := testReadWriteTransactionWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{WriteSessions: 0.0},
	}, map[string]SimulatedExecutionTime{
		MethodBeginTransaction: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteStreamingSql(t *testing.T) {
	t.Parallel()
	if err := testReadWriteTransaction(t, map[string]SimulatedExecutionTime{
		MethodExecuteStreamingSql: {Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteSql,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		rowCount, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		if err != nil {
			return err
		}
		if g, w := rowCount, int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func TestClient_ReadWriteTransaction_SessionNotFoundOnExecuteBatchUpdate(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteBatchDml,
		SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	ctx := context.Background()
	var attempts int
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		rowCounts, err := tx.BatchUpdate(ctx, []Statement{NewStatement(UpdateBarSetFoo)})
		if err != nil {
			return err
		}
		if g, w := len(rowCounts), 1; g != w {
			return status.Errorf(codes.FailedPrecondition, "Row counts length mismatch\nGot: %v\nWant: %v", g, w)
		}
		if g, w := rowCounts[0], int64(UpdateBarSetFooRowCount); g != w {
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch\nGot: %v\nWant: %v", g, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("number of attempts mismatch:\nGot%d\nWant:%d", g, w)
	}
}

func TestClient_ReadWriteTransaction_Query_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var iter *RowIterator
				if tt.query.Options == nil {
					iter = tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
				} else {
					iter = tx.QueryWithOptions(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), tt.query)
				}
				testQueryOptions(t, iter, server.TestSpanner, tt.want)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClient_ReadWriteTransaction_Update_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: tt.client})
			defer teardown()

			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				var rowCount int64
				var err error
				if tt.query.Options == nil {
					rowCount, err = tx.Update(ctx, NewStatement(UpdateBarSetFoo))
				} else {
					rowCount, err = tx.UpdateWithOptions(ctx, NewStatement(UpdateBarSetFoo), tt.query)
				}
				if got, want := rowCount, int64(5); got != want {
					t.Fatalf("Incorrect updated row count: got %v, want %v", got, want)
				}
				return err
			})
			if err != nil {
				t.Fatalf("Failed to update rows: %v", err)
			}
			checkReqsForQueryOptions(t, server.TestSpanner, tt.want)
		})
	}
}

func TestClient_ReadWriteTransactionWithOptions(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	resp, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
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
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
		}
		return nil
	}, TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}})
	if err != nil {
		t.Fatalf("Failed to execute the transaction: %s", err)
	}
	if got, want := resp.CommitStats.MutationCount, int64(1); got != want {
		t.Fatalf("Mismatch mutation count - got: %d, want: %d", got, want)
	}
}

func TestClient_ReadWriteStmtBasedTransactionWithOptions(t *testing.T) {
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	tx, err := NewReadWriteStmtBasedTransactionWithOptions(
		ctx,
		client,
		TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}})
	if err != nil {
		t.Fatalf("Unexpected error when creating transaction: %v", err)
	}

	iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	rowCount := int64(0)
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error when fetching query results: %v", err)
		}
		var singerID, albumID int64
		var albumTitle string
		if err := row.Columns(&singerID, &albumID, &albumTitle); err != nil {
			t.Fatalf("Unexpected error when getting query data: %v", err)
		}
		rowCount++
	}
	resp, err := tx.CommitWithReturnResp(ctx)
	if err != nil {
		t.Fatalf("Unexpected error when committing transaction: %v", err)
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Errorf("Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
	if got, want := resp.CommitStats.MutationCount, int64(1); got != want {
		t.Fatalf("Mismatch mutation count - got: %d, want: %d", got, want)
	}
}

func TestClient_ReadWriteTransaction_DoNotLeakSessionOnPanic(t *testing.T) {
	// Make sure that there is always only one session in the pool.
	sc := SessionPoolConfig{
		MinOpened: 1,
		MaxOpened: 1,
	}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{SessionPoolConfig: sc})
	defer teardown()
	ctx := context.Background()

	// If a panic occurs during a transaction, the session will not leak.
	func() {
		defer func() { recover() }()

		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			panic("cause panic")
			return nil
		})
		if err != nil {
			t.Fatalf("Unexpected error during transaction: %v", err)
		}
	}()

	if g, w := client.idleSessions.idleList.Len(), 1; g != w {
		t.Fatalf("idle session count mismatch.\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_SessionNotFound(t *testing.T) {
	// Ensure we always have at least one session in the pool.
	sc := SessionPoolConfig{
		MinOpened: 1,
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{SessionPoolConfig: sc})
	defer teardown()
	ctx := context.Background()
	for {
		client.idleSessions.mu.Lock()
		numSessions := client.idleSessions.idleList.Len()
		client.idleSessions.mu.Unlock()
		if numSessions > 0 {
			break
		}
		time.After(time.Millisecond)
	}
	// Remove the session from the server without the pool knowing it.
	_, err := server.TestSpanner.DeleteSession(ctx, &sppb.DeleteSessionRequest{Name: client.idleSessions.idleList.Front().Value.(*session).id})
	if err != nil {
		t.Fatalf("Failed to delete session unexpectedly: %v", err)
	}

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
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
		t.Fatalf("Unexpected error during transaction: %v", err)
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

func TestClient_ReadWriteTransaction_CommitAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_DMLAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_BatchDMLAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteBatchDml, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		_, err := tx.BatchUpdate(ctx, []Statement{{SQL: UpdateBarSetFoo}})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_BatchDMLAbortedHalfway(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	abortedStatement := "UPDATE FOO_ABORTED SET BAR=1 WHERE ID=2"
	server.TestSpanner.PutStatementResult(
		abortedStatement,
		&StatementResult{
			Type: StatementResultError,
			Err:  status.Error(codes.Aborted, "Statement was aborted"),
		},
	)
	ctx := context.Background()
	var updateCounts []int64
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		if attempts > 1 {
			// Replace the aborted result with a real result to prevent the
			// transaction from aborting indefinitely.
			server.TestSpanner.PutStatementResult(
				abortedStatement,
				&StatementResult{
					Type:        StatementResultUpdateCount,
					UpdateCount: 3,
				},
			)
		}
		var err error
		updateCounts, err = tx.BatchUpdate(ctx, []Statement{{SQL: abortedStatement}, {SQL: UpdateBarSetFoo}})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
	}
	if g, w := updateCounts, []int64{3, UpdateBarSetFooRowCount}; !testEqual(w, g) {
		t.Fatalf("update count mismatch\nWant: %v\nGot: %v", w, g)
	}
}

func TestClient_ReadWriteTransaction_QueryAborted(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.Aborted, "Aborted")},
	})
	defer teardown()
	ctx := context.Background()
	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		attempts++
		iter := tx.Query(ctx, Statement{SQL: SelectFooFromBar})
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
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempt count mismatch:\nWant: %v\nGot: %v", w, g)
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
	return testReadWriteTransactionWithConfig(t, ClientConfig{SessionPoolConfig: DefaultSessionPoolConfig}, executionTimes, expectedAttempts)
}

func testReadWriteTransactionWithConfig(t *testing.T, config ClientConfig, executionTimes map[string]SimulatedExecutionTime, expectedAttempts int) error {
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
			return status.Errorf(codes.FailedPrecondition, "Row count mismatch, got %v, expected %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
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

func TestClient_ApplyAtLeastOnceReuseSession(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:           0,
			WriteSessions:       0.0,
			TrackSessionHandles: true,
		},
	})
	defer teardown()
	sp := client.idleSessions
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	for i := 0; i < 10; i++ {
		_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
		if err != nil {
			t.Fatal(err)
		}
		sp.mu.Lock()
		if g, w := uint64(sp.idleList.Len())+sp.createReqs, sp.incStep; g != w {
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		if g, w := uint64(len(server.TestSpanner.DumpSessions())), sp.incStep; g != w {
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		sp.mu.Unlock()
	}
	// There should be no sessions marked as checked out.
	sp.mu.Lock()
	g, w := sp.trackedSessionHandles.Len(), 0
	sp.mu.Unlock()
	if g != w {
		t.Fatalf("checked out sessions count mismatch:\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_ApplyAtLeastOnceInvalidArgument(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:           0,
			WriteSessions:       0.0,
			TrackSessionHandles: true,
		},
	})
	defer teardown()
	sp := client.idleSessions
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	for i := 0; i < 10; i++ {
		server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
			SimulatedExecutionTime{
				Errors: []error{status.Error(codes.InvalidArgument, "Invalid data")},
			})
		_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
		sp.mu.Lock()
		if g, w := uint64(sp.idleList.Len())+sp.createReqs, sp.incStep; g != w {
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		if g, w := uint64(len(server.TestSpanner.DumpSessions())), sp.incStep; g != w {
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		sp.mu.Unlock()
	}
	// There should be no sessions marked as checked out.
	client.idleSessions.mu.Lock()
	g, w := client.idleSessions.trackedSessionHandles.Len(), 0
	client.idleSessions.mu.Unlock()
	if g != w {
		t.Fatalf("checked out sessions count mismatch:\nGot: %v\nWant: %v", g, w)
	}
}

func TestReadWriteTransaction_ErrUnexpectedEOF(t *testing.T) {
	t.Parallel()
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

func TestReadWriteTransaction_WrapError(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	// Abort the transaction on both the query as well as commit.
	// The first abort error will be wrapped. The client will unwrap the cause
	// of the error and retry the transaction. The aborted error on commit
	// will not be wrapped, but will also be recognized by the client as an
	// abort that should be retried.
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
		})
	msg := "query failed"
	numAttempts := 0
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error\nGot: %v\nWant: nil", err)
	}
	if g, w := numAttempts, 3; g != w {
		t.Fatalf("Number of transaction attempts mismatch\nGot: %d\nWant: %d", w, w)
	}

	// Execute a transaction that returns a non-retryable error that is
	// wrapped in a custom error. The transaction should return the custom
	// error.
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{status.Error(codes.NotFound, "Table not found")},
		})
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err == nil || err.Error() != msg {
		t.Fatalf("Unexpected error\nGot: %v\nWant: %v", err, msg)
	}
}

func TestReadWriteTransaction_WrapSessionNotFoundError(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodBeginTransaction,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")},
		})
	msg := "query failed"
	numAttempts := 0
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		numAttempts++
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// Wrap the error in another error that implements the
				// (xerrors|errors).Wrapper interface.
				return &wrappedTestError{err, msg}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Unexpected error\nGot: %v\nWant: nil", err)
	}
	// We want 3 attempts. The 'Session not found' error on BeginTransaction
	// will not retry the entire transaction, which means that we will have two
	// failed attempts and then a successful attempt.
	if g, w := numAttempts, 3; g != w {
		t.Fatalf("Number of transaction attempts mismatch\nGot: %d\nWant: %d", g, w)
	}
}

func TestClient_WriteStructWithPointers(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	type T struct {
		ID    int64
		Col1  *string
		Col2  []*string
		Col3  *bool
		Col4  []*bool
		Col5  *int64
		Col6  []*int64
		Col7  *float64
		Col8  []*float64
		Col9  *time.Time
		Col10 []*time.Time
		Col11 *civil.Date
		Col12 []*civil.Date
	}
	t1 := T{
		ID:    1,
		Col2:  []*string{nil},
		Col4:  []*bool{nil},
		Col6:  []*int64{nil},
		Col8:  []*float64{nil},
		Col10: []*time.Time{nil},
		Col12: []*civil.Date{nil},
	}
	s := "foo"
	b := true
	i := int64(100)
	f := 3.14
	tm := time.Now()
	d := civil.DateOf(time.Now())
	t2 := T{
		ID:    2,
		Col1:  &s,
		Col2:  []*string{&s},
		Col3:  &b,
		Col4:  []*bool{&b},
		Col5:  &i,
		Col6:  []*int64{&i},
		Col7:  &f,
		Col8:  []*float64{&f},
		Col9:  &tm,
		Col10: []*time.Time{&tm},
		Col11: &d,
		Col12: []*civil.Date{&d},
	}
	m1, err := InsertStruct("Tab", &t1)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := InsertStruct("Tab", &t2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), []*Mutation{m1, m2})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if commit, ok := req.(*sppb.CommitRequest); ok {
			if g, w := len(commit.Mutations), 2; w != g {
				t.Fatalf("mutation count mismatch\nGot: %v\nWant: %v", g, w)
			}
			insert := commit.Mutations[0].GetInsert()
			// The first insert should contain NULL values and arrays
			// containing exactly one NULL element.
			for i := 1; i < len(insert.Values[0].Values); i += 2 {
				// The non-array columns should contain NULL values.
				g, w := insert.Values[0].Values[i].GetKind(), &structpb.Value_NullValue{}
				if _, ok := g.(*structpb.Value_NullValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, w)
				}
				// The array columns should not be NULL.
				g, wList := insert.Values[0].Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain 1 NULL value.
				if gLength, wLength := len(insert.Values[0].Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g, w = insert.Values[0].Values[i+1].GetListValue().Values[0].GetKind(), &structpb.Value_NullValue{}
				if _, ok := g.(*structpb.Value_NullValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, w)
				}
			}

			// The second insert should contain all non-NULL values.
			insert = commit.Mutations[1].GetInsert()
			for i := 1; i < len(insert.Values[0].Values); i += 2 {
				// The non-array columns should contain non-NULL values.
				g := insert.Values[0].Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should also be non-NULL.
				g, wList := insert.Values[0].Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain exactly 1 non-NULL value.
				if gLength, wLength := len(insert.Values[0].Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g = insert.Values[0].Values[i+1].GetListValue().Values[0].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
			}
		}
	}
}

func TestClient_WriteStructWithCustomTypes(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	type CustomString string
	type CustomBool bool
	type CustomInt64 int64
	type CustomFloat64 float64
	type CustomTime time.Time
	type CustomDate civil.Date
	type T struct {
		ID    int64
		Col1  CustomString
		Col2  []CustomString
		Col3  CustomBool
		Col4  []CustomBool
		Col5  CustomInt64
		Col6  []CustomInt64
		Col7  CustomFloat64
		Col8  []CustomFloat64
		Col9  CustomTime
		Col10 []CustomTime
		Col11 CustomDate
		Col12 []CustomDate
	}
	t1 := T{
		ID:    1,
		Col2:  []CustomString{},
		Col4:  []CustomBool{},
		Col6:  []CustomInt64{},
		Col8:  []CustomFloat64{},
		Col10: []CustomTime{},
		Col12: []CustomDate{},
	}
	t2 := T{
		ID:    2,
		Col1:  "foo",
		Col2:  []CustomString{"foo"},
		Col3:  true,
		Col4:  []CustomBool{true},
		Col5:  100,
		Col6:  []CustomInt64{100},
		Col7:  3.14,
		Col8:  []CustomFloat64{3.14},
		Col9:  CustomTime(time.Now()),
		Col10: []CustomTime{CustomTime(time.Now())},
		Col11: CustomDate(civil.DateOf(time.Now())),
		Col12: []CustomDate{CustomDate(civil.DateOf(time.Now()))},
	}
	m1, err := InsertStruct("Tab", &t1)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := InsertStruct("Tab", &t2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), []*Mutation{m1, m2})
	if err != nil {
		t.Fatal(err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	for _, req := range requests {
		if commit, ok := req.(*sppb.CommitRequest); ok {
			if g, w := len(commit.Mutations), 2; w != g {
				t.Fatalf("mutation count mismatch\nGot: %v\nWant: %v", g, w)
			}
			insert1 := commit.Mutations[0].GetInsert()
			row1 := insert1.Values[0]
			// The first insert should contain empty values and empty arrays
			for i := 1; i < len(row1.Values); i += 2 {
				// The non-array columns should contain empty values.
				g := row1.Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should not be NULL.
				g, wList := row1.Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
			}

			// The second insert should contain all non-NULL values.
			insert2 := commit.Mutations[1].GetInsert()
			row2 := insert2.Values[0]
			for i := 1; i < len(row2.Values); i += 2 {
				// The non-array columns should contain non-NULL values.
				g := row2.Values[i].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
				// The array columns should also be non-NULL.
				g, wList := row2.Values[i+1].GetKind(), &structpb.Value_ListValue{}
				if _, ok := g.(*structpb.Value_ListValue); !ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: %v", g, wList)
				}
				// The array should contain exactly 1 non-NULL value.
				if gLength, wLength := len(row2.Values[i+1].GetListValue().Values), 1; gLength != wLength {
					t.Fatalf("list value length mismatch\nGot: %v\nWant: %v", gLength, wLength)
				}
				g = row2.Values[i+1].GetListValue().Values[0].GetKind()
				if _, ok := g.(*structpb.Value_NullValue); ok {
					t.Fatalf("type mismatch\nGot: %v\nWant: non-NULL value", g)
				}
			}
		}
	}
}

func TestReadWriteTransaction_ContextTimeoutDuringCommit(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until session creation has seized so that
	// context timeout won't happen while a session is being created.
	waitFor(t, func() error {
		sp := client.idleSessions
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if sp.createReqs != 0 {
			return fmt.Errorf("%d sessions are still in creation", sp.createReqs)
		}
		return nil
	})

	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			MinimumExecutionTime: time.Minute,
		})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		tx.BufferWrite([]*Mutation{Insert("FOO", []string{"ID", "NAME"}, []interface{}{int64(1), "bar"})})
		return nil
	})
	errContext, _ := context.WithTimeout(context.Background(), -time.Second)
	w := toSpannerErrorWithCommitInfo(errContext.Err(), true).(*Error)
	var se *Error
	if !errorAs(err, &se) {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", err, w)
	}
	if se.GRPCStatus().Code() != w.GRPCStatus().Code() {
		t.Fatalf("Error status mismatch:\nGot: %v\nWant: %v", se.GRPCStatus(), w.GRPCStatus())
	}
	if se.Error() != w.Error() {
		t.Fatalf("Error message mismatch:\nGot %s\nWant: %s", se.Error(), w.Error())
	}
	var outcome *TransactionOutcomeUnknownError
	if !errorAs(err, &outcome) {
		t.Fatalf("Missing wrapped TransactionOutcomeUnknownError error")
	}
}

func TestFailedCommit_NoRollback(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     0,
			MaxOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodCommitTransaction,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.InvalidArgument, "Invalid mutations")},
		})
	_, err := client.Apply(context.Background(), []*Mutation{
		Insert("FOO", []string{"ID", "BAR"}, []interface{}{1, "value"}),
	})
	if got, want := status.Convert(err).Code(), codes.InvalidArgument; got != want {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", got, want)
	}
	// The failed commit should not trigger a rollback after the commit.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
	}); err != nil {
		t.Fatalf("Received RPCs mismatch: %v", err)
	}
}

func TestFailedUpdate_ShouldRollback(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     0,
			MaxOpened:     1,
			WriteSessions: 0,
		},
	})
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			Errors: []error{status.Errorf(codes.InvalidArgument, "Invalid update")},
		})
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, NewStatement("UPDATE FOO SET BAR='value' WHERE ID=1"))
		return err
	})
	if got, want := status.Convert(err).Code(), codes.InvalidArgument; got != want {
		t.Fatalf("Error mismatch\nGot: %v\nWant: %v", got, want)
	}
	// The failed update should trigger a rollback.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.RollbackRequest{},
	}); err != nil {
		t.Fatalf("Received RPCs mismatch: %v", err)
	}
}

func TestClient_NumChannels(t *testing.T) {
	t.Parallel()

	configuredNumChannels := 8
	_, client, teardown := setupMockedTestServerWithConfig(
		t,
		ClientConfig{NumChannels: configuredNumChannels},
	)
	defer teardown()
	if g, w := client.sc.connPool.Num(), configuredNumChannels; g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPool(t *testing.T) {
	t.Parallel()

	configuredConnPool := 8
	_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(
		t,
		ClientConfig{},
		[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
	)
	defer teardown()
	if g, w := client.sc.connPool.Num(), configuredConnPool; g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPoolAndNumChannels(t *testing.T) {
	t.Parallel()

	configuredNumChannels := 8
	configuredConnPool := 8
	_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(
		t,
		ClientConfig{NumChannels: configuredNumChannels},
		[]option.ClientOption{option.WithGRPCConnectionPool(configuredConnPool)},
	)
	defer teardown()
	if g, w := client.sc.connPool.Num(), configuredConnPool; g != w {
		t.Fatalf("NumChannels mismatch\nGot: %v\nWant: %v", g, w)
	}
}

func TestClient_WithGRPCConnectionPoolAndNumChannels_Misconfigured(t *testing.T) {
	t.Parallel()

	// Deliberately misconfigure NumChannels and ConnPool.
	configuredNumChannels := 8
	configuredConnPool := 16

	_, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()
	opts = append(opts, option.WithGRPCConnectionPool(configuredConnPool))

	_, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{NumChannels: configuredNumChannels}, opts...)
	msg := "Connection pool mismatch:"
	if err == nil {
		t.Fatalf("Error mismatch\nGot: nil\nWant: %s", msg)
	}
	var se *Error
	if ok := errorAs(err, &se); !ok {
		t.Fatalf("Error mismatch\nGot: %v\nWant: An instance of a Spanner error", err)
	}
	if g, w := se.GRPCStatus().Code(), codes.InvalidArgument; g != w {
		t.Fatalf("Error code mismatch\nGot: %v\nWant: %v", g, w)
	}
	if !strings.Contains(se.Error(), msg) {
		t.Fatalf("Error message mismatch\nGot: %s\nWant: %s", se.Error(), msg)
	}
}

func TestClient_CallOptions(t *testing.T) {
	t.Parallel()
	co := &vkit.CallOptions{
		CreateSession: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable, codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}

	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{CallOptions: co})
	defer teardown()

	c, err := client.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to get a session client: %v", err)
	}

	cs := &gax.CallSettings{}
	// This is the default retry setting.
	c.CallOptions.CreateSession[0].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{250000000 32000000000 1.3 0} [14]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}

	// This is the custom retry setting.
	c.CallOptions.CreateSession[1].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{200000000 30000000000 1.25 0} [14 4]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}
}

func TestClient_QueryWithCallOptions(t *testing.T) {
	t.Parallel()
	co := &vkit.CallOptions{
		ExecuteSql: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{CallOptions: co})
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors: []error{status.Error(codes.DeadlineExceeded, "Deadline exceeded")},
	})
	defer teardown()
	ctx := context.Background()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{SQL: UpdateBarSetFoo})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ShouldReceiveMetadataForEmptyResultSet(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	// This creates an empty result set.
	res := server.CreateSingleRowSingersResult(SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	sql := "SELECT SingerId, AlbumId, AlbumTitle FROM Albums WHERE 1=2"
	server.TestSpanner.PutStatementResult(sql, res)
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Query(ctx, NewStatement(sql))
	defer iter.Stop()
	row, err := iter.Next()
	if err != iterator.Done {
		t.Errorf("Query result mismatch:\nGot: %v\nWant: <no rows>", row)
	}
	metadata := iter.Metadata
	if metadata == nil {
		t.Fatalf("Missing ResultSet Metadata")
	}
	if metadata.RowType == nil {
		t.Fatalf("Missing ResultSet RowType")
	}
	if metadata.RowType.Fields == nil {
		t.Fatalf("Missing ResultSet Fields")
	}
	if g, w := len(metadata.RowType.Fields), 3; g != w {
		t.Fatalf("Field count mismatch\nGot: %v\nWant: %v", g, w)
	}
	wantFieldNames := []string{"SingerId", "AlbumId", "AlbumTitle"}
	for i, w := range wantFieldNames {
		g := metadata.RowType.Fields[i].Name
		if g != w {
			t.Fatalf("Field[%v] name mismatch\nGot: %v\nWant: %v", i, g, w)
		}
	}
	wantFieldTypes := []sppb.TypeCode{sppb.TypeCode_INT64, sppb.TypeCode_INT64, sppb.TypeCode_STRING}
	for i, w := range wantFieldTypes {
		g := metadata.RowType.Fields[i].Type.Code
		if g != w {
			t.Fatalf("Field[%v] type mismatch\nGot: %v\nWant: %v", i, g, w)
		}
	}
}

func TestClient_EncodeCustomFieldType(t *testing.T) {
	t.Parallel()

	type typesTable struct {
		Int    customStructToInt    `spanner:"Int"`
		String customStructToString `spanner:"String"`
		Float  customStructToFloat  `spanner:"Float"`
		Bool   customStructToBool   `spanner:"Bool"`
		Time   customStructToTime   `spanner:"Time"`
		Date   customStructToDate   `spanner:"Date"`
	}

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()

	d := typesTable{
		Int:    customStructToInt{1, 23},
		String: customStructToString{"A", "B"},
		Float:  customStructToFloat{1.23, 12.3},
		Bool:   customStructToBool{true, false},
		Time:   customStructToTime{"A", "B"},
		Date:   customStructToDate{"A", "B"},
	}

	m, err := InsertStruct("Types", &d)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	ms := []*Mutation{m}
	_, err = client.Apply(ctx, ms)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)

	for _, req := range reqs {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			val := commitReq.Mutations[0].GetInsert().Values[0]

			if got, want := val.Values[0].GetStringValue(), "123"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[0].GetKind(), want)
			}
			if got, want := val.Values[1].GetStringValue(), "A-B"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[1].GetKind(), want)
			}
			if got, want := val.Values[2].GetNumberValue(), float64(123.123); got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[2].GetKind(), want)
			}
			if got, want := val.Values[3].GetBoolValue(), true; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[3].GetKind(), want)
			}
			if got, want := val.Values[4].GetStringValue(), "2016-11-15T15:04:05.999999999Z"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[4].GetKind(), want)
			}
			if got, want := val.Values[5].GetStringValue(), "2016-11-15"; got != want {
				t.Fatalf("value mismatch: got %v (kind %T), want %v", got, val.Values[5].GetKind(), want)
			}
		}
	}
}

func setupDecodeCustomFieldResult(server *MockedSpannerInMemTestServer, stmt string) error {
	metadata := &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "Int", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				{Name: "String", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "Float", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
				{Name: "Bool", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
				{Name: "Time", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
				{Name: "Date", Type: &sppb.Type{Code: sppb.TypeCode_DATE}},
			},
		},
	}
	rowValues := []*structpb.Value{
		{Kind: &structpb.Value_StringValue{StringValue: "123"}},
		{Kind: &structpb.Value_StringValue{StringValue: "A-B"}},
		{Kind: &structpb.Value_NumberValue{NumberValue: float64(123.123)}},
		{Kind: &structpb.Value_BoolValue{BoolValue: true}},
		{Kind: &structpb.Value_StringValue{StringValue: "2016-11-15T15:04:05.999999999Z"}},
		{Kind: &structpb.Value_StringValue{StringValue: "2016-11-15"}},
	}
	rows := []*structpb.ListValue{
		{Values: rowValues},
	}
	resultSet := &sppb.ResultSet{
		Metadata: metadata,
		Rows:     rows,
	}
	result := &StatementResult{
		Type:      StatementResultResultSet,
		ResultSet: resultSet,
	}
	return server.TestSpanner.PutStatementResult(stmt, result)
}

func TestClient_DecodeCustomFieldType(t *testing.T) {
	t.Parallel()

	type typesTable struct {
		Int    customStructToInt    `spanner:"Int"`
		String customStructToString `spanner:"String"`
		Float  customStructToFloat  `spanner:"Float"`
		Bool   customStructToBool   `spanner:"Bool"`
		Time   customStructToTime   `spanner:"Time"`
		Date   customStructToDate   `spanner:"Date"`
	}

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	query := "SELECT * FROM Types"
	setupDecodeCustomFieldResult(server, query)

	ctx := context.Background()
	stmt := Statement{SQL: query}
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var results []typesTable
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to get next: %v", err)
		}

		var d typesTable
		if err := row.ToStruct(&d); err != nil {
			t.Fatalf("failed to convert a row to a struct: %v", err)
		}
		results = append(results, d)
	}

	if len(results) > 1 {
		t.Fatalf("mismatch length of array: got %v, want 1", results)
	}

	want := typesTable{
		Int:    customStructToInt{1, 23},
		String: customStructToString{"A", "B"},
		Float:  customStructToFloat{1.23, 12.3},
		Bool:   customStructToBool{true, false},
		Time:   customStructToTime{"A", "B"},
		Date:   customStructToDate{"A", "B"},
	}
	got := results[0]
	if !testEqual(got, want) {
		t.Fatalf("mismatch result: got %v, want %v", got, want)
	}
}

func TestClient_EmulatorWithCredentialsFile(t *testing.T) {
	old := os.Getenv("SPANNER_EMULATOR_HOST")
	defer os.Setenv("SPANNER_EMULATOR_HOST", old)

	os.Setenv("SPANNER_EMULATOR_HOST", "localhost:1234")

	client, err := NewClientWithConfig(
		context.Background(),
		"projects/p/instances/i/databases/d",
		ClientConfig{},
		option.WithCredentialsFile("/path/to/key.json"),
	)
	defer client.Close()
	if err != nil {
		t.Fatalf("Failed to create a client with credentials file when running against an emulator: %v", err)
	}
}

func TestBatchReadOnlyTransaction_QueryOptions(t *testing.T) {
	ctx := context.Background()
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: qo})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)

	if txn.qo != qo {
		t.Fatalf("Query options are mismatched: got %v, want %v", txn.qo, qo)
	}
}

func TestBatchReadOnlyTransactionFromID_QueryOptions(t *testing.T) {
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: qo})
	defer teardown()

	txn := client.BatchReadOnlyTransactionFromID(BatchReadOnlyTransactionID{})

	if txn.qo != qo {
		t.Fatalf("Query options are mismatched: got %v, want %v", txn.qo, qo)
	}
}

type QueryOptionsTestCase struct {
	name   string
	client QueryOptions
	env    QueryOptions
	query  QueryOptions
	want   QueryOptions
}

func queryOptionsTestCases() []QueryOptionsTestCase {
	statsPkg := "latest"
	return []QueryOptionsTestCase{
		{
			"Client level",
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: nil},
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			"Environment level",
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			"Query level",
			QueryOptions{Options: nil},
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			"Environment level has precedence",
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			"Query level has precedence than client level",
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: nil},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
		},
		{
			"Query level has highest precedence",
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "2", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
			QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "3", OptimizerStatisticsPackage: statsPkg}},
		},
	}
}

func TestClient_DoForEachRow_ShouldNotEndSpanWithIteratorDoneError(t *testing.T) {
	// This test cannot be parallel, as the TestExporter does not support that.
	te := itestutil.NewTestExporter()
	defer te.Unregister()
	minOpened := uint64(1)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     minOpened,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	iter := client.Single().Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	iter.Do(func(r *Row) error {
		return nil
	})
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("No stats were exported before timeout")
	}
	// Preferably we would want to lock the TestExporter here, but the mutex TestExporter.mu is not exported, so we
	// cannot do that.
	if len(te.Spans) == 0 {
		t.Fatal("No spans were exported")
	}
	s := te.Spans[len(te.Spans)-1].Status
	if s.Code != int32(codes.OK) {
		t.Errorf("Span status mismatch\nGot: %v\nWant: %v", s.Code, codes.OK)
	}
}

func TestClient_DoForEachRow_ShouldEndSpanWithQueryError(t *testing.T) {
	// This test cannot be parallel, as the TestExporter does not support that.
	te := itestutil.NewTestExporter()
	defer te.Unregister()
	minOpened := uint64(1)
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     minOpened,
			WriteSessions: 0,
		},
	})
	defer teardown()

	// Wait until all sessions have been created, so we know that those requests will not interfere with the test.
	sp := client.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})

	sql := "SELECT * FROM"
	server.TestSpanner.PutStatementResult(sql, &StatementResult{
		Type: StatementResultError,
		Err:  status.Error(codes.InvalidArgument, "Invalid query"),
	})

	iter := client.Single().Query(context.Background(), NewStatement(sql))
	iter.Do(func(r *Row) error {
		return nil
	})
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("No stats were exported before timeout")
	}
	// Preferably we would want to lock the TestExporter here, but the mutex TestExporter.mu is not exported, so we
	// cannot do that.
	if len(te.Spans) == 0 {
		t.Fatal("No spans were exported")
	}
	s := te.Spans[len(te.Spans)-1].Status
	if s.Code != int32(codes.InvalidArgument) {
		t.Errorf("Span status mismatch\nGot: %v\nWant: %v", s.Code, codes.InvalidArgument)
	}
}

func TestClient_ReadOnlyTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_HIGH},
	} {
		for _, tx := range []*ReadOnlyTransaction{
			client.Single(),
			client.ReadOnlyTransaction(),
		} {
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			if tx.singleUse {
				tx = client.Single()
			}
			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
			iter.Next()
			iter.Stop()

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 2, sppb.RequestOptions{Priority: qo.Priority})
			tx.Close()
		}
	}
}

func TestClient_ReadWriteTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{CommitPriority: sppb.RequestOptions_PRIORITY_MEDIUM},
	} {
		for _, qo := range []QueryOptions{
			{},
			{Priority: sppb.RequestOptions_PRIORITY_MEDIUM},
		} {
			client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
				iter.Next()
				iter.Stop()

				iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
				iter.Next()
				iter.Stop()

				tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
				tx.BatchUpdateWithOptions(context.Background(), []Statement{
					NewStatement(UpdateBarSetFoo),
				}, qo)
				checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, sppb.RequestOptions{Priority: qo.Priority})

				return nil
			}, to)
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{Priority: to.CommitPriority})
		}
	}
}

func TestClient_StmtBasedReadWriteTransaction_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{CommitPriority: sppb.RequestOptions_PRIORITY_LOW},
	} {
		for _, qo := range []QueryOptions{
			{},
			{Priority: sppb.RequestOptions_PRIORITY_LOW},
		} {
			tx, _ := NewReadWriteStmtBasedTransactionWithOptions(context.Background(), client, to)
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
			iter.Next()
			iter.Stop()

			tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
			tx.BatchUpdateWithOptions(context.Background(), []Statement{
				NewStatement(UpdateBarSetFoo),
			}, qo)

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, sppb.RequestOptions{Priority: qo.Priority})
			tx.Commit(context.Background())
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{Priority: to.CommitPriority})
		}
	}
}

func TestClient_PDML_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_HIGH},
	} {
		client.PartitionedUpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 1, sppb.RequestOptions{Priority: qo.Priority})
	}
}

func TestClient_Apply_Priority(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})})
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, Priority(sppb.RequestOptions_PRIORITY_HIGH))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{Priority: sppb.RequestOptions_PRIORITY_HIGH})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce())
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce(), Priority(sppb.RequestOptions_PRIORITY_MEDIUM))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{Priority: sppb.RequestOptions_PRIORITY_MEDIUM})
}

func TestClient_ReadOnlyTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, qo := range []QueryOptions{
		{},
		{RequestTag: "tag-1"},
	} {
		for _, tx := range []*ReadOnlyTransaction{
			client.Single(),
			client.ReadOnlyTransaction(),
		} {
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			if tx.singleUse {
				tx = client.Single()
			}
			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
			iter.Next()
			iter.Stop()

			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 2, sppb.RequestOptions{RequestTag: qo.RequestTag})
			tx.Close()
		}
	}
}

func TestClient_ReadWriteTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{TransactionTag: "tx-tag-1"},
	} {
		for _, qo := range []QueryOptions{
			{},
			{RequestTag: "request-tag-1"},
		} {
			client.ReadWriteTransactionWithOptions(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
				iter.Next()
				iter.Stop()

				iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
				iter.Next()
				iter.Stop()

				tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
				tx.BatchUpdateWithOptions(context.Background(), []Statement{
					NewStatement(UpdateBarSetFoo),
				}, qo)

				// Check for SQL requests inside the transaction to prevent the check to
				// drain the commit request from the server.
				checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, sppb.RequestOptions{RequestTag: qo.RequestTag, TransactionTag: to.TransactionTag})
				return nil
			}, to)
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{TransactionTag: to.TransactionTag})
		}
	}
}

func TestClient_StmtBasedReadWriteTransaction_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	for _, to := range []TransactionOptions{
		{},
		{TransactionTag: "tx-tag-1"},
	} {
		for _, qo := range []QueryOptions{
			{},
			{RequestTag: "request-tag-1"},
		} {
			tx, _ := NewReadWriteStmtBasedTransactionWithOptions(context.Background(), client, to)
			iter := tx.QueryWithOptions(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), qo)
			iter.Next()
			iter.Stop()

			iter = tx.ReadWithOptions(context.Background(), "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{RequestTag: qo.RequestTag})
			iter.Next()
			iter.Stop()

			tx.UpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
			tx.BatchUpdateWithOptions(context.Background(), []Statement{
				NewStatement(UpdateBarSetFoo),
			}, qo)
			checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 4, sppb.RequestOptions{RequestTag: qo.RequestTag, TransactionTag: to.TransactionTag})

			tx.Commit(context.Background())
			checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{TransactionTag: to.TransactionTag})
		}
	}
}

func TestClient_PDML_Tag(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{RequestTag: "request-tag-1"},
	} {
		client.PartitionedUpdateWithOptions(context.Background(), NewStatement(UpdateBarSetFoo), qo)
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 1, sppb.RequestOptions{RequestTag: qo.RequestTag})
	}
}

func TestClient_Apply_Tagging(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})})
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, TransactionTag("tx-tag"))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{TransactionTag: "tx-tag"})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce())
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{})

	client.Apply(context.Background(), []*Mutation{Insert("foo", []string{"col1"}, []interface{}{"val1"})}, ApplyAtLeastOnce(), TransactionTag("tx-tag"))
	checkCommitForExpectedRequestOptions(t, server.TestSpanner, sppb.RequestOptions{TransactionTag: "tx-tag"})
}

func TestClient_PartitionQuery_RequestOptions(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, qo := range []QueryOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_LOW},
		{RequestTag: "batch-query-tag"},
		{Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "batch-query-with-medium-prio"},
	} {
		ctx := context.Background()
		txn, _ := client.BatchReadOnlyTransaction(ctx, StrongRead())
		partitions, _ := txn.PartitionQueryWithOptions(ctx, NewStatement(SelectFooFromBar), PartitionOptions{MaxPartitions: 10}, qo)
		for _, p := range partitions {
			iter := txn.Execute(ctx, p)
			iter.Next()
			iter.Stop()
		}
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, len(partitions), sppb.RequestOptions{RequestTag: qo.RequestTag, Priority: qo.Priority})
	}
}

func TestClient_PartitionRead_RequestOptions(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	for _, ro := range []ReadOptions{
		{},
		{Priority: sppb.RequestOptions_PRIORITY_LOW},
		{RequestTag: "batch-read-tag"},
		{Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "batch-read-with-medium-prio"},
	} {
		ctx := context.Background()
		txn, _ := client.BatchReadOnlyTransaction(ctx, StrongRead())
		partitions, _ := txn.PartitionReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{MaxPartitions: 10}, ro)
		for _, p := range partitions {
			iter := txn.Execute(ctx, p)
			iter.Next()
			iter.Stop()
		}
		checkRequestsForExpectedRequestOptions(t, server.TestSpanner, len(partitions), sppb.RequestOptions{RequestTag: ro.RequestTag, Priority: ro.Priority})
	}
}

func checkRequestsForExpectedRequestOptions(t *testing.T, server InMemSpannerServer, reqCount int, ro sppb.RequestOptions) {
	reqs := drainRequestsFromServer(server)
	reqOptions := []*sppb.RequestOptions{}

	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			reqOptions = append(reqOptions, sqlReq.RequestOptions)
		}
		if batchReq, ok := req.(*sppb.ExecuteBatchDmlRequest); ok {
			reqOptions = append(reqOptions, batchReq.RequestOptions)
		}
		if readReq, ok := req.(*sppb.ReadRequest); ok {
			reqOptions = append(reqOptions, readReq.RequestOptions)
		}
	}

	if got, want := len(reqOptions), reqCount; got != want {
		t.Fatalf("Requests length mismatch\nGot: %v\nWant: %v", got, want)
	}

	for _, opts := range reqOptions {
		if opts == nil {
			opts = &sppb.RequestOptions{}
		}
		if got, want := opts.Priority, ro.Priority; got != want {
			t.Fatalf("Request priority mismatch\nGot: %v\nWant: %v", got, want)
		}
		if got, want := opts.RequestTag, ro.RequestTag; got != want {
			t.Fatalf("Request tag mismatch\nGot: %v\nWant: %v", got, want)
		}
		if got, want := opts.TransactionTag, ro.TransactionTag; got != want {
			t.Fatalf("Transaction tag mismatch\nGot: %v\nWant: %v", got, want)
		}
	}
}

func checkCommitForExpectedRequestOptions(t *testing.T, server InMemSpannerServer, ro sppb.RequestOptions) {
	reqs := drainRequestsFromServer(server)
	var commit *sppb.CommitRequest
	var ok bool

	for _, req := range reqs {
		if commit, ok = req.(*sppb.CommitRequest); ok {
			break
		}
	}

	if commit == nil {
		t.Fatalf("Missing commit request")
	}

	var got sppb.RequestOptions_Priority
	if commit.RequestOptions != nil {
		got = commit.RequestOptions.Priority
	}
	want := ro.Priority
	if got != want {
		t.Fatalf("Commit priority mismatch\nGot: %v\nWant: %v", got, want)
	}

	var requestTag string
	var transactionTag string
	if commit.RequestOptions != nil {
		requestTag = commit.RequestOptions.RequestTag
		transactionTag = commit.RequestOptions.TransactionTag
	}
	if got, want := requestTag, ro.RequestTag; got != want {
		t.Fatalf("Commit request tag mismatch\nGot: %v\nWant: %v", got, want)
	}
	if got, want := transactionTag, ro.TransactionTag; got != want {
		t.Fatalf("Commit transaction tag mismatch\nGot: %v\nWant: %v", got, want)
	}
}

func TestClient_Single_Read_WithNumericKey(t *testing.T) {
	t.Parallel()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Read(ctx, "Albums", KeySets(Key{*big.NewRat(1, 1)}), []string{"SingerId", "AlbumId", "AlbumTitle"})
	defer iter.Stop()
	rowCount := int64(0)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rowCount++
	}
	if rowCount != SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount {
		t.Fatalf("row count mismatch\nGot: %v\nWant: %v", rowCount, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount)
	}
}

func TestClient_CloseWithUnresponsiveBackend(t *testing.T) {
	t.Parallel()

	minOpened := uint64(5)
	server, client, teardown := setupMockedTestServerWithConfig(t,
		ClientConfig{
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: minOpened,
			},
		})
	defer teardown()
	sp := client.idleSessions

	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != minOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})
	server.TestSpanner.Freeze()
	defer server.TestSpanner.Unfreeze()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	sp.close(ctx)

	if w, g := context.DeadlineExceeded, ctx.Err(); w != g {
		t.Fatalf("context error mismatch\nWant: %v\nGot: %v", w, g)
	}
}
