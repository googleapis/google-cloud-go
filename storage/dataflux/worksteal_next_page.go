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
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

const (
	// wsDefaultPageSize specifies the number of object results to include in a single page for worksteal listing.
	wsDefaultPageSize = 1000
)

// nextPageOpts specifies options for next page of listing result .
type nextPageOpts struct {
	// startRange is the start offset of the objects to be listed.
	startRange string
	// endRange is the end offset of the objects to be listed.
	endRange string
	// bucketHandle is the bucket handle of the bucket from which objects are to be listed.
	bucketHandle *storage.BucketHandle
	// query is the storage.Query to filter objects for listing.
	query storage.Query
	// skipDirectoryObjects is to indicate whether to skip or list directory objects.
	skipDirectoryObjects bool
	// generation is the generation number of the last object in the page.
	generation int64
}

// nextPageResult represents the results of fetching a single page of objects
// from a GCS listing operation and information for remaining objects to be listed.
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

// nextPage retrieves a single page of objects from GCS using the provided
// listing options (nextPageOpts). It returns a nextPageResult containing the
// list of objects, a flag indicating if the listing is complete, the starting
// point for the next page, and the generation of the last object in the page.
// In case multiple versions of an object needs to be listed, more than one page
// can also be listed to avoid duplicate listing.
func nextPage(ctx context.Context, opts nextPageOpts) (*nextPageResult, error) {

	opts.query.StartOffset = addPrefix(opts.startRange, opts.query.Prefix)
	opts.query.EndOffset = addPrefix(opts.endRange, opts.query.Prefix)
	objectIterator := opts.bucketHandle.Objects(ctx, &opts.query)
	var items []*storage.ObjectAttrs

	// nameLexLast is the name of lexicographically last object in the page.
	nameLexLast := ""
	// indexLexLast is the index of lexicographically last object in the page.
	// If the item is iterated but not added to the items list, then indexLexLast is -1.
	indexLexLast := 0
	// indexItemLast is the index of the last item in the items list.
	indexItemLast := -1

	// The Go Listing API does not expose an interface to list multiple objects together,
	// thus we need to manually loop to construct a page of results using the iterator.
	for i := 0; i < wsDefaultPageSize; i++ {
		attrs, err := objectIterator.Next()

		// If the lister has listed the last item for the assigned range,
		// then set doneListing to true and return.
		if err == iterator.Done {
			return &nextPageResult{
				items:          items,
				doneListing:    true,
				nextStartRange: "",
				generation:     int64(0),
			}, nil
		} else if err != nil {
			return nil, fmt.Errorf("iterating through objects: %w", err)
		}

		// Skip object versions already processed in the previous page to prevent duplicates.
		// Objects are listed in the increasing order of generation.
		// See https://cloud.google.com/storage/docs/json_api/v1/objects/list#parameters.
		if opts.query.Versions && opts.query.StartOffset == attrs.Name && attrs.Generation < opts.generation {
			continue
		}

		// Append object to items.
		// indexItemLast tracks index of the last item added to the items list.
		if !(opts.skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			items = append(items, attrs)
			indexItemLast++
		}

		// If name/prefix of current object is greater than nameLexLast, update nameLexLast and indexLexLast.
		if nameLexLast <= attrs.Name || nameLexLast <= attrs.Prefix {
			updateLexLastObject(&nameLexLast, &indexLexLast, indexItemLast, attrs, opts.skipDirectoryObjects)
		}

		// If the whole page lists different versions of the same object, i.e.
		// "startoffset" value matches the name of the last object,
		// list another page to ensure the next NextStartRange is distinct from the current one.
		sameObjectPage := opts.query.Versions && i == wsDefaultPageSize-1 && attrs.Generation != int64(0) && opts.query.StartOffset == attrs.Name

		// If the generation value is not set, list next page if the last item is a version of previous item to prevent duplicate listing.
		generationNotSet := opts.query.Versions && i == wsDefaultPageSize-1 && attrs.Generation == int64(0) && indexLexLast > 0 && items[indexLexLast-1].Name == items[indexLexLast].Name

		if sameObjectPage || generationNotSet {
			i = -1
		}

	}

	// Make last item as next start range. Remove the prefix from the name so that range calculations
	// remain prefix-agnostic. This is necessary due to the unbounded end-range when splitting string
	// namespaces of unknown size.
	nextStartRange := strings.TrimPrefix(nameLexLast, opts.query.Prefix)
	generation := int64(0)

	// Remove lexicographically last item from the item list to avoid duplicate listing and
	// store generation value of the item removed from the list. indexLexLast less than zero
	// indicats that the lexicographically last item is not added to the items list.
	if indexLexLast >= 0 && len(items) > 0 {
		if indexLexLast >= indexItemLast {
			// If the item is at the end of the list, remove last item.
			generation = items[indexItemLast].Generation
			items = items[:len(items)-1]
		} else {
			// If the item is not at the end of the list, remove the item at indexLexLast.
			// This is possible since directory objects are listed first in a page.
			generation = items[indexLexLast].Generation
			items = append(items[:indexLexLast], items[indexLexLast+1:]...)
		}
	}

	// If versions is false in query, only latest version of the object will be
	// listed. Therefore, generation is not required.
	if !opts.query.Versions {
		generation = int64(0)
	}

	return &nextPageResult{
		items:          items,
		doneListing:    false,
		nextStartRange: nextStartRange,
		generation:     generation,
	}, nil
}

func updateLexLastObject(nameLexLast *string, indexLexLast *int, indexItemLast int, attrs *storage.ObjectAttrs, skipDirectoryObjects bool) {
	*nameLexLast = attrs.Prefix
	if *nameLexLast <= attrs.Name {
		*nameLexLast = attrs.Name
	}
	// If object is added to the items list, then update indexLexLast to current item index, else set indexLexLast to -1.
	// Setting indexLexLast to -1, indicates that the lexicographically last item is not added to items list.
	if !(skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
		*indexLexLast = indexItemLast
	} else {
		*indexLexLast = -1
	}
}

func addPrefix(name, prefix string) string {
	if name != "" {
		return prefix + name
	}
	return name
}
