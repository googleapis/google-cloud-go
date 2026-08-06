/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type lifecycleTestEndpoint struct {
	address string
	conn    *grpc.ClientConn
	active  atomic.Int64
}

func (e *lifecycleTestEndpoint) Address() string {
	return e.address
}

func (e *lifecycleTestEndpoint) IsHealthy() bool {
	if e.conn == nil {
		return false
	}
	return e.conn.GetState() == connectivity.Ready
}

func (e *lifecycleTestEndpoint) IsTransientFailure() bool {
	if e.conn == nil {
		return false
	}
	return e.conn.GetState() == connectivity.TransientFailure
}

func (e *lifecycleTestEndpoint) GetConn() *grpc.ClientConn {
	return e.conn
}

func (e *lifecycleTestEndpoint) IncrementActiveRequests() {
	e.active.Add(1)
}

func (e *lifecycleTestEndpoint) DecrementActiveRequests() {
	e.active.Add(-1)
}

func (e *lifecycleTestEndpoint) ActiveRequestCount() int {
	return int(e.active.Load())
}

type lifecycleTestCache struct {
	mu              sync.Mutex
	defaultEndpoint channelEndpoint
	endpoints       map[string]channelEndpoint
	getCalls        map[string]int
	evicted         map[string]int
	factory         func(ctx context.Context, address string) channelEndpoint
}

func newLifecycleTestCache(defaultAddress string, factory func(ctx context.Context, address string) channelEndpoint) *lifecycleTestCache {
	return &lifecycleTestCache{
		defaultEndpoint: &passthroughChannelEndpoint{address: defaultAddress},
		endpoints:       make(map[string]channelEndpoint),
		getCalls:        make(map[string]int),
		evicted:         make(map[string]int),
		factory:         factory,
	}
}

func (c *lifecycleTestCache) Get(ctx context.Context, address string) channelEndpoint {
	if address == c.defaultEndpoint.Address() {
		return c.defaultEndpoint
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if endpoint, ok := c.endpoints[address]; ok {
		return endpoint
	}
	c.getCalls[address]++
	if c.factory == nil {
		return nil
	}
	endpoint := c.factory(ctx, address)
	if endpoint != nil {
		c.endpoints[address] = endpoint
	}
	return endpoint
}

func (c *lifecycleTestCache) GetIfPresent(address string) channelEndpoint {
	if address == c.defaultEndpoint.Address() {
		return c.defaultEndpoint
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	endpoint, ok := c.endpoints[address]
	if !ok {
		return nil
	}
	return endpoint
}

func (c *lifecycleTestCache) Evict(address string) {
	if address == c.defaultEndpoint.Address() {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.endpoints, address)
	c.evicted[address]++
}

func (c *lifecycleTestCache) DefaultChannel() channelEndpoint       { return c.defaultEndpoint }
func (*lifecycleTestCache) ClientFor(channelEndpoint) spannerClient { return nil }
func (*lifecycleTestCache) Close() error                            { return nil }

func (c *lifecycleTestCache) getCallCount(address string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getCalls[address]
}

func (c *lifecycleTestCache) evictionCount(address string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evicted[address]
}

type lifecycleTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newLifecycleTestClock(now time.Time) *lifecycleTestClock {
	return &lifecycleTestClock{now: now}
}

func (c *lifecycleTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *lifecycleTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestEndpointLifecycleManager_RecordRealTrafficCreatesManagedEndpoint(t *testing.T) {
	conn, cleanup := newReadyTestConn(t)
	defer cleanup()

	cache := newLifecycleTestCache("default:443", func(_ context.Context, address string) channelEndpoint {
		return &lifecycleTestEndpoint{address: address, conn: conn}
	})
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		10*time.Millisecond,
		time.Hour,
		time.Now,
	)
	defer manager.shutdown()

	manager.recordRealTraffic("replica-1:443")

	waitForCondition(t, time.Second, func() bool {
		return cache.GetIfPresent("replica-1:443") != nil && manager.isManaged("replica-1:443")
	})

	state, ok := manager.getEndpointState("replica-1:443")
	if !ok {
		t.Fatal("expected managed endpoint state")
	}
	if state.lastRealTrafficAt.IsZero() {
		t.Fatal("expected lastRealTrafficAt to be recorded")
	}
}

func TestEndpointLifecycleManager_RecordRealTrafficSkipsDefaultEndpoint(t *testing.T) {
	cache := newLifecycleTestCache("default:443", nil)
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		time.Hour,
		time.Hour,
		time.Now,
	)
	defer manager.shutdown()

	manager.recordRealTraffic("default:443")

	if manager.managedEndpointCount() != 0 {
		t.Fatal("expected default endpoint not to be lifecycle-managed")
	}
}

func TestEndpointLifecycleManager_RequestEndpointRecreationCreatesEndpoint(t *testing.T) {
	conn, cleanup := newReadyTestConn(t)
	defer cleanup()

	cache := newLifecycleTestCache("default:443", func(_ context.Context, address string) channelEndpoint {
		return &lifecycleTestEndpoint{address: address, conn: conn}
	})
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		time.Hour,
		time.Hour,
		time.Now,
	)
	defer manager.shutdown()

	manager.requestEndpointRecreation("replica-2:443")

	waitForCondition(t, time.Second, func() bool {
		return cache.getCallCount("replica-2:443") > 0 &&
			cache.GetIfPresent("replica-2:443") != nil &&
			manager.isManaged("replica-2:443")
	})
}

func TestEndpointLifecycleManager_ProbeEvictsTransientFailureEndpoint(t *testing.T) {
	conn, cleanup := newTransientFailureTestConn(t)
	defer cleanup()

	cache := newLifecycleTestCache("default:443", func(_ context.Context, address string) channelEndpoint {
		return &lifecycleTestEndpoint{address: address, conn: conn}
	})
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		10*time.Millisecond,
		time.Hour,
		time.Now,
	)
	defer manager.shutdown()

	manager.recordRealTraffic("replica-3:443")

	waitForCondition(t, time.Second, func() bool {
		return cache.GetIfPresent("replica-3:443") != nil
	})

	waitForCondition(t, time.Second, func() bool {
		return !manager.isManaged("replica-3:443") &&
			cache.evictionCount("replica-3:443") > 0 &&
			manager.wasRecentlyEvictedTransientFailure("replica-3:443")
	})
}

func TestEndpointLifecycleManager_CheckIdleEviction(t *testing.T) {
	conn, cleanup := newReadyTestConn(t)
	defer cleanup()

	clock := newLifecycleTestClock(time.Unix(100, 0))
	cache := newLifecycleTestCache("default:443", func(_ context.Context, address string) channelEndpoint {
		return &lifecycleTestEndpoint{address: address, conn: conn}
	})
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		time.Hour,
		time.Minute,
		clock.Now,
	)
	defer manager.shutdown()

	manager.recordRealTraffic("replica-4:443")
	waitForCondition(t, time.Second, func() bool {
		return manager.isManaged("replica-4:443")
	})

	clock.Advance(2 * time.Minute)
	manager.checkIdleEviction()

	if manager.isManaged("replica-4:443") {
		t.Fatal("expected idle endpoint to be evicted")
	}
	if cache.evictionCount("replica-4:443") == 0 {
		t.Fatal("expected endpoint cache eviction to be recorded")
	}
}

func TestEndpointLifecycleManager_ShutdownIsIdempotent(t *testing.T) {
	cache := newLifecycleTestCache("default:443", nil)
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		time.Hour,
		time.Hour,
		time.Now,
	)

	manager.shutdown()
	manager.shutdown()
}

func TestEndpointLifecycleManager_ShutdownCancelsPendingCreation(t *testing.T) {
	started := make(chan struct{})
	cache := newLifecycleTestCache("default:443", func(ctx context.Context, address string) channelEndpoint {
		close(started)
		<-ctx.Done()
		return nil
	})
	manager := newEndpointLifecycleManagerWithOptions(
		cache,
		time.Hour,
		time.Hour,
		time.Now,
	)

	manager.requestEndpointRecreation("replica-5:443")

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for endpoint creation to start")
	}

	done := make(chan struct{})
	go func() {
		manager.shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected shutdown to cancel pending endpoint creation")
	}
}
