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

	// Parallelism is number of parallel workers to use for listing. Optional.
	Parallelism int

	// BatchSize is the number of objects to list. Optional.
	BatchSize int

	// Query is the query to filter objects for listing. Optional.
	Query storage.Query

	// SkipDirectoryObjects is to indicate whether to list directory objects. Optional.
	SkipDirectoryObjects bool
}

// Lister is used for interacting with Dataflux fast-listing.
// The caller should initialize it with NewLister() instead of creating it directly.
type Lister struct {
	// method indicates the listing method(open, sequential, worksteal) to be used for listing.
	method listingMethod

	// pageToken is the token to use for sequential listing.
	pageToken string

	// ranges is the channel to store the start and end ranges to be listed by the workers in worksteal listing.
	ranges chan *listRange

	// bucket is the bucket handle to list objects from.
	bucket *storage.BucketHandle

	// parallelism is number of parallel workers to use for listing.
	parallelism int

	// batchSize is the number of objects to list.
	batchSize int

	// query is the query to filter objects for listing.
	query storage.Query

	// skipDirectoryObjects is to indicate whether to list directory objects.
	skipDirectoryObjects bool
}

// CloseFunc is the function to close the range channel of a Lister.
type CloseFunc func()

// NewLister creates a new dataflux Lister to list objects in the give bucket.
func NewLister(c *storage.Client, in *ListerInput) (*Lister, CloseFunc) {
	bucket := c.Bucket(in.BucketName)
	lister := &Lister{
		method:               open,
		pageToken:            "",
		bucket:               bucket,
		batchSize:            in.BatchSize,
		query:                in.Query,
		skipDirectoryObjects: in.SkipDirectoryObjects,
	}
	return lister, func() { lister.Close() }
}

// NextBatch runs worksteal algorithm and sequential listing in parallel to quickly
// return a list of objects in the bucket. For smaller dataset,
// sequential listing is expected to be faster. For larger dataset,
// worksteal listing is expected to be faster.
func (c *Lister) NextBatch(ctx context.Context) ([]*storage.ObjectAttrs, error) {
	// countError tracks the number of failed listing methods.
	countError := 0
	var results []*storage.ObjectAttrs
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, childCtx := errgroup.WithContext(ctx)

	// Run worksteal listing when method is Open or WorkSteal.
	// Run sequential listing when method is Open or Sequential.
	if c.method != worksteal {

		g.Go(func() error {
			objects, nextToken, err := sequentialListing(childCtx, *c)
			if err != nil {
				countError++
				return fmt.Errorf("error in running sequential listing: %w", err)
			}
			// If sequential listing completes first, set method to sequential listing and ranges to nil.
			// The nextToken will be used to continue sequential listing.
			results = objects
			c.pageToken = nextToken
			c.method = sequential
			c.ranges = nil
			// Close context when sequential listing is complete.
			cancel()
			return nil
		})
	}

	// Close all functions if either sequential listing or worksteal listing is complete.
	err := g.Wait()

	// If there is not context.Canceled, then return error.
	// As one of the listing method completes, it is expected to cancel context for the other method.
	// If both sequential and worksteal listing fail due to context canceled, then return error.
	if err != nil && (!errors.Is(err, context.Canceled) || countError > 1) {
		return nil, fmt.Errorf("failed waiting for sequntial and work steal lister : %w", err)
	}

	// If ranges for worksteal and pageToken for sequential listing is empty, then listing is complete.
	if len(c.ranges) == 0 && c.pageToken == "" {
		return results, iterator.Done
	}
	return results, nil
}

// Close closes the range channel of the Lister.
func (c *Lister) Close() {
	if c.ranges != nil {
		close(c.ranges)
	}
}