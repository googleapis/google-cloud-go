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

package spanner

import (
	"context"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type recordedPartialResultSetStream struct {
	wire  [][]byte
	index int
}

func (s *recordedPartialResultSetStream) reset() { s.index = 0 }

func (s *recordedPartialResultSetStream) nextWire() []byte {
	wire := s.wire[s.index]
	s.index++
	return wire
}

func (s *recordedPartialResultSetStream) Recv() (*sppb.PartialResultSet, error) {
	result := new(sppb.PartialResultSet)
	if err := proto.Unmarshal(s.nextWire(), result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *recordedPartialResultSetStream) RecvMsg(message any) error {
	return proto.Unmarshal(s.nextWire(), message.(proto.Message))
}

func (*recordedPartialResultSetStream) Context() context.Context { return context.Background() }

func newRecordedPartialResultSetStream(b *testing.B) *recordedPartialResultSetStream {
	b.Helper()
	const (
		messageCount   = 4
		rowsPerMessage = 25
	)
	wire := make([][]byte, 0, messageCount)
	for messageIndex := 0; messageIndex < messageCount; messageIndex++ {
		result := &sppb.PartialResultSet{
			ResumeToken: []byte{byte(messageIndex + 1)},
			Last:        messageIndex == messageCount-1,
		}
		if messageIndex == 0 {
			result.Metadata = &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "SingerId", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				{Name: "Name", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
			}}}
		}
		for rowIndex := 0; rowIndex < rowsPerMessage; rowIndex++ {
			result.Values = append(result.Values,
				structpb.NewStringValue("123456789"),
				structpb.NewStringValue("Ada Lovelace"),
			)
		}
		encoded, err := proto.Marshal(result)
		if err != nil {
			b.Fatal(err)
		}
		wire = append(wire, encoded)
	}
	return &recordedPartialResultSetStream{wire: wire}
}

// BenchmarkPartialResultSetStreamingReceive decodes a recorded four-message,
// 100-row stream through whichever receive path the current build tag selects.
func BenchmarkPartialResultSetStreamingReceive(b *testing.B) {
	stream := newRecordedPartialResultSetStream(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.reset()
		decoder := new(partialResultSetDecoder)
		rowCount := 0
		for range stream.wire {
			result, err := recvPartialResultSet(stream)
			if err != nil {
				b.Fatal(err)
			}
			rows, _, err := decoder.add(result)
			if err != nil {
				b.Fatal(err)
			}
			rowCount += len(rows)
		}
		if rowCount != 100 {
			b.Fatalf("decoded %d rows, want 100", rowCount)
		}
	}
}
