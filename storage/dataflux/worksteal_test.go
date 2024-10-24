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

func TestWorkstealListingEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		numObjects := 10
		if err := createObject(ctx, bucketHandle, numObjects); err != nil {
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
