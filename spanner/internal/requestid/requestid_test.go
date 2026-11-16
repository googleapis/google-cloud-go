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
	"sync"
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

func TestNewTrackerNilNthRequestPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewTracker with nil nthRequest to panic")
		}
	}()
	_ = NewTracker(1, 1, nil)
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
	if !callOptionsHaveRequestID([]gax.CallOption{opt1}) {
		t.Fatal("expected callOptionsHaveRequestID to be true")
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

	// Zero-value callOption{} must be a safe no-op without stripping existing headers.
	callOption{}.Resolve(&cs)
	_, gotReqID, found = GRPCCallOptionsToRequestID(cs.GRPC)
	if !found || string(gotReqID) != "1.proc.9.2.44.3" {
		t.Fatalf("expected zero-value callOption{} to preserve existing request ID, got %q (found=%v)", gotReqID, found)
	}

	// If a trailing gax.WithGRPCOptions(...) overwrites cs.GRPC after an earlier
	// request-ID option, OptsWithNextRequestID must detect the missing header and
	// append a fresh retryerWithRequestID.
	overwrittenOpts := []gax.CallOption{
		opt1,
		gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(1024)),
	}
	if callOptionsHaveRequestID(overwrittenOpts) {
		t.Fatal("expected callOptionsHaveRequestID to be false when trailing WithGRPCOptions overwrites cs.GRPC")
	}
	reinjected := tr.OptsWithNextRequestID(overwrittenOpts)
	if len(reinjected) != 3 {
		t.Fatalf("expected OptsWithNextRequestID to append retryer after overwrite, got len=%d", len(reinjected))
	}
	resolved := resolveCallSettings(reinjected)
	if _, _, ok := GRPCCallOptionsToRequestID(resolved.GRPC); !ok {
		t.Fatal("expected reinjected options to resolve an x-goog-spanner-request-id header")
	}
}

func TestConcurrentSharedGRPCOptionsNoRace(t *testing.T) {
	// Pre-allocate a slice with spare capacity (cap=8 > len=2) and also an initial
	// request-ID HeaderCallOption to exercise both stripRequestIDGRPCOptions and
	// retryerWithRequestID.Resolve concurrently without mutating the shared backing array.
	seedMD := metadata.MD{Header: []string{"1.seed.1.1.1.1"}}
	sharedGRPC := make([]grpc.CallOption, 0, 8)
	sharedGRPC = append(sharedGRPC, grpc.MaxCallRecvMsgSize(4096), grpc.Header(&seedMD))
	sharedOpt := gax.WithGRPCOptions(sharedGRPC...)

	var nth atomic.Uint32
	tr := NewTracker(1, 2, &nth)

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				// Path 1: streaming override replacing prior request-ID option.
				w := tr.NewWrap()
				var cs1 gax.CallSettings
				sharedOpt.Resolve(&cs1)
				w.WithNextRetryAttempt(1).Resolve(&cs1)
				if _, got, ok := GRPCCallOptionsToRequestID(cs1.GRPC); !ok || strings.Contains(string(got), "seed") {
					t.Errorf("goroutine %d: unexpected stream reqID %q (ok=%v)", id, got, ok)
					return
				}

				// Path 2: unary retryer appending onto shared non-request-ID slice with spare cap.
				plainGRPC := make([]grpc.CallOption, 0, 8)
				plainGRPC = append(plainGRPC, grpc.MaxCallRecvMsgSize(4096))
				plainGax := make([]gax.CallOption, 0, 4)
				plainGax = append(plainGax, gax.WithGRPCOptions(plainGRPC...))
				out := tr.OptsWithNextRequestID(plainGax)
				cs2 := resolveCallSettings(out)
				if _, got, ok := GRPCCallOptionsToRequestID(cs2.GRPC); !ok || got == "" {
					t.Errorf("goroutine %d: missing unary reqID (ok=%v)", id, ok)
					return
				}
			}
		}(g)
	}
	wg.Wait()
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
