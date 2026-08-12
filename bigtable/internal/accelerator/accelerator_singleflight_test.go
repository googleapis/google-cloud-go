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

package accelerator

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
)

// slowSessionClient wraps mockSessionClient's Open* methods with a barrier so
// N concurrent openHandle calls are guaranteed past the fast-path miss check
// simultaneously before the winner's openFn completes. Without the barrier,
// the winner is fast enough that the natural scheduling wins the race and
// most callers hit the fast-path cache — the same "31 hits / 1 miss with
// N=32" behavior noted in bigtable/internal/session/pr20366_repro_test.go.
type slowSessionClient struct {
	// arrived counts callers that have entered OpenTable and is used to fire
	// entered once the full burst is inside the slow path.
	arrived atomic.Int32
	// entered fires when arrived == wantN, so the test can release the burst.
	entered chan struct{}
	// wantN is the expected concurrent-caller count for the barrier.
	wantN int32
	// release unblocks all in-flight OpenTable calls together.
	release chan struct{}

	// openTableCalls counts real invocations of OpenTable, i.e. how many
	// times the accelerator's slow path actually reached the underlying
	// session Client for the SAME resource.
	openTableCalls atomic.Int32

	table session.TableAPI
}

func newSlowSessionClient(wantN int, tbl session.TableAPI) *slowSessionClient {
	return &slowSessionClient{
		entered: make(chan struct{}),
		wantN:   int32(wantN),
		release: make(chan struct{}),
		table:   tbl,
	}
}

func (s *slowSessionClient) OpenTable(name string) session.TableAPI {
	s.openTableCalls.Add(1)
	if s.arrived.Add(1) == s.wantN {
		close(s.entered)
	}
	<-s.release
	return s.table
}

func (s *slowSessionClient) OpenAuthorizedView(table, view string) session.TableAPI {
	return s.OpenTable(table)
}
func (s *slowSessionClient) OpenMaterializedView(view string) session.TableAPI {
	return s.OpenTable(view)
}
func (s *slowSessionClient) MeterProvider()                                   metric.MeterProvider { return nil }
func (s *slowSessionClient) SessionDebug()                                    btransport.SessionDebugProvider { return nil }
func (s *slowSessionClient) ChannelDebug()                                    btransport.ChannelDebugProvider { return nil }
func (s *slowSessionClient) ConfigDebug()                                     btransport.ConfigDebugProvider  { return nil }
func (s *slowSessionClient) AddSessionLoadListener(func(float64)) func()      { return func() {} }
func (s *slowSessionClient) Close() error                                     { return nil }

// TestOpenHandle_SingleFlightCollapsesColdStartBurst is the accelerator-side
// half of the pr20366 repro: it pins the cold-start burst dynamic that lets
// the accelerator trigger ErrPoolClosed at the session layer even though
// client.OpenTable never does. Without singleflight in openHandle, N
// concurrent first-touch RPCs for the same resource all reach
// sessionTables.GetOrOpen, all call openFn (= sc.OpenTable), N-1 hit
// GetOrOpen's loser branch that Close()s their sessionTable, and under the
// pre-PR-20366 session layer that Close tore down the shared pool. With
// singleflight, exactly one goroutine calls sc.OpenTable — the other N-1
// wait on singleflight's internal channel and receive the same handle.
//
// Assertion: after N=32 concurrent openHandle calls for the same fresh
// resource, sc.openTableCalls == 1.
func TestOpenHandle_SingleFlightCollapsesColdStartBurst(t *testing.T) {
	const N = 32
	sc := newSlowSessionClient(N, &mockSessionTableAPI{})
	channel := newTestChannel(t, nil)
	// Replace the mock injected by newTestChannel with our slow variant so we
	// can time the burst.
	channel.sc = sc

	// Fan out N openHandle callers for the same fresh resource. Each blocks
	// inside sc.OpenTable on <-sc.release; the winner of the singleflight
	// insert is the one whose sc.OpenTable actually runs, the rest wait on
	// singleflight's internal chan and never call sc.OpenTable at all.
	res := adapters.Resource{
		Kind: adapters.ResourceTable,
		Name: "projects/p/instances/i/tables/t",
	}
	var wg sync.WaitGroup
	handles := make([]session.TableAPI, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handles[idx], errs[idx] = channel.openHandle(res)
		}(i)
	}

	// If singleflight is in place, only ONE goroutine reaches sc.OpenTable
	// — the barrier's arrived counter will never reach N=32, so
	// <-sc.entered would block forever. Release the barrier after a short
	// wait so the winner completes and the waiters can unblock.
	//
	// If singleflight is NOT in place (the bug we're guarding against), all
	// N goroutines reach sc.OpenTable, arrived hits N, and sc.entered fires
	// almost immediately. We wait for either signal, then release.
	select {
	case <-sc.entered:
		// Full burst arrived — this is the pre-singleflight regime.
	case <-time.After(500 * time.Millisecond):
		// Only the winner arrived — this is the with-singleflight regime.
	}
	close(sc.release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("openHandle #%d: unexpected error %v", i, err)
		}
		if handles[i] == nil {
			t.Fatalf("openHandle #%d: returned nil handle", i)
		}
	}
	// The core assertion: singleflight collapses the burst.
	if got := sc.openTableCalls.Load(); got != 1 {
		t.Fatalf("sc.OpenTable was invoked %d times; want 1 (singleflight should collapse the burst; without it, %d concurrent misses each call sc.OpenTable and %d-1 losers tear down the shared pool at the session layer)", got, N, N)
	}

	// Follow-up: subsequent openHandle calls should hit the fast path — no
	// new sc.OpenTable invocation.
	tbl, err := channel.openHandle(res)
	if err != nil {
		t.Fatalf("follow-up openHandle: %v", err)
	}
	if tbl == nil {
		t.Fatal("follow-up openHandle: returned nil handle")
	}
	if got := sc.openTableCalls.Load(); got != 1 {
		t.Fatalf("after fast-path openHandle, sc.OpenTable count = %d; want still 1 (fast path should not touch sc)", got)
	}
}
