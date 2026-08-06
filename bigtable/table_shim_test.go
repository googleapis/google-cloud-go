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

package bigtable

import (
	"context"
	"errors"
	"reflect"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockClassicTable stands in for a classic TableAPI (bigtable.Table
// wrapped in tableImpl). Records which method fired and forwards to
// per-method funcs.
type mockClassicTable struct {
	readRowFn   func(ctx context.Context, row string, opts ...ReadOption) (Row, error)
	applyFn     func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error
	readRowsFn  func(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error
	sampleFn    func(ctx context.Context) ([]string, error)
	applyBulkFn func(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error)
	rmwFn       func(ctx context.Context, row string, m *ReadModifyWrite) (Row, error)

	readRowCalls int
	applyCalls   int
}

func (m *mockClassicTable) ReadRow(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
	m.readRowCalls++
	if m.readRowFn != nil {
		return m.readRowFn(ctx, row, opts...)
	}
	return Row{"fam": []ReadItem{{Row: row}}}, nil
}
func (m *mockClassicTable) Apply(ctx context.Context, row string, mut *Mutation, opts ...ApplyOption) error {
	m.applyCalls++
	if m.applyFn != nil {
		return m.applyFn(ctx, row, mut, opts...)
	}
	return nil
}
func (m *mockClassicTable) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	if m.readRowsFn != nil {
		return m.readRowsFn(ctx, arg, f, opts...)
	}
	return nil
}
func (m *mockClassicTable) SampleRowKeys(ctx context.Context) ([]string, error) {
	if m.sampleFn != nil {
		return m.sampleFn(ctx)
	}
	return nil, nil
}
func (m *mockClassicTable) ApplyBulk(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
	if m.applyBulkFn != nil {
		return m.applyBulkFn(ctx, rowKeys, muts, opts...)
	}
	return nil, nil
}
func (m *mockClassicTable) ApplyReadModifyWrite(ctx context.Context, row string, mut *ReadModifyWrite) (Row, error) {
	if m.rmwFn != nil {
		return m.rmwFn(ctx, row, mut)
	}
	return nil, nil
}

// mockSessionTable is the proto-native session-side mock. Records
// which method fired and returns programmable responses.
type mockSessionTable struct {
	readRowFn   func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error)
	mutateRowFn func(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error)

	readRowCalls   int
	mutateRowCalls int
}

func (m *mockSessionTable) ReadRow(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
	m.readRowCalls++
	if m.readRowFn != nil {
		return m.readRowFn(ctx, req)
	}
	// Return a proto Row with one cell so protoRowToRow produces a
	// non-empty map (Row.Key() reads from the first ReadItem's Row
	// field, not from proto.Key).
	return &btpb.SessionReadRowResponse{Row: &btpb.Row{
		Key: req.GetKey(),
		Families: []*btpb.Family{{
			Name: "fam",
			Columns: []*btpb.Column{{
				Qualifier: []byte("q"),
				Cells:     []*btpb.Cell{{Value: []byte("v")}},
			}},
		}},
	}}, nil
}
func (m *mockSessionTable) MutateRow(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
	m.mutateRowCalls++
	if m.mutateRowFn != nil {
		return m.mutateRowFn(ctx, req)
	}
	return &btpb.SessionMutateRowResponse{}, nil
}
func (m *mockSessionTable) Close() error { return nil }

// TestTableShim_ReadRow_RoutesByDiverter verifies the diverter gates
// classic vs session routing on ReadRow.
func TestTableShim_ReadRow_RoutesByDiverter(t *testing.T) {
	t.Run("classic when SessionLoad=0.0", func(t *testing.T) {
		classic := &mockClassicTable{}
		sess := &mockSessionTable{}
		shim := NewTableShim(classic, sess, btransport.NewDiverter(0.0))

		row, err := shim.ReadRow(context.Background(), "r1")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if row.Key() != "r1" {
			t.Errorf("row.Key() = %q, want r1", row.Key())
		}
		if classic.readRowCalls != 1 {
			t.Errorf("classic.readRowCalls = %d, want 1", classic.readRowCalls)
		}
		if sess.readRowCalls != 0 {
			t.Errorf("sess.readRowCalls = %d, want 0", sess.readRowCalls)
		}
	})

	t.Run("session when SessionLoad=1.0", func(t *testing.T) {
		classic := &mockClassicTable{}
		sess := &mockSessionTable{}
		shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))

		row, err := shim.ReadRow(context.Background(), "r2")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if row.Key() != "r2" {
			t.Errorf("row.Key() = %q, want r2 (from protoRowToRow)", row.Key())
		}
		if classic.readRowCalls != 0 {
			t.Errorf("classic.readRowCalls = %d, want 0", classic.readRowCalls)
		}
		if sess.readRowCalls != 1 {
			t.Errorf("sess.readRowCalls = %d, want 1", sess.readRowCalls)
		}
	})

	t.Run("classic fallback when session is nil", func(t *testing.T) {
		classic := &mockClassicTable{}
		shim := NewTableShim(classic, nil, btransport.NewDiverter(1.0))

		_, err := shim.ReadRow(context.Background(), "r3")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if classic.readRowCalls != 1 {
			t.Errorf("classic.readRowCalls = %d, want 1 (must fall back when session=nil even with SessionLoad=1.0)", classic.readRowCalls)
		}
	})

	t.Run("session error propagates", func(t *testing.T) {
		wantErr := errors.New("session read failed")
		sess := &mockSessionTable{
			readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
				return nil, wantErr
			},
		}
		shim := NewTableShim(&mockClassicTable{}, sess, btransport.NewDiverter(1.0))
		_, err := shim.ReadRow(context.Background(), "r4")
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want unwrap to %v", err, wantErr)
		}
	})
}

// TestTableShim_Apply_ConditionalAlwaysClassic verifies that conditional
// mutations bypass the session path regardless of diverter setting —
// CheckAndMutateRow has no session equivalent.
func TestTableShim_Apply_ConditionalAlwaysClassic(t *testing.T) {
	classic := &mockClassicTable{}
	sess := &mockSessionTable{}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0)) // diverter says session

	cond := NewCondMutation(PassAllFilter(), NewMutation(), nil)
	if err := shim.Apply(context.Background(), "r", cond); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if classic.applyCalls != 1 {
		t.Errorf("classic.applyCalls = %d, want 1 (conditional must go classic)", classic.applyCalls)
	}
	if sess.mutateRowCalls != 0 {
		t.Errorf("sess.mutateRowCalls = %d, want 0 (conditional must NOT go session)", sess.mutateRowCalls)
	}
}

// TestTableShim_Apply_NonConditionalRoutesByDiverter verifies that
// non-conditional mutations follow the diverter.
func TestTableShim_Apply_NonConditionalRoutesByDiverter(t *testing.T) {
	t.Run("session when SessionLoad=1.0", func(t *testing.T) {
		classic := &mockClassicTable{}
		sess := &mockSessionTable{}
		shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))

		mut := NewMutation()
		mut.Set("fam", "col", 1_000_000, []byte("v"))
		if err := shim.Apply(context.Background(), "r", mut); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if classic.applyCalls != 0 || sess.mutateRowCalls != 1 {
			t.Errorf("classic=%d session=%d, want classic=0 session=1", classic.applyCalls, sess.mutateRowCalls)
		}
	})

	t.Run("classic when SessionLoad=0.0", func(t *testing.T) {
		classic := &mockClassicTable{}
		sess := &mockSessionTable{}
		shim := NewTableShim(classic, sess, btransport.NewDiverter(0.0))

		mut := NewMutation()
		mut.Set("fam", "col", 1_000_000, []byte("v"))
		if err := shim.Apply(context.Background(), "r", mut); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if classic.applyCalls != 1 || sess.mutateRowCalls != 0 {
			t.Errorf("classic=%d session=%d, want classic=1 session=0", classic.applyCalls, sess.mutateRowCalls)
		}
	})
}

// TestTableShim_ReadRows_AlwaysClassic — no session equivalent yet.
func TestTableShim_ReadRows_AlwaysClassic(t *testing.T) {
	classicCalled := 0
	classic := &mockClassicTable{
		readRowsFn: func(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
			classicCalled++
			return nil
		},
	}
	shim := NewTableShim(classic, &mockSessionTable{}, btransport.NewDiverter(1.0))
	if err := shim.ReadRows(context.Background(), RowRange{}, func(Row) bool { return true }); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if classicCalled != 1 {
		t.Errorf("classic.ReadRows calls = %d, want 1 (ReadRows always goes classic)", classicCalled)
	}
}

// TestTableShim_SampleRowKeys_AlwaysClassic — spec #14: no vRPC equivalent.
func TestTableShim_SampleRowKeys_AlwaysClassic(t *testing.T) {
	classicCalled := 0
	classic := &mockClassicTable{
		sampleFn: func(ctx context.Context) ([]string, error) {
			classicCalled++
			return []string{"a", "b"}, nil
		},
	}
	shim := NewTableShim(classic, &mockSessionTable{}, btransport.NewDiverter(1.0))
	if _, err := shim.SampleRowKeys(context.Background()); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if classicCalled != 1 {
		t.Errorf("classic.SampleRowKeys calls = %d, want 1 (no vRPC equivalent)", classicCalled)
	}
}

// TestTableShim_ApplyBulk_AlwaysClassic — spec #14: no vRPC equivalent.
func TestTableShim_ApplyBulk_AlwaysClassic(t *testing.T) {
	classicCalled := 0
	classic := &mockClassicTable{
		applyBulkFn: func(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
			classicCalled++
			return nil, nil
		},
	}
	shim := NewTableShim(classic, &mockSessionTable{}, btransport.NewDiverter(1.0))
	if _, err := shim.ApplyBulk(context.Background(), []string{"r1"}, []*Mutation{NewMutation()}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if classicCalled != 1 {
		t.Errorf("classic.ApplyBulk calls = %d, want 1 (no vRPC equivalent)", classicCalled)
	}
}

// TestTableShim_ApplyReadModifyWrite_AlwaysClassic — spec #14: no vRPC equivalent.
func TestTableShim_ApplyReadModifyWrite_AlwaysClassic(t *testing.T) {
	classicCalled := 0
	classic := &mockClassicTable{
		rmwFn: func(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
			classicCalled++
			return Row{"fam": []ReadItem{{Row: row}}}, nil
		},
	}
	shim := NewTableShim(classic, &mockSessionTable{}, btransport.NewDiverter(1.0))
	if _, err := shim.ApplyReadModifyWrite(context.Background(), "r1", NewReadModifyWrite()); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if classicCalled != 1 {
		t.Errorf("classic.ApplyReadModifyWrite calls = %d, want 1 (no vRPC equivalent)", classicCalled)
	}
}

// TestTableShim_NilSession_AllMethodsFallBackToClassic verifies
// SESSION_SPEC.md #14's nil-safety contract across the full TableAPI
// surface. The existing subtest in TestTableShim_ReadRow_RoutesByDiverter
// only covers ReadRow — this test extends the assertion to Apply plus
// every non-vRPC method so a future TableAPI addition is caught by
// coverage of the shim's fallback path.
func TestTableShim_NilSession_AllMethodsFallBackToClassic(t *testing.T) {
	classic := &mockClassicTable{}
	// session=nil, diverter says session (1.0) — nil-safety MUST win.
	shim := NewTableShim(classic, nil, btransport.NewDiverter(1.0))

	if _, err := shim.ReadRow(context.Background(), "r1"); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	if err := shim.Apply(context.Background(), "r1", NewMutation()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := shim.ReadRows(context.Background(), RowRange{}, func(Row) bool { return true }); err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	if _, err := shim.SampleRowKeys(context.Background()); err != nil {
		t.Fatalf("SampleRowKeys: %v", err)
	}
	if _, err := shim.ApplyBulk(context.Background(), []string{"r1"}, []*Mutation{NewMutation()}); err != nil {
		t.Fatalf("ApplyBulk: %v", err)
	}
	if _, err := shim.ApplyReadModifyWrite(context.Background(), "r1", NewReadModifyWrite()); err != nil {
		t.Fatalf("ApplyReadModifyWrite: %v", err)
	}

	// Every routable method must have gone classic (Apply=1, ReadRow=1);
	// non-routable methods are trivially classic. If session were nil-unsafe,
	// one of the two routable calls would nil-panic before reaching here.
	if classic.readRowCalls != 1 {
		t.Errorf("classic.readRowCalls = %d, want 1", classic.readRowCalls)
	}
	if classic.applyCalls != 1 {
		t.Errorf("classic.applyCalls = %d, want 1", classic.applyCalls)
	}
}

// TestTableShim_SessionErrorNotRetriedOnClassic verifies SESSION_SPEC.md
// #14's retry-safety contract: a session-path error MUST propagate as-is;
// the shim MUST NOT silently retry the failed op on the classic path.
// Doing so would violate the retry oracle (spec #9) — a session-side
// TransportFailure on a non-idempotent Apply is not automatically safe to
// re-run on classic.
//
// codes.Unimplemented is the sole exception (see
// TestTableShim_Apply_UnimplementedFallsBackToClassic). Here we use
// plain errors.New — status.Code returns Unknown — so the fallback
// branch does not fire and the retry-safety contract still holds.
func TestTableShim_SessionErrorNotRetriedOnClassic(t *testing.T) {
	wantErr := errors.New("session apply failed")
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		mutateRowFn: func(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
			return nil, wantErr
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))

	mut := NewMutation()
	mut.Set("fam", "col", 1_000_000, []byte("v"))
	err := shim.Apply(context.Background(), "r1", mut)

	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want unwrap to %v", err, wantErr)
	}
	if sess.mutateRowCalls != 1 {
		t.Errorf("sess.mutateRowCalls = %d, want 1 (single attempt)", sess.mutateRowCalls)
	}
	if classic.applyCalls != 0 {
		t.Errorf("classic.applyCalls = %d, want 0 — shim MUST NOT retry session failure on classic (spec #14 + #9)", classic.applyCalls)
	}
}

// TestProtoRowToRow pins the wire-shape → Row-map conversion contract
// against every branch in protoRowToRow, including the "no cells → nil"
// contract that matches classic Table.ReadRow (row-not-found is nil, not
// empty map — callers rely on `row == nil` for the not-found check).
func TestProtoRowToRow(t *testing.T) {
	cell := func(ts int64, v string, labels ...string) *btpb.Cell {
		return &btpb.Cell{TimestampMicros: ts, Value: []byte(v), Labels: labels}
	}
	col := func(q string, cells ...*btpb.Cell) *btpb.Column {
		return &btpb.Column{Qualifier: []byte(q), Cells: cells}
	}
	fam := func(name string, cols ...*btpb.Column) *btpb.Family {
		return &btpb.Family{Name: name, Columns: cols}
	}
	row := func(key string, fams ...*btpb.Family) *btpb.Row {
		return &btpb.Row{Key: []byte(key), Families: fams}
	}

	cases := []struct {
		name string
		in   *btpb.Row
		want Row
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty Families returns nil (matches classic not-found)",
			in:   row("k"),
			want: nil,
		},
		{
			name: "family with no columns returns nil",
			in:   row("k", fam("cf")),
			want: nil,
		},
		{
			name: "column with zero cells is skipped (family drops out entirely)",
			in:   row("k", fam("cf", col("q"))),
			want: nil,
		},
		{
			name: "single family/column/cell — happy path",
			in:   row("k", fam("cf", col("q", cell(1000, "v")))),
			want: Row{"cf": []ReadItem{{Row: "k", Column: "cf:q", Timestamp: 1000, Value: []byte("v")}}},
		},
		{
			name: "multiple cells per column preserves order (versioning)",
			in:   row("k", fam("cf", col("q", cell(3000, "v3"), cell(2000, "v2"), cell(1000, "v1")))),
			want: Row{"cf": []ReadItem{
				{Row: "k", Column: "cf:q", Timestamp: 3000, Value: []byte("v3")},
				{Row: "k", Column: "cf:q", Timestamp: 2000, Value: []byte("v2")},
				{Row: "k", Column: "cf:q", Timestamp: 1000, Value: []byte("v1")},
			}},
		},
		{
			name: "multiple columns per family concatenate into same map key in wire order",
			in:   row("k", fam("cf", col("qA", cell(1, "vA")), col("qB", cell(2, "vB")))),
			want: Row{"cf": []ReadItem{
				{Row: "k", Column: "cf:qA", Timestamp: 1, Value: []byte("vA")},
				{Row: "k", Column: "cf:qB", Timestamp: 2, Value: []byte("vB")},
			}},
		},
		{
			name: "multiple families land as separate map keys",
			in:   row("k", fam("cfA", col("q", cell(1, "vA"))), fam("cfB", col("q", cell(2, "vB")))),
			want: Row{
				"cfA": []ReadItem{{Row: "k", Column: "cfA:q", Timestamp: 1, Value: []byte("vA")}},
				"cfB": []ReadItem{{Row: "k", Column: "cfB:q", Timestamp: 2, Value: []byte("vB")}},
			},
		},
		{
			name: "same family name appearing twice in Families is appended (wire-preserving, no dedup)",
			in:   row("k", fam("cf", col("qA", cell(1, "vA"))), fam("cf", col("qB", cell(2, "vB")))),
			want: Row{"cf": []ReadItem{
				{Row: "k", Column: "cf:qA", Timestamp: 1, Value: []byte("vA")},
				{Row: "k", Column: "cf:qB", Timestamp: 2, Value: []byte("vB")},
			}},
		},
		{
			name: "Labels propagate through to ReadItem",
			in:   row("k", fam("cf", col("q", cell(1000, "v", "L1", "L2")))),
			want: Row{"cf": []ReadItem{{Row: "k", Column: "cf:q", Timestamp: 1000, Value: []byte("v"), Labels: []string{"L1", "L2"}}}},
		},
		{
			name: "TimestampMicros=0 preserved (server-set-to-now semantic is caller's job)",
			in:   row("k", fam("cf", col("q", cell(0, "v")))),
			want: Row{"cf": []ReadItem{{Row: "k", Column: "cf:q", Timestamp: 0, Value: []byte("v")}}},
		},
		{
			name: "empty qualifier produces column name \"cf:\"",
			in:   row("k", fam("cf", col("", cell(1000, "v")))),
			want: Row{"cf": []ReadItem{{Row: "k", Column: "cf:", Timestamp: 1000, Value: []byte("v")}}},
		},
		{
			name: "empty Value ([]byte{}) preserved",
			in:   row("k", fam("cf", col("q", cell(1000, "")))),
			want: Row{"cf": []ReadItem{{Row: "k", Column: "cf:q", Timestamp: 1000, Value: []byte("")}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := protoRowToRow(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("protoRowToRow(%+v)\n got  = %#v\n want = %#v", tc.in, got, tc.want)
			}
		})
	}
}

// --- UNIMPLEMENTED → classic fallback ---------------------------------------

// TestTableShim_ReadRow_UnimplementedFallsBackToClassic: session
// ReadRow returns codes.Unimplemented → shim transparently re-issues
// on classic and returns classic's row. See BreakerTrippedSkipsSession
// for the sticky-breaker arm.
func TestTableShim_ReadRow_UnimplementedFallsBackToClassic(t *testing.T) {
	classicRow := Row{"fam": []ReadItem{{Row: "r1", Column: "fam:q", Value: []byte("classic")}}}
	classic := &mockClassicTable{
		readRowFn: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
			return classicRow, nil
		},
	}
	sess := &mockSessionTable{
		readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
			return nil, status.Error(codes.Unimplemented, "session backend not implemented on this AFE")
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))

	got, err := shim.ReadRow(context.Background(), "r1")
	if err != nil {
		t.Fatalf("ReadRow err = %v, want nil (classic path should succeed after Unimplemented fallback)", err)
	}
	if !reflect.DeepEqual(got, classicRow) {
		t.Errorf("ReadRow returned %v, want classic row %v (fallback must surface classic's result, not session's error)", got, classicRow)
	}
	if sess.readRowCalls != 1 {
		t.Errorf("sess.readRowCalls = %d, want 1 (single attempt before fallback)", sess.readRowCalls)
	}
	if classic.readRowCalls != 1 {
		t.Errorf("classic.readRowCalls = %d, want 1 (fallback must dispatch to classic)", classic.readRowCalls)
	}
}

// TestTableShim_Apply_UnimplementedFallsBackToClassic — same shape
// for mutations. Non-conditional Apply is the only mutation
// entrypoint that reaches the session path.
func TestTableShim_Apply_UnimplementedFallsBackToClassic(t *testing.T) {
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		mutateRowFn: func(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
			return nil, status.Error(codes.Unimplemented, "session backend not implemented on this AFE")
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))

	mut := NewMutation()
	mut.Set("fam", "col", 1_000_000, []byte("v"))
	if err := shim.Apply(context.Background(), "r1", mut); err != nil {
		t.Fatalf("Apply err = %v, want nil (classic path should succeed after Unimplemented fallback)", err)
	}
	if sess.mutateRowCalls != 1 {
		t.Errorf("sess.mutateRowCalls = %d, want 1 (single attempt before fallback)", sess.mutateRowCalls)
	}
	if classic.applyCalls != 1 {
		t.Errorf("classic.applyCalls = %d, want 1 (fallback must dispatch to classic)", classic.applyCalls)
	}
}

// TestTableShim_BreakerTrippedSkipsSession: once the counter hits
// the interceptor's configured threshold, subsequent ReadRow /
// Apply skip the session path outright at useSession(). Threshold
// overridden to 1 so one Unimplemented trips.
func TestTableShim_BreakerTrippedSkipsSession(t *testing.T) {
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
			return nil, status.Error(codes.Unimplemented, "not implemented")
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0))
	withThreshold(t, shim, 1)

	// First ReadRow trips the breaker + falls back to classic.
	if _, err := shim.ReadRow(context.Background(), "r1"); err != nil {
		t.Fatalf("first ReadRow err = %v, want nil (classic fallback)", err)
	}
	if sess.readRowCalls != 1 {
		t.Fatalf("sess.readRowCalls after first call = %d, want 1", sess.readRowCalls)
	}

	// Nine more calls. If the breaker is working, sess.readRowCalls
	// stays at 1 (never re-attempted). classic.readRowCalls climbs.
	for i := 0; i < 9; i++ {
		if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
			t.Fatalf("post-trip ReadRow #%d err = %v, want nil", i+2, err)
		}
	}
	if sess.readRowCalls != 1 {
		t.Errorf("sess.readRowCalls after 10 total calls = %d, want 1 (breaker must gate useSession)", sess.readRowCalls)
	}
	if classic.readRowCalls != 10 {
		t.Errorf("classic.readRowCalls after 10 total calls = %d, want 10", classic.readRowCalls)
	}
}

// withThreshold swaps shim's interceptor for one with a smaller
// threshold so breaker-trip assertions don't have to iterate 30
// times. Accepts the TableAPI value returned by NewTableShim and
// casts to *TableShim internally. Caller must run this BEFORE
// issuing any requests through the shim (prior interceptor state is
// dropped).
func withThreshold(t *testing.T, shim TableAPI, threshold int32) {
	t.Helper()
	shim.(*TableShim).unimplemented = session.NewUnimplementedErrorInterceptor(threshold)
}

// TestTableShim_UnimplementedTripsBreakerAtThreshold: N-1
// Unimplementeds don't trip; the Nth does. Threshold=3 to keep the
// transition boundary explicit.
func TestTableShim_UnimplementedTripsBreakerAtThreshold(t *testing.T) {
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
			return nil, status.Error(codes.Unimplemented, "not implemented")
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0)).(*TableShim)
	withThreshold(t, shim, 3)

	// Calls 1 and 2 hit session (still under threshold), fall back to
	// classic per-call. Breaker stays untripped.
	for i := 0; i < 2; i++ {
		if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
			t.Fatalf("call #%d err = %v, want nil", i+1, err)
		}
		if shim.unimplemented.Bypass() {
			t.Fatalf("breaker tripped after %d/3 Unimplemented — expected trip only at threshold=3", i+1)
		}
	}

	// Call 3 hits session (still under, then increments to threshold)
	// AND flips the breaker.
	if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
		t.Fatalf("call #3 err = %v, want nil", err)
	}
	if !shim.unimplemented.Bypass() {
		t.Errorf("breaker NOT tripped after 3/3 Unimplemented — recordSessionOutcome must flip sessionUnimplemented when count reaches threshold")
	}
	if sess.readRowCalls != 3 {
		t.Errorf("sess.readRowCalls = %d, want 3 (one attempt per call until trip)", sess.readRowCalls)
	}

	// Call 4 skips session outright (breaker gate at useSession).
	if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
		t.Fatalf("call #4 err = %v, want nil (classic-only after trip)", err)
	}
	if sess.readRowCalls != 3 {
		t.Errorf("sess.readRowCalls after post-trip call = %d, want 3 (breaker must gate useSession)", sess.readRowCalls)
	}
	if classic.readRowCalls != 4 {
		t.Errorf("classic.readRowCalls = %d, want 4 (3 per-call fallbacks + 1 breaker-gated)", classic.readRowCalls)
	}
}

// TestTableShim_UnimplementedCounterResetsOnSuccess: a successful
// session response resets the count. 2 unimpl + 1 success + 2 unimpl
// is 4 total but never 3 consecutive, so the breaker must not trip.
func TestTableShim_UnimplementedCounterResetsOnSuccess(t *testing.T) {
	// Programmable session: first 2 calls Unimplemented, next 1 success,
	// next 2 Unimplemented — total 4 Unimplemented but never 3
	// consecutive, so breaker MUST NOT trip.
	callN := 0
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
			callN++
			switch callN {
			case 1, 2, 4, 5:
				return nil, status.Error(codes.Unimplemented, "unimpl")
			}
			// Call 3: session succeeds — resets counter.
			return &btpb.SessionReadRowResponse{Row: &btpb.Row{Key: req.GetKey()}}, nil
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0)).(*TableShim)
	withThreshold(t, shim, 3)

	for i := 0; i < 5; i++ {
		if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
			t.Fatalf("call #%d err = %v, want nil", i+1, err)
		}
	}
	if shim.unimplemented.Bypass() {
		t.Errorf("breaker tripped after 4 non-consecutive Unimplemented (call 3 succeeded — counter must have reset)")
	}
	if got := shim.unimplemented.Count(); got != 2 {
		t.Errorf("unimplementedCount = %d, want 2 (calls 4 and 5, counting from post-reset)", got)
	}
}

// TestTableShim_UnimplementedCounterResetsOnNonUnimplementedError: a
// non-Unimplemented error (e.g. Unavailable) also resets the counter,
// because that response proves the wire understood the RPC.
func TestTableShim_UnimplementedCounterResetsOnNonUnimplementedError(t *testing.T) {
	unavail := status.Error(codes.Unavailable, "boom")

	callN := 0
	classic := &mockClassicTable{}
	sess := &mockSessionTable{
		readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
			callN++
			switch callN {
			case 1, 2:
				return nil, status.Error(codes.Unimplemented, "unimpl")
			case 3:
				return nil, unavail // resets counter
			default:
				return nil, status.Error(codes.Unimplemented, "unimpl")
			}
		},
	}
	shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0)).(*TableShim)
	withThreshold(t, shim, 3)

	// Calls 1 and 2 fall back to classic transparently.
	for i := 0; i < 2; i++ {
		if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
			t.Fatalf("call #%d err = %v, want nil", i+1, err)
		}
	}
	// Call 3 propagates Unavailable (non-Unimplemented — no fallback).
	if _, err := shim.ReadRow(context.Background(), "r"); !errors.Is(err, unavail) {
		t.Fatalf("call #3 err = %v, want unwrap to %v", err, unavail)
	}
	if got := shim.unimplemented.Count(); got != 0 {
		t.Fatalf("unimplementedCount after Unavailable = %d, want 0 (non-Unimplemented response must reset counter)", got)
	}
	// Calls 4 and 5 Unimplemented — counter climbs from 0 again, still
	// below threshold=3.
	for i := 0; i < 2; i++ {
		if _, err := shim.ReadRow(context.Background(), "r"); err != nil {
			t.Fatalf("call #%d err = %v, want nil", i+4, err)
		}
	}
	if shim.unimplemented.Bypass() {
		t.Errorf("breaker tripped after Unavailable reset — counter should be at 2, not threshold=3")
	}
}

// TestTableShim_NonUnimplementedError_DoesNotFallBack: for every gRPC
// code other than Unimplemented, the session error propagates
// unchanged and the breaker does not trip. Fallback is a
// capability-signal concession only.
func TestTableShim_NonUnimplementedError_DoesNotFallBack(t *testing.T) {
	nonFallbackCodes := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.PermissionDenied,
		codes.Canceled,
		codes.Internal,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
	}

	for _, code := range nonFallbackCodes {
		t.Run(code.String(), func(t *testing.T) {
			sessErr := status.Error(code, "session err")
			classic := &mockClassicTable{}
			sess := &mockSessionTable{
				readRowFn: func(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
					return nil, sessErr
				},
				mutateRowFn: func(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
					return nil, sessErr
				},
			}
			shim := NewTableShim(classic, sess, btransport.NewDiverter(1.0)).(*TableShim)

			// ReadRow: session err must propagate, no classic fallback.
			_, gotRead := shim.ReadRow(context.Background(), "r")
			if !errors.Is(gotRead, sessErr) {
				t.Errorf("ReadRow err = %v, want unwrap to %v", gotRead, sessErr)
			}
			if classic.readRowCalls != 0 {
				t.Errorf("classic.readRowCalls = %d, want 0 (only Unimplemented triggers fallback)", classic.readRowCalls)
			}

			// Apply: same contract for mutations.
			mut := NewMutation()
			mut.Set("fam", "col", 1_000_000, []byte("v"))
			gotApply := shim.Apply(context.Background(), "r", mut)
			if !errors.Is(gotApply, sessErr) {
				t.Errorf("Apply err = %v, want unwrap to %v", gotApply, sessErr)
			}
			if classic.applyCalls != 0 {
				t.Errorf("classic.applyCalls = %d, want 0 (only Unimplemented triggers fallback)", classic.applyCalls)
			}

			// Breaker must NOT have tripped — subsequent calls still route session.
			if shim.unimplemented.Bypass() {
				t.Errorf("sessionUnimplemented tripped after code=%v; only codes.Unimplemented may trip it", code)
			}
		})
	}
}
