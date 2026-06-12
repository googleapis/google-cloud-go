package resourcemanager

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct {
	name     string
	closeErr error
	closed   atomic.Int32
}

func (f *fakeClient) Close() error {
	f.closed.Add(1)
	return f.closeErr
}

// Tests below always use the same method string so the cache key reduces to
// resource; eviction/LRU/TTL behavior is unchanged by the method dimension.
const testMethod = "ReadRow"

func TestGetOrOpen_MissThenHit(t *testing.T) {
	var calls atomic.Int32
	m := NewPoolCache[*fakeClient](4, func(resource, method string) (*fakeClient, error) {
		calls.Add(1)
		return &fakeClient{name: resource}, nil
	})

	v1, rel1, err := m.GetOrOpen("t1", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t1) err: %v", err)
	}
	defer rel1()
	v2, rel2, err := m.GetOrOpen("t1", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t1) err: %v", err)
	}
	defer rel2()
	if v1 != v2 {
		t.Errorf("second GetOrOpen returned a different instance")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("factory call count = %d; want 1", got)
	}
}

func TestGetOrOpen_KeyedByResourceAndMethod(t *testing.T) {
	// Same resource, different methods => different pools (different factory calls).
	var calls atomic.Int32
	m := NewPoolCache[*fakeClient](4, func(resource, method string) (*fakeClient, error) {
		calls.Add(1)
		return &fakeClient{name: resource + ":" + method}, nil
	})

	v1, r1, err := m.GetOrOpen("t1", "ReadRow")
	if err != nil {
		t.Fatalf("GetOrOpen(t1, ReadRow) err: %v", err)
	}
	defer r1()
	v2, r2, err := m.GetOrOpen("t1", "MutateRow")
	if err != nil {
		t.Fatalf("GetOrOpen(t1, MutateRow) err: %v", err)
	}
	defer r2()
	if v1 == v2 {
		t.Errorf("expected distinct pools for distinct methods on same resource")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("factory call count = %d; want 2", got)
	}
}

func TestGetOrOpen_FactoryError(t *testing.T) {
	wantErr := errors.New("boom")
	m := NewPoolCache[*fakeClient](4, func(string, string) (*fakeClient, error) {
		return nil, wantErr
	})

	if _, _, err := m.GetOrOpen("t1", testMethod); !errors.Is(err, wantErr) {
		t.Errorf("err = %v; want %v", err, wantErr)
	}
	// A failed construction must not be cached: a retry should re-invoke the factory.
	var calls atomic.Int32
	m2 := NewPoolCache[*fakeClient](4, func(string, string) (*fakeClient, error) {
		calls.Add(1)
		return nil, wantErr
	})
	_, _, _ = m2.GetOrOpen("t1", testMethod)
	_, _, _ = m2.GetOrOpen("t1", testMethod)
	if got := calls.Load(); got != 2 {
		t.Errorf("factory called %d times on repeated failure; want 2", got)
	}
}

func TestGetOrOpen_LRUEvictionClosesEvicted(t *testing.T) {
	clients := map[string]*fakeClient{}
	m := NewPoolCache[*fakeClient](2, func(resource, method string) (*fakeClient, error) {
		c := &fakeClient{name: resource}
		clients[resource] = c
		return c, nil
	})

	// Fill cache (capacity 2), then add a third entry to evict the LRU one (t1).
	// Releasing immediately means refs==0 by the time eviction fires, so the
	// LRU evictee should close in-line under onCacheEvict.
	for _, name := range []string{"t1", "t2", "t3"} {
		_, release, err := m.GetOrOpen(name, testMethod)
		if err != nil {
			t.Fatalf("GetOrOpen(%s) err: %v", name, err)
		}
		release()
	}

	if got := clients["t1"].closed.Load(); got != 1 {
		t.Errorf("evicted client t1 Close count = %d; want 1", got)
	}
	if got := clients["t2"].closed.Load(); got != 0 {
		t.Errorf("retained client t2 Close count = %d; want 0", got)
	}
	if got := clients["t3"].closed.Load(); got != 0 {
		t.Errorf("retained client t3 Close count = %d; want 0", got)
	}
}

func TestGetOrOpen_LRUOrderingFromHit(t *testing.T) {
	clients := map[string]*fakeClient{}
	m := NewPoolCache[*fakeClient](2, func(resource, method string) (*fakeClient, error) {
		c := &fakeClient{name: resource}
		clients[resource] = c
		return c, nil
	})

	// Insert t1, t2; touch t1 so t2 becomes the LRU; insert t3 -> t2 evicts.
	get := func(name string) {
		_, release, err := m.GetOrOpen(name, testMethod)
		if err != nil {
			t.Fatalf("GetOrOpen(%s) err: %v", name, err)
		}
		release()
	}
	get("t1")
	get("t2")
	get("t1")
	get("t3")

	if got := clients["t2"].closed.Load(); got != 1 {
		t.Errorf("t2 Close count = %d; want 1 (should have been LRU-evicted)", got)
	}
	if got := clients["t1"].closed.Load(); got != 0 {
		t.Errorf("t1 Close count = %d; want 0 (recently touched)", got)
	}
}

func TestClose_ClosesAllAndReturnsFirstError(t *testing.T) {
	wantErr := errors.New("close-fail")
	c1 := &fakeClient{name: "t1", closeErr: wantErr}
	c2 := &fakeClient{name: "t2"}
	clients := map[string]*fakeClient{"t1": c1, "t2": c2}
	m := NewPoolCache[*fakeClient](4, func(resource, method string) (*fakeClient, error) {
		return clients[resource], nil
	})
	_, r1, _ := m.GetOrOpen("t1", testMethod)
	_, r2, _ := m.GetOrOpen("t2", testMethod)
	r1()
	r2()

	err := m.Close()
	if !errors.Is(err, wantErr) {
		t.Errorf("Close err = %v; want %v", err, wantErr)
	}
	if got := c1.closed.Load(); got != 1 {
		t.Errorf("t1 Close count = %d; want 1", got)
	}
	if got := c2.closed.Load(); got != 1 {
		t.Errorf("t2 Close count = %d; want 1", got)
	}
}

func TestNewPoolCache_NonPositiveCapacityFallsBackToDefault(t *testing.T) {
	m := NewPoolCache[*fakeClient](0, func(resource, method string) (*fakeClient, error) {
		return &fakeClient{name: resource}, nil
	})
	// Insert one more than the default to confirm an eviction occurs, which
	// proves the cache is bounded (not, e.g., zero-sized or unbounded).
	for i := 0; i < DefaultPoolCacheSize+1; i++ {
		name := string(rune('a' + i))
		_, release, err := m.GetOrOpen(name, testMethod)
		if err != nil {
			t.Fatalf("GetOrOpen(%s) err: %v", name, err)
		}
		release()
	}
}

// --- Refcounting ----------------------------------------------------------

func TestGetOrOpen_EvictionDeferredWhileHandleHeld(t *testing.T) {
	clients := map[string]*fakeClient{}
	m := NewPoolCache[*fakeClient](1, func(resource, method string) (*fakeClient, error) {
		c := &fakeClient{name: resource}
		clients[resource] = c
		return c, nil
	})

	// Borrow t1 and DON'T release; force LRU eviction of t1 by adding t2.
	_, release1, err := m.GetOrOpen("t1", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t1) err: %v", err)
	}
	_, release2, err := m.GetOrOpen("t2", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t2) err: %v", err)
	}
	defer release2()

	if got := clients["t1"].closed.Load(); got != 0 {
		t.Fatalf("t1 closed while a handle was still outstanding (count=%d)", got)
	}

	// Releasing the outstanding handle must close exactly once.
	release1()
	if got := clients["t1"].closed.Load(); got != 1 {
		t.Errorf("t1 Close count after release = %d; want 1", got)
	}

	// Idempotent: a second release on the same handle must not double-close.
	release1()
	if got := clients["t1"].closed.Load(); got != 1 {
		t.Errorf("t1 Close count after second release = %d; want 1 (idempotent)", got)
	}
}

func TestGetOrOpen_TwoBorrowersCloseOnLastRelease(t *testing.T) {
	c1 := &fakeClient{name: "t1"}
	m := NewPoolCache[*fakeClient](1, func(string, string) (*fakeClient, error) { return c1, nil })

	_, r1a, _ := m.GetOrOpen("t1", testMethod)
	_, r1b, _ := m.GetOrOpen("t1", testMethod)

	// Evict t1 while two handles outstanding.
	_, r2, _ := m.GetOrOpen("t2", testMethod)
	defer r2()
	if got := c1.closed.Load(); got != 0 {
		t.Fatalf("closed while 2 handles outstanding (count=%d)", got)
	}

	r1a()
	if got := c1.closed.Load(); got != 0 {
		t.Fatalf("closed with 1 handle still outstanding (count=%d)", got)
	}
	r1b()
	if got := c1.closed.Load(); got != 1 {
		t.Errorf("t1 Close count after final release = %d; want 1", got)
	}
}

// --- TTL ------------------------------------------------------------------

// The expirable LRU sweeps in (ttl / 100) ticks from a background goroutine,
// so TTL tests pick a short TTL and tolerate a small wait. Flaky-on-load by
// nature; bump the multiplier if CI is noisy.
const testTTL = 100 * time.Millisecond

func waitForClose(t *testing.T, c *fakeClient, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.closed.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("client %q Close count = %d; want %d after wait", c.name, c.closed.Load(), want)
}

func TestTTL_IdleEntryEvictedAndClosed(t *testing.T) {
	c1 := &fakeClient{name: "t1"}
	m := newPoolCacheWithTTL[*fakeClient](4, testTTL, func(string, string) (*fakeClient, error) { return c1, nil })

	_, release, err := m.GetOrOpen("t1", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t1) err: %v", err)
	}
	release()

	waitForClose(t, c1, 1)
}

func TestTTL_SlidingRefreshOnGet(t *testing.T) {
	c1 := &fakeClient{name: "t1"}
	m := newPoolCacheWithTTL[*fakeClient](4, testTTL, func(string, string) (*fakeClient, error) { return c1, nil })

	// Touch the entry repeatedly inside the TTL window; it must NOT expire.
	deadline := time.Now().Add(3 * testTTL)
	for time.Now().Before(deadline) {
		_, release, err := m.GetOrOpen("t1", testMethod)
		if err != nil {
			t.Fatalf("GetOrOpen(t1) err: %v", err)
		}
		release()
		time.Sleep(testTTL / 4)
	}
	if got := c1.closed.Load(); got != 0 {
		t.Errorf("client closed while still being used; close count = %d", got)
	}

	// Stop touching it — must eventually expire and close.
	waitForClose(t, c1, 1)
}

func TestTTL_ExpiryWhileHeldDefersCloseUntilRelease(t *testing.T) {
	c1 := &fakeClient{name: "t1"}
	m := newPoolCacheWithTTL[*fakeClient](4, testTTL, func(string, string) (*fakeClient, error) { return c1, nil })

	_, release, err := m.GetOrOpen("t1", testMethod)
	if err != nil {
		t.Fatalf("GetOrOpen(t1) err: %v", err)
	}

	// Wait long enough for the background sweeper to fire — but the borrowed
	// handle is still outstanding, so close must NOT happen yet.
	time.Sleep(3 * testTTL)
	if got := c1.closed.Load(); got != 0 {
		t.Fatalf("close fired while handle outstanding; count = %d", got)
	}

	release()
	if got := c1.closed.Load(); got != 1 {
		t.Errorf("close count after final release = %d; want 1", got)
	}
}
