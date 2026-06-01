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
	"testing"

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
