// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"context"
	"io"
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestSendOptimizer(t *testing.T) {

	exampleReq := &storagepb.AppendRowsRequest{
		WriteStream: "foo",
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: [][]byte{[]byte("row_data")},
				},
				WriterSchema: &storagepb.ProtoSchema{
					ProtoDescriptor: protodesc.ToDescriptorProto((&testdata.SimpleMessageProto2{}).ProtoReflect().Descriptor()),
				},
			},
		},
		TraceId: "trace_id",
	}

	ctx := context.Background()

	var testCases = []struct {
		description string
		optimizer   sendOptimizer
		reqs        []*pendingWrite
		sendResults []error
		wantReqs    []*storagepb.AppendRowsRequest
	}{
		{
			description: "passthrough-optimizer",
			optimizer:   &passthroughOptimizer{},
			reqs: func() []*pendingWrite {
				dv := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("foo")})
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
				}
			}(),
			sendResults: []error{
				nil,
				io.EOF,
				io.EOF,
			},
			wantReqs: []*storagepb.AppendRowsRequest{
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
			},
		},
		{
			description: "simplex no errors",
			optimizer:   &simplexOptimizer{},
			reqs: func() []*pendingWrite {
				dv := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("foo")})
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
				}
			}(),
			sendResults: []error{
				nil,
				nil,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 3)
				// first has no redactions.
				want[0] = proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				req := proto.Clone(want[0]).(*storagepb.AppendRowsRequest)
				req.GetProtoRows().WriterSchema = nil
				req.TraceId = ""
				req.WriteStream = ""
				want[1] = req
				// previous had errors, so unredacted.
				want[2] = req
				return want
			}(),
		},
		{
			description: "simplex w/partial errors",
			optimizer:   &simplexOptimizer{},
			reqs: func() []*pendingWrite {
				dv := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("foo")})
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
				}
			}(),
			sendResults: []error{
				nil,
				io.EOF,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 3)
				want[0] = proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				req := proto.Clone(want[0]).(*storagepb.AppendRowsRequest)
				req.GetProtoRows().WriterSchema = nil
				req.TraceId = ""
				req.WriteStream = ""
				want[1] = req
				want[2] = want[0]
				return want
			}(),
		},
		{
			description: "multiplex single all errors",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				dv := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("foo")})
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
				}
			}(),
			sendResults: []error{
				io.EOF,
				io.EOF,
				io.EOF,
			},
			wantReqs: []*storagepb.AppendRowsRequest{
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReq).(*storagepb.AppendRowsRequest),
			},
		},
		{
			description: "multiplex single no errors",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				dv := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("foo")})
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), dv),
				}
			}(),
			sendResults: []error{
				nil,
				nil,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 3)
				want[0] = proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				req := proto.Clone(want[0]).(*storagepb.AppendRowsRequest)
				req.GetProtoRows().WriterSchema = nil
				req.TraceId = ""
				want[1] = req
				want[2] = req
				return want
			}(),
		},
		{
			description: "multiplex interleave",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				dvA := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("a")})
				dvB := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("b")})

				reqA := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				reqA.WriteStream = "alpha"

				reqB := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				reqB.WriteStream = "beta"
				reqB.GetProtoRows().GetWriterSchema().ProtoDescriptor = protodesc.ToDescriptorProto((&testdata.AllSupportedTypes{}).ProtoReflect().Descriptor())

				writes := make([]*pendingWrite, 10)
				writes[0] = newPendingWrite(ctx, nil, reqA, dvA)
				writes[1] = newPendingWrite(ctx, nil, reqA, dvA)
				writes[2] = newPendingWrite(ctx, nil, reqB, dvB)
				writes[3] = newPendingWrite(ctx, nil, reqA, dvA)
				writes[4] = newPendingWrite(ctx, nil, reqB, dvB)
				writes[5] = newPendingWrite(ctx, nil, reqB, dvB)
				writes[6] = newPendingWrite(ctx, nil, reqB, dvB)
				writes[7] = newPendingWrite(ctx, nil, reqB, dvB)
				writes[8] = newPendingWrite(ctx, nil, reqA, dvA)
				writes[9] = newPendingWrite(ctx, nil, reqA, dvA)

				return writes
			}(),
			sendResults: []error{
				nil,
				nil,
				nil,
				nil,
				nil,
				io.EOF,
				nil,
				nil,
				nil,
				io.EOF,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 10)

				wantReqAFull := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				wantReqAFull.WriteStream = "alpha"

				wantReqANoTrace := proto.Clone(wantReqAFull).(*storagepb.AppendRowsRequest)
				wantReqANoTrace.TraceId = ""

				wantReqAOpt := proto.Clone(wantReqAFull).(*storagepb.AppendRowsRequest)
				wantReqAOpt.GetProtoRows().WriterSchema = nil
				wantReqAOpt.TraceId = ""

				wantReqBFull := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				wantReqBFull.WriteStream = "beta"
				wantReqBFull.GetProtoRows().GetWriterSchema().ProtoDescriptor = protodesc.ToDescriptorProto((&testdata.AllSupportedTypes{}).ProtoReflect().Descriptor())

				wantReqBNoTrace := proto.Clone(wantReqBFull).(*storagepb.AppendRowsRequest)
				wantReqBNoTrace.TraceId = ""

				wantReqBOpt := proto.Clone(wantReqBFull).(*storagepb.AppendRowsRequest)
				wantReqBOpt.GetProtoRows().WriterSchema = nil
				wantReqBOpt.TraceId = ""

				want[0] = wantReqAFull
				want[1] = wantReqAOpt
				want[2] = wantReqBNoTrace
				want[3] = wantReqANoTrace
				want[4] = wantReqBNoTrace
				want[5] = wantReqBOpt
				want[6] = wantReqBFull
				want[7] = wantReqBOpt
				want[8] = wantReqANoTrace
				want[9] = wantReqAOpt

				return want
			}(),
		},
		{
			description: "multiplex w/evolution",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				dvOld := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("old")})
				dvNew := newDescriptorVersion(&descriptorpb.DescriptorProto{Name: proto.String("new")})

				example := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)

				writes := make([]*pendingWrite, 4)
				writes[0] = newPendingWrite(ctx, nil, example, dvOld)
				writes[1] = newPendingWrite(ctx, nil, example, dvOld)
				writes[2] = newPendingWrite(ctx, nil, example, dvNew)
				writes[3] = newPendingWrite(ctx, nil, example, dvNew)

				return writes
			}(),
			sendResults: []error{
				nil,
				nil,
				nil,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 4)

				wantReqFull := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)

				wantReqNoTrace := proto.Clone(wantReqFull).(*storagepb.AppendRowsRequest)
				wantReqNoTrace.TraceId = ""

				wantReqOpt := proto.Clone(wantReqFull).(*storagepb.AppendRowsRequest)
				wantReqOpt.GetProtoRows().WriterSchema = nil
				wantReqOpt.TraceId = ""

				want[0] = wantReqFull
				want[1] = wantReqOpt
				want[2] = wantReqNoTrace
				want[3] = wantReqOpt
				return want
			}(),
		},
	}

	for _, tc := range testCases {
		testARC := &testAppendRowsClient{}
		testARC.sendF = func(req *storagepb.AppendRowsRequest) error {
			testARC.requests = append(testARC.requests, proto.Clone(req).(*storagepb.AppendRowsRequest))
			respErr := tc.sendResults[0]
			tc.sendResults = tc.sendResults[1:]
			return respErr
		}

		for _, req := range tc.reqs {
			tc.optimizer.optimizeSend(testARC, req)
		}
		// now, compare.
		for k, wr := range tc.wantReqs {
			if diff := cmp.Diff(testARC.requests[k], wr, protocmp.Transform()); diff != "" {
				t.Errorf("%s (req %d) mismatch: -got, +want:\n%s", tc.description, k, diff)
			}
		}
	}
}
