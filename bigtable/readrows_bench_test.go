/*
Copyright 2025 Google LLC

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

package bigtable

import (
	"context"
	"fmt"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/mockserver"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// makeResponseChunks returns a btpb.ReadRowsResponse with numRows rows,
// each containing one cell with valueSize bytes of value.
func makeResponseChunks(numRows, valueSize int) *btpb.ReadRowsResponse {
	chunks := make([]*btpb.ReadRowsResponse_CellChunk, 0, numRows)
	value := make([]byte, valueSize)
	for i := range value {
		value[i] = byte(i % 256)
	}
	for i := 0; i < numRows; i++ {
		chunks = append(chunks, &btpb.ReadRowsResponse_CellChunk{
			RowKey:          []byte(fmt.Sprintf("row-%08d", i)),
			FamilyName:      &wrapperspb.StringValue{Value: "cf"},
			Qualifier:       &wrapperspb.BytesValue{Value: []byte("col")},
			TimestampMicros: 1000,
			Value:           value,
			RowStatus:       &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
		})
	}
	return &btpb.ReadRowsResponse{Chunks: chunks}
}

// BenchmarkReadRowsDecode measures the end-to-end ReadRows decoding throughput
// through a local in-process gRPC server, exercising the vtproto UnmarshalVT path.
//
//	go test -run=^$ -bench=BenchmarkReadRowsDecode -benchmem ./...
func BenchmarkReadRowsDecode(b *testing.B) {
	const (
		rowsPerResponse = 100
		valueSize       = 128
	)

	resp := makeResponseChunks(rowsPerResponse, valueSize)

	srv, err := mockserver.NewServer("localhost:0")
	if err != nil {
		b.Fatal(err)
	}
	defer srv.Close()

	srv.ReadRowsFn = func(_ *btpb.ReadRowsRequest, s btpb.Bigtable_ReadRowsServer) error {
		return s.Send(resp)
	}

	conn, err := grpc.Dial(srv.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	c, err := NewClientWithConfig(context.Background(), "proj", "inst",
		disableMetricsConfig, option.WithGRPCConn(conn))
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	tbl := c.Open("tbl")
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	rowCount := 0
	for i := 0; i < b.N; i++ {
		err := tbl.ReadRows(ctx, PrefixRange("row-"), func(Row) bool {
			rowCount++
			return true
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(rowCount)/float64(b.N), "rows/op")
}
