// Copyright 2025 Google LLC
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

package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// storageArrowIterator is a raw interface for getting data from Storage Read API
type storageArrowIterator struct {
	done        uint32 // atomic flag
	initialized bool
	errs        chan error

	schema  *schema
	records chan *storagepb.ArrowRecordBatch

	rs  *storageReader
	ctx context.Context
}

func newRawStorageRowIterator(ctx context.Context, rs *storageReader, schema *schema) (*storageArrowIterator, error) {
	arrowIt := &storageArrowIterator{
		ctx:     ctx,
		rs:      rs,
		schema:  schema,
		records: make(chan *storagepb.ArrowRecordBatch, 0+1),
		errs:    make(chan error, 0+1),
	}
	return arrowIt, nil
}

func (it *storageArrowIterator) init() error {
	if it.initialized {
		return nil
	}

	if it.rs == nil {
		return errors.New("read session not initialized")
	}

	streams := it.rs.rs.Streams
	if len(streams) == 0 {
		return iterator.Done
	}

	wg := sync.WaitGroup{}
	wg.Add(len(streams))
	sem := semaphore.NewWeighted(int64(1)) // TODO: save max worker setting
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
	it.initialized = true
	return nil
}

func (it *storageArrowIterator) markDone() {
	atomic.StoreUint32(&it.done, 1)
}

func (it *storageArrowIterator) isDone() bool {
	return atomic.LoadUint32(&it.done) != 0
}

func (it *storageArrowIterator) processStream(readStream string) {
	bo := gax.Backoff{}
	var offset int64
	for {
		rowStream, err := it.rs.rc.ReadRows(it.ctx, &storagepb.ReadRowsRequest{
			ReadStream: readStream,
			Offset:     offset,
		})
		if err != nil {
			serr := it.handleProcessStreamError(readStream, bo, err)
			if serr != nil {
				return
			}
			continue
		}
		offset, err = it.consumeRowStream(readStream, rowStream, offset)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			serr := it.handleProcessStreamError(readStream, bo, err)
			if serr != nil {
				return
			}
			// try to re-open row stream with updated offset
		}
	}
}

// handleProcessStreamError check if err is retryable,
// waiting with exponential backoff in that scenario.
// If error is not retryable, queue up err to be sent to user.
// Return error if should exit the goroutine.
func (it *storageArrowIterator) handleProcessStreamError(readStream string, bo gax.Backoff, err error) error {
	if it.ctx.Err() != nil { // context cancelled, don't try again
		return it.ctx.Err()
	}
	backoff, shouldRetry := retryReadRows(bo, err)
	if shouldRetry {
		if err := gax.Sleep(it.ctx, backoff); err != nil {
			return err // context cancelled
		}
		return nil
	}
	select {
	case it.errs <- fmt.Errorf("failed to read rows on stream %s: %w", readStream, err):
		return nil
	case <-it.ctx.Done():
		return context.Canceled
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
		codes.Internal,
		codes.Unavailable:
		return bo.Pause(), true
	}
	return bo.Pause(), false
}

func (it *storageArrowIterator) consumeRowStream(readStream string, rowStream storagepb.BigQueryRead_ReadRowsClient, offset int64) (int64, error) {
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
			recordBatch := r.GetArrowRecordBatch()
			it.records <- recordBatch
		}
	}
}

// next return the next batch of rows as an arrow.Record.
// Accessing Arrow Records directly has the drawnback of having to deal
// with memory management.
func (it *storageArrowIterator) next() (*storagepb.ArrowRecordBatch, error) {
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
