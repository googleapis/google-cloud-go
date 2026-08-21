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
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestCustomCodecSpannerClient(t *testing.T) {
	ctx := context.Background()

	// 1. Set up the in-memory Spanner mock server
	mockServer := testutil.NewInMemSpannerServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, mockServer)
	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	// 2. Register mock query result
	sql := "SELECT 1 AS col_int, 'Hello from custom codec' AS col_str, CURRENT_TIMESTAMP() AS col_ts"
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
				structpb.NewStringValue("Hello from custom codec"),
				structpb.NewStringValue(expectedTime.Format(time.RFC3339Nano)),
			},
		},
	}
	resultSet := &sppb.ResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: rowType},
		Rows:     mockRows,
	}
	mockServer.PutStatementResult(sql, &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: resultSet,
	})

	// 3. Create client configured with CustomSpannerCodec and CustomPartialResultSetPool
	client, err := NewCustomOptimizedSpannerClient(
		ctx,
		"projects/p/instances/i/databases/d",
		option.WithEndpoint(listener.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 4. Run query and verify decoded rows
	statement := spanner.Statement{SQL: sql}
	iter := client.Single().Query(ctx, statement)
	defer iter.Stop()

	row, err := iter.Next()
	if err != nil {
		t.Fatalf("Failed to read row: %v", err)
	}

	var colInt int64
	var colStr string
	var colTs time.Time
	if err := row.Columns(&colInt, &colStr, &colTs); err != nil {
		t.Fatalf("Failed to extract columns: %v", err)
	}

	if colInt != 1 {
		t.Errorf("col_int = %d, want 1", colInt)
	}
	if colStr != "Hello from custom codec" {
		t.Errorf("col_str = %q, want 'Hello from custom codec'", colStr)
	}
	if !colTs.Equal(expectedTime) {
		t.Errorf("col_ts = %v, want %v", colTs, expectedTime)
	}

	_, err = iter.Next()
	if err != iterator.Done {
		t.Errorf("Expected iterator.Done, got %v", err)
	}
}

type spyCustomCodec struct {
	SpannerFastCodec
	unmarshalCalls int
	marshalCalls   int
}

func (s *spyCustomCodec) Unmarshal(data []byte, value any) error {
	s.unmarshalCalls++
	return s.SpannerFastCodec.Unmarshal(data, value)
}

func (s *spyCustomCodec) Marshal(value any) ([]byte, error) {
	s.marshalCalls++
	return s.SpannerFastCodec.Marshal(value)
}

func TestCustomCodecIsActuallyCalled(t *testing.T) {
	ctx := context.Background()

	mockServer := testutil.NewInMemSpannerServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, mockServer)
	go grpcServer.Serve(listener)
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

	spy := &spyCustomCodec{}
	client, err := spanner.NewClient(
		ctx,
		"projects/test-project/instances/test-instance/databases/test-db",
		option.WithEndpoint(listener.Addr().String()),
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
		t.Fatalf("Expected custom codec Unmarshal to be called at least once, got 0")
	}
	if spy.marshalCalls == 0 {
		t.Fatalf("Expected custom codec Marshal to be called at least once, got 0")
	}
}

func TestDirectFastUnmarshalPartialResultSet(t *testing.T) {
	chunk := &sppb.PartialResultSet{
		Values: []*structpb.Value{
			structpb.NewStringValue("test-string"),
			structpb.NewNumberValue(42.5),
			structpb.NewBoolValue(true),
			structpb.NewNullValue(),
		},
		ResumeToken:  []byte("token-12345"),
		ChunkedValue: true,
	}

	wireBytes, err := proto.Marshal(chunk)
	if err != nil {
		t.Fatalf("Failed to marshal sample chunk: %v", err)
	}

	var target sppb.PartialResultSet
	if err := FastUnmarshalPartialResultSet(wireBytes, &target); err != nil {
		t.Fatalf("FastUnmarshalPartialResultSet failed: %v", err)
	}

	if len(target.Values) != 4 {
		t.Fatalf("Values length mismatch: got %d, want 4", len(target.Values))
	}
	if target.Values[0].GetStringValue() != "test-string" {
		t.Errorf("Values[0] = %q, want 'test-string'", target.Values[0].GetStringValue())
	}
	if target.Values[1].GetNumberValue() != 42.5 {
		t.Errorf("Values[1] = %v, want 42.5", target.Values[1].GetNumberValue())
	}
	if !target.Values[2].GetBoolValue() {
		t.Errorf("Values[2] = false, want true")
	}
	if _, ok := target.Values[3].Kind.(*structpb.Value_NullValue); !ok {
		t.Errorf("Values[3] is not NullValue")
	}
	if string(target.ResumeToken) != "token-12345" {
		t.Errorf("ResumeToken = %q, want 'token-12345'", string(target.ResumeToken))
	}
	if !target.ChunkedValue {
		t.Errorf("ChunkedValue = false, want true")
	}
}

func helperCreateBenchmarkData(numRows int) (*sppb.ResultSetMetadata, []*structpb.ListValue) {
	rowType := &sppb.StructType{
		Fields: []*sppb.StructType_Field{
			{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			{Name: "amount", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
			{Name: "active", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
			{Name: "description", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		},
	}
	metadata := &sppb.ResultSetMetadata{RowType: rowType}

	var mockRows []*structpb.ListValue
	for i := 0; i < numRows; i++ {
		mockRows = append(mockRows, &structpb.ListValue{
			Values: []*structpb.Value{
				structpb.NewStringValue(fmt.Sprintf("item-%06d", i)),
				structpb.NewNumberValue(float64(i) * 1.5),
				structpb.NewBoolValue(i%2 == 0),
				structpb.NewStringValue("Description text representing query payload for performance evaluation"),
			},
		})
	}
	return metadata, mockRows
}

func BenchmarkSpannerClient(b *testing.B) {
	ctx := context.Background()
	numRows := 1000
	metadata, mockRows := helperCreateBenchmarkData(numRows)

	mockServer := testutil.NewInMemSpannerServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	grpcServer := grpc.NewServer()
	sppb.RegisterSpannerServer(grpcServer, mockServer)
	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	sql := "SELECT * FROM large_table"
	mockServer.PutStatementResult(sql, &testutil.StatementResult{
		Type: testutil.StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: metadata,
			Rows:     mockRows,
		},
	})

	baseOptions := []option.ClientOption{
		option.WithEndpoint(listener.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}

	// 1. Standard Client (Standard Protobuf runtime, reflection-based, no pooling)
	clientStd, err := spanner.NewClient(ctx, "projects/p/instances/i/databases/d", baseOptions...)
	if err != nil {
		b.Fatalf("Failed to create std client: %v", err)
	}
	defer clientStd.Close()

	// 2. Custom Optimized Client (Custom fast-path codec + memory pooling)
	clientCustom, err := NewCustomOptimizedSpannerClient(ctx, "projects/p/instances/i/databases/d", baseOptions...)
	if err != nil {
		b.Fatalf("Failed to create custom client: %v", err)
	}
	defer clientCustom.Close()

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
				var id, description string
				var amount float64
				var active bool
				if err := row.Columns(&id, &amount, &active, &description); err != nil {
					b.Fatal(err)
				}
			}
			iter.Stop()
		}
	})

	b.Run("CustomCodec_With_Pooling", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			iter := clientCustom.Single().Query(ctx, spanner.Statement{SQL: sql})
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					b.Fatal(err)
				}
				var id, description string
				var amount float64
				var active bool
				if err := row.Columns(&id, &amount, &active, &description); err != nil {
					b.Fatal(err)
				}
			}
			iter.Stop()
		}
	})
}
