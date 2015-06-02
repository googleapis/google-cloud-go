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
	"fmt"
	"reflect"
	"testing"

	"golang.org/x/net/context"
)

// readServiceStub services read requests by returning data from an in-memory list of values.
type readServiceStub struct {
	values     [][][]Value          // contains pages / rows / columns.
	pageTokens map[string]string    // maps incoming page token to returned page token.
	arguments  []*readTabledataConf // arguments are recorded for later inspection.

	service
}

func (s *readServiceStub) readTabledata(ctx context.Context, conf *readTabledataConf) (*readTabledataResult, error) {
	s.arguments = append(s.arguments, conf)

	result := &readTabledataResult{
		pageToken: s.pageTokens[conf.paging.pageToken],
		rows:      s.values[0],
	}
	s.values = s.values[1:]

	return result, nil
}

func TestReadTable(t *testing.T) {
	testCases := []struct {
		data       [][][]Value
		pageTokens map[string]string
		want       []ValueList
	}{
		{
			data:       [][][]Value{{{1, 2}, {11, 12}}, {{30, 40}, {31, 41}}},
			pageTokens: map[string]string{"": "a", "a": ""},
			want:       []ValueList{{1, 2}, {11, 12}, {30, 40}, {31, 41}},
		},
		{
			data:       [][][]Value{{{1, 2}, {11, 12}}, {{30, 40}, {31, 41}}},
			pageTokens: map[string]string{"": ""}, // no more pages after first one.
			want:       []ValueList{{1, 2}, {11, 12}},
		},
	}

Cases:
	for _, tc := range testCases {
		c := &Client{
			service: &readServiceStub{
				values:     tc.data,
				pageTokens: tc.pageTokens,
			},
		}
		it, err := c.Read(context.Background(), defaultTable)
		if err != nil {
			t.Errorf("err calling Read: %v", err)
			continue
		}
		var got []ValueList
		for it.Next(context.Background()) {
			var vals ValueList
			if err := it.Get(&vals); err != nil {
				t.Errorf("err calling Get: %v", err)
				continue Cases
			} else {
				got = append(got, vals)
			}
		}

		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("reading: got:\n%v\nwant:\n%v", got, tc.want)
		}
	}
}

func TestNoMoreValues(t *testing.T) {
	c := &Client{
		service: &readServiceStub{
			values: [][][]Value{{{1, 2}, {11, 12}}},
		},
	}
	it, err := c.Read(context.Background(), defaultTable)
	if err != nil {
		t.Fatalf("err calling Read: %v", err)
	}
	var vals ValueList
	// We expect to retrieve two values and then fail on the next attempt.
	if !it.Next(context.Background()) {
		t.Fatalf("Next: got: false: want: true")
	}
	if !it.Next(context.Background()) {
		t.Fatalf("Next: got: false: want: true")
	}
	if err := it.Get(&vals); err != nil {
		t.Fatalf("Get: got: %v: want: nil", err)
	}
	if it.Next(context.Background()) {
		t.Fatalf("Next: got: true: want: false")
	}
	if err := it.Get(&vals); err == nil {
		t.Fatalf("Get: got: %v: want: non-nil", err)
	}
}

type errorReadService struct {
	service
}

func (s *errorReadService) readTabledata(ctx context.Context, conf *readTabledataConf) (*readTabledataResult, error) {
	return nil, fmt.Errorf("bang!")
}

func TestReadError(t *testing.T) {
	// test that service read errors are propagated back to the caller.
	c := &Client{service: &errorReadService{}}
	it, err := c.Read(context.Background(), defaultTable)
	if err != nil {
		// Read should not return an error; only Err should.
		t.Fatalf("err calling Read: %v", err)
	}
	if it.Next(context.Background()) {
		t.Fatalf("Next: got: true: want: false")
	}
	if err := it.Err(); err.Error() != "bang!" {
		t.Fatalf("Get: got: %v: want: bang!", err)
	}
}

func TestReadOptions(t *testing.T) {
	// test that read options are propagated.
	s := &readServiceStub{
		values: [][][]Value{{{1, 2}}},
	}
	c := &Client{service: s}
	it, err := c.Read(context.Background(), defaultTable, RecordsPerRequest(5))

	if err != nil {
		t.Fatalf("err calling Read: %v", err)
	}
	if !it.Next(context.Background()) {
		t.Fatalf("Next: got: false: want: true")
	}

	want := []*readTabledataConf{&readTabledataConf{
		projectID: "project-id",
		datasetID: "dataset-id",
		tableID:   "table-id",
		paging: pagingConf{pageToken: "",
			recordsPerRequest:    5,
			setRecordsPerRequest: true,
		},
	}}

	if !reflect.DeepEqual(s.arguments, want) {
		t.Errorf("reading: got:\n%v\nwant:\n%v", s.arguments, want)
	}
}
