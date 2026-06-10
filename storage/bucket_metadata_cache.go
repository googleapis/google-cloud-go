// Copyright 2026 Google LLC
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

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultBucketMetadataCacheLimit = 10000
	fetchBackgroundTimeout          = 10 * time.Second
)

type bucketMetadataFetcher interface {
	fetchBucketMetadata(ctx context.Context, bucket string) (resource string, location string, err error)
}

type bucketMetadata struct {
	resource string
	location string
}

type bucketMetadataCache struct {
	muSF    singleflight.Group
	lru     *lruCache[string, bucketMetadata]
	fetcher bucketMetadataFetcher
	// fetchDone is a hook channel used to signal completion of fetchBackground in tests
	fetchDone chan struct{}
}

func newBucketMetadataCache(limit int, fetcher bucketMetadataFetcher) *bucketMetadataCache {
	return &bucketMetadataCache{
		lru:     newLRUCache[string, bucketMetadata](limit),
		fetcher: fetcher,
	}
}

func (c *bucketMetadataCache) get(bucket string) (bucketMetadata, bool) {
	if c == nil {
		return bucketMetadata{}, false
	}
	return c.lru.get(bucket)
}

func (c *bucketMetadataCache) put(bucket string, entry bucketMetadata) {
	if c == nil {
		return
	}
	c.lru.put(bucket, entry)
}

func (c *bucketMetadataCache) evict(bucket string) {
	if c == nil {
		return
	}
	c.lru.evict(bucket)
}

func (c *bucketMetadataCache) fetchBackground(bucket string) {
	if c == nil || c.fetcher == nil {
		return
	}

	go func() {
		defer func() {
			if c.fetchDone != nil {
				select {
				case c.fetchDone <- struct{}{}:
				default:
				}
			}
		}()

		resVal, err, _ := c.muSF.Do(bucket, func() (interface{}, error) {
			// Perform the call with context.Background and a timeout so it runs outside request context lifetime but is bounded
			ctx, cancel := context.WithTimeout(context.Background(), fetchBackgroundTimeout)
			defer cancel()
			resource, location, err := c.fetcher.fetchBucketMetadata(ctx, bucket)
			if err != nil {
				return nil, err
			}
			return bucketMetadata{
				resource: resource,
				location: location,
			}, nil
		})

		var entry bucketMetadata
		if err != nil {
			if isForbiddenOrPermissionError(err) {
				entry = bucketMetadata{
					resource: fmt.Sprintf("projects/_/buckets/%s", bucket),
					location: "global",
				}
				c.put(bucket, entry)
			} else {
				c.evict(bucket)
			}
		} else {
			entry = resVal.(bucketMetadata)
			c.put(bucket, entry)
		}
	}()
}

func isForbiddenOrPermissionError(err error) bool {
	var e *googleapi.Error
	if errors.As(err, &e) {
		if e.Code == http.StatusForbidden {
			return true
		}
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() == codes.PermissionDenied {
			return true
		}
	}
	return false
}
