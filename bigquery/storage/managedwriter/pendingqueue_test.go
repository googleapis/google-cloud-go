// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"math/rand"
	"testing"
)

func TestPendingQueueLifecycle(t *testing.T) {

	maxMessages := 1000
	dests := []string{"d1", "d2", "d3", "d4"}
	pq := newPendingQueue(true, maxMessages)

	for i := 0; i < maxMessages; i++ {
		dest := dests[rand.Intn(len(dests))]
		fakePw := &pendingWrite{writeStreamID: dest}
		if err := pq.addPending(fakePw); err != nil {
			t.Fatalf("error inserting write %d: %v", i, err)
		}
	}

	if _, err := pq.drain(); err == nil {
		t.Fatalf("expected non-closed drain to fail, but succeeded")
	}
	pq.closeAdd()
	dest := dests[rand.Intn(len(dests))]
	fakePw := &pendingWrite{writeStreamID: dest}
	if err := pq.addPending(fakePw); err == nil {
		t.Fatalf("expected addPending to fail after close, but succeeded")
	}

	info := pq.listDests()
	totalQueued := 0
	if len(info) > len(dests) {
		// more keys than expected.
		t.Fatalf("more keys present in queue than expected.  used %d dests, but %d in queue", len(dests), len(info))
	}
	for _, c := range info {
		totalQueued = totalQueued + c
	}
	if totalQueued != maxMessages {
		t.Errorf("wanted %d queued, got %d", maxMessages, totalQueued)
	}
}
