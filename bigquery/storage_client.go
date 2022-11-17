// Copyright 2022 Google LLC
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

package bigquery

import (
	"context"
	"fmt"
	"runtime"

	"cloud.google.com/go/bigquery/internal"
	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/internal/detect"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
)

// ReadClient is a managed BigQuery Storage read client scoped to a single project.
type ReadClient struct {
	rawClient *storage.BigQueryReadClient
	projectID string
}

// NewReadClient instantiates a new storage read client.
func NewReadClient(ctx context.Context, projectID string, opts ...option.ClientOption) (c *ReadClient, err error) {
	numConns := runtime.GOMAXPROCS(0)
	if numConns > 4 {
		numConns = 4
	}
	o := []option.ClientOption{
		option.WithGRPCConnectionPool(numConns),
	}
	o = append(o, opts...)

	rawClient, err := storage.NewBigQueryReadClient(ctx, o...)
	if err != nil {
		return nil, err
	}
	rawClient.SetGoogleClientInfo("gccl", internal.Version)

	// Handle project autodetection.
	projectID, err = detect.ProjectID(ctx, projectID, "", opts...)
	if err != nil {
		return nil, err
	}

	return &ReadClient{
		rawClient: rawClient,
		projectID: projectID,
	}, nil
}

// Close releases resources held by the client.
func (c *ReadClient) Close() error {
	// TODO: consider if we should propagate a cancellation from client to all associated managed streams.
	if c.rawClient == nil {
		return fmt.Errorf("already closed")
	}
	c.rawClient.Close()
	c.rawClient = nil
	return nil
}

// SessionForQuery establishes a new session to fetch from a query using the Storage API
func (c *ReadClient) SessionForQuery(ctx context.Context, query *Query, opts ...StorageReadOption) (*ReadSession, error) {
	job, err := query.Run(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := c.buildJobSession(ctx, job, opts...)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

// SessionForTable establishes a new session to fetch from a table using the Storage API
func (c *ReadClient) SessionForTable(ctx context.Context, table *Table, opts ...StorageReadOption) (*ReadSession, error) {
	return c.buildTableSession(ctx, table, opts...)
}

// SessionForJob establishes a new session to fetch from a bigquery Job using the Storage API
func (c *ReadClient) SessionForJob(ctx context.Context, job *Job, opts ...StorageReadOption) (*ReadSession, error) {
	return c.buildJobSession(ctx, job, opts...)
}

func (c *ReadClient) buildJobSession(ctx context.Context, job *Job, opts ...StorageReadOption) (*ReadSession, error) {
	cfg, err := job.Config()
	if err != nil {
		return nil, err
	}
	qcfg := cfg.(*QueryConfig)
	if qcfg.Dst == nil {
		// TODO: script job ?
		return nil, fmt.Errorf("nil job destination table")
	}
	return c.buildTableSession(ctx, qcfg.Dst, opts...)
}

func (c *ReadClient) buildTableSession(ctx context.Context, table *Table, opts ...StorageReadOption) (*ReadSession, error) {
	tableID, err := table.Identifier(StorageAPIResourceID)
	if err != nil {
		return nil, err
	}

	r := &ReadSession{
		rc:       c,
		ctx:      ctx,
		table:    table,
		tableID:  tableID,
		settings: defaultSettings(),
	}

	// apply read options
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (c *ReadClient) createReadSession(ctx context.Context, req *storagepb.CreateReadSessionRequest, opts ...gax.CallOption) (*storagepb.ReadSession, error) {
	return c.rawClient.CreateReadSession(ctx, req, opts...)
}

func (c *ReadClient) readRows(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
	return c.rawClient.ReadRows(ctx, req, opts...)
}
