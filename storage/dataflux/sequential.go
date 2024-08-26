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
	// defaultPageSize specifies the number of object results to include on a single page.
	defaultPageSize = 5000
)

// sequentialListing performs a sequential listing on the given bucket.
// It returns a list of objects and the next token to use to continue listing.
// If the next token is empty, then listing is complete.
func sequentialListing(ctx context.Context, opts Lister) ([]*storage.ObjectAttrs, string, error) {
	var result []*storage.ObjectAttrs
	objectIterator := opts.bucket.Objects(ctx, &opts.query)
	// Number of pages to request from GCS pagination.
	numPageRequest := opts.batchSize / defaultPageSize
	var lastToken string
	i := 0
	for {
		// If page size is set, then stop listing after numPageRequest.
		if opts.batchSize > 0 && i >= numPageRequest {
			break
		}
		i++
		objects, nextToken, err := doListing(opts, objectIterator)
		if err != nil {
			return nil, "", fmt.Errorf("failed while listing objects: %w", err)
		}
		result = append(result, objects...)
		lastToken = nextToken
		if nextToken == "" {
			break
		}
		opts.pageToken = nextToken
	}
	return result, lastToken, nil
}

func doListing(opts Lister, objectIterator *storage.ObjectIterator) ([]*storage.ObjectAttrs, string, error) {

	var objects []*storage.ObjectAttrs

	pageInfo := objectIterator.PageInfo()
	pageInfo.MaxSize = defaultPageSize
	pageInfo.Token = opts.pageToken
	for {
		attrs, err := objectIterator.Next()
		// When last item for the assigned range is listed, then stop listing.
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("iterating through objects %w", err)
		}
		if !(opts.skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			objects = append(objects, attrs)
		}
		if defaultPageSize != 0 && (pageInfo.Remaining() == 0) {
			break
		}
	}
	nextToken := objectIterator.PageInfo().Token
	return objects, nextToken, nil
}
