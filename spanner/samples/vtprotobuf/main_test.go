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
	"fmt"
	"net"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/testutil"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
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

type spyCodec struct {
	vtgrpc.Codec
	unmarshalCalls int
	marshalCalls   int
}

func (s *spyCodec) Unmarshal(data []byte, v any) error {
	s.unmarshalCalls++
	return s.Codec.Unmarshal(data, v)
}

func (s *spyCodec) Marshal(v any) ([]byte, error) {
	s.marshalCalls++
	return s.Codec.Marshal(v)
}

func TestVTProtobufCodecIsActuallyCalled(t *testing.T) {
	ctx := context.Background()

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

	sql := "SELECT 1 AS num"
	mockServer.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				RowType: &sppb.StructType{
					Fields: []*sppb.StructType_Field{
						{Name: "num", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("1")}},
			},
		},
	})

	spy := &spyCodec{}
	client, err := spanner.NewClient(
		ctx,
		"projects/test-project/instances/test-instance/databases/test-db",
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.ForceCodec(spy))),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	iter := client.Single().Query(ctx, spanner.Statement{SQL: sql})
	defer iter.Stop()

	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
	}

	t.Logf("Spy Codec Calls: Unmarshal=%d, Marshal=%d", spy.unmarshalCalls, spy.marshalCalls)
	if spy.unmarshalCalls == 0 {
		t.Fatalf("Expected vtprotobuf codec Unmarshal to be called at least once, but it was called 0 times!")
	}
	if spy.marshalCalls == 0 {
		t.Fatalf("Expected vtprotobuf codec Marshal to be called at least once, but it was called 0 times!")
	}
}

func helperCreateBenchmarkData(numRows int) (*sppb.ResultSetMetadata, []*structpb.ListValue, *sppb.PartialResultSet) {
	rowType := &sppb.StructType{
		Fields: []*sppb.StructType_Field{
			{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "amount", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
			{Name: "active", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
			{Name: "description", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		},
	}
	metadata := &sppb.ResultSetMetadata{RowType: rowType}

	mockRows := make([]*structpb.ListValue, 0, numRows)
	values := make([]*structpb.Value, 0, numRows*4)

	for r := 0; r < numRows; r++ {
		v0 := structpb.NewStringValue(fmt.Sprintf("user_id_%d", r))
		v1 := structpb.NewNumberValue(float64(r * 10))
		v2 := structpb.NewBoolValue(r%2 == 0)
		v3 := structpb.NewStringValue(fmt.Sprintf("sample_payload_data_string_for_user_row_%d", r))

		mockRows = append(mockRows, &structpb.ListValue{
			Values: []*structpb.Value{v0, v1, v2, v3},
		})
		values = append(values, v0, v1, v2, v3)
	}

	pr := &sppb.PartialResultSet{
		Metadata:    metadata,
		Values:      values,
		ResumeToken: []byte("sample_resume_token"),
	}

	return metadata, mockRows, pr
}

// BenchmarkSpannerClient_EndToEnd benchmarks full query execution and row decoding
// comparing standard protobuf vs vtprotobuf.
func BenchmarkSpannerClient_EndToEnd(b *testing.B) {
	ctx := context.Background()
	numRows := 1000
	metadata, mockRows, _ := helperCreateBenchmarkData(numRows)

	mockServer := testutil.NewInMemSpannerServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to listen: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, mockServer)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	sql := "SELECT * FROM large_table"
	mockServer.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: metadata,
			Rows:     mockRows,
		},
	})

	baseOpts := []option.ClientOption{
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}

	// 1. Standard Client
	clientStd, err := spanner.NewClient(ctx, "projects/p/instances/i/databases/d", baseOpts...)
	if err != nil {
		b.Fatalf("Failed to create std client: %v", err)
	}
	defer clientStd.Close()

	// 2. VTProtobuf Client
	clientVT, err := NewHighThroughputSpannerClient(ctx, "projects/p/instances/i/databases/d", baseOpts...)
	if err != nil {
		b.Fatalf("Failed to create vt client: %v", err)
	}
	defer clientVT.Close()

	b.Run("Standard_Protobuf", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			iter := clientStd.Single().Query(ctx, spanner.Statement{SQL: sql})
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					b.Fatal(err)
				}
				var id, desc string
				var amount float64
				var active bool
				if err := row.Columns(&id, &amount, &active, &desc); err != nil {
					b.Fatal(err)
				}
			}
			iter.Stop()
		}
	})

	b.Run("VTProtobuf_Codec", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			iter := clientVT.Single().Query(ctx, spanner.Statement{SQL: sql})
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					b.Fatal(err)
				}
				var id, desc string
				var amount float64
				var active bool
				if err := row.Columns(&id, &amount, &active, &desc); err != nil {
					b.Fatal(err)
				}
			}
			iter.Stop()
		}
	})
}

// BenchmarkUnmarshal_PartialResultSet benchmarks low-level protobuf deserialization
// comparing standard proto.Unmarshal vs vtprotobuf UnmarshalVT on Spanner result sets.
func BenchmarkUnmarshal_PartialResultSet(b *testing.B) {
	_, _, pr := helperCreateBenchmarkData(1000)
	rawBytes, err := proto.Marshal(pr)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("Standard_proto.Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(rawBytes)))
		for i := 0; i < b.N; i++ {
			var target sppb.PartialResultSet
			if err := proto.Unmarshal(rawBytes, &target); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("VTProtobuf_UnmarshalVT", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(rawBytes)))
		for i := 0; i < b.N; i++ {
			var target sppb.PartialResultSet
			if err := target.UnmarshalVT(rawBytes); err != nil {
				b.Fatal(err)
			}
		}
	})
}

