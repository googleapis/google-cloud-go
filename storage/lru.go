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

import "sync"

// lruCache is a generic, concurrency-safe Least Recently Used cache.
type lruCache[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]V
	keys    []K // tracking access order (least recent at the front, most recent at the end)
	limit   int
}

func newLRUCache[K comparable, V any](limit int) *lruCache[K, V] {
	return &lruCache[K, V]{
		entries: make(map[K]V),
		limit:   limit,
	}
}

func (c *lruCache[K, V]) get(key K) (V, bool) {
	if c == nil {
		var zero V
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if ok {
		c.promote(key)
	}
	return entry, ok
}

func (c *lruCache[K, V]) put(key K, val V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		c.entries[key] = val
		c.promote(key)
		return
	}

	if len(c.entries) >= c.limit {
		oldest := c.keys[0]
		c.keys = c.keys[1:]
		delete(c.entries, oldest)
	}

	c.entries[key] = val
	c.keys = append(c.keys, key)
}

func (c *lruCache[K, V]) evict(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists {
		return
	}
	delete(c.entries, key)

	for i, k := range c.keys {
		if k == key {
			c.keys = append(c.keys[:i], c.keys[i+1:]...)
			break
		}
	}
}

// promote moves the key to the end of the keys slice (most recently used).
// Must be called under lock c.mu.
func (c *lruCache[K, V]) promote(key K) {
	for i, k := range c.keys {
		if k == key {
			c.keys = append(c.keys[:i], c.keys[i+1:]...)
			c.keys = append(c.keys, key)
			break
		}
	}
}
