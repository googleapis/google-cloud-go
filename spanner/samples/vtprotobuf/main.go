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

// Package main demonstrates how to use the high-performance vtprotobuf codec
// with the official Google Cloud Spanner Go client.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// VTPartialResultSetPool adapts vtprotobuf's generated pool to spanner.PartialResultSetPool.
type VTPartialResultSetPool struct{}

func (p VTPartialResultSetPool) Get() *sppb.PartialResultSet {
	return sppb.PartialResultSetFromVTPool()
}

func (p VTPartialResultSetPool) Put(m *sppb.PartialResultSet) {
	if m != nil {
		m.ReturnToVTPool()
	}
}

// NewHighThroughputSpannerClient creates a standard Spanner client configured
// with the high-performance vtprotobuf gRPC codec and memory pooler.
func NewHighThroughputSpannerClient(ctx context.Context, database string, opts ...option.ClientOption) (*spanner.Client, error) {
	vtprotoOpt := option.WithGRPCDialOption(
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(vtgrpc.Codec{}),
		),
	)

	allOpts := append([]option.ClientOption{vtprotoOpt}, opts...)
	return spanner.NewClientWithConfig(ctx, database,
		spanner.ClientConfig{
			SessionPoolConfig:    spanner.DefaultSessionPoolConfig,
			PartialResultSetPool: VTPartialResultSetPool{},
		},
		allOpts...,
	)
}

func main() {
	ctx := context.Background()

	database := os.Getenv("SPANNER_DATABASE")
	if database == "" {
		database = "projects/my-project/instances/my-instance/databases/my-database"
	}

	fmt.Printf("Initializing Spanner client with vtprotobuf codec for: %s\n", database)

	client, err := NewHighThroughputSpannerClient(ctx, database)
	if err != nil {
		log.Fatalf("Failed to create Spanner client: %v", err)
	}
	defer client.Close()

	// Use standard Spanner client operations normally:
	stmt := spanner.Statement{
		SQL: "SELECT 1 AS col_int, 'Hello from vtprotobuf' AS col_str, CURRENT_TIMESTAMP() AS col_ts",
	}

	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}

		var colInt int64
		var colStr string
		var colTs time.Time

		if err := row.Columns(&colInt, &colStr, &colTs); err != nil {
			log.Fatalf("Failed to read columns: %v", err)
		}

		fmt.Printf("Row received -> col_int: %d, col_str: %q, col_ts: %v\n", colInt, colStr, colTs)
	}

	fmt.Println("Done! Streaming query decoded successfully using vtprotobuf.")
}
