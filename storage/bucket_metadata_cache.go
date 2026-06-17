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
	"sync"
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
	resource    string
	location    string
	placeholder bool
}

type bucketMetadataCache struct {
	mu      sync.Mutex
	muSF    singleflight.Group
	lru     *lruCache[string, bucketMetadata]
	fetcher bucketMetadataFetcher
	// fetchDone is a hook channel used to signal completion of fetchBackground in tests.
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
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.get(bucket)
}

func (c *bucketMetadataCache) put(bucket string, entry bucketMetadata) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Don't let a placeholder overwrite valid metadata
	if entry.placeholder {
		if curr, hit := c.lru.get(bucket); hit && !curr.placeholder {
			return
		}
	}
	c.lru.put(bucket, entry)
}

func (c *bucketMetadataCache) evict(bucket string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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

		c.muSF.Do(bucket, func() (interface{}, error) {
			// Perform the call with context.Background and a timeout so it runs outside request context lifetime but is bounded.
			ctx, cancel := context.WithTimeout(context.Background(), fetchBackgroundTimeout)
			defer cancel()
			resource, location, err := c.fetcher.fetchBucketMetadata(ctx, bucket)

			c.mu.Lock()
			defer c.mu.Unlock()

			curr, hit := c.lru.get(bucket)
			if err != nil {
				if errors.Is(err, ErrBucketNotExist) || isError(err, http.StatusNotFound, codes.NotFound) {
					c.lru.evict(bucket)
				} else if ShouldRetry(err) {
					if curr.placeholder {
						c.lru.evict(bucket)
					}
				} else {
					if !hit {
						c.lru.put(bucket, bucketMetadata{
							resource:    fmt.Sprintf("projects/_/buckets/%s", bucket),
							location:    "global",
							placeholder: true,
						})
					}
				}
				return nil, err
			}

			entry := bucketMetadata{
				resource: resource,
				location: location,
			}
			c.lru.put(bucket, entry)
			return entry, nil
		})
	}()
}

func isError(err error, httpErrorCode int, grpcErrorCode codes.Code) bool {
	var e *googleapi.Error
	if errors.As(err, &e) {
		if e.Code == httpErrorCode {
			return true
		}
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() == grpcErrorCode {
			return true
		}
	}
	return false
}
