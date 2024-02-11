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

package firestore

import (
	"context"
	"testing"
)

func TestCGR_TestQueryPartition_ToQuery(t *testing.T) {
	cgr := newCollectionGroupRef(testClient, testClient.path(), "collectionID")
	qp := queryPartition{
		CollectionGroupQuery: cgr.Query.OrderBy(DocumentID, Asc),
		StartAt:              "documents/start/at",
		EndBefore:            "documents/end/before",
	}

	got := qp.toQuery()

	want := Query{
		c:              testClient,
		path:           "projects/projectID/databases/(default)",
		parentPath:     "projects/projectID/databases/(default)/documents",
		collectionID:   "collectionID",
		startVals:      []interface{}{"documents/start/at"},
		endVals:        []interface{}{"documents/end/before"},
		startBefore:    true,
		endBefore:      true,
		allDescendants: true,
		orders:         []order{{fieldPath: []string{"__name__"}, dir: 1}},
	}

	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCGR_TestGetPartitions(t *testing.T) {
	cgr := newCollectionGroupRef(testClient, testClient.path(), "collectionID")
	_, err := cgr.getPartitions(context.Background(), 0)
	if err == nil {
		t.Error("Expected an error when requested partition count is < 1")
	}

	parts, err := cgr.getPartitions(context.Background(), 1)
	if err != nil {
		t.Error("Didn't expect an error when requested partition count is 1")
	}
	if len(parts) != 1 {
		t.Fatal("Expected 1 queryPartition")
	}
	got := parts[0]
	want := queryPartition{
		CollectionGroupQuery: cgr.Query.OrderBy(DocumentID, Asc),
		StartAt:              "",
		EndBefore:            "",
	}
	if !testEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
