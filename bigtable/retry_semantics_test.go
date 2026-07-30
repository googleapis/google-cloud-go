// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// End-to-end MutateRow retry-behavior tests via mockserver. Pins the
// classic-path retry oracle at the surface Table.Apply exposes:
//   - idempotent mutation (explicit timestamp) retries on Unavailable
//   - non-idempotent mutation (ServerTime) does NOT retry on Unavailable
//   - server-returned Aborted is retried for idempotent mutations
//
// TestMutationsAreRetryable in bigtable_test.go only exercises the
// predicate; these tests drive the full retry loop through a real
// bigtable.Client dialed at a mockserver that injects faults.

package bigtable

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/mockserver"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// faultyMutateRow returns a MutateRowFn that fails the first failN
// attempts with the given code, then succeeds. attemptCount is
// incremented on every call so tests can assert exact attempts.
func faultyMutateRow(attemptCount *int64, failN int, code codes.Code) func(context.Context, *btpb.MutateRowRequest) (*btpb.MutateRowResponse, error) {
	return func(_ context.Context, _ *btpb.MutateRowRequest) (*btpb.MutateRowResponse, error) {
		n := atomic.AddInt64(attemptCount, 1)
		if int(n) <= failN {
			return nil, status.Error(code, "injected fault")
		}
		return &btpb.MutateRowResponse{}, nil
	}
}

// newRetryTestClient wires a real bigtable.Client at the mockserver
// with unauthenticated insecure creds. Installs a no-op PingAndWarm
// handler on srv so the connection pool factory prime step succeeds.
func newRetryTestClient(t *testing.T, srv *mockserver.Server) *Client {
	t.Helper()
	if srv.PingAndWarmFn == nil {
		srv.PingAndWarmFn = func(context.Context, *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
			return &btpb.PingAndWarmResponse{}, nil
		}
	}
	ctx := context.Background()
	client, err := NewClient(ctx, "proj", "inst",
		option.WithEndpoint(srv.Addr),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestRetrySemantics_Classic_Idempotent asserts that a mutation with
// only explicit-timestamp cells retries through the classic
// {Unavailable, DeadlineExceeded, Aborted} retry option
// (mutationsAreRetryable → t.c.retryOption in bigtable.go).
func TestRetrySemantics_Classic_Idempotent(t *testing.T) {
	srv, err := mockserver.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("mockserver: %v", err)
	}
	defer srv.Close()

	var attempts int64
	srv.MutateRowFn = faultyMutateRow(&attempts, 2, codes.Unavailable)

	client := newRetryTestClient(t, srv)
	defer client.Close()
	tbl := client.OpenTable("t")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mut := NewMutation()
	mut.Set("cf", "q", Time(time.Unix(0, 1_000_000)), []byte("v")) // explicit ts → idempotent
	err = tbl.Apply(ctx, "k", mut)
	if err != nil {
		t.Fatalf("Apply: got err=%v, want nil (should have retried past 2 faults)", err)
	}
	if got := atomic.LoadInt64(&attempts); got != 3 {
		t.Errorf("attempts=%d, want 3 (2 injected fails + 1 success)", got)
	}
}

// TestRetrySemantics_Classic_NonIdempotent asserts that a mutation
// with any ServerTime cell does NOT attach the retry option, so a
// transient Unavailable propagates to the caller after the first
// attempt.
func TestRetrySemantics_Classic_NonIdempotent(t *testing.T) {
	srv, err := mockserver.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("mockserver: %v", err)
	}
	defer srv.Close()

	var attempts int64
	srv.MutateRowFn = faultyMutateRow(&attempts, 2, codes.Unavailable)

	client := newRetryTestClient(t, srv)
	defer client.Close()
	tbl := client.OpenTable("t")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mut := NewMutation()
	mut.Set("cf", "q", ServerTime, []byte("v")) // ServerTime → non-idempotent
	err = tbl.Apply(ctx, "k", mut)
	if err == nil {
		t.Fatalf("Apply: got err=nil, want Unavailable (non-idempotent should not retry)")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("err code=%s, want Unavailable", got)
	}
	if got := atomic.LoadInt64(&attempts); got != 1 {
		t.Errorf("attempts=%d, want 1 (non-idempotent must not retry)", got)
	}
}

// TestRetrySemantics_Classic_AbortedRetries asserts that a
// server-returned Aborted is retried for idempotent mutations on the
// classic path (Aborted is in the retry set alongside Unavailable and
// DeadlineExceeded).
func TestRetrySemantics_Classic_AbortedRetries(t *testing.T) {
	srv, err := mockserver.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("mockserver: %v", err)
	}
	defer srv.Close()

	var attempts int64
	srv.MutateRowFn = faultyMutateRow(&attempts, 1, codes.Aborted)

	client := newRetryTestClient(t, srv)
	defer client.Close()
	tbl := client.OpenTable("t")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mut := NewMutation()
	mut.Set("cf", "q", Time(time.Unix(0, 1_000_000)), []byte("v"))
	err = tbl.Apply(ctx, "k", mut)
	if err != nil {
		t.Fatalf("Apply: got err=%v, want nil", err)
	}
	if got := atomic.LoadInt64(&attempts); got < 2 {
		t.Errorf("attempts=%d, want >=2 (classic retries Aborted for idempotent)", got)
	}
}
