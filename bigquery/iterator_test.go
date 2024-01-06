// Copyright 2015 Google LLC
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
	"context"
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/iterator"
)

type fetchResponse struct {
	result *fetchPageResult // The result to return.
	err    error            // The error to return.
}

// pageFetcherStub services fetch requests by returning data from an in-memory list of values.
type pageFetcherStub struct {
	fetchResponses map[string]fetchResponse
	err            error
}

func (pf *pageFetcherStub) fetchPage(ctx context.Context, _ *rowSource, _ Schema, _ uint64, _ int64, pageToken string) (*fetchPageResult, error) {
	call, ok := pf.fetchResponses[pageToken]
	if !ok {
		pf.err = fmt.Errorf("Unexpected page token: %q", pageToken)
	}
	return call.result, call.err
}

func TestRowIteratorCacheBehavior(t *testing.T) {

	testSchema := &bq.TableSchema{
		Fields: []*bq.TableFieldSchema{
			{Type: "INTEGER", Name: "field1"},
			{Type: "STRING", Name: "field2"},
		},
	}
	testRows := []*bq.TableRow{
		{F: []*bq.TableCell{
			{V: "1"},
			{V: "foo"},
		},
		},
	}
	convertedSchema := bqToSchema(testSchema)

	convertedRows, _ := convertRows(testRows, convertedSchema)

	testCases := []struct {
		inSource     *rowSource
		inSchema     Schema
		inStartIndex uint64
		inPageSize   int64
		inPageToken  string
		wantErr      error
		wantResult   *fetchPageResult
	}{
		{
			inSource: &rowSource{},
			wantErr:  errNoCacheData,
		},
		{
			// primary success case: schema in cache
			inSource: &rowSource{
				cachedSchema: testSchema,
				cachedRows:   testRows,
			},
			wantResult: &fetchPageResult{
				totalRows: uint64(len(convertedRows)),
				schema:    convertedSchema,
				rows:      convertedRows,
			},
		},
		{
			// secondary success case: schema provided
			inSource: &rowSource{
				cachedRows:      testRows,
				cachedNextToken: "foo",
			},
			inSchema: convertedSchema,
			wantResult: &fetchPageResult{
				totalRows: uint64(len(convertedRows)),
				schema:    convertedSchema,
				rows:      convertedRows,
				pageToken: "foo",
			},
		},
		{
			// misaligned page size.
			inSource: &rowSource{
				cachedSchema: testSchema,
				cachedRows:   testRows,
			},
			inPageSize: 99,
			wantErr:    errNoCacheData,
		},
		{
			// nonzero start.
			inSource: &rowSource{
				cachedSchema: testSchema,
				cachedRows:   testRows,
			},
			inStartIndex: 1,
			wantErr:      errNoCacheData,
		},
		{
			// data without schema
			inSource: &rowSource{
				cachedSchema: testSchema,
				cachedRows:   testRows,
			},
			inStartIndex: 1,
			wantErr:      errNoCacheData,
		},
		{
			// data without schema
			inSource: &rowSource{
				cachedSchema: testSchema,
				cachedRows:   testRows,
			},
			inStartIndex: 1,
			wantErr:      errNoCacheData,
		},
	}
	for _, tc := range testCases {
		gotResp, gotErr := fetchCachedPage(context.Background(), tc.inSource, tc.inSchema, tc.inStartIndex, tc.inPageSize, tc.inPageToken)
		if gotErr != tc.wantErr {
			t.Errorf("err mismatch.  got %v, want %v", gotErr, tc.wantErr)
		} else {
			if diff := testutil.Diff(gotResp, tc.wantResult,
				cmp.AllowUnexported(fetchPageResult{}, rowSource{}, Job{}, Client{}, Table{})); diff != "" {
				t.Errorf("response diff (got=-, want=+):\n%s", diff)
			}
		}
	}

}

func TestIterator(t *testing.T) {
	var (
		iiSchema = Schema{
			{Type: IntegerFieldType},
			{Type: IntegerFieldType},
		}
		siSchema = Schema{
			{Type: StringFieldType},
			{Type: IntegerFieldType},
		}
	)
	fetchFailure := errors.New("fetch failure")

	testCases := []struct {
		desc           string
		pageToken      string
		fetchResponses map[string]fetchResponse
		want           [][]Value
		wantErr        error
		wantSchema     Schema
		wantTotalRows  uint64
	}{
		{
			desc: "Iteration over single empty page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{},
						schema:    Schema{},
					},
				},
			},
			want:       [][]Value{},
			wantSchema: Schema{},
		},
		{
			desc: "Iteration over single page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
						totalRows: 4,
					},
				},
			},
			want:          [][]Value{{1, 2}, {11, 12}},
			wantSchema:    iiSchema,
			wantTotalRows: 4,
		},
		{
			desc: "Iteration over single page with different schema",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{"1", 2}, {"11", 12}},
						schema:    siSchema,
					},
				},
			},
			want:       [][]Value{{"1", 2}, {"11", 12}},
			wantSchema: siSchema,
		},
		{
			desc: "Iteration over two pages",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
						totalRows: 4,
					},
				},
				"a": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
						totalRows: 4,
					},
				},
			},
			want:          [][]Value{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
			wantSchema:    iiSchema,
			wantTotalRows: 4,
		},
		{
			desc: "Server response includes empty page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &fetchPageResult{
						pageToken: "b",
						rows:      [][]Value{},
						schema:    iiSchema,
					},
				},
				"b": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       [][]Value{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
			wantSchema: iiSchema,
		},
		{
			desc: "Fetch error",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					// We returns some data from this fetch, but also an error.
					// So the end result should include only data from the previous fetch.
					err: fetchFailure,
					result: &fetchPageResult{
						pageToken: "b",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       [][]Value{{1, 2}, {11, 12}},
			wantErr:    fetchFailure,
			wantSchema: iiSchema,
		},

		{
			desc:      "Skip over an entire page",
			pageToken: "a",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       [][]Value{{101, 102}, {111, 112}},
			wantSchema: iiSchema,
		},

		{
			desc:      "Skip beyond all data",
			pageToken: "b",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &fetchPageResult{
						pageToken: "b",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
				"b": {
					result: &fetchPageResult{},
				},
			},
			// In this test case, Next will return false on its first call,
			// so we won't even attempt to call Get.
			want:       [][]Value{},
			wantSchema: Schema{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchResponses: tc.fetchResponses,
		}
		it := newRowIterator(context.Background(), nil, pf.fetchPage)
		it.PageInfo().Token = tc.pageToken
		values, schema, totalRows, err := consumeRowIterator(it)
		if err != tc.wantErr {
			t.Fatalf("%s: got %v, want %v", tc.desc, err, tc.wantErr)
		}
		if (len(values) != 0 || len(tc.want) != 0) && !testutil.Equal(values, tc.want) {
			t.Errorf("%s: values:\ngot: %v\nwant:%v", tc.desc, values, tc.want)
		}
		if (len(schema) != 0 || len(tc.wantSchema) != 0) && !testutil.Equal(schema, tc.wantSchema) {
			t.Errorf("%s: iterator.Schema:\ngot: %v\nwant: %v", tc.desc, schema, tc.wantSchema)
		}
		if totalRows != tc.wantTotalRows {
			t.Errorf("%s: totalRows: got %d, want %d", tc.desc, totalRows, tc.wantTotalRows)
		}
	}
}

// consumeRowIterator reads the schema and all values from a RowIterator and returns them.
func consumeRowIterator(it *RowIterator) ([][]Value, Schema, uint64, error) {
	var (
		got       [][]Value
		schema    Schema
		totalRows uint64
	)
	for {
		var vls []Value
		err := it.Next(&vls)
		if err == iterator.Done {
			return got, schema, totalRows, nil
		}
		if err != nil {
			return got, schema, totalRows, err
		}
		got = append(got, vls)
		schema = it.Schema
		totalRows = it.TotalRows
	}
}

func TestNextDuringErrorState(t *testing.T) {
	pf := &pageFetcherStub{
		fetchResponses: map[string]fetchResponse{
			"": {err: errors.New("bang")},
		},
	}
	it := newRowIterator(context.Background(), nil, pf.fetchPage)
	var vals []Value
	if err := it.Next(&vals); err == nil {
		t.Errorf("Expected error after calling Next")
	}
	if err := it.Next(&vals); err == nil {
		t.Errorf("Expected error calling Next again when iterator has a non-nil error.")
	}
}

func TestNextAfterFinished(t *testing.T) {
	testCases := []struct {
		fetchResponses map[string]fetchResponse
		want           [][]Value
	}{
		{
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
			},
			want: [][]Value{{1, 2}, {11, 12}},
		},
		{
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &fetchPageResult{
						pageToken: "",
						rows:      [][]Value{},
					},
				},
			},
			want: [][]Value{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchResponses: tc.fetchResponses,
		}
		it := newRowIterator(context.Background(), nil, pf.fetchPage)

		values, _, _, err := consumeRowIterator(it)
		if err != nil {
			t.Fatal(err)
		}
		if (len(values) != 0 || len(tc.want) != 0) && !testutil.Equal(values, tc.want) {
			t.Errorf("values: got:\n%v\nwant:\n%v", values, tc.want)
		}
		// Try calling Get again.
		var vals []Value
		if err := it.Next(&vals); err != iterator.Done {
			t.Errorf("Expected Done calling Next when there are no more values")
		}
	}
}

func TestIteratorNextTypes(t *testing.T) {
	it := newRowIterator(context.Background(), nil, nil)
	for _, v := range []interface{}{3, "s", []int{}, &[]int{},
		map[string]Value{}, &map[string]interface{}{},
		struct{}{},
	} {
		if err := it.Next(v); err == nil {
			t.Errorf("%v: want error, got nil", v)
		}
	}
}

func TestIteratorSourceJob(t *testing.T) {
	testcases := []struct {
		description string
		src         *rowSource
		wantJob     *Job
	}{
		{
			description: "nil source",
			src:         nil,
			wantJob:     nil,
		},
		{
			description: "empty source",
			src:         &rowSource{},
			wantJob:     nil,
		},
		{
			description: "table source",
			src:         &rowSource{t: &Table{ProjectID: "p", DatasetID: "d", TableID: "t"}},
			wantJob:     nil,
		},
		{
			description: "job source",
			src:         &rowSource{j: &Job{projectID: "p", location: "l", jobID: "j"}},
			wantJob:     &Job{projectID: "p", location: "l", jobID: "j"},
		},
	}

	for _, tc := range testcases {
		// Don't pass a page func, we're not reading from the iterator.
		it := newRowIterator(context.Background(), tc.src, nil)
		got := it.SourceJob()
		// AllowUnexported because of the embedded client reference, which we're ignoring.
		if !cmp.Equal(got, tc.wantJob, cmp.AllowUnexported(Job{})) {
			t.Errorf("%s: mismatch on SourceJob, got %v want %v", tc.description, got, tc.wantJob)
		}
	}
}

func TestIteratorQueryID(t *testing.T) {
	testcases := []struct {
		description string
		src         *rowSource
		want        string
	}{
		{
			description: "nil source",
			src:         nil,
			want:        "",
		},
		{
			description: "empty source",
			src:         &rowSource{},
			want:        "",
		},
		{
			description: "populated id",
			src:         &rowSource{queryID: "foo"},
			want:        "foo",
		},
	}

	for _, tc := range testcases {
		// Don't pass a page func, we're not reading from the iterator.
		it := newRowIterator(context.Background(), tc.src, nil)
		got := it.QueryID()
		if got != tc.want {
			t.Errorf("%s: mismatch queryid, got %q want %q", tc.description, got, tc.want)
		}
	}
}
