// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	longrunning "cloud.google.com/go/longrunning/autogen"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/api/option"
	longrunningpb "google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	gstatus "google.golang.org/grpc/status"
)

func Test_extractDBName(t *testing.T) {
	g, w := extractDBName("CREATE DATABASE FOO"), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("  CREATE DATABASE FOO"), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("  CREATE\nDATABASE\tFOO"), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("  CREATE     DATABASE      FOO   "), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("CREATE DATABASE `FOO`"), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("  CREATE     DATABASE      `FOO`   "), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("CREATE DATABASE ```FOO```"), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
	g, w = extractDBName("  CREATE     DATABASE      ```FOO```   "), "FOO"
	if g != w {
		t.Errorf("database name mismatch\nGot: %q\nWant: %q\n", g, w)
	}
}

func (s *mockDatabaseAdminServer) ListDatabaseOperations(ctx context.Context, req *database.ListDatabaseOperationsRequest) (*database.ListDatabaseOperationsResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if xg := md["x-goog-api-client"]; len(xg) == 0 || !strings.Contains(xg[0], "gl-go/") {
		return nil, fmt.Errorf("x-goog-api-client = %v, expected gl-go key", xg)
	}
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	resp := s.resps[0]
	s.resps = s.resps[1:]
	return resp.(*database.ListDatabaseOperationsResponse), nil
}

var (
	operationsClientOpt option.ClientOption
	mockOperations      mockOperationsServer
)

type mockOperationsServer struct {
	// Embed for forward compatibility.
	// Tests will keep working if more methods are added
	// in the future.
	longrunningpb.OperationsServer

	reqs []proto.Message

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []proto.Message
}

// initMockOperations initializes a separate long-running operations server
// that can be used for testing calls that need to list operations. The client
// needs to be manually linked with this server instead of the
// mockDatabaseAdmin server.
func initMockOperations() {
	if operationsClientOpt != nil {
		return
	}
	serv := grpc.NewServer()
	longrunningpb.RegisterOperationsServer(serv, &mockOperations)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	operationsClientOpt = option.WithGRPCConn(conn)
}

func (s *mockOperationsServer) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if xg := md["x-goog-api-client"]; len(xg) == 0 || !strings.Contains(xg[0], "gl-go/") {
		return nil, fmt.Errorf("x-goog-api-client = %v, expected gl-go key", xg)
	}
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	resp := s.resps[0]
	s.resps = s.resps[1:]
	return resp.(*longrunningpb.Operation), nil
}

func Test_CreateDatabaseWithRetry(t *testing.T) {
	var name string = "name3373707"
	var expectedResponse = &database.Database{
		Name: name,
	}
	mockDatabaseAdmin.err = nil
	mockDatabaseAdmin.reqs = nil

	any, err := ptypes.MarshalAny(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &longrunningpb.Operation{
		Name:   "longrunning-test",
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: any},
	})

	var formattedParent string = fmt.Sprintf("projects/%s/instances/%s", "[PROJECT]", "[INSTANCE]")
	var createStatement string = fmt.Sprintf("CREATE DATABASE %s", name)
	var request = &database.CreateDatabaseRequest{
		Parent:          formattedParent,
		CreateStatement: createStatement,
	}

	c, err := NewDatabaseAdminClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}
	respLRO, err := c.CreateDatabaseWithRetry(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := respLRO.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want, got := request, mockDatabaseAdmin.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("wrong request %q, want %q", got, want)
	}
	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("wrong response %q, want %q)", got, want)
	}
}

type testRetryer struct {
	f func(err error) (pause time.Duration, shouldRetry bool)
}

func (r *testRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	return r.f(err)
}

func Test_CreateDatabaseWithRetry_Unavailable_ServerReceivedRequest_OperationInProgress(t *testing.T) {
	// Initialize a mock operations server as we will need to list long-running
	// operations.
	initMockOperations()
	// Use a specific test retry that will ensure that consecutive calls to the
	// mock server will return different answers.
	originalRetryer := retryer
	defer func() { retryer = originalRetryer }()

	// Set up mockDatabaseAdmin to return an Unavailable error for
	// CreateDatabase.
	var name string = "name3373707"
	errs := []error{gstatus.Error(codes.Unavailable, "test error")}
	mockDatabaseAdmin.err = errs[0]
	retryer = &testRetryer{f: func(err error) (pause time.Duration, shouldRetry bool) {
		code := gstatus.Code(err)
		if code == codes.Unavailable || code == codes.DeadlineExceeded {
			// Pop the errors from the stack to prevent the same error from
			// being returned each time.
			if len(errs) > 1 {
				mockDatabaseAdmin.err = errs[0]
				errs = errs[1:]
			} else {
				mockDatabaseAdmin.err = nil
			}
			return time.Millisecond, true
		}
		return 0, false
	}}

	// Set up the mockDatabaseAdmin to return a long-running operation for the
	// initial CreateDatabase call.
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &database.ListDatabaseOperationsResponse{
		Operations: []*longrunningpb.Operation{
			{
				Name: fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
				Done: false,
			},
		},
		NextPageToken: "",
	})

	// Setup the mockOperations to first return an operation that is not yet
	// done, and then an operation that is done with the expected database.
	mockOperations.resps = append(mockOperations.resps, &longrunningpb.Operation{
		Name: fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
		Done: false,
	})
	var expectedResponse = &database.Database{
		Name: name,
	}
	any, err := ptypes.MarshalAny(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}
	mockOperations.resps = append(mockOperations.resps, &longrunningpb.Operation{
		Name:   fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: any},
	})

	mockDatabaseAdmin.reqs = nil
	var formattedParent string = fmt.Sprintf("projects/%s/instances/%s", "[PROJECT]", "[INSTANCE]")
	var createStatement string = fmt.Sprintf("CREATE DATABASE %s", name)
	var request = &database.CreateDatabaseRequest{
		Parent:          formattedParent,
		CreateStatement: createStatement,
	}

	c, err := NewDatabaseAdminClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}
	// Set the operations client manually.
	c.LROClient, err = longrunning.NewOperationsClient(context.Background(), operationsClientOpt)
	if err != nil {
		t.Fatal(err)
	}

	respLRO, err := c.CreateDatabaseWithRetry(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := respLRO.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The requests should be:
	// 1. CreateDatabase
	// 2. ListDatabaseOperations
	// 3. GetOperation (poll, done=False)
	// 4. GetOperation (poll, done=True)
	if want, got := request, mockDatabaseAdmin.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("wrong request %q, want %q", got, want)
	}
	listReq := &database.ListDatabaseOperationsRequest{
		Parent: "projects/[PROJECT]/instances/[INSTANCE]",
		Filter: fmt.Sprintf("(metadata.@type:type.googleapis.com/google.spanner.admin.database.v1.CreateDatabaseMetadata) AND (name:projects/[PROJECT]/instances/[INSTANCE]/databases/%s/operations/)", name),
	}
	if want, got := listReq, mockDatabaseAdmin.reqs[1]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	getReq := &longrunningpb.GetOperationRequest{
		Name: fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
	}
	if want, got := getReq, mockOperations.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	if want, got := getReq, mockOperations.reqs[1]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}

	// The end result for the user should just be the database.
	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("request mismatch:\nGot: %q\nWant: %q)", got, want)
	}
}

func Test_CreateDatabaseWithRetry_Unavailable_ServerReceivedRequest_OperationFinished(t *testing.T) {
	// Initialize a mock operations server as we will need to list long-running
	// operations.
	initMockOperations()
	// Use a specific test retry that will ensure that consecutive calls to the
	// mock server will return different answers.
	originalRetryer := retryer
	defer func() { retryer = originalRetryer }()

	// Set up mockDatabaseAdmin to return an Unavailable error for
	// CreateDatabase.
	var name string = "name3373707"
	errs := []error{gstatus.Error(codes.Unavailable, "test error")}
	mockDatabaseAdmin.err = errs[0]
	retryer = &testRetryer{f: func(err error) (pause time.Duration, shouldRetry bool) {
		code := gstatus.Code(err)
		if code == codes.Unavailable || code == codes.DeadlineExceeded {
			// Pop the errors from the stack to prevent the same error from
			// being returned each time.
			if len(errs) > 1 {
				mockDatabaseAdmin.err = errs[0]
				errs = errs[1:]
			} else {
				mockDatabaseAdmin.err = nil
			}
			return time.Millisecond, true
		}
		return 0, false
	}}

	var expectedResponse = &database.Database{
		Name: name,
	}
	any, err := ptypes.MarshalAny(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}
	// Set up the mockDatabaseAdmin to return a long-running operation that was
	// created by the initial CreateDatabase call.
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &database.ListDatabaseOperationsResponse{
		Operations: []*longrunningpb.Operation{
			{
				Name:   fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
				Done:   true,
				Result: &longrunningpb.Operation_Response{Response: any},
			},
		},
		NextPageToken: "",
	})
	// Append the actual database as the next response for the GetDatabase call.
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps, expectedResponse)
	// Append a long-running operation that will return the database.
	mockOperations.resps = append(mockOperations.resps, &longrunningpb.Operation{
		Name:   fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: any},
	})

	mockDatabaseAdmin.reqs = nil
	var formattedParent string = fmt.Sprintf("projects/%s/instances/%s", "[PROJECT]", "[INSTANCE]")
	var createStatement string = fmt.Sprintf("CREATE DATABASE %s", name)
	var request = &database.CreateDatabaseRequest{
		Parent:          formattedParent,
		CreateStatement: createStatement,
	}

	c, err := NewDatabaseAdminClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}
	// Set the operations client manually.
	c.LROClient, err = longrunning.NewOperationsClient(context.Background(), operationsClientOpt)
	if err != nil {
		t.Fatal(err)
	}

	respLRO, err := c.CreateDatabaseWithRetry(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := respLRO.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The requests should be:
	// 1. CreateDatabase
	// 2. ListDatabaseOperations
	// 3. GetDatabase
	// 4. GetOperation (poll, done=True)
	if want, got := request, mockDatabaseAdmin.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("wrong request %q, want %q", got, want)
	}
	listReq := &database.ListDatabaseOperationsRequest{
		Parent: "projects/[PROJECT]/instances/[INSTANCE]",
		Filter: fmt.Sprintf("(metadata.@type:type.googleapis.com/google.spanner.admin.database.v1.CreateDatabaseMetadata) AND (name:projects/[PROJECT]/instances/[INSTANCE]/databases/%s/operations/)", name),
	}
	if want, got := listReq, mockDatabaseAdmin.reqs[1]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	getDbReq := &database.GetDatabaseRequest{
		Name: fmt.Sprintf("projects/[PROJECT]/instances/[INSTANCE]/databases/%s", name),
	}
	if want, got := getDbReq, mockDatabaseAdmin.reqs[2]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	getReq := &longrunningpb.GetOperationRequest{
		Name: fmt.Sprintf("projects/p/instances/i/databases/%s/operations/1", name),
	}
	if want, got := getReq, mockOperations.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}

	// The end result for the user should just be the database.
	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("request mismatch:\nGot: %q\nWant: %q)", got, want)
	}
}

func Test_CreateDatabaseWithRetry_Unavailable_ServerDidNotReceiveRequest(t *testing.T) {
	// Initialize a mock operations server as we will need to list long-running
	// operations.
	initMockOperations()
	// Use a specific test retry that will ensure that consecutive calls to the
	// mock server will return different answers.
	originalRetryer := retryer
	defer func() { retryer = originalRetryer }()

	var name string = "name3373707"
	var expectedResponse = &database.Database{
		Name: name,
	}
	any, err := ptypes.MarshalAny(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}

	// Set up mockDatabaseAdmin to return an Unavailable error for
	// CreateDatabase.
	errs := []error{gstatus.Error(codes.Unavailable, "test error")}
	mockDatabaseAdmin.err = errs[0]
	retryer = &testRetryer{f: func(err error) (pause time.Duration, shouldRetry bool) {
		code := gstatus.Code(err)
		if code == codes.Unavailable || code == codes.DeadlineExceeded {
			// Pop the errors from the stack to prevent the same error from
			// being returned each time.
			if len(errs) > 1 {
				mockDatabaseAdmin.err = errs[0]
				errs = errs[1:]
			} else {
				mockDatabaseAdmin.err = nil
			}
			return time.Millisecond, true
		}
		return 0, false
	}}

	// Set up the mockDatabaseAdmin to return an empty list of operations.
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &database.ListDatabaseOperationsResponse{
		Operations:    []*longrunningpb.Operation{},
		NextPageToken: "",
	})
	// The next call should succeed directly.
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps, &longrunningpb.Operation{
		Name:   "longrunning-test",
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: any},
	})

	mockDatabaseAdmin.reqs = nil
	var formattedParent string = fmt.Sprintf("projects/%s/instances/%s", "[PROJECT]", "[INSTANCE]")
	var createStatement string = fmt.Sprintf("CREATE DATABASE %s", name)
	var request = &database.CreateDatabaseRequest{
		Parent:          formattedParent,
		CreateStatement: createStatement,
	}

	c, err := NewDatabaseAdminClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}
	// Set the operations client manually.
	c.LROClient, err = longrunning.NewOperationsClient(context.Background(), operationsClientOpt)
	if err != nil {
		t.Fatal(err)
	}

	respLRO, err := c.CreateDatabaseWithRetry(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := respLRO.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The requests should be:
	// 1. CreateDatabase
	// 2. ListDatabaseOperations
	// 3. CreateDatabase
	if want, got := request, mockDatabaseAdmin.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	listReq := &database.ListDatabaseOperationsRequest{
		Parent: "projects/[PROJECT]/instances/[INSTANCE]",
		Filter: fmt.Sprintf("(metadata.@type:type.googleapis.com/google.spanner.admin.database.v1.CreateDatabaseMetadata) AND (name:projects/[PROJECT]/instances/[INSTANCE]/databases/%s/operations/)", name),
	}
	if want, got := listReq, mockDatabaseAdmin.reqs[1]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}
	if want, got := request, mockDatabaseAdmin.reqs[2]; !proto.Equal(want, got) {
		t.Errorf("request mismatch\nGot: %q\nWant %q\n", got, want)
	}

	// The end result for the user should just be the database.
	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("request mismatch:\nGot: %q\nWant: %q)", got, want)
	}
}
