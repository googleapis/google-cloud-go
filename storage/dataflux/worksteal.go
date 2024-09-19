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
	"sync"

	"cloud.google.com/go/storage"
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
}

// workstealListing is the main entry point of the worksteal algorithm.
// It performs worksteal to achieve highly dynamic object listing.
func (c *Lister) workstealListing(ctx context.Context) []*storage.ObjectAttrs {
	return nil
}

// newObjectListerOpts specifies options for instantiating the NewObjectLister.
type newObjectListerOpts struct {
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

// nextPageResult holds the next page of object names and indicates whether the
// lister has completed listing (no more objects to retrieve).
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

func addPrefix(name, prefix string) string {
	if name != "" {
		return prefix + name
	}
	return name
}
