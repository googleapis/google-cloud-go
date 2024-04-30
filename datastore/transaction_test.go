// Copyright 2017 Google LLC
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

package datastore

import (
	"context"
	"testing"

	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewTransaction(t *testing.T) {
	var got *pb.BeginTransactionRequest
	client := &Client{
		dataset: "project",
		client: &fakeDatastoreClient{
			beginTransaction: func(req *pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
				got = req
				return &pb.BeginTransactionResponse{
					Transaction: []byte("tid"),
				}, nil
			},
		},
	}
	ctx := context.Background()
	rt := timestamppb.Now()
	for _, test := range []struct {
		settings *transactionSettings
		want     *pb.BeginTransactionRequest
	}{
		{
			&transactionSettings{},
			&pb.BeginTransactionRequest{ProjectId: "project"},
		},
		{
			&transactionSettings{readOnly: true},
			&pb.BeginTransactionRequest{
				ProjectId: "project",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{}},
				},
			},
		},
		{
			&transactionSettings{prevID: []byte("tid")},
			&pb.BeginTransactionRequest{
				ProjectId: "project",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadWrite_{ReadWrite: &pb.TransactionOptions_ReadWrite{
						PreviousTransaction: []byte("tid"),
					},
					},
				},
			},
		},
		{
			&transactionSettings{readOnly: true, readTime: rt},
			&pb.BeginTransactionRequest{
				ProjectId: "project",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{
						ReadTime: rt,
					}},
				},
			},
		},
	} {
		_, err := client.newTransaction(ctx, test.settings)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(got, test.want) {
			t.Errorf("%+v:\ngot  %+v\nwant %+v", test.settings, got, test.want)
		}
	}
}

func TestBeginLaterTransactionOption(t *testing.T) {
	type ent struct {
		A int
	}
	type addRPCInput struct {
		wantReq proto.Message
		resp    interface{}
	}

	mockKind := "mockKind"
	mockTxnID := []byte("tid")
	mockKey := NameKey(mockKind, "testName", nil)
	mockEntity := &pb.Entity{
		Key: keyToProto(mockKey),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 0}},
		},
	}
	mockEntityResults := []*pb.EntityResult{
		{
			Entity:  mockEntity,
			Version: 1,
		},
	}

	// Requests and responses to be used in tests
	txnReadOptions := &pb.ReadOptions{
		ConsistencyType: &pb.ReadOptions_Transaction{
			Transaction: mockTxnID,
		},
	}
	newTxnReadOptions := &pb.ReadOptions{
		ConsistencyType: &pb.ReadOptions_NewTransaction{},
	}

	lookupReqWithTxn := &pb.LookupRequest{
		ProjectId:  mockProjectID,
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(mockKey),
		},
		ReadOptions: txnReadOptions,
	}
	lookupResWithTxn := &pb.LookupResponse{
		Found: mockEntityResults,
	}

	lookupReqWithNewTxn := &pb.LookupRequest{
		ProjectId:  mockProjectID,
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(mockKey),
		},
		ReadOptions: newTxnReadOptions,
	}
	lookupResWithNewTxn := &pb.LookupResponse{
		Transaction: mockTxnID,
		Found:       mockEntityResults,
	}

	runQueryReqWithTxn := &pb.RunQueryRequest{
		ProjectId: mockProjectID,
		QueryType: &pb.RunQueryRequest_Query{Query: &pb.Query{
			Kind: []*pb.KindExpression{{Name: mockKind}},
		}},
		ReadOptions: txnReadOptions,
	}
	runQueryResWithTxn := &pb.RunQueryResponse{
		Batch: &pb.QueryResultBatch{
			MoreResults:      pb.QueryResultBatch_NO_MORE_RESULTS,
			EntityResultType: pb.EntityResult_FULL,
			EntityResults:    mockEntityResults,
		},
	}

	runQueryReqWithNewTxn := &pb.RunQueryRequest{
		ProjectId: mockProjectID,
		QueryType: &pb.RunQueryRequest_Query{Query: &pb.Query{
			Kind: []*pb.KindExpression{{Name: mockKind}},
		}},
		ReadOptions: newTxnReadOptions,
	}
	runQueryResWithNewTxn := &pb.RunQueryResponse{
		Transaction: mockTxnID,
		Batch: &pb.QueryResultBatch{
			MoreResults:      pb.QueryResultBatch_NO_MORE_RESULTS,
			EntityResultType: pb.EntityResult_FULL,
			EntityResults:    mockEntityResults,
		},
	}

	countAlias := "count"
	runAggQueryReqWithTxn := &pb.RunAggregationQueryRequest{
		ProjectId:   mockProjectID,
		ReadOptions: txnReadOptions,
		QueryType: &pb.RunAggregationQueryRequest_AggregationQuery{
			AggregationQuery: &pb.AggregationQuery{
				QueryType: &pb.AggregationQuery_NestedQuery{
					NestedQuery: &pb.Query{
						Kind: []*pb.KindExpression{{Name: mockKind}},
					},
				},
				Aggregations: []*pb.AggregationQuery_Aggregation{
					{
						Operator: &pb.AggregationQuery_Aggregation_Count_{},
						Alias:    countAlias,
					},
				},
			},
		},
	}
	runAggQueryResWithTxn := &pb.RunAggregationQueryResponse{
		Batch: &pb.AggregationResultBatch{
			AggregationResults: []*pb.AggregationResult{
				{
					AggregateProperties: map[string]*pb.Value{
						countAlias: {
							ValueType: &pb.Value_IntegerValue{IntegerValue: 1},
						},
					},
				},
			},
		},
	}

	runAggQueryReqWithNewTxn := &pb.RunAggregationQueryRequest{
		ProjectId:   mockProjectID,
		ReadOptions: newTxnReadOptions,
		QueryType: &pb.RunAggregationQueryRequest_AggregationQuery{
			AggregationQuery: &pb.AggregationQuery{
				QueryType: &pb.AggregationQuery_NestedQuery{
					NestedQuery: &pb.Query{
						Kind: []*pb.KindExpression{{Name: mockKind}},
					},
				},
				Aggregations: []*pb.AggregationQuery_Aggregation{
					{
						Operator: &pb.AggregationQuery_Aggregation_Count_{},
						Alias:    countAlias,
					},
				},
			},
		},
	}
	runAggQueryResWithNewTxn := &pb.RunAggregationQueryResponse{
		Batch: &pb.AggregationResultBatch{
			AggregationResults: []*pb.AggregationResult{
				{
					AggregateProperties: map[string]*pb.Value{
						countAlias: {
							ValueType: &pb.Value_IntegerValue{IntegerValue: 1},
						},
					},
				},
			},
		},
		Transaction: mockTxnID,
	}

	commitReq := &pb.CommitRequest{
		ProjectId: mockProjectID,
		Mode:      pb.CommitRequest_TRANSACTIONAL,
		TransactionSelector: &pb.CommitRequest_Transaction{
			Transaction: mockTxnID,
		},
		Mutations: []*pb.Mutation{
			{
				Operation: &pb.Mutation_Upsert{
					Upsert: mockEntity,
				},
			},
		},
	}
	commitRes := &pb.CommitResponse{}

	beginTxnReq := &pb.BeginTransactionRequest{
		ProjectId: mockProjectID,
	}
	beginTxnRes := &pb.BeginTransactionResponse{
		Transaction: mockTxnID,
	}

	testcases := []struct {
		desc      string
		rpcInputs []addRPCInput
		ops       []string
		settings  *transactionSettings
	}{
		{
			desc: "[Get, Get, Put, Commit] No options. First Get does not pass new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: beginTxnReq,
					resp:    beginTxnRes,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"Get", "Get", "Put", "Commit"},
			settings: &transactionSettings{},
		},
		{
			desc: "[Get, Get, Put, Commit] BeginLater. First Get passes new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: lookupReqWithNewTxn,
					resp:    lookupResWithNewTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"Get", "Get", "Put", "Commit"},
			settings: &transactionSettings{beginLater: true},
		},
		{
			desc: "[RunQuery, Get, Put, Commit] No options. RunQuery does not pass new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: beginTxnReq,
					resp:    beginTxnRes,
				},
				{
					wantReq: runQueryReqWithTxn,
					resp:    runQueryResWithTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"RunQuery", "Get", "Put", "Commit"},
			settings: &transactionSettings{},
		},
		{
			desc: "[RunQuery, Get, Put, Commit] BeginLater. RunQuery passes new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: runQueryReqWithNewTxn,
					resp:    runQueryResWithNewTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"RunQuery", "Get", "Put", "Commit"},
			settings: &transactionSettings{beginLater: true},
		},
		{
			desc: "[RunAggregationQuery, Get, Put, Commit] No options. RunAggregationQuery does not pass new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: beginTxnReq,
					resp:    beginTxnRes,
				},
				{
					wantReq: runAggQueryReqWithTxn,
					resp:    runAggQueryResWithTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"RunAggregationQuery", "Get", "Put", "Commit"},
			settings: &transactionSettings{},
		},
		{
			desc: "[RunAggregationQuery, Get, Put, Commit] BeginLater. RunAggregationQuery passes new_transaction",
			rpcInputs: []addRPCInput{
				{
					wantReq: runAggQueryReqWithNewTxn,
					resp:    runAggQueryResWithNewTxn,
				},
				{
					wantReq: lookupReqWithTxn,
					resp:    lookupResWithTxn,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"RunAggregationQuery", "Get", "Put", "Commit"},
			settings: &transactionSettings{beginLater: true},
		},
		{
			desc: "[Put, Commit] No options. BeginTransaction request sent",
			rpcInputs: []addRPCInput{
				{
					wantReq: beginTxnReq,
					resp:    beginTxnRes,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"Put", "Commit"},
			settings: &transactionSettings{},
		},
		{
			desc: "[Put, Commit] BeginLater. BeginTransaction request sent",
			rpcInputs: []addRPCInput{
				{
					wantReq: beginTxnReq,
					resp:    beginTxnRes,
				},
				{
					wantReq: commitReq,
					resp:    commitRes,
				},
			},
			ops:      []string{"Put", "Commit"},
			settings: &transactionSettings{beginLater: true},
		},
	}

	for _, testcase := range testcases {
		ctx := context.Background()
		client, srv, cleanup := newMock(t)
		defer cleanup()
		for _, rpcInput := range testcase.rpcInputs {
			srv.addRPC(rpcInput.wantReq, rpcInput.resp)
		}

		dst := &ent{}

		txn, err := client.newTransaction(ctx, testcase.settings)
		if err != nil {
			t.Fatalf("%q: %v", testcase.desc, err)
		}

		for i, op := range testcase.ops {
			switch op {
			case "RunQuery":
				query := NewQuery(mockKind).Transaction(txn)
				got := []*ent{}
				if _, err := client.GetAll(ctx, query, &got); err != nil {
					t.Fatalf("%q RunQuery[%v] failed with error %v", testcase.desc, i, err)
				}
			case "RunAggregationQuery":
				aggQuery := NewQuery(mockKind).Transaction(txn).NewAggregationQuery()
				aggQuery.WithCount(countAlias)

				if _, err := client.RunAggregationQuery(ctx, aggQuery); err != nil {
					t.Fatalf("%q RunAggregationQuery[%v] failed with error %v", testcase.desc, i, err)
				}
			case "Get":
				if err := txn.Get(mockKey, dst); err != nil {
					t.Fatalf("%q Get[%v] failed with error %v", testcase.desc, i, err)
				}
			case "Put":
				_, err := txn.Put(mockKey, dst)
				if err != nil {
					t.Fatalf("%q Put[%v] failed with error %v", testcase.desc, i, err)
				}
			case "Commit":
				_, err := txn.Commit()
				if err != nil {
					t.Fatalf("%q Commit[%v] failed with error %v", testcase.desc, i, err)
				}
			}
		}
	}
}
