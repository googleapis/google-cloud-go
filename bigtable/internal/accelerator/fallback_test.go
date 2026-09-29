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
	"context"
	"sync"
	"testing"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mockTableAPI ------------------------------------------------------------

// mockTableAPI implements bigtable.TableAPI for use in tests. Only ReadRow and
// Apply are wired; the rest panic to catch unexpected calls.
type mockTableAPI struct {
	readRowFn func(ctx context.Context, row string, opts ...bigtable.ReadOption) (bigtable.Row, error)
	applyFn   func(ctx context.Context, row string, m *bigtable.Mutation, opts ...bigtable.ApplyOption) error
}

func (m *mockTableAPI) ReadRow(ctx context.Context, row string, opts ...bigtable.ReadOption) (bigtable.Row, error) {
	if m.readRowFn != nil {
		return m.readRowFn(ctx, row, opts...)
	}
	return nil, nil
}

func (m *mockTableAPI) Apply(ctx context.Context, row string, mut *bigtable.Mutation, opts ...bigtable.ApplyOption) error {
	if m.applyFn != nil {
		return m.applyFn(ctx, row, mut, opts...)
	}
	return nil
}

func (m *mockTableAPI) ReadRows(_ context.Context, _ bigtable.RowSet, _ func(bigtable.Row) bool, _ ...bigtable.ReadOption) error {
	panic("mockTableAPI.ReadRows not implemented")
}
func (m *mockTableAPI) SampleRowKeys(_ context.Context) ([]string, error) {
	panic("mockTableAPI.SampleRowKeys not implemented")
}
func (m *mockTableAPI) ApplyBulk(_ context.Context, _ []string, _ []*bigtable.Mutation, _ ...bigtable.ApplyOption) ([]error, error) {
	panic("mockTableAPI.ApplyBulk not implemented")
}
func (m *mockTableAPI) ApplyReadModifyWrite(_ context.Context, _ string, _ *bigtable.ReadModifyWrite) (bigtable.Row, error) {
	panic("mockTableAPI.ApplyReadModifyWrite not implemented")
}

// newClassicFallbackWithMock builds a classicFallback without dialing a real
// Bigtable server by pre-populating the table cache with mocks. The supplied
// mocks map must use full V2 resource names as keys.
func newClassicFallbackWithMock(mocks map[string]bigtable.TableAPI) *classicFallback {
	tables := make(map[string]bigtable.TableAPI, len(mocks))
	for k, v := range mocks {
		tables[k] = v
	}
	return &classicFallback{c: nil, mu: sync.Mutex{}, tables: tables}
}

// --- classicLeafID -----------------------------------------------------------

func TestClassicLeafID_HappyPath(t *testing.T) {
	tests := []struct {
		name, segPrefix, want string
	}{
		{"projects/p/instances/i/tables/T", "tables/", "T"},
		{"projects/p/instances/i/materializedViews/MV", "materializedViews/", "MV"},
	}
	for _, tc := range tests {
		got, err := classicLeafID(tc.name, tc.segPrefix)
		if err != nil {
			t.Errorf("classicLeafID(%q, %q) error: %v", tc.name, tc.segPrefix, err)
		} else if got != tc.want {
			t.Errorf("classicLeafID(%q, %q) = %q, want %q", tc.name, tc.segPrefix, got, tc.want)
		}
	}
}

func TestClassicLeafID_MissingPrefix(t *testing.T) {
	_, err := classicLeafID("projects/p/instances/i/tables/T", "materializedViews/")
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want codes.Internal", err)
	}
}

func TestClassicLeafID_EmptyLeaf(t *testing.T) {
	_, err := classicLeafID("projects/p/instances/i/tables/", "tables/")
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want codes.Internal", err)
	}
}

func TestClassicLeafID_LeafWithSlash(t *testing.T) {
	_, err := classicLeafID("projects/p/instances/i/tables/T/extra", "tables/")
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want codes.Internal", err)
	}
}

// --- classicAVLeafIDs --------------------------------------------------------

func TestClassicAVLeafIDs_HappyPath(t *testing.T) {
	tableID, viewID, err := classicAVLeafIDs("projects/p/instances/i/tables/T/authorizedViews/V")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tableID != "T" {
		t.Errorf("tableID = %q, want T", tableID)
	}
	if viewID != "V" {
		t.Errorf("viewID = %q, want V", viewID)
	}
}

func TestClassicAVLeafIDs_MissingSegment(t *testing.T) {
	_, _, err := classicAVLeafIDs("projects/p/instances/i/tables/T")
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want codes.Internal", err)
	}
}

func TestClassicAVLeafIDs_EmptyViewID(t *testing.T) {
	_, _, err := classicAVLeafIDs("projects/p/instances/i/tables/T/authorizedViews/")
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want codes.Internal", err)
	}
}

// --- classicFallback.ReadRow range validation --------------------------------

const testTableName = "projects/p/instances/i/tables/t"

func TestClassicFallbackReadRow_RowKeyPath(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		readRowFn: func(_ context.Context, row string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
			gotKey = row
			return bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("v")}}}, nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte("mykey")}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	resp, err := cf.ReadRow(context.Background(), res, req)
	if err != nil {
		t.Fatalf("ReadRow error: %v", err)
	}
	if gotKey != "mykey" {
		t.Errorf("called ReadRow with key %q, want %q", gotKey, "mykey")
	}
	if len(resp.Chunks) == 0 {
		t.Error("expected non-empty chunks for found row")
	}
}

func TestClassicFallbackReadRow_ClosedClosedRange(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		readRowFn: func(_ context.Context, row string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
			gotKey = row
			return nil, nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("k")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("k")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	if _, err := cf.ReadRow(context.Background(), res, req); err != nil {
		t.Fatalf("ReadRow error: %v", err)
	}
	if gotKey != "k" {
		t.Errorf("called ReadRow with key %q, want %q", gotKey, "k")
	}
}

func TestClassicFallbackReadRow_NonSingleRowRange_ReturnsUnimplemented(t *testing.T) {
	cf := newClassicFallbackWithMock(nil)

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("z")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

func TestClassicFallbackReadRow_EmptyStartKey_ReturnsUnimplemented(t *testing.T) {
	cf := newClassicFallbackWithMock(nil)

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			// start_key_closed not set → GetStartKeyClosed() returns nil/empty
			EndKey: &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("k")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

func TestClassicFallbackReadRow_DefaultCase_ReturnsUnimplemented(t *testing.T) {
	cf := newClassicFallbackWithMock(nil)

	// No RowKeys and no RowRanges — hits the default case.
	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

// --- classicFallback.Close nil guard ----------------------------------------

func TestClassicFallbackClose_NilClient(t *testing.T) {
	cf := &classicFallback{c: nil}
	if err := cf.Close(); err != nil {
		t.Errorf("Close with nil client: %v", err)
	}
}

// --- classicFallback.MutateRow ----------------------------------------------

func TestClassicFallbackMutateRow_CallsApply(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		applyFn: func(_ context.Context, row string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
			gotKey = row
			return nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.MutateRowRequest{
		TableName: testTableName,
		RowKey:    []byte("thekey"),
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	if err := cf.MutateRow(context.Background(), res, req); err != nil {
		t.Fatalf("MutateRow error: %v", err)
	}
	if gotKey != "thekey" {
		t.Errorf("Apply called with key %q, want %q", gotKey, "thekey")
	}
}
