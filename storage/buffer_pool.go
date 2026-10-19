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

	// defaultBufferPoolMaxBlocks bounds the number of blocks the default pool
	// will hand out through TryGet (512 MiB). Get ignores this limit so that
	// every upload can always make progress.
	defaultBufferPoolMaxBlocks = 64

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
// client, and provides:
//
//   - Bounded admission: TryGet refuses to grow the pool past maxBlocks live
//     blocks; Get always succeeds so the first buffer of an upload is
//     guaranteed.
//   - Async pre-allocation: when the number of spare blocks falls below what
//     the caller is likely to need next, replacement blocks are allocated in
//     the background, overlapping allocation with network I/O.
//   - Staged idle eviction: after idleTimeout without activity, half of the
//     cached blocks are released to the GC; this repeats every idleTimeout
//     until the pool is empty, at which point the cleaner goroutine exits
//     until blocks are cached again.
//
// All methods are safe for concurrent use.
type defaultBufferPool struct {
	blockSize     int
	maxBlocks     int64
	cleanInterval time.Duration
	idleTimeout   time.Duration
	now           func() time.Time

	// free holds cached blocks ready to be vended. Its capacity is maxBlocks.
	free chan []byte

	// live is the number of blocks currently owned by the pool, either cached
	// in free or handed out to callers.
	live atomic.Int64

	// lastActive is the UnixNano timestamp of the last Get/TryGet/Put.
	lastActive atomic.Int64

	// refilling is set while a background pre-allocation goroutine is running.
	refilling atomic.Bool

	mu sync.Mutex
	// cleanerRunning reports whether the cleaner goroutine is active.
	cleanerRunning bool
	// lastEvict is when the cleaner last evicted blocks.
	lastEvict time.Time
	// closed is set once close has been called.
	closed bool
	// shut mirrors closed for lock-free checks on the Put path.
	shut atomic.Bool
	// stop is closed to terminate the cleaner goroutine.
	stop chan struct{}
	// wg tracks background goroutines; used by close and tests.
	wg sync.WaitGroup
}

// newDefaultBufferPool returns a defaultBufferPool with the SDK defaults.
func newDefaultBufferPool() *defaultBufferPool {
	return newDefaultBufferPoolWithConfig(defaultBufferPoolBlockSize, defaultBufferPoolMaxBlocks,
		defaultBufferPoolCleanInterval, defaultBufferPoolIdleTimeout)
}

func newDefaultBufferPoolWithConfig(blockSize, maxBlocks int, cleanInterval, idleTimeout time.Duration) *defaultBufferPool {
	p := &defaultBufferPool{
		blockSize:     blockSize,
		maxBlocks:     int64(maxBlocks),
		cleanInterval: cleanInterval,
		idleTimeout:   idleTimeout,
		now:           time.Now,
		free:          make(chan []byte, maxBlocks),
		stop:          make(chan struct{}),
	}
	p.touch()
	return p
}

// Get returns a buffer of min(maxSize, blockSize) bytes. It ignores the pool
// limit, never blocks waiting for another caller, and only fails if maxSize is
// not positive.
func (p *defaultBufferPool) Get(maxSize int) ([]byte, error) {
	if maxSize <= 0 {
		return nil, errInvalidBufferSize
	}
	p.touch()
	var b []byte
	select {
	case b = <-p.free:
	default:
		p.live.Add(1)
		b = p.allocate()
	}
	p.maybeRefill(maxSize)
	return b[:min(maxSize, p.blockSize)], nil
}

// TryGet returns a buffer of min(maxSize, blockSize) bytes if one is cached or
// if the pool is below its limit. It returns false, with a nil error, if the
// pool is exhausted. It never blocks.
func (p *defaultBufferPool) TryGet(maxSize int) ([]byte, bool, error) {
	if maxSize <= 0 {
		return nil, false, errInvalidBufferSize
	}
	p.touch()
	var b []byte
	select {
	case b = <-p.free:
	default:
		if !p.reserve() {
			return nil, false, nil
		}
		b = p.allocate()
	}
	p.maybeRefill(maxSize)
	return b[:min(maxSize, p.blockSize)], true, nil
}

// Put returns a buffer obtained from Get or TryGet to the pool. Buffers that
// were not vended by this pool (wrong capacity) are ignored.
func (p *defaultBufferPool) Put(buf []byte) {
	if cap(buf) != p.blockSize {
		return
	}
	p.touch()
	p.cache(buf[:p.blockSize])
}

// cache stores b in the free list, or drops it if the free list is full (which
// can happen when Get grew the pool past maxBlocks). It (re)starts the cleaner
// so cached memory is eventually reclaimed.
func (p *defaultBufferPool) cache(b []byte) {
	if p.shut.Load() {
		p.live.Add(-1)
		return
	}
	select {
	case p.free <- b:
		p.ensureCleaner()
	default:
		p.live.Add(-1)
	}
}

// reserve increments live if doing so keeps it within maxBlocks.
func (p *defaultBufferPool) reserve() bool {
	for {
		n := p.live.Load()
		if n >= p.maxBlocks {
			return false
		}
		if p.live.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

func (p *defaultBufferPool) allocate() []byte {
	return make([]byte, p.blockSize)
}

// blocksFor returns the number of blocks needed to hold size bytes.
func (p *defaultBufferPool) blocksFor(size int) int {
	return max(1, (size+p.blockSize-1)/p.blockSize)
}

// maybeRefill starts a background goroutine that pre-allocates blocks when
// fewer spare blocks are cached than a request of maxSize bytes would need.
func (p *defaultBufferPool) maybeRefill(maxSize int) {
	want := p.blocksFor(maxSize)
	if len(p.free) >= want {
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
		for len(p.free) < want && p.reserve() {
			p.cache(p.allocate())
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
// It returns true, after marking the cleaner as stopped, once the free list is
// empty; the cleaner is restarted by the next cache.
func (p *defaultBufferPool) evictIfIdle() (stopped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	idleSince := time.Unix(0, p.lastActive.Load())
	if now.Sub(idleSince) >= p.idleTimeout && now.Sub(p.lastEvict) >= p.idleTimeout {
		n := len(p.free)
		// Evict ceil(n/2) so that a single cached block is eventually freed.
		for i := 0; i < n-n/2; i++ {
			select {
			case <-p.free:
				p.live.Add(-1)
			default:
			}
		}
		p.lastEvict = now
	}

	if len(p.free) == 0 {
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
	for {
		select {
		case <-p.free:
			p.live.Add(-1)
		default:
			return
		}
	}
}
