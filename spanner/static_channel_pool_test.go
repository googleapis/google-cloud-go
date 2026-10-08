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

package spanner

import (
	"context"
	"sync/atomic"
	"testing"

	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
)

const (
	executeSQLMethod = "/google.spanner.v1.Spanner/ExecuteSql"
	commitMethod     = "/google.spanner.v1.Spanner/Commit"
)

// runReadWriteTransactionsOnChannels runs n read-write transactions of two
// statements each and returns the channel ids each transaction used, in order.
func runReadWriteTransactionsOnChannels(t *testing.T, client *Client, recorder *dcpChannelRecorder, n int) [][]uint64 {
	t.Helper()
	var txns [][]uint64
	for i := 0; i < n; i++ {
		before := len(recorder.recorded())
		_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
			for j := 0; j < 2; j++ {
				if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("ReadWriteTransaction() failed: %v", err)
		}
		var ids []uint64
		for _, c := range recorder.recorded()[before:] {
			ids = append(ids, c.channelID)
		}
		txns = append(txns, ids)
	}
	return txns
}

func TestStaticChannelPoolBindsTransactionsToSlots(t *testing.T) {
	skipDialectRerun(t)
	recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
	_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{
		DisableNativeMetrics: true,
		NumChannels:          4,
	}, []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
	})
	defer teardown()

	pool, ok := client.sc.connPool.(*staticChannelPool)
	if !ok {
		t.Fatalf("connection pool type = %T, want *staticChannelPool", client.sc.connPool)
	}
	if got, want := pool.Num(), 4; got != want {
		t.Fatalf("slot count = %d, want %d", got, want)
	}

	used := map[uint64]int{}
	for i, ids := range runReadWriteTransactionsOnChannels(t, client, recorder, 8) {
		if len(ids) != 3 {
			t.Fatalf("transaction %d recorded %d RPCs, want 3 (%v)", i, len(ids), ids)
		}
		for _, id := range ids {
			if id != ids[0] {
				t.Fatalf("transaction %d used channels %v, want one channel", i, ids)
			}
		}
		if ids[0] < 1 || ids[0] > 4 {
			t.Fatalf("transaction %d channel id = %d, want a slot id in [1, 4]", i, ids[0])
		}
		used[ids[0]]++
	}
	if len(used) != 4 {
		t.Fatalf("transactions used slots %v, want all 4 slots", used)
	}
}

func TestStaticChannelPoolWithCallerSuppliedConnection(t *testing.T) {
	skipDialectRerun(t)
	server, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	conn, err := gtransport.Dial(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got, want := client.sc.connPool.Num(), 1; got != want {
		t.Fatalf("slot count with a caller-supplied connection = %d, want %d", got, want)
	}
	iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
	defer iter.Stop()
	if err := iter.Do(func(*Row) error { return nil }); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	_ = server
}

// closeCountingConnPool wraps a dialed pool and counts Close calls.
type closeCountingConnPool struct {
	gtransport.ConnPool
	closes atomic.Int32
}

func (p *closeCountingConnPool) Close() error {
	p.closes.Add(1)
	return p.ConnPool.Close()
}

func TestStaticChannelPoolClosesDialedPoolOnce(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	dialed, err := gtransport.DialPool(context.Background(), append(opts, option.WithGRPCConnectionPool(3))...)
	if err != nil {
		t.Fatal(err)
	}
	direct := &closeCountingConnPool{ConnPool: dialed}
	pool, err := newStaticChannelPool(direct, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[*grpc.ClientConn]bool{}
	for i, s := range pool.slots {
		if s.id != uint64(i+1) {
			t.Fatalf("slot %d id = %d, want %d", i, s.id, i+1)
		}
		seen[s.direct.Conn()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("slots share connections: %d distinct connections for 3 slots", len(seen))
	}
	pool.Close()
	pool.Close()
	if got := direct.closes.Load(); got != 1 {
		t.Fatalf("dialed pool closes = %d, want 1", got)
	}
}
