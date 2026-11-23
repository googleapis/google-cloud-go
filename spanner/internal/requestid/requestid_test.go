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
	"fmt"
	"io"
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
	// Calling OptsWithNextRequestID or LogicalRequestKeyFromCallOptions on a slice
	// that contains an unresolved *retryerWithRequestID must be idempotent and must
	// not advance nthRequest before the actual gax.Invoke resolve.
	nthBeforeInspect := nth.Load()
	if got := tr.OptsWithNextRequestID(reinjected); len(got) != 3 {
		t.Fatalf("expected second OptsWithNextRequestID call to be idempotent, got len=%d", len(got))
	}
	unaryOpts := tr.OptsWithNextRequestID(nil)
	if got := LogicalRequestKeyFromCallOptions(unaryOpts); got != "" {
		t.Fatalf("expected LogicalRequestKeyFromCallOptions on unresolved retryer to return empty string, got %q", got)
	}
	if got := nth.Load(); got != nthBeforeInspect {
		t.Fatalf("inspecting *retryerWithRequestID mutated nthRequest: got %d, want %d", got, nthBeforeInspect)
	}
	var resolved gax.CallSettings
	for _, o := range reinjected {
		o.Resolve(&resolved)
	}
	if _, _, ok := GRPCCallOptionsToRequestID(resolved.GRPC); !ok {
		t.Fatal("expected reinjected options to resolve an x-goog-spanner-request-id header")
	}
}

// rawGRPCOpt assigns a pre-allocated []grpc.CallOption slice directly onto cs.GRPC
// without copying, exercising spare-capacity clipping on shared backing arrays.
type rawGRPCOpt []grpc.CallOption

func (o rawGRPCOpt) Resolve(cs *gax.CallSettings) {
	cs.GRPC = o
}

func TestConcurrentSharedGRPCOptionsNoRace(t *testing.T) {
	// 1. Realistic shared gax.WithGRPCOptions without a request-ID header (len=1, cap=1):
	//    catches in-place opts[:0] mutation on the !hasReqID branch.
	sharedNoReqID := gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(4096))

	// 2. Shared non-copying slice with spare capacity (len=1, cap=8) and no request-ID header:
	//    catches dropping opts[:len:len] in stripRequestIDGRPCOptions and dropping
	//    cs.GRPC[:len:len] in retryerWithRequestID.Resolve.
	sharedRaw := make(rawGRPCOpt, 1, 8)
	sharedRaw[0] = grpc.MaxCallRecvMsgSize(4096)

	// 3. Shared gax.WithGRPCOptions carrying a seeded request-ID header:
	//    catches in-place opts[:0] mutation on the hasReqID branch.
	seedMD := metadata.MD{Header: []string{"1.seed.1.1.1.1"}}
	sharedSeeded := gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(4096), grpc.Header(&seedMD))

	// 4. Shared gax.CallOption slice with spare capacity (len=1, cap=4):
	//    catches dropping priors[:len:len] in OptsWithNextRequestID.
	sharedPriors := make([]gax.CallOption, 1, 4)
	sharedPriors[0] = sharedRaw

	var nth atomic.Uint32
	tr := NewTracker(1, 2, &nth)
	proc := RandIDForProcess()

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				// Case A: !hasReqID via realistic gax.WithGRPCOptions.
				w1 := tr.NewWrap()
				var csA gax.CallSettings
				sharedNoReqID.Resolve(&csA)
				w1.WithNextRetryAttempt(1).Resolve(&csA)
				wantA := fmt.Sprintf("1.%s.1.2.%d.1", proc, w1.nthRequest)
				if _, got, ok := GRPCCallOptionsToRequestID(csA.GRPC); !ok || string(got) != wantA {
					t.Errorf("goroutine %d case A: got %q (ok=%v), want %q", id, got, ok, wantA)
					return
				}

				// Case B: !hasReqID with spare capacity on cs.GRPC.
				w2 := tr.NewWrap()
				var csB gax.CallSettings
				sharedRaw.Resolve(&csB)
				w2.WithNextRetryAttempt(1).Resolve(&csB)
				wantB := fmt.Sprintf("1.%s.1.2.%d.1", proc, w2.nthRequest)
				if _, got, ok := GRPCCallOptionsToRequestID(csB.GRPC); !ok || string(got) != wantB {
					t.Errorf("goroutine %d case B: got %q (ok=%v), want %q", id, got, ok, wantB)
					return
				}

				// Case C: hasReqID override replacing seeded request-ID option.
				w3 := tr.NewWrap()
				var csC gax.CallSettings
				sharedSeeded.Resolve(&csC)
				w3.WithNextRetryAttempt(1).Resolve(&csC)
				wantC := fmt.Sprintf("1.%s.1.2.%d.1", proc, w3.nthRequest)
				if _, got, ok := GRPCCallOptionsToRequestID(csC.GRPC); !ok || string(got) != wantC {
					t.Errorf("goroutine %d case C: got %q (ok=%v), want %q", id, got, ok, wantC)
					return
				}

				// Case D: unary OptsWithNextRequestID + retryerWithRequestID.Resolve
				// on shared priors (cap=4) and shared cs.GRPC (cap=8).
				var csD gax.CallSettings
				for _, o := range tr.OptsWithNextRequestID(sharedPriors) {
					o.Resolve(&csD)
				}
				if _, got, ok := GRPCCallOptionsToRequestID(csD.GRPC); !ok || got == "" {
					t.Errorf("goroutine %d case D: missing unary reqID (ok=%v)", id, ok)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

type fakeClientStream struct {
	grpc.ClientStream
	recvErr error
}

func (f *fakeClientStream) RecvMsg(any) error { return f.recvErr }
func (f *fakeClientStream) SendMsg(any) error { return f.recvErr }

func TestHeaderInjectorAugmentsErrorAndEnforcesContract(t *testing.T) {
	var nth atomic.Uint32
	tr := NewTracker(1, 1, &nth)
	var cs gax.CallSettings
	for _, o := range tr.OptsWithNextRequestID(nil) {
		o.Resolve(&cs)
	}

	t.Run("attaches request ID on non-EOF errors and skips io.EOF", func(t *testing.T) {
		inj := NewHeaderInjector(func(err error, reqID string) error {
			return errors.New(err.Error() + " [reqID=" + reqID + "]")
		})

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

		// io.EOF must pass through unwrapped on both unary and stream RecvMsg.
		eofUnary := inj.InterceptUnary(
			context.Background(),
			"/google.spanner.v1.Spanner/ExecuteSql",
			nil,
			nil,
			nil,
			func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return io.EOF },
			cs.GRPC...,
		)
		if eofUnary != io.EOF {
			t.Fatalf("expected io.EOF untouched from InterceptUnary, got %v", eofUnary)
		}

		st, err := inj.InterceptStream(
			context.Background(),
			&grpc.StreamDesc{ServerStreams: true},
			nil,
			"/google.spanner.v1.Spanner/ExecuteStreamingSql",
			func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
				return &fakeClientStream{recvErr: io.EOF}, nil
			},
			cs.GRPC...,
		)
		if err != nil {
			t.Fatalf("InterceptStream failed: %v", err)
		}
		if got := st.RecvMsg(nil); got != io.EOF {
			t.Fatalf("expected io.EOF untouched from stream.RecvMsg, got %v", got)
		}
	})

	t.Run("nil-returning augmenter preserves original non-nil error", func(t *testing.T) {
		origErr := errors.New("rpc failed")
		inj := NewHeaderInjector(func(error, string) error { return nil })
		got := inj.InterceptUnary(
			context.Background(),
			"/google.spanner.v1.Spanner/ExecuteSql",
			nil,
			nil,
			nil,
			func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return origErr },
			cs.GRPC...,
		)
		if got != origErr {
			t.Fatalf("expected original error %v when augmenter returns nil, got %v", origErr, got)
		}
	})

	t.Run("nil augmenter and nil HeaderInjector still inject outgoing header", func(t *testing.T) {
		origErr := errors.New("unchanged")
		rawStream := &fakeClientStream{recvErr: origErr}

		for _, inj := range []*HeaderInjector{NewHeaderInjector(nil), nil} {
			var wireReqID string
			got := inj.InterceptUnary(
				context.Background(),
				"/google.spanner.v1.Spanner/ExecuteSql",
				nil,
				nil,
				nil,
				func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
					if md, ok := metadata.FromOutgoingContext(ctx); ok {
						if vals := md.Get(Header); len(vals) > 0 {
							wireReqID = vals[0]
						}
					}
					return origErr
				},
				cs.GRPC...,
			)
			if got != origErr || wireReqID == "" {
				t.Fatalf("inj=%v: got err=%v wireReqID=%q, want origErr and non-empty header", inj, got, wireReqID)
			}

			var streamWireID string
			st, sErr := inj.InterceptStream(
				context.Background(),
				&grpc.StreamDesc{ServerStreams: true},
				nil,
				"/google.spanner.v1.Spanner/ExecuteStreamingSql",
				func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
					if md, ok := metadata.FromOutgoingContext(ctx); ok {
						if vals := md.Get(Header); len(vals) > 0 {
							streamWireID = vals[0]
						}
					}
					return rawStream, nil
				},
				cs.GRPC...,
			)
			if sErr != nil || st != rawStream || streamWireID == "" {
				t.Fatalf("inj=%v: expected unwrapped rawStream with injected header, got st=%T err=%v header=%q", inj, st, sErr, streamWireID)
			}
		}
	})
}
