// Copyright 2023 Google LLC
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
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
)

func TestSimpleRouter(t *testing.T) {
	ctx := context.Background()

	pool := &connectionPool{
		ctx: ctx,
		open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
			return &testAppendRowsClient{}, nil
		},
	}

	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		ctx:   ctx,
		retry: newStatelessRetryer(),
	}

	pw := newPendingWrite(ctx, ms, &storagepb.AppendRowsRequest{}, nil, "", "")

	// picking before attaching should yield error
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
	writer := &ManagedStream{
		id: "writer",
	}
	if err := pool.addWriter(writer); err != nil {
		t.Errorf("addWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err != nil {
		t.Errorf("pickConnection error: %v", err)
	}
	if err := pool.removeWriter(writer); err != nil {
		t.Errorf("disconnectWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
}

func TestSharedRouter_Basic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &connectionPool{
		ctx:    ctx,
		cancel: cancel,
		open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
			return &testAppendRowsClient{}, nil
		},
	}

	router := newSharedRouter(false, 0)
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}
	if gotConns := len(router.exclusiveConns); gotConns != 0 {
		t.Errorf("expected zero connections are start, got %d", gotConns)
	}

	ms := &ManagedStream{
		ctx:   ctx,
		retry: newStatelessRetryer(),
	}
	pw := newPendingWrite(ctx, ms, &storagepb.AppendRowsRequest{}, nil, "", "")
	// picking before attaching should yield error
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
	// attaching a writer without an ID should error.
	if err := pool.addWriter(ms); err == nil {
		t.Errorf("expected id-less addWriter to fail")
	}
	ms.id = "writer"
	if err := pool.addWriter(ms); err != nil {
		t.Errorf("addWriter: %v", err)
	}

	if _, err := pool.router.pickConnection(pw); err != nil {
		t.Errorf("pickConnection error: %v", err)
	}
	if err := pool.removeWriter(ms); err != nil {
		t.Errorf("disconnectWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
}

func TestSharedRouter_Multiplex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &connectionPool{
		id:     newUUID(poolIDPrefix),
		ctx:    ctx,
		cancel: cancel,
		open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
			return &testAppendRowsClient{}, nil
		},
		baseFlowController: newFlowController(2, 10),
	}
	defer pool.Close()

	router := newSharedRouter(true, 3)
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	wantConnCount := 0
	if got := len(router.multiConns); wantConnCount != got {
		t.Errorf("wanted %d conns, got %d", wantConnCount, got)
	}

	writerA := &ManagedStream{
		id:             newUUID(writerIDPrefix),
		streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},
		ctx:            ctx,
		cancel:         cancel,
	}
	if err := pool.router.writerAttach(writerA); err != nil {
		t.Fatalf("writerA attach: %v", err)
	}

	// after a writer attached, we expect one conn.
	wantConnCount = 1
	if got := len(router.multiConns); wantConnCount != got {
		t.Errorf("wanted %d conns, got %d", wantConnCount, got)
	}

	writerB := &ManagedStream{
		id:             newUUID(writerIDPrefix),
		streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},
		ctx:            ctx,
		cancel:         cancel,
	}
	if err := pool.router.writerAttach(writerB); err != nil {
		t.Fatalf("writerA attach: %v", err)
	}
	writerC := &ManagedStream{
		id:             newUUID(writerIDPrefix),
		streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},
		ctx:            ctx,
		cancel:         cancel,
	}
	if err := pool.router.writerAttach(writerC); err != nil {
		t.Fatalf("writerA attach: %v", err)
	}

	wantConnCount = 1
	if got := len(router.multiConns); wantConnCount != got {
		t.Fatalf("wanted %d conns, got %d", wantConnCount, got)
	}

	pw := newPendingWrite(ctx, writerA, &storagepb.AppendRowsRequest{}, nil, "", "")
	conn, err := router.pickConnection(pw)
	if err != nil {
		t.Fatalf("pickConnection writerA: %v", err)
	}
	// generate fake load on the conn associated with writer A
	conn.fc.acquire(ctx, 1)
	conn.fc.acquire(ctx, 1)

	if !conn.isLoaded() {
		t.Errorf("expected conn to be loaded, was not")
	}
	// wait for a watchdog interval
	time.Sleep(watchDogInterval * 2)

	wantConnCount = 2
	// grab read lock so we can assert internal state of the router
	router.multiMu.RLock()
	defer router.multiMu.RUnlock()
	if got := len(router.multiConns); wantConnCount != got {
		t.Fatalf("wanted %d conns, got %d", wantConnCount, got)
	}
	gotLoad0 := router.multiConns[0].curLoad()
	gotLoad1 := router.multiConns[1].curLoad()
	if gotLoad0 > gotLoad1 {
		t.Errorf("expected connections to be ordered by load, got %f, %f", gotLoad0, gotLoad1)
	}
	// verify that rebalance occurred
	connsWithWriters := 0
	for _, v := range router.invertedMultiMap {
		if len(v) > 0 {
			connsWithWriters++
		}
	}
	if connsWithWriters < wantConnCount {
		t.Errorf("wanted at least %d connections to have writers attached, got %d", wantConnCount, connsWithWriters)
	}

}

func BenchmarkRoutingParallel(b *testing.B) {

	for _, bm := range []struct {
		desc              string
		router            poolRouter
		numWriters        int
		numDefaultWriters int
	}{
		{
			desc:              "SimpleRouter",
			router:            newSimpleRouter(""),
			numWriters:        1,
			numDefaultWriters: 1,
		},
		{
			desc:              "SimpleRouter",
			router:            newSimpleRouter(""),
			numWriters:        10,
			numDefaultWriters: 10,
		},
		{
			desc:              "SharedRouter_NoMultiplex",
			router:            newSharedRouter(false, 0),
			numWriters:        1,
			numDefaultWriters: 1,
		},
		{
			desc:              "SharedRouter_NoMultiplex",
			router:            newSharedRouter(false, 0),
			numWriters:        10,
			numDefaultWriters: 10,
		},
		{
			desc:              "SharedRouter_Multiplex1conn",
			router:            newSharedRouter(true, 1),
			numWriters:        1,
			numDefaultWriters: 1,
		},
		{
			desc:              "SharedRouterMultiplex1conn",
			router:            newSharedRouter(true, 1),
			numWriters:        10,
			numDefaultWriters: 10,
		},
		{
			desc:              "SharedRouterMultiplex1conn",
			router:            newSharedRouter(true, 1),
			numWriters:        50,
			numDefaultWriters: 50,
		},
		{
			desc:              "SharedRouterMultiplex10conn",
			router:            newSharedRouter(true, 10),
			numWriters:        50,
			numDefaultWriters: 50,
		},
	} {

		ctx, cancel := context.WithCancel(context.Background())
		pool := &connectionPool{
			ctx:    ctx,
			cancel: cancel,
			open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				return &testAppendRowsClient{}, nil
			},
		}
		if err := pool.activateRouter(bm.router); err != nil {
			b.Errorf("%q: activateRouter: %v", bm.desc, err)
		}

		// setup both explicit and default stream writers.
		var explicitWriters []*ManagedStream
		var defaultWriters []*ManagedStream

		for i := 0; i < bm.numWriters; i++ {
			wCtx, wCancel := context.WithCancel(ctx)
			writer := &ManagedStream{
				id:             newUUID(writerIDPrefix),
				streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/abc123"},
				ctx:            wCtx,
				cancel:         wCancel,
				retry:          newStatelessRetryer(),
			}
			explicitWriters = append(explicitWriters, writer)
		}
		for i := 0; i < bm.numDefaultWriters; i++ {
			wCtx, wCancel := context.WithCancel(ctx)
			writer := &ManagedStream{
				id:             newUUID(writerIDPrefix),
				streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},

				ctx:    wCtx,
				cancel: wCancel,
				retry:  newStatelessRetryer(),
			}
			defaultWriters = append(defaultWriters, writer)
		}

		// attach all writers to router.
		for k, writer := range explicitWriters {
			if err := pool.addWriter(writer); err != nil {
				b.Errorf("addWriter %d: %v", k, err)
			}
		}
		for k, writer := range defaultWriters {
			if err := pool.addWriter(writer); err != nil {
				b.Errorf("addWriter %d: %v", k, err)
			}
		}

		baseBenchName := fmt.Sprintf("%s_%dexwriters_%dmpwriters", bm.desc, bm.numWriters, bm.numDefaultWriters)

		// Benchmark routing for explicit writers.
		if bm.numWriters > 0 {
			benchName := fmt.Sprintf("%s_explicitwriters", baseBenchName)

			b.Run(benchName, func(b *testing.B) {
				r := rand.New(rand.NewSource(1))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// pick a random explicit writer each time.
					writer := explicitWriters[r.Intn(bm.numWriters)]
					pw := newPendingWrite(context.Background(), writer, &storagepb.AppendRowsRequest{}, nil, "", "")
					if _, err := bm.router.pickConnection(pw); err != nil {
						b.Errorf("pickConnection: %v", err)
					}
				}
			})
		}

		// Benchmark concurrent routing for explicit writers.
		if bm.numWriters > 0 {
			benchName := fmt.Sprintf("%s_explicitwriters_concurrent", baseBenchName)
			b.Run(benchName, func(b *testing.B) {
				b.RunParallel(func(pb *testing.PB) {
					r := rand.New(rand.NewSource(1))
					for pb.Next() {
						writer := explicitWriters[r.Intn(bm.numWriters)]
						pw := newPendingWrite(context.Background(), writer, &storagepb.AppendRowsRequest{}, nil, "", "")
						if _, err := bm.router.pickConnection(pw); err != nil {
							b.Errorf("pickConnection: %v", err)
						}
					}
				})
			})
		}

		// Benchmark routing for default writers.
		if bm.numDefaultWriters > 0 {
			benchName := fmt.Sprintf("%s_defaultwriters", baseBenchName)

			b.Run(benchName, func(b *testing.B) {
				r := rand.New(rand.NewSource(1))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// pick a random default writer each time.
					writer := defaultWriters[r.Intn(bm.numDefaultWriters)]
					pw := newPendingWrite(context.Background(), writer, &storagepb.AppendRowsRequest{}, nil, "", "")
					if _, err := bm.router.pickConnection(pw); err != nil {
						b.Errorf("pickConnection: %v", err)
					}
				}
			})
		}

		// Benchmark concurrent routing for default writers.
		if bm.numDefaultWriters > 0 {
			benchName := fmt.Sprintf("%s_defaultwriters_concurrent", baseBenchName)

			b.Run(benchName, func(b *testing.B) {
				b.RunParallel(func(pb *testing.PB) {
					r := rand.New(rand.NewSource(1))
					for pb.Next() {
						writer := defaultWriters[r.Intn(bm.numDefaultWriters)]
						pw := newPendingWrite(context.Background(), writer, &storagepb.AppendRowsRequest{}, nil, "", "")
						if _, err := bm.router.pickConnection(pw); err != nil {
							b.Errorf("pickConnection: %v", err)
						}
					}
				})
			})
		}

		for _, writer := range explicitWriters {
			writer.Close()
		}
		for _, writer := range defaultWriters {
			writer.Close()
		}

		pool.Close()

	}

}

func BenchmarkWatchdogPulse(b *testing.B) {
	maxFlowInserts := 100
	maxFlowBytes := 1024
	for _, numWriters := range []int{1, 2, 5, 10, 50, 100, 250} {
		for _, numConnections := range []int{1, 2, 4} {

			ctx, cancel := context.WithCancel(context.Background())
			// we build the router manually so we can control the watchdog for this benchmark.
			router := newSharedRouter(false, numConnections)

			pool := &connectionPool{
				ctx:    ctx,
				cancel: cancel,
				open: func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
					return &testAppendRowsClient{}, nil
				},
				baseFlowController: newFlowController(maxFlowInserts, maxFlowBytes),
			}
			if err := pool.activateRouter(router); err != nil {
				b.Fatalf("(@%d-@%d): activateRouter: %v", numWriters, numConnections, err)
			}
			// now, set router as multiplex.  We do this to avoid router activation starting the watchdog
			// in a seperate goroutine.
			router.multiplex = true

			var writers []*ManagedStream

			for i := 0; i < numWriters; i++ {
				wCtx, wCancel := context.WithCancel(ctx)
				writer := &ManagedStream{
					id:             newUUID(writerIDPrefix),
					streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},

					ctx:    wCtx,
					cancel: wCancel,
					retry:  newStatelessRetryer(),
				}
				writers = append(writers, writer)
				if err := pool.addWriter(writer); err != nil {
					b.Fatalf("addWriter %d (@%d-@%d): %v", i, numWriters, numConnections, err)
				}
			}

			// Generate fake load for all connections.
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			countLoad := make([]int, numConnections)
			byteLoad := make([]int, numConnections)
			for i := 0; i < numConnections; i++ {
				countLoad[i] = r.Intn(maxFlowInserts)
				byteLoad[i] = r.Intn(maxFlowBytes)
			}

			benchName := fmt.Sprintf("%dwriters_%dconns", numWriters, numConnections)
			b.Run(benchName, func(b *testing.B) {
				if b.N > 9999 {
					b.Skip("benchmark unstable, only run with -benchtime=NNNNx")
				}
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					// Each iteration, we reset the loads to the predetermined values, and repoint
					// all writers to the first connection.
					for c := 0; c < len(router.multiConns); c++ {
						router.multiConns[c].fc.countTracked = int64(countLoad[c])
						router.multiConns[c].fc.bytesTracked = int64(byteLoad[c])
					}
					for k := range router.multiMap {
						router.multiMap[k] = router.multiConns[0]
					}
					router.invertedMultiMap = make(map[string][]*ManagedStream)
					writerSlice := make([]*ManagedStream, len(writers))
					copy(writerSlice, writers)
					router.invertedMultiMap[router.multiConns[0].id] = writerSlice
					b.StartTimer()
					router.watchdogPulse()
				}
			})

			for _, writer := range writers {
				writer.Close()
			}

			pool.Close()

		}
	}

}
