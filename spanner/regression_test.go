// Copyright 2025 Google LLC
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

package spanner

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"cloud.google.com/go/spanner/internal/testutil"
)

type methodAndMetadata struct {
	method string
	md     metadata.MD
}

type ourInterceptor struct {
	unaryHeaders  []*methodAndMetadata
	streamHeaders []*methodAndMetadata
}

func (oi *ourInterceptor) interceptStream(srv any, ss grpc.ServerStream, ssi *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return errors.New("missing metadata in stream")
	}
	oi.streamHeaders = append(oi.streamHeaders, &methodAndMetadata{ssi.FullMethod, md})
	return handler(srv, ss)
}

func (oi *ourInterceptor) interceptUnary(ctx context.Context, req any, usi *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.New("missing metadata in unary")
	}
	oi.unaryHeaders = append(oi.unaryHeaders, &methodAndMetadata{usi.FullMethod, md})
	return handler(ctx, req)
}

// This is a regression test to assert that all the expected headers are propagated
// along to the final gRPC server avoiding scenarios where headers got dropped from a
// destructive context augmentation call.
// Please see https://github.com/googleapis/google-cloud-go/issues/11656
func TestAllHeadersForwardedAppropriately(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	// 1. Set up the server interceptor that'll record and collect
	// all the headers that  are received by the server.
	oint := new(ourInterceptor)
	sopts := []grpc.ServerOption{
		grpc.UnaryInterceptor(oint.interceptUnary), grpc.StreamInterceptor(oint.interceptStream),
	}
	mockedServer, clientOpts, teardown := makeMockServer(t, sopts...)
	defer teardown()

	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		EnableEndToEndTracing: true,
		DisableRouteToLeader:  false,
	}
	formattedDatabase := fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	sc, err := NewClientWithConfig(context.Background(), formattedDatabase, clientConfig, clientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()

	// 2. Perform a simple "SELECT 1" to trigger both unary and streaming gRPC calls.
	sqlSELECT1 := "SELECT 1"
	resultSet := &sppb.ResultSet{
		Rows: []*structpb.ListValue{
			{Values: []*structpb.Value{
				{Kind: &structpb.Value_NumberValue{NumberValue: 1}},
			}},
		},
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{
				Fields: []*sppb.StructType_Field{
					{Name: "Int", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				},
			},
		},
	}
	result := &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: resultSet,
	}
	mockedServer.TestSpanner.PutStatementResult(sqlSELECT1, result)

	txn := sc.ReadOnlyTransaction()
	defer txn.Close()

	ctx := context.Background()
	stmt := NewStatement(sqlSELECT1)
	rowIter := txn.Query(ctx, stmt)
	defer rowIter.Stop()
	for {
		rows, err := rowIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		_ = rows
	}

	// 3. Now perform the assertions of expected headers.
	type headerExpectation struct {
		MethodName  string
		WantHeaders []string
	}

	wantUnaryExpectations := []*headerExpectation{
		{
			"/google.spanner.v1.Spanner/BatchCreateSessions",
			[]string{
				":authority", "content-type", "google-cloud-resource-prefix",
				"grpc-accept-encoding", "user-agent", "x-goog-api-client",
				"x-goog-request-params", "x-goog-spanner-end-to-end-tracing",
				"x-goog-spanner-request-id", "x-goog-spanner-route-to-leader",
			},
		},
		{
			"/google.spanner.v1.Spanner/BeginTransaction",
			[]string{
				":authority", "content-type", "google-cloud-resource-prefix",
				"grpc-accept-encoding", "user-agent", "x-goog-api-client",
				"x-goog-request-params", "x-goog-spanner-end-to-end-tracing",
				"x-goog-spanner-request-id",
			},
		},
	}

	wantStreamingExpectations := []*headerExpectation{
		{
			"/google.spanner.v1.Spanner/ExecuteStreamingSql",
			[]string{
				":authority", "content-type", "google-cloud-resource-prefix",
				"grpc-accept-encoding", "user-agent", "x-goog-api-client",
				"x-goog-request-params", "x-goog-spanner-end-to-end-tracing",
				"x-goog-spanner-request-id",
			},
		},
	}

	var gotUnaryExpectations []*headerExpectation
	for _, mdp := range oint.unaryHeaders {
		gotHeaderKeys := slices.Collect(maps.Keys(mdp.md))
		gotUnaryExpectations = append(gotUnaryExpectations, &headerExpectation{mdp.method, gotHeaderKeys})
	}

	var gotStreamingExpectations []*headerExpectation
	for _, mdp := range oint.streamHeaders {
		gotHeaderKeys := slices.Collect(maps.Keys(mdp.md))
		gotStreamingExpectations = append(gotStreamingExpectations, &headerExpectation{mdp.method, gotHeaderKeys})
	}

	sortHeaderExpectations := func(expectations []*headerExpectation) {
		// Firstly sort by method name.
		sort.Slice(expectations, func(i, j int) bool {
			return expectations[i].MethodName < expectations[j].MethodName
		})

		// 2. Within each expectation, also then sort the header keys.
		for i := range expectations {
			exp := expectations[i]
			sort.Strings(exp.WantHeaders)
		}
	}

	sortHeaderExpectations(gotUnaryExpectations)
	sortHeaderExpectations(wantUnaryExpectations)
	if diff := cmp.Diff(gotUnaryExpectations, wantUnaryExpectations); diff != "" {
		t.Fatalf("Unary headers mismatch: got - want +\n%s", diff)
	}

	sortHeaderExpectations(gotStreamingExpectations)
	sortHeaderExpectations(wantStreamingExpectations)
	if diff := cmp.Diff(gotStreamingExpectations, wantStreamingExpectations); diff != "" {
		t.Fatalf("Streaming headers mismatch: got - want +\n%s", diff)
	}
}
