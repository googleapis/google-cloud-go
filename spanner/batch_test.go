/*
Copyright 2018 Google LLC

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
	"context"
	"os"
	"sync"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func TestPartitionRoundTrip(t *testing.T) {
	t.Parallel()
	for i, want := range []Partition{
		{rreq: &sppb.ReadRequest{Table: "t"}},
		{qreq: &sppb.ExecuteSqlRequest{Sql: "sql"}},
	} {
		got := serdesPartition(t, i, &want)
		if !testEqual(got, want) {
			t.Errorf("got: %#v\nwant:%#v", got, want)
		}
	}
}

func TestBROTIDRoundTrip(t *testing.T) {
	t.Parallel()
	tm := time.Now()
	want := BatchReadOnlyTransactionID{
		tid: []byte("tid"),
		sid: "sid",
		rts: tm,
	}
	data, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var got BatchReadOnlyTransactionID
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatal(err)
	}
	if !testEqual(got, want) {
		t.Errorf("got: %#v\nwant:%#v", got, want)
	}
}

// serdesPartition is a helper that serialize a Partition then deserialize it.
func serdesPartition(t *testing.T, i int, p1 *Partition) (p2 Partition) {
	var (
		data []byte
		err  error
	)
	if data, err = p1.MarshalBinary(); err != nil {
		t.Fatalf("#%d: encoding failed %v", i, err)
	}
	if err = p2.UnmarshalBinary(data); err != nil {
		t.Fatalf("#%d: decoding failed %v", i, err)
	}
	return p2
}

func TestPartitionQuery_QueryOptions(t *testing.T) {
	for _, tt := range queryOptionsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env.Options != nil {
				os.Setenv("SPANNER_OPTIMIZER_VERSION", tt.env.Options.OptimizerVersion)
				defer os.Setenv("SPANNER_OPTIMIZER_VERSION", "")
			}

			ctx := context.Background()
			_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{QueryOptions: tt.client})
			defer teardown()

			var (
				err  error
				txn  *BatchReadOnlyTransaction
				ps   []*Partition
				stmt = NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)
			)

			if txn, err = client.BatchReadOnlyTransaction(ctx, StrongRead()); err != nil {
				t.Fatal(err)
			}
			defer txn.Cleanup(ctx)

			if tt.query.Options == nil {
				ps, err = txn.PartitionQuery(ctx, stmt, PartitionOptions{0, 3})
			} else {
				ps, err = txn.PartitionQueryWithOptions(ctx, stmt, PartitionOptions{0, 3}, tt.query)
			}
			if err != nil {
				t.Fatal(err)
			}

			for _, p := range ps {
				if got, want := p.qreq.QueryOptions.OptimizerVersion, tt.want.Options.OptimizerVersion; got != want {
					t.Fatalf("Incorrect optimizer version: got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestPartitionQuery_Parallel(t *testing.T) {
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	ps, err := txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{0, 10})
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range ps {
		server.TestSpanner.PutPartitionResult(p.pt, server.CreateSingleRowSingersResult(int64(i)))
	}

	wg := &sync.WaitGroup{}
	mu := sync.Mutex{}
	var total int64

	for _, p := range ps {
		p := p
		go func() {
			iter := txn.Execute(context.Background(), p)
			defer iter.Stop()

			var count int64
			err := iter.Do(func(row *Row) error {
				count++
				return nil
			})
			if err != nil {
				return
			}

			mu.Lock()
			total += count
			mu.Unlock()
			wg.Done()
		}()
		wg.Add(1)
	}

	wg.Wait()
	if g, w := total, SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount; g != w {
		t.Errorf("Row count mismatch\nGot: %d\nWant: %d", g, w)
	}
}
