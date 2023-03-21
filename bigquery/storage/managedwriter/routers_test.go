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
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
)

func TestSimpleRouter(t *testing.T) {

	ctx := context.Background()

	pool := &connectionPool{
		ctx: ctx,
		open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
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
		open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
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

func BenchmarkRouting(b *testing.B) {

	for _, bm := range []struct {
		desc                string
		router              poolRouter
		numWriters          int
		numMultiplexWriters int
	}{
		{
			desc:                "SimpleRouter",
			router:              newSimpleRouter(""),
			numWriters:          1,
			numMultiplexWriters: 1,
		},
		{
			desc:                "SimpleRouter",
			router:              newSimpleRouter(""),
			numWriters:          10,
			numMultiplexWriters: 10,
		},
		{
			desc:                "SharedRouterNoMultiplex",
			router:              newSharedRouter(false, 0),
			numWriters:          1,
			numMultiplexWriters: 1,
		},
		{
			desc:                "SharedRouterNoMultiplex",
			router:              newSharedRouter(false, 0),
			numWriters:          10,
			numMultiplexWriters: 10,
		},
		{
			desc:                "SharedRouterMultiplex1conn",
			router:              newSharedRouter(true, 1),
			numWriters:          1,
			numMultiplexWriters: 1,
		},
		{
			desc:                "SharedRouterMultiplex1conn",
			router:              newSharedRouter(true, 1),
			numWriters:          10,
			numMultiplexWriters: 10,
		},
		{
			desc:                "SharedRouterMultiplex1conn",
			router:              newSharedRouter(true, 1),
			numWriters:          50,
			numMultiplexWriters: 50,
		},
		{
			desc:                "SharedRouterMultiplex10conn",
			router:              newSharedRouter(true, 10),
			numWriters:          50,
			numMultiplexWriters: 50,
		},
	} {

		ctx, cancel := context.WithCancel(context.Background())
		pool := &connectionPool{
			ctx:    ctx,
			cancel: cancel,
			open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
				return &testAppendRowsClient{}, nil
			},
		}
		if err := pool.activateRouter(bm.router); err != nil {
			b.Errorf("%q: activateRouter: %v", bm.desc, err)
		}

		// setup both explicit and default stream writers.
		var writers []*ManagedStream
		for i := 0; i < bm.numWriters; i++ {
			wCtx, wCancel := context.WithCancel(ctx)
			writer := &ManagedStream{
				id:             newUUID(writerIDPrefix),
				streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/abc123"},
				ctx:            wCtx,
				cancel:         wCancel,
				retry:          newStatelessRetryer(),
			}
			writers = append(writers, writer)
		}
		for i := 0; i < bm.numMultiplexWriters; i++ {
			wCtx, wCancel := context.WithCancel(ctx)
			writer := &ManagedStream{
				id:             newUUID(writerIDPrefix),
				streamSettings: &streamSettings{streamID: "projects/foo/datasets/bar/tables/baz/streams/_default"},

				ctx:    wCtx,
				cancel: wCancel,
				retry:  newStatelessRetryer(),
			}
			writers = append(writers, writer)
		}

		for k, writer := range writers {
			if err := pool.addWriter(writer); err != nil {
				b.Errorf("addWriter %d: %v", k, err)
			}
		}
		b.Logf("%d writers %d %d", len(writers), bm.numWriters, bm.numMultiplexWriters)

		benchName := fmt.Sprintf("%s_%dexwriters_%dmpwriters", bm.desc, bm.numWriters, bm.numMultiplexWriters)
		b.Log(benchName)
		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// route each writer once
				writer := writers[0]
				pw := newPendingWrite(context.Background(), writer, &storagepb.AppendRowsRequest{}, nil, "", "")
				if _, err := bm.router.pickConnection(pw); err != nil {
					b.Errorf("pickConnection: %v", err)
				}
			}
		})

		for _, writer := range writers {
			writer.Close()
		}
		pool.Close()

	}

}
