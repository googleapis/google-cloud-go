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
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/option"
)

const testBlockSize = 1024

// newTestBufferPool returns a pool with small blocks and a clean interval long
// enough that the cleaner never runs; tests call shrinkUnused directly.
func newTestBufferPool(t *testing.T) *defaultBufferPool {
	t.Helper()
	p := newDefaultBufferPool()
	p.blockSize = testBlockSize
	p.cleanInterval = time.Hour
	t.Cleanup(p.close)
	return p
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

// cached returns the number of cached blocks.
func (p *defaultBufferPool) cached() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.free)
}

func (p *defaultBufferPool) isCleanerRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cleanerRunning
}

func (p *defaultBufferPool) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func TestDefaultBufferPoolGetSizes(t *testing.T) {
	p := newTestBufferPool(t)
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
		b2, ok, err := p.TryGet(tc.maxSize)
		if !ok || err != nil {
			t.Fatalf("TryGet(%d) = %v, %v; want true, nil", tc.maxSize, ok, err)
		}
		if len(b2) != tc.wantLen || cap(b2) != testBlockSize {
			t.Errorf("TryGet(%d): len=%d cap=%d, want len=%d cap=%d", tc.maxSize, len(b2), cap(b2), tc.wantLen, testBlockSize)
		}
		// Nothing is allocated ahead of time.
		if got := p.cached(); got != 0 {
			t.Errorf("cached = %d before Put, want 0", got)
		}
		p.Put(b)
		p.Put(b2)
	}
}

func TestDefaultBufferPoolInvalidSize(t *testing.T) {
	p := newTestBufferPool(t)
	for _, n := range []int{0, -1} {
		if _, err := p.Get(n); !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("Get(%d) err = %v, want %v", n, err, errInvalidBufferSize)
		}
		if _, ok, err := p.TryGet(n); ok || !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("TryGet(%d) = %v, %v; want false, %v", n, ok, err, errInvalidBufferSize)
		}
	}
}

// Put racing with close must never leave a block cached in a closed pool.
func TestDefaultBufferPoolPutCloseRace(t *testing.T) {
	const (
		rounds  = 50
		workers = 16
	)
	for r := 0; r < rounds; r++ {
		p := newDefaultBufferPool()
		p.blockSize = testBlockSize
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
	}
}

// Put caches the block for reuse, and the cache hands out the most recently
// returned block first (LIFO).
func TestDefaultBufferPoolReuse(t *testing.T) {
	p := newTestBufferPool(t)
	b1, _ := p.Get(testBlockSize)
	b2, _ := p.Get(testBlockSize / 2)
	p.Put(b1)
	// Put accepts the exact (possibly short) slice returned by Get.
	p.Put(b2)

	got, _ := p.Get(testBlockSize)
	if &got[0] != &b2[0] || len(got) != testBlockSize {
		t.Errorf("Get did not return the most recently Put block at full length (len=%d)", len(got))
	}
	got, _ = p.Get(testBlockSize)
	if &got[0] != &b1[0] {
		t.Error("second Get did not reuse the earlier Put block")
	}
}

func TestDefaultBufferPoolPutForeignBuffer(t *testing.T) {
	p := newTestBufferPool(t)
	p.Put(make([]byte, testBlockSize+1))
	p.Put(make([]byte, 10))
	p.Put(nil)
	if got := p.cached(); got != 0 {
		t.Errorf("foreign buffers were cached: cached=%d", got)
	}
}

// cacheBlocks Gets n blocks and Puts them all back, so n blocks are cached and
// minFree is n.
func cacheBlocks(tb testing.TB, p *defaultBufferPool, n int) {
	tb.Helper()
	bufs := make([][]byte, n)
	for i := range bufs {
		bufs[i], _ = p.Get(p.blockSize)
	}
	for _, b := range bufs {
		p.Put(b)
	}
	// Only the first Put starts the cleaner and sets minFree, so it is 1 here.
	// Reset it to the cache size, as a cleaner run would.
	p.mu.Lock()
	p.minFree = len(p.free)
	p.mu.Unlock()
	if got := p.cached(); got != n {
		tb.Fatalf("cached = %d, want %d", got, n)
	}
}

// A fully idle pool halves its cache on every cleaner run.
func TestDefaultBufferPoolStagedEviction(t *testing.T) {
	p := newTestBufferPool(t)
	cacheBlocks(t, p, 7)
	for _, want := range []int{3, 1, 0} {
		drained := p.shrinkUnused()
		if got := p.cached(); got != want || drained != (want == 0) {
			t.Fatalf("cached=%d drained=%v, want cached=%d drained=%v", got, drained, want, want == 0)
		}
	}
}

// Blocks that stay cached while a smaller working set is reused are evicted
// down to that working set, even though the pool is never idle.
func TestDefaultBufferPoolShrinksToWorkingSet(t *testing.T) {
	p := newTestBufferPool(t)
	cacheBlocks(t, p, 10)
	// Each interval, one writer reuses a single block, so the bottom blocks
	// are never touched.
	for _, want := range []int{5, 3, 2, 1, 1} {
		b, _ := p.Get(testBlockSize)
		p.Put(b)
		if p.shrinkUnused() {
			t.Fatal("pool drained while a block is still in use")
		}
		if got := p.cached(); got != want {
			t.Fatalf("cached = %d, want %d", got, want)
		}
		// Eviction drops the coldest blocks at the bottom, so the warm block
		// on top survives.
		got, _ := p.Get(testBlockSize)
		if &got[0] != &b[0] {
			t.Fatal("next Get did not return the warm block")
		}
		p.Put(got)
	}
	// Blocks held across a run are not counted as unused.
	held, _ := p.Get(testBlockSize)
	p.shrinkUnused()
	if got := p.cached(); got != 0 {
		t.Fatalf("cached = %d while the only block is held, want 0", got)
	}
	p.Put(held)
	if got := p.cached(); got != 1 {
		t.Fatalf("cached = %d after Put, want 1", got)
	}
}

// Blocks that enter the cache during an interval were in use for part of it,
// so the next cleaner run must not evict them.
func TestDefaultBufferPoolKeepsBlocksAddedMidInterval(t *testing.T) {
	p := newTestBufferPool(t)
	cacheBlocks(t, p, 2)
	var held [][]byte
	// Take both cached blocks (minFree drops to 0), then three fresh ones.
	for i := 0; i < 5; i++ {
		b, _ := p.Get(testBlockSize)
		held = append(held, b)
	}
	for _, b := range held {
		p.Put(b)
	}
	p.shrinkUnused()
	if got := p.cached(); got != 5 {
		t.Fatalf("cached = %d after first run, want 5 (nothing was unused all interval)", got)
	}
	// All five then sit unused for a full interval: evict ceil(5/2) = 3.
	p.shrinkUnused()
	if got := p.cached(); got != 2 {
		t.Fatalf("cached = %d after second run, want 2", got)
	}
}

func TestDefaultBufferPoolCleanerLifecycle(t *testing.T) {
	p := newDefaultBufferPool()
	p.blockSize = testBlockSize
	// Short, so the drain below finishes quickly, but long enough that the
	// cleaner is unlikely to evict the only block and exit before the test
	// checks it is running.
	p.cleanInterval = 10 * time.Millisecond
	defer p.close()

	if p.isCleanerRunning() {
		t.Fatal("cleaner running before any block was cached")
	}
	b, _ := p.Get(testBlockSize)
	p.Put(b)
	if !p.isCleanerRunning() {
		t.Fatal("cleaner not started after a block was cached")
	}
	waitFor(t, "cleaner to drain the pool and stop", func() bool {
		return p.cached() == 0 && !p.isCleanerRunning()
	})

	// The next cached block restarts the cleaner.
	b, _ = p.Get(testBlockSize)
	p.Put(b)
	if !p.isCleanerRunning() {
		t.Fatal("cleaner not restarted after a block was cached")
	}
}

func TestDefaultBufferPoolClose(t *testing.T) {
	p := newTestBufferPool(t)
	b1, _ := p.Get(testBlockSize)
	b2, _ := p.Get(testBlockSize)
	p.Put(b1)
	p.close()
	// close must be idempotent.
	p.close()

	if got := p.cached(); got != 0 {
		t.Errorf("cached = %d after close, want 0", got)
	}
	// Buffers returned after close are dropped and must not restart the
	// cleaner goroutine.
	p.Put(b2)
	if got := p.cached(); got != 0 {
		t.Errorf("cached = %d after Put on a closed pool, want 0", got)
	}
	if p.isCleanerRunning() {
		t.Error("cleaner restarted by Put after close")
	}
	// Get and TryGet still work.
	if _, ok, err := p.TryGet(4 * testBlockSize); !ok || err != nil {
		t.Errorf("TryGet after close = %v, %v; want true, nil", ok, err)
	}
	if _, err := p.Get(4 * testBlockSize); err != nil {
		t.Errorf("Get after close: %v", err)
	}
}

// Many writers assembling parts while the cleaner stops and restarts as often
// as possible. Once they finish, the cleaner must drain the pool; a block
// stranded without a running cleaner would keep it from emptying.
func TestDefaultBufferPoolConcurrency(t *testing.T) {
	const (
		workers = 32
		iters   = 500
	)
	p := newDefaultBufferPool()
	p.blockSize = testBlockSize
	p.cleanInterval = time.Millisecond
	defer p.close()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Mimic part assembly: one Get, then TryGet for the rest.
				first, err := p.Get(4 * testBlockSize)
				if err != nil {
					t.Error(err)
					return
				}
				held := [][]byte{first}
				for j := 0; j < 3; j++ {
					b, ok, err := p.TryGet(4 * testBlockSize)
					if !ok || err != nil {
						t.Errorf("TryGet = %v, %v; want true, nil", ok, err)
						return
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

	waitFor(t, "cleaner to drain the pool", func() bool {
		return p.cached() == 0 && !p.isCleanerRunning()
	})
}

// Steady-state Get/Put must not allocate. Benchmarks do not run in CI, so this
// asserts it deterministically.
func TestDefaultBufferPoolZeroAllocs(t *testing.T) {
	p := newTestBufferPool(t)
	b, _ := p.Get(testBlockSize)
	p.Put(b)
	allocs := testing.AllocsPerRun(1000, func() {
		b, _ := p.Get(testBlockSize)
		p.Put(b)
	})
	if allocs != 0 {
		t.Errorf("Get+Put allocs = %v, want 0", allocs)
	}
}

func BenchmarkDefaultBufferPoolSteadyState(b *testing.B) {
	p := newDefaultBufferPool()
	defer p.close()
	// A multi-block part: one Get followed by TryGet for the rest.
	const blocksPerPart = 4
	const partSize = blocksPerPart * poolBlockSize
	// Cache one part's blocks so the timed loop never grows the pool.
	cacheBlocks(b, p, blocksPerPart)

	held := make([][]byte, 0, blocksPerPart)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, _ := p.Get(partSize)
		held = append(held, buf)
		for len(held) < cap(held) {
			buf, _, _ := p.TryGet(partSize)
			held = append(held, buf)
		}
		for _, buf := range held {
			p.Put(buf)
		}
		held = held[:0]
	}
}

func BenchmarkDefaultBufferPoolParallel(b *testing.B) {
	p := newDefaultBufferPool()
	defer p.close()
	// Warm the pool with enough blocks for every parallel goroutine so the
	// timed loop measures steady state rather than pool growth.
	cacheBlocks(b, p, 2*runtime.GOMAXPROCS(0))
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf, _ := p.Get(poolBlockSize)
			p.Put(buf)
		}
	})
}

// fakePool is a minimal experimental.BufferPool used to check that a custom
// pool is passed through to the client.
type fakePool struct{}

func (*fakePool) Get(maxSize int) ([]byte, error)          { return make([]byte, maxSize), nil }
func (*fakePool) TryGet(maxSize int) ([]byte, bool, error) { return make([]byte, maxSize), true, nil }
func (*fakePool) Put([]byte)                               {}

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
		if !pool.isClosed() {
			t.Error("default pool not closed with the client")
		}
	})

	t.Run("custom", func(t *testing.T) {
		custom := &fakePool{}
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
