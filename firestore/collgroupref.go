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
	"fmt"

	firestore "cloud.google.com/go/firestore/apiv1"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
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

// PartitionQuery partitions a query by returning partition cursors that can be used to run
// the query in parallel. The returned partition cursors are split points that
// can be used by RunQuery as starting/end points for the query results.
// Partition queries require explicit ordering by __name__ and
// select all descendant collections.
//
// count is the desired maximum number of partition points. The number must be
// strictly positive.
func (c *CollectionGroupRef) PartitionQuery(ctx context.Context, count int) (Query, []Query, error) {
	client, err := firestore.NewClient(ctx)
	if err != nil {
		return Query{}, nil, fmt.Errorf("firestore.NewClient: %v", err)
	}
	defer client.Close()
	// Partition queries require explicit ordering by __name__ and
	// select all descendant collections.
	q := c.OrderBy(DocumentID, Asc)
	q.allDescendants = true
	strQuery, err := q.toProto()
	if err != nil {
		return Query{}, nil, err
	}
	req := &pb.PartitionQueryRequest{
		Parent:         c.parentPath,
		PartitionCount: int64(count),
		QueryType: &pb.PartitionQueryRequest_StructuredQuery{
			StructuredQuery: strQuery,
		},
	}
	var partitions []Query
	var docs []string
	it := client.PartitionQuery(ctx, req)
	for {
		cursor, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return q, nil, fmt.Errorf("PartitionQuery: %v", err)
		}
		doc := cursor.Values[0].GetReferenceValue()
		docs = append(docs, doc)
	}
	for i, doc := range docs {
		partition := q
		if i == 0 {
			partitions = append(partitions, partition.EndBefore(doc))
		} else if i == len(docs)-1 {
			partitions = append(partitions, partition.StartAt(doc))
		} else {
			partitions = append(partitions, partition.StartAt(doc).EndBefore(docs[i+1]))
		}
	}
	return q, partitions, nil
}
