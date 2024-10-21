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
func (c *Lister) sequentialListing(ctx context.Context) ([]*storage.ObjectAttrs, string, error) {
	var result []*storage.ObjectAttrs
	var objectsIterated int
	var lastToken string
	objectIterator := c.bucket.Objects(ctx, &c.query)
	objectIterator.PageInfo().Token = c.pageToken
	objectIterator.PageInfo().MaxSize = defaultPageSize

	for {
		objects, nextToken, pageSize, err := doSeqListing(objectIterator, c.skipDirectoryObjects)
		if err != nil {
			return nil, "", fmt.Errorf("failed while listing objects: %w", err)
		}
		result = append(result, objects...)
		lastToken = nextToken
		objectsIterated += pageSize
		if nextToken == "" || (c.batchSize > 0 && objectsIterated >= c.batchSize) {
			break
		}
		c.pageToken = nextToken
	}
	return result, lastToken, nil
}

func doSeqListing(objectIterator *storage.ObjectIterator, skipDirectoryObjects bool) (result []*storage.ObjectAttrs, token string, pageSize int, err error) {

	for {
		attrs, errObjectIterator := objectIterator.Next()
		// Stop listing when all the requested objects have been listed.
		if errObjectIterator == iterator.Done {
			break
		}
		if errObjectIterator != nil {
			err = fmt.Errorf("iterating through objects %w", errObjectIterator)
			return
		}
		pageSize++
		if !(skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			result = append(result, attrs)
		}
		if objectIterator.PageInfo().Remaining() == 0 {
			break
		}
	}
	token = objectIterator.PageInfo().Token
	return
}
