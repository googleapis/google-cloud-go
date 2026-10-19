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

import "testing"

func TestLRUCache(t *testing.T) {
	c := newLRUCache[string, int](3)

	// Put b1, b2, b3.
	c.put("b1", 1)
	c.put("b2", 2)
	c.put("b3", 3)

	// Access b1 to promote it (MRU).
	if _, found := c.get("b1"); !found {
		t.Fatalf("expected b1 to be found")
	}

	// Put b4 to trigger eviction of oldest key (b2).
	c.put("b4", 4)

	// Verify b2 is evicted.
	if _, found := c.get("b2"); found {
		t.Errorf("expected b2 to be evicted")
	}

	// Verify others still exist.
	for _, k := range []string{"b1", "b3", "b4"} {
		if _, found := c.get(k); !found {
			t.Errorf("expected %q to remain in cache", k)
		}
	}

	// Evict b3 manually.
	c.evict("b3")
	if _, found := c.get("b3"); found {
		t.Errorf("expected b3 to be evicted manually")
	}
}
