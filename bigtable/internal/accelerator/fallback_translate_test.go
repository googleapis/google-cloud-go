// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package accelerator

import (
	"testing"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRowToReadRowsResponse_NilRow(t *testing.T) {
	resp := rowToReadRowsResponse(nil, []byte("k"))
	if len(resp.Chunks) != 0 {
		t.Errorf("nil row: got %d chunks, want 0", len(resp.Chunks))
	}
}

func TestRowToReadRowsResponse_EmptyRow(t *testing.T) {
	resp := rowToReadRowsResponse(bigtable.Row{}, []byte("k"))
	if len(resp.Chunks) != 0 {
		t.Errorf("empty row: got %d chunks, want 0", len(resp.Chunks))
	}
}

func TestRowToReadRowsResponse_SingleCell(t *testing.T) {
	row := bigtable.Row{
		"cf": {
			{Column: "cf:col", Timestamp: 1000, Value: []byte("v")},
		},
	}
	resp := rowToReadRowsResponse(row, []byte("rk"))
	if len(resp.Chunks) != 1 {
		t.Fatalf("single cell: got %d chunks, want 1", len(resp.Chunks))
	}
	c := resp.Chunks[0]
	if string(c.RowKey) != "rk" {
		t.Errorf("RowKey = %q, want %q", c.RowKey, "rk")
	}
	if c.FamilyName == nil || c.FamilyName.Value != "cf" {
		t.Errorf("FamilyName = %v, want cf", c.FamilyName)
	}
	if c.Qualifier == nil || string(c.Qualifier.Value) != "col" {
		t.Errorf("Qualifier = %v, want col", c.Qualifier)
	}
	if c.TimestampMicros != 1000 {
		t.Errorf("TimestampMicros = %d, want 1000", c.TimestampMicros)
	}
	if string(c.Value) != "v" {
		t.Errorf("Value = %q, want %q", c.Value, "v")
	}
	if _, ok := c.RowStatus.(*v2pb.ReadRowsResponse_CellChunk_CommitRow); !ok {
		t.Errorf("last chunk missing CommitRow; RowStatus = %T", c.RowStatus)
	}
}

func TestRowToReadRowsResponse_MultiFamilySorted(t *testing.T) {
	// Three families: "z", "a", "m" — output must be alphabetical.
	row := bigtable.Row{
		"z": {{Column: "z:q", Timestamp: 1, Value: []byte("vz")}},
		"a": {{Column: "a:q", Timestamp: 2, Value: []byte("va")}},
		"m": {{Column: "m:q", Timestamp: 3, Value: []byte("vm")}},
	}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(resp.Chunks))
	}
	wantFamilies := []string{"a", "m", "z"}
	for i, want := range wantFamilies {
		c := resp.Chunks[i]
		if c.FamilyName == nil || c.FamilyName.Value != want {
			t.Errorf("chunk[%d].FamilyName = %v, want %q", i, c.FamilyName, want)
		}
	}
	// Only the first chunk carries RowKey.
	if string(resp.Chunks[0].RowKey) != "k" {
		t.Errorf("chunk[0].RowKey = %q, want %q", resp.Chunks[0].RowKey, "k")
	}
	for i := 1; i < len(resp.Chunks); i++ {
		if len(resp.Chunks[i].RowKey) != 0 {
			t.Errorf("chunk[%d].RowKey = %q, want empty", i, resp.Chunks[i].RowKey)
		}
	}
	// CommitRow only on last chunk.
	if _, ok := resp.Chunks[2].RowStatus.(*v2pb.ReadRowsResponse_CellChunk_CommitRow); !ok {
		t.Errorf("last chunk missing CommitRow")
	}
	for i := 0; i < 2; i++ {
		if resp.Chunks[i].RowStatus != nil {
			t.Errorf("chunk[%d] has unexpected RowStatus %T", i, resp.Chunks[i].RowStatus)
		}
	}
}

func TestRowToReadRowsResponse_QualifierTransitions(t *testing.T) {
	// Two cells in the same family, different qualifiers.
	row := bigtable.Row{
		"cf": {
			{Column: "cf:col1", Timestamp: 1, Value: []byte("v1")},
			{Column: "cf:col2", Timestamp: 2, Value: []byte("v2")},
		},
	}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(resp.Chunks))
	}
	// First chunk: FamilyName + Qualifier set.
	c0 := resp.Chunks[0]
	if c0.FamilyName == nil {
		t.Error("chunk[0].FamilyName is nil")
	}
	if c0.Qualifier == nil || string(c0.Qualifier.Value) != "col1" {
		t.Errorf("chunk[0].Qualifier = %v, want col1", c0.Qualifier)
	}
	// Second chunk: FamilyName nil (no transition), Qualifier set.
	c1 := resp.Chunks[1]
	if c1.FamilyName != nil {
		t.Errorf("chunk[1].FamilyName = %v, want nil (no family transition)", c1.FamilyName)
	}
	if c1.Qualifier == nil || string(c1.Qualifier.Value) != "col2" {
		t.Errorf("chunk[1].Qualifier = %v, want col2", c1.Qualifier)
	}
}

func TestRowToReadRowsResponse_SameQualifier_NoRedundantField(t *testing.T) {
	// Two cells with the same qualifier (different timestamps) — Qualifier
	// field should only be set on the first cell.
	row := bigtable.Row{
		"cf": {
			{Column: "cf:q", Timestamp: 2000, Value: []byte("v2")},
			{Column: "cf:q", Timestamp: 1000, Value: []byte("v1")},
		},
	}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(resp.Chunks))
	}
	if resp.Chunks[0].Qualifier == nil {
		t.Error("chunk[0].Qualifier is nil; expected set on first cell")
	}
	if resp.Chunks[1].Qualifier != nil {
		t.Errorf("chunk[1].Qualifier = %v; expected nil (same qualifier, no transition)", resp.Chunks[1].Qualifier)
	}
}

func TestRowToReadRowsResponse_Labels(t *testing.T) {
	row := bigtable.Row{
		"cf": {
			{Column: "cf:q", Timestamp: 1, Value: []byte("v"), Labels: []string{"l1", "l2"}},
		},
	}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(resp.Chunks))
	}
	got := resp.Chunks[0].Labels
	if len(got) != 2 || got[0] != "l1" || got[1] != "l2" {
		t.Errorf("Labels = %v, want [l1 l2]", got)
	}
}

// Verify the CommitRow oneof encoding — callers depend on the concrete type.
func TestRowToReadRowsResponse_CommitRowType(t *testing.T) {
	row := bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("v")}}}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) == 0 {
		t.Fatal("no chunks")
	}
	cr, ok := resp.Chunks[len(resp.Chunks)-1].RowStatus.(*v2pb.ReadRowsResponse_CellChunk_CommitRow)
	if !ok || !cr.CommitRow {
		t.Errorf("last chunk RowStatus = %v, want CommitRow=true", resp.Chunks[len(resp.Chunks)-1].RowStatus)
	}
}

// Verify FamilyName uses wrapperspb.StringValue (not just a raw string).
func TestRowToReadRowsResponse_FamilyNameType(t *testing.T) {
	row := bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("v")}}}
	resp := rowToReadRowsResponse(row, []byte("k"))
	if len(resp.Chunks) == 0 {
		t.Fatal("no chunks")
	}
	fn := resp.Chunks[0].FamilyName
	if fn == nil {
		t.Fatal("FamilyName is nil")
	}
	if _, ok := any(fn).(*wrapperspb.StringValue); !ok {
		t.Errorf("FamilyName type = %T, want *wrapperspb.StringValue", fn)
	}
}
