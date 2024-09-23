// Copyright 2021 Google LLC
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
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPendingWrite(t *testing.T) {
	ctx := context.Background()
	wantReq := &storagepb.AppendRowsRequest{
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
				Rows: &storagepb.ProtoRows{
					SerializedRows: [][]byte{
						[]byte("row1"),
						[]byte("row2"),
						[]byte("row3"),
					},
				},
			},
		},
	}

	// verify no offset behavior
	pending := newPendingWrite(ctx, nil, wantReq, nil, "", "")
	if pending.req.GetOffset() != nil {
		t.Errorf("request should have no offset, but is present: %q", pending.req.GetOffset().GetValue())
	}

	if diff := cmp.Diff(pending.req, wantReq, protocmp.Transform()); diff != "" {
		t.Errorf("request mismatch: -got, +want:\n%s", diff)
	}

	// Verify request is not acknowledged.
	select {
	case <-pending.result.Ready():
		t.Errorf("got Ready() on incomplete AppendResult")
	case <-time.After(100 * time.Millisecond):

	}

	// Mark completed, verify result.
	pending.markDone(&storage.AppendRowsResponse{}, nil)
	if gotOff := pending.result.offset(ctx); gotOff != NoStreamOffset {
		t.Errorf("mismatch on completed AppendResult without offset: got %d want %d", gotOff, NoStreamOffset)
	}
	if pending.result.err != nil {
		t.Errorf("mismatch in error on AppendResult, got %v want nil", pending.result.err)
	}

	// Create new write to verify error result.
	pending = newPendingWrite(ctx, nil, wantReq, nil, "", "")

	// Manually invoke option to apply offset to request.
	// This would normally be appied as part of the AppendRows() method on the managed stream.
	wantOffset := int64(101)
	f := WithOffset(wantOffset)
	f(pending)

	if pending.req.GetOffset() == nil {
		t.Errorf("expected offset, got none")
	}
	if pending.req.GetOffset().GetValue() != wantOffset {
		t.Errorf("offset mismatch, got %d wanted %d", pending.req.GetOffset().GetValue(), wantOffset)
	}

	// Verify completion behavior with an error.
	wantErr := fmt.Errorf("foo")

	testResp := &storagepb.AppendRowsResponse{
		Response: &storagepb.AppendRowsResponse_AppendResult_{
			AppendResult: &storagepb.AppendRowsResponse_AppendResult{
				Offset: &wrapperspb.Int64Value{
					Value: wantOffset,
				},
			},
		},
	}
	pending.markDone(testResp, wantErr)

	if pending.req != nil {
		t.Errorf("expected request to be cleared, is present: %#v", pending.req)
	}

	select {

	case <-time.After(100 * time.Millisecond):
		t.Errorf("possible blocking on completed AppendResult")
	case <-pending.result.Ready():
		gotOffset, gotErr := pending.result.GetResult(ctx)
		if gotOffset != wantOffset {
			t.Errorf("GetResult: mismatch on completed AppendResult offset: got %d want %d", gotOffset, wantOffset)
		}
		if gotErr != wantErr {
			t.Errorf("GetResult: mismatch in errors, got %v want %v", gotErr, wantErr)
		}
		// Now, check FullResponse.
		gotResp, gotErr := pending.result.FullResponse(ctx)
		if gotErr != wantErr {
			t.Errorf("FullResponse: mismatch in errors, got %v want %v", gotErr, wantErr)
		}
		if diff := cmp.Diff(gotResp, testResp, protocmp.Transform()); diff != "" {
			t.Errorf("FullResponse diff: %s", diff)
		}
	}
}

func TestPendingWrite_ConstructFullRequest(t *testing.T) {

	testDP := &descriptorpb.DescriptorProto{Name: proto.String("foo")}
	testTmpl := newVersionedTemplate().revise(reviseProtoSchema(testDP))

	testEmptyTraceID := buildTraceID(&streamSettings{})

	for _, tc := range []struct {
		desc     string
		pw       *pendingWrite
		addTrace bool
		want     *storagepb.AppendRowsRequest
	}{
		{
			desc: "nil request",
			pw: &pendingWrite{
				reqTmpl: testTmpl,
			},
			want: &storagepb.AppendRowsRequest{
				Rows: &storagepb.AppendRowsRequest_ProtoRows{
					ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
						WriterSchema: &storagepb.ProtoSchema{
							ProtoDescriptor: testDP,
						},
					},
				},
			},
		},
		{
			desc: "empty req w/trace",
			pw: &pendingWrite{
				req:     &storagepb.AppendRowsRequest{},
				reqTmpl: testTmpl,
			},
			addTrace: true,
			want: &storagepb.AppendRowsRequest{
				Rows: &storagepb.AppendRowsRequest_ProtoRows{
					ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
						WriterSchema: &storagepb.ProtoSchema{
							ProtoDescriptor: testDP,
						},
					},
				},
				TraceId: testEmptyTraceID,
			},
		},
		{
			desc: "basic req",
			pw: &pendingWrite{
				req:     &storagepb.AppendRowsRequest{},
				reqTmpl: testTmpl,
			},
			want: &storagepb.AppendRowsRequest{
				Rows: &storagepb.AppendRowsRequest_ProtoRows{
					ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
						WriterSchema: &storagepb.ProtoSchema{
							ProtoDescriptor: testDP,
						},
					},
				},
			},
		},
		{
			desc: "everything w/trace",
			pw: &pendingWrite{
				req:           &storagepb.AppendRowsRequest{},
				reqTmpl:       testTmpl,
				traceID:       "foo",
				writeStreamID: "streamid",
			},
			addTrace: true,
			want: &storagepb.AppendRowsRequest{
				WriteStream: "streamid",
				Rows: &storagepb.AppendRowsRequest_ProtoRows{
					ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
						WriterSchema: &storagepb.ProtoSchema{
							ProtoDescriptor: testDP,
						},
					},
				},
				TraceId: buildTraceID(&streamSettings{TraceID: "foo"}),
			},
		},
	} {
		got := tc.pw.constructFullRequest(tc.addTrace)
		if diff := cmp.Diff(got, tc.want, protocmp.Transform()); diff != "" {
			t.Errorf("%s diff: %s", tc.desc, diff)
		}
	}
}
