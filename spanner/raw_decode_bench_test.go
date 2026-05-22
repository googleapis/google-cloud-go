/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

const rawBenchSQL = "SELECT id, metadata, status FROM CustomerShape"

func setupRawBenchClient(b *testing.B, rows, payloadLen int) (*Client, func()) {
	b.Helper()
	backend := NewInMemSpannerServer()
	instance := NewInMemInstanceAdminServer()
	server := grpc.NewServer()
	sppb.RegisterSpannerServer(server, backend)
	// Instance admin is not needed by this benchmark.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = server.Serve(lis) }()

	fields := []*sppb.StructType_Field{
		{Name: "id", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		{Name: "metadata", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		{Name: "status", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
	}
	payload := strings.Repeat("x", payloadLen)
	rsRows := make([]*structpb.ListValue, rows)
	for i := 0; i < rows; i++ {
		rsRows[i] = &structpb.ListValue{Values: []*structpb.Value{
			{Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("id-%d", i)}},
			{Kind: &structpb.Value_StringValue{StringValue: payload}},
			{Kind: &structpb.Value_StringValue{StringValue: "ready"}},
		}}
	}
	if err := backend.PutStatementResult(rawBenchSQL, &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
			Rows:     rsRows,
		},
		SetLastFlag: true,
	}); err != nil {
		b.Fatal(err)
	}

	logger := log.Default()
	logger.SetOutput(io.Discard)
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true, Logger: logger},
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		b.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		client.sm.mu.Lock()
		ready := client.sm.multiplexedSession != nil
		client.sm.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("timeout waiting for multiplexed session")
		}
		time.Sleep(time.Millisecond)
	}
	return client, func() {
		client.Close()
		backend.Stop()
		instance.Stop()
		server.Stop()
	}
}

func BenchmarkCustomerShapePublicQuery(b *testing.B) {
	useRaw := os.Getenv("SPANNER_RAW_BENCH") == "1"
	client, teardown := setupRawBenchClient(b, 100, 1600)
	defer teardown()
	ctx := context.Background()
	stmt := NewStatement(rawBenchSQL)
	var total int
	buf := make([]byte, 0, 1600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ExperimentalRawDecode: useRaw})
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
			buf = buf[:0]
			if useRaw {
				buf, err = row.ColumnBytes(1, buf)
			} else {
				var s string
				err = row.Column(1, &s)
				buf = append(buf, s...)
			}
			if err != nil {
				b.Fatal(err)
			}
			total += len(buf)
		}
		iter.Stop()
	}
	if total == 0 {
		b.Fatal(total)
	}
}

const rawExactShapeBenchSQL = "SELECT exact shape"

func setupExactShapeBenchClient(b *testing.B, rows int) (*Client, func()) {
	b.Helper()
	backend := NewInMemSpannerServer()
	instance := NewInMemInstanceAdminServer()
	server := grpc.NewServer()
	sppb.RegisterSpannerServer(server, backend)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = server.Serve(lis) }()

	fields := []*sppb.StructType_Field{
		{Name: "random_bool", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
		{Name: "random_bytes", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}},
		{Name: "random_date", Type: &sppb.Type{Code: sppb.TypeCode_DATE}},
		{Name: "random_float32", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}},
		{Name: "random_float64", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
		{Name: "random_interval", Type: &sppb.Type{Code: sppb.TypeCode_INTERVAL}},
		{Name: "random_json", Type: &sppb.Type{Code: sppb.TypeCode_JSON}},
		{Name: "random_int64", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
		{Name: "random_numeric", Type: &sppb.Type{Code: sppb.TypeCode_NUMERIC}},
		{Name: "random_string", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		{Name: "random_timestamp", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
		{Name: "random_uuid", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
	}
	bytesVal := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	rsRows := make([]*structpb.ListValue, rows)
	for i := 0; i < rows; i++ {
		rsRows[i] = &structpb.ListValue{Values: []*structpb.Value{
			{Kind: &structpb.Value_BoolValue{BoolValue: i%2 == 0}},
			{Kind: &structpb.Value_StringValue{StringValue: bytesVal}},
			{Kind: &structpb.Value_StringValue{StringValue: "2024-01-02"}},
			{Kind: &structpb.Value_NumberValue{NumberValue: 1.25}},
			{Kind: &structpb.Value_NumberValue{NumberValue: 2.5}},
			{Kind: &structpb.Value_StringValue{StringValue: "P1Y2M3DT4H5M6S"}},
			{Kind: &structpb.Value_StringValue{StringValue: `{"key":"value"}`}},
			{Kind: &structpb.Value_StringValue{StringValue: "123456789"}},
			{Kind: &structpb.Value_StringValue{StringValue: "123.456"}},
			{Kind: &structpb.Value_StringValue{StringValue: "9f04d5de-c169-4c7d-93df-8a1c54c17b2f"}},
			{Kind: &structpb.Value_StringValue{StringValue: "2024-01-02T03:04:05Z"}},
			{Kind: &structpb.Value_StringValue{StringValue: "d4c3b2a1-1111-2222-3333-444455556666"}},
		}}
	}
	if err := backend.PutStatementResult(rawExactShapeBenchSQL, &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
			Rows:     rsRows,
		},
		SetLastFlag: true,
	}); err != nil {
		b.Fatal(err)
	}

	logger := log.Default()
	logger.SetOutput(io.Discard)
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true, Logger: logger},
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		b.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		client.sm.mu.Lock()
		ready := client.sm.multiplexedSession != nil
		client.sm.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("timeout waiting for multiplexed session")
		}
		time.Sleep(time.Millisecond)
	}
	return client, func() {
		client.Close()
		backend.Stop()
		instance.Stop()
		server.Stop()
	}
}

func BenchmarkReadLargeResultSetExactShapePublicQuery(b *testing.B) {
	useRaw := os.Getenv("SPANNER_RAW_BENCH") == "1"
	client, teardown := setupExactShapeBenchClient(b, 100)
	defer teardown()
	ctx := context.Background()
	stmt := NewStatement(rawExactShapeBenchSQL)
	var total int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ExperimentalRawDecode: useRaw})
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
			var randomBool bool
			var randomBytes []byte
			var randomDate NullDate
			var randomFloat32 float32
			var randomFloat64 float64
			var randomInterval GenericColumnValue
			var randomJSON NullJSON
			var randomInt64 int64
			var randomNumeric big.Rat
			var randomString string
			var randomTimestamp time.Time
			var randomUUID string

			if err := row.Column(0, &randomBool); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(1, &randomBytes); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(2, &randomDate); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(3, &randomFloat32); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(4, &randomFloat64); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(5, &randomInterval); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(6, &randomJSON); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(7, &randomInt64); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(8, &randomNumeric); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(9, &randomString); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(10, &randomTimestamp); err != nil {
				b.Fatal(err)
			}
			if err := row.Column(11, &randomUUID); err != nil {
				b.Fatal(err)
			}
			total += len(randomBytes) + len(randomString) + len(randomUUID)
		}
		iter.Stop()
	}
	if total == 0 {
		b.Fatal(total)
	}
}
