// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"net"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// TestVTProtobufSpannerClient verifies that the Spanner client configured
// with vtprotobuf correctly communicates with Spanner and parses results.
func TestVTProtobufSpannerClient(t *testing.T) {
	ctx := context.Background()

	// 1. Verify that the generated spannerpb structs implement UnmarshalVT and memory pooling
	var pr sppb.PartialResultSet
	if _, ok := any(&pr).(interface{ UnmarshalVT([]byte) error }); !ok {
		t.Fatalf("Expected *spannerpb.PartialResultSet to implement UnmarshalVT, but it does not")
	}
	if _, ok := any(&pr).(interface{ ResetVT() }); !ok {
		t.Fatalf("Expected *spannerpb.PartialResultSet to implement ResetVT, but it does not")
	}
	if _, ok := any(&pr).(interface{ ReturnToVTPool() }); !ok {
		t.Fatalf("Expected *spannerpb.PartialResultSet to implement ReturnToVTPool, but it does not")
	}

	// Test acquiring from pool, resetting, and returning to pool
	pooledPR := sppb.PartialResultSetFromVTPool()
	if pooledPR == nil {
		t.Fatalf("Expected non-nil PartialResultSet from VTPool")
	}
	pooledPR.ResumeToken = []byte("test_token")
	pooledPR.ReturnToVTPool()

	// 2. Set up the in-memory Spanner mock server
	mockServer := testutil.NewInMemSpannerServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, mockServer)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	// 3. Register mock query result
	sql := "SELECT 1 AS col_int, 'Hello from vtprotobuf' AS col_str, CURRENT_TIMESTAMP() AS col_ts"
	expectedTime := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	rowType := &sppb.StructType{
		Fields: []*sppb.StructType_Field{
			{Name: "col_int", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			{Name: "col_str", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "col_ts", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
		},
	}
	mockRows := []*structpb.ListValue{
		{
			Values: []*structpb.Value{
				structpb.NewStringValue("1"),
				structpb.NewStringValue("Hello from vtprotobuf"),
				structpb.NewStringValue(expectedTime.Format(time.RFC3339Nano)),
			},
		},
	}
	res := &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: rowType},
		Rows:     mockRows,
	}
	mockServer.PutStatementResult(sql, &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: res,
	})

	// 4. Create the Spanner client with vtprotobuf codec enabled
	client, err := NewHighThroughputSpannerClient(
		ctx,
		"projects/test-project/instances/test-instance/databases/test-db",
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 5. Execute query and verify results
	iter := client.Single().Query(ctx, spanner.Statement{SQL: sql})
	defer iter.Stop()

	count := 0
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Query execution failed: %v", err)
		}
		count++

		var colInt int64
		var colStr string
		var colTs time.Time

		if err := row.Columns(&colInt, &colStr, &colTs); err != nil {
			t.Fatalf("Failed to decode row columns: %v", err)
		}

		if colInt != 1 {
			t.Errorf("colInt mismatch: got %d, want 1", colInt)
		}
		if colStr != "Hello from vtprotobuf" {
			t.Errorf("colStr mismatch: got %q, want 'Hello from vtprotobuf'", colStr)
		}
		if !colTs.Equal(expectedTime) {
			t.Errorf("colTs mismatch: got %v, want %v", colTs, expectedTime)
		}
	}

	if count != 1 {
		t.Fatalf("Expected 1 row, got %d", count)
	}
}
