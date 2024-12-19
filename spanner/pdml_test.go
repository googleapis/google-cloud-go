// Copyright 2018 Google LLC
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
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
)

func TestMockPartitionedUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := NewStatement(UpdateBarSetFoo)
	rowCount, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(UpdateBarSetFooRowCount)
	if rowCount != want {
		t.Errorf("got %d, want %d", rowCount, want)
	}
}

func TestMockPartitionedUpdateWithQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := NewStatement(SelectFooFromBar)
	_, err := client.PartitionedUpdate(ctx, stmt)
	wantCode := codes.InvalidArgument
	var serr *Error
	if !errors.As(err, &serr) {
		t.Errorf("got error %v, want spanner.Error", err)
	}
	if ErrCode(serr) != wantCode {
		t.Errorf("got error %v, want code %s", serr, wantCode)
	}
}

// PDML should be retried if the transaction is aborted.
func TestPartitionedUpdate_Aborted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			Errors: []error{
				status.Error(codes.Aborted, "Transaction aborted"),
				status.Error(codes.Internal, "Received unexpected EOS on DATA frame from server"),
			},
		})
	stmt := NewStatement(UpdateBarSetFoo)
	rowCount, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(UpdateBarSetFooRowCount)
	if rowCount != want {
		t.Errorf("Row count mismatch\ngot: %d\nwant: %d", rowCount, want)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	id1 := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetId()
	id2 := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Transaction.GetId()
	if bytes.Equal(id1, id2) {
		t.Errorf("same transaction used twice, expected two different transactions\ngot tx1: %q\ngot tx2: %q", id1, id2)
	}
}

// Test that a deadline is respected by PDML, and that the session that was
// created is also deleted, even though the update timed out.
func TestPartitionedUpdate_WithDeadline(t *testing.T) {
	t.Parallel()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig:    DefaultSessionPoolConfig,
		Logger:               logger,
	})
	defer teardown()

	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(50*time.Millisecond))
	defer cancel()
	server.TestSpanner.PutExecutionTime(MethodExecuteSql,
		SimulatedExecutionTime{
			MinimumExecutionTime: 100 * time.Millisecond,
		})
	stmt := NewStatement(UpdateBarSetFoo)
	// The following update will cause a 'Failed to delete session' warning to
	// be logged. This is expected. To avoid spamming the log, we temporarily
	// set the output to be discarded.
	logger.SetOutput(io.Discard)
	_, err := client.PartitionedUpdate(ctx, stmt)
	logger.SetOutput(os.Stderr)
	if err == nil {
		t.Fatalf("missing expected error")
	}
	wantCode := codes.DeadlineExceeded
	if status.Code(err) != wantCode {
		t.Fatalf("got error %v, want code %s", err, wantCode)
	}
}

func TestPartitionedUpdate_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client, Compression: gzip.Name})
			defer teardown()

			var err error
			if tt.query.Options == nil {
				_, err = client.PartitionedUpdate(ctx, NewStatement(UpdateBarSetFoo))
			} else {
				_, err = client.PartitionedUpdateWithOptions(ctx, NewStatement(UpdateBarSetFoo), tt.query)
			}
			if err != nil {
				t.Fatalf("expect no errors, but got %v", err)
			}
			checkReqsForQueryOptions(t, server.TestSpanner, tt.want)
		})
	}
}

func TestPartitionedUpdate_Tagging(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	_, err := client.PartitionedUpdateWithOptions(ctx, NewStatement(UpdateBarSetFoo), QueryOptions{RequestTag: "pdml-tag"})
	if err != nil {
		t.Fatalf("expect no errors, but got %v", err)
	}
	checkRequestsForExpectedRequestOptions(t, server.TestSpanner, 1, &sppb.RequestOptions{RequestTag: "pdml-tag"})
}

func TestPartitionedUpdate_ExcludeTxnFromChangeStreams(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	_, err := client.PartitionedUpdateWithOptions(ctx, NewStatement(UpdateBarSetFoo), QueryOptions{ExcludeTxnFromChangeStreams: true})
	if err != nil {
		t.Fatalf("expect no errors, but got %v", err)
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{}}, requests); err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}

	if !requests[1+muxCreateBuffer].(*sppb.BeginTransactionRequest).GetOptions().GetExcludeTxnFromChangeStreams() {
		t.Fatal("Transaction is not set to be excluded from change streams")
	}
}
