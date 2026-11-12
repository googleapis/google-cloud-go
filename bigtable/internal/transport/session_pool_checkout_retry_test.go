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

package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// injectActiveSessionWithAfe registers a Ready session pinned to afeID.
func injectActiveSessionWithAfe(t testing.TB, p *SessionPoolImpl, name string, afeID AfeID) *SessionHandle {
	t.Helper()
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Now())
	s := NewSession(name, stream, SessionHooks{
		OnStart:   p.onStart,
		OnActive:  func(_ *Session) { p.onActive(sh) },
		OnClosing: func(_ *Session) { p.onClosing(sh) },
		OnClose:   func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.peerInfo.Store(&spb.PeerInfo{ApplicationFrontendId: int64(afeID)})
	s.state.Store(int32(StateReady))
	sh.session = s
	sh.activated.Store(true)
	p.sl.OnSessionStarted(sh)
	return sh
}

// TestCheckoutSession_Concurrent1SessionAfesNeverParksWhenIdleExists verifies
// that N concurrent callers checking out from exactly N 1-session AFEs all
// succeed on the atomic fast path without ever parking in p.waiters. Before
// atomic CheckoutWithPicker, lock-split ReadyAfes → PickAfe → Checkout races
// caused losing callers to fall into p.waiters (and stall behind any queued
// waiter) despite idle sessions remaining on other AFEs.
func TestCheckoutSession_Concurrent1SessionAfesNeverParksWhenIdleExists(t *testing.T) {
	const numCallers = 32
	p := newTestPool(t, numCallers, numCallers*2)
	for i := 1; i <= numCallers; i++ {
		_ = injectActiveSessionWithAfe(t, p, "s", AfeID(i))
	}

	// Park an unclosed sentinel waiter at the head of p.waiters: any caller
	// that spuriously enters the waiter queue will block behind dummy and
	// fail with DeadlineExceeded.
	dummy := &waiter{ready: make(chan struct{})}
	p.waitersMu.Lock()
	dummy.elem = p.waiters.PushBack(dummy)
	p.waitersMu.Unlock()
	t.Cleanup(func() { p.removeWaiter(dummy) })

	for round := 0; round < 25; round++ {
		startGate := make(chan struct{})
		checkedOut := make([]*SessionHandle, numCallers)
		errs := make([]error, numCallers)

		var wg sync.WaitGroup
		wg.Add(numCallers)
		for i := 0; i < numCallers; i++ {
			go func(idx int) {
				defer wg.Done()
				<-startGate
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				checkedOut[idx], errs[idx] = p.CheckoutSession(ctx)
			}(i)
		}
		close(startGate)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d caller %d: CheckoutSession failed (spurious waiter park): %v", round, i, err)
			}
		}
		seen := make(map[*SessionHandle]int, numCallers)
		for i, sh := range checkedOut {
			if sh == nil {
				t.Fatalf("round %d caller %d: CheckoutSession returned nil handle without error", round, i)
			}
			if prev, dup := seen[sh]; dup {
				t.Fatalf("round %d: duplicate *SessionHandle %p returned to callers %d and %d", round, sh, prev, i)
			}
			seen[sh] = i
		}
		for _, sh := range checkedOut {
			sh.DecOutstanding()
			p.sl.ReleaseToPool(sh)
		}
		checkInvariants(t, p.sl)
	}
}
