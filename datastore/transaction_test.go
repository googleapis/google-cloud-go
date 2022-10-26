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
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewTransaction(t *testing.T) {
	client, srv, cleanup := newMock(t)
	defer cleanup()

	ctx := context.Background()
	rt := timestamppb.Now()

	for _, test := range []struct {
		settings *transactionSettings
		want     *pb.BeginTransactionRequest
	}{
		{
			&transactionSettings{},
			&pb.BeginTransactionRequest{ProjectId: "projectID"},
		},
		{
			&transactionSettings{readOnly: true},
			&pb.BeginTransactionRequest{
				ProjectId: "projectID",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{}},
				},
			},
		},
		{
			&transactionSettings{prevID: []byte("tid")},
			&pb.BeginTransactionRequest{
				ProjectId: "projectID",
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
				ProjectId: "projectID",
				TransactionOptions: &pb.TransactionOptions{
					Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{
						ReadTime: rt,
					}},
				},
			},
		},
	} {
		srv.addRPC(test.want, &pb.BeginTransactionResponse{
			Transaction: []byte("tid"),
		})
		_, err := client.newTransaction(ctx, test.settings)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransactionRollbackOnPanic(t *testing.T) {
	client, srv, cleanup := newMock(t)
	defer cleanup()

	tid := []byte("tid")

	var isRecovered bool
	defer func(r *bool) {
		if p := recover(); p != nil {
			*r = true
		}
	}(&isRecovered)

	srv.addRPC(&pb.BeginTransactionRequest{
		ProjectId: "projectID",
	}, &pb.BeginTransactionResponse{
		Transaction: tid,
	})

	srv.addRPC(&pb.RollbackRequest{
		ProjectId:   "projectID",
		Transaction: tid,
	}, &pb.RollbackResponse{})

	client.RunInTransaction(context.Background(), func(t *Transaction) error {
		panic("test panic")
	})

	if !isRecovered {
		t.Log("datastore: transaction didn't recover from panic")
	}
}
