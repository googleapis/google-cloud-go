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
)

type fetchCall struct {
	tok    string          // The expected value of the pageToken.
	result *readDataResult // The result to return.
	err    error           // The error to return.
}

// pageFetcherStub services fetch requests by returning data from an in-memory list of values.
type pageFetcherStub struct {
	token string

	fetchCalls []fetchCall

	err error
}

func (cur *pageFetcherStub) fetch(ctx context.Context, c *Client, token string) (*readDataResult, error) {
	call := cur.fetchCalls[0]
	cur.fetchCalls = cur.fetchCalls[1:]
	if call.tok != token {
		cur.err = fmt.Errorf("Unexpected pagetoken: got:\n%v\nwant:\n%v", token, call.tok)
	}
	return call.result, call.err
}

func TestIterator(t *testing.T) {
	fetchFailure := errors.New("fetch failure")

	testCases := []struct {
		desc            string
		alreadyConsumed int64 // amount to advance offset before commencing reading.
		fetchCalls      []fetchCall
		want            []ValueList
		wantErr         error
	}{
		{
			desc: "Iteration over single empty page",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{},
					},
				},
			},
			want: []ValueList{},
		},
		{
			desc: "Iteration over single page",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}},
		},
		{
			desc: "Iteration over two pages",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
		},
		{
			desc: "Server response includes empty page",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "b",
						rows:      [][]Value{},
					},
				},
				{
					tok: "b",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
		},
		{
			desc: "Fetch error",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					// We returns some data from this fetch, but also an error.
					// So the end result should include only data from the previous fetch.
					err: fetchFailure,
					result: &readDataResult{
						pageToken: "b",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want:    []ValueList{{1, 2}, {11, 12}},
			wantErr: fetchFailure,
		},
		{
			desc: "Fetch of incomplete job",
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					err: errIncompleteJob,
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}, {101, 102}, {111, 112}},
		},
		{
			desc:            "Skip over a single element",
			alreadyConsumed: 1,
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{11, 12}, {101, 102}, {111, 112}},
		},
		{
			desc:            "Skip over an entire page",
			alreadyConsumed: 2,
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{101, 102}, {111, 112}},
		},
		{
			desc:            "Skip beyond start of second page",
			alreadyConsumed: 3,
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			want: []ValueList{{111, 112}},
		},
		{
			desc:            "Skip beyond all data",
			alreadyConsumed: 4,
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "a",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
				{
					tok: "a",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{101, 102}, {111, 112}},
					},
				},
			},
			// In this test case, Next will return false on its first call,
			// so we won't even attempt to call Get.
			want: []ValueList{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchCalls: tc.fetchCalls,
		}
		it := newIterator(nil, pf)
		it.offset += tc.alreadyConsumed

		values, err := consumeIterator(it)
		if err != nil {
			t.Fatalf("%s: %v", tc.desc, err)
		}

		if !(len(values) == 0 && len(tc.want) == 0) && !reflect.DeepEqual(values, tc.want) {
			t.Errorf("%s: values:\ngot: %v\nwant:%v", tc.desc, values, tc.want)
		}
		if it.Err() != tc.wantErr {
			t.Errorf("%s: iterator.Err:\ngot: %v\nwant: %v", tc.desc, it.Err(), tc.wantErr)
		}

		// Check whether there was an unexpected call to fetch.
		if pf.err != nil {
			t.Errorf("%s: %v", tc.desc, pf.err)
		}
		// Check whether any expected calls to fetch were not made.
		if len(pf.fetchCalls) != 0 {
			t.Errorf("%s: outstanding fetchCalls: %v", tc.desc, pf.fetchCalls)
		}
	}
}

// consumeIterator reads all values from an iterator and returns them.
func consumeIterator(it *Iterator) ([]ValueList, error) {
	var got []ValueList
	for it.Next(context.Background()) {
		var vals ValueList
		if err := it.Get(&vals); err != nil {
			return nil, fmt.Errorf("err calling Get: %v", err)
		} else {
			got = append(got, vals)
		}
	}

	return got, nil
}

func TestGetBeforeNext(t *testing.T) {
	// TODO: once mashalling/unmarshalling of iterators is implemented, do a similar test for unmarshalled iterators.
	pf := &pageFetcherStub{
		fetchCalls: []fetchCall{
			{
				tok: "",
				result: &readDataResult{
					pageToken: "",
					rows:      [][]Value{{1, 2}, {11, 12}},
				},
			},
		},
	}
	it := newIterator(nil, pf)
	var vals ValueList
	if err := it.Get(&vals); err == nil {
		t.Errorf("Expected error calling Get before Next")
	}
}

func TestGetDuringErrorState(t *testing.T) {
	pf := &pageFetcherStub{
		fetchCalls: []fetchCall{
			{err: errors.New("bang")},
		},
	}
	it := newIterator(nil, pf)
	var vals ValueList
	it.Next(context.Background())
	if it.Err() == nil {
		t.Errorf("Expected error after calling Next")
	}
	if err := it.Get(&vals); err == nil {
		t.Errorf("Expected error calling Get when iterator has a non-nil error.")
	}
}

func TestGetAfterFinished(t *testing.T) {
	testCases := []struct {
		alreadyConsumed int64 // amount to advance offset before commencing reading.
		fetchCalls      []fetchCall
		want            []ValueList
	}{
		{
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
			},
			want: []ValueList{{1, 2}, {11, 12}},
		},
		{
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{},
					},
				},
			},
			want: []ValueList{},
		},
		{
			alreadyConsumed: 100,
			fetchCalls: []fetchCall{
				{
					tok: "",
					result: &readDataResult{
						pageToken: "",
						rows:      [][]Value{{1, 2}, {11, 12}},
					},
				},
			},
			want: []ValueList{},
		},
	}

	for _, tc := range testCases {
		pf := &pageFetcherStub{
			fetchCalls: tc.fetchCalls,
		}
		it := newIterator(nil, pf)
		it.offset += tc.alreadyConsumed

		values, err := consumeIterator(it)
		if err != nil {
			t.Fatalf("%s", err)
		}

		if !(len(values) == 0 && len(tc.want) == 0) && !reflect.DeepEqual(values, tc.want) {
			t.Errorf("values: got:\n%v\nwant:\n%v", values, tc.want)
		}
		if it.Err() != nil {
			t.Fatalf("iterator.Err: got:\n%v\nwant:\n:nil", it.Err())
		}
		// Try calling Get again.
		var vals ValueList
		if err := it.Get(&vals); err == nil {
			t.Errorf("Expected error calling Get when there are no more values")
		}
	}
}
