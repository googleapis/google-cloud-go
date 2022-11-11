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
	"sync"

	"cloud.google.com/go/bigquery"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	"google.golang.org/api/iterator"
)

// arrowIterator is a raw interface for getting data from Storage Read API
type arrowIterator struct {
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
	*arrowIterator

	rows [][]bigquery.Value
}

func newRawJobRowIterator(ctx context.Context, rs *ReadSession, job *bigquery.Job) (*arrowIterator, error) {
	rowIt, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}
	arrowIt := &arrowIterator{
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
		arrowIterator: arrowIt,
		rows:          [][]bigquery.Value{},
	}
	return it, nil
}

func newRawTableRowIterator(ctx context.Context, rs *ReadSession, table *bigquery.Table) (*arrowIterator, error) {
	rowIt := table.Read(ctx)
	arrowIt := &arrowIterator{
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
		arrowIterator: arrowIt,
		rows:          [][]bigquery.Value{},
	}
	return it, nil
}

func (it *arrowIterator) init() error {
	if it.decoder != nil { // Already nitialized
		return nil
	}
	streams := it.Session.ReadStreams
	if len(streams) == 0 {
		it.errs <- iterator.Done
		return nil
	}

	meta, err := it.Session.table.Metadata(it.ctx)
	if err != nil {
		return err
	}
	it.schema = meta.Schema

	decoder, err := newArrowDecoderFromSession(it.Session, it.schema)
	if err != nil {
		return err
	}
	it.decoder = decoder

	go func() {
		it.wg.Wait()
		close(it.records)
		it.done = true
	}()

	for _, readStream := range streams {
		it.wg.Add(1)
		go it.processStream(readStream)
	}
	return nil
}

func (it *arrowIterator) processStream(readStream string) {
	var offset int64
	for {
		rowStream, err := it.Session.ReadRows(ReadRowsRequest{
			ReadStream: readStream,
			Offset:     offset,
		})
		if err != nil {
			it.errs <- fmt.Errorf("failed to read rows on stream %s: %v", readStream, err)
		}
		for {
			r, err := rowStream.Next()
			if err == iterator.Done {
				it.wg.Done()
				return
			}
			if err != nil {
				it.errs <- err
			}
			if r.RowCount > 0 {
				offset += r.RowCount
				records, err := it.decoder.decodeArrowRecords(r.SerializedArrowRecordBatch)
				if err != nil {
					it.errs <- fmt.Errorf("failed to decode arrow record on stream %s: %v", readStream, err)
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
func (it *arrowIterator) Schema() *arrow.Schema {
	return it.decoder.arrowSchema
}

// Next return the next batch of rows as an arrow.Record.
// Accessing Arrow Records directly has the drawnback of having to deal
// with memory management.
// Make sure to call Release() after using it
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

// Table consumes all the iterator by calling Next and
// building an arrow table mixing all records and schema.
// Sequential calls will fail.
// Accessing Arrow Table directly has the drawback of having to deal
// with memory management.
// Make sure to call Release() after using it.
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
	return array.NewTableFromRecords(it.decoder.arrowSchema, records), nil
}

func (it *RowIterator) init() error {
	if err := it.arrowIterator.init(); err != nil {
		return err
	}
	return nil
}

func (it *RowIterator) processRecord(record arrow.Record) error {
	rows, err := it.arrowIterator.decoder.convertArrowRecordValue(record)
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

	record, err := it.arrowIterator.Next()
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
