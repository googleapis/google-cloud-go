// Copyright 2025 Google LLC
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

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func fcSettings(c int, s int, l LimitExceededBehavior) FlowControlSettings {
	return FlowControlSettings{
		MaxOutstandingMessages: c,
		MaxOutstandingBytes:    s,
		LimitExceededBehavior:  l,
	}
}

func TestFlowControllerCancel(t *testing.T) {
	// Test canceling a flow controller's context.
	t.Parallel()
	fc := newFlowController(fcSettings(3, 10, FlowControlBlock))
	if err := fc.acquire(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	// Experiment: a context that times out should always return an error.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := fc.acquire(ctx, 6); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, expected DeadlineExceeded", err)
	}
	// Control: a context that is not done should always return nil.
	go func() {
		time.Sleep(5 * time.Millisecond)
		fc.release(ctx, 5)
	}()
	if err := fc.acquire(context.Background(), 6); err != nil {
		t.Errorf("got %v, expected nil", err)
	}
}

func TestFlowControllerLargeRequest(t *testing.T) {
	// Large requests succeed, consuming the entire allotment.
	t.Parallel()
	fc := newFlowController(fcSettings(3, 10, FlowControlBlock))
	err := fc.acquire(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlowControllerNoStarve(t *testing.T) {
	// A large request won't starve, because the flowController is
	// (best-effort) FIFO.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fc := newFlowController(fcSettings(10, 10, FlowControlBlock))
	first := make(chan int)
	for i := 0; i < 20; i++ {
		go func() {
			for {
				if err := fc.acquire(ctx, 1); err != nil {
					if !errors.Is(err, context.Canceled) {
						t.Error(err)
					}
					return
				}
				select {
				case first <- 1:
				default:
				}
				fc.release(ctx, 1)
			}
		}()
	}
	<-first // Wait until the flowController's state is non-zero.
	if err := fc.acquire(ctx, 11); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestFlowControllerSaturation(t *testing.T) {
	t.Parallel()
	const (
		maxCount = 6
		maxSize  = 10
	)
	for _, test := range []struct {
		acquireSize         int
		wantCount, wantSize int64
	}{
		{
			// Many small acquires cause the flow controller to reach its max count.
			acquireSize: 1,
			wantCount:   6,
			wantSize:    6,
		},
		{
			// Five acquires of size 2 will cause the flow controller to reach its max size,
			// but not its max count.
			acquireSize: 2,
			wantCount:   5,
			wantSize:    10,
		},
		{
			// If the requests are the right size (relatively prime to maxSize),
			// the flow controller will not saturate on size. (In this case, not on count either.)
			acquireSize: 3,
			wantCount:   3,
			wantSize:    9,
		},
	} {
		fc := newFlowController(fcSettings(maxCount, maxSize, FlowControlBlock))
		// Atomically track flow controller state.
		// The flowController itself tracks count.
		var curSize int64
		success := errors.New("")
		// Time out if wantSize or wantCount is never reached.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		g, ctx := errgroup.WithContext(ctx)
		for i := 0; i < 10; i++ {
			g.Go(func() error {
				var hitCount, hitSize bool
				// Run at least until we hit the expected values, and at least
				// for enough iterations to exceed them if the flow controller
				// is broken.
				for i := 0; i < 100 || !hitCount || !hitSize; i++ {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					if err := fc.acquire(ctx, test.acquireSize); err != nil {
						return err
					}
					c := int64(fc.count())
					if c > test.wantCount {
						return fmt.Errorf("count %d exceeds want %d", c, test.wantCount)
					}
					if c == test.wantCount {
						hitCount = true
					}
					s := atomic.AddInt64(&curSize, int64(test.acquireSize))
					if s > test.wantSize {
						return fmt.Errorf("size %d exceeds want %d", s, test.wantSize)
					}
					if s == test.wantSize {
						hitSize = true
					}
					time.Sleep(5 * time.Millisecond) // Let other goroutines make progress.
					if atomic.AddInt64(&curSize, -int64(test.acquireSize)) < 0 {
						return errors.New("negative size")
					}
					fc.release(ctx, test.acquireSize)
				}
				return success
			})
		}
		if err := g.Wait(); err != success {
			t.Errorf("%+v: %v", test, err)
			continue
		}
	}
}

func TestFlowControllerUnboundedCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newFlowController(fcSettings(0, 10, FlowControlSignalError))

	// Successfully acquire 4 bytes.
	if err := fc.acquire(ctx, 4); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}

	// Successfully acquire 4 bytes.
	if err := fc.acquire(ctx, 4); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}

	// Fail to acquire 3 bytes.
	if err := fc.acquire(ctx, 3); err == nil {
		t.Errorf("got nil, wanted %v", ErrFlowControllerMaxOutstandingBytes)
	}
}

func TestFlowControllerUnboundedCount2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newFlowController(fcSettings(0, 0, FlowControlSignalError))
	// Successfully acquire 4 bytes.
	if err := fc.acquire(ctx, 4); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}
	fc.release(ctx, 1)
	fc.release(ctx, 1)
	fc.release(ctx, 1)
	wantCount := int64(0)
	c := int64(fc.count())
	if c != wantCount {
		t.Fatalf("got count %d, want %d", c, wantCount)
	}
}

func TestFlowControllerUnboundedBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fc := newFlowController(fcSettings(2, 0, FlowControlSignalError))

	// Successfully acquire 4GB.
	if err := fc.acquire(ctx, 4e9); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}

	// Successfully acquired 4GB bytes.
	if err := fc.acquire(ctx, 4e9); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}

	// Fail to acquire a third message.
	if err := fc.acquire(ctx, 3); err == nil {
		t.Errorf("got nil, wanted %v", ErrFlowControllerMaxOutstandingMessages)
	}
}

func TestFlowControllerIgnoreWithMetrics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Initialize fc with bytes > acquire to ensure no bound restrictions occur.
	fc := newFlowController(fcSettings(5, 50, FlowControlIgnore))

	if err := fc.acquire(ctx, 5); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}
	if err := fc.acquire(ctx, 5); err != nil {
		t.Errorf("got %v, wanted no error", err)
	}

	if c := fc.count(); c != 2 {
		t.Errorf("got %d messages, want 2", c)
	}

	if s := fc.size(); s != 10 {
		t.Errorf("got %d bytes, want 10", s)
	}

	fc.release(ctx, 5)

	if c := fc.count(); c != 1 {
		t.Errorf("got %d messages, want 1", c)
	}
	if s := fc.size(); s != 5 {
		t.Errorf("got %d bytes, want 5", s)
	}
}
