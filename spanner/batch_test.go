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
	"strings"
	"sync"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	. "cloud.google.com/go/spanner/internal/testutil"
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
				unset := setQueryOptionsEnvVars(tt.env.Options)
				defer unset()
			}

			ctx := context.Background()
			_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, QueryOptions: tt.client})
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
				if got, want := p.qreq.QueryOptions.OptimizerStatisticsPackage, tt.want.Options.OptimizerStatisticsPackage; got != want {
					t.Fatalf("Incorrect optimizer statistics package: got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestPartitionQuery_ReadOptions(t *testing.T) {
	testcases := []ReadOptionsTestCase{
		{
			name:   "Client level",
			client: &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag"},
			// Index and Limit are always ignored
			want: &ReadOptions{Index: "", Limit: 0, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag"},
		},
		{
			name:   "Read level",
			client: &ReadOptions{},
			read:   &ReadOptions{Index: "testIndex", Limit: 100, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag"},
			// Index and Limit are always ignored
			want: &ReadOptions{Index: "", Limit: 0, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "testRequestTag"},
		},
		{
			name:   "Read level has precedence than client level",
			client: &ReadOptions{Index: "clientIndex", Limit: 10, Priority: sppb.RequestOptions_PRIORITY_LOW, RequestTag: "clientRequestTag"},
			read:   &ReadOptions{Index: "readIndex", Limit: 20, Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "readRequestTag"},
			// Index and Limit are always ignored
			want: &ReadOptions{Index: "", Limit: 0, Priority: sppb.RequestOptions_PRIORITY_MEDIUM, RequestTag: "readRequestTag"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ReadOptions: *tt.client})
			defer teardown()

			var (
				err error
				txn *BatchReadOnlyTransaction
				ps  []*Partition
			)

			if txn, err = client.BatchReadOnlyTransaction(ctx, StrongRead()); err != nil {
				t.Fatal(err)
			}
			defer txn.Cleanup(ctx)

			if tt.read == nil {
				ps, err = txn.PartitionRead(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{0, 3})
			} else {
				ps, err = txn.PartitionReadWithOptions(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{0, 3}, *tt.read)
			}
			if err != nil {
				t.Fatal(err)
			}

			for _, p := range ps {
				req := p.rreq
				if got, want := req.Index, tt.want.Index; got != want {
					t.Fatalf("Incorrect index: got %v, want %v", got, want)
				}
				if got, want := req.Limit, int64(tt.want.Limit); got != want {
					t.Fatalf("Incorrect limit: got %v, want %v", got, want)
				}

				ro := req.RequestOptions
				if got, want := ro.Priority, tt.want.Priority; got != want {
					t.Fatalf("Incorrect priority: got %v, want %v", got, want)
				}
				if got, want := ro.RequestTag, tt.want.RequestTag; got != want {
					t.Fatalf("Incorrect request tag: got %v, want %v", got, want)
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

func TestPartitionQuery_Multiplexed(t *testing.T) {
	if !isMultiplexEnabled {
		t.Skip("Skipping multiplex session tests when regular sessions enabled")
	}
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
	})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)

	// Test PartitionQuery
	paritions, err := txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{0, 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paritions {
		iter := txn.Execute(ctx, p)
		iter.Do(func(row *Row) error {
			return nil
		})
		break
	}
	uniqueReq := make(map[string]bool)
	handled := 0
	reqs := drainRequestsFromServer(server.TestSpanner)
	for _, s := range reqs {
		switch req := s.(type) {
		case *sppb.BeginTransactionRequest:
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionQuery_Multiplexed expected multiplexed session to be used, got: %v", req.Session)
			}
			if _, ok := uniqueReq["BeginTransactionRequest"]; !ok {
				handled++
			}
		case *sppb.ExecuteSqlRequest:
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionQuery_Multiplexed expected multiplexed session to be used with execute sql request, got: %v", req.Session)
			}
			if _, ok := uniqueReq["ExecuteSqlRequest"]; !ok {
				handled++
			}
		case *sppb.PartitionQueryRequest:
			// Validate the session is multiplexed
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionQuery_Multiplexed expected multiplexed session to be used with partition query request, got: %v", req.Session)
			}
			if _, ok := uniqueReq["PartitionQueryRequest"]; !ok {
				handled++
			}
		}
	}
	if handled != 3 {
		t.Errorf("TestPartitionQuery_Multiplexed: expected 3 requests to be handled, got: %d", handled)
	}
}

func TestPartitionRead_Multiplexed(t *testing.T) {
	if !isMultiplexEnabled {
		t.Skip("Skipping multiplex session tests when regular sessions enabled")
	}
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
	})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)

	// Test PartitionRead
	paritions, err := txn.PartitionRead(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{0, 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paritions {
		iter := txn.Execute(ctx, p)
		iter.Do(func(row *Row) error {
			return nil
		})
		break
	}
	reqs := drainRequestsFromServer(server.TestSpanner)
	uniqueReq := make(map[string]bool)
	handled := 0
	for _, s := range reqs {
		switch req := s.(type) {
		case *sppb.BeginTransactionRequest:
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionQuery_Multiplexed expected multiplexed session to be used, got: %v", req.Session)
			}
			if _, ok := uniqueReq["BeginTransactionRequest"]; !ok {
				handled++
			}
		case *sppb.ReadRequest:
			// Validate the session is multiplexed
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionRead_Multiplexed expected multiplexed session to be used with read request, got: %v", req.Session)
			}
			if _, ok := uniqueReq["ReadRequest"]; !ok {
				handled++
			}
		case *sppb.PartitionReadRequest:
			// Validate the session is multiplexed
			if !strings.Contains(req.Session, "multiplexed") {
				t.Errorf("TestPartitionRead_Multiplexed expected multiplexed session to be used with partition read request, got: %v", req.Session)
			}
			if _, ok := uniqueReq["PartitionReadRequest"]; !ok {
				handled++
			}
		}
	}
	if handled != 3 {
		t.Errorf("TestPartitionQuery_Multiplexed: expected 2 requests to be handled, got: %d", handled)
	}
}

func TestBatchExecute_Query_PreparesRoutingHint(t *testing.T) {
	t.Setenv(experimentalLocationAPIEnvVar, "true")

	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	if txn.locationRouter == nil {
		t.Fatal("expected location router to be enabled")
	}
	txn.locationRouter.observePartialResultSet(&sppb.PartialResultSet{
		CacheUpdate: &sppb.CacheUpdate{DatabaseId: 7},
	})

	partitions, err := txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{MaxPartitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least one partition")
	}
	if err := server.TestSpanner.PutPartitionResult(partitions[0].pt, server.CreateSingleRowSingersResult(0)); err != nil {
		t.Fatal(err)
	}

	iter := txn.Execute(ctx, partitions[0])
	defer iter.Stop()
	if err := iter.Do(func(*Row) error { return nil }); err != nil {
		t.Fatal(err)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	var executeReq *sppb.ExecuteSqlRequest
	for _, req := range requests {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && len(r.GetPartitionToken()) > 0 {
			executeReq = r
			break
		}
	}
	if executeReq == nil {
		t.Fatal("expected an ExecuteSqlRequest with partition token")
	}
	if executeReq.GetRoutingHint() == nil {
		t.Fatal("expected routing hint on ExecuteSqlRequest")
	}
	if got := executeReq.GetRoutingHint().GetDatabaseId(); got != 7 {
		t.Fatalf("unexpected routing hint database id: got %d, want 7", got)
	}
	if executeReq.GetRoutingHint().GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set on routing hint")
	}
}

func TestBatchExecute_Read_PreparesRoutingHint(t *testing.T) {
	t.Setenv(experimentalLocationAPIEnvVar, "true")

	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	if txn.locationRouter == nil {
		t.Fatal("expected location router to be enabled")
	}
	txn.locationRouter.observePartialResultSet(&sppb.PartialResultSet{
		CacheUpdate: &sppb.CacheUpdate{DatabaseId: 9},
	})

	partitions, err := txn.PartitionRead(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"}, PartitionOptions{MaxPartitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least one partition")
	}
	if err := server.TestSpanner.PutPartitionResult(partitions[0].pt, server.CreateSingleRowSingersResult(0)); err != nil {
		t.Fatal(err)
	}

	iter := txn.Execute(ctx, partitions[0])
	defer iter.Stop()
	if err := iter.Do(func(*Row) error { return nil }); err != nil {
		t.Fatal(err)
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	var readReq *sppb.ReadRequest
	for _, req := range requests {
		if r, ok := req.(*sppb.ReadRequest); ok && len(r.GetPartitionToken()) > 0 {
			readReq = r
			break
		}
	}
	if readReq == nil {
		t.Fatal("expected a ReadRequest with partition token")
	}
	if readReq.GetRoutingHint() == nil {
		t.Fatal("expected routing hint on ReadRequest")
	}
	if got := readReq.GetRoutingHint().GetDatabaseId(); got != 9 {
		t.Fatalf("unexpected routing hint database id: got %d, want 9", got)
	}
	if readReq.GetRoutingHint().GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set on routing hint")
	}
}
