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
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc"
)

// readClient is a managed BigQuery Storage read client scoped to a single project.
type readClient struct {
	rawClient *storage.BigQueryReadClient
	projectID string

	maxStreamCount int
	maxWorkerCount int
}

// newReadClient instantiates a new storage read client.
func newReadClient(ctx context.Context, projectID string, opts ...option.ClientOption) (c *readClient, err error) {
	numConns := runtime.GOMAXPROCS(0)
	if numConns > 4 {
		numConns = 4
	}
	o := []option.ClientOption{
		option.WithGRPCConnectionPool(numConns),
		option.WithUserAgent(fmt.Sprintf("%s/%s", userAgentPrefix, internal.Version)),
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

	maxWorkerCount := runtime.GOMAXPROCS(0)
	rc := &readClient{
		rawClient:      rawClient,
		projectID:      projectID,
		maxWorkerCount: maxWorkerCount,
		maxStreamCount: 0,
	}

	return rc, nil
}

// close releases resources held by the client.
func (c *readClient) Close() error {
	if c.rawClient == nil {
		return fmt.Errorf("already closed")
	}
	c.rawClient.Close()
	c.rawClient = nil
	return nil
}

// SessionForTable establishes a new session to fetch from a table using the Storage API
func (c *readClient) sessionForTable(ctx context.Context, table *Table) (*readSession, error) {
	tableID, err := table.Identifier(StorageAPIResourceID)
	if err != nil {
		return nil, err
	}

	rs := &readSession{
		rc:      c,
		ctx:     ctx,
		table:   table,
		tableID: tableID,
	}
	return rs, nil
}

func (c *readClient) createReadSession(ctx context.Context, req *storagepb.CreateReadSessionRequest, opts ...gax.CallOption) (*storagepb.ReadSession, error) {
	return c.rawClient.CreateReadSession(ctx, req, opts...)
}

func (c *readClient) readRows(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
	return c.rawClient.ReadRows(ctx, req, opts...)
}

// ReadSession is the abstraction over a storage API read session.
type readSession struct {
	rc *readClient

	ctx     context.Context
	table   *Table
	tableID string

	bqSession *bqStoragepb.ReadSession
}

// Start initiates a read session
func (rs *readSession) start() error {
	tableReadOptions := &bqStoragepb.ReadSession_TableReadOptions{
		SelectedFields: []string{},
	}
	createReadSessionRequest := &bqStoragepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", rs.table.ProjectID),
		ReadSession: &bqStoragepb.ReadSession{
			Table:       rs.tableID,
			DataFormat:  bqStoragepb.DataFormat_ARROW,
			ReadOptions: tableReadOptions,
		},
		MaxStreamCount: int32(rs.rc.maxStreamCount),
	}
	rpcOpts := gax.WithGRPCOptions(
		grpc.MaxCallRecvMsgSize(1024 * 1024 * 129), // TODO: why needs to be of this size
	)
	session, err := rs.rc.createReadSession(rs.ctx, createReadSessionRequest, rpcOpts)
	if err != nil {
		return err
	}

	rs.bqSession = session
	return nil
}

// readRows returns a more direct iterators to the underlying Storage API row stream.
func (rs *readSession) readRows(req *storagepb.ReadRowsRequest) (storagepb.BigQueryRead_ReadRowsClient, error) {
	if rs.bqSession == nil {
		err := rs.start()
		if err != nil {
			return nil, err
		}
	}
	return rs.rc.readRows(rs.ctx, req)
}
