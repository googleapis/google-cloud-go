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
}

type logicalRequestEndpointExclusionCache struct {
	mu         sync.Mutex
	entries    map[string]logicalRequestEndpointExclusion
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
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
	c.entries[logicalRequestKey] = entry

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
	for logicalRequestKey, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, logicalRequestKey)
		}
	}
}

func (c *logicalRequestEndpointExclusionCache) pruneOverflowLocked() {
	for len(c.entries) > c.maxEntries {
		var (
			oldestKey string
			oldestAt  time.Time
			found     bool
		)
		for logicalRequestKey, entry := range c.entries {
			if !found || entry.expiresAt.Before(oldestAt) {
				oldestKey = logicalRequestKey
				oldestAt = entry.expiresAt
				found = true
			}
		}
		if !found {
			return
		}
		delete(c.entries, oldestKey)
	}
}

func (c *logicalRequestEndpointExclusionCache) size() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
