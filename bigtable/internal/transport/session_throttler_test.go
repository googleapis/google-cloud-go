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
	"sync/atomic"
	"testing"
	"time"
)

// newThrottlerWithClock returns an AdaptiveSessionThrottler with an
// injectable clock so penalty-expiry tests don't sleep.
func newThrottlerWithClock(max int, penalty time.Duration, now func() time.Time) *AdaptiveSessionThrottler {
	t := NewAdaptiveSessionThrottler(max, penalty).(*AdaptiveSessionThrottler)
	t.nowFn = now
	return t
}

func TestThrottler_AcquireReleaseSuccess(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(2, time.Minute)
	ctx := context.Background()

	if err := tr.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if err := tr.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	if got := tr.Snapshot(); got.InUse != 2 || got.Capacity != 2 {
		t.Fatalf("Snapshot after two Acquires = %+v, want InUse=2 Capacity=2", got)
	}

	tr.Release(true)
	if got := tr.Snapshot(); got.InUse != 1 {
		t.Fatalf("InUse after success Release = %d, want 1", got.InUse)
	}

	// Third Acquire should succeed after the release freed a slot.
	if err := tr.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 3 after release: %v", err)
	}
}

func TestThrottler_AcquireBlocksAtCap(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(1, time.Minute)
	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire seed: %v", err)
	}

	// Second Acquire should block until Release fires.
	acquired := make(chan error, 1)
	go func() { acquired <- tr.Acquire(context.Background()) }()

	select {
	case err := <-acquired:
		t.Fatalf("Acquire returned before slot freed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	tr.Release(true)
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("blocked Acquire returned err after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Release did not unblock waiter")
	}
}

func TestThrottler_AcquireRespectsCtx(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(1, time.Minute)
	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := tr.Acquire(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Acquire ctx=%v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Acquire waited too long past ctx deadline: %v", elapsed)
	}
}

func TestThrottler_FailurePenaltyReservesSlot(t *testing.T) {
	// Fixed clock — advance it manually to test penalty expiry.
	var nowNs atomic.Int64
	nowNs.Store(time.Now().UnixNano())
	clock := func() time.Time { return time.Unix(0, nowNs.Load()) }

	tr := newThrottlerWithClock(1, 100*time.Millisecond, clock)

	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	tr.Release(false) // failure → slot held for 100ms

	if got := tr.Snapshot(); got.InUse != 1 {
		t.Fatalf("InUse after failure Release = %d, want 1 (penalty holds slot)", got.InUse)
	}

	// Acquire should block while the penalty is live.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := tr.Acquire(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Acquire during penalty: got %v, want DeadlineExceeded", err)
	}

	// Advance clock past the penalty; next Acquire should succeed.
	nowNs.Add(int64(200 * time.Millisecond))
	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire after penalty expiry: %v", err)
	}
	if got := tr.Snapshot(); got.InUse != 1 {
		t.Fatalf("InUse after penalty drained + reacquire = %d, want 1", got.InUse)
	}
}

func TestThrottler_UpdateConfigGrowsUnblocks(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(1, time.Minute)
	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire seed: %v", err)
	}

	acquired := make(chan error, 1)
	go func() { acquired <- tr.Acquire(context.Background()) }()

	// Confirm it's blocked before the config bump.
	select {
	case <-acquired:
		t.Fatal("second Acquire returned before UpdateConfig")
	case <-time.After(30 * time.Millisecond):
	}

	tr.UpdateConfig(5, time.Minute)

	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("Acquire after grow: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("UpdateConfig did not unblock waiter")
	}
	if got := tr.Snapshot(); got.Capacity != 5 {
		t.Fatalf("Capacity after grow = %d, want 5", got.Capacity)
	}
}

func TestThrottler_UpdateConfigShrinkHonoredOnNextAcquire(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(3, time.Minute)
	for i := 0; i < 2; i++ {
		if err := tr.Acquire(context.Background()); err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
	}
	// Shrink below current in-use. Existing holders keep their slots
	// (the throttler tolerates over-cap holders after a shrink), but
	// no new Acquire succeeds until Releases bring the counter under
	// the new cap.
	tr.UpdateConfig(1, time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := tr.Acquire(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Acquire after shrink: got %v, want DeadlineExceeded", err)
	}

	tr.Release(true)
	tr.Release(true) // drops to 0; still below new cap of 1

	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire after drain: %v", err)
	}
}

func TestThrottler_ConcurrentAcquireRelease(t *testing.T) {
	// Cheap smoke under -race: many goroutines Acquire/Release; the
	// counter must not drift.
	const cap = 8
	const workers = 32
	const perWorker = 200
	tr := NewAdaptiveSessionThrottler(cap, 0)

	var wg sync.WaitGroup
	var successes atomic.Int64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := tr.Acquire(context.Background()); err != nil {
					t.Errorf("Acquire: %v", err)
					return
				}
				successes.Add(1)
				tr.Release(true)
			}
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != workers*perWorker {
		t.Fatalf("successful acquires = %d, want %d", got, workers*perWorker)
	}
	if got := tr.Snapshot(); got.InUse != 0 {
		t.Fatalf("InUse after all releases = %d, want 0", got.InUse)
	}
}

// TestThrottler_BlockedWaiterWokenByPenaltyExpiry pins down the
// regression fixed by scheduling a time.AfterFunc broadcast on
// failed Release: a waiter parked in cond.Wait must wake when the
// penalty expires, without needing an unrelated Release/UpdateConfig
// event to fire the broadcast. Uses real time so the AfterFunc timer
// runs against the same clock as the deadline.
func TestThrottler_BlockedWaiterWokenByPenaltyExpiry(t *testing.T) {
	tr := NewAdaptiveSessionThrottler(1, 50*time.Millisecond)
	if err := tr.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	tr.Release(false) // failure → slot held for 50ms

	acquired := make(chan error, 1)
	go func() { acquired <- tr.Acquire(context.Background()) }()

	// Waiter must still be parked well before penalty expiry.
	select {
	case err := <-acquired:
		t.Fatalf("Acquire returned before penalty expired: err=%v", err)
	case <-time.After(20 * time.Millisecond):
	}

	// After penalty expiry the scheduled broadcast must wake the waiter.
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("Acquire after penalty expiry: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Acquire hung past penalty expiry — timer wake did not fire")
	}
}
