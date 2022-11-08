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
	"google.golang.org/api/iterator"
	bqStoragepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
)

// ArrowIterator is a raw interface for getting data from Storage Read API
type ArrowIterator struct {
	done bool
	errs chan error
	wg   sync.WaitGroup
	ctx  context.Context

	schema  bigquery.Schema
	decoder *arrowDecoder
	records chan arrow.Record

	// Session contains information about the
	// Storage API Read Session.
	// Available after the first call to Run or Next.
	Session *ReadSession

	// Total rows on the result set.
	// Available after the first call to Next.
	TotalRows uint64
}

// RowIterator interface for Storage Read API
type RowIterator struct {
	*ArrowIterator

	rows [][]bigquery.Value
}

func newRawJobRowIterator(ctx context.Context, rs *ReadSession, job *bigquery.Job) (*ArrowIterator, error) {
	rowIt, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}
	arrowIt := &ArrowIterator{
		ctx:       ctx,
		Session:   rs,
		TotalRows: rowIt.TotalRows,
		records:   make(chan arrow.Record, 0),
		errs:      make(chan error, 0),
	}
	return arrowIt, nil
}

func newJobRowIterator(ctx context.Context, rs *ReadSession, job *bigquery.Job) (*RowIterator, error) {
	arrowIt, err := newRawJobRowIterator(ctx, rs, job)
	if err != nil {
		return nil, err
	}
	it := &RowIterator{
		ArrowIterator: arrowIt,
		rows:          [][]bigquery.Value{},
	}
	return it, nil
}

func newRawTableRowIterator(ctx context.Context, rs *ReadSession, table *bigquery.Table) (*ArrowIterator, error) {
	rowIt := table.Read(ctx)
	arrowIt := &ArrowIterator{
		ctx:       ctx,
		Session:   rs,
		TotalRows: rowIt.TotalRows,
		records:   make(chan arrow.Record, 0),
		errs:      make(chan error, 0),
	}
	return arrowIt, nil
}

func newTableRowIterator(ctx context.Context, rs *ReadSession, table *bigquery.Table) (*RowIterator, error) {
	arrowIt, err := newRawTableRowIterator(ctx, rs, table)
	if err != nil {
		return nil, err
	}
	it := &RowIterator{
		ArrowIterator: arrowIt,
		rows:          [][]bigquery.Value{},
	}
	return it, nil
}

func (it *ArrowIterator) init() error {
	if it.decoder != nil { // Already nitialized
		return nil
	}
	session := it.Session.bqSession
	if len(session.GetStreams()) == 0 {
		it.errs <- iterator.Done
		return nil
	}

	meta, err := it.Session.table.Metadata(it.ctx)
	if err != nil {
		return err
	}
	it.schema = meta.Schema

	decoder, err := newArrowDecoderFromSession(session, it.schema)
	if err != nil {
		return err
	}
	it.decoder = decoder

	go func() {
		it.wg.Wait()
		close(it.records)
		it.done = true
	}()

	streams := session.GetStreams()
	for _, readStream := range streams {
		it.wg.Add(1)
		go it.processStream(readStream)
	}
	return nil
}

func (it *ArrowIterator) processStream(stream *bqStoragepb.ReadStream) {
	var offset int64
	for {
		rowStream, err := it.Session.readRows(it.ctx, &bqStoragepb.ReadRowsRequest{
			ReadStream: stream.Name,
			Offset:     offset,
		})
		if err != nil {
			it.errs <- fmt.Errorf("failed to read rows on stream %s: %v", stream.Name, err)
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
				records, err := it.decoder.decodeArrowRecords(recordBatch)
				if err != nil {
					it.errs <- fmt.Errorf("failed to decode arrow record on stream %s: %v", stream.Name, err)
				}
				for _, record := range records {
					it.records <- record
				}
			}
		}
	}
}

// Schema returns Arrow schema of the given result set.
// Available after the first call to Next.
func (it *ArrowIterator) Schema() *arrow.Schema {
	return it.decoder.arrowSchema
}

// Next return the next batch of rows as an arrow.Record.
// Accessing Arrow Records directly has the drawnback of having to deal
// with memory management.
// Make sure to call Release() after using it
func (it *ArrowIterator) Next() (arrow.Record, error) {
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

// Table consumes all the iterator by calling Next and
// building an arrow table mixing all records and schema.
// Sequential calls will fail.
// Accessing Arrow Table directly has the drawback of having to deal
// with memory management.
// Make sure to call Release() after using it.
func (it *ArrowIterator) Table() (arrow.Table, error) {
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
	return array.NewTableFromRecords(it.decoder.arrowSchema, records), nil
}

func (it *RowIterator) init() error {
	if err := it.ArrowIterator.init(); err != nil {
		return err
	}
	return nil
}

func (it *RowIterator) processRecord(record arrow.Record) error {
	rows, err := it.ArrowIterator.decoder.convertArrowRecordValue(record)
	if err != nil {
		return err
	}
	for _, row := range rows {
		it.rows = append(it.rows, row)
	}
	return nil
}

// Next loads the next row into dst. Its return value is iterator.Done if there
// are no more results. Once Next returns iterator.Done, all subsequent calls
// will return iterator.Done.
// See more on the core package bigquery.RowIterator Next method.
func (it *RowIterator) Next(dst interface{}) error {
	if err := it.init(); err != nil {
		return err
	}

	vl, err := bigquery.ResolveValueLoader(dst, it.schema)
	if err != nil {
		return err
	}

	if len(it.rows) > 0 {
		row := it.rows[0]
		it.rows = it.rows[1:]
		return vl.Load(row, it.schema)
	}

	record, err := it.ArrowIterator.Next()
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
	return vl.Load(row, it.schema)
}
