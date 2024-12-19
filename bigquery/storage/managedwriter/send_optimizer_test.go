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
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: [][]byte{[]byte("row_data")},
				},
			},
		},
	}
	exampleStreamID := "foo"
	exampleTraceID := "trace_id"
	exampleReqFull := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
	exampleReqFull.WriteStream = exampleStreamID
	exampleReqFull.TraceId = buildTraceID(&streamSettings{TraceID: exampleTraceID})
	exampleDP := &descriptorpb.DescriptorProto{Name: proto.String("schema")}
	exampleReqFull.GetProtoRows().WriterSchema = &storagepb.ProtoSchema{
		ProtoDescriptor: proto.Clone(exampleDP).(*descriptorpb.DescriptorProto),
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
			description: "verbose-optimizer",
			optimizer:   &verboseOptimizer{},
			reqs: func() []*pendingWrite {
				tmpl := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
				}
			}(),
			sendResults: []error{
				nil,
				io.EOF,
				io.EOF,
			},
			wantReqs: []*storagepb.AppendRowsRequest{
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
			},
		},
		{
			description: "simplex no errors",
			optimizer:   &simplexOptimizer{},
			reqs: func() []*pendingWrite {
				tmpl := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
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
				want[0] = proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)
				req := proto.Clone(want[0]).(*storagepb.AppendRowsRequest)
				req.GetProtoRows().WriterSchema = nil
				req.TraceId = ""
				req.WriteStream = ""
				// second and third are optimized.
				want[1] = req
				want[2] = req
				return want
			}(),
		},
		{
			description: "simplex w/partial errors",
			optimizer:   &simplexOptimizer{},
			reqs: func() []*pendingWrite {
				tmpl := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
				}
			}(),
			sendResults: []error{
				nil,
				io.EOF,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 3)
				want[0] = proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)
				req := proto.Clone(want[0]).(*storagepb.AppendRowsRequest)
				req.GetProtoRows().WriterSchema = nil
				req.TraceId = ""
				req.WriteStream = ""
				// second request is optimized
				want[1] = req
				// error causes third request to be full again.
				want[2] = want[0]
				return want
			}(),
		},
		{
			description: "multiplex single all errors",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				tmpl := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
				}
			}(),
			sendResults: []error{
				io.EOF,
				io.EOF,
				io.EOF,
			},
			wantReqs: []*storagepb.AppendRowsRequest{
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
				proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest),
			},
		},
		{
			description: "multiplex single no errors",
			optimizer:   &multiplexOptimizer{},
			reqs: func() []*pendingWrite {
				tmpl := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				return []*pendingWrite{
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
					newPendingWrite(ctx, nil, proto.Clone(exampleReq).(*storagepb.AppendRowsRequest), tmpl, exampleStreamID, exampleTraceID),
				}
			}(),
			sendResults: []error{
				nil,
				nil,
				nil,
			},
			wantReqs: func() []*storagepb.AppendRowsRequest {
				want := make([]*storagepb.AppendRowsRequest, 3)
				want[0] = proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)
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
				tmplA := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				tmplB := newVersionedTemplate().revise(reviseProtoSchema(protodesc.ToDescriptorProto((&testdata.AllSupportedTypes{}).ProtoReflect().Descriptor())))

				reqA := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				reqA.WriteStream = "alpha"

				reqB := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)
				reqB.WriteStream = "beta"

				writes := make([]*pendingWrite, 10)
				writes[0] = newPendingWrite(ctx, nil, reqA, tmplA, reqA.GetWriteStream(), exampleTraceID)
				writes[1] = newPendingWrite(ctx, nil, reqA, tmplA, reqA.GetWriteStream(), exampleTraceID)
				writes[2] = newPendingWrite(ctx, nil, reqB, tmplB, reqB.GetWriteStream(), exampleTraceID)
				writes[3] = newPendingWrite(ctx, nil, reqA, tmplA, reqA.GetWriteStream(), exampleTraceID)
				writes[4] = newPendingWrite(ctx, nil, reqB, tmplB, reqB.GetWriteStream(), exampleTraceID)
				writes[5] = newPendingWrite(ctx, nil, reqB, tmplB, reqB.GetWriteStream(), exampleTraceID)
				writes[6] = newPendingWrite(ctx, nil, reqB, tmplB, reqB.GetWriteStream(), exampleTraceID)
				writes[7] = newPendingWrite(ctx, nil, reqB, tmplB, reqB.GetWriteStream(), exampleTraceID)
				writes[8] = newPendingWrite(ctx, nil, reqA, tmplA, reqA.GetWriteStream(), exampleTraceID)
				writes[9] = newPendingWrite(ctx, nil, reqA, tmplA, reqA.GetWriteStream(), exampleTraceID)

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

				wantReqAFull := proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)
				wantReqAFull.WriteStream = "alpha"

				wantReqANoTrace := proto.Clone(wantReqAFull).(*storagepb.AppendRowsRequest)
				wantReqANoTrace.TraceId = ""

				wantReqAOpt := proto.Clone(wantReqAFull).(*storagepb.AppendRowsRequest)
				wantReqAOpt.GetProtoRows().WriterSchema = nil
				wantReqAOpt.TraceId = ""

				wantReqBFull := proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)
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
				tmplOld := newVersionedTemplate().revise(reviseProtoSchema(exampleDP))
				tmplNew := tmplOld.revise(reviseProtoSchema(&descriptorpb.DescriptorProto{Name: proto.String("new")}))

				example := proto.Clone(exampleReq).(*storagepb.AppendRowsRequest)

				writes := make([]*pendingWrite, 4)
				writes[0] = newPendingWrite(ctx, nil, example, tmplOld, exampleStreamID, exampleTraceID)
				writes[1] = newPendingWrite(ctx, nil, example, tmplOld, exampleStreamID, exampleTraceID)
				writes[2] = newPendingWrite(ctx, nil, example, tmplNew, exampleStreamID, exampleTraceID)
				writes[3] = newPendingWrite(ctx, nil, example, tmplNew, exampleStreamID, exampleTraceID)

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

				wantBaseReqFull := proto.Clone(exampleReqFull).(*storagepb.AppendRowsRequest)

				wantBaseReqOpt := proto.Clone(wantBaseReqFull).(*storagepb.AppendRowsRequest)
				wantBaseReqOpt.TraceId = ""
				wantBaseReqOpt.GetProtoRows().WriterSchema = nil

				wantEvolved := proto.Clone(wantBaseReqOpt).(*storagepb.AppendRowsRequest)
				wantEvolved.GetProtoRows().WriterSchema = &storagepb.ProtoSchema{
					ProtoDescriptor: &descriptorpb.DescriptorProto{Name: proto.String("new")},
				}

				want[0] = wantBaseReqFull
				want[1] = wantBaseReqOpt
				want[2] = wantEvolved
				want[3] = wantBaseReqOpt
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
			err := tc.optimizer.optimizeSend(testARC, req)
			if err != nil {
				tc.optimizer.signalReset()
			}
		}
		// now, compare.
		for k, wr := range tc.wantReqs {
			if diff := cmp.Diff(testARC.requests[k], wr, protocmp.Transform()); diff != "" {
				t.Errorf("%s (req %d) mismatch: -got, +want:\n%s", tc.description, k, diff)
			}
		}
	}
}

func TestVersionedTemplate(t *testing.T) {
	testCases := []struct {
		desc           string
		inputTmpl      *storagepb.AppendRowsRequest
		changes        []templateRevisionF
		wantCompatible bool
	}{
		{
			desc:           "nil template",
			wantCompatible: true,
		},
		{
			desc:           "no changes",
			inputTmpl:      &storagepb.AppendRowsRequest{},
			wantCompatible: true,
		},
		{
			desc:      "empty schema",
			inputTmpl: &storagepb.AppendRowsRequest{},
			changes: []templateRevisionF{
				reviseProtoSchema(nil),
			},
			wantCompatible: false,
		},
		{
			desc: "same default mvi",
			inputTmpl: &storagepb.AppendRowsRequest{
				DefaultMissingValueInterpretation: storagepb.AppendRowsRequest_NULL_VALUE,
			},
			changes: []templateRevisionF{
				reviseDefaultMissingValueInterpretation(storagepb.AppendRowsRequest_NULL_VALUE),
			},
			wantCompatible: true,
		},
		{
			desc: "differing default mvi",
			inputTmpl: &storagepb.AppendRowsRequest{
				DefaultMissingValueInterpretation: storagepb.AppendRowsRequest_NULL_VALUE,
			},
			changes: []templateRevisionF{
				reviseDefaultMissingValueInterpretation(storagepb.AppendRowsRequest_DEFAULT_VALUE),
			},
			wantCompatible: false,
		},
	}

	for _, tc := range testCases {
		orig := newVersionedTemplate()
		orig.tmpl = tc.inputTmpl
		orig.computeHash()

		rev := orig.revise(tc.changes...)
		if orig.Compatible(rev) != rev.Compatible(orig) {
			t.Errorf("case %q: inconsistent compatibility, orig %t rev %t", tc.desc, orig.Compatible(rev), rev.Compatible(orig))
		}
		if got := orig.Compatible(rev); tc.wantCompatible != got {
			t.Errorf("case %q: Compatible mismatch, got %t want %t", tc.desc, got, tc.wantCompatible)
		}
	}
}
