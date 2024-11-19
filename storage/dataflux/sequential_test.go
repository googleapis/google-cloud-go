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
	"testing"

	"cloud.google.com/go/storage"
)

func TestListNextPageSequentiallyEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 10
		if err := createObject(ctx, bucketHandle, wantObjects); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		objectIterator := bucketHandle.Objects(ctx, nil)
		objects, nextToken, pageSize, err := listNextPageSequentially(objectIterator, false)
		if err != nil {
			t.Fatalf("failed to call listNextPageSequentially() : %v", err)
		}
		if len(objects) != wantObjects {
			t.Errorf("listNextPageSequentially() expected to receive %d results, got %d results", len(objects), wantObjects)
		}
		if nextToken != "" {
			t.Errorf("doSequential() expected to receive empty token, got %q", nextToken)
		}
		if pageSize > seqDefaultPageSize {
			t.Errorf("doSequential() expected to receive less than %d results, got %d results", seqDefaultPageSize, pageSize)
		}
	})
}

func TestSequentialListingEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 10
		if err := createObject(ctx, bucketHandle, wantObjects); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}

		c := &Lister{
			method: sequential,
			bucket: bucketHandle,
			query:  storage.Query{},
		}
		defer c.Close()
		objects, nextToken, err := c.sequentialListing(ctx)

		if err != nil {
			t.Fatalf("failed to call listNextPageSequentially() : %v", err)
		}
		if len(objects) != wantObjects {
			t.Errorf("sequentialListing() expected to receive %d results, got %d results", len(objects), wantObjects)
		}
		if nextToken != "" {
			t.Errorf("sequentialListing() expected to receive empty token, got %q", nextToken)
		}
	})
}
