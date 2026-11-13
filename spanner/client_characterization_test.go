/*
Copyright 2026 Google LLC

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
	"io"
	"log"
	"net"
	"testing"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

func TestNewClientWithConfig_Characterization(t *testing.T) {
	t.Run("static pool default and custom NumChannels and conflict check", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			DisableNativeMetrics: true,
		})
		defer teardown()
		if client.sc == nil || client.sc.connPool == nil {
			t.Fatal("expected non-nil sessionClient and connPool")
		}
		if got := client.sc.connPool.Num(); got != numChannels {
			t.Fatalf("expected default pool.Num()=%d, got %d", numChannels, got)
		}

		// Conflicting NumChannels and WithGRPCConnectionPool returns InvalidArgument.
		server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
		defer serverTeardown()
		badClient, err := NewClientWithConfig(
			context.Background(),
			"projects/p/instances/i/databases/d",
			ClientConfig{
				NumChannels:          4,
				DisableNativeMetrics: true,
			},
			append(opts, option.WithGRPCConnectionPool(6))...,
		)
		if err == nil {
			badClient.Close()
			t.Fatal("expected InvalidArgument when NumChannels=4 conflicts with WithGRPCConnectionPool(6)")
		}
		if ErrCode(err) != codes.InvalidArgument {
			t.Fatalf("expected codes.InvalidArgument, got %v", err)
		}
		_ = server
	})

	t.Run("DCP enabled vs bypassed on SPANNER_EMULATOR_HOST", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		_, dcpClient, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			DisableNativeMetrics: true,
			DynamicChannelPoolConfig: DynamicChannelPoolConfig{
				DCPEnabled:         true,
				DCPInitialChannels: 3,
				DCPMinChannels:     2,
				DCPMaxChannels:     6,
			},
		})
		defer teardown()
		if dcpClient.sc.dynamicPool == nil {
			t.Fatal("expected dynamicPool to be initialized when DCPEnabled=true and emulator unset")
		}
		if got := dcpClient.sc.dynamicPool.Num(); got != 3 {
			t.Fatalf("expected 3 DCP channels, got %d", got)
		}

		// When SPANNER_EMULATOR_HOST is set, DCP is bypassed and emulator scheme is stripped.
		server, _, serverTeardown := NewMockedSpannerInMemTestServer(t)
		defer serverTeardown()
		for _, scheme := range []string{"", "http://", "https://", "passthrough:///"} {
			t.Setenv("SPANNER_EMULATOR_HOST", scheme+server.ServerAddress)
			emuClient, err := NewClientWithConfig(
				context.Background(),
				"projects/p/instances/i/databases/d",
				ClientConfig{
					DisableNativeMetrics: true,
					DynamicChannelPoolConfig: DynamicChannelPoolConfig{
						DCPEnabled: true,
					},
				},
			)
			if err != nil {
				t.Fatalf("NewClientWithConfig with SPANNER_EMULATOR_HOST(%q) failed: %v", scheme, err)
			}
			if emuClient.sc.dynamicPool != nil {
				emuClient.Close()
				t.Fatalf("expected DCP to be disabled when SPANNER_EMULATOR_HOST(%q) is set", scheme)
			}
			iter := emuClient.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					iter.Stop()
					emuClient.Close()
					t.Fatalf("query via SPANNER_EMULATOR_HOST(%q) failed: %v", scheme, err)
				}
			}
			iter.Stop()
			emuClient.Close()
		}
	})

	t.Run("compression, end-to-end tracing, user-agent, session labels, and DirectPath env", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_ENABLE_END_TO_END_TRACING", "true")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "false")

		_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			Compression:   "gzip",
			UserAgent:     "custom-agent/2.0",
			SessionLabels: map[string]string{"env": "test"},
		})
		defer teardown()

		if got := client.sc.md.Get(requestsCompressionHeader); len(got) != 1 || got[0] != "gzip" {
			t.Fatalf("expected %s=[gzip] in sc.md, got %v", requestsCompressionHeader, got)
		}
		if got := client.sc.md.Get(endToEndTracingHeader); len(got) != 1 || got[0] != "true" {
			t.Fatalf("expected %s=[true] in sc.md, got %v", endToEndTracingHeader, got)
		}
		if client.sc.sessionLabels["env"] != "test" {
			t.Fatalf("expected session label env=test, got %v", client.sc.sessionLabels)
		}
		iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
		defer iter.Stop()
		rowCount := 0
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("unexpected query error: %v", err)
			}
			rowCount++
		}
		if rowCount != 2 {
			t.Fatalf("expected 2 rows from SelectFooFromBar, got %d", rowCount)
		}
	})
}

func BenchmarkPureGo_Query(b *testing.B) {
	testSpanner := NewInMemSpannerServer()
	_ = testSpanner.PutStatementResult(SelectFooFromBar, &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{Name: "FOO", Type: &spannerpb.Type{Code: spannerpb.TypeCode_INT64}},
					},
				},
			},
			Rows: []*proto3.ListValue{
				{Values: []*proto3.Value{{Kind: &proto3.Value_StringValue{StringValue: "1"}}}},
				{Values: []*proto3.Value{{Kind: &proto3.Value_StringValue{StringValue: "2"}}}},
			},
		},
	})
	srv := grpc.NewServer()
	spannerpb.RegisterSpannerServer(srv, testSpanner)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		testSpanner.Stop()
		srv.Stop()
		_ = lis.Close()
	}()

	ctx := context.Background()
	client, err := NewClientWithConfig(
		ctx,
		"projects/p/instances/i/databases/d",
		ClientConfig{
			DisableNativeMetrics: true,
			Logger:               log.New(io.Discard, "", 0),
		},
		option.WithEndpoint(lis.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	stmt := NewStatement(SelectFooFromBar)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := client.Single().Query(ctx, stmt)
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
		iter.Stop()
	}
}
