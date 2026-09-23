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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type mockMetadataFetcher struct {
	callCount int32
	fetchFunc func(ctx context.Context, bucket string) (resource string, location string, err error)
}

func (m *mockMetadataFetcher) fetchBucketMetadata(ctx context.Context, bucket string) (resource string, location string, err error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, bucket)
	}
	return fmt.Sprintf("projects/p1/buckets/%s", bucket), "us-east1", nil
}

func TestCacheNilSafety(t *testing.T) {
	var cache *bucketMetadataCache = nil
	// Must not panic.
	if _, found := cache.get("b1"); found {
		t.Errorf("expected miss on nil cache")
	}
	cache.put("b1", bucketMetadata{})
	cache.evict("b1")
	cache.fetchBackground("b1")
}

func TestCacheConcurrentSafe(t *testing.T) {
	cache := newBucketMetadataCache(1000, nil)
	var wg sync.WaitGroup
	numWorkers := 10
	numIterations := 100

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				bucketName := fmt.Sprintf("bucket-%d-%d", workerID, j)
				cache.put(bucketName, bucketMetadata{resource: bucketName})
				if _, ok := cache.get(bucketName); !ok {
					t.Errorf("expected key %q to exist", bucketName)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestCacheFetchBackground(t *testing.T) {
	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			return "projects/p/buckets/foo", "us", nil
		},
	}

	cache := newBucketMetadataCache(10, fetcher)
	doneChan := make(chan struct{}, 1)
	cache.fetchDone = doneChan

	cache.fetchBackground("foo")

	select {
	case <-doneChan:
	case <-time.After(fetchBackgroundTimeout):
		t.Fatalf("timeout waiting for fetchBackground completion")
	}

	entry, found := cache.get("foo")
	if !found {
		t.Errorf("expected entry to be populated in cache")
	}
	if entry.resource != "projects/p/buckets/foo" || entry.location != "us" {
		t.Errorf("unexpected cache entry details: %+v", entry)
	}
}

func TestCacheFetchBackgroundSingleFlight(t *testing.T) {
	var callCount int32

	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			atomic.AddInt32(&callCount, 1)
			// Lock it inside fetch for 100ms to guarantee overlapping concurrent threads will join singleflight.
			time.Sleep(100 * time.Millisecond)
			return "projects/p/buckets/foo", "us", nil
		},
	}

	cache := newBucketMetadataCache(10, fetcher)
	doneChan := make(chan struct{}, 10)
	cache.fetchDone = doneChan

	// Fire 10 calls concurrently
	for i := 0; i < 10; i++ {
		go cache.fetchBackground("foo")
	}

	// Wait for all 10 calls to finish.
	for i := 0; i < 10; i++ {
		select {
		case <-doneChan:
		case <-time.After(fetchBackgroundTimeout):
			t.Fatalf("timeout waiting for fetchBackground %d completion", i)
		}
	}

	_, found := cache.get("foo")
	if !found {
		t.Fatalf("expected foo to be in cache")
	}

	// Deduplication verification: Only 1 call allowed to execute mock fetcher.
	calls := atomic.LoadInt32(&callCount)
	if calls != 1 {
		t.Errorf("expected exactly 1 fetch call via singleflight due to concurrent overlap, got %d", calls)
	}
}

func TestCacheFetchBackgroundErrorPlaceholder(t *testing.T) {
	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			return "", "", &googleapi.Error{Code: http.StatusForbidden}
		},
	}

	cache := newBucketMetadataCache(10, fetcher)
	doneChan := make(chan struct{}, 1)
	cache.fetchDone = doneChan

	cache.fetchBackground("failedBucket")

	select {
	case <-doneChan:
	case <-time.After(fetchBackgroundTimeout):
		t.Fatalf("timeout waiting for fetchBackground completion")
	}

	entry, found := cache.get("failedBucket")
	if !found {
		t.Fatalf("expected placeholder to be stored on failure")
	}

	expectedResource := "//storage.googleapis.com/projects/_/buckets/failedBucket"
	expectedLocation := "global"

	if entry.resource != expectedResource || entry.location != expectedLocation {
		t.Errorf("expected placeholder record {resource: %q, location: %q}, got %+v", expectedResource, expectedLocation, entry)
	}
}

func TestCacheFetchBackgroundTransientErrorEviction(t *testing.T) {
	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			return "", "", &googleapi.Error{Code: http.StatusInternalServerError}
		},
	}

	cache := newBucketMetadataCache(10, fetcher)
	doneChan := make(chan struct{}, 1)
	cache.fetchDone = doneChan

	// Populate cache with placeholder first (simulate startSpanWithBucket).
	cache.put("failedBucket", bucketMetadata{
		resource:    "//storage.googleapis.com/projects/_/buckets/failedBucket",
		location:    "global",
		placeholder: true,
	})

	cache.fetchBackground("failedBucket")

	select {
	case <-doneChan:
	case <-time.After(fetchBackgroundTimeout):
		t.Fatalf("timeout waiting for fetchBackground completion")
	}

	_, found := cache.get("failedBucket")
	if found {
		t.Fatalf("expected placeholder to be evicted from cache on transient failure")
	}
}

func TestOpportunisticCacheFill(t *testing.T) {
	ctx := context.Background()
	bucketName := "test-opportunistic-cache-fill"
	expectedLocation := "us-east1"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/storage/v1/b/%s", bucketName)
		if r.URL.Path != expectedPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"kind":          "storage#bucket",
			"name":          bucketName,
			"location":      expectedLocation,
			"projectNumber": "987654321",
			"locationType":  "region",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client, err := NewClient(ctx, option.WithEndpoint(ts.URL+"/storage/v1/"), option.WithoutAuthentication(), option.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Verify cache is empty initially.
	if _, found := client.bucketMetadataCache.get(bucketName); found {
		t.Fatalf("expected cache to be empty for %q", bucketName)
	}

	// Trigger GetBucket via Attrs.
	attrs, err := client.Bucket(bucketName).Attrs(ctx)
	if err != nil {
		t.Fatalf("Bucket.Attrs failed: %v", err)
	}

	if attrs.ProjectNumber != 987654321 {
		t.Errorf("got ProjectNumber %d, want %d", attrs.ProjectNumber, 987654321)
	}

	// Verify the cache got populated synchronously (opportunistic cache fill).
	entry, found := client.bucketMetadataCache.get(bucketName)
	if !found {
		t.Fatalf("expected cache to be populated synchronously by Attrs")
	}

	wantResource := "//storage.googleapis.com/projects/987654321/buckets/" + bucketName
	if entry.resource != wantResource {
		t.Errorf("got resource %q, want %q", entry.resource, wantResource)
	}
	if entry.location != "us-east1" {
		t.Errorf("got location %q, want %q", entry.location, "us-east1")
	}
}

// TestGetMetadataFromAttrsResourceNameFormat verifies that the resource name
// emitted in the gcp.resource.destination.id span attribute is a well-formed
// full resource name.
//
// Cloud Trace's App Hub extractor validates this attribute with
// IsWellFormedApiResourceName, which requires the "//{service}/{path}" form. A
// bare "projects/{p}/buckets/{b}" path is rejected and the span is silently
// dropped from App Hub enrichment with no error surfaced to the caller, so a
// regression here is invisible outside of an end-to-end test.
func TestGetMetadataFromAttrsResourceNameFormat(t *testing.T) {
	for _, tc := range []struct {
		name         string
		location     string
		locationType string
		project      string
		bucket       string
		wantResource string
		wantLocation string
	}{
		{
			name:         "region is lowercased",
			location:     "US-EAST1",
			locationType: "region",
			project:      "525947918171",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/525947918171/buckets/my-bucket",
			wantLocation: "us-east1",
		},
		{
			name:         "zone is lowercased",
			location:     "US-CENTRAL1-A",
			locationType: "zone",
			project:      "525947918171",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/525947918171/buckets/my-bucket",
			wantLocation: "us-central1-a",
		},
		{
			name:         "multi-region falls back to global",
			location:     "US",
			locationType: "multi-region",
			project:      "525947918171",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/525947918171/buckets/my-bucket",
			wantLocation: "global",
		},
		{
			name:         "project already in projects/ form (gRPC)",
			location:     "US-EAST1",
			locationType: "region",
			project:      "projects/525947918171",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/525947918171/buckets/my-bucket",
			wantLocation: "us-east1",
		},
		{
			name:         "unknown project number",
			location:     "",
			locationType: "",
			project:      "0",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/_/buckets/my-bucket",
			wantLocation: "global",
		},
		{
			name:         "empty project",
			location:     "",
			locationType: "",
			project:      "",
			bucket:       "my-bucket",
			wantResource: "//storage.googleapis.com/projects/_/buckets/my-bucket",
			wantLocation: "global",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotResource, gotLocation := getMetadataFromAttrs(tc.location, tc.locationType, tc.project, tc.bucket)
			if gotResource != tc.wantResource {
				t.Errorf("resource: got %q, want %q", gotResource, tc.wantResource)
			}
			if gotLocation != tc.wantLocation {
				t.Errorf("location: got %q, want %q", gotLocation, tc.wantLocation)
			}
			// Guard the exact shape the App Hub extractor requires, independent
			// of the expectations above.
			if !wellFormedResourceName.MatchString(gotResource) {
				t.Errorf("resource %q does not match %v; App Hub linkage will silently fail", gotResource, wellFormedResourceName)
			}
		})
	}
}

// wellFormedResourceName mirrors the constraint enforced by Cloud Trace's
// IsWellFormedApiResourceName for GCS bucket destinations.
var wellFormedResourceName = regexp.MustCompile(`^//storage\.googleapis\.com/projects/[^/]+/buckets/[^/]+$`)
