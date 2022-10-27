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

package reader

import (
	"context"
	"fmt"
	"io"
	"sync"

	"cloud.google.com/go/bigquery"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"

	"google.golang.org/grpc"
)

// RowIterator interface for Storage Read API
type RowIterator interface {
	// Next loads the next row into dst. Its return value is iterator.Done if there
	// are no more results. Once Next returns iterator.Done, all subsequent calls
	// will return iterator.Done.
	// See more on the core package bigquery.RowIterator Next method.
	Next(interface{}) error

	// Total rows on the result set.
	// Available after the first call to Next.
	TotalRows() uint64
}

// ArrowIterator is a raw interface for getting data from Storage Read API
type ArrowIterator interface {
	// The Arrow schema of the given result set.
	// Available after the first call to Next.
	Schema() *arrow.Schema

	// Consumes all the iterator by calling Next and
	// building an arrow table mixing all records and schema.
	// Sequential calls will fail.
	// Accessing Arrow Table directly has the drawback of having to deal
	// with memory management.
	// Make sure to call Release() after using it.
	Table() (arrow.Table, error)

	// Return the next batch of rows as an arrow.Record.
	// Accessing Arrow Records directly has the drawnback of having to deal
	// with memory management.
	// Make sure to call Release() after using it.
	Next() (arrow.Record, error)

	// Total rows on the result set.
	// Available after the first call to Next.
	TotalRows() uint64
}

func newRawQueryRowIterator(ctx context.Context, r *Reader, q *bigquery.Query) (ArrowIterator, error) {
	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return newRawJobRowIterator(ctx, r, job)
}

func newQueryRowIterator(ctx context.Context, r *Reader, q *bigquery.Query) (RowIterator, error) {
	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return newJobRowIterator(ctx, r, job)
}

func newRawJobRowIterator(ctx context.Context, r *Reader, job *bigquery.Job) (ArrowIterator, error) {
	rowIt, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}
	arrowIt := &arrowIterator{
		ctx:       ctx,
		job:       job,
		r:         r,
		totalRows: rowIt.TotalRows,
		records:   make(chan arrow.Record, 0),
		errs:      make(chan error, 0),
	}
	return arrowIt, nil
}

func newJobRowIterator(ctx context.Context, r *Reader, job *bigquery.Job) (RowIterator, error) {
	arrowIt, err := newRawJobRowIterator(ctx, r, job)
	if err != nil {
		return nil, err
	}
	it := &streamIterator{
		ctx:     ctx,
		arrowIt: arrowIt.(*arrowIterator),
		rows:    [][]bigquery.Value{},
	}
	return it, nil
}

func newRawTableRowIterator(ctx context.Context, r *Reader, table *bigquery.Table) (ArrowIterator, error) {
	rowIt := table.Read(ctx)
	arrowIt := &arrowIterator{
		ctx:       ctx,
		table:     table,
		r:         r,
		totalRows: rowIt.TotalRows,
		records:   make(chan arrow.Record, 0),
		errs:      make(chan error, 0),
	}
	return arrowIt, nil
}

func newTableRowIterator(ctx context.Context, r *Reader, table *bigquery.Table) (RowIterator, error) {
	arrowIt, err := newRawTableRowIterator(ctx, r, table)
	if err != nil {
		return nil, err
	}
	it := &streamIterator{
		ctx:     ctx,
		arrowIt: arrowIt.(*arrowIterator),
		rows:    [][]bigquery.Value{},
	}
	return it, nil
}

type arrowIterator struct {
	done bool
	errs chan error
	wg   sync.WaitGroup

	ctx context.Context
	r   *Reader

	job     *bigquery.Job
	table   *bigquery.Table
	tableID string
	schema  bigquery.Schema

	parser *arrowParser

	records   chan arrow.Record
	totalRows uint64

	streamCount  int
	bytesScanned int64
}

func (it *arrowIterator) init() error {
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

func (it *arrowIterator) setup() error {
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

func (it *arrowIterator) start() error {
	tableReadOptions := &bqStoragepb.ReadSession_TableReadOptions{
		SelectedFields: []string{},
	}
	maxStreamCount := it.r.settings.MaxStreamCount
	createReadSessionRequest := &bqStoragepb.CreateReadSessionRequest{
		Parent: fmt.Sprintf("projects/%s", it.table.ProjectID),
		ReadSession: &bqStoragepb.ReadSession{
			Table:       it.tableID,
			DataFormat:  bqStoragepb.DataFormat_ARROW,
			ReadOptions: tableReadOptions,
		},
		MaxStreamCount: int32(maxStreamCount),
	}
	rpcOpts := gax.WithGRPCOptions(
		grpc.MaxCallRecvMsgSize(1024 * 1024 * 129), // TODO: why needs to be of this size
	)
	session, err := it.r.c.createReadSession(it.ctx, createReadSessionRequest, rpcOpts)
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
		close(it.records)
		it.done = true
	}()

	streams := session.GetStreams()
	it.streamCount = len(streams)
	it.bytesScanned = session.EstimatedTotalBytesScanned
	for _, readStream := range streams {
		it.wg.Add(1)
		go it.processStream(readStream)
	}
	return nil
}

func (it *arrowIterator) processStream(stream *bqStoragepb.ReadStream) {
	var offset int64
	for {
		rowStream, err := it.r.c.readRows(it.ctx, &bqStoragepb.ReadRowsRequest{
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
				recordBatch := r.GetArrowRecordBatch()
				records, err := it.parser.parseArrowRecords(recordBatch)
				if err != nil {
					it.errs <- err
				}
				for _, record := range records {
					it.records <- record
				}
			}
		}
	}
}

func (it *arrowIterator) Schema() *arrow.Schema {
	return it.parser.arrowSchema
}

func (it *arrowIterator) Next() (arrow.Record, error) {
	if err := it.init(); err != nil {
		return nil, err
	}
	if it.done {
		return nil, iterator.Done
	}
	select {
	case record := <-it.records:
		if record == nil {
			return nil, iterator.Done
		}
		return record, nil
	case err := <-it.errs:
		return nil, err
	case <-it.ctx.Done():
		return nil, it.ctx.Err()
	}
}

func (it *arrowIterator) Table() (arrow.Table, error) {
	records := []arrow.Record{}
	for {
		record, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return array.NewTableFromRecords(it.parser.arrowSchema, records), nil
}

func (it *arrowIterator) TotalRows() uint64 {
	return it.totalRows
}

type streamIterator struct {
	ctx     context.Context
	arrowIt *arrowIterator

	rows [][]bigquery.Value
}

func (it *streamIterator) init() error {
	if err := it.arrowIt.init(); err != nil {
		return err
	}
	return nil
}

func (it *streamIterator) processRecord(record arrow.Record) error {
	rows, err := it.arrowIt.parser.convertArrowRecordValue(record)
	if err != nil {
		return err
	}
	for _, row := range rows {
		it.rows = append(it.rows, row)
	}
	return nil
}

func (it *streamIterator) TotalRows() uint64 {
	return it.arrowIt.TotalRows()
}

func (it *streamIterator) Next(dst interface{}) error {
	if err := it.init(); err != nil {
		return err
	}

	vl, err := bigquery.ResolveValueLoader(dst, it.arrowIt.schema)
	if err != nil {
		return err
	}

	if len(it.rows) > 0 {
		row := it.rows[0]
		it.rows = it.rows[1:]
		return vl.Load(row, it.arrowIt.schema)
	}

	record, err := it.arrowIt.Next()
	if err != nil {
		return err
	}
	defer record.Release()

	err = it.processRecord(record)
	if err != nil {
		return err
	}

	if len(it.rows) == 0 {
		return iterator.Done
	}

	row := it.rows[0]
	it.rows = it.rows[1:]
	return vl.Load(row, it.arrowIt.schema)
}
