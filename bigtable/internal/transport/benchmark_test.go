package internal

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
)

// Simplified connEntry for benchmark
type connEntrys struct {
	id int
	// Add other fields to mimic size if necessary
}

const poolSize = 100

// --- atomic.Value Implementation ---

type AtomicPool struct {
	conns atomic.Value // Stores []*connEntrys
}

func NewAtomicPool() *AtomicPool {
	p := &AtomicPool{}
	initialConns := make([]*connEntrys, poolSize)
	for i := range initialConns {
		initialConns[i] = &connEntrys{id: i}
	}
	p.conns.Store(initialConns)
	return p
}

func (p *AtomicPool) GetConns() []*connEntrys {
	return p.conns.Load().([]*connEntrys)
}

func (p *AtomicPool) ReplaceConnection(idx int, newEntry *connEntrys) {
	// Copy-on-write
	oldConns := p.GetConns()
	newConns := make([]*connEntrys, len(oldConns))
	copy(newConns, oldConns)
	newConns[idx] = newEntry
	p.conns.Store(newConns)
}

// --- sync.Map Implementation ---

type SyncMapPool struct {
	conns sync.Map // Stores connEntry with key int
}

func NewSyncMapPool() *SyncMapPool {
	p := &SyncMapPool{}
	for i := 0; i < poolSize; i++ {
		p.conns.Store(i, &connEntrys{id: i})
	}
	return p
}

func (p *SyncMapPool) GetConns() []*connEntrys {
	conns := make([]*connEntrys, 0, poolSize)
	p.conns.Range(func(key, value interface{}) bool {
		conns = append(conns, value.(*connEntrys))
		return true
	})
	return conns
}

func (p *SyncMapPool) ReplaceConnection(idx int, newEntry *connEntrys) {
	p.conns.Store(idx, newEntry)
}

// --- Benchmarks ---

// BenchmarkAtomicPool_GetConns benchmarks reading the entire connection list.
func BenchmarkAtomicPool_GetConns(b *testing.B) {
	pool := NewAtomicPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conns := pool.GetConns()
			if len(conns) != poolSize {
				b.Fatalf("Unexpected pool size: %d", len(conns))
			}
		}
	})
}

// BenchmarkSyncMapPool_GetConns benchmarks fetching all connections from sync.Map.
func BenchmarkSyncMapPool_GetConns(b *testing.B) {
	pool := NewSyncMapPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conns := pool.GetConns()
			if len(conns) != poolSize {
				b.Fatalf("Unexpected pool size: %d", len(conns))
			}
		}
	})
}

// BenchmarkAtomicPool_ReplaceConnection benchmarks updating a connection.
func BenchmarkAtomicPool_ReplaceConnection(b *testing.B) {
	pool := NewAtomicPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			idx := r.Intn(poolSize)
			newEntry := &connEntrys{id: idx + poolSize}
			pool.ReplaceConnection(idx, newEntry)
		}
	})
}

// BenchmarkSyncMapPool_ReplaceConnection benchmarks updating a connection in sync.Map.
func BenchmarkSyncMapPool_ReplaceConnection(b *testing.B) {
	pool := NewSyncMapPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			idx := r.Intn(poolSize)
			newEntry := &connEntrys{id: idx + poolSize}
			pool.ReplaceConnection(idx, newEntry)
		}
	})
}

// BenchmarkAtomicPool_Mixed benchmarks a mix of 80% reads and 20% writes.
func BenchmarkAtomicPool_Mixed(b *testing.B) {
	pool := NewAtomicPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if r.Float32() < 0.8 {
				// Read
				conns := pool.GetConns()
				if len(conns) != poolSize {
					b.Fatalf("Unexpected pool size: %d", len(conns))
				}
			} else {
				// Write
				idx := r.Intn(poolSize)
				newEntry := &connEntrys{id: idx + poolSize}
				pool.ReplaceConnection(idx, newEntry)
			}
		}
	})
}

// BenchmarkSyncMapPool_Mixed benchmarks a mix of 80% reads and 20% writes for sync.Map.
func BenchmarkSyncMapPool_Mixed(b *testing.B) {
	pool := NewSyncMapPool()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if r.Float32() < 0.8 {
				// Read
				_ = pool.GetConns()
			} else {
				// Write
				idx := r.Intn(poolSize)
				newEntry := &connEntrys{id: idx + poolSize}
				pool.ReplaceConnection(idx, newEntry)
			}
		}
	})
}
