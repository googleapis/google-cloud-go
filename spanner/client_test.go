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
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	itestutil "cloud.google.com/go/internal/testutil"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
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

// Test getInstanceName()
func TestGetInstanceName(t *testing.T) {
	validDbURI := "projects/spanner-cloud-test/instances/foo/databases/foodb"
	invalidDbUris := []string{
		// Completely wrong DB URI.
		"foobarDB",
		// Project ID contains "/".
		"projects/spanner-cloud/test/instances/foo/databases/foodb",
		// No instance ID.
		"projects/spanner-cloud-test/instances//databases/foodb",
	}
	want := "projects/spanner-cloud-test/instances/foo"
	got, err := getInstanceName(validDbURI)
	if err != nil {
		t.Errorf("getInstanceName(%q) has an error: %q, want nil", validDbURI, err)
	}
	if got != want {
		t.Errorf("getInstanceName(%q) = %q, want %q", validDbURI, got, want)
	}
	for _, d := range invalidDbUris {
		wantErr := "Failed to retrieve instance name"
		_, err = getInstanceName(d)
		if !strings.Contains(err.Error(), wantErr) {
			t.Errorf("getInstanceName(%q) has an error: %q, want error pattern %q", validDbURI, err, wantErr)
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

func TestClient_ResourceBasedRouting_WithEndpointsReturned(t *testing.T) {
	os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "true")
	defer os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "")

	// Create two servers. The base server receives the GetInstance request and
	// returns the instance endpoint of the target server. The client should contact
	// the target server after getting the instance endpoint.
	serverBase, optsBase, serverTeardownBase := NewMockedSpannerInMemTestServerWithAddr(t, "localhost:8081")
	defer serverTeardownBase()
	serverTarget, optsTarget, serverTeardownTarget := NewMockedSpannerInMemTestServerWithAddr(t, "localhost:8082")
	defer serverTeardownTarget()

	// Return the instance endpoint.
	instanceEndpoint := fmt.Sprintf("%s", optsTarget[0])
	resps := []proto.Message{&instancepb.Instance{
		EndpointUris: []string{instanceEndpoint},
	}}
	serverBase.TestInstanceAdmin.SetResps(resps)

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "some-project", "some-instance", "some-database")
	client, err := NewClientWithConfig(ctx, formattedDatabase, ClientConfig{}, optsBase...)
	if err != nil {
		t.Fatal(err)
	}

	if err := executeSingerQuery(ctx, client.Single()); err != nil {
		t.Fatal(err)
	}

	// The base server should not receive any requests.
	if _, err := shouldHaveReceived(serverBase.TestSpanner, []interface{}{}); err != nil {
		t.Fatal(err)
	}

	// The target server should receive requests.
	if _, err = shouldHaveReceived(serverTarget.TestSpanner, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ResourceBasedRouting_WithoutEndpointsReturned(t *testing.T) {
	os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "true")
	defer os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "")

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	// Return an empty list of endpoints.
	resps := []proto.Message{&instancepb.Instance{
		EndpointUris: []string{},
	}}
	server.TestInstanceAdmin.SetResps(resps)

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "some-project", "some-instance", "some-database")
	client, err := NewClientWithConfig(ctx, formattedDatabase, ClientConfig{}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	if err := executeSingerQuery(ctx, client.Single()); err != nil {
		t.Fatal(err)
	}

	// Check if the request goes to the default endpoint.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ResourceBasedRouting_WithPermissionDeniedError(t *testing.T) {
	os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "true")
	defer os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "")

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	server.TestInstanceAdmin.SetErr(status.Error(codes.PermissionDenied, "Permission Denied"))

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "some-project", "some-instance", "some-database")
	// `PermissionDeniedError` causes a warning message to be logged, which is expected.
	// We set the output to be discarded to avoid spamming the log.
	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	client, err := NewClientWithConfig(ctx, formattedDatabase, ClientConfig{logger: logger}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	if err := executeSingerQuery(ctx, client.Single()); err != nil {
		t.Fatal(err)
	}

	// Fallback to use the default endpoint when calling GetInstance() returns
	// a PermissionDenied error.
	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ResourceBasedRouting_WithUnavailableError(t *testing.T) {
	os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "true")
	defer os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "")

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	resps := []proto.Message{&instancepb.Instance{
		EndpointUris: []string{},
	}}
	server.TestInstanceAdmin.SetResps(resps)
	server.TestInstanceAdmin.SetErr(status.Error(codes.Unavailable, "Temporary unavailable"))

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "some-project", "some-instance", "some-database")
	_, err := NewClientWithConfig(ctx, formattedDatabase, ClientConfig{}, opts...)
	// The first request will get an error and the server resets the error to nil,
	// so the next request will be fine. Due to retrying, there is no errors.
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_ResourceBasedRouting_WithInvalidArgumentError(t *testing.T) {
	os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "true")
	defer os.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_RESOURCE_BASED_ROUTING", "")

	server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
	defer serverTeardown()

	server.TestInstanceAdmin.SetErr(status.Error(codes.InvalidArgument, "Invalid argument"))

	ctx := context.Background()
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "some-project", "some-instance", "some-database")
	_, err := NewClientWithConfig(ctx, formattedDatabase, ClientConfig{}, opts...)

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got unexpected exception %v, expected InvalidArgument", err)
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
	want := toSpannerError(newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"))
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
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	for i := 0; i < 10; i++ {
		_, err := client.Apply(context.Background(), ms, ApplyAtLeastOnce())
		if err != nil {
			t.Fatal(err)
		}
		if g, w := client.idleSessions.idleList.Len(), 1; g != w {
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		if g, w := len(server.TestSpanner.DumpSessions()), 1; g != w {
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
	}
	// There should be no sessions marked as checked out.
	client.idleSessions.mu.Lock()
	g, w := client.idleSessions.trackedSessionHandles.Len(), 0
	client.idleSessions.mu.Unlock()
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
		if g, w := client.idleSessions.idleList.Len(), 1; g != w {
			t.Fatalf("idle session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
		if g, w := len(server.TestSpanner.DumpSessions()), 1; g != w {
			t.Fatalf("server session count mismatch:\nGot: %v\nWant: %v", g, w)
		}
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

func TestReadWriteTransaction_ContextTimeoutDuringDuringCommit(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
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
		&sppb.CreateSessionRequest{},
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
		&sppb.CreateSessionRequest{},
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
	_, err := NewClientWithConfig(
		context.Background(),
		"projects/p/instances/i/databases/d",
		ClientConfig{NumChannels: configuredNumChannels},
		option.WithGRPCConnectionPool(configuredConnPool),
	)
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
