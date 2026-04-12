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
	"container/heap"
	"sync"
	"time"
)

const (
	maxTrackedExcludedLogicalRequests = 100_000
	excludedLogicalRequestTTL         = 10 * time.Minute
)

type endpointExcluder func(string) bool

type logicalRequestEndpointExclusion struct {
	addresses map[string]struct{}
	expiresAt time.Time
	version   uint64
}

type logicalRequestEndpointExclusionExpiry struct {
	logicalRequestKey string
	expiresAt         time.Time
	version           uint64
}

type logicalRequestEndpointExclusionExpiryHeap []logicalRequestEndpointExclusionExpiry

func (h logicalRequestEndpointExclusionExpiryHeap) Len() int {
	return len(h)
}

func (h logicalRequestEndpointExclusionExpiryHeap) Less(i, j int) bool {
	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h logicalRequestEndpointExclusionExpiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *logicalRequestEndpointExclusionExpiryHeap) Push(x any) {
	*h = append(*h, x.(logicalRequestEndpointExclusionExpiry))
}

func (h *logicalRequestEndpointExclusionExpiryHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type logicalRequestEndpointExclusionCache struct {
	mu         sync.Mutex
	entries    map[string]logicalRequestEndpointExclusion
	expiries   logicalRequestEndpointExclusionExpiryHeap
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
	nextVer    uint64
}

func noExcludedEndpoints(string) bool {
	return false
}

func isEndpointExcluded(excluded endpointExcluder, address string) bool {
	return excluded != nil && excluded(address)
}

func newLogicalRequestEndpointExclusionCache() *logicalRequestEndpointExclusionCache {
	return newLogicalRequestEndpointExclusionCacheWithOptions(
		maxTrackedExcludedLogicalRequests,
		excludedLogicalRequestTTL,
		time.Now,
	)
}

func newLogicalRequestEndpointExclusionCacheWithOptions(
	maxEntries int,
	ttl time.Duration,
	now func() time.Time,
) *logicalRequestEndpointExclusionCache {
	if maxEntries <= 0 {
		maxEntries = maxTrackedExcludedLogicalRequests
	}
	if ttl <= 0 {
		ttl = excludedLogicalRequestTTL
	}
	if now == nil {
		now = time.Now
	}
	return &logicalRequestEndpointExclusionCache{
		entries:    make(map[string]logicalRequestEndpointExclusion),
		maxEntries: maxEntries,
		ttl:        ttl,
		now:        now,
	}
}

func (c *logicalRequestEndpointExclusionCache) record(logicalRequestKey, address string) {
	if c == nil || logicalRequestKey == "" || address == "" {
		return
	}

	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pruneExpiredLocked(now)

	entry := c.entries[logicalRequestKey]
	if entry.addresses == nil {
		entry.addresses = make(map[string]struct{})
	}
	entry.addresses[address] = struct{}{}
	entry.expiresAt = now.Add(c.ttl)
	entry.version = c.nextVersionLocked()
	c.entries[logicalRequestKey] = entry
	heap.Push(&c.expiries, logicalRequestEndpointExclusionExpiry{
		logicalRequestKey: logicalRequestKey,
		expiresAt:         entry.expiresAt,
		version:           entry.version,
	})

	c.pruneOverflowLocked()
}

func (c *logicalRequestEndpointExclusionCache) consume(logicalRequestKey string) endpointExcluder {
	if c == nil || logicalRequestKey == "" {
		return noExcludedEndpoints
	}

	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pruneExpiredLocked(now)

	entry, ok := c.entries[logicalRequestKey]
	if !ok || len(entry.addresses) == 0 {
		return noExcludedEndpoints
	}
	delete(c.entries, logicalRequestKey)

	addresses := make(map[string]struct{}, len(entry.addresses))
	for address := range entry.addresses {
		addresses[address] = struct{}{}
	}
	return func(address string) bool {
		_, ok := addresses[address]
		return ok
	}
}

func (c *logicalRequestEndpointExclusionCache) pruneExpiredLocked(now time.Time) {
	for c.expiries.Len() > 0 {
		next := c.expiries[0]
		if next.expiresAt.After(now) {
			return
		}
		heap.Pop(&c.expiries)
		entry, ok := c.entries[next.logicalRequestKey]
		if !ok || entry.version != next.version {
			continue
		}
		delete(c.entries, next.logicalRequestKey)
	}
}

func (c *logicalRequestEndpointExclusionCache) pruneOverflowLocked() {
	for len(c.entries) > c.maxEntries {
		for c.expiries.Len() > 0 {
			next := heap.Pop(&c.expiries).(logicalRequestEndpointExclusionExpiry)
			entry, ok := c.entries[next.logicalRequestKey]
			if !ok || entry.version != next.version {
				continue
			}
			delete(c.entries, next.logicalRequestKey)
			break
		}
		if len(c.entries) <= c.maxEntries {
			return
		}
		for logicalRequestKey := range c.entries {
			delete(c.entries, logicalRequestKey)
			break
		}
	}
}

func (c *logicalRequestEndpointExclusionCache) nextVersionLocked() uint64 {
	c.nextVer++
	return c.nextVer
}

func (c *logicalRequestEndpointExclusionCache) size() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
