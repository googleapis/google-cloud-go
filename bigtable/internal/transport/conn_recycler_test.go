// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"testing"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

func TestConnectionRecycler_CheckRecycle(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }
	ctx := context.Background()

	setAge := func(entry *connEntry, age time.Duration) {
		entry.conn.createdAt.Store(time.Now().Add(-age).UnixMilli())
	}

	t.Run("RecycleOldConnection", func(t *testing.T) {
		config := btopt.ConnectionRecycleConfig{
			MaxAge:    10 * time.Minute,
			MaxJitter: 0,
		}

		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, time.Now())
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		recycler := NewConnectionRecycler(config, pool)

		conns := pool.getConns()
		if len(conns) != 1 {
			t.Fatalf("Expected 1 connection, got %d", len(conns))
		}
		originalEntry := conns[0]
		originalConnPtr := originalEntry.conn

		// maxAge > 20m
		setAge(originalEntry, 20*time.Minute)
		recycler.checkRecycle()

		// recycled fast as it does not have any pending rpcs
		newConns := pool.getConns()
		if newConns[0].conn == originalConnPtr {
			t.Error("Connection was older than MaxAge but was NOT recycled")
		}
	})

	t.Run("DoesNotReplaceIfConnWithinMaxAge", func(t *testing.T) {
		config := btopt.ConnectionRecycleConfig{
			MaxAge:    10 * time.Minute,
			MaxJitter: 0,
		}

		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, time.Now())
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		recycler := NewConnectionRecycler(config, pool)

		entry := pool.getConns()[0]
		originalConnPtr := entry.conn

		// < 10mins
		setAge(entry, 5*time.Minute)

		// recycled fast as it does not have any pending rpcs
		recycler.checkRecycle()

		if pool.getConns()[0].conn != originalConnPtr {
			t.Error("Connection WAS recycled unexpectedly")
		}
	})

	t.Run("RespectsMaxRecyclePerBatch", func(t *testing.T) {
		config := btopt.ConnectionRecycleConfig{
			MaxAge:    10 * time.Minute,
			MaxJitter: 0,
		}
		// 5 conns
		poolSize := 5
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, time.Now())
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		recycler := NewConnectionRecycler(config, pool)

		// force age to be old
		conns := pool.getConns()
		originalConns := make(map[*BigtableConn]bool)
		for _, e := range conns {
			setAge(e, 60*time.Minute)
			originalConns[e.conn] = true
		}

		// Trigger recycle
		recycler.checkRecycle()

		currentConns := pool.getConns()
		changedCount := 0
		for _, e := range currentConns {
			if !originalConns[e.conn] {
				changedCount++
			}
		}
		if changedCount != maxRecyclePerBatch {
			t.Errorf("Expected exactly %d recycled connections (batch limit), but got %d", maxRecyclePerBatch, changedCount)
		}
	})
}
