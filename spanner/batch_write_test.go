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
	"strings"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
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
		// stopOnly stops the iterator without calling Next.
		stopOnly bool
	}{
		{
			name:     "stopped before first Next",
			stopOnly: true,
		},
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
			if tc.stopOnly {
				iter.Stop()
			} else if err := iter.Do(func(*sppb.BatchWriteResponse) error { return nil }); err != nil && status.Code(err) != codes.InvalidArgument {
				t.Fatalf("Do() error = %v", err)
			}

			if got, want := len(recorder.Ended()), 1; got != want {
				t.Fatalf("ended spans = %d, want %d", got, want)
			}
		})
	}
}

// failingRecvBatchWriteStream fails RecvMsg with err after it has received
// one message.
type failingRecvBatchWriteStream struct {
	grpc.ClientStream
	received int
	err      error
}

func (s *failingRecvBatchWriteStream) RecvMsg(m any) error {
	if s.received == 1 {
		return s.err
	}
	if err := s.ClientStream.RecvMsg(m); err != nil {
		return err
	}
	s.received++
	return nil
}

// TestBatchWriteResponseIteratorRecordsBuiltInMetrics verifies the built-in
// metrics that BatchWriteResponseIterator records. As for streaming queries,
// the BatchWrite request is one operation that ends when Next returns
// iterator.Done or an error, or when the iterator is stopped, and every stream
// that is opened for it is an attempt.
func TestBatchWriteResponseIteratorRecordsBuiltInMetrics(t *testing.T) {
	const method = "Spanner.BatchWrite"
	readAll := func(iter *BatchWriteResponseIterator) (responses int, err error) {
		for {
			if _, err = iter.Next(); err != nil {
				return responses, err
			}
			responses++
		}
	}
	readWithDo := func(iter *BatchWriteResponseIterator) (responses int, err error) {
		err = iter.Do(func(*sppb.BatchWriteResponse) error {
			responses++
			return nil
		})
		return responses, err
	}
	errDoCallback := errors.New("callback failed")
	doCallbackError := func(iter *BatchWriteResponseIterator) (responses int, err error) {
		err = iter.Do(func(*sppb.BatchWriteResponse) error {
			responses++
			return errDoCallback
		})
		return responses, err
	}
	stopBeforeNext := func(iter *BatchWriteResponseIterator) (int, error) {
		iter.Stop()
		return 0, nil
	}
	stopAfterFirstResponse := func(iter *BatchWriteResponseIterator) (int, error) {
		defer iter.Stop()
		_, err := iter.Next()
		return 1, err
	}
	op := func(code string, attempts int64) map[string]int64 {
		return map[string]int64{
			`attempt_count method="` + method + `" status=` + code:       attempts,
			`operation_count method="` + method + `" status=` + code:     1,
			`operation_latencies method="` + method + `" status=` + code: 1,
		}
	}
	attempts := func(want map[string]int64, codes ...string) map[string]int64 {
		for _, code := range codes {
			want[`attempt_latencies method="`+method+`" status=`+code]++
			want[`gfe_connectivity_error_count method="`+method+`" status=`+code]++
		}
		return want
	}
	unavailable := status.Error(codes.Unavailable, "unavailable")

	for _, test := range []struct {
		name                string
		canceled            bool
		failFirstStreamOpen bool
		failFirstStreamRecv bool
		executionTime       *SimulatedExecutionTime
		iterate             func(*BatchWriteResponseIterator) (int, error)
		wantResponses       int
		wantCode            codes.Code
		want                map[string]int64
	}{
		{
			name:          "all mutation groups applied",
			iterate:       readAll,
			wantResponses: 2,
			want:          attempts(op("OK", 1), "OK"),
		},
		{
			name:          "all mutation groups applied with Do",
			iterate:       readWithDo,
			wantResponses: 2,
			want:          attempts(op("OK", 1), "OK"),
		},
		{
			name:          "error returned by Do callback",
			iterate:       doCallbackError,
			wantResponses: 1,
			wantCode:      codes.Unknown,
			want:          attempts(op("OK", 1), "OK"),
		},
		{
			name:    "stopped before first Next",
			iterate: stopBeforeNext,
			want:    map[string]int64{},
		},
		{
			name:          "stopped before end of stream",
			iterate:       stopAfterFirstResponse,
			wantResponses: 1,
			want:          attempts(op("OK", 1), "OK"),
		},
		{
			name:                "retryable error opening first stream",
			failFirstStreamOpen: true,
			iterate:             readAll,
			wantResponses:       2,
			want:                attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name:          "retryable error before first response",
			executionTime: &SimulatedExecutionTime{Errors: []error{unavailable}},
			iterate:       readAll,
			wantResponses: 2,
			want:          attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name:                "stream resumed after retryable error",
			failFirstStreamRecv: true,
			iterate:             readAll,
			wantResponses:       2,
			want:                attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name:     "context canceled before first stream",
			canceled: true,
			iterate:  readAll,
			wantCode: codes.Canceled,
			want:     attempts(op("Canceled", 1), "Canceled"),
		},
		{
			name:          "non-retryable error before first response",
			executionTime: &SimulatedExecutionTime{Errors: []error{status.Error(codes.InvalidArgument, "invalid")}},
			iterate:       readAll,
			wantCode:      codes.InvalidArgument,
			want:          attempts(op("InvalidArgument", 1), "InvalidArgument"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, provider := newTestMeterProvider()
			var opts []option.ClientOption
			if test.failFirstStreamOpen || test.failFirstStreamRecv {
				var opened bool
				opts = append(opts, option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(
					func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
						if !strings.HasSuffix(method, "/BatchWrite") || opened {
							return streamer(ctx, desc, cc, method, opts...)
						}
						opened = true
						if test.failFirstStreamOpen {
							return nil, unavailable
						}
						stream, err := streamer(ctx, desc, cc, method, opts...)
						if err != nil {
							return nil, err
						}
						return &failingRecvBatchWriteStream{ClientStream: stream, err: unavailable}, nil
					})))
			}
			server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{DisableNativeMetrics: true, ClientMetricsProvider: provider}, opts)
			defer teardown()
			if test.executionTime != nil {
				server.TestSpanner.PutExecutionTime(MethodBatchWrite, *test.executionTime)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			iter := client.BatchWrite(ctx, []*MutationGroup{
				{[]*Mutation{{op: opInsertOrUpdate, table: "t_test", columns: []string{"key", "val"}, values: []any{"foo1", 1}}}},
				{[]*Mutation{{op: opInsertOrUpdate, table: "t_test", columns: []string{"key", "val"}, values: []any{"foo2", 2}}}},
			})
			defer iter.Stop()
			if test.canceled {
				cancel()
			}
			gotResponses, err := test.iterate(iter)
			var gotCode codes.Code
			if err != nil && err != iterator.Done {
				gotCode = ErrCode(err)
			}
			if g, w := gotCode, test.wantCode; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			if g, w := gotResponses, test.wantResponses; g != w {
				t.Fatalf("response count mismatch\n Got: %v\nWant: %v", g, w)
			}
			got := builtInMetricsForMethod(t, collectTestMetrics(t, reader), method)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("recorded metrics mismatch (-want +got):\n%s", diff)
			}

			// The operation ends only once.
			iter.Next()
			iter.Stop()
			if diff := cmp.Diff(got, builtInMetricsForMethod(t, collectTestMetrics(t, reader), method)); diff != "" {
				t.Errorf("Next() and Stop() after the end recorded metrics (-before +after):\n%s", diff)
			}
		})
	}
}
