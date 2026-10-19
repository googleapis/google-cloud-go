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
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/option"
)

const testBlockSize = 1024

// newTestBufferPool returns a pool with small blocks, a fake clock, and a
// cleaner interval long enough that the test drives eviction manually.
func newTestBufferPool(t *testing.T, maxBlocks int) (*defaultBufferPool, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	p := newDefaultBufferPoolWithConfig(testBlockSize, maxBlocks, time.Hour, 30*time.Second)
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

func TestDefaultBufferPoolDefaults(t *testing.T) {
	p := newDefaultBufferPool()
	defer p.close()
	if p.blockSize != 8<<20 {
		t.Errorf("blockSize = %d, want %d", p.blockSize, 8<<20)
	}
	if p.cleanInterval != 15*time.Second {
		t.Errorf("cleanInterval = %v, want 15s", p.cleanInterval)
	}
	if p.idleTimeout != 30*time.Second {
		t.Errorf("idleTimeout = %v, want 30s", p.idleTimeout)
	}
}

func TestDefaultBufferPoolGetSizes(t *testing.T) {
	p, _ := newTestBufferPool(t, 4)
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
		if err != nil || !ok {
			t.Fatalf("TryGet(%d): ok=%v err=%v", tc.maxSize, ok, err)
		}
		if len(b2) != tc.wantLen {
			t.Errorf("TryGet(%d): len=%d, want %d", tc.maxSize, len(b2), tc.wantLen)
		}
		p.Put(b)
		p.Put(b2)
		waitIdle(t, p)
	}
}

func TestDefaultBufferPoolInvalidSize(t *testing.T) {
	p, _ := newTestBufferPool(t, 4)
	for _, n := range []int{0, -1} {
		if _, err := p.Get(n); !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("Get(%d) err = %v, want %v", n, err, errInvalidBufferSize)
		}
		if _, ok, err := p.TryGet(n); ok || !errors.Is(err, errInvalidBufferSize) {
			t.Errorf("TryGet(%d) = %v, %v; want false, %v", n, ok, err, errInvalidBufferSize)
		}
	}
}

// Get must always succeed, even when the pool is exhausted, so that the first
// segment of every upload makes progress. TryGet must report exhaustion
// without an error.
func TestDefaultBufferPoolExhaustion(t *testing.T) {
	p, _ := newTestBufferPool(t, 2)
	var held [][]byte
	for i := 0; i < 2; i++ {
		b, ok, err := p.TryGet(testBlockSize)
		if err != nil || !ok {
			t.Fatalf("TryGet #%d: ok=%v err=%v", i, ok, err)
		}
		held = append(held, b)
	}
	waitIdle(t, p)

	if b, ok, err := p.TryGet(testBlockSize); ok || err != nil || b != nil {
		t.Fatalf("TryGet on exhausted pool = %v, %v, %v; want nil, false, nil", b, ok, err)
	}
	b, err := p.Get(testBlockSize)
	if err != nil || len(b) != testBlockSize {
		t.Fatalf("Get on exhausted pool = len %d, %v; want len %d, nil", len(b), err, testBlockSize)
	}
	held = append(held, b)
	if got := p.live.Load(); got != 3 {
		t.Errorf("live = %d, want 3", got)
	}

	// Returning everything caches at most maxBlocks; the overflow is dropped.
	for _, b := range held {
		p.Put(b)
	}
	if got, want := len(p.free), 2; got != want {
		t.Errorf("cached = %d, want %d", got, want)
	}
	if got := p.live.Load(); got != 2 {
		t.Errorf("live = %d, want 2", got)
	}
	// After Put, TryGet succeeds again.
	if _, ok, err := p.TryGet(testBlockSize); !ok || err != nil {
		t.Errorf("TryGet after Put = %v, %v; want true, nil", ok, err)
	}
}

func TestDefaultBufferPoolNonBlocking(t *testing.T) {
	p, _ := newTestBufferPool(t, 1)
	if _, err := p.Get(testBlockSize); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			p.TryGet(testBlockSize)
			p.Get(testBlockSize)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Get/TryGet blocked on an exhausted pool")
	}
}

func TestDefaultBufferPoolReuse(t *testing.T) {
	p, _ := newTestBufferPool(t, 1)
	b, err := p.Get(testBlockSize / 2)
	if err != nil {
		t.Fatal(err)
	}
	waitIdle(t, p)
	// Put accepts the exact (possibly short) slice returned by Get.
	p.Put(b)
	b2, err := p.Get(testBlockSize)
	if err != nil {
		t.Fatal(err)
	}
	if &b[0] != &b2[0] {
		t.Error("Get after Put did not reuse the cached block")
	}
}

func TestDefaultBufferPoolPutForeignBuffer(t *testing.T) {
	p, _ := newTestBufferPool(t, 2)
	p.Put(make([]byte, testBlockSize+1))
	p.Put(make([]byte, 10))
	p.Put(nil)
	if len(p.free) != 0 || p.live.Load() != 0 {
		t.Errorf("foreign buffers were cached: free=%d live=%d", len(p.free), p.live.Load())
	}
}

func TestDefaultBufferPoolPreallocation(t *testing.T) {
	p, _ := newTestBufferPool(t, 8)
	// A request for a 3-block part should trigger background allocation of
	// enough spare blocks for the rest of the part.
	if _, err := p.Get(3 * testBlockSize); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pre-allocation", func() bool { return len(p.free) >= 3 })
	waitIdle(t, p)
	if got := p.live.Load(); got != 4 {
		t.Errorf("live = %d, want 4", got)
	}

	// Pre-allocation never grows the pool beyond maxBlocks.
	for i := 0; i < 8; i++ {
		p.Get(8 * testBlockSize)
	}
	waitIdle(t, p)
	if got := len(p.free); got != 0 {
		t.Errorf("cached = %d after over-limit Gets, want 0", got)
	}
}

func TestDefaultBufferPoolStagedEviction(t *testing.T) {
	p, clk := newTestBufferPool(t, 16)
	var bufs [][]byte
	for i := 0; i < 8; i++ {
		b, ok, _ := p.TryGet(1)
		if !ok {
			t.Fatalf("TryGet #%d failed", i)
		}
		bufs = append(bufs, b)
	}
	waitIdle(t, p)
	for _, b := range bufs {
		p.Put(b)
	}
	n := len(p.free)
	if n < 8 {
		t.Fatalf("cached = %d, want >= 8", n)
	}

	steps := []struct {
		advance  time.Duration
		wantFree int
		wantStop bool
		desc     string
	}{
		{15 * time.Second, n, false, "idle 15s: no eviction"},
		{15 * time.Second, n - (n - n/2), false, "idle 30s: halve"},
		{15 * time.Second, n / 2, false, "15s after eviction: no eviction"},
		{15 * time.Second, n / 4, false, "30s after eviction: halve again"},
	}
	for _, s := range steps {
		clk.Advance(s.advance)
		stopped := p.evictIfIdle()
		if got := len(p.free); got != s.wantFree || stopped != s.wantStop {
			t.Fatalf("%s: free=%d stopped=%v, want free=%d stopped=%v", s.desc, got, stopped, s.wantFree, s.wantStop)
		}
	}

	// Activity postpones eviction.
	b, _ := p.Get(1)
	p.Put(b)
	before := len(p.free)
	clk.Advance(15 * time.Second)
	if p.evictIfIdle(); len(p.free) != before {
		t.Fatalf("evicted %d blocks 15s after activity", before-len(p.free))
	}

	// Keep halving until empty; the cleaner then reports it has stopped.
	for i := 0; i < 10 && len(p.free) > 0; i++ {
		clk.Advance(30 * time.Second)
		if stopped := p.evictIfIdle(); stopped != (len(p.free) == 0) {
			t.Fatalf("stopped=%v with %d cached", stopped, len(p.free))
		}
	}
	if len(p.free) != 0 || p.live.Load() != 0 {
		t.Fatalf("pool not drained: free=%d live=%d", len(p.free), p.live.Load())
	}
}

func TestDefaultBufferPoolCleanerLifecycle(t *testing.T) {
	p := newDefaultBufferPoolWithConfig(testBlockSize, 4, time.Millisecond, 50*time.Millisecond)
	defer p.close()

	running := func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.cleanerRunning
	}
	if running() {
		t.Fatal("cleaner running before any block was cached")
	}
	b, _ := p.Get(testBlockSize)
	p.Put(b)
	if !running() {
		t.Fatal("cleaner not started after Put")
	}
	waitFor(t, "pool to drain", func() bool { return len(p.free) == 0 && !p.refilling.Load() })
	waitFor(t, "cleaner to stop", func() bool { return !running() })

	// The next Put restarts the cleaner.
	b, _ = p.Get(testBlockSize)
	p.Put(b)
	if !running() {
		t.Fatal("cleaner not restarted after Put")
	}
}

func TestDefaultBufferPoolClose(t *testing.T) {
	p := newDefaultBufferPoolWithConfig(testBlockSize, 4, time.Hour, time.Hour)
	b1, _ := p.Get(testBlockSize)
	b2, _ := p.Get(testBlockSize)
	p.Put(b1)
	p.close()
	p.close() // idempotent

	if len(p.free) != 0 {
		t.Errorf("cached = %d after close, want 0", len(p.free))
	}
	// Buffers returned after close are dropped; Get still works.
	p.Put(b2)
	if len(p.free) != 0 || p.live.Load() != 0 {
		t.Errorf("after close: free=%d live=%d, want 0, 0", len(p.free), p.live.Load())
	}
	if _, err := p.Get(testBlockSize); err != nil {
		t.Errorf("Get after close: %v", err)
	}
}

func TestDefaultBufferPoolConcurrency(t *testing.T) {
	const (
		workers   = 32
		iters     = 500
		maxBlocks = 16
	)
	p := newDefaultBufferPoolWithConfig(testBlockSize, maxBlocks, time.Millisecond, 2*time.Millisecond)
	defer p.close()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Mimic part assembly: one guaranteed Get, then TryGet until
				// the part is full or the pool is exhausted.
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
	if live, free := p.live.Load(), int64(len(p.free)); live != free {
		t.Errorf("leaked blocks: live=%d cached=%d", live, free)
	}
	if free := len(p.free); free > maxBlocks {
		t.Errorf("cached = %d, exceeds maxBlocks %d", free, maxBlocks)
	}
}

func BenchmarkDefaultBufferPoolSteadyState(b *testing.B) {
	p := newDefaultBufferPool()
	defer p.close()
	const partSize = 32 << 20
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
		if c.defaultPool == nil {
			t.Fatal("defaultPool is nil")
		}
		if c.bufferPool != experimental.BufferPool(c.defaultPool) {
			t.Error("bufferPool is not the default pool")
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		if !c.defaultPool.shut.Load() {
			t.Error("default pool not closed with the client")
		}
	})

	t.Run("custom", func(t *testing.T) {
		custom := &struct{ experimental.BufferPool }{}
		opts := append(baseOpts, experimental.WithBufferPool(custom))
		c, err := newGRPCStorageClient(ctx, withClientOptions(opts...))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if c.defaultPool != nil {
			t.Error("defaultPool created even though a custom pool was provided")
		}
		if c.bufferPool != experimental.BufferPool(custom) {
			t.Error("bufferPool is not the custom pool")
		}
	})
}
