// Copyright 2023 Google LLC
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
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery/internal/query"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// arrowIterator is a raw interface for getting data from Storage Read API
type arrowIterator struct {
	done uint32 // atomic flag
	errs chan error
	ctx  context.Context

	schema  Schema
	decoder *arrowDecoder
	records chan arrowRecordBatch

	session *readSession
}

type arrowRecordBatch []byte

func newStorageRowIteratorFromTable(ctx context.Context, table *Table, ordered bool) (*RowIterator, error) {
	md, err := table.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := table.c.rc.sessionForTable(ctx, table, ordered)
	if err != nil {
		return nil, err
	}
	it, err := newStorageRowIterator(rs)
	if err != nil {
		return nil, err
	}
	it.arrowIterator.schema = md.Schema
	return it, nil
}

func newStorageRowIteratorFromJob(ctx context.Context, j *Job) (*RowIterator, error) {
	// Needed to fetch destination table
	job, err := j.c.JobFromID(ctx, j.jobID)
	if err != nil {
		return nil, err
	}
	cfg, err := job.Config()
	if err != nil {
		return nil, err
	}
	qcfg := cfg.(*QueryConfig)
	if qcfg.Dst == nil {
		if !job.isScript() {
			return nil, fmt.Errorf("job has no destination table to read")
		}
		lastJob, err := resolveLastChildSelectJob(ctx, job)
		if err != nil {
			return nil, err
		}
		return newStorageRowIteratorFromJob(ctx, lastJob)
	}
	ordered := query.HasOrderedResults(qcfg.Q)
	return newStorageRowIteratorFromTable(ctx, qcfg.Dst, ordered)
}

func resolveLastChildSelectJob(ctx context.Context, job *Job) (*Job, error) {
	childJobs := []*Job{}
	it := job.Children(ctx)
	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to resolve table for script job: %w", err)
		}
		if !job.isSelectQuery() {
			continue
		}
		childJobs = append(childJobs, job)
	}
	if len(childJobs) == 0 {
		return nil, fmt.Errorf("failed to resolve table for script job: no child jobs found")
	}
	return childJobs[0], nil
}

func newRawStorageRowIterator(rs *readSession) (*arrowIterator, error) {
	arrowIt := &arrowIterator{
		ctx:     rs.ctx,
		session: rs,
		records: make(chan arrowRecordBatch, rs.settings.maxWorkerCount+1),
		errs:    make(chan error, rs.settings.maxWorkerCount+1),
	}
	if rs.bqSession == nil {
		err := rs.start()
		if err != nil {
			return nil, err
		}
	}
	return arrowIt, nil
}

func newStorageRowIterator(rs *readSession) (*RowIterator, error) {
	arrowIt, err := newRawStorageRowIterator(rs)
	if err != nil {
		return nil, err
	}
	totalRows := arrowIt.session.bqSession.EstimatedRowCount
	it := &RowIterator{
		ctx:           rs.ctx,
		arrowIterator: arrowIt,
		TotalRows:     uint64(totalRows),
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

	wg := sync.WaitGroup{}
	wg.Add(len(streams))
	sem := semaphore.NewWeighted(int64(it.session.settings.maxWorkerCount))
	go func() {
		wg.Wait()
		close(it.records)
		close(it.errs)
		it.markDone()
	}()

	go func() {
		for _, readStream := range streams {
			err := sem.Acquire(it.ctx, 1)
			if err != nil {
				wg.Done()
				continue
			}
			go func(readStreamName string) {
				it.processStream(readStreamName)
				sem.Release(1)
				wg.Done()
			}(readStream.Name)
		}
	}()
	return nil
}

func (it *arrowIterator) markDone() {
	atomic.StoreUint32(&it.done, 1)
}

func (it *arrowIterator) isDone() bool {
	return atomic.LoadUint32(&it.done) != 0
}

func (it *arrowIterator) processStream(readStream string) {
	bo := gax.Backoff{}
	var offset int64
	for {
		rowStream, err := it.session.readRows(&storagepb.ReadRowsRequest{
			ReadStream: readStream,
			Offset:     offset,
		})
		if err != nil {
			if it.session.ctx.Err() != nil { // context cancelled, don't try again
				return
			}
			backoff, shouldRetry := retryReadRows(bo, err)
			if shouldRetry {
				if err := gax.Sleep(it.ctx, backoff); err != nil {
					return // context cancelled
				}
				continue
			}
			it.errs <- fmt.Errorf("failed to read rows on stream %s: %w", readStream, err)
			continue
		}
		offset, err = it.consumeRowStream(readStream, rowStream, offset)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			if it.session.ctx.Err() != nil { // context cancelled, don't queue error
				return
			}
			// try to re-open row stream with updated offset
		}
	}
}

func retryReadRows(bo gax.Backoff, err error) (time.Duration, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return bo.Pause(), false
	}
	switch s.Code() {
	case codes.Aborted,
		codes.Canceled,
		codes.DeadlineExceeded,
		codes.FailedPrecondition,
		codes.Internal,
		codes.Unavailable:
		return bo.Pause(), true
	}
	return bo.Pause(), false
}

func (it *arrowIterator) consumeRowStream(readStream string, rowStream storagepb.BigQueryRead_ReadRowsClient, offset int64) (int64, error) {
	for {
		r, err := rowStream.Recv()
		if err != nil {
			if err == io.EOF {
				return offset, err
			}
			return offset, fmt.Errorf("failed to consume rows on stream %s: %w", readStream, err)
		}
		if r.RowCount > 0 {
			offset += r.RowCount
			arrowRecordBatch := r.GetArrowRecordBatch()
			it.records <- arrowRecordBatch.SerializedRecordBatch
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
	if it.isDone() {
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
