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

package reader

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	gax "github.com/googleapis/gax-go/v2"
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc"
)

// ReadSession is the abstraction over a storage API read session.
type ReadSession struct {
	settings *settings
	c        *Client

	ctx     context.Context
	job     *bigquery.Job
	table   *bigquery.Table
	tableID string

	bqSession *bqStoragepb.ReadSession

	// EstimatedTotalBytesScanned on the number of bytes this session will scan when
	// all streams are completely consumed. This estimate is based on
	// metadata from the table which might be incomplete or stale.
	EstimatedTotalBytesScanned int64

	// StreamCount represents the number of streams opened to for this session.
	// Available after session is initialized.
	StreamCount int

	// StreamNames represents the number of streams opened to for this session.
	// Available after session is initialized.
	StreamNames []string

	// SessionID is a unique identifier for the session, in the form
	// projects/{project_id}/locations/{location}/sessions/{session_id}.
	// Available after session is initialized.
	SessionID string

	// ExpireTime at which the session becomes invalid. After this time, subsequent
	// requests to read this Session will return errors.
	ExpireTime *time.Time
}

type settings struct {
	// MaxStreamCount governs how many parallel streams
	// can be opened.
	MaxStreamCount int
}

func defaultSettings() *settings {
	return &settings{
		MaxStreamCount: 0,
	}
}

func (rs *ReadSession) readRows(ctx context.Context, req *storagepb.ReadRowsRequest, opts ...gax.CallOption) (storagepb.BigQueryRead_ReadRowsClient, error) {
	return rs.c.readRows(ctx, req, opts...)
}

// Run initiates a read session
func (rs *ReadSession) Run() error {
	tableReadOptions := &bqStoragepb.ReadSession_TableReadOptions{
		SelectedFields: []string{},
	}
	maxStreamCount := rs.settings.MaxStreamCount
	createReadSessionRequest := &bqStoragepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", rs.table.ProjectID),
		ReadSession: &bqStoragepb.ReadSession{
			Table:       rs.tableID,
			DataFormat:  bqStoragepb.DataFormat_ARROW,
			ReadOptions: tableReadOptions,
		},
		MaxStreamCount: int32(maxStreamCount),
	}
	rpcOpts := gax.WithGRPCOptions(
		grpc.MaxCallRecvMsgSize(1024 * 1024 * 129), // TODO: why needs to be of this size
	)
	session, err := rs.c.createReadSession(rs.ctx, createReadSessionRequest, rpcOpts)
	if err != nil {
		return err
	}

	rs.bqSession = session

	rs.SessionID = session.Name
	rs.StreamNames = []string{}
	streams := session.GetStreams()
	for _, stream := range streams {
		rs.StreamNames = append(rs.StreamNames, stream.Name)
	}
	rs.StreamCount = len(streams)
	if session.ExpireTime != nil {
		t := session.ExpireTime.AsTime()
		rs.ExpireTime = &t
	}
	rs.EstimatedTotalBytesScanned = session.EstimatedTotalBytesScanned
	return nil
}

// Read initiates a read session (if not ran before)
// and returns the results via a RowIterator.
func (rs *ReadSession) Read() (*RowIterator, error) {
	if rs.bqSession == nil {
		err := rs.Run()
		if err != nil {
			return nil, err
		}
	}
	if rs.job != nil {
		return newJobRowIterator(rs.ctx, rs, rs.job)
	}
	return newTableRowIterator(rs.ctx, rs, rs.table)
}

// ReadArrow initiates a read session (if not started before)
// and returns the results via an ArrowIterator.
func (rs *ReadSession) ReadArrow() (*ArrowIterator, error) {
	if rs.bqSession == nil {
		err := rs.Run()
		if err != nil {
			return nil, err
		}
	}
	if rs.job != nil {
		return newRawJobRowIterator(rs.ctx, rs, rs.job)
	}
	return newRawTableRowIterator(rs.ctx, rs, rs.table)
}
