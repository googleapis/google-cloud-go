/*
Copyright 2017 Google LLC

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
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	// Mocked transaction timestamp.
	trxTs = time.Unix(1, 2)
	// Metadata for mocked KV table, its rows are returned by SingleUse
	// transactions.
	kvMeta = func() *sppb.ResultSetMetadata {
		meta := KvMeta()
		meta.Transaction = &sppb.Transaction{
			ReadTimestamp: timestampProto(trxTs),
		}
		return meta
	}()
	// Metadata for mocked ListKV table, which uses List for its key and value.
	// Its rows are returned by snapshot readonly transactions, as indicated in
	// the transaction metadata.
	kvListMeta = &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{
					Name: "Key",
					Type: &sppb.Type{
						Code: sppb.TypeCode_ARRAY,
						ArrayElementType: &sppb.Type{
							Code: sppb.TypeCode_STRING,
						},
					},
				},
				{
					Name: "Value",
					Type: &sppb.Type{
						Code: sppb.TypeCode_ARRAY,
						ArrayElementType: &sppb.Type{
							Code: sppb.TypeCode_STRING,
						},
					},
				},
			},
		},
		Transaction: &sppb.Transaction{
			Id:            transactionID{5, 6, 7, 8, 9},
			ReadTimestamp: timestampProto(trxTs),
		},
	}
	// Metadata for mocked schema of a query result set, which has two struct
	// columns named "Col1" and "Col2", the struct's schema is like the
	// following:
	//
	//	STRUCT {
	//		INT
	//		LIST<STRING>
	//	}
	//
	// Its rows are returned in readwrite transaction, as indicated in the
	// transaction metadata.
	kvObjectMeta = &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{
					Name: "Col1",
					Type: &sppb.Type{
						Code: sppb.TypeCode_STRUCT,
						StructType: &sppb.StructType{
							Fields: []*sppb.StructType_Field{
								{
									Name: "foo-f1",
									Type: &sppb.Type{
										Code: sppb.TypeCode_INT64,
									},
								},
								{
									Name: "foo-f2",
									Type: &sppb.Type{
										Code: sppb.TypeCode_ARRAY,
										ArrayElementType: &sppb.Type{
											Code: sppb.TypeCode_STRING,
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "Col2",
					Type: &sppb.Type{
						Code: sppb.TypeCode_STRUCT,
						StructType: &sppb.StructType{
							Fields: []*sppb.StructType_Field{
								{
									Name: "bar-f1",
									Type: &sppb.Type{
										Code: sppb.TypeCode_INT64,
									},
								},
								{
									Name: "bar-f2",
									Type: &sppb.Type{
										Code: sppb.TypeCode_ARRAY,
										ArrayElementType: &sppb.Type{
											Code: sppb.TypeCode_STRING,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Transaction: &sppb.Transaction{
			Id: transactionID{1, 2, 3, 4, 5},
		},
	}
)

func describeRows(l []*Row) string {
	// generate a nice test failure description
	var s = "["
	for i, r := range l {
		if i != 0 {
			s += ",\n "
		}
		s += fmt.Sprint(r)
	}
	s += "]"
	return s
}

// Helper for generating proto3 Value_ListValue instances, making test code
// shorter and readable.
func genProtoListValue(v ...string) *structpb.Value_ListValue {
	r := &structpb.Value_ListValue{
		ListValue: &structpb.ListValue{
			Values: []*structpb.Value{},
		},
	}
	for _, e := range v {
		r.ListValue.Values = append(
			r.ListValue.Values,
			&structpb.Value{
				Kind: &structpb.Value_StringValue{StringValue: e},
			},
		)
	}
	return r
}

// Test Row generation logics of partialResultSetDecoder.
type partialResultSetDecoderTest struct {
	input    []*sppb.PartialResultSet
	wantF    []*Row
	wantTxID transactionID
	wantTs   time.Time
	wantD    bool
}

// partialResultSetDecoderTests returns test cases that assemble
// PartialResultSets into rows.
func partialResultSetDecoderTests() []partialResultSetDecoderTest {
	return []partialResultSetDecoderTest{
		{
			// Empty input.
			wantD: true,
		},
		// String merging examples.
		{
			// Single KV result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Incomplete partial result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  false,
		},
		{
			// Complete splitted result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
					},
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Multi-row example with splitted row in the middle.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
						{Kind: &structpb.Value_StringValue{StringValue: "A"}},
					},
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "1"}},
						{Kind: &structpb.Value_StringValue{StringValue: "B"}},
						{Kind: &structpb.Value_StringValue{StringValue: "2"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "foo"}},
						{Kind: &structpb.Value_StringValue{StringValue: "bar"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "A"}},
						{Kind: &structpb.Value_StringValue{StringValue: "1"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "B"}},
						{Kind: &structpb.Value_StringValue{StringValue: "2"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Merging example in result_set.proto.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "Hello"}},
						{Kind: &structpb.Value_StringValue{StringValue: "W"}},
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "orl"}},
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "d"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "Hello"}},
						{Kind: &structpb.Value_StringValue{StringValue: "World"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// More complex example showing completing a merge and
			// starting a new merge in the same partialResultSet.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "Hello"}},
						{Kind: &structpb.Value_StringValue{StringValue: "W"}}, // start split in value
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "orld"}}, // complete value
						{Kind: &structpb.Value_StringValue{StringValue: "i"}},    // start split in key
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "s"}}, // complete key
						{Kind: &structpb.Value_StringValue{StringValue: "not"}},
						{Kind: &structpb.Value_StringValue{StringValue: "a"}},
						{Kind: &structpb.Value_StringValue{StringValue: "qu"}}, // split in value
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "estion"}}, // complete value
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "Hello"}},
						{Kind: &structpb.Value_StringValue{StringValue: "World"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "is"}},
						{Kind: &structpb.Value_StringValue{StringValue: "not"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "a"}},
						{Kind: &structpb.Value_StringValue{StringValue: "question"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		// List merging examples.
		{
			// Non-splitting Lists.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvListMeta,
					Values: []*structpb.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
					},
				},
				{
					Values: []*structpb.Value{
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvListMeta.RowType.Fields,
					vals: []*structpb.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantTxID: transactionID{5, 6, 7, 8, 9},
			wantTs:   trxTs,
			wantD:    true,
		},
		{
			// Simple List merge case: splitted string element.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvListMeta,
					Values: []*structpb.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-"),
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{
							Kind: genProtoListValue("2"),
						},
					},
				},
				{
					Values: []*structpb.Value{
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvListMeta.RowType.Fields,
					vals: []*structpb.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantTxID: transactionID{5, 6, 7, 8, 9},
			wantTs:   trxTs,
			wantD:    true,
		},
		{
			// Struct merging is also implemented by List merging. Note that
			// Cloud Spanner uses proto.ListValue to encode Structs as well.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvObjectMeta,
					Values: []*structpb.Value{
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: &structpb.Value_NumberValue{NumberValue: 23}},
										{Kind: genProtoListValue("foo-1", "fo")},
									},
								},
							},
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: genProtoListValue("o-2", "f")},
									},
								},
							},
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*structpb.Value{
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: genProtoListValue("oo-3")},
									},
								},
							},
						},
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: &structpb.Value_NumberValue{NumberValue: 45}},
										{Kind: genProtoListValue("bar-1")},
									},
								},
							},
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvObjectMeta.RowType.Fields,
					vals: []*structpb.Value{
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: &structpb.Value_NumberValue{NumberValue: 23}},
										{Kind: genProtoListValue("foo-1", "foo-2", "foo-3")},
									},
								},
							},
						},
						{
							Kind: &structpb.Value_ListValue{
								ListValue: &structpb.ListValue{
									Values: []*structpb.Value{
										{Kind: &structpb.Value_NumberValue{NumberValue: 45}},
										{Kind: genProtoListValue("bar-1")},
									},
								},
							},
						},
					},
				},
			},
			wantTxID: transactionID{1, 2, 3, 4, 5},
			wantD:    true,
		},
	}
}

func TestPartialResultSetDecoder(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
nextTest:
	for i, test := range partialResultSetDecoderTests() {
		var rows []*Row
		p := &partialResultSetDecoder{}
		for j, v := range test.input {
			rs, _, err := p.add(v)
			if err != nil {
				t.Errorf("test %d.%d: partialResultSetDecoder.add(%v) = %v; want nil", i, j, v, err)
				continue nextTest
			}
			rows = append(rows, rs...)
		}
		if !testEqual(p.ts, test.wantTs) {
			t.Errorf("got transaction(%v), want %v", p.ts, test.wantTs)
		}
		if !testEqual(rows, test.wantF) {
			t.Errorf("test %d: rows=\n%v\n; want\n%v\n; p.row:\n%v\n", i, describeRows(rows), describeRows(test.wantF), p.row)
		}
		if got := p.done(); got != test.wantD {
			t.Errorf("test %d: partialResultSetDecoder.done() = %v", i, got)
		}
	}
}

const (
	// max number of PartialResultSets that will be buffered in tests.
	maxBuffers = 16
)

// setMaxBytesBetweenResumeTokens sets the global maxBytesBetweenResumeTokens to
// a smaller value more suitable for tests. It returns a function which should
// be called to restore the maxBytesBetweenResumeTokens to its old value.
func setMaxBytesBetweenResumeTokens() func() {
	o := atomic.LoadInt32(&maxBytesBetweenResumeTokens)
	atomic.StoreInt32(&maxBytesBetweenResumeTokens, int32(maxBuffers*proto.Size(&sppb.PartialResultSet{
		Metadata: kvMeta,
		Values: []*structpb.Value{
			{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
			{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
		},
	})))
	return func() {
		atomic.StoreInt32(&maxBytesBetweenResumeTokens, o)
	}
}

// keyStr generates key string for kvMeta schema.
func keyStr(i int) string {
	return fmt.Sprintf("foo-%02d", i)
}

// valStr generates value string for kvMeta schema.
func valStr(i int) string {
	return fmt.Sprintf("bar-%02d", i)
}

// Test state transitions of resumableStreamDecoder where state machine ends up
// to a non-blocking state(resumableStreamDecoder.Next returns on non-blocking
// state).
func TestRsdNonblockingStates(t *testing.T) {
	t.Skip("Does not work with the Last flag")

	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	tests := []struct {
		name         string
		resumeTokens [][]byte
		prsErrors    []PartialResultSetExecutionTime
		rpc          func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error)
		sql          string
		// Expected values
		want         []*sppb.PartialResultSet      // PartialResultSets that should be returned to caller
		queue        []*sppb.PartialResultSet      // PartialResultSets that should be buffered
		resumeToken  []byte                        // Resume token that is maintained by resumableStreamDecoder
		stateHistory []resumableStreamDecoderState // State transition history of resumableStreamDecoder
		wantErr      error
	}{
		{
			// unConnected->queueingRetryable->finished
			name:         "unConnected->queueingRetryable->finished",
			resumeTokens: make([][]byte, 2),
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
					},
				},
			},
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(1)}},
					},
				},
			},
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				finished,          // got EOF
			},
		},
		{
			// unConnected->queueingRetryable->aborted
			name:         "unConnected->queueingRetryable->aborted",
			resumeTokens: [][]byte{{}, EncodeResumeToken(1), {}, EncodeResumeToken(2)},
			prsErrors: []PartialResultSetExecutionTime{{
				ResumeToken: EncodeResumeToken(2),
				Err:         status.Error(codes.Unknown, "I quit"),
			}},
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
					},
				},
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(1)}},
					},
					ResumeToken: EncodeResumeToken(1),
				},
			},
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				queueingRetryable, // foo-01, resume token
				queueingRetryable, // got foo-02
				aborted,           // got error
			},
			wantErr: ToSpannerError(status.Errorf(codes.Unknown, "I quit")),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->queueingUnretryable
			name:         "unConnected->queueingRetryable->queueingUnretryable->queueingUnretryable",
			resumeTokens: make([][]byte, maxBuffers+1),
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers+1; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &structpb.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // the internal queue of resumableStreamDecoder fills up
				}
				// the first item fills up the queue and triggers state transition;
				// the second item is received under queueingUnretryable state.
				s = append(s, queueingUnretryable)
				s = append(s, queueingUnretryable)
				return s
			}(),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->aborted
			name: "unConnected->queueingRetryable->queueingUnretryable->aborted",
			resumeTokens: func() (rts [][]byte) {
				rts = make([][]byte, maxBuffers+1)
				rts[maxBuffers] = EncodeResumeToken(1)
				return rts
			}(),
			prsErrors: []PartialResultSetExecutionTime{{
				ResumeToken: EncodeResumeToken(1),
				Err:         status.Error(codes.Unknown, "Just Abort It"),
			}},
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &structpb.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder fills up
				}
				s = append(s, queueingUnretryable) // the last row triggers state change
				s = append(s, aborted)             // Error happens
				return s
			}(),
			wantErr: ToSpannerError(status.Errorf(codes.Unknown, "Just Abort It")),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, c, teardown := setupMockedTestServer(t)
			defer teardown()
			mc, err := c.sc.nextClient()
			if err != nil {
				t.Fatalf("failed to create a grpc client")
			}

			session, err := createSession(mc)
			if err != nil {
				t.Fatalf("failed to create a session")
			}

			if test.rpc == nil {
				test.rpc = func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
					return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
						Session:     session.Name,
						Sql:         test.sql,
						ResumeToken: resumeToken,
					}, opts...)
				}
			}
			md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
			ctx := metadata.NewOutgoingContext(context.Background(), md)
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			r := newResumableStreamDecoder(
				ctx,
				cancel,
				nil,
				test.rpc,
				mc.(*grpcSpannerClient),
				false,
				false,
			)
			st := []resumableStreamDecoderState{}
			var lastErr error
			// Once the expected number of state transitions are observed,
			// send a signal by setting stateDone = true.
			stateDone := false
			// Set stateWitness to listen to state changes.
			hl := len(test.stateHistory) // To avoid data race on test.
			r.stateWitness = func(rs resumableStreamDecoderState) {
				if !stateDone {
					// Record state transitions.
					st = append(st, rs)
					if len(st) == hl {
						lastErr = r.lastErr()
						stateDone = true
					}
				}
			}
			// Let mock server stream given messages to resumableStreamDecoder.
			err = setupStatementResult(t, server, test.sql, len(test.resumeTokens), test.resumeTokens)
			if err != nil {
				t.Fatalf("failed to set up a result for a statement: %v", err)
			}

			for _, et := range test.prsErrors {
				server.TestSpanner.AddPartialResultSetError(
					test.sql,
					et,
				)
			}
			var rs []*sppb.PartialResultSet
			for {
				select {
				case <-ctx.Done():
					if test.stateHistory[len(test.stateHistory)-1] != finished {
						t.Fatal("context cancelled or timeout during test")
					}
				default:
				}
				if stateDone {
					// Check if resumableStreamDecoder carried out expected
					// state transitions.
					if !testEqual(st, test.stateHistory) {
						t.Fatalf("observed state transitions: \n%v\n, want \n%v\n", st, test.stateHistory)
					}
					// Check if resumableStreamDecoder returns expected array of
					// PartialResultSets.
					if !testEqual(rs, test.want) {
						t.Fatalf("received PartialResultSets: \n%v\n, want \n%v\n", rs, test.want)
					}
					// Verify that resumableStreamDecoder's internal buffering is
					// also correct.
					var q []*sppb.PartialResultSet
					for {
						item := r.q.pop()
						if item == nil {
							break
						}
						q = append(q, item.(*sppb.PartialResultSet))
					}
					if !testEqual(q, test.queue) {
						t.Fatalf("PartialResultSets still queued: \n%v\n, want \n%v\n", q, test.queue)
					}
					// Verify resume token.
					if test.resumeToken != nil && !testEqual(r.resumeToken, test.resumeToken) {
						t.Fatalf("Resume token is %v, want %v\n", r.resumeToken, test.resumeToken)
					}
					// Verify error message.
					if !testEqual(lastErr, test.wantErr) {
						t.Fatalf("Error mismatch\n\tGot:  %v\n\tWant: %v", lastErr, test.wantErr)
					}
					return
				}
				// Receive next decoded item.
				if r.next() {
					rs = append(rs, r.get().(*sppb.PartialResultSet))
				}
			}
		})
	}
}

// Test state transitions of resumableStreamDecoder where state machine
// ends up to a blocking state(resumableStreamDecoder.Next blocks
// on blocking state).
func TestRsdBlockingStates(t *testing.T) {
	t.Skip("Does not work with the Last flag")

	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	for _, test := range []struct {
		name         string
		resumeTokens [][]byte
		rpc          func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error)
		sql          string
		// Expected values
		want         []*sppb.PartialResultSet      // PartialResultSets that should be returned to caller
		queue        []*sppb.PartialResultSet      // PartialResultSets that should be buffered
		resumeToken  []byte                        // Resume token that is maintained by resumableStreamDecoder
		stateHistory []resumableStreamDecoderState // State transition history of resumableStreamDecoder
		wantErr      error
	}{
		{
			// unConnected -> unConnected
			name: "unConnected -> unConnected",
			rpc: func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				return nil, status.Errorf(codes.Unavailable, "trust me: server is unavailable")
			},
			sql:          "SELECT * from t_whatever",
			stateHistory: []resumableStreamDecoderState{unConnected, unConnected, unConnected},
			wantErr:      status.Errorf(codes.Unavailable, "trust me: server is unavailable"),
		},
		{
			// unConnected -> queueingRetryable
			name:         "unConnected -> queueingRetryable",
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			stateHistory: []resumableStreamDecoderState{queueingRetryable},
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
				},
			},
		},
		{
			// unConnected->queueingRetryable->queueingRetryable
			name:         "unConnected->queueingRetryable->queueingRetryable",
			resumeTokens: [][]byte{{}, EncodeResumeToken(1), EncodeResumeToken(2), {}},
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
					},
				},
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(1)}},
					},
					ResumeToken: EncodeResumeToken(1),
				},
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(2)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(2)}},
					},
					ResumeToken: EncodeResumeToken(2),
				},
				// The server sends an io.EOF at last and the decoder will
				// flush out all messages in the internal queue.
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(3)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(3)}},
					},
				},
			},
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(3)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(3)}},
					},
				},
			},
			resumeToken: EncodeResumeToken(2),
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				queueingRetryable, // foo-01, resume token
				queueingRetryable, // got foo-02
				queueingRetryable, // foo-02, resume token
				queueingRetryable, // got foo-03
			},
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->queueingRetryable->queueingRetryable
			name: "unConnected->queueingRetryable->queueingUnretryable->queueingRetryable->queueingRetryable",
			resumeTokens: func() (rts [][]byte) {
				rts = make([][]byte, maxBuffers+3)
				rts[maxBuffers+1] = EncodeResumeToken(maxBuffers + 1)
				return rts
			}(),
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				// The server sends an io.EOF at last and the decoder will
				// flush out all messages in the internal queue. Although the
				// last message is supposed to be queued and the decoder waits
				// for the next resume token, an io.EOF leads to a `finished`
				// state that the last message will be removed from the queue
				// and be read by the client side.
				for i := 0; i < maxBuffers+3; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &structpb.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				s[maxBuffers+1].ResumeToken = EncodeResumeToken(maxBuffers + 1)
				return s
			}(),
			resumeToken: EncodeResumeToken(maxBuffers + 1),
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: keyStr(maxBuffers + 2)}},
						{Kind: &structpb.Value_StringValue{StringValue: valStr(maxBuffers + 2)}},
					},
				},
			},
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder filles up
				}
				for i := maxBuffers - 1; i < maxBuffers+1; i++ {
					// the first item fills up the queue and triggers state
					// change; the second item is received under
					// queueingUnretryable state.
					s = append(s, queueingUnretryable)
				}
				s = append(s, queueingUnretryable) // got (maxBuffers+1)th row under Unretryable state
				s = append(s, queueingRetryable)   // (maxBuffers+1)th row has resume token
				s = append(s, queueingRetryable)   // (maxBuffers+2)th row has no resume token
				return s
			}(),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->finished
			name:         "unConnected->queueingRetryable->queueingUnretryable->finished",
			resumeTokens: make([][]byte, maxBuffers),
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*structpb.Value{
							{Kind: &structpb.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &structpb.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder fills up
				}
				s = append(s, queueingUnretryable) // last row triggers state change
				s = append(s, finished)            // query finishes
				return s
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, c, teardown := setupMockedTestServer(t)
			defer teardown()
			mc, err := c.sc.nextClient()
			if err != nil {
				t.Fatalf("failed to create a grpc client")
			}

			session, err := createSession(mc)
			if err != nil {
				t.Fatalf("failed to create a session")
			}

			if test.rpc == nil {
				// Avoid using test.sql directly in closure because for loop changes
				// test.
				sql := test.sql
				test.rpc = func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
					return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
						Session:     session.Name,
						Sql:         sql,
						ResumeToken: resumeToken,
					}, opts...)
				}
			}
			md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
			ctx := metadata.NewOutgoingContext(context.Background(), md)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			r := newResumableStreamDecoder(
				ctx,
				cancel,
				nil,
				test.rpc,
				mc.(*grpcSpannerClient),
				false,
				false,
			)
			// Override backoff to make the test run faster.
			r.backoff = gax.Backoff{
				Initial:    1 * time.Nanosecond,
				Max:        1 * time.Nanosecond,
				Multiplier: 1.3,
			}
			// st is the set of observed state transitions.
			st := []resumableStreamDecoderState{}
			// q is the content of the decoder's partial result queue when expected
			// number of state transitions are done.
			q := []*sppb.PartialResultSet{}
			var lastErr error
			// Once the expected number of state transitions are observed, send a
			// signal to channel stateDone.
			stateDone := make(chan int)
			// Set stateWitness to listen to state changes.
			hl := len(test.stateHistory) // To avoid data race on test.
			r.stateWitness = func(rs resumableStreamDecoderState) {
				select {
				case <-stateDone:
					// Noop after expected number of state transitions
				default:
					// Record state transitions.
					st = append(st, rs)
					if len(st) == hl {
						lastErr = r.lastErr()
						for _, item := range r.q.dump() {
							q = append(q, item.(*sppb.PartialResultSet))
						}
						close(stateDone)
					}
				}
			}
			// Let mock server stream given messages to resumableStreamDecoder.
			err = setupStatementResult(t, server, test.sql, len(test.resumeTokens), test.resumeTokens)
			if err != nil {
				t.Fatalf("failed to set up a result for a statement: %v", err)
			}
			mutex := &sync.Mutex{}
			var rs []*sppb.PartialResultSet
			rowsFetched := make(chan int)
			go func() {
				for {
					if !r.next() {
						// Note that r.Next also exits on context cancel/timeout.
						close(rowsFetched)
						return
					}
					mutex.Lock()
					rs = append(rs, r.get().(*sppb.PartialResultSet))
					mutex.Unlock()
				}
			}()
			// Wait until all rows have been fetched.
			if len(test.want) > 0 {
				select {
				case <-rowsFetched:
				case <-time.After(1 * time.Second):
					t.Fatal("Timeout in waiting for rows to be fetched")
				}
			}
			// Verify that resumableStreamDecoder reaches expected state.
			select {
			case <-stateDone: // Note that at this point, receiver is still blocking on r.next().
				// Check if resumableStreamDecoder carried out expected state
				// transitions.
				if !testEqual(st, test.stateHistory) {
					t.Fatalf("observed state transitions: \n%v\n, want \n%v\n", st, test.stateHistory)
				}
				// Check if resumableStreamDecoder returns expected array of
				// PartialResultSets.
				mutex.Lock()
				defer mutex.Unlock()
				if !testEqual(rs, test.want) {
					t.Fatalf("%s: received PartialResultSets: \n%v\n, want \n%v\n", test.name, rs, test.want)
				}
				// Verify that resumableStreamDecoder's internal buffering is also
				// correct.
				if !testEqual(q, test.queue) {
					t.Fatalf("PartialResultSets still queued: \n%v\n, want \n%v\n", q, test.queue)
				}
				// Verify resume token.
				if test.resumeToken != nil && !testEqual(r.resumeToken, test.resumeToken) {
					t.Fatalf("Resume token is %v, want %v\n", r.resumeToken, test.resumeToken)
				}
				// Verify error message.
				if !testEqual(lastErr, test.wantErr) {
					t.Fatalf("got error %v, want %v", lastErr, test.wantErr)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout in waiting for state change")
			}
		})
	}
}

// sReceiver signals every receiving attempt through a channel, used by
// TestResumeToken to determine if the receiving of a certain PartialResultSet
// will be attempted next.
type sReceiver struct {
	c           chan int
	rpcReceiver sppb.Spanner_ExecuteStreamingSqlClient
}

// Recv() implements streamingReceiver.Recv for sReceiver.
func (sr *sReceiver) Recv() (*sppb.PartialResultSet, error) {
	sr.c <- 1
	return sr.rpcReceiver.Recv()
}

func (sr *sReceiver) Context() context.Context {
	return sr.rpcReceiver.Context()
}

// waitn waits for nth receiving attempt from now on, until the signal for nth
// Recv() attempts is received or timeout. Note that because the way stream()
// works, the signal for the nth Recv() means that the previous n - 1
// PartialResultSets has already been returned to caller or queued, if no error
// happened.
func (sr *sReceiver) waitn(n int) error {
	for i := 0; i < n; i++ {
		select {
		case <-sr.c:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout in waiting for %v-th Recv()", i+1)
		}
	}
	return nil
}

// Test the handling of resumableStreamDecoder.bytesBetweenResumeTokens.
func TestQueueBytes(t *testing.T) {
	t.Skip("Does not work with the Last flag")

	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	rt1 := EncodeResumeToken(1)
	rt2 := EncodeResumeToken(2)
	rt3 := EncodeResumeToken(3)
	resumeTokens := [][]byte{rt1, rt1, rt1, rt2, rt2, rt3}
	err = setupStatementResult(t, server, "SELECT t.key key, t.value value FROM t_mock t", len(resumeTokens), resumeTokens)
	if err != nil {
		t.Fatalf("failed to set up a result for a statement: %v", err)
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	sr := &sReceiver{
		c: make(chan int, 1000), // will never block in this test
	}
	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	decoder := newResumableStreamDecoder(
		ctx,
		cancel,
		nil,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			r, err := mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         "SELECT t.key key, t.value value FROM t_mock t",
				ResumeToken: resumeToken,
			}, opts...)
			sr.rpcReceiver = r
			return sr, err
		},
		mc.(*grpcSpannerClient),
		false,
		false,
	)

	sizeOfPRS := proto.Size(&sppb.PartialResultSet{
		Metadata: kvMeta,
		Values: []*structpb.Value{
			{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
			{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
		},
		ResumeToken: rt1,
	})

	decoder.next()
	decoder.next()
	decoder.next()
	if got, want := decoder.bytesBetweenResumeTokens, int32(2*sizeOfPRS); got != want {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want %v", got, want)
	}

	decoder.next()
	if decoder.bytesBetweenResumeTokens != 0 {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want 0", decoder.bytesBetweenResumeTokens)
	}

	decoder.next()
	if got, want := decoder.bytesBetweenResumeTokens, int32(sizeOfPRS); got != want {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want %v", got, want)
	}

	decoder.next()
	if decoder.bytesBetweenResumeTokens != 0 {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want 0", decoder.bytesBetweenResumeTokens)
	}
}

// Verify that client can deal with resume token correctly
func TestResumeToken(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	query := "SELECT t.key key, t.value value FROM t_mock t"
	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	rt1 := EncodeResumeToken(1)
	rt2 := EncodeResumeToken(2)
	resumeTokens := make([][]byte, 3+maxBuffers)
	resumeTokens[1] = rt1
	resumeTokens[3+maxBuffers-1] = rt2
	err = setupStatementResult(t, server, query, len(resumeTokens), resumeTokens)
	if err != nil {
		t.Fatalf("failed to set up a result for a statement: %v", err)
	}

	// The first error will be retried.
	server.TestSpanner.AddPartialResultSetError(
		query,
		PartialResultSetExecutionTime{
			ResumeToken: rt1,
			Err:         status.Error(codes.Unavailable, "mock server unavailable"),
		},
	)
	// The second error will not be retried because maxBytesBetweenResumeTokens
	// is reached and the state of resumableStreamDecoder:
	// queueingRetryable -> queueingUnretryable. The query will just fail.
	server.TestSpanner.AddPartialResultSetError(
		query,
		PartialResultSetExecutionTime{
			ResumeToken: rt2,
			Err:         status.Error(codes.Unavailable, "mock server wants some sleep"),
		},
	)

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	sr := &sReceiver{
		c: make(chan int, 1000), // will never block in this test
	}
	rows := []*Row{}

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	streaming := func() *RowIterator {
		return stream(ctx, nil,
			c.metricsTracerFactory,
			func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				r, err := mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Session:     session.Name,
					Sql:         query,
					ResumeToken: resumeToken,
				}, opts...)
				sr.rpcReceiver = r
				return sr, err
			},
			nil,
			func(error) {}, mc.(*grpcSpannerClient))
	}

	// Establish a stream to mock cloud spanner server.
	iter := streaming()
	defer iter.Stop()
	var row *Row

	// Read first two rows.
	for i := 0; i < 3; i++ {
		row, err = iter.Next()
		if err != nil {
			t.Fatalf("failed to get next value: %v", err)
		}
		rows = append(rows, detachRow(row))
	}

	want := []*Row{
		{
			fields: kvMeta.RowType.Fields,
			vals: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
				{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
			},
		},
		{
			fields: kvMeta.RowType.Fields,
			vals: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: keyStr(1)}},
				{Kind: &structpb.Value_StringValue{StringValue: valStr(1)}},
			},
		},
		{
			fields: kvMeta.RowType.Fields,
			vals: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: keyStr(2)}},
				{Kind: &structpb.Value_StringValue{StringValue: valStr(2)}},
			},
		},
	}
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n; but want\n%v\n", rows, want)
	}

	// Trigger state change of resumableStreamDecoder:
	// queueingRetryable -> queueingUnretryable
	for i := 0; i < maxBuffers-1; i++ {
		row, err = iter.Next()
		if err != nil {
			t.Fatalf("failed to get next value: %v", err)
		}
		rows = append(rows, detachRow(row))
	}

	// Since resumableStreamDecoder is already at queueingUnretryable state,
	// query will just fail.
	_, err = iter.Next()
	if wantErr := spannerErrorf(codes.Unavailable, "mock server wants some sleep"); !testEqual(err, wantErr) {
		t.Fatalf("stream() returns error: %v, but want error: %v", err, wantErr)
	}

	// Let server send two rows without resume token.
	resumeTokens = make([][]byte, 2)
	err = setupStatementResult(t, server, query, len(resumeTokens), resumeTokens)
	if err != nil {
		t.Fatalf("failed to set up a result for a statement: %v", err)
	}

	// Reconnect to mock Cloud Spanner.
	rows = []*Row{}
	iter = streaming()
	defer iter.Stop()

	for i := 0; i < 2; i++ {
		row, err = iter.Next()
		if err != nil {
			t.Fatalf("failed to get next value: %v", err)
		}
		rows = append(rows, detachRow(row))
	}

	// Verify if a normal server side EOF flushes all queued rows.
	want = []*Row{
		{
			fields: kvMeta.RowType.Fields,
			vals: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: keyStr(0)}},
				{Kind: &structpb.Value_StringValue{StringValue: valStr(0)}},
			},
		},
		{
			fields: kvMeta.RowType.Fields,
			vals: []*structpb.Value{
				{Kind: &structpb.Value_StringValue{StringValue: keyStr(1)}},
				{Kind: &structpb.Value_StringValue{StringValue: valStr(1)}},
			},
		},
	}
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n; but want\n%v\n", rows, want)
	}
}

// Verify that streaming query get retried upon real gRPC server transport
// failures.
func TestGrpcReconnect(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	// Simulate an unavailable error to interrupt the stream of PartialResultSet
	// in order to test the grpc retrying mechanism.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
		},
	)

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	// The retry is counted from the second call.
	r := -1
	// Establish a stream to mock cloud spanner server.
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			r++
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	if r != 1 {
		t.Errorf("retry count = %v, want 1", r)
	}
}

func TestRetryResourceExhaustedWithoutRetryInfo(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	// Simulate an ResourceExhausted error to interrupt the stream of PartialResultSet
	// in order to test the grpc retrying mechanism.
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         status.Errorf(codes.ResourceExhausted, "server is unavailable"),
		},
	)

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	// The retry is counted from the second call.
	r := -1
	// Establish a stream to mock cloud spanner server.
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			r++
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	if r != 0 {
		t.Errorf("retry count = %v, want 0", r)
	}
}

// Verify that streaming query get retried upon ResourceExhausted real gRPC server
// transport failures.
func TestRetryResourceExhaustedWithRetryInfo(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	// Simulate an ResourceExhausted error to interrupt the stream of PartialResultSet
	// in order to test the grpc retrying mechanism.
	st := status.New(codes.ResourceExhausted, "server is unavailable")
	retry := &errdetails.RetryInfo{
		RetryDelay: durationpb.New(time.Nanosecond),
	}
	st, _ = st.WithDetails(retry)
	server.TestSpanner.AddPartialResultSetError(
		SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		PartialResultSetExecutionTime{
			ResumeToken: EncodeResumeToken(2),
			Err:         st.Err(),
		},
	)

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	// The retry is counted from the second call.
	r := -1
	// Establish a stream to mock cloud spanner server.
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			r++
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	if r != 1 {
		t.Errorf("retry count = %v, want 1", r)
	}
}

// Test cancel/timeout for client operations.
func TestCancelTimeout(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	server, c, teardown := setupMockedTestServer(t)
	defer teardown()
	server.TestSpanner.PutExecutionTime(
		MethodExecuteStreamingSql,
		SimulatedExecutionTime{MinimumExecutionTime: 1 * time.Second},
	)
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}
	done := make(chan int)

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	// Test cancelling query.
	ctx, cancel := context.WithCancel(metadata.NewOutgoingContext(context.Background(), md))
	go func() {
		// Establish a stream to mock cloud spanner server.
		iter := stream(ctx, nil, c.metricsTracerFactory,
			func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Session:     session.Name,
					Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
					ResumeToken: resumeToken,
				}, opts...)
			},
			nil,
			func(error) {}, mc.(*grpcSpannerClient))
		defer iter.Stop()
		for {
			_, err = iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				done <- 0
				break
			}
		}
	}()
	cancel()
	select {
	case <-done:
		if ErrCode(err) != codes.Canceled {
			t.Errorf("streaming query is canceled and returns error %v, want error code %v", err, codes.Canceled)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("query doesn't exit timely after being cancelled")
	}

	// Test query timeout.
	ctx, cancel = context.WithTimeout(metadata.NewOutgoingContext(context.Background(), md), 100*time.Millisecond)
	defer cancel()
	go func() {
		// Establish a stream to mock cloud spanner server.
		iter := stream(ctx, nil, c.metricsTracerFactory,
			func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Session:     session.Name,
					Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
					ResumeToken: resumeToken,
				}, opts...)
			},
			nil,
			func(error) {}, mc.(*grpcSpannerClient))
		defer iter.Stop()
		for {
			_, err = iter.Next()
			if err == iterator.Done {
				err = nil
				break
			}
			if err != nil {
				break
			}
		}
		done <- 0
	}()
	select {
	case <-done:
		if wantErr := codes.DeadlineExceeded; ErrCode(err) != wantErr {
			t.Errorf("streaming query timeout returns error %v, want error code %v", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("query doesn't timeout as expected")
	}
}

func setupStatementResult(t *testing.T, server *MockedSpannerInMemTestServer, stmt string, rowCount int, resumeTokens [][]byte) error {
	selectValues := make([][]string, rowCount)
	for i := 0; i < rowCount; i++ {
		selectValues[i] = []string{keyStr(i), valStr(i)}
	}

	rows := make([]*structpb.ListValue, len(selectValues))
	for i, values := range selectValues {
		rowValues := make([]*structpb.Value, len(kvMeta.RowType.Fields))
		for j, value := range values {
			rowValues[j] = &structpb.Value{
				Kind: &structpb.Value_StringValue{StringValue: value},
			}
		}
		rows[i] = &structpb.ListValue{
			Values: rowValues,
		}
	}
	resultSet := &sppb.ResultSet{
		Metadata: kvMeta,
		Rows:     rows,
	}
	result := &StatementResult{
		Type:         StatementResultResultSet,
		ResultSet:    resultSet,
		ResumeTokens: resumeTokens,
	}
	return server.TestSpanner.PutStatementResult(stmt, result)
}

func TestSkippingTrailersForExecuteStreamingSql(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)
	res := server.CreateSingersResults(4, true)
	sql := "SELECT SingerId, AlbumId, AlbumTitle FROM Albums WHERE 1=2"
	err := server.TestSpanner.PutStatementResult(sql, res)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Query(ctx, NewStatement(sql))
	defer iter.Stop()
	var noOfRows int
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		noOfRows++
		if err != nil {
			t.Fatal(err)
		}

		if row.ColumnValue(0).GetStringValue() != fmt.Sprintf("%v", noOfRows) {
			t.Fatalf("ID Mismatch in the result")
		}
	}
	if g, w := noOfRows, 4; g != w {
		t.Fatalf("num rows mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestSkippingTrailersForStreamingRead(t *testing.T) {
	t.Parallel()

	server, client, teardown := setupMockedTestServer(t)

	res := server.CreateSingersResults(5, true)
	sql := "SELECT SingerId, AlbumId, AlbumTitle FROM Albums"
	err := server.TestSpanner.PutStatementResult(sql, res)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()
	ctx := context.Background()
	iter := client.Single().Read(ctx, "Albums", KeySets(Key{"foo"}), []string{"SingerId", "AlbumId", "AlbumTitle"})
	defer iter.Stop()
	var noOfRows int
	for {
		row, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		noOfRows++
		if err != nil {
			t.Fatal(err)
		}

		if row.ColumnValue(0).GetStringValue() != fmt.Sprintf("%v", noOfRows) {
			t.Fatalf("ID Mismatch in the result")
		}
	}
	if g, w := noOfRows, 5; g != w {
		t.Fatalf("num rows mismatch\n Got: %v\nWant: %v", g, w)
	}
}

func TestRowIteratorDo(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	_, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	nRows := 0
	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	err = iter.Do(func(r *Row) error { nRows++; return nil })
	if err != nil {
		t.Errorf("Using Do: %v", err)
	}
	if nRows != 3 {
		t.Errorf("got %d rows, want 3", nRows)
	}
}

func TestRowIteratorDoWithError(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	_, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	injected := errors.New("Failed iterator")
	err = iter.Do(func(r *Row) error { return injected })
	if err != injected {
		t.Errorf("got <%v>, want <%v>", err, injected)
	}
}

func TestIteratorStopEarly(t *testing.T) {
	ctx := context.Background()
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()

	_, c, teardown := setupMockedTestServer(t)
	defer teardown()
	mc, err := c.sc.nextClient()
	if err != nil {
		t.Fatalf("failed to create a grpc client")
	}

	session, err := createSession(mc)
	if err != nil {
		t.Fatalf("failed to create a session")
	}

	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx = metadata.NewOutgoingContext(ctx, md)
	iter := stream(ctx, nil, c.metricsTracerFactory,
		func(ct context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Session:     session.Name,
				Sql:         SelectSingerIDAlbumIDAlbumTitleFromAlbums,
				ResumeToken: resumeToken,
			}, opts...)
		},
		nil,
		func(error) {}, mc.(*grpcSpannerClient))
	_, err = iter.Next()
	if err != nil {
		t.Fatalf("before Stop: %v", err)
	}
	iter.Stop()
	// Stop sets r.err to the FailedPrecondition error "Next called after Stop".
	_, err = iter.Next()
	if g, w := ErrCode(err), codes.FailedPrecondition; g != w {
		t.Errorf("after Stop: got: %v, want: %v", g, w)
	}
}

func TestIteratorWithError(t *testing.T) {
	injected := errors.New("Failed iterator")
	iter := RowIterator{err: injected}
	defer iter.Stop()
	if _, err := iter.Next(); err != injected {
		t.Fatalf("Expected error: %v, got %v", injected, err)
	}
}

func createSession(client spannerClient) (*sppb.Session, error) {
	var formattedDatabase string = fmt.Sprintf("projects/%s/instances/%s/databases/%s", "[PROJECT]", "[INSTANCE]", "[DATABASE]")
	var request = &sppb.CreateSessionRequest{
		Database: formattedDatabase,
		Session:  &sppb.Session{},
	}
	ctx := context.Background()
	md := metadata.Pairs(resourcePrefixHeader, "projects/p/instances/i/databases/d")
	ctx = metadata.NewOutgoingContext(ctx, md)
	return client.CreateSession(ctx, request)
}

// partialResultSetsReceiver is a streamingReceiver that returns the given
// PartialResultSets in order, followed by io.EOF.
type partialResultSetsReceiver struct {
	prs []*sppb.PartialResultSet
}

func (r *partialResultSetsReceiver) Recv() (*sppb.PartialResultSet, error) {
	if len(r.prs) == 0 {
		return nil, io.EOF
	}
	prs := r.prs[0]
	r.prs = r.prs[1:]
	return prs, nil
}

func (r *partialResultSetsReceiver) Context() context.Context {
	return context.Background()
}

func TestRowIteratorNextBufferedRowsDoesNotAllocate(t *testing.T) {
	const rows = 200
	values := make([]*structpb.Value, rows)
	for i := range values {
		values[i] = structpb.NewStringValue(fmt.Sprint(i))
	}
	prs := &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "Value", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
		}}},
		Values:      values,
		ResumeToken: EncodeResumeToken(rows),
	}
	_, clientMetricsProvider := newTestMeterProvider()

	for _, test := range []struct {
		name                  string
		clientMetricsProvider otelmetric.MeterProvider
		wantEnabled           bool
	}{
		{name: "metrics disabled"},
		{name: "metrics enabled", clientMetricsProvider: clientMetricsProvider, wantEnabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory, err := newBuiltinMetricsTracerFactory(context.Background(), "projects/p/instances/i/databases/d", "identity", false, false, noop.NewMeterProvider(), test.clientMetricsProvider)
			if err != nil {
				t.Fatalf("failed to create metrics tracer factory: %v", err)
			}
			if factory.enabled != test.wantEnabled {
				t.Fatalf("metrics tracer factory enabled = %v, want %v", factory.enabled, test.wantEnabled)
			}
			receiver := &partialResultSetsReceiver{prs: []*sppb.PartialResultSet{prs}}
			iter := stream(context.Background(), nil, factory,
				func(context.Context, []byte, ...gax.CallOption) (streamingReceiver, error) {
					return receiver, nil
				},
				nil,
				func(error) {}, &grpcSpannerClient{nthRequest: new(atomic.Uint32)})
			defer iter.Stop()

			// The first call receives the PartialResultSet and buffers its rows.
			if _, err := iter.Next(); err != nil {
				t.Fatalf("Next() failed: %v", err)
			}
			allocs := testing.AllocsPerRun(rows/2, func() {
				if _, err := iter.Next(); err != nil {
					t.Fatalf("Next() failed: %v", err)
				}
			})
			if allocs != 0 {
				t.Errorf("Next() on a buffered row allocated %v times, want 0", allocs)
			}
		})
	}
}

// streamingQueryMetrics returns the built-in metrics recorded for
// ExecuteStreamingSql, and any recorded without a method, keyed by metric
// name, method and status. Counters are summed and histograms report their
// number of recordings.
func streamingQueryMetrics(t *testing.T, rm metricdata.ResourceMetrics) map[string]int64 {
	t.Helper()
	return builtInMetricsForMethod(t, rm, "Spanner.ExecuteStreamingSql")
}

// builtInMetricsForMethod returns the built-in metrics recorded for method,
// and any recorded without a method, keyed by metric name, method and status.
// Counters are summed and histograms report their number of recordings.
func builtInMetricsForMethod(t *testing.T, rm metricdata.ResourceMetrics, wantMethod string) map[string]int64 {
	t.Helper()
	got := make(map[string]int64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			name := strings.TrimPrefix(m.Name, clientMetricsPrefix)
			add := func(attrs attribute.Set, value int64) {
				method, ok := attrs.Value(metricLabelKeyMethod)
				if !ok || (method.AsString() != wantMethod && method.AsString() != "") {
					return
				}
				rpcStatus, _ := attrs.Value(metricLabelKeyStatus)
				got[fmt.Sprintf("%s method=%q status=%s", name, method.AsString(), rpcStatus.AsString())] += value
			}
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					add(point.Attributes, point.Value)
				}
			case metricdata.Histogram[float64]:
				for _, point := range data.DataPoints {
					add(point.Attributes, int64(point.Count))
				}
			}
		}
	}
	return got
}

// TestRowIteratorRecordsBuiltInMetrics verifies the built-in metrics that
// RowIterator records for a streaming query. The result stream is one
// operation that ends when Next returns iterator.Done or an error, or when the
// iterator is stopped, and every stream that is opened for it is an attempt.
func TestRowIteratorRecordsBuiltInMetrics(t *testing.T) {
	const singleRowSQL = "SELECT Value FROM SingleRow"
	singleRowResult := &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "Value", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			}}},
			Rows: []*structpb.ListValue{{Values: []*structpb.Value{structpb.NewStringValue("1")}}},
		},
		SetLastFlag: true,
	}
	const emptySQL = "SELECT Value FROM Empty"
	emptyResult := &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: singleRowResult.ResultSet.Metadata,
		},
	}
	readAll := func(iter *RowIterator) (rows int, err error) {
		for {
			if _, err = iter.Next(); err != nil {
				return rows, err
			}
			rows++
		}
	}
	readWithDo := func(iter *RowIterator) (rows int, err error) {
		err = iter.Do(func(*Row) error {
			rows++
			return nil
		})
		return rows, err
	}
	stopBeforeNext := func(iter *RowIterator) (int, error) {
		iter.Stop()
		return 0, nil
	}
	errDoCallback := errors.New("callback failed")
	doCallbackError := func(iter *RowIterator) (rows int, err error) {
		err = iter.Do(func(*Row) error {
			rows++
			return errDoCallback
		})
		return rows, err
	}
	stopAfterFirstRow := func(iter *RowIterator) (int, error) {
		defer iter.Stop()
		_, err := iter.Next()
		return 1, err
	}
	op := func(code string, attempts int64) map[string]int64 {
		return map[string]int64{
			`attempt_count method="Spanner.ExecuteStreamingSql" status=` + code:       attempts,
			`operation_count method="Spanner.ExecuteStreamingSql" status=` + code:     1,
			`operation_latencies method="Spanner.ExecuteStreamingSql" status=` + code: 1,
		}
	}
	attempts := func(want map[string]int64, codes ...string) map[string]int64 {
		for _, code := range codes {
			want[`attempt_latencies method="Spanner.ExecuteStreamingSql" status=`+code]++
			want[`gfe_connectivity_error_count method="Spanner.ExecuteStreamingSql" status=`+code]++
		}
		return want
	}

	for _, test := range []struct {
		name                string
		sql                 string
		canceled            bool
		failFirstStreamOpen bool
		executionTime       *SimulatedExecutionTime
		streamErr           *PartialResultSetExecutionTime
		iterate             func(*RowIterator) (int, error)
		wantRows            int
		wantCode            codes.Code
		want                map[string]int64
	}{
		{
			name:     "no rows",
			sql:      emptySQL,
			iterate:  readAll,
			wantRows: 0,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:     "multiple rows",
			iterate:  readAll,
			wantRows: 3,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:     "multiple rows with Do",
			iterate:  readWithDo,
			wantRows: 3,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:     "error returned by Do callback",
			iterate:  doCallbackError,
			wantRows: 1,
			wantCode: codes.Unknown,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:    "stopped before first Next",
			iterate: stopBeforeNext,
			want:    map[string]int64{},
		},
		{
			name:                "retryable error opening first stream",
			failFirstStreamOpen: true,
			iterate:             readAll,
			wantRows:            3,
			want:                attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name:     "context canceled before first stream",
			canceled: true,
			iterate:  readAll,
			wantCode: codes.Canceled,
			want:     attempts(op("Canceled", 1), "Canceled"),
		},
		{
			name:          "retryable error before first row",
			executionTime: &SimulatedExecutionTime{Errors: []error{status.Error(codes.Unavailable, "unavailable")}},
			iterate:       readAll,
			wantRows:      3,
			want:          attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name: "stream resumed after retryable error",
			streamErr: &PartialResultSetExecutionTime{
				ResumeToken: EncodeResumeToken(2),
				Err:         status.Error(codes.Unavailable, "unavailable"),
			},
			iterate:  readAll,
			wantRows: 3,
			want:     attempts(op("OK", 2), "Unavailable", "OK"),
		},
		{
			name:     "stopped before end of stream",
			iterate:  stopAfterFirstRow,
			wantRows: 1,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:     "stopped after end of stream",
			sql:      singleRowSQL,
			iterate:  stopAfterFirstRow,
			wantRows: 1,
			want:     attempts(op("OK", 1), "OK"),
		},
		{
			name:          "non-retryable error before first row",
			executionTime: &SimulatedExecutionTime{Errors: []error{status.Error(codes.InvalidArgument, "invalid")}},
			iterate:       readAll,
			wantCode:      codes.InvalidArgument,
			want:          attempts(op("InvalidArgument", 1), "InvalidArgument"),
		},
		{
			name: "non-retryable error after first row",
			streamErr: &PartialResultSetExecutionTime{
				ResumeToken: EncodeResumeToken(2),
				Err:         status.Error(codes.InvalidArgument, "invalid"),
			},
			iterate:  readAll,
			wantRows: 1,
			wantCode: codes.InvalidArgument,
			want:     attempts(op("InvalidArgument", 1), "InvalidArgument"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, provider := newTestMeterProvider()
			var opts []option.ClientOption
			if test.failFirstStreamOpen {
				var opened bool
				opts = append(opts, option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(
					func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
						if strings.HasSuffix(method, "/ExecuteStreamingSql") && !opened {
							opened = true
							return nil, status.Error(codes.Unavailable, "unavailable")
						}
						return streamer(ctx, desc, cc, method, opts...)
					})))
			}
			server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{DisableNativeMetrics: true, ClientMetricsProvider: provider}, opts)
			defer teardown()
			if err := server.TestSpanner.PutStatementResult(singleRowSQL, singleRowResult); err != nil {
				t.Fatal(err)
			}
			if err := server.TestSpanner.PutStatementResult(emptySQL, emptyResult); err != nil {
				t.Fatal(err)
			}
			sql := test.sql
			if sql == "" {
				sql = SelectSingerIDAlbumIDAlbumTitleFromAlbums
			}
			if test.executionTime != nil {
				server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, *test.executionTime)
			}
			if test.streamErr != nil {
				server.TestSpanner.AddPartialResultSetError(sql, *test.streamErr)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			iter := client.Single().Query(ctx, NewStatement(sql))
			defer iter.Stop()
			if test.canceled {
				cancel()
			}
			gotRows, err := test.iterate(iter)
			var gotCode codes.Code
			if err != nil && err != iterator.Done {
				gotCode = ErrCode(err)
			}
			if g, w := gotCode, test.wantCode; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			if gotRows != test.wantRows {
				t.Fatalf("row count mismatch\n Got: %v\nWant: %v", gotRows, test.wantRows)
			}
			got := streamingQueryMetrics(t, collectTestMetrics(t, reader))
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("recorded metrics mismatch (-want +got):\n%s", diff)
			}

			// The operation ends only once.
			iter.Next()
			iter.Stop()
			if diff := cmp.Diff(got, streamingQueryMetrics(t, collectTestMetrics(t, reader))); diff != "" {
				t.Errorf("Next() and Stop() after the end recorded metrics (-before +after):\n%s", diff)
			}
		})
	}
}

func TestRowIteratorOperationLatencyIncludesApplicationTime(t *testing.T) {
	const pause = 20 * time.Millisecond
	reader, provider := newTestMeterProvider()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ClientMetricsProvider: provider})
	defer teardown()

	iter := client.Single().Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	var rows int
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("Next() failed: %v", err)
		}
		rows++
		time.Sleep(pause)
	}

	m := requireTestMetric(t, collectTestMetrics(t, reader), clientMetricsPrefix+metricNameOperationLatencies)
	var got float64
	for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
		if method, _ := point.Attributes.Value(metricLabelKeyMethod); method.AsString() == "Spanner.ExecuteStreamingSql" {
			got += point.Sum
		}
	}
	if want := float64(time.Duration(rows) * pause / time.Millisecond); got < want {
		t.Errorf("operation latency = %vms, want at least %vms", got, want)
	}
}

func TestRowIteratorNextErrorWithoutTracerDoesNotAllocate(t *testing.T) {
	_, provider := newTestMeterProvider()
	factory, err := newBuiltinMetricsTracerFactory(context.Background(), "projects/p/instances/i/databases/d", "identity", false, false, noop.NewMeterProvider(), provider)
	if err != nil {
		t.Fatalf("failed to create metrics tracer factory: %v", err)
	}
	for _, test := range []struct {
		name string
		mt   *builtinMetricsTracer
	}{
		{name: "metrics disabled"},
		{name: "operation already ended", mt: factory.newBuiltinMetricsTracer(context.Background())},
	} {
		t.Run(test.name, func(t *testing.T) {
			iter := &RowIterator{
				err:     spannerErrorf(codes.InvalidArgument, "invalid"),
				streamd: &resumableStreamDecoder{streamOperationMetrics: streamOperationMetrics{mt: test.mt}},
			}
			// The first call ends the operation, if there is one.
			iter.Next()
			allocs := testing.AllocsPerRun(100, func() {
				iter.Next()
			})
			if allocs != 0 {
				t.Errorf("Next() returning an error allocated %v times, want 0", allocs)
			}
		})
	}
}

func TestRowIteratorDoesNotRecordAttemptsRejectedBeforeRPC(t *testing.T) {
	for _, test := range []struct {
		name         string
		rowsBefore   int
		wantRequests int
		want         map[string]int64
	}{
		{
			name: "rejected before first stream",
			want: map[string]int64{},
		},
		{
			name:         "resume rejected after stream error",
			rowsBefore:   1,
			wantRequests: 1,
			// attempt_count has the status of the last attempt, which is the
			// rejected resume.
			want: map[string]int64{
				`attempt_count method="Spanner.ExecuteStreamingSql" status=FailedPrecondition`:         1,
				`attempt_latencies method="Spanner.ExecuteStreamingSql" status=Unavailable`:            1,
				`gfe_connectivity_error_count method="Spanner.ExecuteStreamingSql" status=Unavailable`: 1,
				`operation_count method="Spanner.ExecuteStreamingSql" status=FailedPrecondition`:       1,
				`operation_latencies method="Spanner.ExecuteStreamingSql" status=FailedPrecondition`:   1,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, provider := newTestMeterProvider()
			server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, ClientMetricsProvider: provider})
			defer teardown()
			ctx := context.Background()
			tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
			if err != nil {
				t.Fatal(err)
			}
			sql := SelectSingerIDAlbumIDAlbumTitleFromAlbums
			server.TestSpanner.AddPartialResultSetError(sql, PartialResultSetExecutionTime{
				ResumeToken: EncodeResumeToken(2),
				Err:         status.Error(codes.Unavailable, "unavailable"),
			})
			iter := tx.Query(ctx, NewStatement(sql))
			defer iter.Stop()
			for i := 0; i < test.rowsBefore; i++ {
				if _, err := iter.Next(); err != nil {
					t.Fatalf("Next() failed: %v", err)
				}
			}
			// Rolling back the transaction makes the query reject opening or
			// resuming its stream before sending an RPC.
			tx.Rollback(ctx)
			for err == nil {
				_, err = iter.Next()
			}
			if g, w := ErrCode(err), codes.FailedPrecondition; g != w {
				t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
			}
			requests := requestsOfType(drainRequestsFromServer(server.TestSpanner), reflect.TypeOf(&sppb.ExecuteSqlRequest{}))
			if g, w := len(requests), test.wantRequests; g != w {
				t.Fatalf("ExecuteStreamingSql request count mismatch\n Got: %v\nWant: %v", g, w)
			}
			if diff := cmp.Diff(test.want, streamingQueryMetrics(t, collectTestMetrics(t, reader))); diff != "" {
				t.Errorf("recorded metrics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// statusCountingError counts how often its status is read.
type statusCountingError struct {
	code  codes.Code
	calls *int
}

func (e statusCountingError) Error() string { return e.code.String() }

func (e statusCountingError) GRPCStatus() *status.Status {
	*e.calls++
	return status.New(e.code, e.code.String())
}

func TestResumableStreamDecoderDisabledMetricsDoNotReadErrorStatus(t *testing.T) {
	_, provider := newTestMeterProvider()
	enabled, err := newBuiltinMetricsTracerFactory(context.Background(), "projects/p/instances/i/databases/d", "identity", false, false, noop.NewMeterProvider(), provider)
	if err != nil {
		t.Fatalf("failed to create metrics tracer factory: %v", err)
	}
	for _, test := range []struct {
		name    string
		openErr bool
	}{
		{name: "stream open error", openErr: true},
		{name: "receive error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// statusReads returns how often the error status is read while
			// the decoder fails with a non-retryable error.
			statusReads := func(factory *builtinMetricsTracerFactory) int {
				var calls int
				streamErr := statusCountingError{code: codes.InvalidArgument, calls: &calls}
				d := newResumableStreamDecoder(context.Background(), nil, nil,
					func(context.Context, []byte, ...gax.CallOption) (streamingReceiver, error) {
						if test.openErr {
							return nil, streamErr
						}
						return &errorReceiver{err: streamErr}, nil
					},
					&grpcSpannerClient{nthRequest: new(atomic.Uint32)}, true, false)
				d.meterTracerFactory = factory
				if d.next() {
					t.Fatal("next() succeeded, want error")
				}
				return calls
			}
			// Enabled metrics read the status once to record the attempt.
			if g, w := statusReads(nil), statusReads(enabled)-1; g != w {
				t.Errorf("status reads with metrics disabled = %v, want %v", g, w)
			}
		})
	}
}

// errorReceiver is a streamingReceiver that fails with err.
type errorReceiver struct {
	err error
}

func (r *errorReceiver) Recv() (*sppb.PartialResultSet, error) { return nil, r.err }

func (r *errorReceiver) Context() context.Context { return context.Background() }
