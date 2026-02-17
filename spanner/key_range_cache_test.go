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
	"fmt"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type testEndpoint struct {
	address string
	healthy bool
}

func (e *testEndpoint) Address() string {
	return e.address
}

func (e *testEndpoint) IsHealthy() bool {
	return e.healthy
}

type testEndpointCache struct {
	endpoints map[string]*testEndpoint
}

func newTestEndpointCache() *testEndpointCache {
	return &testEndpointCache{endpoints: make(map[string]*testEndpoint)}
}

func (c *testEndpointCache) Get(address string) channelEndpoint {
	if endpoint, ok := c.endpoints[address]; ok {
		return endpoint
	}
	endpoint := &testEndpoint{address: address, healthy: true}
	c.endpoints[address] = endpoint
	return endpoint
}

func (c *testEndpointCache) setHealthy(address string, healthy bool) {
	if endpoint, ok := c.endpoints[address]; ok {
		endpoint.healthy = healthy
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
	initialEndpoint := cache.fillRoutingHint(false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, initialHint)
	if initialEndpoint == nil {
		t.Fatal("expected initial endpoint")
	}

	endpointCache.setHealthy("server1", false)

	hint := &sppb.RoutingHint{Key: []byte("a")}
	endpoint := cache.fillRoutingHint(false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint)
	if endpoint == nil || endpoint.Address() != "server2" {
		t.Fatalf("expected server2 after server1 became unhealthy, got %#v", endpoint)
	}
	if len(hint.GetSkippedTabletUid()) != 1 || hint.GetSkippedTabletUid()[0].GetTabletUid() != 1 {
		t.Fatalf("expected tablet 1 to be marked skipped, got %#v", hint.GetSkippedTabletUid())
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

func countCacheHits(cache *keyRangeCache, numRanges int) int {
	hits := 0
	for i := 0; i < numRanges; i++ {
		key := []byte(fmt.Sprintf("%04d", i))
		hint := &sppb.RoutingHint{Key: key}
		if endpoint := cache.fillRoutingHint(false, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint); endpoint != nil {
			hits++
		}
	}
	return hits
}
