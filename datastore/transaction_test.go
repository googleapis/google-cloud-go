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
	"errors"
	"strings"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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

func TestBackoffBeforefRetry(t *testing.T) {
	ctx := context.Background()
	retryer := gax.OnCodes([]codes.Code{codes.DeadlineExceeded}, gax.Backoff{
		Initial:    10 * time.Millisecond,
		Max:        100 * time.Millisecond,
		Multiplier: 2,
	})
	tests := []struct {
		desc     string
		err      error
		gaxSleep func(ctx context.Context, d time.Duration) error
		wantErr  bool
	}{
		{
			desc: "Retryable error",
			err:  status.Error(codes.DeadlineExceeded, "deadline exceeded"),
			gaxSleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
			wantErr: false,
		},
		{
			desc: "Non-retryable error",
			err:  errors.New("non-retryable error"),
			gaxSleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
			wantErr: true,
		},
		{
			desc: "Sleep error",
			err:  status.Error(codes.DeadlineExceeded, "deadline exceeded"),
			gaxSleep: func(ctx context.Context, d time.Duration) error {
				return errors.New("sleep error")
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			origGaxSleep := gaxSleep
			defer func() {
				gaxSleep = origGaxSleep
			}()
			gaxSleep = test.gaxSleep

			gotErr := backoffBeforeRetry(ctx, retryer, test.err)
			if (gotErr != nil && !test.wantErr) || (gotErr == nil && test.wantErr) {
				t.Errorf("error gotErr: %v, want error: %v", gotErr, test.wantErr)
			}
		})
	}
}

func equalErrs(gotErr error, wantErr error) bool {
	if gotErr == nil && wantErr == nil {
		return true
	}
	if gotErr == nil || wantErr == nil {
		return false
	}
	return strings.Contains(gotErr.Error(), wantErr.Error())
}

func TestRunInTransaction(t *testing.T) {

	type Counter struct {
		N int
	}
	key := NameKey("Counter", "c-01", nil)
	gotNumFRuns := 0
	f := func(tx *Transaction) error {
		gotNumFRuns++
		var c Counter
		if err := tx.Get(key, &c); err != nil {
			return err
		}
		c.N++
		if _, err := tx.Put(key, &c); err != nil {
			return err
		}
		return nil
	}

	e := &pb.Entity{
		Key: keyToProto(key),
		Properties: map[string]*pb.Value{
			"N": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
		},
	}

	txnID1 := []byte("tid")
	txnID2 := []byte("tid-2")
	projectID := "projectID"

	// BeginTransaction
	beginTxnReqAttempt1 := &pb.BeginTransactionRequest{
		ProjectId: projectID,
	}
	beginTxnReqAttempt2 := &pb.BeginTransactionRequest{
		ProjectId: projectID,
		TransactionOptions: &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadWrite_{ReadWrite: &pb.TransactionOptions_ReadWrite{
				// The second attempt should include previous transaction ID
				PreviousTransaction: txnID1,
			}},
		},
	}
	beginTxnResAttempt1Success := &pb.BeginTransactionResponse{
		Transaction: txnID1,
	}
	beginTxnResAttempt2Success := &pb.BeginTransactionResponse{
		Transaction: txnID2,
	}
	beginTxnRetryableErr := status.Error(codes.Internal, "Mock Internal error")

	// Lookup
	lookupReqBeginLaterAttempt1 := &pb.LookupRequest{
		ProjectId: projectID,
		Keys: []*pb.Key{
			keyToProto(key),
		},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_NewTransaction{
				NewTransaction: &pb.TransactionOptions{},
			},
		},
	}
	lookupResBeginLaterSuccess := &pb.LookupResponse{
		Found: []*pb.EntityResult{
			{
				Entity:  e,
				Version: 1,
			},
		},
		Transaction: txnID1,
	}
	lookupReqAttempt1 := &pb.LookupRequest{
		ProjectId: projectID,
		Keys: []*pb.Key{
			keyToProto(key),
		},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_Transaction{
				Transaction: txnID1,
			},
		},
	}
	lookupReqAttempt2 := &pb.LookupRequest{
		ProjectId: projectID,
		Keys: []*pb.Key{
			keyToProto(key),
		},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_Transaction{
				Transaction: txnID2,
			},
		},
	}
	lookupResSuccess := &pb.LookupResponse{
		Found: []*pb.EntityResult{
			{
				Entity:  e,
				Version: 1,
			},
		},
	}
	lookupRetryableErr := status.Error(codes.Aborted, "Mock Aborted error")
	lookupNonRetryableErr := status.Error(codes.FailedPrecondition, "Mock FailedPrecondition error")

	// Commit
	commitReqAttempt1 := &pb.CommitRequest{
		ProjectId: projectID,
		Mode:      pb.CommitRequest_TRANSACTIONAL,
		TransactionSelector: &pb.CommitRequest_Transaction{
			Transaction: txnID1,
		},
		Mutations: []*pb.Mutation{
			{
				Operation: &pb.Mutation_Upsert{
					Upsert: &pb.Entity{
						Key: keyToProto(key),
						Properties: map[string]*pb.Value{
							"N": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
						},
					},
				},
			},
		},
	}
	commitReqAttempt2 := &pb.CommitRequest{
		ProjectId: projectID,
		Mode:      pb.CommitRequest_TRANSACTIONAL,
		TransactionSelector: &pb.CommitRequest_Transaction{
			Transaction: txnID2,
		},
		Mutations: []*pb.Mutation{
			{
				Operation: &pb.Mutation_Upsert{
					Upsert: &pb.Entity{
						Key: keyToProto(key),
						Properties: map[string]*pb.Value{
							"N": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
						},
					},
				},
			},
		},
	}
	commitResSucces := &pb.CommitResponse{}
	commitNonRetryableErr := status.Error(codes.FailedPrecondition, "Mock FailedPrecondition error")
	commitRetryableErrNotAborted := status.Error(codes.Canceled, "Mock Canceled error")
	commitRetryableErrAborted := status.Error(codes.Aborted, "Mock Aborted error")

	// Rollback
	rollbackReq := &pb.RollbackRequest{
		ProjectId:   projectID,
		Transaction: txnID1,
	}
	rollbackResSuccess := &pb.RollbackResponse{}
	rollbackRetryableErr := status.Error(codes.Internal, "Mock Internal error")

	type addRPCInput struct {
		wantReq protoreflect.ProtoMessage
		resp    interface{}
	}
	tests := []struct {
		desc         string
		addRPCInputs []addRPCInput
		opts         []TransactionOption
		wantNumFRuns int
		wantErr      error
	}{
		{
			desc: "With BeginLater, success in first attempt",
			addRPCInputs: []addRPCInput{
				{lookupReqBeginLaterAttempt1, lookupResBeginLaterSuccess},
				{commitReqAttempt1, commitResSucces},
			},
			opts:         []TransactionOption{BeginLater},
			wantNumFRuns: 1,
		},
		{
			desc: "With BeginLater, retryable failure in f leads to transaction restart",
			addRPCInputs: []addRPCInput{
				{lookupReqBeginLaterAttempt1, lookupRetryableErr},
				// No rollback since retryableLookupErr did not return transaction ID i.e. transaction was not started
				{lookupReqBeginLaterAttempt1, lookupResBeginLaterSuccess},
				{commitReqAttempt1, commitResSucces},
			},
			opts:         []TransactionOption{BeginLater},
			wantNumFRuns: 2,
		},
		{
			desc: "Without BeginLater, success in first attempt",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupResSuccess},
				{commitReqAttempt1, commitResSucces},
			},
			wantNumFRuns: 1,
		},
		{
			desc: "Retryable failure in f leads to rollback and transaction restart",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupRetryableErr},
				{rollbackReq, rollbackResSuccess},

				{beginTxnReqAttempt2, beginTxnResAttempt2Success},
				{lookupReqAttempt2, lookupResSuccess},
				{commitReqAttempt2, commitResSucces},
			},
			wantNumFRuns: 2,
		},
		{
			desc: "Retryable failure in commit leads to rollback and restart of transaction",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupResSuccess},
				{commitReqAttempt1, commitRetryableErrNotAborted},
				{rollbackReq, rollbackResSuccess},

				{beginTxnReqAttempt2, beginTxnResAttempt2Success},
				{lookupReqAttempt2, lookupResSuccess},
				{commitReqAttempt2, commitResSucces},
			},
			wantNumFRuns: 2,
		},
		{
			desc: "Non-retryable failure in commit leads to rollback and does not restart transaction",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupResSuccess},
				{commitReqAttempt1, commitNonRetryableErr},
				{rollbackReq, rollbackResSuccess},
			},
			wantNumFRuns: 1,
			wantErr:      commitNonRetryableErr,
		},
		{
			desc: "Failure in newTransactionWithRetry does not run f",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnRetryableErr},
				{beginTxnReqAttempt1, beginTxnRetryableErr},
				{beginTxnReqAttempt1, beginTxnRetryableErr},
				{beginTxnReqAttempt1, beginTxnRetryableErr},
				{beginTxnReqAttempt1, beginTxnRetryableErr},
			},
			wantErr: beginTxnRetryableErr,
		},
		{
			desc: "Non-retryable failure in f leads to rollback and return",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupNonRetryableErr},
			},
			wantNumFRuns: 1,
			wantErr:      lookupNonRetryableErr,
		},
		{
			desc: "commit failed with aborted error with rollbackWithRetry failure returns commit concurrent transaction error and does not restart transaction",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupResSuccess},
				{commitReqAttempt1, commitRetryableErrAborted},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
			},
			wantNumFRuns: 1,
			wantErr:      ErrConcurrentTransaction,
		},
		{
			desc: "commit failed with retryable error other than aborted with rollbackWithRetry failure returns commit error and does not restart transaction",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupResSuccess},
				{commitReqAttempt1, commitRetryableErrNotAborted},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
			},
			wantNumFRuns: 1,
			wantErr:      commitRetryableErrNotAborted,
		},
		{
			desc: "f failed with retryable error with rollbackWithRetry failure returns lookup error and does not restart transaction",
			addRPCInputs: []addRPCInput{
				{beginTxnReqAttempt1, beginTxnResAttempt1Success},
				{lookupReqAttempt1, lookupRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
				{rollbackReq, rollbackRetryableErr},
			},
			wantNumFRuns: 1,
			wantErr:      lookupRetryableErr,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(*testing.T) {
			ctx := context.Background()
			client, srv, cleanup := newMock(t)
			defer cleanup()
			gotNumFRuns = 0
			for _, input := range test.addRPCInputs {
				srv.addRPC(input.wantReq, input.resp)
			}
			_, gotErr := client.RunInTransaction(ctx, f, test.opts...)
			if !equalErrs(gotErr, test.wantErr) {
				t.Errorf("%v: error got: %v, want: %v", test.desc, gotErr, test.wantErr)
			}
			if gotNumFRuns != test.wantNumFRuns {
				t.Errorf("%v: f runs got: %v, want: %v", test.desc, gotNumFRuns, test.wantNumFRuns)
			}
			remReqs := len(srv.reqItems)
			if remReqs != 0 {
				t.Errorf("%v: remaining requests expected by server should be 0. got: %v", test.desc, remReqs)
			}
		})
	}
}
