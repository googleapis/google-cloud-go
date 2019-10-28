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
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc/codes"
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
	if serr, ok := err.(*Error); !ok || serr.Code != wantCode {
		t.Errorf("got error %v, want code %s", err, wantCode)
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
			Errors: []error{status.Error(codes.Aborted, "Transaction aborted")},
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
		&sppb.CreateSessionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.DeleteSessionRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	id1 := gotReqs[2].(*sppb.ExecuteSqlRequest).Transaction.GetId()
	id2 := gotReqs[4].(*sppb.ExecuteSqlRequest).Transaction.GetId()
	if bytes.Equal(id1, id2) {
		t.Errorf("same transaction used twice, expected two different transactions\ngot tx1: %q\ngot tx2: %q", id1, id2)
	}
}

// Test that a deadline is respected by PDML, and that the session that was
// created is also deleted, even though the update timed out.
func TestPartitionedUpdate_WithDeadline(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
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
	// be logged. This is expected. Once each client has its own logger, we
	// should temporarily turn off logging to prevent this warning to be
	// logged.
	_, err := client.PartitionedUpdate(ctx, stmt)
	if err == nil {
		t.Fatalf("missing expected error")
	}
	wantCode := codes.DeadlineExceeded
	if status.Code(err) != wantCode {
		t.Fatalf("got error %v, want code %s", err, wantCode)
	}
}
