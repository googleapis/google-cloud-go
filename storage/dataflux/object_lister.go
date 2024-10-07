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

	opts.query.StartOffset = addPrefix(opts.startRange, opts.query.Prefix)
	opts.query.EndOffset = addPrefix(opts.endRange, opts.query.Prefix)

	// objectLexLast is the lexicographically last item in the page.
	objectLexLast := ""
	// indexLexLast is the index of lexicographically last item in the page.
	indexLexLast := 0

	objectIterator := opts.bucketHandle.Objects(ctx, &opts.query)
	var items []*storage.ObjectAttrs
	// itemIndex is the index of the last item in the items list.
	itemIndex := -1
	// The Go Listing API does not expose a convenient interface to list multiple objects together,
	// thus we need to manually loop to construct a page of results using the iterator.
	for i := 0; i < defaultPageSize; i++ {
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
		}

		if err != nil {
			return nil, fmt.Errorf("iterating through objects: %w", err)
		}

		// Skip object versions already processed in the previous page to prevent duplicates.
		if opts.query.Versions && opts.query.StartOffset == attrs.Name && attrs.Generation < opts.generation {
			continue
		}

		if !(opts.skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			items = append(items, attrs)
			// Track index of the current item added to the items list.
			itemIndex++
		}

		// If name/prefix is greater than objectLexLast, update objectLexLast and indexLexLast.
		if objectLexLast <= attrs.Name || objectLexLast <= attrs.Prefix {
			objectLexLast = attrs.Prefix
			if objectLexLast <= attrs.Name {
				objectLexLast = attrs.Name
			}
			// If object is added to the items list, then update indexLexLast to current item index, else set indexLexLast to -1.
			// Setting indexLexLast to -1, indicates that the lexicographically last item is not added to items list.
			if !(opts.skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
				indexLexLast = itemIndex
			} else {
				indexLexLast = -1
			}
		}

		// If the "startoffset" value matches the name of the last object,
		// list another page to ensure the next NextStartRange is distinct from the current one.
		if opts.query.Versions && attrs.Generation != int64(0) && i == defaultPageSize-1 && opts.query.StartOffset == attrs.Name {
			i = -1
		}

		// When generation value is not set, list next page if the last item is a version of previous item to prevent duplicate listing.
		if opts.query.Versions && attrs.Generation == int64(0) && i == defaultPageSize-1 && indexLexLast > 0 && items[indexLexLast-1].Name == items[indexLexLast].Name {
			i = -1
		}
	}

	// Make last item as next start range.
	nextStartRange := strings.TrimPrefix(objectLexLast, opts.query.Prefix)
	// When the lexicographically last item is not added to items list due to skipDirectoryObjects,
	// then set doneListing return objectLexLast as next start range.
	if len(items) < 1 || indexLexLast == -1 {
		return &nextPageResult{
			items:          items,
			doneListing:    false,
			nextStartRange: nextStartRange,
		}, nil
	}

	generation := int64(0)

	// Remove lexicographically last item from the item list to avoid duplicate listing.
	// Store generation of the item to be removed from the list.
	if indexLexLast >= itemIndex {
		generation = items[itemIndex].Generation
		items = items[:len(items)-1]
	} else if indexLexLast >= 0 {
		generation = items[indexLexLast].Generation
		items = append(items[:indexLexLast], items[indexLexLast+1:]...)
	}

	// Check if is versions is false, generation is not required.
	if !opts.query.Versions {
		generation = int64(0)
	}

	return &nextPageResult{
		items:          items,
		doneListing:    false,
		nextStartRange: nextStartRange,
		generation:     generation,
	}, nil

	return nil, nil
}

func addPrefix(name, prefix string) string {
	if name != "" {
		return prefix + name
	}
	return name
}
