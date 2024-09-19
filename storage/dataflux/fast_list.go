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

	// Parallelism is number of parallel workers to use for listing. Default value is 10x number of available CPU. Optional.
	Parallelism int

	// BatchSize is the number of objects to list. Default value returns all objects at once. Optional.
	// The number of objects returned will be rounded up to a multiple of gcs page size.
	BatchSize int

	// Query is the query to filter objects for listing. Default value is nil. Optional.
	Query storage.Query

	// SkipDirectoryObjects is to indicate whether to list directory objects. Default value is false. Optional.
	SkipDirectoryObjects bool
}

// Lister is used for interacting with Dataflux fast-listing.
// The caller should initialize it with NewLister() instead of creating it directly.
type Lister struct {
	// method indicates the listing method(open, sequential, worksteal) to be used for listing.
	method listingMethod

	// pageToken is the token to use for sequential listing.
	pageToken string

	// bucket is the bucket handle to list objects from.
	bucket *storage.BucketHandle

	// batchSize is the number of objects to list.
	batchSize int

	// query is the query to filter objects for listing.
	query storage.Query

	// skipDirectoryObjects is to indicate whether to list directory objects.
	skipDirectoryObjects bool
}

// NewLister creates a new dataflux Lister to list objects in the give bucket.
func NewLister(c *storage.Client, in *ListerInput) *Lister {
	bucket := c.Bucket(in.BucketName)
	lister := &Lister{
		method:               open,
		pageToken:            "",
		bucket:               bucket,
		batchSize:            in.BatchSize,
		query:                in.Query,
		skipDirectoryObjects: in.SkipDirectoryObjects,
	}
	return lister
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
	// Errgroup takes care of running both methods in parallel. As soon as one of the method
	// is complete, the running method also stops.
	g, childCtx := errgroup.WithContext(ctx)

	// To start listing method is Open and runs both worksteal and sequential listing in parallel.
	// The method which completes first is used for all subsequent runs.
	// TODO: Run worksteal listing when method is Open or WorkSteal.
	// Run sequential listing when method is Open or Sequential.
	if c.method != worksteal {

		g.Go(func() error {
			objects, nextToken, err := c.sequentialListing(childCtx)
			if err != nil {
				countError++
				return fmt.Errorf("error in running sequential listing: %w", err)
			}
			// If sequential listing completes first, set method to sequential listing and ranges to nil.
			// The nextToken will be used to continue sequential listing.
			results = objects
			c.pageToken = nextToken
			c.method = sequential
			// Close context when sequential listing is complete.
			cancel()
			return nil
		})
	}

	// Close all functions if either sequential listing or worksteal listing is complete.
	err := g.Wait()

	// If the error is not context.Canceled, then return error instead of falling back
	// to the other method. This is so that the error can be fixed and user can take
	//  advantage of fast-listing.
	// As one of the listing method completes, it is expected to cancel context for the other method.
	// If both sequential and worksteal listing fail due to context canceled, only then return error.
	if err != nil && (!errors.Is(err, context.Canceled) || countError > 1) {
		return nil, fmt.Errorf("failed waiting for sequntial and work steal lister : %w", err)
	}

	// If ranges for worksteal and pageToken for sequential listing is empty, then listing is complete.
	if c.pageToken == "" {
		return results, iterator.Done
	}
	return results, nil
}

// Close closes the range channel of the Lister.
func (c *Lister) Close() {

	// TODO: Close range channel for worksteal lister.
}
