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
	"reflect"
	"testing"

	"golang.org/x/net/context"
)

// readServiceStub services read requests by returning data from an in-memory list of values.
type readServiceStub struct {
	// values and pageTokens are used as sources of data to return in response to calls to readTabledata or readQuery.
	values     [][][]Value       // contains pages / rows / columns.
	pageTokens map[string]string // maps incoming page token to returned page token.

	// arguments are recorded for later inspection.
	readTabledataArgs    []*readTabledataConf
	readQueryResultsArgs []*readQueryConf

	service
}

func (s *readServiceStub) readValues(tok string) *readDataResult {
	result := &readDataResult{
		pageToken: s.pageTokens[tok],
		rows:      s.values[0],
	}
	s.values = s.values[1:]

	return result
}
func (s *readServiceStub) readTabledata(ctx context.Context, conf *readTabledataConf) (*readDataResult, error) {
	s.readTabledataArgs = append(s.readTabledataArgs, conf)
	return s.readValues(conf.paging.pageToken), nil
}

func (s *readServiceStub) readQuery(ctx context.Context, conf *readQueryConf) (*readDataResult, error) {
	s.readQueryResultsArgs = append(s.readQueryResultsArgs, conf)
	return s.readValues(conf.paging.pageToken), nil
}

func TestRead(t *testing.T) {
	// The data for the service stub to return is populated for each test case in the testCases for loop.
	service := &readServiceStub{}
	c := &Client{
		service: service,
	}

	queryJob := &Job{
		projectID: "project-id",
		jobID:     "job-id",
		service:   service,
		isQuery:   true,
	}

	for _, src := range []ReadSource{defaultTable, queryJob} {
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

		for _, tc := range testCases {
			service.values = tc.data
			service.pageTokens = tc.pageTokens
			if got, ok := doRead(t, c, src); ok {
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("reading: got:\n%v\nwant:\n%v", got, tc.want)
				}
			}
		}
	}
}

// doRead calls Read with a ReadSource. Get is repeatedly called on the Iterator returned by Read and the results are returned.
func doRead(t *testing.T, c *Client, src ReadSource) ([]ValueList, bool) {
	it, err := c.Read(context.Background(), src)
	if err != nil {
		t.Errorf("err calling Read: %v", err)
		return nil, false
	}
	var got []ValueList
	for it.Next(context.Background()) {
		var vals ValueList
		if err := it.Get(&vals); err != nil {
			t.Errorf("err calling Get: %v", err)
			return nil, false
		} else {
			got = append(got, vals)
		}
	}

	return got, true
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

// delayedReadStub simulates reading results from a query that has not yet
// completed. Its readQuery method initially reports that the query job is not
// yet complete. Subsequently, it proxies the request through to another
// service stub.
type delayedReadStub struct {
	numDelays int

	readServiceStub
}

func (s *delayedReadStub) readQuery(ctx context.Context, conf *readQueryConf) (*readDataResult, error) {
	if s.numDelays > 0 {
		s.numDelays--
		return nil, incompleteJobError
	}
	return s.readServiceStub.readQuery(ctx, conf)
}

// TestIncompleteJob tests that an Iterator which reads from a query job will block until the job is complete.
func TestIncompleteJob(t *testing.T) {
	service := &delayedReadStub{
		numDelays: 2,
		readServiceStub: readServiceStub{
			values: [][][]Value{{{1, 2}}},
		},
	}
	c := &Client{service: service}
	queryJob := &Job{
		projectID: "project-id",
		jobID:     "job-id",
		service:   service,
		isQuery:   true,
	}
	it, err := c.Read(context.Background(), queryJob)
	if err != nil {
		t.Fatalf("err calling Read: %v", err)
	}
	var got ValueList
	want := ValueList{1, 2}
	if !it.Next(context.Background()) {
		t.Fatalf("Next: got: false: want: true")
	}
	if err := it.Get(&got); err != nil {
		t.Fatalf("Error calling Get: %v", err)
	}
	if service.numDelays != 0 {
		t.Errorf("remaining numDelays : got: %v want:0", service.numDelays)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reading: got:\n%v\nwant:\n%v", got, want)
	}
}

type errorReadService struct {
	service
}

func (s *errorReadService) readTabledata(ctx context.Context, conf *readTabledataConf) (*readDataResult, error) {
	return nil, errors.New("bang!")
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

func TestReadTabledataOptions(t *testing.T) {
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

	if !reflect.DeepEqual(s.readTabledataArgs, want) {
		t.Errorf("reading: got:\n%v\nwant:\n%v", s.readTabledataArgs, want)
	}
}

func TestReadQueryOptions(t *testing.T) {
	// test that read options are propagated.
	s := &readServiceStub{
		values: [][][]Value{{{1, 2}}},
	}
	c := &Client{service: s}

	queryJob := &Job{
		projectID: "project-id",
		jobID:     "job-id",
		service:   s,
		isQuery:   true,
	}
	it, err := c.Read(context.Background(), queryJob, RecordsPerRequest(5))

	if err != nil {
		t.Fatalf("err calling Read: %v", err)
	}
	if !it.Next(context.Background()) {
		t.Fatalf("Next: got: false: want: true")
	}

	want := []*readQueryConf{&readQueryConf{
		projectID: "project-id",
		jobID:     "job-id",
		paging: pagingConf{pageToken: "",
			recordsPerRequest:    5,
			setRecordsPerRequest: true,
		},
	}}

	if !reflect.DeepEqual(s.readQueryResultsArgs, want) {
		t.Errorf("reading: got:\n%v\nwant:\n%v", s.readQueryResultsArgs, want)
	}
}
