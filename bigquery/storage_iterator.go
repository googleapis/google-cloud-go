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

package bigquery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
)

// arrowIterator is a raw interface for getting data from Storage Read API
type arrowIterator struct {
	done bool
	errs chan error
	wg   sync.WaitGroup
	ctx  context.Context

	schema  Schema
	decoder *arrowDecoder
	records chan arrowRecordBatch

	session *readSession
}

type arrowRecordBatch []byte

func newStorageRowIteratorFromTable(ctx context.Context, table *Table) (*RowIterator, error) {
	md, err := table.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := table.c.rc.sessionForTable(ctx, table)
	if err != nil {
		return nil, err
	}
	it, err := newStorageRowIterator(ctx, rs, md.NumRows)
	if err != nil {
		return nil, err
	}
	it.arrowIterator.schema = md.Schema
	return it, nil
}

func newStorageRowIteratorFromJob(ctx context.Context, job *Job, totalRows uint64) (*RowIterator, error) {
	cfg, err := job.Config()
	if err != nil {
		return nil, err
	}
	qcfg := cfg.(*QueryConfig)
	if qcfg.Dst == nil {
		// TODO: script job ?
		return nil, fmt.Errorf("nil job destination table")
	}
	return newStorageRowIteratorFromTable(ctx, qcfg.Dst)
}

func newRawStorageRowIterator(ctx context.Context, rs *readSession) (*arrowIterator, error) {
	arrowIt := &arrowIterator{
		ctx:     ctx,
		session: rs,
		records: make(chan arrowRecordBatch, 10000),
		errs:    make(chan error, 1),
	}
	if rs.bqSession == nil {
		err := rs.start()
		if err != nil {
			return nil, err
		}
	}
	return arrowIt, nil
}

func newStorageRowIterator(ctx context.Context, rs *readSession, totalRows uint64) (*RowIterator, error) {
	arrowIt, err := newRawStorageRowIterator(ctx, rs)
	if err != nil {
		return nil, err
	}
	it := &RowIterator{
		ctx:           ctx,
		arrowIterator: arrowIt,
		TotalRows:     totalRows,
		rows:          [][]Value{},
	}
	it.nextFunc = nextFuncForStorageIterator(it)
	it.pageInfo = &iterator.PageInfo{
		Token:   "",
		MaxSize: int(totalRows),
	}
	return it, nil
}

func nextFuncForStorageIterator(it *RowIterator) func() error {
	return func() error {
		if len(it.rows) > 0 {
			return nil
		}
		arrowIt := it.arrowIterator
		record, err := arrowIt.next()
		if err == iterator.Done {
			if len(it.rows) == 0 {
				return iterator.Done
			}
			return nil
		}
		if err != nil {
			return err
		}

		rows, err := arrowIt.decoder.decodeArrowRecords(record)
		if err != nil {
			return err
		}
		it.rows = rows
		return nil
	}
}

func (it *arrowIterator) init() error {
	if it.decoder != nil { // Already initialized
		return nil
	}

	bqSession := it.session.bqSession
	if bqSession == nil {
		return errors.New("read session not initialized")
	}

	streams := bqSession.Streams
	if len(streams) == 0 {
		return iterator.Done
	}

	if it.schema == nil {
		meta, err := it.session.table.Metadata(it.ctx)
		if err != nil {
			return err
		}
		it.schema = meta.Schema
	}

	decoder, err := newArrowDecoderFromSession(it.session, it.schema)
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
		go it.processStream(readStream.Name)
	}
	return nil
}

func (it *arrowIterator) processStream(readStream string) {
	var offset int64
	for {
		rowStream, err := it.session.readRows(&storage.ReadRowsRequest{
			ReadStream: readStream,
			Offset:     offset,
		})
		if err != nil {
			it.errs <- fmt.Errorf("failed to read rows on stream %s: %v", readStream, err)
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
			if r.RowCount > 0 {
				offset += r.RowCount
				arrowRecordBatch := r.GetArrowRecordBatch()
				it.records <- arrowRecordBatch.SerializedRecordBatch
			}
		}
	}
}

// next return the next batch of rows as an arrow.Record.
// Accessing Arrow Records directly has the drawnback of having to deal
// with memory management.
func (it *arrowIterator) next() (arrowRecordBatch, error) {
	if err := it.init(); err != nil {
		return nil, err
	}
	if len(it.records) > 0 {
		return <-it.records, nil
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

// IsAccelerated check if the current RowIterator is
// being accelerated by Storage API.
func (it *RowIterator) IsAccelerated() bool {
	return it.arrowIterator != nil
}
