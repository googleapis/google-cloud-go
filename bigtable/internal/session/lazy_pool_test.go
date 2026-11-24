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
	"sync"
	"testing"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// stubInvoker is a minimal Invoker for tests that only need identity —
// distinct from the shared fakeInvoker to avoid coupling this test file
// to table_test.go's fixture.
type stubInvoker struct{ tag string }

func (s *stubInvoker) Invoke(_ context.Context, _ btransport.VRpcDescriptor, _ interface{}) (btransport.InvokeResult, error) {
	return btransport.InvokeResult{}, nil
}

// TestLazyPool_NilPoolAndNilOpenReturnNilNil verifies the "no session
// support" contract: a nil *lazyPool or one with a nil open closure
// returns (nil, nil). Used for the write side of MatView (spec #11).
func TestLazyPool_NilPoolAndNilOpenReturnNilNil(t *testing.T) {
	var nilPool *lazyPool
	if p, err := nilPool.get(); p != nil || err != nil {
		t.Errorf("nil-receiver get() = (%v, %v), want (nil, nil)", p, err)
	}

	empty := &lazyPool{}
	if p, err := empty.get(); p != nil || err != nil {
		t.Errorf("nil-open get() = (%v, %v), want (nil, nil)", p, err)
	}
	if empty.opened() {
		t.Error("empty.opened() = true, want false")
	}
}

// TestLazyPool_FailedOpenNotCached verifies SESSION_SPEC.md #11: failed
// opens MUST NOT be cached. A transient proto.Marshal failure (or any
// other opener error) MUST NOT strand the table for the process
// lifetime. The next get() call re-invokes open().
//
// The counterfactual: if we DID cache the failure, calls == 1 after N
// gets. The invariant is that calls == N.
func TestLazyPool_FailedOpenNotCached(t *testing.T) {
	var (
		mu       sync.Mutex
		calls    int
		failNext = true
		succeed  = &stubInvoker{tag: "opened"}
		wantErr  = errors.New("marshal boom")
	)
	l := &lazyPool{
		open: func() (Invoker, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if failNext {
				return nil, wantErr
			}
			return succeed, nil
		},
	}

	// Four failing opens — each MUST re-invoke the closure.
	for i := 0; i < 4; i++ {
		p, err := l.get()
		if p != nil {
			t.Errorf("call #%d: pool = %v, want nil on open failure", i+1, p)
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("call #%d: err = %v, want %v", i+1, err, wantErr)
		}
		if l.opened() {
			t.Errorf("call #%d: opened() = true after failure; failure MUST NOT be cached", i+1)
		}
	}
	mu.Lock()
	if calls != 4 {
		t.Errorf("open() invocations after 4 failing gets = %d, want 4 — failed opens were cached, violating spec #11", calls)
	}
	// Now flip: next open succeeds. Subsequent gets MUST return the
	// cached success (open() invoked exactly one more time, not four).
	failNext = false
	mu.Unlock()

	for i := 0; i < 4; i++ {
		p, err := l.get()
		if err != nil {
			t.Errorf("post-success call #%d: err = %v, want nil", i+1, err)
		}
		if p != succeed {
			t.Errorf("post-success call #%d: pool = %v, want cached stubInvoker", i+1, p)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 5 {
		t.Errorf("total open() invocations = %d, want 5 (4 failing + 1 successful cached) — successful opens MUST be cached", calls)
	}
	if !l.opened() {
		t.Error("opened() = false after successful open, want true")
	}
}
