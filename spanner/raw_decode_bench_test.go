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
	"fmt"
	"io"
	"log"
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
