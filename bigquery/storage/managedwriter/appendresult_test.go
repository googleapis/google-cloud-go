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

	wantRowBytes := []byte("rowdata")

	gotAR := newAppendResult(wantRowBytes)
	if !bytes.Equal(gotAR.rowData, wantRowBytes) {
		t.Errorf("mismatch in row data, got %q want %q", gotAR.rowData, wantRowBytes)
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
	pending.markDone(NoStreamOffset, nil)
	for k, ar := range pending.results {
		if ar.offset != NoStreamOffset {
			t.Errorf("mismatch on completed AppendResult(%d) without offset: got %d want %d", k, ar.offset, NoStreamOffset)
		}
		if ar.err != nil {
			t.Errorf("mismatch in error on AppendResult(%d), got %v want nil", k, ar.err)
		}
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

	// verify child AppendResults
	if len(pending.results) != len(wantRowData) {
		t.Errorf("mismatch in rows and append results.  %d rows, %d AppendResults", len(wantRowData), len(pending.results))
	}
	for k, ar := range pending.results {
		gotData := ar.rowData
		if !bytes.Equal(gotData, wantRowData[k]) {
			t.Errorf("row %d mismatch in data: got %q want %q", k, gotData, wantRowData[k])
		}
		select {
		case <-ar.Ready():
			t.Errorf("got Ready() on incomplete AppendResult %d", k)
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

	// verify completion behavior
	reportedOffset := int64(101)
	wantErr := fmt.Errorf("foo")
	pending.markDone(reportedOffset, wantErr)

	if pending.request != nil {
		t.Errorf("expected request to be cleared, is present: %#v", pending.request)
	}
	for k, ar := range pending.results {
		gotData := ar.rowData
		if !bytes.Equal(gotData, wantRowData[k]) {
			t.Errorf("row %d mismatch in data: got %q want %q", k, gotData, wantRowData[k])
		}
		select {
		case <-ar.Ready():
			continue
		case <-time.After(100 * time.Millisecond):
			t.Errorf("possible blocking on completed AppendResult %d", k)
		}
		if ar.offset != reportedOffset+int64(k) {
			t.Errorf("mismatch on completed AppendResult offset: got %d want %d", ar.offset, reportedOffset+int64(k))
		}
		if ar.err != wantErr {
			t.Errorf("mismatch in errors, got %v want %v", ar.err, wantErr)
		}
	}

}
