// Copyright 2021 Google LLC
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
	"fmt"
	"strconv"
	"testing"

	bq "google.golang.org/api/bigquery/v2"
	itest "google.golang.org/api/iterator/testing"
)

type listRowAccessPoliciesStub struct {
	expectedProject, expectedDataset, expectedTable string
	policies                                        []*bq.RowAccessPolicy
}

func (s *listRowAccessPoliciesStub) listPolicies(it *RowAccessPolicyIterator, pageSize int, pageToken string) (*bq.ListRowAccessPoliciesResponse, error) {
	if it.table.ProjectID != s.expectedProject {
		return nil, fmt.Errorf("wrong project id: %q", it.table.ProjectID)
	}
	if it.table.DatasetID != s.expectedDataset {
		return nil, fmt.Errorf("wrong dataset id: %q", it.table.DatasetID)
	}
	if it.table.TableID != s.expectedTable {
		return nil, fmt.Errorf("wrong table id: %q", it.table.TableID)
	}
	const maxPageSize = 2
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	start := 0
	if pageToken != "" {
		var err error
		start, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, err
		}
	}
	end := start + pageSize
	if end > len(s.policies) {
		end = len(s.policies)
	}
	nextPageToken := ""
	if end < len(s.policies) {
		nextPageToken = strconv.Itoa(end)
	}
	return &bq.ListRowAccessPoliciesResponse{
		RowAccessPolicies: s.policies[start:end],
		NextPageToken:     nextPageToken,
	}, nil
}

func TestRowAccessPolicies(t *testing.T) {
	c := &Client{projectID: "p1"}
	inPolicies := []*bq.RowAccessPolicy{
		{RowAccessPolicyReference: &bq.RowAccessPolicyReference{
			ProjectId: "p1", DatasetId: "d1", TableId: "t1", PolicyId: "pol1",
		}},
		{RowAccessPolicyReference: &bq.RowAccessPolicyReference{
			ProjectId: "p1", DatasetId: "d1", TableId: "t1", PolicyId: "pol2",
		}},
		{RowAccessPolicyReference: &bq.RowAccessPolicyReference{
			ProjectId: "p1", DatasetId: "d1", TableId: "t1", PolicyId: "pol3",
		}},
	}
	outPolicies := []*RowAccessPolicy{
		{ProjectID: "p1", DatasetID: "d1", TableID: "t1", PolicyID: "pol1", c: c},
		{ProjectID: "p1", DatasetID: "d1", TableID: "t1", PolicyID: "pol2", c: c},
		{ProjectID: "p1", DatasetID: "d1", TableID: "t1", PolicyID: "pol3", c: c},
	}

	lps := &listRowAccessPoliciesStub{
		expectedProject: "p1",
		expectedDataset: "d1",
		expectedTable:   "t1",
		policies:        inPolicies,
	}
	old := listPolicies
	listPolicies = lps.listPolicies
	defer func() { listPolicies = old }()

	msg, ok := itest.TestIterator(outPolicies,
		func() interface{} { return c.Dataset("d1").Table("t1").RowAccessPolicies(context.Background()) },
		func(it interface{}) (interface{}, error) { return it.(*RowAccessPolicyIterator).Next() })
	if !ok {
		t.Error(msg)
	}
}
