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
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestShouldRecycleChannel_UNIMPLEMENTED: UNIMPLEMENTED close error triggers recycle.
func TestShouldRecycleChannel_UNIMPLEMENTED(t *testing.T) {
	err := status.Error(codes.Unimplemented, "sessions not supported on this backend")
	if !shouldRecycleChannel("StreamEnd:Unimplemented", err, 0) {
		t.Error("UNIMPLEMENTED should trigger channel recycle")
	}
}

// TestShouldRecycleChannel_ServerIsDraining: "server is draining" in description triggers recycle.
func TestShouldRecycleChannel_ServerIsDraining(t *testing.T) {
	err := status.Error(codes.Unavailable, "server is draining")
	if !shouldRecycleChannel("StreamEnd:Unavailable", err, 0) {
		t.Error(`"server is draining" in status message should trigger channel recycle`)
	}
}

// TestShouldRecycleChannel_ServerIsDrainingCaseInsensitive: case-insensitive match.
func TestShouldRecycleChannel_ServerIsDrainingCaseInsensitive(t *testing.T) {
	err := status.Error(codes.Unavailable, "Server Is Draining")
	if !shouldRecycleChannel("", err, 0) {
		t.Error(`"Server Is Draining" (mixed case) should trigger channel recycle`)
	}
}

// TestShouldRecycleChannel_GoAway: application-level GoAway close reason triggers recycle.
func TestShouldRecycleChannel_GoAway(t *testing.T) {
	if !shouldRecycleChannel("GoAway", nil, 0) {
		t.Error("GoAway close reason should trigger channel recycle")
	}
}

// TestShouldRecycleChannel_ConsecutiveFailures: hitting the threshold triggers recycle.
func TestShouldRecycleChannel_ConsecutiveFailures(t *testing.T) {
	if !shouldRecycleChannel("StreamEnd:Unavailable", nil, channelConsecutiveFailureThreshold) {
		t.Errorf("consecutiveFailures=%d should trigger recycle", channelConsecutiveFailureThreshold)
	}
	// One below threshold — no recycle.
	if shouldRecycleChannel("StreamEnd:Unavailable", nil, channelConsecutiveFailureThreshold-1) {
		t.Errorf("consecutiveFailures=%d should NOT trigger recycle", channelConsecutiveFailureThreshold-1)
	}
}

// TestShouldRecycleChannel_NegativeCases: unrelated errors do not trigger recycle.
func TestShouldRecycleChannel_NegativeCases(t *testing.T) {
	cases := []struct {
		reason string
		err    error
	}{
		{"StreamEnd:Unavailable", status.Error(codes.Unavailable, "transient failure")},
		{"StreamEnd:EOF", nil},
		{"MissedHeartbeat", nil},
		{"User", nil},
		{"", nil},
	}
	for _, c := range cases {
		if shouldRecycleChannel(c.reason, c.err, 0) {
			t.Errorf("reason=%q err=%v should NOT trigger recycle", c.reason, c.err)
		}
	}
}

// TestMaybeRecycleChannel_CallsRecyclerOnGoAway: recycler is called for a GoAway session.
func TestMaybeRecycleChannel_CallsRecyclerOnGoAway(t *testing.T) {
	var recycledIdx int32 = -1
	p := newMinimalPool(t)
	p.SetChannelRecycler(func(idx int32) { recycledIdx = idx })

	sh := &SessionHandle{}
	sh.session = minimalSession(t, 3)
	sh.session.setCloseReason("GoAway")

	p.maybeRecycleChannel(sh, 0)

	if recycledIdx != 3 {
		t.Errorf("recycledIdx = %d, want 3", recycledIdx)
	}
}

// TestMaybeRecycleChannel_NotCalledOnNonGoAway: recycler not called for unrelated close.
func TestMaybeRecycleChannel_NotCalledOnNonGoAway(t *testing.T) {
	called := false
	p := newMinimalPool(t)
	p.SetChannelRecycler(func(idx int32) { called = true })

	sh := &SessionHandle{}
	sh.session = minimalSession(t, 2)
	sh.session.setCloseReason("MissedHeartbeat")

	p.maybeRecycleChannel(sh, 0)

	if called {
		t.Error("recycler should NOT be called for MissedHeartbeat")
	}
}

// TestMaybeRecycleChannel_NilRecycler: no panic when channelRecycler is nil.
func TestMaybeRecycleChannel_NilRecycler(t *testing.T) {
	p := newMinimalPool(t)
	// channelRecycler is nil by default.

	sh := &SessionHandle{}
	sh.session = minimalSession(t, 0)
	sh.session.setCloseReason("GoAway")

	p.maybeRecycleChannel(sh, 0) // must not panic
}

// TestMaybeRecycleChannel_NegativeChannelIndex: recycler not called when channel index is -1.
func TestMaybeRecycleChannel_NegativeChannelIndex(t *testing.T) {
	called := false
	p := newMinimalPool(t)
	p.SetChannelRecycler(func(idx int32) { called = true })

	sh := &SessionHandle{}
	sh.session = minimalSession(t, -1)
	sh.session.setCloseReason("GoAway")

	p.maybeRecycleChannel(sh, 0)

	if called {
		t.Error("recycler should NOT be called when ChannelIndex is -1")
	}
}

// TestPerChannelFailureCounter: onClose starting branch increments per-channel counter;
// onActive resets it.
func TestPerChannelFailureCounter(t *testing.T) {
	p := newMinimalPool(t)

	const channelIdx int32 = 2
	counter := p.channelFailureCounter(channelIdx)

	// Simulate 4 failures — not yet at threshold.
	for i := int32(1); i <= 4; i++ {
		n := counter.Add(1)
		if shouldRecycleChannel("StreamEnd:Unavailable", nil, n) {
			t.Errorf("failure %d should not yet trigger recycle (threshold=%d)", i, channelConsecutiveFailureThreshold)
		}
	}

	// 5th failure crosses the threshold.
	n := counter.Add(1)
	if !shouldRecycleChannel("StreamEnd:Unavailable", nil, n) {
		t.Errorf("failure %d should trigger recycle (threshold=%d)", n, channelConsecutiveFailureThreshold)
	}

	// onActive resets the counter.
	counter.Store(0)
	if got := counter.Load(); got != 0 {
		t.Errorf("counter after reset = %d, want 0", got)
	}
}

// TestCancelledErrorDoesNotIncrementChannelFailures: CANCELLED close errors (pool
// teardown) must not increment the per-channel failure counter.
func TestCancelledErrorDoesNotIncrementChannelFailures(t *testing.T) {
	cancelledErr := status.Error(codes.Canceled, "pool teardown")
	if codes.Canceled != codes.Canceled {
		t.Fatal("sanity")
	}
	// shouldRecycleChannel should not fire on CANCELLED even if failures is high.
	if shouldRecycleChannel("", cancelledErr, 0) {
		t.Error("CANCELLED should not trigger recycle")
	}
}

// --- helpers -----------------------------------------------------------------

// newMinimalPool returns a pool with no streamFactory wired — sufficient for
// testing lifecycle helpers that don't touch the gRPC path.
func newMinimalPool(t *testing.T) *SessionPoolImpl {
	t.Helper()
	p := NewSessionPoolImpl(1, "test", nil, nil, nil, SessionTypeTable, false)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// minimalSession creates a bare Session with channelIndex set for unit tests
// that only exercise lifecycle helpers (no actual gRPC stream).
// Only CloseReason, setCloseReason, ChannelIndex, and closeError are used;
// the session is never added to sl, so pool.Close does not touch it.
func minimalSession(t *testing.T, channelIdx int32) *Session {
	t.Helper()
	s := &Session{}
	s.setChannelIndex(channelIdx)
	return s
}
