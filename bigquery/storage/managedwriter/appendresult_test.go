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
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAppendResult(t *testing.T) {

	wantRowBytes := [][]byte{[]byte("rowdata")}

	gotAR := newAppendResult(wantRowBytes)
	if len(gotAR.rowData) != len(wantRowBytes) {
		t.Fatalf("length mismatch, got %d want %d elements", len(gotAR.rowData), len(wantRowBytes))
	}
	for i := 0; i < len(gotAR.rowData); i++ {
		if !bytes.Equal(gotAR.rowData[i], wantRowBytes[i]) {
			t.Errorf("mismatch in row data %d, got %q want %q", i, gotAR.rowData, wantRowBytes)
		}
	}
}

func TestPendingWrite(t *testing.T) {
	ctx := context.Background()
	wantRowData := [][]byte{
		[]byte("row1"),
		[]byte("row2"),
		[]byte("row3"),
	}

	// verify no offset behavior
	pending := newPendingWrite(ctx, wantRowData)
	if pending.request.GetOffset() != nil {
		t.Errorf("request should have no offset, but is present: %q", pending.request.GetOffset().GetValue())
	}

	gotRowCount := len(pending.request.GetProtoRows().GetRows().GetSerializedRows())
	if gotRowCount != len(wantRowData) {
		t.Errorf("pendingWrite request mismatch, got %d rows, want %d rows", gotRowCount, len(wantRowData))
	}

	// Verify request is not acknowledged.
	select {
	case <-pending.result.Ready():
		t.Errorf("got Ready() on incomplete AppendResult")
	case <-time.After(100 * time.Millisecond):

	}

	// Mark completed, verify result.
	pending.markDone(&storage.AppendRowsResponse{}, nil, nil)
	if gotOff := pending.result.offset(ctx); gotOff != NoStreamOffset {
		t.Errorf("mismatch on completed AppendResult without offset: got %d want %d", gotOff, NoStreamOffset)
	}
	if pending.result.err != nil {
		t.Errorf("mismatch in error on AppendResult, got %v want nil", pending.result.err)
	}
	gotData := pending.result.rowData
	if len(gotData) != len(wantRowData) {
		t.Errorf("length mismatch on appendresult, got %d, want %d", len(gotData), len(wantRowData))
	}
	for i := 0; i < len(gotData); i++ {
		if !bytes.Equal(gotData[i], wantRowData[i]) {
			t.Errorf("row %d mismatch in data: got %q want %q", i, gotData[i], wantRowData[i])
		}
	}

	// Create new write to verify error result.
	pending = newPendingWrite(ctx, wantRowData)

	// Manually invoke option to apply offset to request.
	// This would normally be appied as part of the AppendRows() method on the managed stream.
	wantOffset := int64(101)
	f := WithOffset(wantOffset)
	f(pending)

	if pending.request.GetOffset() == nil {
		t.Errorf("expected offset, got none")
	}
	if pending.request.GetOffset().GetValue() != wantOffset {
		t.Errorf("offset mismatch, got %d wanted %d", pending.request.GetOffset().GetValue(), wantOffset)
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
	pending.markDone(testResp, wantErr, nil)

	if pending.request != nil {
		t.Errorf("expected request to be cleared, is present: %#v", pending.request)
	}
	gotData = pending.result.rowData
	if len(gotData) != len(wantRowData) {
		t.Errorf("length mismatch in data: got %d, want %d", len(gotData), len(wantRowData))
	}
	for i := 0; i < len(gotData); i++ {
		if !bytes.Equal(gotData[i], wantRowData[i]) {
			t.Errorf("row %d mismatch in data: got %q want %q", i, gotData[i], wantRowData[i])
		}
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
