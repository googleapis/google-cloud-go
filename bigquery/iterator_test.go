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
	"errors"
	"fmt"
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
)

type fetchResponse struct {
	result *readDataResult // The result to return.
	err    error           // The error to return.
}

// pageFetcherStub services fetch requests by returning data from an in-memory list of values.
type pageFetcherStub struct {
	fetchResponses map[string]fetchResponse

	err error
}

func (pf *pageFetcherStub) fetch(ctx context.Context, s service, token string) (*readDataResult, error) {
	call, ok := pf.fetchResponses[token]
	if !ok {
		pf.err = fmt.Errorf("Unexpected page token: %q", token)
	}
	return call.result, call.err
}

func (pf *pageFetcherStub) setPaging(pc *pagingConf) {}

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
		want           []ValueList
		wantErr        error
		wantSchema     Schema
	}{
		{
			desc: "Iteration over single empty page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{},
						schema:    Schema{},
					},
				},
			},
			want:       []ValueList{},
			wantSchema: Schema{},
		},
		{
			desc: "Iteration over single page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
			},
			want:       []ValueList{{1, 2}, {11, 12}},
			wantSchema: iiSchema,
		},
		{
			desc: "Iteration over single page with different schema",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{"1", 2}, {"11", 12}},
						schema:    siSchema,
					},
				},
			},
			want:       []ValueList{{"1", 2}, {"11", 12}},
			wantSchema: siSchema,
		},
		{
			desc: "Iteration over two pages",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
			wantSchema: iiSchema,
		},
		{
			desc: "Server response includes empty page",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &readDataResult{
						pageToken: "b",
						rows:      [][]Value{},
						schema:    iiSchema,
					},
				},
				"b": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
			wantSchema: iiSchema,
		},
		{
			desc: "Fetch error",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					// We returns some data from this fetch, but also an error.
					// So the end result should include only data from the previous fetch.
					err: fetchFailure,
					result: &readDataResult{
						pageToken: "b",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       []ValueList{{1, 2}, {11, 12}},
			wantErr:    fetchFailure,
			wantSchema: iiSchema,
		},

		{
			desc:      "Skip over an entire page",
			pageToken: "a",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
			},
			want:       []ValueList{{101, 102}, {111, 112}},
			wantSchema: iiSchema,
		},

		{
			desc:      "Skip beyond all data",
			pageToken: "b",
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
						schema:    iiSchema,
					},
				},
				"a": {
					result: &readDataResult{
						pageToken: "b",
						rows:      [][]Value{{101, 102}, {111, 112}},
						schema:    iiSchema,
					},
				},
				"b": {
					result: &readDataResult{},
				},
			},
			// In this test case, Next will return false on its first call,
			// so we won't even attempt to call Get.
			want:       []ValueList{},
			wantSchema: Schema{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchResponses: tc.fetchResponses,
		}
		it := newRowIterator(context.Background(), nil, pf)
		it.PageInfo().Token = tc.pageToken
		values, schema, err := consumeRowIterator(it)
		if err != tc.wantErr {
			t.Fatalf("%s: got %v, want %v", tc.desc, err, tc.wantErr)
		}
		if (len(values) != 0 || len(tc.want) != 0) && !reflect.DeepEqual(values, tc.want) {
			t.Errorf("%s: values:\ngot: %v\nwant:%v", tc.desc, values, tc.want)
		}
		if (len(schema) != 0 || len(tc.wantSchema) != 0) && !reflect.DeepEqual(schema, tc.wantSchema) {
			t.Errorf("%s: iterator.Schema:\ngot: %v\nwant: %v", tc.desc, schema, tc.wantSchema)
		}
	}
}

type valueListWithSchema struct {
	vals   ValueList
	schema Schema
}

func (v *valueListWithSchema) Load(vs []Value, s Schema) error {
	v.vals.Load(vs, s)
	v.schema = s
	return nil
}

// consumeRowIterator reads the schema and all values from a RowIterator and returns them.
func consumeRowIterator(it *RowIterator) ([]ValueList, Schema, error) {
	var got []ValueList
	var schema Schema
	for {
		var vls valueListWithSchema
		err := it.Next(&vls)
		if err == iterator.Done {
			return got, schema, nil
		}
		if err != nil {
			return got, schema, err
		}
		got = append(got, vls.vals)
		schema = vls.schema
	}
}

type delayedPageFetcher struct {
	pageFetcherStub
	delayCount int
}

func (pf *delayedPageFetcher) fetch(ctx context.Context, s service, token string) (*readDataResult, error) {
	if pf.delayCount > 0 {
		pf.delayCount--
		return nil, errIncompleteJob
	}
	return pf.pageFetcherStub.fetch(ctx, s, token)
}

func TestIterateIncompleteJob(t *testing.T) {
	want := []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}}
	pf := pageFetcherStub{
		fetchResponses: map[string]fetchResponse{
			"": {
				result: &readDataResult{
					pageToken: "a",
					rows:      [][]Value{{1, 2}, {11, 12}},
				},
			},
			"a": {
				result: &readDataResult{
					pageToken: "",
					rows:      [][]Value{{101, 102}, {111, 112}},
				},
			},
		},
	}
	dpf := &delayedPageFetcher{
		pageFetcherStub: pf,
		delayCount:      1,
	}
	it := newRowIterator(context.Background(), nil, dpf)

	values, _, err := consumeRowIterator(it)
	if err != nil {
		t.Fatal(err)
	}

	if (len(values) != 0 || len(want) != 0) && !reflect.DeepEqual(values, want) {
		t.Errorf("values: got:\n%v\nwant:\n%v", values, want)
	}
	if dpf.delayCount != 0 {
		t.Errorf("delayCount: got: %v, want: 0", dpf.delayCount)
	}
}

func TestNextDuringErrorState(t *testing.T) {
	pf := &pageFetcherStub{
		fetchResponses: map[string]fetchResponse{
			"": {err: errors.New("bang")},
		},
	}
	it := newRowIterator(context.Background(), nil, pf)
	var vals ValueList
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
		want           []ValueList
	}{
		{
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}},
		},
		{
			fetchResponses: map[string]fetchResponse{
				"": {
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{},
					},
				},
			},
			want: []ValueList{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchResponses: tc.fetchResponses,
		}
		it := newRowIterator(context.Background(), nil, pf)

		values, _, err := consumeRowIterator(it)
		if err != nil {
			t.Fatal(err)
		}
		if (len(values) != 0 || len(tc.want) != 0) && !reflect.DeepEqual(values, tc.want) {
			t.Errorf("values: got:\n%v\nwant:\n%v", values, tc.want)
		}
		// Try calling Get again.
		var vals ValueList
		if err := it.Next(&vals); err != iterator.Done {
			t.Errorf("Expected Done calling Next when there are no more values")
		}
	}
}
