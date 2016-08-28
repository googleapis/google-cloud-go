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
	"strconv"
	"testing"

	"golang.org/x/net/context"
	itest "google.golang.org/api/iterator/testing"
)

// readServiceStub services read requests by returning data from an in-memory list of values.
type listTablesServiceStub struct {
	expectedProject, expectedDataset string
	values                           [][]*Table        // contains pages of tables.
	pageTokens                       map[string]string // maps incoming page token to returned page token.

	service
}

func (s *listTablesServiceStub) listTables(ctx context.Context, projectID, datasetID, pageToken string) ([]*Table, string, error) {
	if projectID != s.expectedProject {
		return nil, "", errors.New("wrong project id")
	}
	if datasetID != s.expectedDataset {
		return nil, "", errors.New("wrong dataset id")
	}

	tables := s.values[0]
	s.values = s.values[1:]
	return tables, s.pageTokens[pageToken], nil
}

func TestListTables(t *testing.T) {
	t1 := &Table{ProjectID: "p1", DatasetID: "d1", TableID: "t1"}
	t2 := &Table{ProjectID: "p1", DatasetID: "d1", TableID: "t2"}
	t3 := &Table{ProjectID: "p1", DatasetID: "d1", TableID: "t3"}
	testCases := []struct {
		data       [][]*Table
		pageTokens map[string]string
		want       []*Table
	}{
		{
			data:       [][]*Table{{t1, t2}, {t3}},
			pageTokens: map[string]string{"": "a", "a": ""},
			want:       []*Table{t1, t2, t3},
		},
		{
			data:       [][]*Table{{t1, t2}, {t3}},
			pageTokens: map[string]string{"": ""}, // no more pages after first one.
			want:       []*Table{t1, t2},
		},
	}

	for _, tc := range testCases {
		c := &Client{
			service: &listTablesServiceStub{
				expectedProject: "x",
				expectedDataset: "y",
				values:          tc.data,
				pageTokens:      tc.pageTokens,
			},
			projectID: "x",
		}
		got, err := c.Dataset("y").ListTables(context.Background())
		if err != nil {
			t.Errorf("err calling ListTables: %v", err)
			continue
		}

		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("reading: got:\n%v\nwant:\n%v", got, tc.want)
		}
	}
}

func TestListTablesError(t *testing.T) {
	c := &Client{
		service: &listTablesServiceStub{
			expectedProject: "x",
			expectedDataset: "y",
		},
		projectID: "x",
	}
	// Test that service read errors are propagated back to the caller.
	// Passing "not y" as the dataset id will cause the service to return an error.
	_, err := c.Dataset("not y").ListTables(context.Background())
	if err == nil {
		// Read should not return an error; only Err should.
		t.Errorf("ListTables expected: non-nil err, got: nil")
	}
}

type listDatasetsFake struct {
	service

	projectID string
	datasets  []*Dataset
	hidden    map[*Dataset]bool
}

func (df *listDatasetsFake) listDatasets(_ context.Context, projectID string, pageSize int, pageToken string, listHidden bool, filter string) ([]*Dataset, string, error) {
	const maxPageSize = 2
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	if filter != "" {
		return nil, "", errors.New("filter not supported")
	}
	if projectID != df.projectID {
		return nil, "", errors.New("bad project ID")
	}
	start := 0
	if pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, "", err
		}
	}
	var (
		i             int
		result        []*Dataset
		nextPageToken string
	)
	for i = start; len(result) < pageSize && i < len(df.datasets); i++ {
		if df.hidden[df.datasets[i]] && !listHidden {
			continue
		}
		result = append(result, df.datasets[i])
	}
	if i < len(df.datasets) {
		nextPageToken = strconv.Itoa(i)
	}
	return result, nextPageToken, nil
}

func TestDatasets(t *testing.T) {
	service := &listDatasetsFake{projectID: "p"}
	datasets := []*Dataset{
		{"p", "a", service},
		{"p", "b", service},
		{"p", "hidden", service},
		{"p", "c", service},
	}
	service.datasets = datasets
	service.hidden = map[*Dataset]bool{datasets[2]: true}
	c := &Client{
		projectID: "p",
		service:   service,
	}
	msg, ok := itest.TestIterator(datasets,
		func() interface{} { it := c.Datasets(context.Background()); it.ListHidden = true; return it },
		func(it interface{}) (interface{}, error) { return it.(*DatasetIterator).Next() })
	if !ok {
		t.Fatalf("ListHidden=true: %s", msg)
	}

	msg, ok = itest.TestIterator([]*Dataset{datasets[0], datasets[1], datasets[3]},
		func() interface{} { it := c.Datasets(context.Background()); it.ListHidden = false; return it },
		func(it interface{}) (interface{}, error) { return it.(*DatasetIterator).Next() })
	if !ok {
		t.Fatalf("ListHidden=false: %s", msg)
	}
}
