/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"fmt"

	"google.golang.org/api/iterator"
)

// mockRowIterator is a mock for the rowIterator type.
type mockRowIterator struct {
	pending []mockRow

	stopCalled bool
}

type mockRow struct {
	Row   *Row
	Error error
}

func newMockIterator(values ...any) *mockRowIterator {
	it := &mockRowIterator{}
	for _, row := range values {
		switch row := row.(type) {
		case *Row:
			it.pending = append(it.pending, mockRow{Row: row})
		case error:
			it.pending = append(it.pending, mockRow{Error: row})
		default:
			panic(fmt.Sprintf("unsupported type %T", row))
		}
	}
	return it
}

func (it *mockRowIterator) Next() (*Row, error) {
	if len(it.pending) == 0 {
		panic("no more rows to return")
	}

	v := it.pending[0]
	it.pending = it.pending[1:]

	return v.Row, v.Error
}

func (it *mockRowIterator) Do(f func(r *Row) error) error {
	defer it.Stop()
	for {
		row, err := it.Next()
		switch err {
		case iterator.Done:
			return nil
		case nil:
			if err = f(row); err != nil {
				return err
			}
		default:
			return err
		}
	}
}

func (it *mockRowIterator) Stop() {
	if it.stopCalled {
		panic("Stop has already been called")
	}
	it.stopCalled = true
}
