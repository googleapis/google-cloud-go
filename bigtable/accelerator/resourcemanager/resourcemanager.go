package resourcemanager

import (
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// DefaultPoolCacheSize is the default capacity used when a non-positive
// capacity is supplied to NewPoolCache.
const DefaultPoolCacheSize = 16

// DefaultPoolCacheTTL is the per-entry idle expiry. Entries that have not
// been fetched via GetOrOpen within this window are evicted; each GetOrOpen
// refreshes the entry's TTL (sliding expiry).
const DefaultPoolCacheTTL = 5 * time.Minute

// PoolFactory constructs a pool for a (resource, method) pair on cache miss.
// The accelerator calls this in PoolCache.GetOrOpen.
type PoolFactory[V io.Closer] func(resource, method string) (V, error)

// PoolKey is the composite cache key: one entry per (resource, method) pair.
// Exported so callers can reason about cache identity if needed.
type PoolKey struct {
	Resource string
	Method   string
}

// entry wraps a cached pool with the bookkeeping needed for safe concurrent
// borrow/release across LRU and TTL eviction. Refs counts outstanding handles
// returned by GetOrOpen; evicted flips true once the cache has detached the
// entry (LRU pressure, TTL expiry, or Close). The underlying pool's Close
// runs exactly once, fired by whichever observer (release or eviction) last
// sees refs==0 && evicted==true.
type entry[V io.Closer] struct {
	key     PoolKey
	pool    V
	refs    atomic.Int32
	evicted atomic.Bool
	closer  sync.Once
}

func (e *entry[V]) close() error {
	var err error
	e.closer.Do(func() {
		err = e.pool.Close()
	})
	return err
}

func (e *entry[V]) maybeClose() {
	if e.evicted.Load() && e.refs.Load() == 0 {
		if err := e.close(); err != nil {
			log.Printf("resourcemanager: Close on evicted pool %+v: %v", e.key, err)
		}
	}
}

// PoolCache caches one pool per (resource, method) pair, bounded by an LRU
// and a per-entry idle TTL. Evicted pools are closed once their last
// outstanding borrowed handle is released.
type PoolCache[V io.Closer] struct {
	factory PoolFactory[V]
	mu      sync.Mutex
	cache   *expirable.LRU[PoolKey, *entry[V]]
}

// NewPoolCache returns a PoolCache that constructs pools on miss via factory,
// caches up to capacity entries, and expires idle entries after
// DefaultPoolCacheTTL. A capacity <= 0 falls back to DefaultPoolCacheSize.
func NewPoolCache[V io.Closer](capacity int, factory PoolFactory[V]) *PoolCache[V] {
	return newPoolCacheWithTTL(capacity, DefaultPoolCacheTTL, factory)
}

// newPoolCacheWithTTL is the constructor backing NewPoolCache; tests use it
// directly with a short TTL.
func newPoolCacheWithTTL[V io.Closer](capacity int, ttl time.Duration, factory PoolFactory[V]) *PoolCache[V] {
	if capacity <= 0 {
		capacity = DefaultPoolCacheSize
	}
	m := &PoolCache[V]{factory: factory}
	m.cache = expirable.NewLRU[PoolKey, *entry[V]](capacity, onCacheEvict[V], ttl)
	return m
}

// onCacheEvict fires for both LRU and TTL eviction. It runs under the cache's
// internal lock (and, for LRU pressure triggered by GetOrOpen's Add, also
// under m.mu). It must not touch the cache. Marks the entry detached and
// closes the underlying pool iff no borrower remains.
func onCacheEvict[V io.Closer](_ PoolKey, e *entry[V]) {
	e.evicted.Store(true)
	e.maybeClose()
}

// GetOrOpen returns the cached pool for (resource, method), constructing one
// via the factory on miss. The returned release MUST be called when the
// caller is finished with the pool; on the last release after eviction the
// pool's Close is invoked. release is idempotent.
//
// Each call refreshes the entry's idle TTL.
//
// Construction is serialized under m.mu, so the factory is invoked at most
// once per concurrent miss for the same key — fine while pools are cheap to
// build.
func (m *PoolCache[V]) GetOrOpen(resource, method string) (V, func(), error) {
	var zero V
	k := PoolKey{Resource: resource, Method: method}

	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.cache.Get(k); ok {
		// Re-Add resets ExpiresAt without firing onCacheEvict (Add on an
		// existing key updates in place; see expirable.LRU.Add).
		m.cache.Add(k, e)
		e.refs.Add(1)
		return e.pool, m.releaseFn(e), nil
	}

	v, err := m.factory(resource, method)
	if err != nil {
		return zero, nil, err
	}
	e := &entry[V]{key: k, pool: v}
	e.refs.Store(1)
	m.cache.Add(k, e) // may trigger LRU eviction of another entry
	return e.pool, m.releaseFn(e), nil
}

func (m *PoolCache[V]) releaseFn(e *entry[V]) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			e.refs.Add(-1)
			e.maybeClose()
		})
	}
}

// Close synchronously closes every cached pool and returns the first error
// encountered. Callers should ensure no borrowed handles are still in use
// (e.g., the gRPC server has drained). Outstanding handles will not
// double-close: their release becomes a no-op for the underlying pool thanks
// to entry.closer.
func (m *PoolCache[V]) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for _, k := range m.cache.Keys() {
		e, ok := m.cache.Peek(k)
		if !ok {
			continue
		}
		e.evicted.Store(true)
		if err := e.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
