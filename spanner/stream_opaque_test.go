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
	object, err := structpb.NewStruct(map[string]interface{}{
		"nested": []interface{}{nil, "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{{
				Name: "SingerId",
				Type: &sppb.Type{Code: sppb.TypeCode_INT64},
			}}},
			Transaction: &sppb.Transaction{Id: []byte("transaction")},
		},
		Values: []*structpb.Value{
			structpb.NewStringValue("1"),
			structpb.NewStructValue(object),
		},
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
	if !proto.Equal(internalPartialResultSetToOpen(got), input) {
		t.Fatalf("received result mismatch\n got: %v\nwant: %v", got, input)
	}
	if got.Values[0] != opaque.GetValues()[0] {
		t.Fatal("opaque Value was copied on the streaming path")
	}
	if got.Values[1].WhichKind() != opaquepb.Value_StructValue_case || got.Values[1].GetStructValue().GetFields()["nested"].WhichKind() != opaquepb.Value_ListValue_case {
		t.Fatalf("nested struct did not remain opaque: %v", got.Values[1])
	}
}

func receiveOpaqueForTest(t *testing.T, result *sppb.PartialResultSet) *internalPartialResultSet {
	t.Helper()
	wire, err := proto.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	got, err := recvPartialResultSet(&opaqueRecvTestStream{wire: wire})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestOpaqueNestedListChunkMerge(t *testing.T) {
	metadata := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{{
		Name: "Nested",
		Type: listType(stringType()),
	}}}}
	first := receiveOpaqueForTest(t, &sppb.PartialResultSet{
		Metadata: metadata,
		Values: []*structpb.Value{structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("nest")}}),
		}})},
		ChunkedValue: true,
	})
	second := receiveOpaqueForTest(t, &sppb.PartialResultSet{
		Values: []*structpb.Value{structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("ed")}}),
			structpb.NewNullValue(),
		}})},
	})

	decoder := new(partialResultSetDecoder)
	if rows, _, err := decoder.add(first); err != nil || len(rows) != 0 {
		t.Fatalf("first chunk: rows=%d err=%v", len(rows), err)
	}
	rows, _, err := decoder.add(second)
	if err != nil || len(rows) != 1 {
		t.Fatalf("second chunk: rows=%d err=%v", len(rows), err)
	}
	value := rows[0].vals[0]
	if _, ok := interface{}(value).(*opaquepb.Value); !ok {
		t.Fatalf("row stored %T, want *opaquepb.Value", value)
	}
	outer := internalListValues(value.GetListValue())
	inner := internalListValues(outer[0].GetListValue())
	if got := inner[0].GetStringValue(); got != "nested" {
		t.Fatalf("merged nested string = %q, want nested", got)
	}
	if internalValueKindOf(outer[1]) != internalValueNull {
		t.Fatalf("second nested value kind = %v, want null", internalValueKindOf(outer[1]))
	}
}

func TestOpaqueRowDecodeAndColumnValueCompatibility(t *testing.T) {
	metadata := &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "SingerId", Type: intType()},
		{Name: "Tags", Type: listType(stringType())},
		{Name: "Details", Type: listType(structType(mkField("Name", stringType()), mkField("Missing", stringType())))},
	}}}
	result := receiveOpaqueForTest(t, &sppb.PartialResultSet{
		Metadata: metadata,
		Values: []*structpb.Value{
			structpb.NewStringValue("7"),
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("math"), structpb.NewStringValue("code")}}),
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
				structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("Ada"), structpb.NewNullValue()}}),
			}}),
		},
	})
	rows, _, err := new(partialResultSetDecoder).add(result)
	if err != nil || len(rows) != 1 {
		t.Fatalf("decode rows=%d err=%v", len(rows), err)
	}
	var id int64
	var tags []string
	var details []*struct {
		Name    string
		Missing NullString
	}
	if err := rows[0].Columns(&id, &tags, &details); err != nil {
		t.Fatal(err)
	}
	if id != 7 || len(tags) != 2 || tags[1] != "code" || len(details) != 1 || details[0].Name != "Ada" || details[0].Missing.Valid {
		t.Fatalf("decoded values: id=%d tags=%v details=%+v", id, tags, details)
	}
	if got := rows[0].ColumnValue(1); !proto.Equal(got, structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("math"), structpb.NewStringValue("code")}})) {
		t.Fatalf("ColumnValue compatibility: %v", got)
	}
	var generic GenericColumnValue
	if err := rows[0].Column(0, &generic); err != nil || generic.Value.GetStringValue() != "7" {
		t.Fatalf("GenericColumnValue: value=%v err=%v", generic.Value, err)
	}
}
