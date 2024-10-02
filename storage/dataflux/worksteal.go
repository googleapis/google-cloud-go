// Copyright 2024 Google LLC
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

package dataflux

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
)

const (
	// defaultAlphabet used to initiliaze rangesplitter. It must contain at least two unique characters.
	defaultAlphabet = "ab"
	// sleepDurationWhenIdle is the milliseconds we want each worker to sleep before checking
	// the next update if it is idle.
	sleepDurationWhenIdle = time.Millisecond * time.Duration(200)
)

// workerStatus indicates the status of a worker.
type workerStatus int

const (
	// idle status shows that the worker is currently not listing.
	idle workerStatus = iota
	// active status shows that the worker is currently listing objects within assigned range.
	active
)

type listerResult struct {
	mu      sync.Mutex
	objects []*storage.ObjectAttrs
}

type worker struct {
	goroutineID   int
	startRange    string
	endRange      string
	status        workerStatus
	rangesplitter *rangeSplitter
	idleChannel   chan int
	result        *listerResult
	generation    int64
	lister        *Lister
}

// workstealListing is the main entry point of the worksteal algorithm.
// It performs worksteal to achieve highly dynamic object listing.
// workstealListing creates multiple (parallelism) workers that simultaneosly lists
// objects from the buckets.
func (c *Lister) workstealListing(ctx context.Context) ([]*storage.ObjectAttrs, error) {
	var workerErr []error
	// Idle channel is used to track number of idle workers.
	idleChannel := make(chan int, c.parallelism)
	// Result is used to store results from each worker.
	result := &listerResult{
		objects: []*storage.ObjectAttrs{},
	}

	rs, err := newRangeSplitter(defaultAlphabet)
	if err != nil {
		return nil, fmt.Errorf("creating new range splitter: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)
	// Initialize all workers as idle.
	for i := 0; i < c.parallelism; i++ {
		idleWorker := &worker{
			goroutineID:   i,
			startRange:    "",
			endRange:      "",
			status:        idle,
			rangesplitter: rs,
			result:        result,
			idleChannel:   idleChannel,
			generation:    int64(0),
			lister:        c,
		}
		idleChannel <- 1
		g.Go(func() error {
			if err := idleWorker.doWorkstealListing(ctx); err != nil {
				workerErr = append(workerErr, err)
				return fmt.Errorf("listing worker ID %q: %w", i, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed waiting for multiple workers : %w", err)
	}
	if len(workerErr) > 0 {
		return nil, fmt.Errorf("failure in workers : %v", workerErr)
	}

	close(idleChannel)

	return result.objects, nil
}

// doWorkstealListing implements the listing logic for each worker.
// An active worker lists next page of objects to be listed within the given range
// and then splits range into two half if there are idle workers. Worker keeps
// the first of splitted range and passes second half of the work in range channel
// for idle workers. It continues to do this until shutdown signal is true.
// An idle worker waits till it finds work in rangeChannel. Once it finds work,
// it acts like an active worker.
func (w *worker) doWorkstealListing(ctx context.Context) error {
	for !w.shutDownSignal() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// If a worker is idle, sleep for a while before checking the next update.
		// Worker is active when it finds work in range channel.
		if w.status == idle {
			if len(w.lister.ranges) == 0 {
				time.Sleep(sleepDurationWhenIdle)
				continue
			} else {
				newRange := <-w.lister.ranges
				<-w.idleChannel
				w.updateWorker(newRange.startRange, newRange.endRange, active)
			}
		}
		// Active worker to list next page of objects within the range.
		doneListing, err := w.objectLister(ctx)
		if err != nil {
			return fmt.Errorf("objectLister failed: %w", err)
		}

		// If listing is complete for the range, make worker idle and continue.
		if doneListing {
			w.status = idle
			w.idleChannel <- 1
			w.generation = int64(0)
			continue
		}

		// If listing not complete and idle workers are available, split the range
		// and give half of work to idle worker.
		if len(w.idleChannel)-len(w.lister.ranges) > 0 && ctx.Err() == nil {
			// Split range and upload half of work for idle worker.
			splitPoint, err := w.rangesplitter.splitRange(w.startRange, w.endRange, 1)
			if err != nil {
				return fmt.Errorf("splitting range for worker ID:%v, err: %w", w.goroutineID, err)
			}
			// If split point is empty, skip splitting the work.
			if len(splitPoint) < 1 {
				continue
			}
			w.lister.ranges <- &listRange{startRange: splitPoint[0], endRange: w.endRange}

			// Update current worker range.
			w.endRange = splitPoint[0]
		}
	}
	// If the worker is active, update range channel to store the remaining work.
	if w.status == active {
		w.lister.ranges <- &listRange{startRange: w.startRange, endRange: w.endRange}
		// Worker is now idle.
		w.status = idle
	}
	return nil
}

// shutDownSignal returns true if all the workers are idle and the
// or number of objects listed is equal to page size.
func (w *worker) shutDownSignal() bool {
	// If all the workers are idle and range channel is empty, no more objects to list.
	noMoreObjects := len(w.idleChannel) == w.lister.parallelism && len(w.lister.ranges) == 0

	// If number of objects listed is equal to the given batchSize, then shutdown.
	// If batch size is not given i.e. 0, then list until all objects have been listed.
	alreadyListedBatchSizeObjects := len(w.idleChannel) == w.lister.parallelism && len(w.lister.ranges) == 0

	return noMoreObjects || alreadyListedBatchSizeObjects
}

// updateWorker updates the worker's start range, end range and status.
func (w *worker) updateWorker(startRange, endRange string, status workerStatus) {
	w.startRange = startRange
	w.endRange = endRange
	w.status = status
	w.generation = int64(0)
}

func (w *worker) objectLister(ctx context.Context) (bool, error) {
	// Active worker to list next page of objects within the range.
	nextPageResult, err := nextPage(ctx, nextPageOpts{
		startRange:           w.startRange,
		endRange:             w.endRange,
		bucketHandle:         w.lister.bucket,
		query:                w.lister.query,
		skipDirectoryObjects: w.lister.skipDirectoryObjects,
		generation:           w.generation,
	})
	if err != nil {
		return false, fmt.Errorf("listing next page for worker ID %v,  err: %w", w.goroutineID, err)
	}

	// Append objects listed by objectLister to result.
	w.result.mu.Lock()
	w.result.objects = append(w.result.objects, nextPageResult.items...)
	w.result.mu.Unlock()

	// Listing completed for default page size for the given range.
	// Update current worker start range to new range and generation
	// of the last objects listed if versions is true.
	w.startRange = nextPageResult.nextStartRange
	w.generation = nextPageResult.generation
	return nextPageResult.doneListing, nil
}

// nextPageOpts specifies options for next page of listing result .
type nextPageOpts struct {
	// startRange is the start offset of the objects to be listed.
	startRange string
	// endRange is the end offset of the objects to be listed.
	endRange string
	// bucketHandle is the bucket handle of the bucket to be listed.
	bucketHandle *storage.BucketHandle
	// query is the storage.Query to filter objects for listing.
	query storage.Query
	// skipDirectoryObjects is to indicate whether to list directory objects.
	skipDirectoryObjects bool
	// generation is the generation number of the last object in the page.
	generation int64
}

// nextPageResult holds the next page of object names, start of the next page
// and indicates whether the lister has completed listing (no more objects to retrieve).
type nextPageResult struct {
	// items is the list of objects listed.
	items []*storage.ObjectAttrs
	// doneListing indicates whether the lister has completed listing.
	doneListing bool
	// nextStartRange is the start offset of the next page of objects to be listed.
	nextStartRange string
	// generation is the generation number of the last object in the page.
	generation int64
}

// nextPage lists objects using the given lister options.
func nextPage(ctx context.Context, opts nextPageOpts) (*nextPageResult, error) {

	// TODO: Implement objectLister.

	return nil, nil
}

func addPrefix(name, prefix string) string {
	if name != "" {
		return prefix + name
	}
	return name
}
