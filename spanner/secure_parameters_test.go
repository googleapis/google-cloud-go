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
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	// 1. Test propagation via QueryOptions
	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: clientContext})
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
	sqlReqs := []*sppb.ExecuteSqlRequest{}
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}
	if len(sqlReqs) != 1 {
		t.Fatalf("expected 1 ExecuteSqlRequest, got %d", len(sqlReqs))
	}
	gotReq := sqlReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}

	// 2. Test propagation via ClientConfig default
	server2, clientWithDefault, teardown2 := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        clientContext,
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
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
	sqlReqs = []*sppb.ExecuteSqlRequest{}
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}
	// Note: New client might have made some other requests (e.g. CreateSession)
	found := false
	for _, r := range sqlReqs {
		if r.Sql == stmt.SQL {
			if !proto.Equal(r.RequestOptions.ClientContext, clientContext) {
				t.Errorf("mismatch in ClientContext (default):\ngot:  %v\nwant: %v", r.RequestOptions.ClientContext, clientContext)
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

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	iter := client.Single().ReadWithOptions(ctx, "Table", KeySets(Key{"key1"}), []string{"Col1"}, &ReadOptions{ClientContext: clientContext})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// Read might fail if result not mocked, but we care about the request
			break
		}
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	readReqs := []*sppb.ReadRequest{}
	for _, req := range reqs {
		if readReq, ok := req.(*sppb.ReadRequest); ok {
			readReqs = append(readReqs, readReq)
		}
	}
	if len(readReqs) != 1 {
		t.Fatalf("expected 1 ReadRequest, got %d", len(readReqs))
	}
	gotReq := readReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_ReadWriteTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		return tx.BufferWrite([]*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})})
	}, TransactionOptions{ClientContext: clientContext})
	if err != nil {
		t.Fatal(err)
	}

	// We expect BeginTransaction and Commit requests
	reqs := drainRequestsFromServer(server.TestSpanner)
	beginReqs := []*sppb.BeginTransactionRequest{}
	commitReqs := []*sppb.CommitRequest{}
	for _, req := range reqs {
		switch r := req.(type) {
		case *sppb.BeginTransactionRequest:
			beginReqs = append(beginReqs, r)
		case *sppb.CommitRequest:
			commitReqs = append(commitReqs, r)
		}
	}

	if len(beginReqs) != 1 {
		t.Fatalf("expected 1 BeginTransactionRequest, got %d", len(beginReqs))
	}
	gotBegin := beginReqs[0]
	if !proto.Equal(gotBegin.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in BeginTransaction ClientContext:\ngot:  %v\nwant: %v", gotBegin.RequestOptions.ClientContext, clientContext)
	}

	if len(commitReqs) != 1 {
		t.Fatalf("expected 1 CommitRequest, got %d", len(commitReqs))
	}
	gotCommit := commitReqs[0]
	if !proto.Equal(gotCommit.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in Commit ClientContext:\ngot:  %v\nwant: %v", gotCommit.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_BatchWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	iter := client.BatchWriteWithOptions(ctx, []*MutationGroup{
		{Mutations: []*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})}},
	}, BatchWriteOptions{ClientContext: clientContext})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// BatchWrite will fail on mock server if not set up, but we just care about the request
			break
		}
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	batchWriteReqs := []*sppb.BatchWriteRequest{}
	for _, req := range reqs {
		if bwReq, ok := req.(*sppb.BatchWriteRequest); ok {
			batchWriteReqs = append(batchWriteReqs, bwReq)
		}
	}

	if len(batchWriteReqs) != 1 {
		t.Fatalf("expected 1 BatchWriteRequest, got %d", len(batchWriteReqs))
	}
	gotReq := batchWriteReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_Merging(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stmt := Statement{SQL: "SELECT 1"}

	defaultContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key1": {Kind: &structpb.Value_StringValue{StringValue: "default_value1"}},
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "default_value2"}},
		},
	}
	requestContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "request_value2"}},
			"key3": {Kind: &structpb.Value_StringValue{StringValue: "request_value3"}},
		},
	}
	expectedContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key1": {Kind: &structpb.Value_StringValue{StringValue: "default_value1"}},
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "request_value2"}},
			"key3": {Kind: &structpb.Value_StringValue{StringValue: "request_value3"}},
		},
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        defaultContext,
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: requestContext})
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
	sqlReqs := []*sppb.ExecuteSqlRequest{}
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok && sqlReq.Sql == stmt.SQL {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}
	if len(sqlReqs) != 1 {
		t.Fatalf("expected 1 ExecuteSqlRequest, got %d", len(sqlReqs))
	}
	gotReq := sqlReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, expectedContext) {
		t.Errorf("mismatch in Merged ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, expectedContext)
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

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	_, err := client.PartitionedUpdateWithOptions(ctx, stmt, QueryOptions{ClientContext: clientContext})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	sqlReqs := []*sppb.ExecuteSqlRequest{}
	for _, req := range reqs {
		if sqlReq, ok := req.(*sppb.ExecuteSqlRequest); ok {
			sqlReqs = append(sqlReqs, sqlReq)
		}
	}
	if len(sqlReqs) != 1 {
		t.Fatalf("expected 1 ExecuteSqlRequest, got %d", len(sqlReqs))
	}
	gotReq := sqlReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_BatchReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	// Test PartitionQuery
	partitions, err := txn.PartitionQueryWithOptions(ctx, stmt, PartitionOptions{MaxPartitions: 1}, QueryOptions{ClientContext: clientContext})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least 1 partition")
	}
	gotReq := partitions[0].qreq
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in Partition.qreq ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}

	// Test PartitionRead
	partitions, err = txn.PartitionReadWithOptions(ctx, "Table", KeySets(Key{"key1"}), []string{"Col1"}, PartitionOptions{MaxPartitions: 1}, ReadOptions{ClientContext: clientContext})
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) == 0 {
		t.Fatal("expected at least 1 partition")
	}
	gotReadReq := partitions[0].rreq
	if !proto.Equal(gotReadReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in Partition.rreq ClientContext:\ngot:  %v\nwant: %v", gotReadReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_ApplyAtLeastOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        clientContext,
		DisableNativeMetrics: true,
	})
	defer teardown()

	_, err := client.Apply(ctx, []*Mutation{Insert("Table", []string{"Col1"}, []interface{}{1})}, ApplyAtLeastOnce())
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	commitReqs := []*sppb.CommitRequest{}
	for _, req := range reqs {
		if commitReq, ok := req.(*sppb.CommitRequest); ok {
			commitReqs = append(commitReqs, commitReq)
		}
	}

	if len(commitReqs) != 1 {
		t.Fatalf("expected 1 CommitRequest, got %d", len(commitReqs))
	}
	gotReq := commitReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_ReadOnlyTransaction_ExplicitBegin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        clientContext,
		DisableNativeMetrics: true,
	})
	defer teardown()

	txn := client.ReadOnlyTransaction()
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	iter := txn.Query(ctx, stmt)
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
	beginReqs := []*sppb.BeginTransactionRequest{}
	for _, req := range reqs {
		if br, ok := req.(*sppb.BeginTransactionRequest); ok {
			beginReqs = append(beginReqs, br)
		}
	}

	if len(beginReqs) != 1 {
		t.Fatalf("expected 1 BeginTransactionRequest, got %d", len(beginReqs))
	}
	gotReq := beginReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in BeginTransaction ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_BatchUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	stmt := Statement{SQL: "UPDATE Table SET Col1=1 WHERE 1=1"}
	server.TestSpanner.PutStatementResult(stmt.SQL, &StatementResult{
		Type:        StatementResultUpdateCount,
		UpdateCount: 1,
	})

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"test-key": {Kind: &structpb.Value_StringValue{StringValue: "test-value"}},
		},
	}

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.BatchUpdateWithOptions(ctx, []Statement{stmt}, QueryOptions{ClientContext: clientContext})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	batchDmlReqs := []*sppb.ExecuteBatchDmlRequest{}
	for _, req := range reqs {
		if req, ok := req.(*sppb.ExecuteBatchDmlRequest); ok {
			batchDmlReqs = append(batchDmlReqs, req)
		}
	}

	if len(batchDmlReqs) != 1 {
		t.Fatalf("expected 1 ExecuteBatchDmlRequest, got %d", len(batchDmlReqs))
	}
	gotReq := batchDmlReqs[0]
	if !proto.Equal(gotReq.RequestOptions.ClientContext, clientContext) {
		t.Errorf("mismatch in ClientContext:\ngot:  %v\nwant: %v", gotReq.RequestOptions.ClientContext, clientContext)
	}
}

func TestClientContext_RWTransaction_MultipleRPCs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	clientContext := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"tx-key": {Kind: &structpb.Value_StringValue{StringValue: "tx-value"}},
		},
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		// 1. First ExecuteSql
		iter := tx.Query(ctx, stmt)
		if _, err := iter.Next(); err != nil && err != iterator.Done {
			return err
		}
		iter.Stop()

		// 2. A Read
		if _, err := tx.ReadRow(ctx, "Table", Key{"key1"}, []string{"Col1"}); err != nil && ToSpannerError(err).(*Error).Code != codes.NotFound {
			// ReadRow might return NotFound if not mocked, which is fine for request verification
		}

		// 3. Second ExecuteSql
		iter2 := tx.Query(ctx, stmt)
		if _, err := iter2.Next(); err != nil && err != iterator.Done {
			return err
		}
		iter2.Stop()

		return nil
	}, TransactionOptions{ClientContext: clientContext})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	sqlCount := 0
	readCount := 0
	for _, req := range reqs {
		switch r := req.(type) {
		case *sppb.ExecuteSqlRequest:
			sqlCount++
			if !proto.Equal(r.RequestOptions.ClientContext, clientContext) {
				t.Errorf("ExecuteSql %d: mismatch in ClientContext:\ngot:  %v\nwant: %v", sqlCount, r.RequestOptions.ClientContext, clientContext)
			}
		case *sppb.ReadRequest:
			readCount++
			if !proto.Equal(r.RequestOptions.ClientContext, clientContext) {
				t.Errorf("ReadRequest %d: mismatch in ClientContext:\ngot:  %v\nwant: %v", readCount, r.RequestOptions.ClientContext, clientContext)
			}
		}
	}
	if sqlCount != 2 {
		t.Errorf("expected 2 ExecuteSqlRequest, got %d", sqlCount)
	}
	if readCount != 1 {
		t.Errorf("expected 1 ReadRequest, got %d", readCount)
	}
}

func TestClientContext_EmptyMap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	clientContext := &sppb.RequestOptions_ClientContext{
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	iter := client.Single().QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: clientContext})
	if _, err := iter.Next(); err != nil && err != iterator.Done {
		t.Fatal(err)
	}
	iter.Stop()

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok {
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

func TestClientContext_ComplexOverriding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	globalCtx := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key1": {Kind: &structpb.Value_StringValue{StringValue: "global1"}},
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "global2"}},
		},
	}
	txCtx := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "tx2"}},
			"key3": {Kind: &structpb.Value_StringValue{StringValue: "tx3"}},
		},
	}
	reqCtx := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key3": {Kind: &structpb.Value_StringValue{StringValue: "req3"}},
			"key4": {Kind: &structpb.Value_StringValue{StringValue: "req4"}},
		},
	}

	// Final merged result for the request:
	// key1: from global
	// key2: from tx (overrides global)
	// key3: from req (overrides tx)
	// key4: from req
	expectedCtx := &sppb.RequestOptions_ClientContext{
		SecureContext: map[string]*structpb.Value{
			"key1": {Kind: &structpb.Value_StringValue{StringValue: "global1"}},
			"key2": {Kind: &structpb.Value_StringValue{StringValue: "tx2"}},
			"key3": {Kind: &structpb.Value_StringValue{StringValue: "req3"}},
			"key4": {Kind: &structpb.Value_StringValue{StringValue: "req4"}},
		},
	}

	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientContext:        globalCtx,
		DisableNativeMetrics: true,
	})
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
				{Values: []*structpb.Value{{Kind: &structpb.Value_NumberValue{NumberValue: 1}}}},
			},
		},
	})

	_, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.QueryWithOptions(ctx, stmt, QueryOptions{ClientContext: reqCtx})
		if _, err := iter.Next(); err != nil && err != iterator.Done {
			return err
		}
		iter.Stop()
		return nil
	}, TransactionOptions{ClientContext: txCtx})
	if err != nil {
		t.Fatal(err)
	}

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, req := range reqs {
		if r, ok := req.(*sppb.ExecuteSqlRequest); ok && r.Sql == stmt.SQL {
			found = true
			if !proto.Equal(r.RequestOptions.ClientContext, expectedCtx) {
				t.Errorf("mismatch in Merged ClientContext:\ngot:  %v\nwant: %v", r.RequestOptions.ClientContext, expectedCtx)
			}
		}
	}
	if !found {
		t.Error("ExecuteSqlRequest not found")
	}
}
