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
	"time"

	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/iterator"
)

type RowAccessPolicy struct {

	// The ID of the table containing this row access policy.
	TableID string

	// The ID of the dataset containing this row access policy.
	DatasetID string

	// The ID of this row access policy.
	PolicyID string

	// The ID of the project containing this row access policy.
	ProjectID string

	c *Client
}

func (rap *RowAccessPolicy) toBQ() *bq.RowAccessPolicyReference {
	if rap == nil {
		return nil
	}
	return &bq.RowAccessPolicyReference{
		TableId:   rap.TableID,
		DatasetId: rap.DatasetID,
		PolicyId:  rap.PolicyID,
		ProjectId: rap.ProjectID,
	}
}

func bqToRowAccessPolicy(r *bq.RowAccessPolicyReference, c *Client) *RowAccessPolicy {
	if r == nil {
		return nil
	}
	return &RowAccessPolicy{
		TableID:   r.TableId,
		DatasetID: r.DatasetId,
		PolicyID:  r.PolicyId,
		ProjectID: r.ProjectId,
		c:         c,
	}
}

func (rap *RowAccessPolicy) Metadata(ctx context.Context) (*RowAccessPolicyMetadata, error) {
	//BROKEN API: We can't do this without a Get method for individual row access policies in the backend
	return nil, fmt.Errorf("backend does not support fetching metadata for individual row access policies")
}

// RowAccessPolicyMetadata represents access on a subset of rows on a specified table.
// Access is defined by its filter predicate, and access it controlled via the
// associated IAM policy.
type RowAccessPolicyMetadata struct {
	// The time when this access policy was created.
	CreationTime time.Time

	// The time when this access policy was last modified.
	ModifiedTime time.Time

	// ETag is the ETag obtained when reading the policy metadata.
	ETag string

	// FilterPredicate is a SQL boolean expression that represents rows defined by
	// this row access policy.  Similar to a boolean expression in a WHERE clause of a
	// SELECT query on a table.  References to other resources (tables, routines, temporary
	// functions) are not supported.
	FilterPredicate string
}

// A RowAccessPolicyIterator is an iterator over Row Access Policies.
type RowAccessPolicyIterator struct {
	ctx      context.Context
	table    *Table
	policies []*RowAccessPolicy
	pageInfo *iterator.PageInfo
	nextFunc func() error
}

// Next returns the next result. Its second return value is Done if there are
// no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *RowAccessPolicyIterator) Next() (*RowAccessPolicy, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	p := it.policies[0]
	it.policies = it.policies[1:]
	return p, nil
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *RowAccessPolicyIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

// listPolicies exists to aid testing.
var listPolicies = func(it *RowAccessPolicyIterator, pageSize int, pageToken string) (*bq.ListRowAccessPoliciesResponse, error) {
	call := it.table.c.bqs.RowAccessPolicies.List(it.table.ProjectID, it.table.DatasetID, it.table.TableID).
		PageToken(pageToken).
		Context(it.ctx)
	setClientHeader(call.Header())
	if pageSize > 0 {
		call.PageSize(int64(pageSize))
	}
	var res *bq.ListRowAccessPoliciesResponse
	err := runWithRetry(it.ctx, func() (err error) {
		res, err = call.Do()
		return err
	})
	return res, err
}

func (it *RowAccessPolicyIterator) fetch(pageSize int, pageToken string) (string, error) {
	res, err := listPolicies(it, pageSize, pageToken)
	if err != nil {
		return "", err
	}
	for _, t := range res.RowAccessPolicies {
		it.policies = append(it.policies, bqToRowAccessPolicy(t.RowAccessPolicyReference, it.table.c))
	}
	return res.NextPageToken, nil
}

func (t *Table) RowAccessPolicies(ctx context.Context) *RowAccessPolicyIterator {
	it := &RowAccessPolicyIterator{
		ctx:   ctx,
		table: t,
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.policies) },
		func() interface{} { b := it.policies; it.policies = nil; return b })
	return it
}
