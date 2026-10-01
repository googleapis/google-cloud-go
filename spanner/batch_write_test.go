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
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/api/iterator"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeBatchWriteResult is a single result returned by fakeBatchWriteStream.Recv.
type fakeBatchWriteResult struct {
	indexes []int32
	status  *spb.Status
	err     error
}

// fakeBatchWriteAttempt scripts the outcome of one BatchWrite RPC.
type fakeBatchWriteAttempt struct {
	// openErr is returned when the stream is opened.
	openErr error
	// results are returned by Recv in order. Recv returns io.EOF once they
	// are exhausted.
	results []fakeBatchWriteResult
}

type fakeBatchWriteStream struct {
	grpc.ClientStream
	results []fakeBatchWriteResult
}

func (s *fakeBatchWriteStream) Recv() (*sppb.BatchWriteResponse, error) {
	if len(s.results) == 0 {
		return nil, io.EOF
	}
	result := s.results[0]
	s.results = s.results[1:]
	if result.err != nil {
		return nil, result.err
	}
	return &sppb.BatchWriteResponse{Indexes: result.indexes, Status: result.status}, nil
}

// fakeBatchWriteRPC replays the scripted attempts and records the mutation
// groups that were sent on each attempt, identified by their table names.
type fakeBatchWriteRPC struct {
	attempts []fakeBatchWriteAttempt
	requests [][]string
}

func (f *fakeBatchWriteRPC) rpc(_ context.Context, mgs []*sppb.BatchWriteRequest_MutationGroup) (sppb.Spanner_BatchWriteClient, error) {
	tables := make([]string, 0, len(mgs))
	for _, mg := range mgs {
		tables = append(tables, mg.GetMutations()[0].GetInsert().GetTable())
	}
	f.requests = append(f.requests, tables)
	if len(f.requests) > len(f.attempts) {
		return nil, fmt.Errorf("unexpected BatchWrite attempt %d", len(f.requests))
	}
	attempt := f.attempts[len(f.requests)-1]
	if attempt.openErr != nil {
		return nil, attempt.openErr
	}
	return &fakeBatchWriteStream{results: attempt.results}, nil
}

func testBatchWriteMutationGroups(n int) []*sppb.BatchWriteRequest_MutationGroup {
	mgs := make([]*sppb.BatchWriteRequest_MutationGroup, n)
	for i := range mgs {
		mgs[i] = &sppb.BatchWriteRequest_MutationGroup{
			Mutations: []*sppb.Mutation{{
				Operation: &sppb.Mutation_Insert{Insert: &sppb.Mutation_Write{Table: fmt.Sprintf("g%d", i)}},
			}},
		}
	}
	return mgs
}

func repeatBatchWriteAttempt(attempt fakeBatchWriteAttempt, n int) []fakeBatchWriteAttempt {
	attempts := make([]fakeBatchWriteAttempt, n)
	for i := range attempts {
		attempts[i] = attempt
	}
	return attempts
}

func TestBatchWriteResponseIterator_RetryAndResume(t *testing.T) {
	t.Parallel()

	unavailable := status.Error(codes.Unavailable, "unavailable")
	invalidArgument := status.Error(codes.InvalidArgument, "invalid argument")
	resourceExhausted := status.Error(codes.ResourceExhausted, "resource exhausted")
	aborted := &spb.Status{Code: int32(codes.Aborted), Message: "aborted"}

	for _, tc := range []struct {
		name     string
		groups   int
		attempts []fakeBatchWriteAttempt
		// wantRequests are the mutation groups sent on each attempt.
		wantRequests [][]string
		// wantIndexes are the indexes of each response returned by Next.
		wantIndexes [][]int32
		// wantCode is the code of the error that ends the iteration, or
		// codes.OK if the iteration ends with iterator.Done.
		wantCode codes.Code
	}{
		{
			name:   "no errors",
			groups: 3,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 2}}, {indexes: []int32{1}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2"}},
			wantIndexes:  [][]int32{{0, 2}, {1}},
		},
		{
			name:   "unavailable when opening stream is retried",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{openErr: unavailable},
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 1}}}},
			},
			wantRequests: [][]string{{"g0", "g1"}, {"g0", "g1"}},
			wantIndexes:  [][]int32{{0, 1}},
		},
		{
			name:   "unavailable midway resumes with unacknowledged groups",
			groups: 4,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 2}}, {err: unavailable}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 1}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2", "g3"}, {"g1", "g3"}},
			wantIndexes:  [][]int32{{0, 2}, {1, 3}},
		},
		{
			name:   "indexes are translated across multiple retries",
			groups: 5,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{1}}, {err: unavailable}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{2}}, {err: unavailable}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{1, 0, 2}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2", "g3", "g4"}, {"g0", "g2", "g3", "g4"}, {"g0", "g2", "g4"}},
			wantIndexes:  [][]int32{{1}, {3}, {2, 0, 4}},
		},
		{
			name:   "premature end of stream resumes with unacknowledged groups",
			groups: 3,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{1}}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 1}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2"}, {"g0", "g2"}},
			wantIndexes:  [][]int32{{1}, {0, 2}},
		},
		{
			name:   "group with error status is acknowledged and not resent",
			groups: 3,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0}, status: aborted}, {err: unavailable}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 1}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2"}, {"g1", "g2"}},
			wantIndexes:  [][]int32{{0}, {1, 2}},
		},
		{
			name:   "duplicate indexes within a stream are returned as sent",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 0}}, {indexes: []int32{1}}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantIndexes:  [][]int32{{0, 0}, {1}},
		},
		{
			name:   "error after all groups are acknowledged ends iteration",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0, 1}}, {err: unavailable}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantIndexes:  [][]int32{{0, 1}},
		},
		{
			name:   "non-retryable error when opening stream",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{openErr: invalidArgument},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantCode:     codes.InvalidArgument,
		},
		{
			name:   "non-retryable error midway",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0}}, {err: invalidArgument}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantIndexes:  [][]int32{{0}},
			wantCode:     codes.InvalidArgument,
		},
		{
			name:   "resource exhausted is not retried",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{err: resourceExhausted}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantCode:     codes.ResourceExhausted,
		},
		{
			name:         "unavailable is returned once attempts are exhausted",
			groups:       1,
			attempts:     repeatBatchWriteAttempt(fakeBatchWriteAttempt{openErr: unavailable}, batchWriteMaxAttempts),
			wantRequests: repeatBatchWriteRequest([]string{"g0"}, batchWriteMaxAttempts),
			wantCode:     codes.Unavailable,
		},
		{
			name:         "premature end of stream is returned once attempts are exhausted",
			groups:       1,
			attempts:     repeatBatchWriteAttempt(fakeBatchWriteAttempt{}, batchWriteMaxAttempts),
			wantRequests: repeatBatchWriteRequest([]string{"g0"}, batchWriteMaxAttempts),
			wantCode:     codes.Unavailable,
		},
		{
			name:   "index out of bounds is not retried",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{2}}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantCode:     codes.Internal,
		},
		{
			name:   "index out of bounds of resumed stream is not retried",
			groups: 3,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{0}}, {err: unavailable}}},
				{results: []fakeBatchWriteResult{{indexes: []int32{2}}}},
			},
			wantRequests: [][]string{{"g0", "g1", "g2"}, {"g1", "g2"}},
			wantIndexes:  [][]int32{{0}},
			wantCode:     codes.Internal,
		},
		{
			name:   "negative index is not retried",
			groups: 2,
			attempts: []fakeBatchWriteAttempt{
				{results: []fakeBatchWriteResult{{indexes: []int32{-1}}}},
			},
			wantRequests: [][]string{{"g0", "g1"}},
			wantCode:     codes.Internal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeBatchWriteRPC{attempts: tc.attempts}
			iter := newBatchWriteResponseIterator(context.Background(), testBatchWriteMutationGroups(tc.groups), fake.rpc, gax.Backoff{Initial: time.Nanosecond, Max: time.Nanosecond, Multiplier: 1})
			defer iter.Stop()

			var gotIndexes [][]int32
			var err error
			for {
				var response *sppb.BatchWriteResponse
				response, err = iter.Next()
				if err != nil {
					break
				}
				gotIndexes = append(gotIndexes, response.GetIndexes())
			}

			if tc.wantCode == codes.OK {
				if !errors.Is(err, iterator.Done) {
					t.Fatalf("Next() error = %v, want iterator.Done", err)
				}
			} else if got := status.Code(err); got != tc.wantCode {
				t.Fatalf("Next() error code = %v, want %v (error: %v)", got, tc.wantCode, err)
			}
			if diff := cmp.Diff(tc.wantIndexes, gotIndexes); diff != "" {
				t.Errorf("response indexes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantRequests, fake.requests); diff != "" {
				t.Errorf("requested mutation groups mismatch (-want +got):\n%s", diff)
			}
			// Once the iteration has ended, Next keeps returning the same error.
			if _, again := iter.Next(); again != err {
				t.Errorf("Next() after end = %v, want %v", again, err)
			}
		})
	}
}

func repeatBatchWriteRequest(request []string, n int) [][]string {
	requests := make([][]string, n)
	for i := range requests {
		requests[i] = request
	}
	return requests
}

func TestBatchWriteResponseIterator_CanceledContextStopsRetry(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeBatchWriteRPC{attempts: []fakeBatchWriteAttempt{{openErr: status.Error(codes.Unavailable, "unavailable")}}}
	iter := newBatchWriteResponseIterator(ctx, testBatchWriteMutationGroups(1), fake.rpc, gax.Backoff{Initial: time.Hour, Max: time.Hour, Multiplier: 1})
	defer iter.Stop()

	if _, err := iter.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v, want %v", err, context.Canceled)
	}
	if got, want := len(fake.requests), 1; got != want {
		t.Fatalf("BatchWrite attempts = %d, want %d", got, want)
	}
}

func TestClient_BatchWrite_RetriesUnavailable(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodBatchWrite,
		SimulatedExecutionTime{Errors: []error{status.Error(codes.Unavailable, "unavailable")}},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{{op: opInsertOrUpdate, table: "t_test", columns: []string{"key", "val"}, values: []any{"foo1", 1}}}},
		{[]*Mutation{{op: opInsertOrUpdate, table: "t_test", columns: []string{"key", "val"}, values: []any{"foo2", 2}}}},
	}

	var gotIndexes []int32
	err := client.BatchWrite(context.Background(), mutationGroups).Do(func(r *sppb.BatchWriteResponse) error {
		gotIndexes = append(gotIndexes, r.GetIndexes()...)
		return nil
	})
	if err != nil {
		t.Fatalf("BatchWrite failed: %v", err)
	}
	if diff := cmp.Diff([]int32{0, 1}, gotIndexes); diff != "" {
		t.Errorf("response indexes mismatch (-want +got):\n%s", diff)
	}
	var batchWrites int
	for _, req := range drainRequestsFromServer(server.TestSpanner) {
		if req, ok := req.(*sppb.BatchWriteRequest); ok {
			batchWrites++
			if got, want := len(req.GetMutationGroups()), len(mutationGroups); got != want {
				t.Errorf("BatchWriteRequest mutation groups = %d, want %d", got, want)
			}
		}
	}
	if got, want := batchWrites, 2; got != want {
		t.Errorf("BatchWrite requests = %d, want %d", got, want)
	}
}

func TestBatchWriteResponseIterator_StopEndsSpan(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		attempts []fakeBatchWriteAttempt
	}{
		{
			name:     "stream completed",
			attempts: []fakeBatchWriteAttempt{{results: []fakeBatchWriteResult{{indexes: []int32{0}}}}},
		},
		{
			name:     "stream failed to open",
			attempts: []fakeBatchWriteAttempt{{openErr: status.Error(codes.InvalidArgument, "invalid argument")}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			ctx, _ := provider.Tracer("test").Start(context.Background(), "BatchWriteResponseIterator")

			fake := &fakeBatchWriteRPC{attempts: tc.attempts}
			iter := newBatchWriteResponseIterator(ctx, testBatchWriteMutationGroups(1), fake.rpc, gax.Backoff{Initial: time.Nanosecond, Max: time.Nanosecond, Multiplier: 1})
			if err := iter.Do(func(*sppb.BatchWriteResponse) error { return nil }); err != nil && status.Code(err) != codes.InvalidArgument {
				t.Fatalf("Do() error = %v", err)
			}

			if got, want := len(recorder.Ended()), 1; got != want {
				t.Fatalf("ended spans = %d, want %d", got, want)
			}
		})
	}
}
