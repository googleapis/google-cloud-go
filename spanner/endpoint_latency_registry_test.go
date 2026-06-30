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

func TestEndpointRuntimeScoreKeysByOperationUID(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := defaultEndpointRoutingConfig()
	cfg.now = clock.Now
	endpoint := &grpcChannelEndpoint{address: "server-a:443"}

	endpoint.recordLatency(cfg, 7, false, 25*time.Millisecond)

	if !endpoint.hasScore(cfg, 7, false) {
		t.Fatal("expected score for recorded operation/address")
	}
	if endpoint.hasScore(cfg, 8, false) {
		t.Fatal("expected different operation UID to have no score")
	}
	if endpoint.hasScore(cfg, 7, true) {
		t.Fatal("expected preferLeader to remain part of the key")
	}
}

func TestEndpointRuntimeScoreLookupRefreshesAccess(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(1_000, 0))
	cfg := defaultEndpointRoutingConfig()
	cfg.now = clock.Now
	endpoint := &grpcChannelEndpoint{address: "server-a:443"}

	endpoint.recordLatency(cfg, 7, false, 25*time.Millisecond)
	if !endpoint.hasScore(cfg, 7, false) {
		t.Fatal("expected score after initial write")
	}

	key := endpointScoreKey{operationUID: 7, preferLeader: false}
	endpoint.stateMu.Lock()
	entry := endpoint.scores[key]
	lastAccess := entry.lastAccess
	endpoint.stateMu.Unlock()
	if entry == nil {
		t.Fatal("expected score entry to exist")
	}

	clock.Advance(time.Minute)
	if !endpoint.hasScore(cfg, 7, false) {
		t.Fatal("expected score to remain present during lookup")
	}
	endpoint.stateMu.Lock()
	touchedAfterHasScore := entry.lastAccess
	endpoint.stateMu.Unlock()
	if !touchedAfterHasScore.After(lastAccess) {
		t.Fatal("expected hasScore lookup to refresh last access")
	}

	clock.Advance(time.Second)
	if cost := endpoint.selectionCost(cfg, 7, false); cost == 0 {
		t.Fatal("expected non-zero selection cost during lookup")
	}
	endpoint.stateMu.Lock()
	touchedAfterSelection := entry.lastAccess
	endpoint.stateMu.Unlock()
	if !touchedAfterSelection.After(touchedAfterHasScore) {
		t.Fatal("expected selection lookup to refresh last access")
	}
}

func TestEndpointRuntimeScoreExpiredEntryIsHiddenBeforeCleanup(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(1_500, 0))
	cfg := defaultEndpointRoutingConfig()
	cfg.now = clock.Now
	cfg.scoreExpireAfter = time.Minute
	endpoint := &grpcChannelEndpoint{address: "server-a:443"}

	endpoint.recordLatency(cfg, 7, false, 25*time.Millisecond)
	clock.Advance(2 * time.Minute)

	if endpoint.hasScore(cfg, 7, false) {
		t.Fatal("expected expired entry to be hidden before cleanup")
	}
	if got := endpoint.selectionCost(cfg, 7, false); got == 0 {
		t.Fatal("expected fallback selection cost after expiry")
	}

	endpoint.pruneRoutingState(cfg, 0)
	endpoint.stateMu.Lock()
	_, ok := endpoint.scores[endpointScoreKey{operationUID: 7, preferLeader: false}]
	endpoint.stateMu.Unlock()
	if ok {
		t.Fatal("expected cleanup to remove expired entry")
	}
}
