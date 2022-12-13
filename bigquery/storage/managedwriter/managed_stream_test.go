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
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestManagedStream_OpenWithRetry(t *testing.T) {

	testCases := []struct {
		desc     string
		errors   []error
		wantFail bool
	}{
		{
			desc:     "no error",
			errors:   []error{nil},
			wantFail: false,
		},
		{
			desc: "transient failures",
			errors: []error{
				status.Errorf(codes.Unavailable, "try 1"),
				status.Errorf(codes.Unavailable, "try 2"),
				nil},
			wantFail: false,
		},
		{
			desc:     "terminal error",
			errors:   []error{status.Errorf(codes.InvalidArgument, "bad args")},
			wantFail: true,
		},
	}

	for _, tc := range testCases {
		ms := &ManagedStream{
			ctx: context.Background(),
			open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				if len(tc.errors) == 0 {
					panic("out of errors")
				}
				err := tc.errors[0]
				tc.errors = tc.errors[1:]
				if err == nil {
					return &testAppendRowsClient{}, nil
				}
				return nil, err
			},
		}
		arc, ch, err := ms.openWithRetry()
		if tc.wantFail && err == nil {
			t.Errorf("case %s: wanted failure, got success", tc.desc)
		}
		if !tc.wantFail && err != nil {
			t.Errorf("case %s: wanted success, got %v", tc.desc, err)
		}
		if err == nil {
			if arc == nil {
				t.Errorf("case %s: expected append client, got nil", tc.desc)
			}
			if ch == nil {
				t.Errorf("case %s: expected channel, got nil", tc.desc)
			}
		}
	}
}

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
func openTestArc(testARC *testAppendRowsClient, sendF func(req *storagepb.AppendRowsRequest) error, recvF func() (*storagepb.AppendRowsResponse, error)) func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
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
	return func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
		testARC.openCount = testARC.openCount + 1
		return testARC, nil
	}
}

func TestManagedStream_FirstAppendBehavior(t *testing.T) {

	ctx := context.Background()

	testARC := &testAppendRowsClient{}
	ms := &ManagedStream{
		ctx:            ctx,
		open:           openTestArc(testARC, nil, nil),
		streamSettings: defaultStreamSettings(),
		fc:             newFlowController(0, 0),
	}
	ms.streamSettings.streamID = "FOO"
	ms.streamSettings.TraceID = "TRACE"
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
	}

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
			if v.GetTraceId() != "" {
				t.Errorf("expected no TraceID on request %d, got %s", k, v.GetTraceId())
			}
			if v.GetWriteStream() != "" {
				t.Errorf("expected no WriteStream on request %d, got %s", k, v.GetWriteStream())
			}
			if v.GetProtoRows().GetWriterSchema().GetProtoDescriptor() != nil {
				t.Errorf("expected test WriterSchema on request %d, got %s", k, v.GetProtoRows().GetWriterSchema().GetProtoDescriptor().String())
			}
		}
	}
}

func TestManagedStream_FlowControllerFailure(t *testing.T) {

	ctx := context.Background()

	// create a flowcontroller with 1 inflight message allowed, and exhaust it.
	fc := newFlowController(1, 0)
	fc.acquire(ctx, 0)

	ms := &ManagedStream{
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		fc:             fc,
		open:           openTestArc(&testAppendRowsClient{}, nil, nil),
	}
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
	}

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

	ms := &ManagedStream{
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		fc:             newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append is intentionally slow.
				time.Sleep(200 * time.Millisecond)
				return nil
			}, nil),
	}
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
	}

	fakeData := [][]byte{
		[]byte("foo"),
	}

	wantCount := 0
	if ct := ms.fc.count(); ct != wantCount {
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
	if ct := ms.fc.count(); ct != wantCount {
		t.Errorf("flowcontroller post-append count mismatch, got %d want %d", ct, wantCount)
	}

	// Wait for the append to finish, then check again.
	time.Sleep(300 * time.Millisecond)
	wantCount = 0
	if ct := ms.fc.count(); ct != wantCount {
		t.Errorf("flowcontroller post-append count mismatch, got %d want %d", ct, wantCount)
	}
}

func TestManagedStream_ContextExpiry(t *testing.T) {
	// Issue: retaining error from append as stream error
	// https://github.com/googleapis/google-cloud-go/issues/6657
	ctx := context.Background()

	ms := &ManagedStream{
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		fc:             newFlowController(0, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append is intentionally slow.
				return nil
			}, nil),
	}
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
	}
	fakeData := [][]byte{
		[]byte("foo"),
	}

	// Create a context and immediately cancel it.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	// First, append with an invalid context.
	pw := newPendingWrite(cancelCtx, fakeData)
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
		openF := openTestArc(&testAppendRowsClient{}, nil, nil)
		ms := &ManagedStream{
			ctx: context.Background(),
			open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				if len(tc.openErrors) == 0 {
					panic("out of open errors")
				}
				curErr := tc.openErrors[0]
				tc.openErrors = tc.openErrors[1:]
				if curErr == nil {
					return openF(opts...)
				}
				return nil, curErr
			},
			streamSettings: &streamSettings{
				streamID: "foo",
			},
		}

		// first append
		pw := newPendingWrite(tc.ctx, [][]byte{[]byte("foo")})
		gotErr := ms.appendWithRetry(pw)
		if !errors.Is(gotErr, tc.respErr) {
			t.Errorf("%s first response: got %v, want %v", tc.desc, gotErr, tc.respErr)
		}
		// second append
		pw = newPendingWrite(tc.ctx, [][]byte{[]byte("bar")})
		gotErr = ms.appendWithRetry(pw)
		if !errors.Is(gotErr, tc.respErr) {
			t.Errorf("%s second response: got %v, want %v", tc.desc, gotErr, tc.respErr)
		}

		// Issue two closes, to ensure we're not deadlocking there either.
		ms.Close()
		ms.Close()
	}

}

func TestManagedStream_LeakingGoroutines(t *testing.T) {
	ctx := context.Background()

	ms := &ManagedStream{
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		fc:             newFlowController(10, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append is intentionally slower than context to cause pressure.
				time.Sleep(40 * time.Millisecond)
				return nil
			}, nil),
	}
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
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

// Ensures we don't lose track of channels/connections during reconnects.
// https://github.com/googleapis/google-cloud-go/issues/6766
func TestManagedStream_LeakingReconnect(t *testing.T) {

	ctx := context.Background()

	ms := &ManagedStream{
		ctx:            ctx,
		streamSettings: defaultStreamSettings(),
		fc:             newFlowController(10, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append always reports EOF on send.
				return io.EOF
			}, nil),
	}
	ms.schemaDescriptor = &descriptorpb.DescriptorProto{
		Name: proto.String("testDescriptor"),
	}

	var chans []chan *pendingWrite

	for i := 0; i < 10; i++ {
		_, ch, err := ms.getStream(nil, true)
		if err != nil {
			t.Fatalf("failed openWithRetry(%d): %v", i, err)
		}
		chans = append(chans, ch)
	}
	var closedCount int
	for _, ch := range chans {
		select {
		case _, ok := <-ch:
			if !ok {
				closedCount = closedCount + 1
			}
		case <-time.After(time.Second):
			// we blocked, likely indicative that the channel is open.
			continue
		}
	}
	if wantClosed := len(chans) - 1; wantClosed != closedCount {
		t.Errorf("closed count mismatch, got %d want %d", closedCount, wantClosed)
	}
}

// Ensures we're propagating call options as expected.
// Background: https://github.com/googleapis/google-cloud-go/issues/6487
func TestOpenCallOptionPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ms := &ManagedStream{
		ctx: ctx,
		callOptions: []gax.CallOption{
			gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(99)),
		},
		open: createOpenF(ctx, func(ctx context.Context, opts ...gax.CallOption) (storage.BigQueryWrite_AppendRowsClient, error) {
			if len(opts) == 0 {
				t.Fatalf("no options were propagated")
			}
			return nil, fmt.Errorf("no real client")
		}),
	}
	ms.openWithRetry()
}

// This test evaluates how the receiver deals with a pending write.
func TestManagedStream_Receiver(t *testing.T) {

	var customErr = fmt.Errorf("foo")

	testCases := []struct {
		description       string
		recvResp          []*testRecvResponse
		wantFinalErr      error
		wantTotalAttempts int
	}{
		{
			description: "no errors",
			recvResp: []*testRecvResponse{
				{
					resp: &storagepb.AppendRowsResponse{},
					err:  nil,
				},
			},
			wantTotalAttempts: 1,
		},
		{
			description: "recv err w/io.EOF",
			recvResp: []*testRecvResponse{
				{
					resp: nil,
					err:  io.EOF,
				},
				{
					resp: &storagepb.AppendRowsResponse{},
					err:  nil,
				},
			},
			wantTotalAttempts: 2,
		},
		{
			description: "recv err retried and then failed",
			recvResp: []*testRecvResponse{
				{
					resp: nil,
					err:  io.EOF,
				},
				{
					resp: nil,
					err:  customErr,
				},
			},
			wantTotalAttempts: 2,
			wantFinalErr:      customErr,
		},
		{
			description: "recv err w/ custom error",
			recvResp: []*testRecvResponse{
				{
					resp: nil,
					err:  customErr,
				},
				{
					resp: &storagepb.AppendRowsResponse{},
					err:  nil,
				},
			},
			wantTotalAttempts: 1,
			wantFinalErr:      customErr,
		},

		{
			description: "resp embeds Unavailable",
			recvResp: []*testRecvResponse{
				{
					resp: &storagepb.AppendRowsResponse{
						Response: &storagepb.AppendRowsResponse_Error{
							Error: &statuspb.Status{
								Code:    int32(codes.Unavailable),
								Message: "foo",
							},
						},
					},
					err: nil,
				},
				{
					resp: &storagepb.AppendRowsResponse{},
					err:  nil,
				},
			},
			wantTotalAttempts: 2,
		},
		{
			description: "resp embeds generic ResourceExhausted",
			recvResp: []*testRecvResponse{
				{
					resp: &storagepb.AppendRowsResponse{
						Response: &storagepb.AppendRowsResponse_Error{
							Error: &statuspb.Status{
								Code:    int32(codes.ResourceExhausted),
								Message: "foo",
							},
						},
					},
					err: nil,
				},
			},
			wantTotalAttempts: 1,
		},
		{
			description: "resp embeds throughput ResourceExhausted",
			recvResp: []*testRecvResponse{
				{
					resp: &storagepb.AppendRowsResponse{
						Response: &storagepb.AppendRowsResponse_Error{
							Error: &statuspb.Status{
								Code:    int32(codes.ResourceExhausted),
								Message: "Exceeds 'AppendRows throughput' quota for stream blah",
							},
						},
					},
					err: nil,
				},
				{
					resp: &storagepb.AppendRowsResponse{},
					err:  nil,
				},
			},
			wantTotalAttempts: 2,
		},
		{
			description: "retriable failures until max attempts",
			recvResp: []*testRecvResponse{
				{
					err: io.EOF,
				},
				{
					err: io.EOF,
				},
				{
					err: io.EOF,
				},
				{
					err: io.EOF,
				},
			},
			wantTotalAttempts: 4,
			wantFinalErr:      io.EOF,
		},
	}

	for _, tc := range testCases {
		ctx, cancel := context.WithCancel(context.Background())

		testArc := &testAppendRowsClient{
			responses: tc.recvResp,
		}

		ms := &ManagedStream{
			ctx: ctx,
			open: openTestArc(testArc, nil,
				func() (*storagepb.AppendRowsResponse, error) {
					if len(testArc.responses) == 0 {
						panic("out of responses")
					}
					curResp := testArc.responses[0]
					testArc.responses = testArc.responses[1:]
					return curResp.resp, curResp.err
				},
			),
			streamSettings: defaultStreamSettings(),
			fc:             newFlowController(0, 0),
			retry:          newStatelessRetryer(),
		}
		// use openWithRetry to get the reference to the channel and add our test pending write.
		_, ch, _ := ms.openWithRetry()
		pw := newPendingWrite(ctx, [][]byte{[]byte("foo")})
		pw.attemptCount = 1 // we're injecting directly, but attribute this as a single attempt.
		ch <- pw

		// Wait until the write is marked done.
		<-pw.result.Ready()

		// Check retry count is as expected.
		gotTotalAttempts, err := pw.result.TotalAttempts(ctx)
		if err != nil {
			t.Errorf("failed to get total appends: %v", err)
		}
		if gotTotalAttempts != tc.wantTotalAttempts {
			t.Errorf("%s: got %d total attempts, want %d attempts", tc.description, gotTotalAttempts, tc.wantTotalAttempts)
		}

		// Check that the write got the expected final result.
		if gotFinalErr := pw.result.err; !errors.Is(gotFinalErr, tc.wantFinalErr) {
			t.Errorf("%s: got final error %v, wanted final error %v", tc.description, gotFinalErr, tc.wantFinalErr)
		}
		ms.Close()
		cancel()
	}
}
