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
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

const (
	// seqDefaultPageSize specifies the number of object results to include on a single page for sequential listing.
	seqDefaultPageSize = 5000
)

// sequentialListing performs a sequential listing on the given bucket.
// It returns a list of objects and the next token to use to continue listing.
// If the next token is empty, then listing is complete.
func (c *Lister) sequentialListing(ctx context.Context) ([]*storage.ObjectAttrs, string, error) {
	var results []*storage.ObjectAttrs
	var objectsIterated int
	var lastToken string
	objectIterator := c.bucket.Objects(ctx, &c.query)
	objectIterator.PageInfo().Token = c.pageToken
	objectIterator.PageInfo().MaxSize = seqDefaultPageSize

	for {
		objects, nextToken, pageSize, err := listNextPageSequentially(objectIterator, c.skipDirectoryObjects)
		if err != nil {
			return nil, "", err
		}
		results = append(results, objects...)
		lastToken = nextToken
		objectsIterated += pageSize
		if nextToken == "" || (c.batchSize > 0 && objectsIterated >= c.batchSize) {
			break
		}
		c.pageToken = nextToken
	}
	return results, lastToken, nil
}

// listNextPageSequentially returns all objects fetched by GCS API in a single request
// and a token to list next page of objects and number of objects iterated(even
// if not in results). This function will make at most one network call to GCS
// and will exhaust all objects currently held in the iterator
func listNextPageSequentially(objectIterator *storage.ObjectIterator, skipDirectoryObjects bool) (results []*storage.ObjectAttrs, token string, pageSize int, err error) {

	for {
		attrs, errObjectIterator := objectIterator.Next()
		// Stop listing when all the requested objects have been listed.
		if errObjectIterator == iterator.Done {
			break
		}
		if errObjectIterator != nil {
			err = errObjectIterator
			return
		}
		// pageSize tracks the number of objects iterated through
		pageSize++
		if !(skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			results = append(results, attrs)
		}
		if objectIterator.PageInfo().Remaining() == 0 {
			break
		}
	}
	token = objectIterator.PageInfo().Token
	return
}
