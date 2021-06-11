// Copyright 2017 Google LLC
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
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// LimitExceededBehavior configures the behavior that flowController can use in case
// the flow control limits are exceeded.
type LimitExceededBehavior int

const (
	// FlowControlIgnore disables flow control.
	FlowControlIgnore LimitExceededBehavior = iota
	// FlowControlBlock signals to wait until the request can be made without exceeding the limit.
	FlowControlBlock
	// FlowControlSignalError signals an error to the caller of acquire.
	FlowControlSignalError
)

var (
	ErrFlowControllerMaxOutstandingMessages = errors.New("pubsub: MaxOutstandingMessages flow controller limit exceeded")
	ErrFlowControllerMaxOutstandingBytes    = errors.New("pubsub: MaxOutstandingBytes flow control limit exceeded")
)

// flowController implements flow control for Subscription.Receive.
type flowController struct {
	maxCount          int
	maxSize           int                 // max total size of messages
	semCount, semSize *semaphore.Weighted // enforces max number and size of messages
	// Number of calls to acquire - number of calls to release. This can go
	// negative if semCount == nil and a large acquire is followed by multiple
	// small releases.
	// Atomic.
	countRemaining int64
	limitBehavior  LimitExceededBehavior
}

// newFlowController creates a new flowController that ensures no more than
// maxCount messages or maxSize bytes are outstanding at once. If maxCount or
// maxSize is < 1, then an unlimited number of messages or bytes is permitted,
// respectively.
func newFlowController(maxCount, maxSize int, behavior LimitExceededBehavior) *flowController {
	fc := &flowController{
		maxCount:      maxCount,
		maxSize:       maxSize,
		semCount:      nil,
		semSize:       nil,
		limitBehavior: behavior,
	}
	if maxCount > 0 {
		fc.semCount = semaphore.NewWeighted(int64(maxCount))
	}
	if maxSize > 0 {
		fc.semSize = semaphore.NewWeighted(int64(maxSize))
	}
	return fc
}

// acquire blocks until one message of size bytes can proceed or ctx is done.
// It returns nil in the first case, or ctx.Err() in the second.
//
// acquire allows large messages to proceed by treating a size greater than maxSize
// as if it were equal to maxSize.
func (f *flowController) acquire(ctx context.Context, size int) error {
	if f.limitBehavior == FlowControlIgnore {
		return nil
	}
	if f.semCount != nil {
		if err := f.semCount.Acquire(ctx, 1); err != nil {
			return err
		}
	}
	if f.semSize != nil {
		if err := f.semSize.Acquire(ctx, f.bound(size)); err != nil {
			if f.semCount != nil {
				f.semCount.Release(1)
			}
			return err
		}
	}
	atomic.AddInt64(&f.countRemaining, 1)
	return nil
}

// tryAcquire returns false if acquire would block. Otherwise, it behaves like
// acquire and returns true.
//
// tryAcquire allows large messages to proceed by treating a size greater than
// maxSize as if it were equal to maxSize.
// func (f *flowController) tryAcquire(size int) bool {
// 	if f.semCount != nil {
// 		if !f.semCount.TryAcquire(1) {
// 			return false
// 		}
// 	}
// 	if f.semSize != nil {
// 		if !f.semSize.TryAcquire(f.bound(size)) {
// 			if f.semCount != nil {
// 				f.semCount.Release(1)
// 			}
// 			return false
// 		}
// 	}
// 	atomic.AddInt64(&f.countRemaining, 1)
// 	return true
// }

// newAcquire acquires space for a message: the message count and its size.
//
// In FlowControlSignalError mode, large messages greater than maxSize
// will be result in an error. In other modes, large messages will be treated
// as if it were equal to maxSize.
func (f *flowController) newAcquire(ctx context.Context, size int) error {
	switch f.limitBehavior {
	case FlowControlIgnore:
		return nil
	case FlowControlBlock:
		if f.semCount != nil {
			if err := f.semCount.Acquire(ctx, 1); err != nil {
				return err
			}
		}
		if f.semSize != nil {
			if err := f.semSize.Acquire(ctx, f.bound(size)); err != nil {
				if f.semCount != nil {
					f.semCount.Release(1)
				}
				return err
			}
		}
		atomic.AddInt64(&f.countRemaining, 1)
	case FlowControlSignalError:
		if f.semCount != nil {
			if !f.semCount.TryAcquire(1) {
				return ErrFlowControllerMaxOutstandingMessages
			}
		}
		if f.semSize != nil {
			// Try to acquire the full size of the message here.
			if !f.semSize.TryAcquire(int64(size)) {
				if f.semCount != nil {
					f.semCount.Release(1)
				}
				return ErrFlowControllerMaxOutstandingBytes
			}
		}
		atomic.AddInt64(&f.countRemaining, 1)
	}
	return nil
}

// release notes that one message of size bytes is no longer outstanding.
func (f *flowController) release(size int) {
	if f.limitBehavior == FlowControlIgnore {
		return
	}
	atomic.AddInt64(&f.countRemaining, -1)
	if f.semCount != nil {
		f.semCount.Release(1)
	}
	if f.semSize != nil {
		f.semSize.Release(f.bound(size))
	}
}

func (f *flowController) bound(size int) int64 {
	if size > f.maxSize {
		return int64(f.maxSize)
	}
	return int64(size)
}

func (f *flowController) count() int {
	return int(atomic.LoadInt64(&f.countRemaining))
}
