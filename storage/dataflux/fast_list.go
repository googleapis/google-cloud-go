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

	// BatchSize is the number of objects to list. Default value returns
	// all objects at once. The number of objects returned will be
	// rounded up to a multiple of gcs page size. Optional.
	BatchSize int

	// Query is the query to filter objects for listing. Default value is nil.
	// Use ProjectionNoACL for faster listing. Including ACLs increases
	// latency while fetching objects. Optional.
	Query storage.Query

	// SkipDirectoryObjects is to indicate whether to list directory objects.
	// Default value is false. Optional.
	SkipDirectoryObjects bool
}

// NewLister creates a new dataflux Lister to list objects in the give bucket.
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
	start, end := updateStartEndOffset(in.Query.StartOffset, in.Query.EndOffset, in.Query.Prefix)
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

// NextBatch runs worksteal algorithm and sequential listing in parallel to quickly
// return a list of objects in the bucket. For smaller dataset,
// sequential listing is expected to be faster. For larger dataset,
// worksteal listing is expected to be faster.
func (c *Lister) NextBatch(ctx context.Context) ([]*storage.ObjectAttrs, error) {
	// countError tracks the number of failed listing methods.
	cc := &countErr{counter: 0}

	var results []*storage.ObjectAttrs
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wsCompletedfirst := false
	seqCompletedfirst := false
	var wsObjects []*storage.ObjectAttrs
	var seqObjects []*storage.ObjectAttrs
	var nextToken string

	// Errgroup takes care of running both methods in parallel. As soon as one of
	// the method is complete, the running method also stops.
	g, childCtx := errgroup.WithContext(ctx)

	// To start listing method is Open and runs both worksteal and sequential listing
	// in parallel. The method which completes first is used for all subsequent runs.

	// Run worksteal listing when method is Open or WorkSteal.
	if c.method != sequential {

		g.Go(func() error {
			objects, err := c.workstealListing(childCtx)
			if err != nil {
				cc.increment()
				return fmt.Errorf("error in running worksteal_lister: %w", err)
			}
			// Close context when sequential listing is complete.
			cancel()
			wsCompletedfirst = true
			wsObjects = objects

			return nil
		})
	}

	// Run sequential listing when method is Open or Sequential.
	if c.method != worksteal {

		g.Go(func() error {
			objects, token, err := c.sequentialListing(childCtx)
			if err != nil {
				cc.increment()
				return fmt.Errorf("error in running sequential listing: %w", err)
			}
			// Close context when sequential listing is complete.
			cancel()
			seqCompletedfirst = true
			seqObjects = objects
			nextToken = token

			return nil
		})
	}

	// Close all functions if either sequential listing or worksteal listing is complete.
	err := g.Wait()

	// If the error is not context.Canceled, then return error instead of falling back
	// to the other method. This is so that the error can be fixed and user can take
	//  advantage of fast-listing.
	// As one of the listing method completes, it is expected to cancel context for the
	// only then return error. other method. If both sequential and worksteal listing
	// fail due to context canceled, return error.
	if err != nil && (!strings.Contains(err.Error(), context.Canceled.Error()) || cc.counter > 1) {
		return nil, fmt.Errorf("failed waiting for sequential and work steal lister : %w", err)
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

// updateStartEndOffset updates start and end offset based on prefix.
// If a prefix is given, adjust start and end value such that it lists
// objects with the given prefix. updateStartEndOffset assumes prefix will
// be added to the object name while listing objects in worksteal algorithm.
//
//	For example:
//	start = "abc",  end = "prefix_a", prefix = "prefix",
//
//	end will change to "_a", prefix will be added in worksteal algorithm.
//	"abc" is lexicographically smaller than "prefix". So start will be the first
//	object with the given prefix.
//
//	Therefore start will change to ""(empty string) and end to "_a" .
func updateStartEndOffset(start, end, prefix string) (string, string) {
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
