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
	"context"
	"errors"
	"math"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/option"
)

const testBlockSize = 1024

// newTestBufferPool returns a pool with small blocks, a fake clock, and a
// cleaner interval long enough that the test drives eviction manually.
func newTestBufferPool(t *testing.T) (*defaultBufferPool, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	p := newDefaultBufferPoolWithConfig(testBlockSize, time.Hour, 30*time.Second)
	p.now = clk.Now
	p.touch()
	t.Cleanup(p.close)
	return p, clk
}

// waitFor polls cond until it is true or the deadline expires.
func waitFor(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", desc)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitIdle waits for any background pre-allocation to finish.
func waitIdle(t *testing.T, p *defaultBufferPool) {
	t.Helper()
	waitFor(t, "refill to finish", func() bool { return !p.refilling.Load() })
}

// tryGetEventually calls TryGet until it succeeds, relying on background
// pre-allocation to fill the cache.
func tryGetEventually(t *testing.T, p *defaultBufferPool, maxSize int) []byte {
	t.Helper()
	var b []byte
	waitFor(t, "TryGet to succeed", func() bool {
		var ok bool
		var err error
		b, ok, err = p.TryGet(maxSize)
		if err != nil {
			t.Fatalf("TryGet(%d): %v", maxSize, err)
		}
		return ok
	})
	return b
}

func TestDefaultBufferPoolGetSizes(t *testing.T) {
	p, _ := newTestBufferPool(t)
	for _, tc := range []struct {
		maxSize int
		wantLen int
	}{
		{1, 1},
		{testBlockSize / 2, testBlockSize / 2},
		{testBlockSize, testBlockSize},
		{3 * testBlockSize, testBlockSize},
	} {
		b, err := p.Get(tc.maxSize)
		if err != nil {
			t.Fatalf("Get(%d): %v", tc.maxSize, err)
		}
		if len(b) != tc.wantLen || cap(b) != testBlockSize {
			t.Errorf("Get(%d): len=%d cap=%d, want len=%d cap=%d", tc.maxSize, len(b), cap(b), tc.wantLen, testBlockSize)
		}
		b2 := tryGetEventually(t, p, tc.maxSize)
		if len(b2) != tc.wantLen || cap(b2) != testBlockSize {
			t.Errorf("TryGet(%d): len=%d cap=%d, want len=%d cap=%d", tc.maxSize, len(b2), cap(b2), tc.wantLen, testBlockSize)
		}
		p.Put(b)
		p.Put(b2)
		waitIdle(t, p)
	}
}

func TestDefaultBufferPoolInvalidSize(t *testing.T) {
	p, _ := newTestBufferPool(t)
	for _, n := range []int{0, -1} {
		if _, err := p.Get(n); !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("Get(%d) err = %v, want %v", n, err, errInvalidBufferSize)
		}
		if _, ok, err := p.TryGet(n); ok || !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("TryGet(%d) = %v, %v; want false, %v", n, ok, err, errInvalidBufferSize)
		}
	}
}

// Get must always succeed, even on an empty pool, so that the first segment
// of every upload makes progress.
func TestDefaultBufferPoolGetAlwaysSucceeds(t *testing.T) {
	p, _ := newTestBufferPool(t)
	const n = 100
	var held [][]byte
	for i := 0; i < n; i++ {
		b, err := p.Get(testBlockSize)
		if err != nil || len(b) != testBlockSize {
			t.Fatalf("Get #%d = len %d, %v", i, len(b), err)
		}
		held = append(held, b)
	}
	waitIdle(t, p)
	// The pool has no limit: every returned block is cached.
	for _, b := range held {
		p.Put(b)
	}
	if got := p.cached(); got < n {
		t.Errorf("cached = %d, want >= %d", got, n)
	}
}

// TryGet must not allocate inline: on an empty pool it reports false without
// an error and starts background pre-allocation so a later call succeeds.
func TestDefaultBufferPoolTryGetCacheOnly(t *testing.T) {
	p, _ := newTestBufferPool(t)

	b, ok, err := p.TryGet(3 * testBlockSize)
	if ok || err != nil || b != nil {
		t.Fatalf("TryGet on cold pool = %v, %v, %v; want nil, false, nil", b, ok, err)
	}
	waitFor(t, "pre-allocation", func() bool { return p.cached() >= 3 })
	waitIdle(t, p)
	for i := 0; i < 3; i++ {
		if _, ok, err := p.TryGet(testBlockSize); !ok || err != nil {
			t.Fatalf("TryGet #%d after pre-allocation = %v, %v; want true, nil", i, ok, err)
		}
	}
}

func TestDefaultBufferPoolBlocksFor(t *testing.T) {
	p, _ := newTestBufferPool(t)
	for _, tc := range []struct {
		size int
		want int
	}{
		{1, 1},
		{testBlockSize - 1, 1},
		{testBlockSize, 1},
		{testBlockSize + 1, 2},
		{3 * testBlockSize, 3},
		// Must not overflow for very large sizes.
		{math.MaxInt, 1 + (math.MaxInt-1)/testBlockSize},
	} {
		if got := p.blocksFor(tc.size); got != tc.want {
			t.Errorf("blocksFor(%d) = %d, want %d", tc.size, got, tc.want)
		}
	}
}

// A huge maxSize must not make background pre-allocation grow without bound.
func TestDefaultBufferPoolRefillCap(t *testing.T) {
	p, _ := newTestBufferPool(t)
	b, err := p.Get(math.MaxInt)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != testBlockSize {
		t.Errorf("Get(math.MaxInt) len = %d, want %d", len(b), testBlockSize)
	}
	waitFor(t, "pre-allocation", func() bool { return p.cached() >= defaultBufferPoolMaxRefillBlocks })
	waitIdle(t, p)
	if got := p.cached(); got != defaultBufferPoolMaxRefillBlocks {
		t.Errorf("cached = %d, want %d", got, defaultBufferPoolMaxRefillBlocks)
	}
	if _, ok, err := p.TryGet(math.MaxInt); !ok || err != nil {
		t.Errorf("TryGet(math.MaxInt) = %v, %v; want true, nil", ok, err)
	}
}

// Put racing with close must never leave a block cached in a closed pool.
func TestDefaultBufferPoolPutCloseRace(t *testing.T) {
	const (
		rounds  = 50
		workers = 16
	)
	for r := 0; r < rounds; r++ {
		p := newDefaultBufferPoolWithConfig(testBlockSize, time.Hour, time.Hour)
		held := make([][]byte, workers)
		for i := range held {
			held[i], _ = p.Get(testBlockSize)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, b := range held {
			wg.Add(1)
			go func(b []byte) {
				defer wg.Done()
				<-start
				p.Put(b)
			}(b)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p.close()
		}()
		close(start)
		wg.Wait()

		if got := p.cached(); got != 0 {
			t.Fatalf("round %d: %d blocks cached in closed pool", r, got)
		}
		if got := p.live.Load(); got != 0 {
			t.Fatalf("round %d: live = %d after close and all Puts, want 0", r, got)
		}
	}
}

func TestDefaultBufferPoolReuse(t *testing.T) {
	p, _ := newTestBufferPool(t)
	b, err := p.Get(testBlockSize / 2)
	if err != nil {
		t.Fatal(err)
	}
	waitIdle(t, p)
	p.evict(p.cached()) // Drop pre-allocated blocks so only b is cached.

	// Put accepts the exact (possibly short) slice returned by Get.
	p.Put(b)
	b2, ok, err := p.TryGet(testBlockSize)
	if !ok || err != nil {
		t.Fatalf("TryGet after Put = %v, %v; want true, nil", ok, err)
	}
	if &b[0] != &b2[0] {
		t.Error("TryGet after Put did not reuse the cached block")
	}
}

func TestDefaultBufferPoolLIFO(t *testing.T) {
	p, _ := newTestBufferPool(t)
	b1, _ := p.Get(testBlockSize)
	b2, _ := p.Get(testBlockSize)
	waitIdle(t, p)
	p.evict(p.cached())
	p.Put(b1)
	p.Put(b2)
	got, _ := p.Get(testBlockSize)
	if &got[0] != &b2[0] {
		t.Error("Get did not return the most recently Put block")
	}
}

func TestDefaultBufferPoolPutForeignBuffer(t *testing.T) {
	p, _ := newTestBufferPool(t)
	p.Put(make([]byte, testBlockSize+1))
	p.Put(make([]byte, 10))
	p.Put(nil)
	if p.cached() != 0 || p.live.Load() != 0 {
		t.Errorf("foreign buffers were cached: cached=%d live=%d", p.cached(), p.live.Load())
	}
}

func TestDefaultBufferPoolPreallocation(t *testing.T) {
	p, _ := newTestBufferPool(t)
	// A request for a 3-block part should trigger background allocation of
	// enough spare blocks for the rest of the part.
	if _, err := p.Get(3 * testBlockSize); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pre-allocation", func() bool { return p.cached() >= 3 })
	waitIdle(t, p)
	if got := p.live.Load(); got != 4 {
		t.Errorf("live = %d, want 4", got)
	}
	// No refill when enough blocks are already cached.
	if _, ok, _ := p.TryGet(testBlockSize); !ok {
		t.Fatal("TryGet failed with cached blocks")
	}
	if p.refilling.Load() {
		t.Error("refill started although enough blocks were cached")
	}
}

func TestDefaultBufferPoolStagedEviction(t *testing.T) {
	p, clk := newTestBufferPool(t)
	var bufs [][]byte
	for i := 0; i < 8; i++ {
		b, _ := p.Get(1)
		bufs = append(bufs, b)
	}
	waitIdle(t, p)
	for _, b := range bufs {
		p.Put(b)
	}
	n := p.cached()
	if n < 8 {
		t.Fatalf("cached = %d, want >= 8", n)
	}

	steps := []struct {
		advance  time.Duration
		wantFree int
		desc     string
	}{
		{15 * time.Second, n, "idle 15s: no eviction"},
		{15 * time.Second, n / 2, "idle 30s: halve"},
		{15 * time.Second, n / 2, "15s after eviction: no eviction"},
		{15 * time.Second, n / 4, "30s after eviction: halve again"},
	}
	for _, s := range steps {
		clk.Advance(s.advance)
		stopped := p.evictIfIdle()
		if got := p.cached(); got != s.wantFree || stopped {
			t.Fatalf("%s: cached=%d stopped=%v, want cached=%d stopped=false", s.desc, got, stopped, s.wantFree)
		}
	}

	// Activity postpones eviction.
	b, _ := p.Get(1)
	p.Put(b)
	waitIdle(t, p)
	before := p.cached()
	clk.Advance(15 * time.Second)
	if p.evictIfIdle(); p.cached() != before {
		t.Fatalf("evicted %d blocks 15s after activity", before-p.cached())
	}

	// Keep halving until empty; the cleaner then reports it has stopped.
	for i := 0; i < 10 && p.cached() > 0; i++ {
		clk.Advance(30 * time.Second)
		if stopped := p.evictIfIdle(); stopped != (p.cached() == 0) {
			t.Fatalf("stopped=%v with %d cached", stopped, p.cached())
		}
	}
	if p.cached() != 0 || p.live.Load() != 0 {
		t.Fatalf("pool not drained: cached=%d live=%d", p.cached(), p.live.Load())
	}
}

func TestDefaultBufferPoolCleanerLifecycle(t *testing.T) {
	// A fast real ticker drives the cleaner goroutine, while the fake clock
	// decides when the pool counts as idle, so eviction never happens early.
	clk := newFakeClock()
	p := newDefaultBufferPoolWithConfig(testBlockSize, time.Millisecond, 30*time.Second)
	p.now = clk.Now
	p.touch()
	defer p.close()

	if p.cleanerRunning.Load() {
		t.Fatal("cleaner running before any block was cached")
	}
	b, _ := p.Get(testBlockSize)
	p.Put(b)
	if !p.cleanerRunning.Load() {
		t.Fatal("cleaner not started after Put")
	}
	waitFor(t, "cleaner to drain the pool and stop", func() bool {
		clk.Advance(30 * time.Second)
		return p.cached() == 0 && !p.refilling.Load() && !p.cleanerRunning.Load()
	})

	// The next Put restarts the cleaner.
	b, _ = p.Get(testBlockSize)
	p.Put(b)
	if !p.cleanerRunning.Load() {
		t.Fatal("cleaner not restarted after Put")
	}
}

// A Put racing with the cleaner's decision to stop must never leave a cached
// block without a running cleaner.
func TestDefaultBufferPoolCleanerStopRace(t *testing.T) {
	// idleTimeout 0 makes every tick evict, so the cleaner stops and restarts
	// as often as possible.
	p := newDefaultBufferPoolWithConfig(testBlockSize, time.Millisecond, 0)
	defer p.close()

	const (
		workers = 8
		iters   = 200
	)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				b, _ := p.Get(testBlockSize)
				p.Put(b)
			}
		}()
	}
	wg.Wait()

	// If a block were stranded without a cleaner, the cache would never empty.
	waitFor(t, "cleaner to drain the pool", func() bool {
		return p.cached() == 0 && !p.refilling.Load() && !p.cleanerRunning.Load()
	})
}

func TestDefaultBufferPoolClose(t *testing.T) {
	p := newDefaultBufferPoolWithConfig(testBlockSize, time.Hour, time.Hour)
	b1, _ := p.Get(testBlockSize)
	b2, _ := p.Get(testBlockSize)
	p.Put(b1)
	p.close()
	// close must be idempotent.
	p.close()

	if p.cached() != 0 {
		t.Errorf("cached = %d after close, want 0", p.cached())
	}
	// Buffers returned after close are dropped.
	p.Put(b2)
	if p.cached() != 0 || p.live.Load() != 0 {
		t.Errorf("after close: cached=%d live=%d, want 0, 0", p.cached(), p.live.Load())
	}
	// TryGet finds nothing and must not start pre-allocation; Get still works.
	if _, ok, err := p.TryGet(testBlockSize); ok || err != nil {
		t.Errorf("TryGet after close = %v, %v; want false, nil", ok, err)
	}
	if p.refilling.Load() {
		t.Error("pre-allocation started after close")
	}
	if _, err := p.Get(testBlockSize); err != nil {
		t.Errorf("Get after close: %v", err)
	}
}

func TestDefaultBufferPoolConcurrency(t *testing.T) {
	const (
		workers = 32
		iters   = 500
	)
	p := newDefaultBufferPoolWithConfig(testBlockSize, time.Millisecond, 2*time.Millisecond)
	defer p.close()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Mimic part assembly: one guaranteed Get, then TryGet until
				// the part is full or no cached block is available.
				first, err := p.Get(4 * testBlockSize)
				if err != nil {
					t.Error(err)
					return
				}
				held := [][]byte{first}
				for j := 0; j < 3; j++ {
					b, ok, err := p.TryGet(4 * testBlockSize)
					if err != nil {
						t.Error(err)
						return
					}
					if !ok {
						break
					}
					held = append(held, b)
				}
				for _, b := range held {
					b[0], b[len(b)-1] = byte(w), byte(i)
					p.Put(b)
				}
			}
		}(w)
	}
	wg.Wait()
	waitIdle(t, p)

	// Nothing is outstanding, so every live block must be cached.
	if live, cached := p.live.Load(), int64(p.cached()); live != cached {
		t.Errorf("leaked blocks: live=%d cached=%d", live, cached)
	}
}

func BenchmarkDefaultBufferPoolSteadyState(b *testing.B) {
	p := newDefaultBufferPool()
	defer p.close()
	// A multi-block part so that TryGet and background refills are exercised.
	const partSize = 4 * defaultBufferPoolBlockSize
	held := make([][]byte, 0, partSize/defaultBufferPoolBlockSize)
	// assemble mimics part assembly: one guaranteed Get followed by TryGet
	// until the part is full, then releases every block.
	assemble := func() {
		buf, _ := p.Get(partSize)
		held = append(held, buf)
		for len(held) < cap(held) {
			buf, ok, _ := p.TryGet(partSize)
			if !ok {
				break
			}
			held = append(held, buf)
		}
		for _, buf := range held {
			p.Put(buf)
		}
		held = held[:0]
	}
	// Warm up until background pre-allocation has grown the pool to its
	// steady-state size.
	for prev := int64(-1); prev != p.live.Load(); {
		prev = p.live.Load()
		for i := 0; i < 10000; i++ {
			assemble()
		}
		for p.refilling.Load() {
			time.Sleep(time.Millisecond)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		assemble()
	}
}

func BenchmarkDefaultBufferPoolParallel(b *testing.B) {
	p := newDefaultBufferPool()
	defer p.close()
	// Warm the pool with enough blocks for every parallel goroutine so the
	// timed loop measures steady state rather than pool growth.
	warm := make([][]byte, 2*runtime.GOMAXPROCS(0))
	for i := range warm {
		warm[i], _ = p.Get(defaultBufferPoolBlockSize)
	}
	for _, buf := range warm {
		p.Put(buf)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf, _ := p.Get(defaultBufferPoolBlockSize)
			p.Put(buf)
		}
	})
}

func TestGRPCClientBufferPool(t *testing.T) {
	ctx := context.Background()
	baseOpts := []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithEndpoint("localhost:0"),
		WithDisabledClientMetrics(),
	}

	t.Run("default", func(t *testing.T) {
		c, err := newGRPCStorageClient(ctx, withClientOptions(baseOpts...))
		if err != nil {
			t.Fatal(err)
		}
		pool, ok := c.bufferPool.(*defaultBufferPool)
		if !ok {
			t.Fatalf("bufferPool = %T, want *defaultBufferPool", c.bufferPool)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		if !pool.closed.Load() {
			t.Error("default pool not closed with the client")
		}
	})

	t.Run("custom", func(t *testing.T) {
		custom := &struct{ experimental.BufferPool }{}
		opts := append(slices.Clone(baseOpts), experimental.WithBufferPool(custom))
		c, err := newGRPCStorageClient(ctx, withClientOptions(opts...))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if c.bufferPool != experimental.BufferPool(custom) {
			t.Error("bufferPool is not the custom pool")
		}
	})
}
