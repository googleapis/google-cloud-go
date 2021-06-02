// Copyright 2019 Google LLC
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
	"errors"
	"sort"

	"google.golang.org/api/iterator"
	firestorepb "google.golang.org/genproto/googleapis/firestore/v1"
)

// A CollectionGroupRef is a reference to a group of collections sharing the
// same ID.
type CollectionGroupRef struct {
	c *Client

	// Use the methods of Query on a CollectionGroupRef to create and run queries.
	Query
}

func newCollectionGroupRef(c *Client, dbPath, collectionID string) *CollectionGroupRef {
	return &CollectionGroupRef{
		c: c,

		Query: Query{
			c:              c,
			collectionID:   collectionID,
			path:           dbPath,
			parentPath:     dbPath + "/documents",
			allDescendants: true,
		},
	}
}

// GetPartitions returns a slice of QueryPartition objects. The number must be
// positive. The actual number of partitions returned may be fewer.
func (cgr CollectionGroupRef) GetPartitions(ctx context.Context, partitionCount int64) ([]QueryPartition, error) {
	if partitionCount <= 0 {
		return nil, errors.New("a positive partitionCount must be provided")
	}

	db := cgr.c.path()
	ctx = withResourceHeader(ctx, db)

	// CollectiongGroup Queries need to be ordered by __name__ ASC
	query, err := cgr.query().OrderBy(DocumentID, Asc).toProto()
	if err != nil {
		return nil, err
	}
	structuredQuery := &firestorepb.PartitionQueryRequest_StructuredQuery{
		StructuredQuery: query,
	}

	// Uses default PageSize
	pbr := &firestorepb.PartitionQueryRequest{
		Parent:         db + "/documents",
		PartitionCount: partitionCount,
		QueryType:      structuredQuery,
	}
	cursorReferences := make([]*firestorepb.Value, 0, partitionCount)
	iter := cgr.c.c.PartitionQuery(ctx, pbr)
	for {
		cursor, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		cursorReferences = append(cursorReferences, cursor.Values...)
	}

	// From Proto documentation:
	// To obtain a complete result set ordered with respect to the results of the
	// query supplied to PartitionQuery, the results sets should be merged:
	// cursor A, cursor B, cursor M, cursor Q, cursor U, cursor W
	// Once we have exhausted the pages, the cursor values need to be sorted in
	// lexicographical order by segment (areas between '/').
	sort.Sort(byReferenceValue(cursorReferences))

	partitionQueries := make([]QueryPartition, 0, len(cursorReferences))
	var previousCursor string = ""

	for _, cursor := range cursorReferences {
		cursorRef := cursor.GetReferenceValue()
		qp := QueryPartition{
			StartAt:   previousCursor,
			EndBefore: cursorRef,
		}
		partitionQueries = append(partitionQueries, qp)
		previousCursor = cursorRef
	}

	// In the case there were no partitions, we still add a single partition to
	// the result, that covers the complete range.
	lastPart := QueryPartition{
		StartAt:   "",
		EndBefore: "",
	}
	if len(cursorReferences) > 0 {
		lastPart.StartAt = cursorReferences[len(cursorReferences)-1].GetReferenceValue()
	}
	partitionQueries = append(partitionQueries, lastPart)

	return partitionQueries, nil
}

// byReferenceValue implements sort.Interface for []*firestorepb.Value based on
// the Age field.
type byReferenceValue []*firestorepb.Value

func (a byReferenceValue) Len() int           { return len(a) }
func (a byReferenceValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byReferenceValue) Less(i, j int) bool { return compareValues(a[i], a[j]) < 0 }

// QueryPartition provides a Collection Group Reference and start and end split
// points. This is used by GetPartition which, given a CollectionGroupReference
// returns smaller sub-queries or partitions
type QueryPartition struct {
	CollectionGroupRef *CollectionGroupRef
	StartAt            string
	EndBefore          string
}

// ToQuery converts a QueryPartition object to a Query object
func (qp QueryPartition) ToQuery() Query {
	// TODO(crwilcox) exposing the collection group reference here,
	//but just as well could pass the collection id, and then compose a ref.
	q := qp.CollectionGroupRef.query().OrderBy(DocumentID, Asc)
	if qp.StartAt != "" {
		q = q.StartAt(qp.StartAt)
	}
	if qp.EndBefore != "" {
		q = q.EndBefore(qp.EndBefore)
	}
	return q
}
