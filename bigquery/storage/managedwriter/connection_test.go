// Copyright 2022 Google LLC
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
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConnection_OpenWithRetry(t *testing.T) {

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
		pool := &connectionPool{
			ctx: context.Background(),
			open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
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
		if err := pool.activateRouter(newSimpleRouter("")); err != nil {
			t.Errorf("activateRouter: %v", err)
		}
		writer := &ManagedStream{id: "foo"}
		if err := pool.addWriter(writer); err != nil {
			t.Errorf("addWriter: %v", err)
		}

		conn, err := pool.router.pickConnection(nil)
		if err != nil {
			t.Errorf("case %s, failed to add connection: %v", tc.desc, err)
		}
		arc, ch, err := pool.openWithRetry(conn)
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

// Ensures we don't lose track of channels/connections during reconnects.
// https://github.com/googleapis/google-cloud-go/issues/6766
func TestConnection_LeakingReconnect(t *testing.T) {

	ctx := context.Background()

	pool := &connectionPool{
		ctx:                ctx,
		baseFlowController: newFlowController(10, 0),
		open: openTestArc(&testAppendRowsClient{},
			func(req *storagepb.AppendRowsRequest) error {
				// Append always reports EOF on send.
				return io.EOF
			}, nil),
	}
	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}
	writer := &ManagedStream{id: "foo"}
	if err := pool.addWriter(writer); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	var chans []chan *pendingWrite

	for i := 0; i < 10; i++ {
		_, ch, err := router.conn.getStream(nil, true)
		if err != nil {
			t.Fatalf("failed getStream(%d): %v", i, err)
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
func TestConnectionPool_OpenCallOptionPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pool := &connectionPool{
		ctx:    ctx,
		cancel: cancel,
		open: createOpenF(func(ctx context.Context, opts ...gax.CallOption) (storage.BigQueryWrite_AppendRowsClient, error) {
			if len(opts) == 0 {
				t.Fatalf("no options were propagated")
			}
			return nil, fmt.Errorf("no real client")
		}, ""),
		callOptions: []gax.CallOption{
			gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(99)),
		},
	}
	conn := newConnection(pool, "", nil)
	pool.openWithRetry(conn)
}

// This test evaluates how the receiver deals with a pending write.
func TestConnection_Receiver(t *testing.T) {

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
			wantFinalErr: func() error {
				return status.ErrorProto(&statuspb.Status{
					Code:    int32(codes.ResourceExhausted),
					Message: "foo",
				})
			}(),
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

		pool := &connectionPool{
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
			baseFlowController: newFlowController(0, 0),
		}
		router := newSimpleRouter("")
		if err := pool.activateRouter(router); err != nil {
			t.Errorf("activateRouter: %v", err)
		}

		ms := &ManagedStream{
			id:    "foo",
			ctx:   ctx,
			retry: newStatelessRetryer(),
		}
		if err := pool.addWriter(ms); err != nil {
			t.Errorf("addWriter: %v", err)
		}
		conn := router.conn
		// use openWithRetry to get the reference to the channel and add our test pending write.
		_, ch, _ := pool.openWithRetry(conn)
		pw := newPendingWrite(ctx, ms, &storagepb.AppendRowsRequest{}, nil, "", "")
		pw.writer = ms
		pw.attemptCount = 1 // we're injecting directly, but attribute this as a single attempt.
		ch <- pw

		// Wait until the write is marked done.
		<-pw.result.Ready()

		// Check retry count is as expected.
		gotTotalAttempts, err := pw.result.TotalAttempts(ctx)
		if err != nil {
			t.Errorf("%s: failed to get total attempts: %v", tc.description, err)
		}
		if gotTotalAttempts != tc.wantTotalAttempts {
			t.Errorf("%s: got %d total attempts, want %d attempts", tc.description, gotTotalAttempts, tc.wantTotalAttempts)
		}

		// Check that the write got the expected final result.
		if gotFinalErr := pw.result.err; !errors.Is(gotFinalErr, tc.wantFinalErr) {
			t.Errorf("%s: got final error %v, wanted final error %v", tc.description, gotFinalErr, tc.wantFinalErr)
		}
		cancel()
	}
}
