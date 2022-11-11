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
	"io"
	"time"

	"cloud.google.com/go/bigquery"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
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

	// SerializedArrowSchema is an IPC-serialized Arrow Schema.
	SerializedArrowSchema []byte

	// EstimatedTotalBytesScanned on the number of bytes this session will scan when
	// all streams are completely consumed. This estimate is based on
	// metadata from the table which might be incomplete or stale.
	EstimatedTotalBytesScanned int64

	// StreamCount represents the number of streams opened to for this session.
	// Available after session is initialized.
	StreamCount int

	// ReadStreams contains at least one stream that is created with
	// given the session, in the form
	// projects/{project_id}/locations/{location}/sessions/{session_id}/streams/{stream_id}.
	// Available after session is initialized.
	ReadStreams []string

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
	rs.ReadStreams = []string{}
	streams := session.GetStreams()
	for _, stream := range streams {
		rs.ReadStreams = append(rs.ReadStreams, stream.Name)
	}
	rs.StreamCount = len(streams)
	if session.ExpireTime != nil {
		t := session.ExpireTime.AsTime()
		rs.ExpireTime = &t
	}
	rs.EstimatedTotalBytesScanned = session.EstimatedTotalBytesScanned
	arrowSchema := session.GetArrowSchema()
	if arrowSchema != nil {
		rs.SerializedArrowSchema = arrowSchema.GetSerializedSchema()
	}
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

// ReadRowsRequest message for ReadRows
type ReadRowsRequest struct {
	// Required. Stream to read rows from.
	ReadStream string
	// The offset requested must be less than the last row read from Read.
	// Requesting a larger offset is undefined. If not specified, start reading
	// from offset zero.
	Offset int64
}

// ReadRows returns a more direct iterators to the underlying Storage API row stream.
func (rs *ReadSession) ReadRows(req ReadRowsRequest) (*RowStreamIterator, error) {
	if rs.bqSession == nil {
		err := rs.Run()
		if err != nil {
			return nil, err
		}
	}
	readRowClient, err := rs.c.readRows(rs.ctx, &storagepb.ReadRowsRequest{
		ReadStream: req.ReadStream,
		Offset:     req.Offset,
	})
	if err != nil {
		return nil, err
	}
	it := &RowStreamIterator{
		readStream:    req.ReadStream,
		readRowClient: readRowClient,
	}
	return it, nil
}

// RowStreamIterator represents an iterator for Storage API row stream.
type RowStreamIterator struct {
	readRowClient storagepb.BigQueryRead_ReadRowsClient
	readStream    string
}

// RowStream include row data on a given ReadSession stream
type RowStream struct {
	// SourceStream is the name of the stream, in the form
	// projects/{project_id}/locations/{location}/sessions/{session_id}/streams/{stream_id}.
	SourceStream string

	// RowCount represents the number of serialized rows in the rows block.
	RowCount int64

	// SerializedArrowRecordBatch is an IPC-serialized Arrow RecordBatch.
	SerializedArrowRecordBatch []byte
}

// Next returns next row on the given RowStream.
// Its return value is iterator.Done if there
// are no more results. Once Next returns iterator.Done, all subsequent calls
// will return iterator.Done.
func (rs *RowStreamIterator) Next() (*RowStream, error) {
	r, err := rs.readRowClient.Recv()
	if err == io.EOF {
		return nil, iterator.Done
	}
	rsRes := &RowStream{
		SourceStream:               rs.readStream,
		RowCount:                   r.GetRowCount(),
		SerializedArrowRecordBatch: r.GetArrowRecordBatch().SerializedRecordBatch,
	}
	return rsRes, nil
}
