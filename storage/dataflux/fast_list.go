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
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
)

// listingMethod represents the method of listing.
type listingMethod int

const (
	// open when any method can be used to list.
	open listingMethod = iota
	// sequential when the listing is done sequentially.
	sequential
	// worksteal when the listing is done using work stealing algorithm.
	worksteal
)

// ListerInput contains options for listing objects.
type ListerInput struct {
	// BucketName is the name of the bucket to list objects from. Required.
	BucketName string

	// Parallelism is number of parallel workers to use for listing.
	// Default value is 10x number of available CPU. Optional.
	Parallelism int

	// BatchSize is the minimum number of objects to list in each batch.
	// The number of objects returned in a batch will be rounded up to
	// include all the objects received in the last request to GCS.
	// By default, the Lister returns all objects in one batch.
	// Optional.
	BatchSize int

	// Query is the query to filter objects for listing. Default value is nil.
	// Use ProjectionNoACL for faster listing. Including ACLs increases
	// latency while fetching objects. Optional.
	Query storage.Query

	// SkipDirectoryObjects is to indicate whether to list directory objects.
	// Note: Even if directory objects are excluded, they contribute to the
	// [ListerInput.BatchSize] count. Default value is false. Optional.
	SkipDirectoryObjects bool
}

// NewLister creates a new [Lister] that can be used to list objects in the given bucket.
func NewLister(c *storage.Client, in *ListerInput) *Lister {
	bucket := c.Bucket(in.BucketName)

	// If parallelism is not given, set default value to 10x the number of
	// available CPU.
	if in.Parallelism == 0 {
		in.Parallelism = runtime.NumCPU() * 10
	}
	// Initialize range channel with entire namespace of object for given
	// prefix, startoffset and endoffset. For the default range to list is
	// entire namespace, start and end will be empty.
	rangeChannel := make(chan *listRange, in.Parallelism*2)
	start, end := prefixAdjustedOffsets(in.Query.StartOffset, in.Query.EndOffset, in.Query.Prefix)
	rangeChannel <- &listRange{startRange: start, endRange: end}

	lister := &Lister{
		method:               open,
		parallelism:          in.Parallelism,
		pageToken:            "",
		bucket:               bucket,
		batchSize:            in.BatchSize,
		query:                in.Query,
		skipDirectoryObjects: in.SkipDirectoryObjects,
		ranges:               rangeChannel,
	}
	return lister
}

// Lister is used for interacting with Dataflux fast-listing. The caller should
// initialize it with NewLister() instead of creating it directly.
type Lister struct {
	// method indicates the listing method(open, sequential, worksteal) to
	// be used for listing.
	method listingMethod

	// bucket is the bucket handle to list objects from.
	bucket *storage.BucketHandle

	// batchSize is the number of objects to list.
	batchSize int

	// parallelism is number of parallel workers to use for listing.
	parallelism int

	// query is the query to filter objects for listing.
	query storage.Query

	// pageToken is the token to use for sequential listing.
	pageToken string

	// ranges is the channel to store the start and end ranges to be listed
	// by the workers in worksteal listing.
	ranges chan *listRange

	// skipDirectoryObjects is to indicate whether to list directory objects.
	skipDirectoryObjects bool
}

// NextBatch returns the next N objects in the bucket, where N is [ListerInput.BatchSize].
// In case of failure, all processes are stopped and an error is returned immediately. Create a new Lister to retry.
// For the first batch, both worksteal listing and sequential
// listing runs in parallel to quickly list N number of objects in the bucket. For subsequent
// batches, only the method which returned object faster in the first batch is used.
// For smaller dataset, sequential listing is expected to be faster. For larger dataset,
// worksteal listing is expected to be faster.
//
// Worksteal algorithm list objects in GCS bucket in parallel using multiple parallel
// workers and each worker in the list operation is able to steal work from its siblings
// once it has finished all currently slated listing work.
func (c *Lister) NextBatch(ctx context.Context) ([]*storage.ObjectAttrs, error) {

	var results []*storage.ObjectAttrs

	// For the first batch, listing method is open and runs both worksteal and sequential listing
	// in parallel. The method which completes first is used for all subsequent NextBatch calls.
	switch c.method {
	case worksteal:
		// Run worksteal algorithm for listing.
		objects, err := c.workstealListing(ctx)
		if err != nil {
			return nil, fmt.Errorf("worksteal listing: %w", err)
		}
		results = objects
	case sequential:
		// Run GCS sequential listing.
		objects, token, err := c.sequentialListing(ctx)
		if err != nil {
			return nil, fmt.Errorf("sequential listing: %w", err)
		}
		results = objects
		c.pageToken = token
		c.ranges = nil
	case open:
		// countError tracks the number of failed listing methods.
		countErr := &countErr{counter: 0}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		// Errgroup takes care of running both methods in parallel. As soon as one of
		// the method is complete, the running method also stops.
		g, ctx := errgroup.WithContext(ctx)
		wsCompletedfirst := false
		seqCompletedfirst := false
		var wsObjects []*storage.ObjectAttrs
		var seqObjects []*storage.ObjectAttrs
		var nextToken string
		g.Go(func() error {
			objects, err := c.workstealListing(ctx)
			if err != nil {
				countErr.increment()
				return fmt.Errorf("worksteal listing: %w", err)
			}
			// Close context when worksteal listing is complete.
			cancel()
			wsCompletedfirst = true
			wsObjects = objects

			return nil
		})
		g.Go(func() error {
			objects, token, err := c.sequentialListing(ctx)
			if err != nil {
				countErr.increment()
				return fmt.Errorf("sequential listing: %w", err)
			}
			// Close context when sequential listing is complete.
			cancel()
			seqCompletedfirst = true
			seqObjects = objects
			nextToken = token

			return nil
		})
		// Close all functions if either sequential listing or worksteal listing is complete.
		err := g.Wait()

		// If the error is not context.Canceled, then return error instead of falling back
		// to the other method. This is so that the error can be fixed and user can take
		// advantage of fast-listing.
		// As one of the listing method completes, it is expected to cancel context and
		// return context canceled error for the other method. Since context canceled is expected, it
		// will not be considered an error. If both sequential and worksteal listing fail due
		// to context canceled, then return error.
		if err != nil && (!errors.Is(err, context.Canceled) || countErr.counter > 1) {
			return nil, fmt.Errorf("dataflux: %w", err)
		}
		if wsCompletedfirst {
			// If worksteal listing completes first, set method to worksteal listing and nextToken to "".
			// The c.ranges channel will be used to continue worksteal listing.
			results = wsObjects
			c.pageToken = ""
			c.method = worksteal
		} else if seqCompletedfirst {
			// If sequential listing completes first, set method to sequential listing
			// and ranges to nil. The nextToken will be used to continue sequential listing.
			results = seqObjects
			c.pageToken = nextToken
			c.method = sequential
			c.ranges = nil
		}
	}

	// If ranges for worksteal and pageToken for sequential listing is empty, then
	// listing is complete.
	if c.pageToken == "" && len(c.ranges) == 0 {
		return results, iterator.Done
	}
	return results, nil
}

// Close is used to close the Lister.
func (c *Lister) Close() {
	if c.ranges != nil {
		close(c.ranges)
	}
}

type countErr struct {
	mu      sync.Mutex
	counter int
}

func (cc *countErr) increment() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.counter++
}

// prefixAdjustedOffsets returns a start and end offset adjusted from the given offsets based on the prefix, stripping the prefix.
// These offsets can be used by adding back the prefix, so that the original offsets do not need to be checked.

// This means that if the given offsets are out of range of the prefix
// (for example, offsets {start:"a", end: "b"}, with prefix "c" which is lexicographically
// outside of "a" to "b"), the returned offsets will ensure no strings fall in their range.

// Otherwise, if the offset is too permissive given the prefix, it returns an empty string
// to indicate there is no offset and all objects starting from or ending at the prefix should
// be listed.
//
//	For example:
//	start = "abc",  end = "prefix_a", prefix = "prefix",
//
// "abc" is lexicographically smaller than "prefix". The start offset indicates first
//
//	object with the given prefix should be listed therefor start offset will be empty.
//	The end offset will change to "_a" as the prefix is stripped.
//	Therefore new offset will change to {start = "",  end = "_a" }.
func prefixAdjustedOffsets(start, end, prefix string) (string, string) {
	if prefix == "" {
		return start, end
	}
	if start != "" && end != "" && start >= end {
		return start, start
	}
	if start != "" {
		if start <= prefix {
			start = ""
		} else if strings.HasPrefix(start, prefix) {
			start = start[len(prefix):]
		} else {
			return start, start
		}
	}

	if end != "" {
		if len(end) > len(prefix) && strings.HasPrefix(end, prefix) {
			end = end[len(prefix):]
		} else if end > prefix {
			end = ""
		} else {
			return end, end
		}
	}
	return start, end
}
