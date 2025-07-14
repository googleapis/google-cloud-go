// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package query

import (
	"context"
	"fmt"

	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
)

// storageReader is used to read the results of a query using Storage Read API.
type storageReader struct {
	c            *Client
	rc           *storage.BigQueryReadClient
	q            *Query
	table        *bigquerypb.TableReference
	rs           *storagepb.ReadSession
	it           *storageArrowIterator
	arrowDecoder *arrowDecoder
}

func newStorageReader(ctx context.Context, c *Client, rc *storage.BigQueryReadClient, q *Query) (*storageReader, error) {
	table, err := resolveDestinationTable(ctx, q)
	if err != nil {
		return nil, err
	}
	if q.cachedSchema == nil {
		t, err := c.c.GetTable(ctx, &bigquerypb.GetTableRequest{
			ProjectId: table.ProjectId,
			DatasetId: table.DatasetId,
			TableId:   table.TableId,
		})
		if err != nil {
			return nil, err
		}
		q.cachedSchema = newSchema(t.Schema)
	}

	rs := &storageReader{
		c:     c,
		rc:    rc,
		q:     q,
		table: table,
	}

	it, err := newRawStorageRowIterator(ctx, rs, q.cachedSchema)
	if err != nil {
		return nil, err
	}
	rs.it = it

	return rs, nil
}

func resolveDestinationTable(ctx context.Context, q *Query) (*bigquerypb.TableReference, error) {
	// Needed to fetch destination table
	job, err := q.c.c.GetJob(ctx, &bigquerypb.GetJobRequest{
		ProjectId: q.projectID,
		JobId:     q.jobID,
		Location:  q.location,
	})
	if err != nil {
		return nil, err
	}
	cfg := job.Configuration
	if cfg == nil {
		return nil, fmt.Errorf("job has no configuration")
	}
	qcfg := cfg.Query
	if qcfg == nil {
		return nil, fmt.Errorf("job is not a query")
	}
	if qcfg.DestinationTable == nil {
		// TODO: handle scripts
		return nil, fmt.Errorf("job has no destination table to read")
	}
	return qcfg.DestinationTable, nil
}

func (r *storageReader) start(ctx context.Context, state *readState) (*RowIterator, error) {
	it := &RowIterator{
		r:         r,
		totalRows: r.q.cachedTotalRows,
	}
	rs, err := r.sessionForTable(ctx)
	if err != nil {
		return nil, err
	}
	r.rs = rs
	it.totalRows = uint64(r.rs.EstimatedRowCount)

	dec, err := newArrowDecoder(rs.GetArrowSchema().SerializedSchema, r.q.cachedSchema)
	if err != nil {
		return nil, err
	}
	r.arrowDecoder = dec

	return it, nil
}

func (r *storageReader) sessionForTable(ctx context.Context) (*storagepb.ReadSession, error) {
	tableID := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", r.table.ProjectId, r.table.DatasetId, r.table.TableId)

	req := &storagepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", r.c.billingProjectID),
		ReadSession: &storagepb.ReadSession{
			Table:      tableID,
			DataFormat: storagepb.DataFormat_ARROW,
		},
		// TODO: have centralized settings for stream count
		MaxStreamCount:          0,
		PreferredMinStreamCount: 1,
	}
	return r.rc.CreateReadSession(ctx, req)
}

func (r *storageReader) nextPage(ctx context.Context, pageToken string) (*resultSet, error) {
	batch, err := r.it.next()
	if err != nil {
		return nil, err
	}
	rows, err := r.arrowDecoder.decodeArrowRecords(batch)
	if err != nil {
		return nil, err
	}
	rs := &resultSet{
		rows:      rows,
		pageToken: "",
	}
	return rs, nil
}
