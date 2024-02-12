// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/descriptorpb"
)

type testRecvResponse struct {
	resp *storagepb.AppendRowsResponse
	err  error
}

type testAppendRowsClient struct {
	storagepb.BigQueryWrite_AppendRowsClient
	openCount int
	requests  []*storagepb.AppendRowsRequest
	responses []*testRecvResponse
	sendF     func(*storagepb.AppendRowsRequest) error
	recvF     func() (*storagepb.AppendRowsResponse, error)
	closeF    func() error
}

func (tarc *testAppendRowsClient) Send(req *storagepb.AppendRowsRequest) error {
	return tarc.sendF(req)
}

func (tarc *testAppendRowsClient) Recv() (*storagepb.AppendRowsResponse, error) {
	return tarc.recvF()
}

func (tarc *testAppendRowsClient) CloseSend() error {
	return tarc.closeF()
}

// openTestArc handles wiring in a test AppendRowsClient into a managedstream by providing the open function.
func openTestArc(testARC *testAppendRowsClient, sendF func(req *storagepb.AppendRowsRequest) error, recvF func() (*storagepb.AppendRowsResponse, error)) func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
	sF := func(req *storagepb.AppendRowsRequest) error {
		testARC.requests = append(testARC.requests, req)
		return nil
	}
	if sendF != nil {
		sF = sendF
	}
	rF := func() (*storagepb.AppendRowsResponse, error) {
		return &storagepb.AppendRowsResponse{
			Response: &storagepb.AppendRowsResponse_AppendResult_{},
		}, nil
	}
	if recvF != nil {
		rF = recvF
	}
	testARC.sendF = sF
	testARC.recvF = rF
	testARC.closeF = func() error {
		return nil
	}
	return func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
		testARC.openCount = testARC.openCount + 1
		// Simulate grpc finalizer goroutine
		go func() {
			<-ctx.Done()
		}()
		return testARC, nil
	}
}

func TestManagedStream_RequestOptimization(t *testing.T) {

	ctx := context.Background()
	testARC := &testAppendRowsClient{}
	pool := &connectionPool{
		ctx:                ctx,
		open:               openTestArc(testARC, nil, nil),
		baseFlowController: newFlowController(0, 0),
	}
	if err := pool.activateRouter(newSimpleRouter("")); err != nil {
		t.Errorf("activateRouter: %v", err)
	}
	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
	}
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}
	ms.streamSettings.streamID = "FOO"
	ms.streamSettings.TraceID = "TRACE"
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))

	fakeData := [][]byte{
		[]byte("foo"),
		[]byte("bar"),
	}

	wantReqs := 3

	for i := 0; i < wantReqs; i++ {
		_, err := ms.AppendRows(ctx, fakeData, WithOffset(int64(i)))
		if err != nil {
			t.Errorf("AppendRows; %v", err)
		}
	}

	if testARC.openCount != 1 {
		t.Errorf("expected a single open, got %d", testARC.openCount)
	}

	if len(testARC.requests) != wantReqs {
		t.Errorf("expected %d requests, got %d", wantReqs, len(testARC.requests))
	}

	for k, v := range testARC.requests {
		if v == nil {
			t.Errorf("request %d was nil", k)
		}
		if v.GetOffset() == nil {
			t.Errorf("request %d had no offset", k)
		} else {
			gotOffset := v.GetOffset().GetValue()
			if gotOffset != int64(k) {
				t.Errorf("request %d wanted offset %d, got %d", k, k, gotOffset)
			}
		}
		if k == 0 {
			if v.GetTraceId() == "" {
				t.Errorf("expected TraceId on first request, was empty")
			}
			if v.GetWriteStream() == "" {
				t.Errorf("expected WriteStream on first request, was empty")
			}
			if v.GetProtoRows().GetWriterSchema().GetProtoDescriptor() == nil {
				t.Errorf("expected WriterSchema on first request, was empty")
			}

		} else {
			// TODO: add validation to ensure we're optimizing requests on the wire.
			// Sending consecutive requests with same dest/schema we should redact.
		}
	}
}

func TestManagedStream_FlowControllerFailure(t *testing.T) {

	ctx := context.Background()

	pool := &connectionPool{
		ctx:                ctx,
		open:               openTestArc(&testAppendRowsClient{}, nil, nil),
		baseFlowController: newFlowController(1, 0),
	}
	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
	}
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWritre: %v", err)
	}

	// Exhaust inflight requests on the single connection.
	router.conn.fc = newFlowController(1, 0)
	router.conn.fc.acquire(ctx, 0)

	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))

	fakeData := [][]byte{
		[]byte("foo"),
		[]byte("bar"),
	}

	// Create a context that will expire during the append.
	// This is expected to surface a flowcontroller error, as there's no
	// capacity.
	expireCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err := ms.AppendRows(expireCtx, fakeData)
	if err == nil {
		t.Errorf("expected AppendRows to error, but it succeeded")
	}
}

func TestManagedStream_AppendWithDeadline(t *testing.T) {
	ctx := context.Background()

	pool := &connectionPool{
		ctx:                ctx,
		baseFlowController: newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append is intentionally slow.
				time.Sleep(200 * time.Millisecond)
				return nil
			}, nil),
	}
	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
	}
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}
	conn := router.conn
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))

	fakeData := [][]byte{
		[]byte("foo"),
	}

	wantCount := 0
	if ct := conn.fc.count(); ct != wantCount {
		t.Errorf("flowcontroller count mismatch, got %d want %d", ct, wantCount)
	}

	// Create a context that will expire during the append, to verify the passed in
	// context expires.
	expireCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := ms.AppendRows(expireCtx, fakeData)
	if err == nil {
		t.Errorf("expected AppendRows to error, but it succeeded")
	}

	// We expect the flowcontroller count to still be occupied, as the Send is slow.
	wantCount = 1
	if ct := conn.fc.count(); ct != wantCount {
		t.Errorf("flowcontroller post-append count mismatch, got %d want %d", ct, wantCount)
	}

	// Wait for the append to finish, then check again.
	time.Sleep(300 * time.Millisecond)
	wantCount = 0
	if ct := conn.fc.count(); ct != wantCount {
		t.Errorf("flowcontroller post-append count mismatch, got %d want %d", ct, wantCount)
	}
}

func TestManagedStream_ContextExpiry(t *testing.T) {
	// Issue: retaining error from append as stream error
	// https://github.com/googleapis/google-cloud-go/issues/6657
	ctx := context.Background()

	pool := &connectionPool{
		ctx:                ctx,
		baseFlowController: newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				return nil
			}, nil),
	}
	if err := pool.activateRouter(newSimpleRouter("")); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
	}
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	fakeData := [][]byte{
		[]byte("foo"),
	}
	fakeReq := &storagepb.AppendRowsRequest{
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: fakeData,
				},
			},
		},
	}

	// Create a context and immediately cancel it.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	// First, append with an invalid context.
	pw := newPendingWrite(cancelCtx, ms, fakeReq, ms.curTemplate, "", "")
	err := ms.appendWithRetry(pw)
	if err != context.Canceled {
		t.Errorf("expected cancelled context error, got: %v", err)
	}

	// a second append with a valid context should succeed
	_, err = ms.AppendRows(ctx, fakeData)
	if err != nil {
		t.Errorf("expected second append to succeed, but failed: %v", err)
	}
}

func TestManagedStream_AppendDeadlocks(t *testing.T) {
	// Ensure we don't deadlock by issing two appends.
	testCases := []struct {
		desc       string
		openErrors []error
		ctx        context.Context
		respErr    error
	}{
		{
			desc:       "no errors",
			openErrors: []error{nil, nil},
			ctx:        context.Background(),
			respErr:    nil,
		},
		{
			desc:       "cancelled caller context",
			openErrors: []error{nil, nil},
			ctx: func() context.Context {
				cctx, cancel := context.WithCancel(context.Background())
				cancel()
				return cctx
			}(),
			respErr: context.Canceled,
		},
		{
			desc:       "expired caller context",
			openErrors: []error{nil, nil},
			ctx: func() context.Context {
				cctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
				defer cancel()
				time.Sleep(2 * time.Millisecond)
				return cctx
			}(),
			respErr: context.DeadlineExceeded,
		},
		{
			desc:       "errored getstream",
			openErrors: []error{status.Errorf(codes.ResourceExhausted, "some error"), status.Errorf(codes.ResourceExhausted, "some error")},
			ctx:        context.Background(),
			respErr:    status.Errorf(codes.ResourceExhausted, "some error"),
		},
	}

	for _, tc := range testCases {
		ctx := context.Background()
		openF := openTestArc(&testAppendRowsClient{}, nil, nil)
		pool := &connectionPool{
			ctx: ctx,
			open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				if len(tc.openErrors) == 0 {
					panic("out of open errors")
				}
				curErr := tc.openErrors[0]
				tc.openErrors = tc.openErrors[1:]
				if curErr == nil {
					return openF(ctx, opts...)
				}
				return nil, curErr
			},
		}
		router := newSimpleRouter("")
		if err := pool.activateRouter(router); err != nil {
			t.Errorf("activateRouter: %v", err)
		}
		ms := &ManagedStream{
			id: "foo",
			streamSettings: &streamSettings{
				streamID: "foo",
			},
		}
		ms.ctx, ms.cancel = context.WithCancel(pool.ctx)
		if err := pool.addWriter(ms); err != nil {
			t.Errorf("addWriter: %v", err)
		}

		testReq := ms.buildRequest([][]byte{[]byte("foo")})
		// first append
		pw := newPendingWrite(tc.ctx, ms, testReq, nil, "", "")
		gotErr := ms.appendWithRetry(pw)
		if !errors.Is(gotErr, tc.respErr) {
			t.Errorf("%s first response: got %v, want %v", tc.desc, gotErr, tc.respErr)
		}
		// second append
		pw = newPendingWrite(tc.ctx, ms, testReq, nil, "", "")
		gotErr = ms.appendWithRetry(pw)
		if !errors.Is(gotErr, tc.respErr) {
			t.Errorf("%s second response: got %v, want %v", tc.desc, gotErr, tc.respErr)
		}

		// Issue two closes, to ensure we're not deadlocking there either.
		ms.Close()
		ms.Close()

		// Issue two more appends, ensure we're not deadlocked as the writer is closed.
		gotErr = ms.appendWithRetry(pw)
		if !errors.Is(gotErr, io.EOF) {
			t.Errorf("expected io.EOF, got %v", gotErr)
		}
		gotErr = ms.appendWithRetry(pw)
		if !errors.Is(gotErr, io.EOF) {
			t.Errorf("expected io.EOF, got %v", gotErr)
		}

	}

}

func TestManagedStream_LeakingGoroutines(t *testing.T) {
	ctx := context.Background()

	pool := &connectionPool{
		ctx: ctx,
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append is intentionally slower than context to cause pressure.
				time.Sleep(40 * time.Millisecond)
				return nil
			}, nil),
		baseFlowController: newFlowController(10, 0),
	}
	if err := pool.activateRouter(newSimpleRouter("")); err != nil {
		t.Errorf("activateRouter: %v", err)
	}
	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
	}
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	fakeData := [][]byte{
		[]byte("foo"),
	}

	threshold := runtime.NumGoroutine() + 20

	// Send a bunch of appends that expire quicker than response, and monitor that
	// goroutine growth stays within bounded threshold.
	for i := 0; i < 500; i++ {
		expireCtx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
		defer cancel()
		ms.AppendRows(expireCtx, fakeData)
		if i%50 == 0 {
			if current := runtime.NumGoroutine(); current > threshold {
				t.Errorf("potential goroutine leak, append %d: current %d, threshold %d", i, current, threshold)
			}
		}
	}
}

func TestManagedStream_LeakingGoroutinesReconnect(t *testing.T) {
	ctx := context.Background()

	reqCount := 0
	testArc := &testAppendRowsClient{}
	pool := &connectionPool{
		ctx: ctx,
		open: openTestArc(testArc,
			func(req *storagepb.AppendRowsRequest) error {
				reqCount++
				if reqCount%2 == 1 {
					return io.EOF
				}
				return nil
			}, nil),
		baseFlowController: newFlowController(1000, 0),
	}
	if err := pool.activateRouter(newSimpleRouter("")); err != nil {
		t.Errorf("activateRouter: %v", err)
	}
	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		retry:          newStatelessRetryer(),
	}
	ms.retry.maxAttempts = 4
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	fakeData := [][]byte{
		[]byte("foo"),
	}

	threshold := runtime.NumGoroutine() + 5

	// Send a bunch of appends that will trigger reconnects and monitor that
	// goroutine growth stays within bounded threshold.
	for i := 0; i < 100; i++ {
		writeCtx := context.Background()
		r, err := ms.AppendRows(writeCtx, fakeData)
		if err != nil {
			t.Fatalf("failed to append row: %v", err)
		}
		_, err = r.GetResult(context.Background())
		if err != nil {
			t.Fatalf("failed to get result: %v", err)
		}
		if r.totalAttempts != 2 {
			t.Fatalf("should trigger a retry, but found: %d attempts", r.totalAttempts)
		}
		if testArc.openCount != i+2 {
			t.Errorf("should trigger a reconnect, but found openCount %d", testArc.openCount)
		}
		if i%10 == 0 {
			if current := runtime.NumGoroutine(); current > threshold {
				t.Errorf("potential goroutine leak, append %d: current %d, threshold %d", i, current, threshold)
			}
		}
	}
}

func TestManagedWriter_CancellationDuringRetry(t *testing.T) {
	// Issue: double close of pending write.
	// https://github.com/googleapis/google-cloud-go/issues/7380
	ctx, cancel := context.WithCancel(context.Background())
	pool := &connectionPool{
		ctx: ctx,
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append doesn't error, but is slow.
				time.Sleep(time.Second)
				return nil
			},
			func() (*storagepb.AppendRowsResponse, error) {
				// Response is slow and always returns a retriable error.
				time.Sleep(2 * time.Second)
				return nil, io.EOF
			}),
		baseFlowController: newFlowController(10, 0),
	}
	if err := pool.activateRouter(newSimpleRouter("")); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		retry:          newStatelessRetryer(),
	}
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	fakeData := [][]byte{
		[]byte("foo"),
	}

	res, err := ms.AppendRows(context.Background(), fakeData)
	if err != nil {
		t.Errorf("AppendRows send err: %v", err)
	}
	cancel()

	select {

	case <-res.Ready():
		if _, err := res.GetResult(context.Background()); err == nil {
			t.Errorf("expected failure, got success")
		}

	case <-time.After(5 * time.Second):
		t.Errorf("result was not ready in expected time")
	}
}

func TestManagedStream_Closure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &connectionPool{
		ctx:                ctx,
		cancel:             cancel,
		baseFlowController: newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				return nil
			}, nil),
	}
	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		streamSettings: defaultStreamSettings(),
	}
	ms.ctx, ms.cancel = context.WithCancel(pool.ctx)
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter A: %v", err)
	}

	if router.conn == nil {
		t.Errorf("expected non-nil connection")
	}

	if err := ms.Close(); err != io.EOF {
		t.Errorf("msB.Close, want %v got %v", io.EOF, err)
	}
	if router.conn != nil {
		t.Errorf("expected nil connection")
	}
	if ms.ctx.Err() == nil {
		t.Errorf("expected writer ctx to be dead, is alive")
	}
}

// This test exists to try to surface data races by sharing
// a single writer with multiple goroutines.  It doesn't assert
// anything about the behavior of the system.
func TestManagedStream_RaceFinder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var totalsMu sync.Mutex
	totalSends := 0
	totalRecvs := 0
	pool := &connectionPool{
		ctx:                ctx,
		cancel:             cancel,
		baseFlowController: newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				totalsMu.Lock()
				totalSends = totalSends + 1
				curSends := totalSends
				totalsMu.Unlock()
				if curSends%25 == 0 {
					//time.Sleep(10 * time.Millisecond)
					return io.EOF
				}
				return nil
			},
			func() (*storagepb.AppendRowsResponse, error) {
				totalsMu.Lock()
				totalRecvs = totalRecvs + 1
				curRecvs := totalRecvs
				totalsMu.Unlock()
				if curRecvs%15 == 0 {
					return nil, io.EOF
				}
				return &storagepb.AppendRowsResponse{}, nil
			}),
	}
	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		id:             "foo",
		streamSettings: defaultStreamSettings(),
		retry:          newStatelessRetryer(),
	}
	ms.retry.maxAttempts = 4
	ms.ctx, ms.cancel = context.WithCancel(pool.ctx)
	ms.curTemplate = newVersionedTemplate().revise(reviseProtoSchema(&descriptorpb.DescriptorProto{}))
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter A: %v", err)
	}

	if router.conn == nil {
		t.Errorf("expected non-nil connection")
	}

	numWriters := 5
	numWrites := 50

	var wg sync.WaitGroup
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func() {
			for j := 0; j < numWrites; j++ {
				result, err := ms.AppendRows(ctx, [][]byte{[]byte("foo")})
				if err != nil {
					continue
				}
				_, err = result.GetResult(ctx)
				if err != nil {
					continue
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
	cancel()
}
