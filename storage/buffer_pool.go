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
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage/experimental"
)

const (
	// defaultBufferPoolBlockSize is the size of every block vended by the
	// default buffer pool. 8 MiB is a divisor of all common GCS part sizes
	// (8, 16, 32, 64, 128 MiB), so a part can be assembled from a whole
	// number of blocks.
	defaultBufferPoolBlockSize = 8 << 20

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
// It manages uniform 8 MiB blocks that are shared across all writers of a
// client. The pool has no memory limit; the memory in use is bounded by the
// writers' PartSize and Concurrency settings. It provides:
//
//   - Guaranteed progress: Get always returns a block, allocating one inline
//     if none is cached, so the first segment of every upload can proceed.
//   - Cache-only admission: TryGet only vends blocks that are already cached
//     and never allocates inline. When the cache is empty it returns false,
//     and the uploader continues with the segments it already holds.
//   - Async pre-allocation: when the number of cached blocks falls below what
//     a request needs, replacement blocks are allocated in the background,
//     overlapping allocation with network I/O. This is how the cache, and
//     therefore TryGet concurrency, ramps up on a cold pool.
//   - Staged idle eviction: after idleTimeout without activity, half of the
//     cached blocks are released to the GC; this repeats every idleTimeout
//     until the pool is empty, at which point the cleaner goroutine exits
//     until blocks are cached again.
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

	// live is the number of blocks currently owned by the pool, either cached
	// in free or handed out to callers. It is used for tests and diagnostics.
	live atomic.Int64

	// lastActive is the UnixNano timestamp of the last Get/TryGet/Put.
	lastActive atomic.Int64

	// refilling is set while a background pre-allocation goroutine is running.
	refilling atomic.Bool

	// shut is set once close has been called; checked lock-free on hot paths.
	shut atomic.Bool

	mu sync.Mutex
	// cleanerRunning reports whether the cleaner goroutine is active.
	cleanerRunning bool
	// lastEvict is when the cleaner last evicted blocks.
	lastEvict time.Time
	// closed is set once close has been called.
	closed bool
	// stop is closed to terminate the cleaner goroutine.
	stop chan struct{}
	// wg tracks background goroutines; used by close and tests.
	wg sync.WaitGroup
}

// newDefaultBufferPool returns a defaultBufferPool with the SDK defaults.
func newDefaultBufferPool() *defaultBufferPool {
	return newDefaultBufferPoolWithConfig(defaultBufferPoolBlockSize,
		defaultBufferPoolCleanInterval, defaultBufferPoolIdleTimeout)
}

func newDefaultBufferPoolWithConfig(blockSize int, cleanInterval, idleTimeout time.Duration) *defaultBufferPool {
	p := &defaultBufferPool{
		blockSize:     blockSize,
		cleanInterval: cleanInterval,
		idleTimeout:   idleTimeout,
		now:           time.Now,
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
	b, ok := p.pop()
	if !ok {
		p.live.Add(1)
		b = p.allocate()
	}
	p.maybeRefill(maxSize)
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
	b, ok := p.pop()
	p.maybeRefill(maxSize)
	if !ok {
		return nil, false, nil
	}
	return b[:min(maxSize, p.blockSize)], true, nil
}

// Put returns a buffer obtained from Get or TryGet to the pool. Buffers that
// were not vended by this pool (wrong capacity) are ignored.
func (p *defaultBufferPool) Put(buf []byte) {
	if cap(buf) != p.blockSize {
		return
	}
	p.touch()
	p.push(buf[:p.blockSize])
}

// pop removes and returns the most recently cached block, if any.
func (p *defaultBufferPool) pop() ([]byte, bool) {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	n := len(p.free)
	if n == 0 {
		return nil, false
	}
	b := p.free[n-1]
	p.free[n-1] = nil // Do not retain the block via the backing array.
	p.free = p.free[:n-1]
	return b, true
}

// push caches b, or drops it if the pool has been closed. It (re)starts the
// cleaner so that cached memory is eventually reclaimed.
func (p *defaultBufferPool) push(b []byte) {
	if p.shut.Load() {
		p.live.Add(-1)
		return
	}
	p.freeMu.Lock()
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

// evict drops up to n cached blocks and returns how many were dropped.
func (p *defaultBufferPool) evict(n int) int {
	p.freeMu.Lock()
	defer p.freeMu.Unlock()
	n = min(n, len(p.free))
	keep := len(p.free) - n
	clear(p.free[keep:])
	p.free = p.free[:keep]
	p.live.Add(-int64(n))
	return n
}

func (p *defaultBufferPool) allocate() []byte {
	return make([]byte, p.blockSize)
}

// blocksFor returns the number of blocks needed to hold size bytes.
func (p *defaultBufferPool) blocksFor(size int) int {
	return max(1, (size+p.blockSize-1)/p.blockSize)
}

// maybeRefill starts a background goroutine that pre-allocates blocks when
// fewer blocks are cached than a request of maxSize bytes would need. At most
// one refill runs at a time; requests that arrive while one is running are
// served by it or trigger a new refill on their next call.
func (p *defaultBufferPool) maybeRefill(maxSize int) {
	want := p.blocksFor(maxSize)
	if p.cached() >= want {
		return
	}
	if !p.refilling.CompareAndSwap(false, true) {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.refilling.Store(false)
		return
	}
	p.wg.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.wg.Done()
		defer p.refilling.Store(false)
		for p.cached() < want && !p.shut.Load() {
			p.live.Add(1)
			p.push(p.allocate())
		}
	}()
}

// touch records activity, postponing idle eviction.
func (p *defaultBufferPool) touch() {
	p.lastActive.Store(p.now().UnixNano())
}

// ensureCleaner starts the cleaner goroutine if it is not running.
func (p *defaultBufferPool) ensureCleaner() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cleanerRunning || p.closed {
		return
	}
	p.cleanerRunning = true
	p.lastEvict = time.Time{}
	p.wg.Add(1)
	go p.runCleaner()
}

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
// at least idleTimeout and no eviction has happened in the last idleTimeout.
// It returns true, after marking the cleaner as stopped, once the cache is
// empty; the cleaner is restarted by the next push.
func (p *defaultBufferPool) evictIfIdle() (stopped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	idleSince := time.Unix(0, p.lastActive.Load())
	if now.Sub(idleSince) >= p.idleTimeout && now.Sub(p.lastEvict) >= p.idleTimeout {
		n := p.cached()
		// Evict ceil(n/2) so that a single cached block is eventually freed.
		p.evict(n - n/2)
		p.lastEvict = now
	}

	if p.cached() == 0 {
		p.cleanerRunning = false
		return true
	}
	return false
}

// close stops all background goroutines and releases cached blocks. Buffers
// that are still held by callers may be Put afterwards; they are dropped.
func (p *defaultBufferPool) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.shut.Store(true)
	close(p.stop)
	p.mu.Unlock()
	p.wg.Wait()
	p.evict(p.cached())
}
