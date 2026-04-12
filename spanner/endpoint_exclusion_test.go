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
	"testing"
	"time"
)

func TestLogicalRequestEndpointExclusionCache_RecordAndConsume(t *testing.T) {
	cache := newLogicalRequestEndpointExclusionCacheWithOptions(10, time.Minute, time.Now)

	cache.record("logical-1", "replica-a:443")
	cache.record("logical-1", "replica-b:443")

	excluded := cache.consume("logical-1")
	if !excluded("replica-a:443") {
		t.Fatal("expected replica-a to be excluded")
	}
	if !excluded("replica-b:443") {
		t.Fatal("expected replica-b to be excluded")
	}
	if excluded("replica-c:443") {
		t.Fatal("did not expect replica-c to be excluded")
	}
	if cache.size() != 0 {
		t.Fatalf("expected consumed entry to be removed, got size %d", cache.size())
	}
}

func TestLogicalRequestEndpointExclusionCache_ExpiresEntries(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newLogicalRequestEndpointExclusionCacheWithOptions(
		10,
		time.Minute,
		func() time.Time { return now },
	)

	cache.record("logical-2", "replica-a:443")
	now = now.Add(2 * time.Minute)

	excluded := cache.consume("logical-2")
	if excluded("replica-a:443") {
		t.Fatal("did not expect expired exclusion to be returned")
	}
}

func TestLogicalRequestEndpointExclusionCache_RefreshKeepsEntryAlive(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newLogicalRequestEndpointExclusionCacheWithOptions(
		10,
		time.Minute,
		func() time.Time { return now },
	)

	cache.record("logical-3", "replica-a:443")
	now = now.Add(30 * time.Second)
	cache.record("logical-3", "replica-b:443")
	now = now.Add(40 * time.Second)

	excluded := cache.consume("logical-3")
	if !excluded("replica-a:443") || !excluded("replica-b:443") {
		t.Fatal("expected refreshed logical request entry to remain active")
	}
}

func TestLogicalRequestEndpointExclusionCache_PrunesOldestOnOverflow(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newLogicalRequestEndpointExclusionCacheWithOptions(
		2,
		time.Minute,
		func() time.Time { return now },
	)

	cache.record("logical-1", "replica-a:443")
	now = now.Add(time.Second)
	cache.record("logical-2", "replica-b:443")
	now = now.Add(time.Second)
	cache.record("logical-3", "replica-c:443")

	if cache.consume("logical-1")("replica-a:443") {
		t.Fatal("expected oldest logical request to be evicted on overflow")
	}
	if !cache.consume("logical-2")("replica-b:443") {
		t.Fatal("expected second logical request to remain present")
	}
	if !cache.consume("logical-3")("replica-c:443") {
		t.Fatal("expected newest logical request to remain present")
	}
}
