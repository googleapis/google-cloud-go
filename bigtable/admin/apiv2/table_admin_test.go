// Copyright 2026 Google LLC
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

package admin

import (
	"context"
	"fmt"
	"net"
	"testing"

	adminpb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/anypb"
)

type mockServer struct {
	adminpb.UnimplementedBigtableTableAdminServer
	longrunningpb.UnimplementedOperationsServer

	restoreTableFunc             func(context.Context, *adminpb.RestoreTableRequest) (*longrunningpb.Operation, error)
	getOperationFunc             func(context.Context, *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error)
	generateConsistencyTokenFunc func(context.Context, *adminpb.GenerateConsistencyTokenRequest) (*adminpb.GenerateConsistencyTokenResponse, error)
	checkConsistencyFunc         func(context.Context, *adminpb.CheckConsistencyRequest) (*adminpb.CheckConsistencyResponse, error)
}

func (m *mockServer) RestoreTable(ctx context.Context, req *adminpb.RestoreTableRequest) (*longrunningpb.Operation, error) {
	if m.restoreTableFunc != nil {
		return m.restoreTableFunc(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockServer) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
	if m.getOperationFunc != nil {
		return m.getOperationFunc(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockServer) GenerateConsistencyToken(ctx context.Context, req *adminpb.GenerateConsistencyTokenRequest) (*adminpb.GenerateConsistencyTokenResponse, error) {
	if m.generateConsistencyTokenFunc != nil {
		return m.generateConsistencyTokenFunc(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (m *mockServer) CheckConsistency(ctx context.Context, req *adminpb.CheckConsistencyRequest) (*adminpb.CheckConsistencyResponse, error) {
	if m.checkConsistencyFunc != nil {
		return m.checkConsistencyFunc(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func setupMockClient(t *testing.T, mock *mockServer) (*BigtableTableAdminClient, func()) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	s := grpc.NewServer()
	adminpb.RegisterBigtableTableAdminServer(s, mock)
	longrunningpb.RegisterOperationsServer(s, mock)

	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			panic(fmt.Sprintf("Server exited with error: %v", err))
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}

	client, err := NewBigtableTableAdminClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("Failed to create admin client: %v", err)
	}

	cleanup := func() {
		client.Close()
		s.Stop()
		lis.Close()
	}

	return client, cleanup
}

func TestRestoreTable_Success(t *testing.T) {
	mock := &mockServer{}
	const opName = "operations/restore-test-op"

	// 1. RestoreTable returns a pending LRO
	mock.restoreTableFunc = func(ctx context.Context, req *adminpb.RestoreTableRequest) (*longrunningpb.Operation, error) {
		return &longrunningpb.Operation{
			Name: opName,
			Done: false,
		}, nil
	}

	// 2. GetOperation returns Done: true on second call
	getCalls := 0
	mock.getOperationFunc = func(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
		if req.GetName() != opName {
			return nil, status.Errorf(codes.NotFound, "operation not found: %s", req.GetName())
		}
		getCalls++
		if getCalls < 2 {
			return &longrunningpb.Operation{
				Name: opName,
				Done: false,
			}, nil
		}

		table := &adminpb.Table{
			Name: "projects/p/instances/i/tables/restored-table",
		}
		anyTable, err := anypb.New(table)
		if err != nil {
			return nil, err
		}

		return &longrunningpb.Operation{
			Name: opName,
			Done: true,
			Result: &longrunningpb.Operation_Response{
				Response: anyTable,
			},
		}, nil
	}

	client, cleanup := setupMockClient(t, mock)
	defer cleanup()

	req := &adminpb.RestoreTableRequest{
		Parent:  "projects/p/instances/i",
		TableId: "restored-table",
	}

	err := client.RestoreTable(context.Background(), req)
	if err != nil {
		t.Fatalf("RestoreTable failed: %v", err)
	}

	if getCalls != 2 {
		t.Errorf("Expected 2 GetOperation calls, got %d", getCalls)
	}
}

func TestRestoreTable_Error(t *testing.T) {
	mock := &mockServer{}
	const opName = "operations/restore-test-op"

	mock.restoreTableFunc = func(ctx context.Context, req *adminpb.RestoreTableRequest) (*longrunningpb.Operation, error) {
		return &longrunningpb.Operation{
			Name: opName,
			Done: false,
		}, nil
	}

	mock.getOperationFunc = func(ctx context.Context, req *longrunningpb.GetOperationRequest) (*longrunningpb.Operation, error) {
		return &longrunningpb.Operation{
			Name: opName,
			Done: true,
			Result: &longrunningpb.Operation_Error{
				Error: status.New(codes.Aborted, "restore aborted by server").Proto(),
			},
		}, nil
	}

	client, cleanup := setupMockClient(t, mock)
	defer cleanup()

	req := &adminpb.RestoreTableRequest{
		Parent:  "projects/p/instances/i",
		TableId: "restored-table",
	}

	err := client.RestoreTable(context.Background(), req)
	if err == nil {
		t.Fatal("RestoreTable succeeded, wanted LRO failure error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Aborted {
		t.Errorf("RestoreTable error code: %v, want %v", st.Code(), codes.Aborted)
	}
}

func TestWaitForReplication_Success(t *testing.T) {
	mock := &mockServer{}
	const token = "test-consistency-token"

	mock.generateConsistencyTokenFunc = func(ctx context.Context, req *adminpb.GenerateConsistencyTokenRequest) (*adminpb.GenerateConsistencyTokenResponse, error) {
		return &adminpb.GenerateConsistencyTokenResponse{
			ConsistencyToken: token,
		}, nil
	}

	checkCalls := 0
	mock.checkConsistencyFunc = func(ctx context.Context, req *adminpb.CheckConsistencyRequest) (*adminpb.CheckConsistencyResponse, error) {
		if req.GetConsistencyToken() != token {
			return nil, status.Error(codes.InvalidArgument, "invalid token")
		}
		checkCalls++
		return &adminpb.CheckConsistencyResponse{
			Consistent: true,
		}, nil
	}

	client, cleanup := setupMockClient(t, mock)
	defer cleanup()

	err := client.WaitForReplication(context.Background(), "projects/p/instances/i/tables/t")
	if err != nil {
		t.Fatalf("WaitForReplication failed: %v", err)
	}

	if checkCalls != 1 {
		t.Errorf("Expected 1 CheckConsistency call, got %d", checkCalls)
	}
}

func TestWaitForReplication_ContextCancelled(t *testing.T) {
	mock := &mockServer{}
	const token = "test-consistency-token"

	ctx, cancel := context.WithCancel(context.Background())

	mock.generateConsistencyTokenFunc = func(ctx context.Context, req *adminpb.GenerateConsistencyTokenRequest) (*adminpb.GenerateConsistencyTokenResponse, error) {
		return &adminpb.GenerateConsistencyTokenResponse{
			ConsistencyToken: token,
		}, nil
	}

	// CheckConsistency returns Consistent: false, prompting a sleep/tick
	mock.checkConsistencyFunc = func(ctx context.Context, req *adminpb.CheckConsistencyRequest) (*adminpb.CheckConsistencyResponse, error) {
		cancel() // Cancel the context to trigger the select case
		return &adminpb.CheckConsistencyResponse{
			Consistent: false,
		}, nil
	}

	client, cleanup := setupMockClient(t, mock)
	defer cleanup()

	err := client.WaitForReplication(ctx, "projects/p/instances/i/tables/t")
	if err != context.Canceled && status.Code(err) != codes.Canceled {
		t.Fatalf("WaitForReplication error: %v, want %v or gRPC Canceled status", err, context.Canceled)
	}
}
