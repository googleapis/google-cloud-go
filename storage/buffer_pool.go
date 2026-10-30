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

package storage

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage/experimental"
)

const (
	// defaultBufferPoolBlockSize is the size of every block vended by the
	// default buffer pool. The minimum GCS parallel upload part size is
	// 32 MiB and valid part sizes are multiples of 32 MiB, so a 32 MiB block
	// backs each minimum-sized part with a single contiguous buffer while
	// still dividing larger part sizes (64 MiB, 128 MiB).
	defaultBufferPoolBlockSize = 32 << 20

	// defaultBufferPoolMaxRefillBlocks caps how many cached blocks a single
	// background pre-allocation aims for (128 MiB with 32 MiB blocks). It
	// keeps a very large maxSize from pre-allocating unbounded memory; it
	// does not limit Get, which always succeeds.
	defaultBufferPoolMaxRefillBlocks = 4

	// defaultBufferPoolCleanInterval is how often the background cleaner
	// checks whether the pool has been idle.
	defaultBufferPoolCleanInterval = 15 * time.Second

	// defaultBufferPoolIdleTimeout is how long the pool must be idle before
	// half of the cached blocks are evicted. Each subsequent idle period of
	// the same length evicts half of what remains.
	defaultBufferPoolIdleTimeout = 30 * time.Second
)

var errInvalidBufferSize = errors.New("storage: buffer pool: maxSize must be positive")

// Compile-time check that defaultBufferPool implements experimental.BufferPool.
var _ experimental.BufferPool = (*defaultBufferPool)(nil)

// defaultBufferPool is the BufferPool used for parallel uploads when the
// application does not provide one through experimental.WithBufferPool.
//
// It manages uniform 32 MiB blocks that are shared across all writers of a
// client. The pool has no memory limit; the memory in use is bounded by the
// writers' PartSize and Concurrency settings. It provides:
//
//   - Guaranteed progress: Get always returns a block, allocating one inline
//     if none is cached, so the first segment of every upload can proceed.
//   - Cache-only admission: TryGet only vends blocks that are already cached
//     and never allocates inline. When the cache is empty it returns false,
//     and the uploader continues with the segments it already holds.
//   - Async pre-allocation: when the number of cached blocks falls below what
//     a request needs (capped at defaultBufferPoolMaxRefillBlocks),
//     replacement blocks are allocated in the background,
//     overlapping allocation with network I/O. This is how the cache, and
//     therefore TryGet concurrency, ramps up on a cold pool.
//   - Staged idle eviction: after idleTimeout without activity, half of the
//     cached blocks are released to the garbage collector; this repeats
//     every idleTimeout until the pool is empty, at which point the cleaner
//     goroutine exits until blocks are cached again.
//
// All methods are safe for concurrent use.
type defaultBufferPool struct {
	blockSize     int
	cleanInterval time.Duration
	idleTimeout   time.Duration
	now           func() time.Time

	// freeMu guards free.
	freeMu sync.Mutex
	// free is a LIFO stack of cached blocks ready to be vended. LIFO hands
	// out the most recently used (and most likely CPU-cache-warm) block.
	free [][]byte

	// live is the number of blocks owned by the pool, cached or checked out;
	// tests use it to detect leaks.
	live atomic.Int64

	// start is when the pool was created. It carries the monotonic clock
	// reading that lastActive is measured against.
	start time.Time

	// lastActive is when the last Get, TryGet, Put or eviction happened, in
	// nanoseconds since start. Storing an offset from start keeps idle
	// detection on the monotonic clock, so wall-clock changes (for example
	// NTP steps) do not make the pool evict early or late.
	lastActive atomic.Int64

	// refilling is set while a background pre-allocation goroutine is running.
	refilling atomic.Bool

	// closed is set once close has been called.
	closed atomic.Bool

	// cleanerRunning reports whether the cleaner goroutine is active. It is
	// read without mu on the Put path and only modified while holding mu.
	cleanerRunning atomic.Bool

	// mu serializes starting and stopping background goroutines with close.
	mu sync.Mutex
	// stop is closed to terminate the cleaner goroutine.
	stop chan struct{}
	// wg tracks background goroutines; used by close.
	wg sync.WaitGroup
}

// newDefaultBufferPool returns a defaultBufferPool with the SDK defaults.
func newDefaultBufferPool() *defaultBufferPool {
	return newDefaultBufferPoolWithConfig(defaultBufferPoolBlockSize,
		defaultBufferPoolCleanInterval, defaultBufferPoolIdleTimeout, time.Now)
}

// newDefaultBufferPoolWithConfig returns a defaultBufferPool with the given
// block size, eviction timings and clock. Tests use it to shrink blocks and
// timers and to inject a fake clock. cleanInterval must be positive, because
// time.NewTicker panics otherwise.
func newDefaultBufferPoolWithConfig(blockSize int, cleanInterval, idleTimeout time.Duration, now func() time.Time) *defaultBufferPool {
	p := &defaultBufferPool{
		blockSize:     blockSize,
		cleanInterval: cleanInterval,
		idleTimeout:   idleTimeout,
		now:           now,
		start:         now(),
		stop:          make(chan struct{}),
	}
	p.touch()
	return p
}

// Get returns a buffer of min(maxSize, blockSize) bytes. It reuses a cached
// block if one is available and otherwise allocates one inline. It never
// blocks waiting for another caller and only fails if maxSize is not positive.
func (p *defaultBufferPool) Get(maxSize int) ([]byte, error) {
	if maxSize <= 0 {
		return nil, errInvalidBufferSize
	}
	p.touch()
	b, ok, cached := p.pop()
	if !ok {
		b = p.allocate()
	}
	p.maybeRefill(maxSize, cached)
	return b[:min(maxSize, p.blockSize)], nil
}

// TryGet returns a cached buffer of min(maxSize, blockSize) bytes. It never
// allocates inline: if no block is cached it returns false, with a nil error,
// and starts background pre-allocation so that a later call can succeed. It
// never blocks.
func (p *defaultBufferPool) TryGet(maxSize int) ([]byte, bool, error) {
	if maxSize <= 0 {
		return nil, false, errInvalidBufferSize
	}
	p.touch()
	b, ok, cached := p.pop()
	p.maybeRefill(maxSize, cached)
	if !ok {
		return nil, false, nil
	}
	return b[:min(maxSize, p.blockSize)], true, nil
}

// Put returns a buffer obtained from Get or TryGet to the pool. Buffers whose
// capacity is not blockSize are dropped. A buffer from elsewhere with the same
// capacity is adopted (without being counted in live), so callers must only
// Put buffers this pool vended.
//
// Put must be called exactly once per buffer. Returning the same buffer twice
// caches it twice, so two later callers would share, and corrupt, the same
// memory.
func (p *defaultBufferPool) Put(buf []byte) {
	if cap(buf) != p.blockSize {
		return
	}
	p.touch()
	p.push(buf[:p.blockSize])
}

// pop removes and returns the most recently cached block, if any, along with
// the number of blocks that remain cached.
func (p *defaultBufferPool) pop() (b []byte, ok bool, remaining int) {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	n := len(p.free)
	if n == 0 {
		return nil, false, 0
	}
	b = p.free[n-1]
	p.free[n-1] = nil // Do not retain the block via the backing array.
	p.free = p.free[:n-1]
	return b, true, n - 1
}

// push caches b, or drops it if the pool has been closed. It (re)starts the
// cleaner so that cached memory is eventually reclaimed.
//
// closed is checked while holding freeMu so that the check and the append are
// atomic with respect to close, which sets closed before draining free under
// the same lock. A push therefore either lands before the drain (and is
// drained) or observes closed and drops the block; it can never leave a block
// cached in a closed pool.
func (p *defaultBufferPool) push(b []byte) {
	p.freeMu.Lock()
	if p.closed.Load() {
		p.freeMu.Unlock()
		p.live.Add(-1)
		return
	}
	p.free = append(p.free, b)
	p.freeMu.Unlock()
	p.ensureCleaner()
}

// cached returns the number of cached blocks.
func (p *defaultBufferPool) cached() int {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	return len(p.free)
}

// evict drops up to n cached blocks in a single critical section.
func (p *defaultBufferPool) evict(n int) {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	p.evictLocked(n)
}

// evictHalf drops ceil(n/2) of the n cached blocks, so that a single cached
// block is eventually freed. Counting and dropping happen in one critical
// section, so concurrent Get or TryGet calls cannot make it drop more than
// half of what remains.
func (p *defaultBufferPool) evictHalf() {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	n := len(p.free)
	p.evictLocked(n - n/2)
}

// evictLocked drops up to n cached blocks. The caller must hold freeMu.
func (p *defaultBufferPool) evictLocked(n int) {
	n = min(n, len(p.free))
	keep := len(p.free) - n
	clear(p.free[keep:])
	p.free = p.free[:keep]
	p.live.Add(-int64(n))
}

// allocate returns a new zeroed block and counts it as owned by the pool.
func (p *defaultBufferPool) allocate() []byte {
	p.live.Add(1)
	return make([]byte, p.blockSize)
}

// blocksFor returns the number of blocks needed to hold size bytes, which
// must be positive. It is written as 1 + (size-1)/blockSize rather than
// (size+blockSize-1)/blockSize so that it cannot overflow for large sizes.
func (p *defaultBufferPool) blocksFor(size int) int {
	return 1 + (size-1)/p.blockSize
}

// maybeRefill starts a background goroutine that pre-allocates blocks when
// cached, the number of cached blocks, is below what a request of maxSize
// bytes would need, capped at defaultBufferPoolMaxRefillBlocks. At most one
// refill runs at a time; requests that arrive while one is running are served
// by it or trigger a new refill on their next call.
func (p *defaultBufferPool) maybeRefill(maxSize, cached int) {
	want := min(p.blocksFor(maxSize), defaultBufferPoolMaxRefillBlocks)
	// closed is checked again under mu below; this check only avoids touching
	// refilling and mu on a closed pool.
	if cached >= want || p.closed.Load() {
		return
	}
	if !p.refilling.CompareAndSwap(false, true) {
		return
	}
	p.mu.Lock()
	if p.closed.Load() {
		p.mu.Unlock()
		p.refilling.Store(false)
		return
	}
	p.wg.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.wg.Done()
		defer p.refilling.Store(false)
		for p.cached() < want && !p.closed.Load() {
			p.push(p.allocate())
		}
	}()
}

// touch records activity, postponing idle eviction.
func (p *defaultBufferPool) touch() {
	p.lastActive.Store(int64(p.now().Sub(p.start)))
}

// ensureCleaner starts the cleaner goroutine if it is not running. The common
// case, a running cleaner, is detected without taking mu.
func (p *defaultBufferPool) ensureCleaner() {
	if p.cleanerRunning.Load() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cleanerRunning.Load() || p.closed.Load() {
		return
	}
	p.cleanerRunning.Store(true)
	p.wg.Add(1)
	go p.runCleaner()
}

// runCleaner calls evictIfIdle every cleanInterval until the cache is empty
// or the pool is closed.
func (p *defaultBufferPool) runCleaner() {
	defer p.wg.Done()
	t := time.NewTicker(p.cleanInterval)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			if p.evictIfIdle() {
				return
			}
		}
	}
}

// evictIfIdle halves the number of cached blocks if the pool has been idle for
// at least idleTimeout. An eviction counts as activity, so each further
// halving needs another full idleTimeout without Get, TryGet or Put.
// It returns true, after marking the cleaner as stopped, once the cache is
// empty; the cleaner is restarted by the next push.
//
// cleanerRunning is only cleared when the cache looks empty, so a Put during
// an ordinary tick still takes the lock-free path in ensureCleaner. The
// cleaner then re-checks the cache. A concurrent push appends before reading
// cleanerRunning, so either this re-check sees the new block and the cleaner
// keeps running, or the push sees the cleared flag and starts a new cleaner.
func (p *defaultBufferPool) evictIfIdle() (stopped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	idle := p.now().Sub(p.start) - time.Duration(p.lastActive.Load())
	if idle >= p.idleTimeout {
		p.evictHalf()
		p.touch()
	}

	if p.cached() > 0 {
		return false
	}
	p.cleanerRunning.Store(false)
	if p.cached() > 0 {
		p.cleanerRunning.Store(true)
		return false
	}
	return true
}

// close stops all background goroutines and releases cached blocks. Buffers
// that are still held by callers may be Put afterwards; they are dropped.
func (p *defaultBufferPool) close() {
	p.mu.Lock()
	if p.closed.Load() {
		p.mu.Unlock()
		return
	}
	p.closed.Store(true)
	close(p.stop)
	p.mu.Unlock()
	p.wg.Wait()

	// The cleaner exits through stop without clearing cleanerRunning, so clear
	// it here. closed is set, so ensureCleaner can no longer set it again.
	p.mu.Lock()
	p.cleanerRunning.Store(false)
	p.mu.Unlock()

	// closed is already set, so any push that acquires freeMu after this
	// drain drops its block.
	p.evict(math.MaxInt)
}
