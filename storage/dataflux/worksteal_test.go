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
	"testing"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
)

// createObject creates given number of objects in the given bucket.
func createObjectWithVersion(ctx context.Context, bucket *storage.BucketHandle, numObjects int, objectName string) error {
	for i := 0; i < numObjects; i++ {
		// Create a writer for the object
		wc := bucket.Object(objectName).NewWriter(ctx)

		// Close the writer to finalize the upload
		if err := wc.Close(); err != nil {
			return fmt.Errorf("failed to close writer for object %q: %v", objectName, err)
		}
	}
	return nil
}

func TestWorkstealListingEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		numObjects := 5000
		if err := createObject(ctx, bucketHandle, numObjects, ""); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		in := &ListerInput{
			BucketName:  bucket,
			Parallelism: 3,
		}
		c := NewLister(client, in)
		c.method = worksteal
		objects, err := c.workstealListing(ctx)
		if err != nil {
			t.Fatalf("failed to call workstealListing() : %v", err)
		}
		if len(objects) != numObjects {
			t.Errorf("workstealListing() expected to receive  %d results, got %d results", numObjects, len(objects))
		}
	})
}

func TestObjectListerEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name:              bucket,
			VersioningEnabled: true,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		numObjects := 1005
		if err := createObjectWithVersion(ctx, bucketHandle, numObjects, "object"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}

		c := NewLister(client, &ListerInput{
			BucketName: bucket,
			Query:      storage.Query{Versions: true},
		})
		w := &worker{
			id:     0,
			status: active,
			result: &listerResult{objects: []*storage.ObjectAttrs{}},
			lister: c,
		}
		doneListing, err := w.objectLister(ctx)

		if err != nil {
			t.Fatalf("objectLister() failed: %v", err)
		}
		if doneListing {
			t.Errorf("objectLister() doneListing got = %v, want = true", doneListing)
		}
		if len(w.result.objects) != wsDefaultPageSize-1 {
			t.Errorf("objectLister() got = %d objects, want = %d objects", len(w.result.objects), wsDefaultPageSize-1)
		}
		if w.generation == 0 {
			t.Errorf("objectLister() got = 0 generation, want greater than 0 generation")
		}
	},
	)
}

func TestObjectListerMultipleWorkersEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 200
		if err := createObject(ctx, bucketHandle, wantObjects, ""); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}

		c := NewLister(client, &ListerInput{
			BucketName: bucket,
		})
		result := &listerResult{objects: []*storage.ObjectAttrs{}}
		w1 := &worker{
			id:     0,
			status: active,
			result: result,
			lister: c,
		}
		w2 := &worker{
			id:     1,
			status: active,
			result: result,
			lister: c,
		}
		g, ctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			doneListing1, err := w1.objectLister(ctx)
			if err != nil {
				return fmt.Errorf("listing worker ID %d: %w", w1.id, err)
			}
			if !doneListing1 {
				t.Errorf("objectLister() doneListing1 got = %v, want = true", doneListing1)
			}
			return nil
		})
		g.Go(func() error {
			doneListing2, err := w2.objectLister(ctx)
			if err != nil {
				return fmt.Errorf("listing worker ID %d: %w", w2.id, err)
			}
			if !doneListing2 {
				t.Errorf("objectLister() doneListing1 got = %v, want = true", doneListing2)
			}
			return nil
		})

		if err := g.Wait(); err != nil {
			t.Fatalf("failed waiting for multiple workers : %v", err)
		}

		if len(result.objects) != wantObjects*2 {
			t.Errorf("objectLister() expected to receive  %d results, got %d results", wantObjects*2, len(result.objects))
		}
	},
	)
}

func TestObjectListerErrorEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		c := NewLister(client, &ListerInput{
			BucketName: bucket,
		})

		w := &worker{
			id:     0,
			status: idle,
			result: &listerResult{objects: []*storage.ObjectAttrs{}},
			lister: c,
		}

		if _, err := w.objectLister(ctx); err == nil {
			t.Errorf("objectLister() expected to fail as bucket does not exist")
		}
	})
}
