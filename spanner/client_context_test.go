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

func TestClientContext_Query(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := Statement{SQL: "SELECT 1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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
	var foundReq *sppb.ExecuteSqlRequest
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok && sqlReq.Sql == stmt.SQL {
			foundReq = sqlReq
			break
		}
	}
	if foundReq == nil {
		t.Fatal("ExecuteSqlRequest not found")
	}
	if !proto.Equal(foundReq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", foundReq.RequestOptions.ClientContext, cc)
	}

	// 2. Test propagation via ClientConfig default
	server2, clientWithDefault, teardown2 := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown2()

	server2.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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

	iter = clientWithDefault.Single().Query(ctx, stmt)
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

	reqs = drainRequestsFromServer(server2.TestSpanner)
	found := false
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			if !proto.Equal(r.RequestOptions.ClientContext, cc) {
				t.Errorf("mismatch in ClientContext (default):\ngot:  %v\nwant: %v", r.RequestOptions.ClientContext, cc)
			}
			found = true
		}
	}
	if !found {
		t.Error("ExecuteSqlRequest not found for stmt")
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
	var foundReq *sppb.ReadRequest
	for _, req := range reqs {
		if readReq, ok := req.(*sppb.ReadRequest); ok {
			foundReq = readReq
			break
		}
	}
	if foundReq == nil {
		t.Fatal("ReadRequest not found")
	}
	if !proto.Equal(foundReq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", foundReq.RequestOptions.ClientContext, cc)
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
	var gotBegin *sppb.BeginTransactionRequest
	var gotCommit *sppb.CommitRequest
	for _, req := range reqs {
		switch r := req.(type) {
		case *sppb.BeginTransactionRequest:
			gotBegin = r
		case *sppb.CommitRequest:
			gotCommit = r
		}
	}

	if gotBegin == nil {
		t.Fatal("expected BeginTransactionRequest")
	}
	if !proto.Equal(gotBegin.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in BeginTransaction ClientContext:\ngot:  %v\nwant: %v", gotBegin.RequestOptions.ClientContext, cc)
	}

	if gotCommit == nil {
		t.Fatal("expected CommitRequest")
	}
	if !proto.Equal(gotCommit.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in Commit ClientContext:\ngot:  %v\nwant: %v", gotCommit.RequestOptions.ClientContext, cc)
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
	var gotReq *sppb.BatchWriteRequest
	for _, req := range reqs {
		if bwReq, ok := req.(*sppb.BatchWriteRequest); ok {
			gotReq = bwReq
			break
		}
	}

	if gotReq == nil {
		t.Fatal("expected BatchWriteRequest")
	}
	if !proto.Equal(gotReq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, cc)
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

	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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
	var foundReq *sppb.ExecuteSqlRequest
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok && sqlReq.Sql == stmt.SQL {
			foundReq = sqlReq
			break
		}
	}
	if foundReq == nil {
		t.Fatal("ExecuteSqlRequest not found")
	}
	expected := makeClientContext(expectedKV)
	if !proto.Equal(foundReq.RequestOptions.ClientContext, expected) {
		t.Errorf("mismatch in Merged ClientContext:\ngot:  %v\nwant: %v", foundReq.RequestOptions.ClientContext, expected)
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

	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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
	found := false
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			found = true
			expected := makeClientContext(expectedKV)
			if !proto.Equal(r.RequestOptions.ClientContext, expected) {
				t.Errorf("mismatch in hierarchical ClientContext:\ngot:  %v\nwant: %v", r.RequestOptions.ClientContext, expected)
			}
		}
	}
	if !found {
		t.Error("ExecuteSqlRequest not found")
	}
}

func TestClientContext_PDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := Statement{SQL: "UPDATE Table SET Col1=1 WHERE 1=1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	_, err := client.PartitionedUpdateWithOptions(ctx, stmt, QueryOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	var sqlReq *sppb.ExecuteSqlRequest
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			sqlReq = r
			break
		}
	}
	if sqlReq == nil {
		t.Fatal("ExecuteSqlRequest for PDML not found")
	}
	if !proto.Equal(sqlReq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", sqlReq.RequestOptions.ClientContext, cc)
	}
}

func TestClientContext_BatchReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	stmt := Statement{SQL: "SELECT 1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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

	// Test PartitionQuery
	partitions, err := txn.PartitionQueryWithOptions(ctx, stmt, PartitionOptions{MaxPartitions: 1}, QueryOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least 1 partition")
	}
	if !proto.Equal(partitions[0].qreq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in Partition.qreq ClientContext:\ngot:  %v\nwant: %v", partitions[0].qreq.RequestOptions.ClientContext, cc)
	}

	// Test PartitionRead
	partitions, err = txn.PartitionReadWithOptions(ctx, "Table", KeySets(Key{"key1"}), []string{"Col1"}, PartitionOptions{MaxPartitions: 1}, ReadOptions{ClientContext: cc})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least 1 partition")
	}
	if !proto.Equal(partitions[0].rreq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in Partition.rreq ClientContext:\ngot:  %v\nwant: %v", partitions[0].rreq.RequestOptions.ClientContext, cc)
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
	var gotReq *sppb.CommitRequest
	for _, req := range reqs {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			gotReq = commitReq
			break
		}
	}

	if gotReq == nil {
		t.Fatal("expected CommitRequest")
	}
	if !proto.Equal(gotReq.RequestOptions.ClientContext, cc) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, cc)
	}
}

func TestClientContext_EmptyMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	cc := &sppb.RequestOptions_ClientContext{
		SecureContext: make(map[string]*structpb.Value),
	}

	stmt := Statement{SQL: "SELECT 1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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

	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: cc})
	if _, err := iter.Next(); err != nil && err != iterator.Done {
		t.Fatal(err)
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			found = true
			if r.RequestOptions.ClientContext == nil {
				t.Error("expected non-nil ClientContext for empty map")
			} else if len(r.RequestOptions.ClientContext.SecureContext) != 0 {
				t.Errorf("expected empty SecureContext map, got %v", r.RequestOptions.ClientContext.SecureContext)
			}
		}
	}
	if !found {
		t.Error("ExecuteSqlRequest not found")
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
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	_, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	var sqlReq *sppb.ExecuteSqlRequest
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			sqlReq = r
			break
		}
	}
	if sqlReq == nil {
		t.Fatal("expected ExecuteSqlRequest for PDML")
	}
	if !proto.Equal(sqlReq.RequestOptions.ClientContext, cc) {
		t.Errorf("PDML default: mismatch in ClientContext:\ngot:  %v\nwant: %v", sqlReq.RequestOptions.ClientContext, cc)
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

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	stmt := Statement{SQL: "SELECT 1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
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

	// Test PartitionQuery (default)
	partitions, err := txn.PartitionQuery(ctx, stmt, PartitionOptions{MaxPartitions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least 1 partition")
	}
	if !proto.Equal(partitions[0].qreq.RequestOptions.ClientContext, cc) {
		t.Errorf("Batch default: mismatch in Partition.qreq ClientContext:\ngot:  %v\nwant: %v", partitions[0].qreq.RequestOptions.ClientContext, cc)
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
	var gotBegin *sppb.BeginTransactionRequest
	for _, req := range reqs {
		if br, ok := req.(*sppb.BeginTransactionRequest); ok {
			gotBegin = br
			break
		}
	}
	if gotBegin == nil {
		t.Fatal("expected BeginTransactionRequest")
	}
	if !proto.Equal(gotBegin.RequestOptions.ClientContext, cc) {
		t.Errorf("BeginTransaction default: mismatch in ClientContext:\ngot:  %v\nwant: %v", gotBegin.RequestOptions.ClientContext, cc)
	}
}

func TestClientContext_PDML_BeginTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	stmt := Statement{SQL: "UPDATE Table SET Col1=1 WHERE 1=1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	_, err := client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	var gotBegin *sppb.BeginTransactionRequest
	for _, req := range reqs {
		if br, ok := req.(*sppb.BeginTransactionRequest); ok {
			gotBegin = br
			break
		}
	}
	if gotBegin == nil {
		t.Fatal("expected BeginTransactionRequest for PDML")
	}
	if !proto.Equal(gotBegin.RequestOptions.ClientContext, cc) {
		t.Errorf("PDML BeginTransaction: mismatch in ClientContext:\ngot:  %v\nwant: %v", gotBegin.RequestOptions.ClientContext, cc)
	}
}

func TestClientContext_Batch_BeginTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cc := makeClientContext(map[string]string{"test-key": "test-value"})

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        cc,
		DisableNativeMetrics: true,
	})
	defer teardown()

	_, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	var gotBegin *sppb.BeginTransactionRequest
	for _, req := range reqs {
		if br, ok := req.(*sppb.BeginTransactionRequest); ok {
			gotBegin = br
			break
		}
	}
	if gotBegin == nil {
		t.Fatal("expected BeginTransactionRequest for Batch")
	}
	if !proto.Equal(gotBegin.RequestOptions.ClientContext, cc) {
		t.Errorf("Batch BeginTransaction: mismatch in ClientContext:\ngot:  %v\nwant: %v", gotBegin.RequestOptions.ClientContext, cc)
	}
}
