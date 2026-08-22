// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build spanner_opaque

package spanner

import (
	"context"
	"errors"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/opaquepb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type opaqueRecvTestStream struct {
	wire       []byte
	recvCalled bool
	received   proto.Message
}

func (s *opaqueRecvTestStream) Recv() (*sppb.PartialResultSet, error) {
	s.recvCalled = true
	return nil, errors.New("typed Recv must not be used")
}

func (s *opaqueRecvTestStream) RecvMsg(message any) error {
	m := message.(proto.Message)
	s.received = m
	return proto.Unmarshal(s.wire, m)
}

func (*opaqueRecvTestStream) Context() context.Context { return context.Background() }

func TestOpaquePartialResultSetReceive(t *testing.T) {
	input := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{{
				Name: "SingerId",
				Type: &sppb.Type{Code: sppb.TypeCode_INT64},
			}}},
			Transaction: &sppb.Transaction{Id: []byte("transaction")},
		},
		Values:         []*structpb.Value{structpb.NewStringValue("1")},
		ChunkedValue:   true,
		ResumeToken:    []byte("resume"),
		Stats:          &sppb.ResultSetStats{QueryPlan: &sppb.QueryPlan{}},
		PrecommitToken: &sppb.MultiplexedSessionPrecommitToken{SeqNum: 7},
		Last:           true,
		CacheUpdate:    &sppb.CacheUpdate{DatabaseId: 9},
	}
	wire, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	stream := &opaqueRecvTestStream{wire: wire}

	got, err := recvPartialResultSet(stream)
	if err != nil {
		t.Fatal(err)
	}
	if stream.recvCalled {
		t.Fatal("typed open Recv was called")
	}
	opaque, ok := stream.received.(*opaquepb.PartialResultSet)
	if !ok {
		t.Fatalf("RecvMsg target type = %T, want *opaquepb.PartialResultSet", stream.received)
	}
	if !proto.Equal(got, input) {
		t.Fatalf("received result mismatch\n got: %v\nwant: %v", got, input)
	}
	if got.Values[0] != opaque.GetValues()[0] {
		t.Fatal("structpb.Value was re-marshaled at the open boundary")
	}
}
