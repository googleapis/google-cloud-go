// Copyright 2022 Google LLC
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

package managedreader

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"

	"cloud.google.com/go/bigquery"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"

	bqStorage "cloud.google.com/go/bigquery/storage/apiv1"
	"google.golang.org/grpc"
)

// RowIterator interface for storage api read
type RowIterator interface {
	Next(interface{}) error
	TotalRows() uint64
}

// Upgrade row iterator to use Storage API
func Upgrade(ctx context.Context, storageClient *bqStorage.BigQueryReadClient, q *bigquery.Query) (RowIterator, error) {
	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	rowIt, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}
	it := &streamIterator{
		ctx:       ctx,
		job:       job,
		totalRows: rowIt.TotalRows,
		storage:   storageClient,
		rows:      make(chan []bigquery.Value, 0),
		errs:      make(chan error, 0),
	}
	return it, err
}

type streamIterator struct {
	done bool
	errs chan error
	wg   sync.WaitGroup

	ctx     context.Context
	storage *bqStorage.BigQueryReadClient

	job     *bigquery.Job
	table   *bigquery.Table
	tableID string
	schema  bigquery.Schema

	parser *arrowParser

	rows      chan []bigquery.Value
	totalRows uint64
}

func (it *streamIterator) init() error {
	if it.parser == nil { // Not initialized
		err := it.setup()
		if err != nil {
			return err
		}
		err = it.start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (it *streamIterator) setup() error {
	table := it.table
	if it.job != nil {
		cfg, err := it.job.Config()
		if err != nil {
			return err
		}
		qcfg := cfg.(*bigquery.QueryConfig)
		if qcfg.Dst == nil {
			// TODO: script job ?
			return fmt.Errorf("nil job destination table")
		}
		table = qcfg.Dst
	}
	tableID, err := table.Identifier(bigquery.StorageAPIResourceID)
	if err != nil {
		return err
	}
	it.table = table
	it.tableID = tableID
	return nil
}

func (it *streamIterator) start() error {
	tableReadOptions := &bqStoragepb.ReadSession_TableReadOptions{
		SelectedFields: []string{},
	}
	createReadSessionRequest := &bqStoragepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", it.table.ProjectID),
		ReadSession: &bqStoragepb.ReadSession{
			Table:       it.tableID,
			DataFormat:  bqStoragepb.DataFormat_ARROW,
			ReadOptions: tableReadOptions,
		},
		MaxStreamCount: int32(runtime.GOMAXPROCS(0)), // TODO: control when to open multiple streams
	}
	rpcOpts := gax.WithGRPCOptions(
		grpc.MaxCallRecvMsgSize(1024 * 1024 * 129), // TODO: why needs to be of this size
	)
	session, err := it.storage.CreateReadSession(it.ctx, createReadSessionRequest, rpcOpts)
	if err != nil {
		return err
	}

	if len(session.GetStreams()) == 0 {
		it.errs <- iterator.Done
		return nil
	}

	meta, err := it.table.Metadata(it.ctx)
	if err != nil {
		return err
	}
	it.schema = meta.Schema

	parser, err := newArrowParserFromSession(session, it.schema)
	if err != nil {
		return err
	}
	it.parser = parser

	go func() {
		it.wg.Wait()
		close(it.rows)
		it.done = true
	}()

	for _, readStream := range session.GetStreams() {
		it.wg.Add(1)
		go it.processStream(readStream)
	}
	return nil
}

func (it *streamIterator) processStream(stream *bqStoragepb.ReadStream) {
	var offset int64
	for {
		rowStream, err := it.storage.ReadRows(it.ctx, &bqStoragepb.ReadRowsRequest{
			ReadStream: stream.Name,
			Offset:     offset,
		})
		if err != nil {
			it.errs <- err
		}
		for {
			r, err := rowStream.Recv()
			if err == io.EOF {
				it.wg.Done()
				return
			}
			if err != nil {
				it.errs <- err
			}
			rc := r.GetRowCount()
			if rc > 0 {
				offset += rc
				arrowRecords := r.GetArrowRecordBatch()
				rows, err := it.parser.convertArrowRows(arrowRecords)
				if err != nil {
					it.errs <- err
				}
				for _, row := range rows {
					it.rows <- row
				}
			}
		}
	}
}

func (it *streamIterator) TotalRows() uint64 {
	return it.totalRows
}

func (it *streamIterator) Next(dst interface{}) error {
	if err := it.init(); err != nil {
		return err
	}
	vl, err := bigquery.ResolveValueLoader(dst, it.schema)
	if err != nil {
		return err
	}
	if it.done {
		return iterator.Done
	}
	select {
	case row := <-it.rows:
		if len(row) == 0 {
			return iterator.Done
		}
		return vl.Load(row, it.schema)
	case err := <-it.errs:
		return err
	case <-it.ctx.Done():
		return it.ctx.Err()
	}
}
