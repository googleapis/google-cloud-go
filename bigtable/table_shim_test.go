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
	internal "cloud.google.com/go/bigtable/internal/transport"
)

type mockTableAPI struct {
	readRowFunc              func(ctx context.Context, row string, opts ...ReadOption) (Row, error)
	applyFunc                func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error
	readRowsFunc             func(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error
	sampleRowKeysFunc        func(ctx context.Context) ([]string, error)
	applyBulkFunc            func(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error)
	applyReadModifyWriteFunc func(ctx context.Context, row string, m *ReadModifyWrite) (Row, error)
}

func (m *mockTableAPI) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	if m.readRowsFunc != nil {
		return m.readRowsFunc(ctx, arg, f, opts...)
	}
	return nil
}

func (m *mockTableAPI) ReadRow(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
	if m.readRowFunc != nil {
		return m.readRowFunc(ctx, row, opts...)
	}
	return nil, nil
}

func (m *mockTableAPI) SampleRowKeys(ctx context.Context) ([]string, error) {
	if m.sampleRowKeysFunc != nil {
		return m.sampleRowKeysFunc(ctx)
	}
	return nil, nil
}

func (m *mockTableAPI) Apply(ctx context.Context, row string, mutation *Mutation, opts ...ApplyOption) error {
	if m.applyFunc != nil {
		return m.applyFunc(ctx, row, mutation, opts...)
	}
	return nil
}

func (m *mockTableAPI) ApplyBulk(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
	if m.applyBulkFunc != nil {
		return m.applyBulkFunc(ctx, rowKeys, muts, opts...)
	}
	return nil, nil
}

func (m *mockTableAPI) ApplyReadModifyWrite(ctx context.Context, row string, rmw *ReadModifyWrite) (Row, error) {
	if m.applyReadModifyWriteFunc != nil {
		return m.applyReadModifyWriteFunc(ctx, row, rmw)
	}
	return nil, nil
}

func TestTableShim_ReadRow(t *testing.T) {
	dummyRow := Row{"fam": []ReadItem{{Row: "row1"}}}
	dummyErr := errors.New("dummy error")

	t.Run("Classic only when UseSession is false", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				classicCalled = true
				return dummyRow, nil
			},
		}
		session := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				sessionCalled = true
				return nil, dummyErr
			},
		}

		diverter := internal.NewDiverter(0.0) // 0% load
		shim := NewTableShim(classic, session, diverter)

		res, err := shim.ReadRow(context.Background(), "row1")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if res.Key() != "row1" {
			t.Errorf("Expected row key row1, got %s", res.Key())
		}
		if !classicCalled {
			t.Error("Expected classic to be called")
		}
		if sessionCalled {
			t.Error("Expected session NOT to be called")
		}
	})

	t.Run("Session only when UseSession is true and succeeds", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				classicCalled = true
				return nil, dummyErr
			},
		}
		session := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				sessionCalled = true
				return dummyRow, nil
			},
		}

		diverter := internal.NewDiverter(1.0) // 100% load
		shim := NewTableShim(classic, session, diverter)

		res, err := shim.ReadRow(context.Background(), "row1")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if res.Key() != "row1" {
			t.Errorf("Expected row key row1, got %s", res.Key())
		}
		if classicCalled {
			t.Error("Expected classic NOT to be called")
		}
		if !sessionCalled {
			t.Error("Expected session to be called")
		}
	})

	t.Run("Session fails and returns error directly without falling back to classic", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				classicCalled = true
				return dummyRow, nil
			},
		}
		session := &mockTableAPI{
			readRowFunc: func(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
				sessionCalled = true
				return nil, dummyErr
			},
		}

		diverter := internal.NewDiverter(1.0)
		shim := NewTableShim(classic, session, diverter)

		_, err := shim.ReadRow(context.Background(), "row1")
		if err != dummyErr {
			t.Errorf("Expected session error %v, got %v", dummyErr, err)
		}
		if classicCalled {
			t.Error("Expected classic NOT to be called")
		}
		if !sessionCalled {
			t.Error("Expected session to be called")
		}
	})
}

func TestTableShim_Apply(t *testing.T) {
	dummyErr := errors.New("dummy error")

	t.Run("Classic only when UseSession is false", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				classicCalled = true
				return nil
			},
		}
		session := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				sessionCalled = true
				return dummyErr
			},
		}

		diverter := internal.NewDiverter(0.0)
		shim := NewTableShim(classic, session, diverter)

		err := shim.Apply(context.Background(), "row1", NewMutation())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !classicCalled {
			t.Error("Expected classic to be called")
		}
		if sessionCalled {
			t.Error("Expected session NOT to be called")
		}
	})

	t.Run("Session only when UseSession is true and succeeds", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				classicCalled = true
				return dummyErr
			},
		}
		session := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				sessionCalled = true
				return nil
			},
		}

		diverter := internal.NewDiverter(1.0)
		shim := NewTableShim(classic, session, diverter)

		err := shim.Apply(context.Background(), "row1", NewMutation())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if classicCalled {
			t.Error("Expected classic NOT to be called")
		}
		if !sessionCalled {
			t.Error("Expected session to be called")
		}
	})

	t.Run("Session fails and returns error directly without falling back to classic", func(t *testing.T) {
		classicCalled := false
		sessionCalled := false

		classic := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				classicCalled = true
				return nil
			},
		}
		session := &mockTableAPI{
			applyFunc: func(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
				sessionCalled = true
				return dummyErr
			},
		}

		diverter := internal.NewDiverter(1.0)
		shim := NewTableShim(classic, session, diverter)

		err := shim.Apply(context.Background(), "row1", NewMutation())
		if err != dummyErr {
			t.Errorf("Expected session error %v, got %v", dummyErr, err)
		}
		if classicCalled {
			t.Error("Expected classic NOT to be called")
		}
		if !sessionCalled {
			t.Error("Expected session to be called")
		}
	})
}

func TestTableShim_DelegatedMethods(t *testing.T) {
	classicCalled := false
	sessionCalled := false

	classic := &mockTableAPI{
		readRowsFunc: func(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
			classicCalled = true
			return nil
		},
		sampleRowKeysFunc: func(ctx context.Context) ([]string, error) {
			classicCalled = true
			return nil, nil
		},
		applyBulkFunc: func(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
			classicCalled = true
			return nil, nil
		},
		applyReadModifyWriteFunc: func(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
			classicCalled = true
			return nil, nil
		},
	}
	session := &mockTableAPI{
		readRowsFunc: func(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
			sessionCalled = true
			return nil
		},
		sampleRowKeysFunc: func(ctx context.Context) ([]string, error) {
			sessionCalled = true
			return nil, nil
		},
		applyBulkFunc: func(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
			sessionCalled = true
			return nil, nil
		},
		applyReadModifyWriteFunc: func(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
			sessionCalled = true
			return nil, nil
		},
	}

	diverter := internal.NewDiverter(1.0) // Even with 100% session load, these delegate to classic
	shim := NewTableShim(classic, session, diverter)

	// ReadRows
	classicCalled = false
	_ = shim.ReadRows(context.Background(), RowRange{}, func(r Row) bool { return true })
	if !classicCalled || sessionCalled {
		t.Errorf("ReadRows: expected classic called: %v, session called: %v", classicCalled, sessionCalled)
	}

	// SampleRowKeys
	classicCalled = false
	sessionCalled = false
	_, _ = shim.SampleRowKeys(context.Background())
	if !classicCalled || sessionCalled {
		t.Errorf("SampleRowKeys: expected classic called: %v, session called: %v", classicCalled, sessionCalled)
	}

	// ApplyBulk
	classicCalled = false
	sessionCalled = false
	_, _ = shim.ApplyBulk(context.Background(), nil, nil)
	if !classicCalled || sessionCalled {
		t.Errorf("ApplyBulk: expected classic called: %v, session called: %v", classicCalled, sessionCalled)
	}

	// ApplyReadModifyWrite
	classicCalled = false
	sessionCalled = false
	_, _ = shim.ApplyReadModifyWrite(context.Background(), "row1", nil)
	if !classicCalled || sessionCalled {
		t.Errorf("ApplyReadModifyWrite: expected classic called: %v, session called: %v", classicCalled, sessionCalled)
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
