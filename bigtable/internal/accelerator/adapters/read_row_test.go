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

package adapters

import (
	"bytes"
	"testing"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestReadRowRequestAdapter(t *testing.T) {
	reqAdapter := &ReadRowRequestAdapter{}
	v2Req := &v2pb.ReadRowsRequest{
		TableName: "projects/p1/instances/i1/tables/t1",
		Rows: &v2pb.RowSet{
			RowKeys: [][]byte{[]byte("test-key")},
		},
		Filter: &v2pb.RowFilter{
			Filter: &v2pb.RowFilter_FamilyNameRegexFilter{FamilyNameRegexFilter: "family-regex"},
		},
	}
	jsReq, err := reqAdapter.Adapt(v2Req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if jsReq == nil {
		t.Fatal("expected non-nil jsReq")
	}

	if string(jsReq.Key) != "test-key" {
		t.Errorf("expected Key test-key, got %s", string(jsReq.Key))
	}

	if jsReq.Filter.GetFamilyNameRegexFilter() != "family-regex" {
		t.Errorf("expected Filter family-regex, got %s", jsReq.Filter.GetFamilyNameRegexFilter())
	}

	res, err := reqAdapter.ExtractResource(v2Req)
	if err != nil {
		t.Fatalf("ExtractResource failed: %v", err)
	}
	if res.Kind != ResourceTable || res.Name != "projects/p1/instances/i1/tables/t1" {
		t.Errorf("ExtractResource = %+v; want {ResourceTable, projects/p1/instances/i1/tables/t1}", res)
	}
}

func TestReadRowRequestAdapter_ExtractResource(t *testing.T) {
	reqAdapter := &ReadRowRequestAdapter{}
	cases := []struct {
		name string
		req  *v2pb.ReadRowsRequest
		want Resource
	}{
		{
			"table",
			&v2pb.ReadRowsRequest{TableName: "projects/p/instances/i/tables/t"},
			Resource{Kind: ResourceTable, Name: "projects/p/instances/i/tables/t"},
		},
		{
			"authorized-view",
			&v2pb.ReadRowsRequest{AuthorizedViewName: "projects/p/instances/i/tables/t/authorizedViews/v"},
			Resource{Kind: ResourceAuthorizedView, Name: "projects/p/instances/i/tables/t/authorizedViews/v"},
		},
		{
			"materialized-view",
			&v2pb.ReadRowsRequest{MaterializedViewName: "projects/p/instances/i/materializedViews/mv"},
			Resource{Kind: ResourceMaterializedView, Name: "projects/p/instances/i/materializedViews/mv"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reqAdapter.ExtractResource(tc.req)
			if err != nil {
				t.Fatalf("ExtractResource: %v", err)
			}
			if got != tc.want {
				t.Errorf("ExtractResource = %+v; want %+v", got, tc.want)
			}
		})
	}
}

func TestReadRowRequestAdapter_ExtractResource_Empty(t *testing.T) {
	reqAdapter := &ReadRowRequestAdapter{}
	_, err := reqAdapter.ExtractResource(&v2pb.ReadRowsRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("ExtractResource(empty) code = %v; want InvalidArgument", status.Code(err))
	}
}

func TestReadRowRequestAdapter_ClosedClosedRange(t *testing.T) {
	reqAdapter := &ReadRowRequestAdapter{}
	v2Req := &v2pb.ReadRowsRequest{
		TableName: "projects/p1/instances/i1/tables/t1",
		Rows: &v2pb.RowSet{
			RowRanges: []*v2pb.RowRange{{
				StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("range-key")},
				EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("range-key")},
			}},
		},
	}
	jsReq, err := reqAdapter.Adapt(v2Req)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if jsReq == nil {
		t.Fatal("Adapt returned nil")
	}
	if string(jsReq.Key) != "range-key" {
		t.Errorf("Key = %q; want range-key", jsReq.Key)
	}
}

func TestReadRowResponseAdapter_NilAndEmpty(t *testing.T) {
	a := &ReadRowResponseAdapter{}

	got, err := a.Adapt(nil)
	if err != nil {
		t.Fatalf("Adapt(nil) error: %v", err)
	}
	if got != nil {
		t.Errorf("Adapt(nil) = %v; want nil", got)
	}

	got, err = a.Adapt(&v2pb.SessionReadRowResponse{}) // Row == nil → missing row
	if err != nil {
		t.Fatalf("Adapt(missing row) error: %v", err)
	}
	if got != nil {
		t.Errorf("Adapt(missing row) = %v; want nil", got)
	}

	got, err = a.Adapt(&v2pb.SessionReadRowResponse{Row: &v2pb.Row{Key: []byte("k")}}) // no families
	if err != nil {
		t.Fatalf("Adapt(empty row) error: %v", err)
	}
	if got != nil {
		t.Errorf("Adapt(empty row) = %v; want nil", got)
	}
}

func TestReadRowResponseAdapter_SingleCell(t *testing.T) {
	a := &ReadRowResponseAdapter{}
	resp := &v2pb.SessionReadRowResponse{
		Row: &v2pb.Row{
			Key: []byte("rk"),
			Families: []*v2pb.Family{{
				Name: "fam",
				Columns: []*v2pb.Column{{
					Qualifier: []byte("q"),
					Cells: []*v2pb.Cell{{
						TimestampMicros: 1234,
						Value:           []byte("v"),
						Labels:          []string{"L"},
					}},
				}},
			}},
		},
	}
	got, err := a.Adapt(resp)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if got == nil || len(got.Chunks) != 1 {
		t.Fatalf("Chunks len = %d; want 1 (got=%+v)", len(got.GetChunks()), got)
	}
	cc := got.Chunks[0]
	if !bytes.Equal(cc.RowKey, []byte("rk")) {
		t.Errorf("RowKey = %q; want rk", cc.RowKey)
	}
	if cc.FamilyName == nil || cc.FamilyName.Value != "fam" {
		t.Errorf("FamilyName = %v; want fam", cc.FamilyName)
	}
	if cc.Qualifier == nil || !bytes.Equal(cc.Qualifier.Value, []byte("q")) {
		t.Errorf("Qualifier = %v; want q", cc.Qualifier)
	}
	if cc.TimestampMicros != 1234 {
		t.Errorf("TimestampMicros = %d; want 1234", cc.TimestampMicros)
	}
	if !bytes.Equal(cc.Value, []byte("v")) {
		t.Errorf("Value = %q; want v", cc.Value)
	}
	if len(cc.Labels) != 1 || cc.Labels[0] != "L" {
		t.Errorf("Labels = %v; want [L]", cc.Labels)
	}
	commit, ok := cc.RowStatus.(*v2pb.ReadRowsResponse_CellChunk_CommitRow)
	if !ok || !commit.CommitRow {
		t.Errorf("RowStatus = %v; want CommitRow=true", cc.RowStatus)
	}
}

func TestReadRowResponseAdapter_MultiFamilyMultiCell_ChunkBoundaries(t *testing.T) {
	a := &ReadRowResponseAdapter{}
	resp := &v2pb.SessionReadRowResponse{
		Row: &v2pb.Row{
			Key: []byte("rk"),
			Families: []*v2pb.Family{
				{
					Name: "fam1",
					Columns: []*v2pb.Column{
						{
							Qualifier: []byte("c1"),
							Cells: []*v2pb.Cell{
								{TimestampMicros: 30, Value: []byte("v1")},
								{TimestampMicros: 20, Value: []byte("v2")},
							},
						},
						{
							Qualifier: []byte("c2"),
							Cells: []*v2pb.Cell{
								{TimestampMicros: 10, Value: []byte("v3")},
							},
						},
					},
				},
				{
					Name: "fam2",
					Columns: []*v2pb.Column{{
						Qualifier: []byte("c3"),
						Cells: []*v2pb.Cell{
							{TimestampMicros: 5, Value: []byte("v4")},
						},
					}},
				},
			},
		},
	}

	got, err := a.Adapt(resp)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if got == nil || len(got.Chunks) != 4 {
		t.Fatalf("Chunks len = %d; want 4", len(got.GetChunks()))
	}

	want := []chunkExpect{
		{[]byte("rk"), "fam1", []byte("c1"), 30, "v1", false},
		{nil, "", nil, 20, "v2", false},
		{nil, "", []byte("c2"), 10, "v3", false},
		{nil, "fam2", []byte("c3"), 5, "v4", true},
	}
	assertChunks(t, got.Chunks, want)
}

type chunkExpect struct {
	rowKey     []byte
	familyName string // "" → expect nil FamilyName
	qualifier  []byte // nil → expect nil Qualifier
	ts         int64
	value      string
	commit     bool
}

func assertChunks(t *testing.T, got []*v2pb.ReadRowsResponse_CellChunk, want []chunkExpect) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Chunks len = %d; want %d", len(got), len(want))
	}
	for i, cc := range got {
		w := want[i]
		if !bytes.Equal(cc.RowKey, w.rowKey) {
			t.Errorf("chunk[%d].RowKey = %q; want %q", i, cc.RowKey, w.rowKey)
		}
		if w.familyName == "" {
			if cc.FamilyName != nil {
				t.Errorf("chunk[%d].FamilyName = %v; want nil", i, cc.FamilyName)
			}
		} else if cc.FamilyName == nil || cc.FamilyName.Value != w.familyName {
			t.Errorf("chunk[%d].FamilyName = %v; want %q", i, cc.FamilyName, w.familyName)
		}
		if w.qualifier == nil {
			if cc.Qualifier != nil {
				t.Errorf("chunk[%d].Qualifier = %v; want nil", i, cc.Qualifier)
			}
		} else if cc.Qualifier == nil || !bytes.Equal(cc.Qualifier.Value, w.qualifier) {
			t.Errorf("chunk[%d].Qualifier = %v; want %q", i, cc.Qualifier, w.qualifier)
		}
		if cc.TimestampMicros != w.ts {
			t.Errorf("chunk[%d].TimestampMicros = %d; want %d", i, cc.TimestampMicros, w.ts)
		}
		if string(cc.Value) != w.value {
			t.Errorf("chunk[%d].Value = %q; want %q", i, cc.Value, w.value)
		}
		commit, isCommit := cc.RowStatus.(*v2pb.ReadRowsResponse_CellChunk_CommitRow)
		gotCommit := isCommit && commit.CommitRow
		if gotCommit != w.commit {
			t.Errorf("chunk[%d].CommitRow = %v; want %v", i, gotCommit, w.commit)
		}
	}
}

// oneCell is a helper for concise fixture construction.
func oneCell(ts int64, val string) *v2pb.Cell {
	return &v2pb.Cell{TimestampMicros: ts, Value: []byte(val)}
}

// TestReadRowResponseAdapter_TwoFamiliesTwoColumns_QualifierRepeats covers
// cf1:{a,b}, cf2:{b,c}. Verifies that (a) qualifier "b" is emitted at both
// family boundaries even though the byte string is the same, (b) FamilyName
// is set on the first chunk of each family, (c) RowKey only appears on the
// first chunk, and (d) CommitRow only on the last.
func TestReadRowResponseAdapter_TwoFamiliesTwoColumns_QualifierRepeats(t *testing.T) {
	a := &ReadRowResponseAdapter{}
	resp := &v2pb.SessionReadRowResponse{
		Row: &v2pb.Row{
			Key: []byte("rk"),
			Families: []*v2pb.Family{
				{
					Name: "cf1",
					Columns: []*v2pb.Column{
						{Qualifier: []byte("a"), Cells: []*v2pb.Cell{oneCell(40, "va")}},
						{Qualifier: []byte("b"), Cells: []*v2pb.Cell{oneCell(30, "v1b")}},
					},
				},
				{
					Name: "cf2",
					Columns: []*v2pb.Column{
						{Qualifier: []byte("b"), Cells: []*v2pb.Cell{oneCell(20, "v2b")}},
						{Qualifier: []byte("c"), Cells: []*v2pb.Cell{oneCell(10, "vc")}},
					},
				},
			},
		},
	}
	got, err := a.Adapt(resp)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	assertChunks(t, got.GetChunks(), []chunkExpect{
		{[]byte("rk"), "cf1", []byte("a"), 40, "va", false},
		{nil, "", []byte("b"), 30, "v1b", false},
		{nil, "cf2", []byte("b"), 20, "v2b", false},
		{nil, "", []byte("c"), 10, "vc", true},
	})
}

// TestReadRowResponseAdapter_TwoFamiliesMultiCell covers the same shape as
// above but with multiple cells per column, exercising the inner "cell after
// the first in a column has no boundary markers" rule.
func TestReadRowResponseAdapter_TwoFamiliesMultiCell(t *testing.T) {
	a := &ReadRowResponseAdapter{}
	resp := &v2pb.SessionReadRowResponse{
		Row: &v2pb.Row{
			Key: []byte("rk"),
			Families: []*v2pb.Family{
				{
					Name: "cf1",
					Columns: []*v2pb.Column{
						{Qualifier: []byte("a"), Cells: []*v2pb.Cell{oneCell(40, "va")}},
						{Qualifier: []byte("b"), Cells: []*v2pb.Cell{oneCell(35, "v1b_new"), oneCell(30, "v1b_old")}},
					},
				},
				{
					Name: "cf2",
					Columns: []*v2pb.Column{
						{Qualifier: []byte("b"), Cells: []*v2pb.Cell{oneCell(20, "v2b")}},
						{Qualifier: []byte("c"), Cells: []*v2pb.Cell{oneCell(15, "vc_new"), oneCell(10, "vc_old")}},
					},
				},
			},
		},
	}
	got, err := a.Adapt(resp)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	assertChunks(t, got.GetChunks(), []chunkExpect{
		{[]byte("rk"), "cf1", []byte("a"), 40, "va", false},
		{nil, "", []byte("b"), 35, "v1b_new", false},
		{nil, "", nil, 30, "v1b_old", false}, // second cell in cf1:b — no boundary markers
		{nil, "cf2", []byte("b"), 20, "v2b", false},
		{nil, "", []byte("c"), 15, "vc_new", false},
		{nil, "", nil, 10, "vc_old", true}, // last chunk — CommitRow
	})
}

// TestReadRowResponseAdapter_FamilyWithNoColumns treats a family whose
// Columns slice is empty as contributing zero chunks — it must not emit a
// bare FamilyName chunk with no cell payload, and it must not shift where
// CommitRow lands.
func TestReadRowResponseAdapter_FamilyWithNoColumns(t *testing.T) {
	a := &ReadRowResponseAdapter{}
	resp := &v2pb.SessionReadRowResponse{
		Row: &v2pb.Row{
			Key: []byte("rk"),
			Families: []*v2pb.Family{
				{Name: "empty"}, // no columns
				{
					Name: "cf1",
					Columns: []*v2pb.Column{
						{Qualifier: []byte("q"), Cells: []*v2pb.Cell{oneCell(1, "v")}},
					},
				},
			},
		},
	}
	got, err := a.Adapt(resp)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	assertChunks(t, got.GetChunks(), []chunkExpect{
		{[]byte("rk"), "cf1", []byte("q"), 1, "v", true},
	})
}
