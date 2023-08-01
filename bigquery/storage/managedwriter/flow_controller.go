// Copyright 2021 Google LLC
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
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// Flow controller for write API.  Adapted from pubsub.
type flowController struct {
	// The max number of pending write requests.
	maxInsertCount int
	// The max pending request bytes.
	maxInsertBytes int

	// Semaphores for governing pending inserts.
	semInsertCount, semInsertBytes *semaphore.Weighted

	countTracked int64 // Atomic.
	bytesTracked int64 // Atomic.  Only tracked if bytes are bounded.
}

func newFlowController(maxInserts, maxInsertBytes int) *flowController {
	fc := &flowController{
		maxInsertCount: maxInserts,
		maxInsertBytes: maxInsertBytes,
		semInsertCount: nil,
		semInsertBytes: nil,
	}
	if maxInserts > 0 {
		fc.semInsertCount = semaphore.NewWeighted(int64(maxInserts))
	}
	if maxInsertBytes > 0 {
		fc.semInsertBytes = semaphore.NewWeighted(int64(maxInsertBytes))
	}
	return fc
}

// copyFlowController is for creating a new flow controller based on
// settings from another.  It does not copy flow state.
func copyFlowController(in *flowController) *flowController {
	var maxInserts, maxBytes int
	if in != nil {
		maxInserts = in.maxInsertCount
		maxBytes = in.maxInsertBytes
	}
	return newFlowController(maxInserts, maxBytes)
}

// acquire blocks until one insert of size bytes can proceed or ctx is done.
// It returns nil in the first case, or ctx.Err() in the second.
//
// acquire allows large messages to proceed by treating a size greater than maxSize
// as if it were equal to maxSize.
func (fc *flowController) acquire(ctx context.Context, sizeBytes int) error {
	if fc.semInsertCount != nil {
		if err := fc.semInsertCount.Acquire(ctx, 1); err != nil {
			return err
		}
	}
	if fc.semInsertBytes != nil {
		if err := fc.semInsertBytes.Acquire(ctx, fc.bound(sizeBytes)); err != nil {
			if fc.semInsertCount != nil {
				fc.semInsertCount.Release(1)
			}
			return err
		}
	}
	atomic.AddInt64(&fc.bytesTracked, fc.bound(sizeBytes))
	atomic.AddInt64(&fc.countTracked, 1)
	return nil
}

// tryAcquire returns false if acquire would block. Otherwise, it behaves like
// acquire and returns true.
//
// tryAcquire allows large inserts to proceed by treating a size greater than
// maxSize as if it were equal to maxSize.
func (fc *flowController) tryAcquire(sizeBytes int) bool {
	if fc.semInsertCount != nil {
		if !fc.semInsertCount.TryAcquire(1) {
			return false
		}
	}
	if fc.semInsertBytes != nil {
		if !fc.semInsertBytes.TryAcquire(fc.bound(sizeBytes)) {
			if fc.semInsertCount != nil {
				fc.semInsertCount.Release(1)
			}
			return false
		}
	}
	atomic.AddInt64(&fc.bytesTracked, fc.bound(sizeBytes))
	atomic.AddInt64(&fc.countTracked, 1)
	return true
}

func (fc *flowController) release(sizeBytes int) {
	atomic.AddInt64(&fc.countTracked, -1)
	atomic.AddInt64(&fc.bytesTracked, (0 - fc.bound(sizeBytes)))
	if fc.semInsertCount != nil {
		fc.semInsertCount.Release(1)
	}
	if fc.semInsertBytes != nil {
		fc.semInsertBytes.Release(fc.bound(sizeBytes))
	}
}

// bound normalizes input size to maxInsertBytes if it exceeds the limit.
func (fc *flowController) bound(sizeBytes int) int64 {
	if sizeBytes > fc.maxInsertBytes {
		return int64(fc.maxInsertBytes)
	}
	return int64(sizeBytes)
}

func (fc *flowController) count() int {
	return int(atomic.LoadInt64(&fc.countTracked))
}

func (fc *flowController) bytes() int {
	return int(atomic.LoadInt64(&fc.bytesTracked))
}
