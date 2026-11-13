// Copyright 2026 Google LLC
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

package requestid

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestRandIDForProcessIsHex(t *testing.T) {
	pid := RandIDForProcess()
	if len(pid) != 16 {
		t.Fatalf("RandIDForProcess() length = %d, want 16 (%q)", len(pid), pid)
	}
	if _, err := hex.DecodeString(pid); err != nil {
		t.Fatalf("RandIDForProcess() = %q is not valid hex: %v", pid, err)
	}
}

func TestRequestIDAndCallOptions(t *testing.T) {
	reqID := RequestID("1.proc.7.3.19.4")
	if got, want := reqID.LogicalRequestKey(), "1.proc.7.3.19"; got != want {
		t.Fatalf("LogicalRequestKey() = %q, want %q", got, want)
	}

	var nth atomic.Uint32
	tr := NewTracker(2, 4, &nth)
	w := tr.NewWrap()
	opt1 := w.WithNextRetryAttempt(1)
	if !CallOptionsHaveRequestID([]gax.CallOption{opt1}) {
		t.Fatal("expected CallOptionsHaveRequestID to be true")
	}
	wantKey := "1." + RandIDForProcess() + ".2.4.1"
	if got := LogicalRequestKeyFromCallOptions([]gax.CallOption{opt1}); got != wantKey {
		t.Fatalf("LogicalRequestKeyFromCallOptions() = %q, want %q", got, wantKey)
	}

	opt2 := LogicalWrap{LogicalKey: "1.proc.9.2.44"}.WithNextRetryAttempt(3)
	var cs gax.CallSettings
	opt1.Resolve(&cs)
	opt2.Resolve(&cs)
	_, gotReqID, found := GRPCCallOptionsToRequestID(cs.GRPC)
	if !found || string(gotReqID) != "1.proc.9.2.44.3" {
		t.Fatalf("expected overridden request ID 1.proc.9.2.44.3, got %q (found=%v)", gotReqID, found)
	}

	// Verify that CallOptionsHaveRequestID and repeated OptsWithNextRequestID do not
	// mutate NthRequest before Resolve is called.
	var retryNth atomic.Uint32
	retryTr := NewTracker(3, 5, &retryNth)
	retryOpts := retryTr.OptsWithNextRequestID(nil)
	if got := retryNth.Load(); got != 0 {
		t.Fatalf("expected NthRequest=0 before Resolve, got %d", got)
	}
	if !CallOptionsHaveRequestID(retryOpts) {
		t.Fatal("expected CallOptionsHaveRequestID to be true for retryerWithRequestID")
	}
	retryOpts = retryTr.OptsWithNextRequestID(retryOpts)
	if len(retryOpts) != 1 {
		t.Fatalf("expected OptsWithNextRequestID to be idempotent, got len=%d", len(retryOpts))
	}
	if got := retryNth.Load(); got != 0 {
		t.Fatalf("expected CallOptionsHaveRequestID not to increment NthRequest, got %d", got)
	}
}

func TestHeaderInjectorAugmentsError(t *testing.T) {
	inj := NewHeaderInjector(func(err error, reqID string) error {
		return errors.New(err.Error() + " [reqID=" + reqID + "]")
	})
	var nth atomic.Uint32
	tr := NewTracker(1, 1, &nth)
	opts := tr.OptsWithNextRequestID(nil)
	var cs gax.CallSettings
	for _, o := range opts {
		o.Resolve(&cs)
	}

	var seenIncoming string
	err := inj.InterceptUnary(
		context.Background(),
		"/google.spanner.v1.Spanner/ExecuteSql",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			if md, ok := metadata.FromOutgoingContext(ctx); ok {
				if vals := md.Get(Header); len(vals) > 0 {
					seenIncoming = vals[0]
				}
			}
			return errors.New("boom")
		},
		cs.GRPC...,
	)
	if err == nil || !strings.Contains(err.Error(), "[reqID="+seenIncoming+"]") || seenIncoming == "" {
		t.Fatalf("expected augmented error with reqID %q, got %v", seenIncoming, err)
	}
}
