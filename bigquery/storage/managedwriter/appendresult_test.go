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
	"fmt"
	"testing"
	"time"
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
	wantRowData := [][]byte{
		[]byte("row1"),
		[]byte("row2"),
		[]byte("row3"),
	}

	var wantOffset int64 = 99

	// first, verify no offset behavior
	pending := newPendingWrite(wantRowData, NoStreamOffset)
	if pending.request.GetOffset() != nil {
		t.Errorf("request should have no offset, but is present: %q", pending.request.GetOffset().GetValue())
	}
	pending.markDone(NoStreamOffset, nil, nil)
	if pending.result.offset != NoStreamOffset {
		t.Errorf("mismatch on completed AppendResult without offset: got %d want %d", pending.result.offset, NoStreamOffset)
	}
	if pending.result.err != nil {
		t.Errorf("mismatch in error on AppendResult, got %v want nil", pending.result.err)
	}

	// now, verify behavior with a valid offset
	pending = newPendingWrite(wantRowData, 99)
	if pending.request.GetOffset() == nil {
		t.Errorf("offset not set, should be %d", wantOffset)
	}
	if gotOffset := pending.request.GetOffset().GetValue(); gotOffset != wantOffset {
		t.Errorf("offset mismatch, got %d want %d", gotOffset, wantOffset)
	}

	// check request shape
	gotRowCount := len(pending.request.GetProtoRows().GetRows().GetSerializedRows())
	if gotRowCount != len(wantRowData) {
		t.Errorf("pendingWrite request mismatch, got %d rows, want %d rows", gotRowCount, len(wantRowData))
	}

	// verify AppendResult

	gotData := pending.result.rowData
	if len(gotData) != len(wantRowData) {
		t.Errorf("length mismatch on appendresult, got %d, want %d", len(gotData), len(wantRowData))
	}
	for i := 0; i < len(gotData); i++ {
		if !bytes.Equal(gotData[i], wantRowData[i]) {
			t.Errorf("row %d mismatch in data: got %q want %q", i, gotData[i], wantRowData[i])
		}
	}
	select {
	case <-pending.result.Ready():
		t.Errorf("got Ready() on incomplete AppendResult")
	case <-time.After(100 * time.Millisecond):

	}

	// verify completion behavior
	reportedOffset := int64(101)
	wantErr := fmt.Errorf("foo")
	pending.markDone(reportedOffset, wantErr, nil)

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
		if pending.result.offset != reportedOffset {
			t.Errorf("mismatch on completed AppendResult offset: got %d want %d", pending.result.offset, reportedOffset)
		}
		if pending.result.err != wantErr {
			t.Errorf("mismatch in errors, got %v want %v", pending.result.err, wantErr)
		}
	}

}
