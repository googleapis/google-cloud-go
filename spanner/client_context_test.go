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
	"context"
	"reflect"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// makeClientContext creates a RequestOptions_ClientContext from a map of strings.
func makeClientContext(kv map[string]string) *sppb.RequestOptions_ClientContext {
	if kv == nil {
		return nil
	}
	ctx := &sppb.RequestOptions_ClientContext{
		SecureContext: make(map[string]*structpb.Value),
	}
	for k, v := range kv {
		ctx.SecureContext[k] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: v}}
	}
	return ctx
}

func addSelect1Result(server *MockedSpannerInMemTestServer) {
	_ = server.TestSpanner.PutStatementResult("SELECT 1", &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				RowType: &sppb.StructType{
					Fields: []*sppb.StructType_Field{
						{Name: "Col1", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{{Kind: &structpb.Value_StringValue{StringValue: "1"}}}},
			},
		},
	})
}

func TestClientContext_Query(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := Statement{SQL: "SELECT 1"}
	addSelect1Result(server)
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	// 1. Test propagation via QueryOptions
	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: cc})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	foundReq := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := foundReq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch:\n Got: %v\nWant: %v", g, w)
	}
}

func TestDefaultClientContext_Query(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	stmt := Statement{SQL: "SELECT 1"}
	addSelect1Result(server)

	iter := client.Single().Query(ctx, stmt)
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	foundReq := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := foundReq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch:\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_Read(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	iter := client.Single().ReadWithOptions(ctx, "Table", KeySets(Key{"key1"}), []string{"Col1"}, &ReadOptions{ClientContext: cc})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ReadRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	foundReq := executeRequests[0].(*sppb.ReadRequest)
	if g, w := foundReq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch:\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_ReadWriteTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		return tx.BufferWrite([]*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})})
	}, TransactionOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	beginRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BeginTransactionRequest{}))
	commitRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.CommitRequest{}))
	if g, w := len(beginRequests), 1; g != w {
		t.Fatalf("num begin requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	beginRequest := beginRequests[0].(*sppb.BeginTransactionRequest)
	if g, w := beginRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
	if g, w := len(commitRequests), 1; g != w {
		t.Fatalf("num commit requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	commitRequest := commitRequests[0].(*sppb.CommitRequest)
	if g, w := commitRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_BatchWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	iter := client.BatchWriteWithOptions(ctx, []*MutationGroup{
		{Mutations: []*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})}},
	}, BatchWriteOptions{ClientContext: cc})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	batchRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BatchWriteRequest{}))
	if g, w := len(batchRequests), 1; g != w {
		t.Fatalf("num BatchWrite requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	batchRequest := batchRequests[0].(*sppb.BatchWriteRequest)
	if g, w := batchRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_Merging(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stmt := Statement{SQL: "SELECT 1"}

	defaultKV := map[string]string{"key1": "default_value1", "key2": "default_value2"}
	requestKV := map[string]string{"key2": "request_value2", "key3": "request_value3"}
	expectedKV := map[string]string{
		"key1": "default_value1",
		"key2": "request_value2",
		"key3": "request_value3",
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        makeClientContext(defaultKV),
		DisableNativeMetrics: true,
	})
	defer teardown()
	addSelect1Result(server)

	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: makeClientContext(requestKV)})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, makeClientContext(expectedKV); !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_Hierarchy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stmt := Statement{SQL: "SELECT 1"}

	// Hierarchy:
	// 1. Client Global Context (least specific)
	// 2. Transaction Global Context
	// 3. Client Query Options Context
	// 4. Per-query Context (most specific)

	clientGlobal := map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4"}
	txGlobal := map[string]string{"k2": "tx-v2", "k3": "tx-v3", "k4": "tx-v4"}
	clientQuery := map[string]string{"k3": "cq-v3", "k4": "cq-v4"}
	perQuery := map[string]string{"k4": "pq-v4"}

	// Expected order: 4 > 3 > 2 > 1
	expectedKV := map[string]string{
		"k1": "v1",
		"k2": "tx-v2",
		"k3": "cq-v3",
		"k4": "pq-v4",
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        makeClientContext(clientGlobal),
		QueryOptions:         QueryOptions{ClientContext: makeClientContext(clientQuery)},
		DisableNativeMetrics: true,
	})
	defer teardown()
	addSelect1Result(server)

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: makeClientContext(perQuery)})
		if _, err := iter.Next(); err != nil && err != iterator.Done {
			return err
		}
		iter.Stop()
		return nil
	}, TransactionOptions{ClientContext: makeClientContext(txGlobal)})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, makeClientContext(expectedKV); !proto.Equal(g, w) {
		t.Fatalf("Hierarchical ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_PDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := Statement{SQL: "UPDATE Table SET Col1=1 WHERE 1=1"}
	_ = server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	_, err := client.PartitionedUpdateWithOptions(ctx, stmt, QueryOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	beginRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BeginTransactionRequest{}))
	if g, w := len(beginRequests), 1; g != w {
		t.Fatalf("num begin requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	beginRequest := beginRequests[0].(*sppb.BeginTransactionRequest)
	if g, w := beginRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch for Begin request\n Got: %v\nWant: %v", g, w)
	}
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("PDML ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_BatchReadOnlyTransaction_Query(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	addSelect1Result(server)

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	stmt := Statement{SQL: "SELECT 1"}

	// Test PartitionQuery
	partitions, err := txn.PartitionQueryWithOptions(ctx, stmt, PartitionOptions{MaxPartitions: 1}, QueryOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := len(partitions), 1; g != w {
		t.Fatalf("num partitions mismatch\n Got: %d\nWant: %d", g, w)
	}
	if g, w := partitions[0].qreq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("mismatch in Partition.qreq ClientContext\nGot: %v\nWant: %v", g, w)
	}
	iter := txn.Execute(ctx, partitions[0])
	// This fails, but that does not matter. We are only interested in inspecting the request that is sent.
	_, _ = iter.Next()
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_BatchReadOnlyTransaction_Read(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	addSelect1Result(server)

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	// Test PartitionRead
	partitions, err := txn.PartitionReadWithOptions(ctx, "Table", KeySets(Key{"key1"}), []string{"Col1"}, PartitionOptions{MaxPartitions: 1}, ReadOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := len(partitions), 1; g != w {
		t.Fatalf("num partitions mismatch\n Got: %d\nWant: %d", g, w)
	}
	if g, w := partitions[0].rreq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("mismatch in Partition.rreq ClientContext\nGot: %v\nWant: %v", g, w)
	}
	iter := txn.Execute(ctx, partitions[0])
	// This fails, but that does not matter. We are only interested in inspecting the request that is sent.
	_, _ = iter.Next()
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	readRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ReadRequest{}))
	if g, w := len(readRequests), 1; g != w {
		t.Fatalf("num read requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	readRequest := readRequests[0].(*sppb.ReadRequest)
	if g, w := readRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Fatalf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_ApplyAtLeastOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	_, err := client.Apply(ctx, []*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})}, ApplyAtLeastOnce())
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	commitRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.CommitRequest{}))
	if g, w := len(commitRequests), 1; g != w {
		t.Fatalf("num commit requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	commitRequest := commitRequests[0].(*sppb.CommitRequest)
	if g, w := commitRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_EmptyMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	addSelect1Result(server)

	cc := &sppb.RequestOptions_ClientContext{
		SecureContext: make(map[string]*structpb.Value),
	}
	stmt := Statement{SQL: "SELECT 1"}

	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: cc})
	if _, err := iter.Next(); err != nil && err != iterator.Done {
		t.Fatal(err)
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_PDML_Default(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	stmt := Statement{SQL: "UPDATE Table SET Col1=1 WHERE 1=1"}
	_ = server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	_, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	beginRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BeginTransactionRequest{}))
	if g, w := len(beginRequests), 1; g != w {
		t.Fatalf("num begin requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	beginRequest := beginRequests[0].(*sppb.BeginTransactionRequest)
	if g, w := beginRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_Batch_Default(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()
	addSelect1Result(server)

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	stmt := Statement{SQL: "SELECT 1"}

	// Test PartitionQuery (default)
	partitions, err := txn.PartitionQuery(ctx, stmt, PartitionOptions{MaxPartitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := len(partitions), 1; g != w {
		t.Fatalf("num partitions mismatch\n Got: %d\nWant: %d", g, w)
	}
	if g, w := partitions[0].qreq.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}

	iter := txn.Execute(ctx, partitions[0])
	_, _ = iter.Next()
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	beginRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BeginTransactionRequest{}))
	if g, w := len(beginRequests), 1; g != w {
		t.Fatalf("num begin requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	beginRequest := beginRequests[0].(*sppb.BeginTransactionRequest)
	if g, w := beginRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
	executeRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
	if g, w := len(executeRequests), 1; g != w {
		t.Fatalf("num execute requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	executeRequest := executeRequests[0].(*sppb.ExecuteSqlRequest)
	if g, w := executeRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestClientContext_BeginTransaction_Default(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	// Use a statement-based transaction to force an explicit BeginTransaction
	txn, err := NewReadWriteStmtBasedTransaction(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback(ctx)

	reqs := drainRequestsFromServer(server.TestSpanner)
	beginRequests := requestsOfType(reqs, reflect.TypeOf(&sppb.BeginTransactionRequest{}))
	if g, w := len(beginRequests), 1; g != w {
		t.Fatalf("num begin requests mismatch\n Got: %d\nWant: %d", g, w)
	}
	beginRequest := beginRequests[0].(*sppb.BeginTransactionRequest)
	if g, w := beginRequest.RequestOptions.ClientContext, cc; !proto.Equal(g, w) {
		t.Errorf("ClientContext mismatch\n Got: %v\nWant: %v", g, w)
	}
}
