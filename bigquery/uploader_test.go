// Copyright 2015 Google Inc. All Rights Reserved.
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

package bigquery

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"
)

type testSaver struct {
	ir  *insertionRow
	err error
}

func (ts testSaver) Save() (map[string]Value, string, error) {
	return ts.ir.Row, ts.ir.InsertID, ts.err
}

func TestRejectsNonValueSavers(t *testing.T) {
	u := Uploader{defaultTable}

	testCases := []struct {
		src interface{}
	}{
		{
			src: 1,
		},
		{
			src: []int{1, 2},
		},
		{
			src: []interface{}{
				testSaver{ir: &insertionRow{"a", map[string]Value{"one": 1}}},
				1,
			},
		},
	}

	for _, tc := range testCases {
		if err := u.Put(context.Background(), tc.src); err == nil {
			t.Errorf("put value: %v; got err: %v; want nil", tc.src, err)
		}
	}
}

type insertRowsRecorder struct {
	rowBatches [][]*insertionRow
	service
}

func (irr *insertRowsRecorder) insertRows(ctx context.Context, projectID, datasetID, tableID string, rows []*insertionRow) error {
	irr.rowBatches = append(irr.rowBatches, rows)
	return nil
}

func TestInsertsData(t *testing.T) {
	table := &Table{
		ProjectID: "project-id",
		DatasetID: "dataset-id",
		TableID:   "table-id",
	}

	testCases := []struct {
		data [][]*insertionRow
	}{
		{
			data: [][]*insertionRow{
				{
					&insertionRow{"a", map[string]Value{"one": 1}},
				},
			},
		},
		{

			data: [][]*insertionRow{
				{
					&insertionRow{"a", map[string]Value{"one": 1}},
					&insertionRow{"b", map[string]Value{"two": 2}},
				},
			},
		},
		{

			data: [][]*insertionRow{
				{
					&insertionRow{"a", map[string]Value{"one": 1}},
				},
				{
					&insertionRow{"b", map[string]Value{"two": 2}},
				},
			},
		},
		{

			data: [][]*insertionRow{
				{
					&insertionRow{"a", map[string]Value{"one": 1}},
					&insertionRow{"b", map[string]Value{"two": 2}},
				},
				{
					&insertionRow{"c", map[string]Value{"three": 3}},
					&insertionRow{"d", map[string]Value{"four": 4}},
				},
			},
		},
	}
	for _, tc := range testCases {
		irr := &insertRowsRecorder{}
		table.service = irr
		u := Uploader{table}
		for _, batch := range tc.data {
			if len(batch) == 0 {
				continue
			}
			var toUpload interface{}
			if len(batch) == 1 {
				toUpload = testSaver{ir: batch[0]}
			} else {
				savers := []testSaver{}
				for _, row := range batch {
					savers = append(savers, testSaver{ir: row})
				}
				toUpload = savers
			}

			err := u.Put(context.Background(), toUpload)
			if err != nil {
				t.Errorf("expected successful Put of ValueSaver; got: %v")
			}
		}
		if got, want := irr.rowBatches, tc.data; !reflect.DeepEqual(got, want) {
			t.Errorf("got: %v, want: %v", got, want)
		}
	}
}
