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

package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeInvoker satisfies the Invoker interface. The first errBefore
// calls return err; every call after that returns result. Counts total
// calls under a mutex so tests can assert attempt fan-out.
type fakeInvoker struct {
	mu        sync.Mutex
	calls     int
	errBefore int
	err       error
	result    btransport.InvokeResult
}

func (f *fakeInvoker) Invoke(_ context.Context, _ btransport.VRpcDescriptor, _ interface{}) (btransport.InvokeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.errBefore {
		return btransport.InvokeResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeInvoker) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newTestTable wires a sessionTable to the given invokers, a
// ManualReader-backed MeterProvider, and stub VRpc descriptors. The
// returned reader can be Collect()ed to inspect emitted histograms.
func newTestTable(t *testing.T, readInv, writeInv Invoker) (*sessionTable, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	factory, err := metrics.NewFactoryForTest("test-project", "test-instance", "test-profile", mp)
	if err != nil {
		t.Fatalf("metrics.NewFactoryForTest: %v", err)
	}
	openRead := func() (Invoker, error) { return readInv, nil }
	openWrite := func() (Invoker, error) { return writeInv, nil }
	if readInv == nil {
		openRead = nil
	}
	if writeInv == nil {
		openWrite = nil
	}
	tbl := newSessionTable(
		"test-table",
		openRead,
		openWrite,
		nil, // closeRead — fake pools don't back a real poolCloser
		nil, // closeWrite
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		&btransport.VRpcDescriptorImpl{MethodName: "test.MutateRow"},
		nil,
		factory,
	)
	return tbl, reader
}

// sumHistogramSamples returns the total sample count across every data
// point of the named metric. Zero (and false) if the metric is absent.
func sumHistogramSamples(t *testing.T, reader *sdkmetric.ManualReader, name string) (uint64, bool) {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("reader.Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %q data is %T, want Histogram[float64]", name, m.Data)
			}
			var total uint64
			for _, dp := range hist.DataPoints {
				total += dp.Count
			}
			return total, true
		}
	}
	return 0, false
}

// sampleAttribute returns the string value of the named attribute on
// the first data point of the named histogram. Fails the test if the
// metric is absent, has zero data points, or the attribute is missing.
// Used to pin the shape of monitored-resource labels (e.g. `table`).
func sampleAttribute(t *testing.T, reader *sdkmetric.ManualReader, metricName, attrKey string) string {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("reader.Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %q data is %T, want Histogram[float64]", metricName, m.Data)
			}
			if len(hist.DataPoints) == 0 {
				t.Fatalf("metric %q has no data points", metricName)
			}
			v, ok := hist.DataPoints[0].Attributes.Value(attribute.Key(attrKey))
			if !ok {
				t.Fatalf("metric %q data point missing attribute %q", metricName, attrKey)
			}
			return v.AsString()
		}
	}
	t.Fatalf("metric %q not emitted", metricName)
	return ""
}

func assertSamples(t *testing.T, reader *sdkmetric.ManualReader, name string, want uint64) {
	t.Helper()
	got, ok := sumHistogramSamples(t, reader, name)
	if !ok {
		t.Errorf("metric %q not emitted; want %d sample(s)", name, want)
		return
	}
	if got != want {
		t.Errorf("metric %q sample count = %d, want %d", name, got, want)
	}
}

func TestSessionTableReadRow_RecordsAttemptAndOperation(t *testing.T) {
	inv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionReadRowResponse{}}}
	tbl, reader := newTestTable(t, inv, nil)

	if _, err := tbl.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("row1")}); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	if got := inv.callCount(); got != 1 {
		t.Fatalf("invoker calls = %d, want 1", got)
	}
	assertSamples(t, reader, "attempt_latencies", 1)
	assertSamples(t, reader, "attempt_latencies2", 1)
	assertSamples(t, reader, "operation_latencies", 1)
}

// TestSessionTable_TableAttributeIsShortID pins the `table` monitored-
// resource label to the short id passed at construction. Regression
// guard: the session path used to stamp the fully-qualified resource
// name here, breaking cross-path dashboards that group by table id.
func TestSessionTable_TableAttributeIsShortID(t *testing.T) {
	inv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionReadRowResponse{}}}
	tbl, reader := newTestTable(t, inv, nil)

	if _, err := tbl.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("row1")}); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	for _, name := range []string{"attempt_latencies", "attempt_latencies2", "operation_latencies"} {
		if got := sampleAttribute(t, reader, name, "table"); got != "test-table" {
			t.Errorf("metric %q: table attribute = %q, want %q (short id, not fully-qualified name)", name, got, "test-table")
		}
	}
}

func TestSessionTableMutateRow_RecordsAttemptAndOperation(t *testing.T) {
	inv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionMutateRowResponse{}}}
	tbl, reader := newTestTable(t, nil, inv)

	req := &btpb.SessionMutateRowRequest{
		Key: []byte("row1"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q"),
				TimestampMicros: 1_000_000,
				Value:           []byte("v"),
			}},
		}},
	}
	if _, err := tbl.MutateRow(context.Background(), req); err != nil {
		t.Fatalf("MutateRow: %v", err)
	}
	if got := inv.callCount(); got != 1 {
		t.Fatalf("invoker calls = %d, want 1", got)
	}
	assertSamples(t, reader, "attempt_latencies", 1)
	assertSamples(t, reader, "attempt_latencies2", 1)
	assertSamples(t, reader, "operation_latencies", 1)
}

// TestSessionTableReadRow_RetriesRecordAttemptPerAttempt drives the
// retry loop three times (two transport failures, then success) and
// asserts each attempt emits a fresh attempt_latencies / attempt_latencies2
// sample while operation_latencies is recorded exactly once for the
// operation as a whole. This is what the previous version of the code
// silently violated — attempts on the session path were never recorded.
func TestSessionTableReadRow_RetriesRecordAttemptPerAttempt(t *testing.T) {
	retriable := btransport.TagErr(btransport.StateUncommitted, status.Error(codes.Unavailable, "test"))
	inv := &fakeInvoker{
		errBefore: 2,
		err:       retriable,
		result:    btransport.InvokeResult{Response: &btpb.SessionReadRowResponse{}},
	}
	tbl, reader := newTestTable(t, inv, nil)

	if _, err := tbl.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("row1")}); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	if got := inv.callCount(); got != 3 {
		t.Fatalf("invoker calls = %d, want 3", got)
	}
	assertSamples(t, reader, "attempt_latencies", 3)
	assertSamples(t, reader, "attempt_latencies2", 3)
	assertSamples(t, reader, "operation_latencies", 1)
}

// TestSessionTable_MatView_MutateRowReturnsErrWriteNotSupported verifies
// SESSION_SPEC.md #11: MaterializedView is read-only by contract. When
// constructed with a nil writeInv (which OpenMaterializedView passes for
// openWrite in client.go:415), MutateRow MUST return ErrWriteNotSupported
// — NOT a generic "no pool" error, and NOT panic.
func TestSessionTable_MatView_MutateRowReturnsErrWriteNotSupported(t *testing.T) {
	readInv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionReadRowResponse{}}}
	// writeInv=nil mirrors OpenMaterializedView's construction path.
	tbl, _ := newTestTable(t, readInv, nil)

	_, err := tbl.MutateRow(context.Background(), &btpb.SessionMutateRowRequest{
		Key: []byte("row1"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q"),
				TimestampMicros: 1_000_000,
				Value:           []byte("v"),
			}},
		}},
	})
	if err == nil {
		t.Fatal("MutateRow on read-only (MatView-shape) table returned nil, want ErrWriteNotSupported")
	}
	if err != ErrWriteNotSupported {
		t.Errorf("err = %v, want ErrWriteNotSupported (SESSION_SPEC.md #11)", err)
	}
	// Read side must still work — MatView is read-only, not fully disabled.
	if _, rerr := tbl.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("row1")}); rerr != nil {
		t.Errorf("ReadRow on MatView table = %v, want nil (read path must remain functional)", rerr)
	}
}

// mutationSetCell / mutationServerTime keep the SetCell shape used by
// the idempotency-plumbing tests below in one place, so a future proto
// change lands in one edit instead of scattered literals.
func mutationSetCell(ts int64) *btpb.Mutation {
	return &btpb.Mutation{Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
		FamilyName:      "cf",
		ColumnQualifier: []byte("q"),
		TimestampMicros: ts,
		Value:           []byte("v"),
	}}}
}

// TestMutationsAreRetryable_SessionHelper pins the session-package copy
// of the classifier. The classic helper is duplicated here to avoid an
// import cycle (table.go:274); the two must stay in sync or a
// non-idempotent MutateRow on the session path would retry when the
// classic path wouldn't.
func TestMutationsAreRetryable_SessionHelper(t *testing.T) {
	const serverTime int64 = -1
	cases := []struct {
		name string
		muts []*btpb.Mutation
		want bool
	}{
		{"nil", nil, true},
		{"single_explicit_ts", []*btpb.Mutation{mutationSetCell(1_000_000)}, true},
		{"single_server_time", []*btpb.Mutation{mutationSetCell(serverTime)}, false},
		{"mixed_explicit_and_server_time", []*btpb.Mutation{mutationSetCell(1_000_000), mutationSetCell(serverTime)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mutationsAreRetryable(tc.muts); got != tc.want {
				t.Errorf("mutationsAreRetryable(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestSessionTableMutateRow_IdempotencyFlowsToRetry verifies the
// end-to-end plumbing from mutationsAreRetryable(req.GetMutations()) →
// RetryingOptions.Idempotent → shouldRetryDefault at table.go:119,176.
// The retry interceptor is configured with MaxAttempts=3 in dispatch,
// so an idempotent TransportFailure runs the full budget
// while a non-idempotent one fails after exactly one attempt. A regression that
// hard-wired Idempotent: true (or broke mutationsAreRetryable) would
// silently allow double-apply on non-idempotent Apply.
func TestSessionTableMutateRow_IdempotencyFlowsToRetry(t *testing.T) {
	const serverTime int64 = -1
	transportErr := btransport.TagErr(btransport.StateTransportFailure, status.Error(codes.Unavailable, "wire error"))

	cases := []struct {
		name           string
		muts           []*btpb.Mutation
		wantAttemptCap int // max attempts we expect the interceptor to burn
	}{
		{
			name:           "explicit_timestamp_idempotent_retries_full_budget",
			muts:           []*btpb.Mutation{mutationSetCell(1_000_000)},
			wantAttemptCap: 3, // MaxAttempts in session/table.go dispatch
		},
		{
			name:           "server_time_non_idempotent_stops_at_one_attempt",
			muts:           []*btpb.Mutation{mutationSetCell(serverTime)},
			wantAttemptCap: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv := &fakeInvoker{errBefore: 1000, err: transportErr}
			tbl, _ := newTestTable(t, nil, inv)

			_, err := tbl.MutateRow(context.Background(), &btpb.SessionMutateRowRequest{
				Key:       []byte("row1"),
				Mutations: tc.muts,
			})
			if err == nil {
				t.Fatal("MutateRow returned nil; want the TransportFailure to surface")
			}
			if got := inv.callCount(); got != tc.wantAttemptCap {
				t.Errorf("invoker calls = %d, want %d (Idempotent gate must control retry budget)",
					got, tc.wantAttemptCap)
			}
		})
	}
}

// blockingInvoker's Invoke waits on release before returning err. Lets a
// test kick off a call, cancel ctx while it's in flight, then observe
// what the retry interceptor does with the resulting error.
type blockingInvoker struct {
	mu      sync.Mutex
	calls   int
	release chan struct{}
	err     error
}

func (b *blockingInvoker) Invoke(ctx context.Context, _ btransport.VRpcDescriptor, _ interface{}) (btransport.InvokeResult, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return btransport.InvokeResult{}, btransport.TagErr(btransport.StateTransportFailure, ctx.Err())
	case <-b.release:
		return btransport.InvokeResult{}, b.err
	}
}

func (b *blockingInvoker) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// TestSessionTableMutateRow_CtxDoneStopsAtOneAttempt composes the full
// chain: real dispatch (RetryingVRpc), real ctx.Done, real ServerTime
// mutation → attempt tagged StateTransportFailure by the invoker → retry
// interceptor short-circuits (ctx.Err() branch AND non-idempotent gate
// would each be sufficient; both together document the safety net).
// A regression that stripped the ctx.Err check or accidentally retried
// TransportFailure regardless of Idempotent would show up as
// callCount > 1.
func TestSessionTableMutateRow_CtxDoneStopsAtOneAttempt(t *testing.T) {
	const serverTime int64 = -1
	inv := &blockingInvoker{release: make(chan struct{})}
	tbl, _ := newTestTable(t, nil, inv)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := tbl.MutateRow(ctx, &btpb.SessionMutateRowRequest{
			Key:       []byte("row1"),
			Mutations: []*btpb.Mutation{mutationSetCell(serverTime)},
		})
		done <- err
	}()

	// Wait until the first attempt is in flight, then cancel.
	waitUntil := time.Now().Add(2 * time.Second)
	for inv.callCount() == 0 && time.Now().Before(waitUntil) {
		time.Sleep(5 * time.Millisecond)
	}
	if inv.callCount() == 0 {
		t.Fatal("invoker never received the first call within 2s")
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("MutateRow returned nil after ctx cancel; want error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MutateRow did not return after ctx cancel within 2s")
	}
	if got := inv.callCount(); got != 1 {
		t.Errorf("invoker calls = %d, want 1 (ctx.Done on non-idempotent must not retry)", got)
	}
}

// TestSessionTable_ReadAndWritePoolsDoNotShareSessions verifies
// SESSION_SPEC.md #11: read and write pools are distinct. ReadRow MUST
// dispatch through the READ Invoker; MutateRow MUST dispatch through the
// WRITE Invoker; the two MUST NOT cross-invoke. This is what keeps the
// multiplex=1 rule (spec #2) from starving cross-direction traffic.
func TestSessionTable_ReadAndWritePoolsDoNotShareSessions(t *testing.T) {
	readInv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionReadRowResponse{}}}
	writeInv := &fakeInvoker{result: btransport.InvokeResult{Response: &btpb.SessionMutateRowResponse{}}}
	tbl, _ := newTestTable(t, readInv, writeInv)

	if _, err := tbl.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("row1")}); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	if got, want := readInv.callCount(), 1; got != want {
		t.Errorf("readInv.calls after ReadRow = %d, want %d", got, want)
	}
	if got := writeInv.callCount(); got != 0 {
		t.Errorf("writeInv.calls after ReadRow = %d, want 0 — write pool MUST NOT receive read traffic", got)
	}

	req := &btpb.SessionMutateRowRequest{
		Key: []byte("row1"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q"),
				TimestampMicros: 1_000_000,
				Value:           []byte("v"),
			}},
		}},
	}
	if _, err := tbl.MutateRow(context.Background(), req); err != nil {
		t.Fatalf("MutateRow: %v", err)
	}
	if got, want := writeInv.callCount(), 1; got != want {
		t.Errorf("writeInv.calls after MutateRow = %d, want %d", got, want)
	}
	if got := readInv.callCount(); got != 1 {
		t.Errorf("readInv.calls after MutateRow = %d, want still 1 — read pool MUST NOT receive write traffic", got)
	}
}

// --- Close teardown ---------------------------------------------------------

// TestSessionTable_Close_CallsBothReleasers verifies Close fires the
// closeRead and closeWrite release closures exactly once each.
func TestSessionTable_Close_CallsBothReleasers(t *testing.T) {
	var reads, writes int
	tbl := newSessionTable(
		"t",
		func() (Invoker, error) { return nil, nil },
		func() (Invoker, error) { return nil, nil },
		func() error { reads++; return nil },
		func() error { writes++; return nil },
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		&btransport.VRpcDescriptorImpl{MethodName: "test.MutateRow"},
		nil, nil,
	)
	if err := tbl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if reads != 1 || writes != 1 {
		t.Errorf("release call counts = (read=%d write=%d), want (1,1)", reads, writes)
	}
}

// TestSessionTable_Close_NilWriteReleaserOK covers the materialized-
// view case: no write side means closeWrite is nil; Close must not
// panic and must still call closeRead.
func TestSessionTable_Close_NilWriteReleaserOK(t *testing.T) {
	var reads int
	tbl := newSessionTable(
		"",
		func() (Invoker, error) { return nil, nil },
		nil,
		func() error { reads++; return nil },
		nil,
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		nil,
		nil, nil,
	)
	if err := tbl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if reads != 1 {
		t.Errorf("closeRead called %d times, want 1", reads)
	}
}

// TestSessionTable_Close_JoinsErrors: when BOTH release closures
// return errors, Close returns them via errors.Join so callers see
// both root causes rather than losing one to a first-error return.
func TestSessionTable_Close_JoinsErrors(t *testing.T) {
	readErr := status.Error(codes.Internal, "read pool teardown failed")
	writeErr := status.Error(codes.Internal, "write pool teardown failed")
	tbl := newSessionTable(
		"t",
		func() (Invoker, error) { return nil, nil },
		func() (Invoker, error) { return nil, nil },
		func() error { return readErr },
		func() error { return writeErr },
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		&btransport.VRpcDescriptorImpl{MethodName: "test.MutateRow"},
		nil, nil,
	)
	err := tbl.Close()
	if err == nil {
		t.Fatal("Close returned nil, want joined errors")
	}
	if got := err.Error(); !strings.Contains(got, "read pool teardown failed") || !strings.Contains(got, "write pool teardown failed") {
		t.Errorf("Close error = %q; want both read+write causes surfaced", got)
	}
}

// TestSessionTable_Close_ReleasersIdempotent verifies a second Close
// call still runs the release closures (they're idempotent at the
// sessionClient layer — the poolCloser behind each is single-shot, so a
// second call no-ops). The point is that sessionTable.Close
// itself does not gate on a once — the underlying release is where
// idempotency lives.
func TestSessionTable_Close_ReleasersIdempotent(t *testing.T) {
	var reads int
	tbl := newSessionTable(
		"t",
		func() (Invoker, error) { return nil, nil },
		nil,
		func() error { reads++; return nil },
		nil,
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		nil,
		nil, nil,
	)
	if err := tbl.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}
	if err := tbl.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}
	if reads != 2 {
		t.Errorf("closeRead called %d times across two Close calls, want 2 (release is caller-idempotent, not sessionTable-once-guarded)", reads)
	}
}

// TestSessionTable_Close_RacingLazyOpen_NoLeak proves the guardOpen
// wrapper cleans up a fresh pool that the underlying opener inserted
// AFTER Close's releaser already no-op'd on an empty map. Reproduces
// PR #20264 review comment (high): openRead's slow path straddles
// Close, releaser sees no entry, opener finishes and inserts → leak.
//
// Interleaving:
//  1. Goroutine A calls readPool.get() → guardOpen check passes
//     (closed=false) → openRead blocks on unblockOpen chan.
//  2. Main calls tbl.Close() → sets closed=true → invokes closeRead
//     which increments releaseCalls; simulated "map lookup misses"
//     because openRead hasn't inserted yet (fake release no-ops but
//     the counter records the attempt).
//  3. Main unblocks openRead. openRead returns "inserted a pool".
//  4. guardOpen's post-check sees closed=true → calls release AGAIN
//     to clean up its own insert. releaseCalls == 2.
//  5. Return ErrClientClosed to the caller.
//
// Without the fix: releaseCalls == 1 (Close-side only), openRead
// returned a fake pool that no one owns → leak.
func TestSessionTable_Close_RacingLazyOpen_NoLeak(t *testing.T) {
	unblockOpen := make(chan struct{})
	openStarted := make(chan struct{})
	var releaseCalls atomic.Int32
	// Sentinel invoker returned by the fake openRead — proves the opener
	// actually completed and returned a "pool" that would leak absent the
	// post-check.
	fakeInv := (Invoker)(nil)

	tbl := newSessionTable(
		"t",
		func() (Invoker, error) {
			close(openStarted)
			<-unblockOpen
			return fakeInv, nil
		},
		nil,
		func() error { releaseCalls.Add(1); return nil },
		nil,
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		nil,
		nil, nil,
	)

	var (
		wg         sync.WaitGroup
		gotErr     error
		gotInvoker Invoker
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		gotInvoker, gotErr = tbl.readPool.get()
	}()

	<-openStarted // opener is blocked, guarantees Close runs first
	if err := tbl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(unblockOpen)
	wg.Wait()

	if !errors.Is(gotErr, ErrClientClosed) {
		t.Errorf("readPool.get() err = %v, want ErrClientClosed", gotErr)
	}
	if gotInvoker != nil {
		t.Errorf("readPool.get() returned non-nil invoker %v, want nil", gotInvoker)
	}
	if got := releaseCalls.Load(); got != 2 {
		t.Errorf("release called %d times, want 2 (once by Close, once by guardOpen post-check cleanup)", got)
	}
}

// TestSessionTable_Close_BeforeLazyOpen_EarlyBail proves the guardOpen
// early check short-circuits when Close ran before the opener ever
// started. openRead is never invoked.
func TestSessionTable_Close_BeforeLazyOpen_EarlyBail(t *testing.T) {
	var openCalled atomic.Int32
	var releaseCalls atomic.Int32
	tbl := newSessionTable(
		"t",
		func() (Invoker, error) { openCalled.Add(1); return nil, nil },
		nil,
		func() error { releaseCalls.Add(1); return nil },
		nil,
		&btransport.VRpcDescriptorImpl{MethodName: "test.ReadRow"},
		nil,
		nil, nil,
	)

	if err := tbl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	inv, err := tbl.readPool.get()
	if !errors.Is(err, ErrClientClosed) {
		t.Errorf("readPool.get() after Close err = %v, want ErrClientClosed", err)
	}
	if inv != nil {
		t.Errorf("readPool.get() after Close inv = %v, want nil", inv)
	}
	if openCalled.Load() != 0 {
		t.Errorf("openRead was called %d times after Close, want 0 (early bail should short-circuit)", openCalled.Load())
	}
	if releaseCalls.Load() != 1 {
		t.Errorf("release called %d times, want 1 (Close's own release only; guardOpen never entered inner)", releaseCalls.Load())
	}
}
