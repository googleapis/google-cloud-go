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
	"fmt"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type testEndpoint struct {
	address          string
	healthy          bool
	transientFailure bool
	conn             *grpc.ClientConn
}

func (e *testEndpoint) Address() string {
	return e.address
}

func (e *testEndpoint) IsHealthy() bool {
	if e.conn != nil {
		return e.conn.GetState() == connectivity.Ready
	}
	return e.healthy
}

func (e *testEndpoint) IsTransientFailure() bool {
	if e.conn != nil {
		return e.conn.GetState() == connectivity.TransientFailure
	}
	return e.transientFailure
}

func (e *testEndpoint) GetConn() *grpc.ClientConn {
	return e.conn
}

type testEndpointCache struct {
	endpoints map[string]*testEndpoint
	onGet     func(address string)
}

func newTestEndpointCache() *testEndpointCache {
	return &testEndpointCache{endpoints: make(map[string]*testEndpoint)}
}

func (c *testEndpointCache) ClientFor(_ channelEndpoint) spannerClient { return nil }
func (c *testEndpointCache) Close() error                              { return nil }
func (c *testEndpointCache) DefaultChannel() channelEndpoint {
	return &testEndpoint{address: "", healthy: true}
}

func (c *testEndpointCache) Get(_ context.Context, address string) channelEndpoint {
	if c.onGet != nil {
		c.onGet(address)
	}
	if endpoint, ok := c.endpoints[address]; ok {
		return endpoint
	}
	endpoint := &testEndpoint{address: address, healthy: true}
	c.endpoints[address] = endpoint
	return endpoint
}

func (c *testEndpointCache) GetIfPresent(address string) channelEndpoint {
	endpoint, ok := c.endpoints[address]
	if !ok {
		return nil
	}
	return endpoint
}

func (c *testEndpointCache) Evict(address string) {
	delete(c.endpoints, address)
}

func (c *testEndpointCache) setHealthy(address string, healthy bool) {
	if endpoint, ok := c.endpoints[address]; ok {
		endpoint.healthy = healthy
	}
}

func (c *testEndpointCache) setTransientFailure(address string, transientFailure bool) {
	if endpoint, ok := c.endpoints[address]; ok {
		endpoint.transientFailure = transientFailure
	}
}

func TestCachedTabletMatches_DirectedReadOptionsWrappers(t *testing.T) {
	tablet := &cachedTablet{
		distance: 1,
		role:     sppb.Tablet_READ_ONLY,
		location: "us-east1",
	}

	include := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{Location: "us-east1", Type: sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY},
				},
			},
		},
	}
	if !tablet.matches(include) {
		t.Fatal("expected include_replicas selection to match tablet")
	}

	exclude := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_ExcludeReplicas_{
			ExcludeReplicas: &sppb.DirectedReadOptions_ExcludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{Location: "us-east1", Type: sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY},
				},
			},
		},
	}
	if tablet.matches(exclude) {
		t.Fatal("expected exclude_replicas selection to reject tablet")
	}
}

func TestCachedGroupFillRoutingHint_PreferLeaderDisabledByDirectedReadOptions(t *testing.T) {
	group := &cachedGroup{
		leaderIdx: 0,
		tablets: []*cachedTablet{
			{
				tabletUID:     1,
				serverAddress: "leader",
				distance:      1,
				role:          sppb.Tablet_READ_WRITE,
				location:      "us-central1",
			},
			{
				tabletUID:     2,
				serverAddress: "replica",
				distance:      1,
				role:          sppb.Tablet_READ_ONLY,
				location:      "us-east1",
			},
		},
	}

	endpoint := group.fillRoutingHint(
		context.Background(),
		newPassthroughChannelEndpointCache(),
		true,
		&sppb.DirectedReadOptions{},
		&sppb.RoutingHint{},
	)
	if endpoint == nil || endpoint.Address() != "leader" {
		t.Fatalf("expected leader for default reads, got %#v", endpoint)
	}

	directedRead := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{Location: "us-east1", Type: sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY},
				},
			},
		},
	}
	endpoint = group.fillRoutingHint(
		context.Background(),
		newPassthroughChannelEndpointCache(),
		true,
		directedRead,
		&sppb.RoutingHint{},
	)
	if endpoint == nil || endpoint.Address() != "replica" {
		t.Fatalf("expected directed read to route to replica, got %#v", endpoint)
	}
}

func TestKeyRangeCache_FillRoutingHintSkipsUnhealthyCachedTablet(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)
	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 0,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server1",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     2,
						ServerAddress: "server2",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	initialHint := &sppb.RoutingHint{Key: []byte("a")}
	initialEndpoint := cache.fillRoutingHint(context.Background(), false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, initialHint)
	if initialEndpoint == nil {
		t.Fatal("expected initial endpoint")
	}

	endpointCache.setHealthy("server1", false)

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(context.Background(), false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server2" {
		t.Fatalf("expected server2 after server1 became unhealthy, got %#v", endpoint)
	}
	if len(hint.GetSkippedTabletUid()) != 0 {
		t.Fatalf("expected unhealthy non-transient tablet to be skipped silently, got %#v", hint.GetSkippedTabletUid())
	}
}

func TestKeyRangeCache_FillRoutingHintSkipsTransientFailureTablet(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)
	manager := newEndpointLifecycleManagerWithOptions(endpointCache, time.Hour, time.Hour, time.Now)
	defer manager.shutdown()
	cache.setLifecycleManager(manager)

	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 0,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server1",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     2,
						ServerAddress: "server2",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	endpointCache.Get(context.Background(), "server1")
	endpointCache.Get(context.Background(), "server2")
	endpointCache.setHealthy("server1", false)
	endpointCache.setTransientFailure("server1", true)

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(context.Background(), false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server2" {
		t.Fatalf("expected server2 after transient failure on server1, got %#v", endpoint)
	}
	if len(hint.GetSkippedTabletUid()) != 1 || hint.GetSkippedTabletUid()[0].GetTabletUid() != 1 {
		t.Fatalf("expected tablet 1 to be marked skipped, got %#v", hint.GetSkippedTabletUid())
	}

	waitForCondition(t, time.Second, func() bool {
		return manager.isManaged("server1")
	})
}

func TestKeyRangeCache_FillRoutingHintRecordsKnownTransientFailure(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)
	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 0,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server-leader",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     2,
						ServerAddress: "server-failed",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	endpointCache.Get(context.Background(), "server-leader")
	endpointCache.Get(context.Background(), "server-failed")
	endpointCache.setHealthy("server-failed", false)
	endpointCache.setTransientFailure("server-failed", true)

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(context.Background(), true, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server-leader" {
		t.Fatalf("expected leader endpoint, got %#v", endpoint)
	}
	if len(hint.GetSkippedTabletUid()) != 1 || hint.GetSkippedTabletUid()[0].GetTabletUid() != 2 {
		t.Fatalf("expected tablet 2 to be marked skipped, got %#v", hint.GetSkippedTabletUid())
	}
}

func TestKeyRangeCache_FillRoutingHintRecordsRecentlyEvictedTransientFailure(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)
	manager := newEndpointLifecycleManagerWithOptions(endpointCache, time.Hour, time.Hour, time.Now)
	defer manager.shutdown()
	cache.setLifecycleManager(manager)

	manager.mu.Lock()
	manager.transientFailureEvictedAddresses["server-failed"] = struct{}{}
	manager.mu.Unlock()

	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 0,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server-leader",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     2,
						ServerAddress: "server-failed",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	endpointCache.Get(context.Background(), "server-leader")

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(context.Background(), true, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server-leader" {
		t.Fatalf("expected leader endpoint, got %#v", endpoint)
	}
	if len(hint.GetSkippedTabletUid()) != 1 || hint.GetSkippedTabletUid()[0].GetTabletUid() != 2 {
		t.Fatalf("expected tablet 2 to be marked skipped, got %#v", hint.GetSkippedTabletUid())
	}
}

func TestKeyRangeCache_FillRoutingHintWithDetailsTracksSelectionAndSkipReasons(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)

	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 3,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server-first",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     3,
						ServerAddress: "server-transient",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     4,
						ServerAddress: "server-excluded",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
					{
						TabletUid:     2,
						ServerAddress: "server-leader",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	endpointCache.Get(context.Background(), "server-first")
	endpointCache.Get(context.Background(), "server-leader")
	endpointCache.Get(context.Background(), "server-transient")
	endpointCache.setHealthy("server-first", false)
	endpointCache.setHealthy("server-transient", false)
	endpointCache.setTransientFailure("server-transient", true)

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint, details := cache.fillRoutingHintWithExclusionsAndDetails(
		context.Background(),
		false,
		rangeModeCoveringSplit,
		&sppb.DirectedReadOptions{},
		hint,
		func(address string) bool { return address == "server-excluded" },
	)

	if endpoint == nil || endpoint.Address() != "server-leader" {
		t.Fatalf("expected server-leader endpoint, got %#v", endpoint)
	}
	if !details.selectedIsLeader {
		t.Fatal("expected selected endpoint to be marked as leader")
	}
	if details.selectedIsFirstReplica {
		t.Fatal("did not expect selected endpoint to be marked as first replica")
	}
	if got, want := details.selectedEndpoint, "server-leader"; got != want {
		t.Fatalf("selectedEndpoint=%q, want %q", got, want)
	}
	if got, want := details.notReadySkipList(), []string{"server-first"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("notReadySkipList=%v, want %v", got, want)
	}
	if got, want := details.transientFailureSkipList(), []string{"server-transient"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("transientFailureSkipList=%v, want %v", got, want)
	}
	if got, want := details.resourceExhaustedExclusionList(), []string{"server-excluded"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("resourceExhaustedExclusionList=%v, want %v", got, want)
	}
}

func TestKeyRangeCache_FillRoutingHintWithDetailsMarksCacheMiss(t *testing.T) {
	cache := newKeyRangeCache(newPassthroughChannelEndpointCache())

	endpoint, details := cache.fillRoutingHintWithExclusionsAndDetails(
		context.Background(),
		false,
		rangeModeCoveringSplit,
		&sppb.DirectedReadOptions{},
		&sppb.RoutingHint{Key: []byte("missing")},
		nil,
	)

	if endpoint != nil {
		t.Fatalf("expected nil endpoint on cache miss, got %#v", endpoint)
	}
	if got, want := details.defaultReasonCode, routeReasonRangeCacheMiss; got != want {
		t.Fatalf("defaultReasonCode=%q, want %q", got, want)
	}
}

func TestKeyRangeCache_FillRoutingHintCreatesMissingEndpointOutsideGroupLock(t *testing.T) {
	endpointCache := newTestEndpointCache()
	cache := newKeyRangeCache(endpointCache)

	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   5,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:    5,
				Generation:  []byte("1"),
				LeaderIndex: 0,
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server-missing",
						Incarnation:   []byte("1"),
						Distance:      0,
					},
				},
			},
		},
	})

	state := cache.loadState()
	targetRange := cache.findRangeInState(state, []byte("a"), nil, rangeModeCoveringSplit, cache.loadRoutingConfig())
	if targetRange == nil {
		t.Fatal("expected target range")
	}
	group := state.findGroup(targetRange.groupUID)
	if group == nil {
		t.Fatal("expected target group")
	}

	endpointCache.onGet = func(address string) {
		if address != "server-missing" {
			return
		}
		locked := make(chan struct{}, 1)
		go func() {
			group.mu.Lock()
			group.mu.Unlock()
			locked <- struct{}{}
		}()
		select {
		case <-locked:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("endpoint creation happened while group lock was held")
		}
	}

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(context.Background(), false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server-missing" {
		t.Fatalf("expected warmed endpoint server-missing, got %#v", endpoint)
	}
}

func TestKeyRangeCache_ShrinkTo(t *testing.T) {
	cache := newKeyRangeCache(newPassthroughChannelEndpointCache())
	const numRanges = 40
	for i := 0; i < numRanges; i++ {
		startKey := []byte(fmt.Sprintf("%04d", i))
		limitKey := []byte(fmt.Sprintf("%04d", i+1))
		cache.addRanges(&sppb.CacheUpdate{
			Range: []*sppb.Range{
				{
					StartKey:   startKey,
					LimitKey:   limitKey,
					GroupUid:   uint64(i + 1),
					SplitId:    uint64(i + 1),
					Generation: []byte("1"),
				},
			},
			Group: []*sppb.Group{
				{
					GroupUid:    uint64(i + 1),
					Generation:  []byte("1"),
					LeaderIndex: 0,
					Tablets: []*sppb.Tablet{
						{
							TabletUid:     uint64(i + 1),
							ServerAddress: fmt.Sprintf("server-%d", i),
							Incarnation:   []byte("1"),
							Distance:      0,
						},
					},
				},
			},
		})
	}

	if got := cache.size(); got != numRanges {
		t.Fatalf("cache.size()=%d, want %d", got, numRanges)
	}
	if got := countCacheHits(cache, numRanges); got != numRanges {
		t.Fatalf("countCacheHits=%d, want %d", got, numRanges)
	}

	cache.shrinkTo(30)
	if got := cache.size(); got != 30 {
		t.Fatalf("cache.size()=%d, want 30", got)
	}
	if got := countCacheHits(cache, numRanges); got != 30 {
		t.Fatalf("countCacheHits=%d, want 30", got)
	}

	cache.shrinkTo(5)
	if got := cache.size(); got != 5 {
		t.Fatalf("cache.size()=%d, want 5", got)
	}
	if got := countCacheHits(cache, numRanges); got != 5 {
		t.Fatalf("countCacheHits=%d, want 5", got)
	}

	cache.shrinkTo(0)
	if got := cache.size(); got != 0 {
		t.Fatalf("cache.size()=%d, want 0", got)
	}
	if got := countCacheHits(cache, numRanges); got != 0 {
		t.Fatalf("countCacheHits=%d, want 0", got)
	}
}

func TestKeyRangeCache_GetActiveAddresses(t *testing.T) {
	cache := newKeyRangeCache(newPassthroughChannelEndpointCache())
	cache.addRanges(&sppb.CacheUpdate{
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("m"),
				GroupUid:   1,
				SplitId:    1,
				Generation: []byte("1"),
			},
			{
				StartKey:   []byte("m"),
				LimitKey:   []byte("z"),
				GroupUid:   2,
				SplitId:    2,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:   1,
				Generation: []byte("1"),
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     1,
						ServerAddress: "server-a",
						Incarnation:   []byte("1"),
					},
					{
						TabletUid:     2,
						ServerAddress: "server-b",
						Incarnation:   []byte("1"),
					},
				},
			},
			{
				GroupUid:   2,
				Generation: []byte("1"),
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     3,
						ServerAddress: "server-b",
						Incarnation:   []byte("1"),
					},
					{
						TabletUid:   4,
						Incarnation: []byte("1"),
					},
				},
			},
		},
	})

	addresses := activeAddressesForTest(cache)
	if len(addresses) != 2 {
		t.Fatalf("expected 2 active addresses, got %d", len(addresses))
	}
	if _, ok := addresses["server-a"]; !ok {
		t.Fatal("expected server-a to be active")
	}
	if _, ok := addresses["server-b"]; !ok {
		t.Fatal("expected server-b to be active")
	}
}

func activeAddressesForTest(cache *keyRangeCache) map[string]struct{} {
	state := cache.loadState()
	addresses := make(map[string]struct{})
	for _, shard := range state.groupShards {
		for _, group := range shard {
			group.mu.Lock()
			for _, tablet := range group.tablets {
				if tablet.serverAddress == "" {
					continue
				}
				addresses[tablet.serverAddress] = struct{}{}
			}
			group.mu.Unlock()
		}
	}
	return addresses
}

func countCacheHits(cache *keyRangeCache, numRanges int) int {
	hits := 0
	for i := 0; i < numRanges; i++ {
		key := []byte(fmt.Sprintf("%04d", i))
		hint := &sppb.RoutingHint{Key: key}
		if endpoint := cache.fillRoutingHint(context.Background(), false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint); endpoint != nil {
			hits++
		}
	}
	return hits
}
