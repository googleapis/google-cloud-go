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
	defaultPageSize = 2000
)

// sequentialListing performs a sequential listing on the given bucket.
// It returns a list of objects and the next token to use to continue listing.
// If the next token is empty, then listing is complete.
func (c *Lister) sequentialListing(ctx context.Context) ([]*storage.ObjectAttrs, string, error) {
	var result []*storage.ObjectAttrs
	var objectsListed int
	var lastToken string
	objectIterator := c.bucket.Objects(ctx, &c.query)
	objectIterator.PageInfo().Token = c.pageToken
	objectIterator.PageInfo().MaxSize = defaultPageSize

	for {
		objects, nextToken, err := doListing(objectIterator, c.skipDirectoryObjects, &objectsListed)
		if err != nil {
			return nil, "", fmt.Errorf("failed while listing objects: %w", err)
		}
		result = append(result, objects...)
		lastToken = nextToken
		if nextToken == "" || (c.batchSize > 0 && objectsListed >= c.batchSize) {
			break
		}
		c.pageToken = nextToken
	}
	return result, lastToken, nil
}

func doListing(objectIterator *storage.ObjectIterator, skipDirectoryObjects bool, objectsListed *int) ([]*storage.ObjectAttrs, string, error) {

	var result []*storage.ObjectAttrs
	for {
		attrs, err := objectIterator.Next()
		*objectsListed++
		// Stop listing when all the requested objects have been listed.
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("iterating through objects %w", err)
		}
		if !(skipDirectoryObjects && strings.HasSuffix(attrs.Name, "/")) {
			result = append(result, attrs)
		}
		if objectIterator.PageInfo().Remaining() == 0 {
			break
		}
	}
	return result, objectIterator.PageInfo().Token, nil
}
