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

	"fmt"

	"cloud.google.com/go/storage"
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

func TestNextPageWithVersionEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name:              bucket,
			VersioningEnabled: true,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 1200
		objectName := "object1"
		if err := createObjectWithVersion(ctx, bucketHandle, wantObjects, objectName); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}

		// nextPage lists multiple versions of the same objects.
		query := storage.Query{Versions: true}
		firstpageResult, err := nextPage(ctx, nextPageOpts{
			bucketHandle: bucketHandle,
			query:        query,
		})
		if err != nil {
			t.Fatalf("failed to call nextPage() : %v", err)
		}

		if len(firstpageResult.items) != wsDefaultPageSize-1 || firstpageResult.doneListing || firstpageResult.nextStartRange != objectName {
			t.Errorf("nextPage() got (len of objects = %d, doneListing = %v, nextStartRange = %s) , want (len of objects = %d, doneListing = false, nextStartRange = %s)", len(firstpageResult.items), firstpageResult.doneListing, firstpageResult.nextStartRange, wsDefaultPageSize-1, objectName)
		}
		if firstpageResult.generation <= firstpageResult.items[len(firstpageResult.items)-1].Generation {
			t.Errorf("nextPage() generation value for next start object got %v, want greater than %v", firstpageResult.generation, firstpageResult.items[len(firstpageResult.items)-1].Generation)
		}
		// nextPage lists multiple versions of the same objects where generation value is greater than
		// the generation value of the last object listed.
		secondPageResult, err := nextPage(ctx, nextPageOpts{
			startRange:   firstpageResult.nextStartRange,
			bucketHandle: bucketHandle,
			query:        query,
			generation:   firstpageResult.generation,
		})
		if err != nil {
			t.Fatalf("failed to call nextPage() : %v", err)
		}
		wantSecondPageItems := wantObjects - len(firstpageResult.items)
		if len(secondPageResult.items) != wantSecondPageItems || !secondPageResult.doneListing || secondPageResult.nextStartRange != "" {
			t.Errorf("nextPage() got (len of objects = %d, doneListing = %v, nextStartRange = %s), want (len of objects = %d, doneListing = true, nextStartRange = empty string)", len(secondPageResult.items), secondPageResult.doneListing, secondPageResult.nextStartRange, wantSecondPageItems)
		}
	})
}

func TestNextPageWithoutGenerationEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name:              bucket,
			VersioningEnabled: true,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 1200
		objectName := "object1"
		if err := createObjectWithVersion(ctx, bucketHandle, wantObjects, objectName); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		query := storage.Query{Versions: true}
		// nextPage lists multiple versions of the same object when generation value is disabled.
		if err := query.SetAttrSelection([]string{"Name", "Size"}); err != nil {
			t.Fatalf("failed to call SetAttrSelection() : %v", err)
		}
		pageResult, err := nextPage(ctx, nextPageOpts{
			bucketHandle: bucketHandle,
			query:        query,
		})
		if err != nil {
			t.Fatalf("failed to call nextPage() : %v", err)
		}
		if len(pageResult.items) != wantObjects || !pageResult.doneListing || pageResult.nextStartRange != "" || pageResult.generation != 0 {
			t.Errorf("nextPage() got (len of objects = %d, doneListing = %v, nextStartRange = %s, generation = %v), want (len of objects = %d, doneListing = true, nextStartRange = empty string, generation = 0)", len(pageResult.items), pageResult.doneListing, pageResult.nextStartRange, pageResult.generation, wantObjects)
		}

	})
}

func TestNextPageStartEndOffsetEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		if err := createObject(ctx, bucketHandle, 5, "prefix/a"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		if err := createObject(ctx, bucketHandle, 5, "prefix/b"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		if err := createObject(ctx, bucketHandle, 5, "prefix/c"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		if err := createObjectWithVersion(ctx, bucketHandle, 1, "prefix/"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		query := storage.Query{Prefix: "prefix/"}

		testcases := []struct {
			desc                string
			start               string
			end                 string
			skipDirectoryObject bool
			wantObjects         int
		}{
			{
				desc:        "start offset is given ",
				start:       "b",
				wantObjects: 10,
			},
			{
				desc:        "end offset is given",
				end:         "c",
				wantObjects: 11,
			},
			{
				desc:                "end offset is given and skipDirectoryObject",
				end:                 "c",
				skipDirectoryObject: true,
				wantObjects:         10,
			},
			{
				desc:        "start and end offset are given",
				start:       "b",
				end:         "c",
				wantObjects: 5,
			},
		}
		for _, tc := range testcases {
			t.Run(tc.desc, func(t *testing.T) {
				pageResult, err := nextPage(ctx, nextPageOpts{
					startRange:           tc.start,
					endRange:             tc.end,
					bucketHandle:         bucketHandle,
					query:                query,
					skipDirectoryObjects: tc.skipDirectoryObject,
				})
				if err != nil {
					t.Fatalf("failed to call nextPage() : %v", err)
				}
				if len(pageResult.items) != tc.wantObjects {
					t.Errorf("nextPage() got = %d objects, want = %d objects", len(pageResult.items), tc.wantObjects)
				}
			})
		}
	})
}

func TestNextPageErrorEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		bucketHandle := client.Bucket(bucket)

		query := storage.Query{Versions: true}
		if _, err := nextPage(ctx, nextPageOpts{
			bucketHandle: bucketHandle,
			query:        query,
		}); err == nil {
			t.Errorf("nextPage() expected to fail as bucket does not exist")
		}
	})
}

func TestNextPageWithQueryEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, &storage.BucketAttrs{
			Name:              bucket,
			VersioningEnabled: true,
		}); err != nil {
			t.Fatal(err)
		}
		numObject := 10
		prefixa := "pre/a/"
		prefix := "pre/"
		// Create a "prefix/" object.
		if err := createObjectWithVersion(ctx, bucketHandle, 1, prefix); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		// Create 10 objects with "prefix/a/" prefix.
		if err := createObject(ctx, bucketHandle, numObject, prefixa); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		// Create 10 objects with "prefix/" prefix.
		if err := createObject(ctx, bucketHandle, numObject, prefix); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		testcase := []struct {
			desc                 string
			skipDirectoryObjects bool
			query                storage.Query
			want                 int
		}{
			{
				desc:                 "list all objects",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: "", Delimiter: ""},
				want:                 21,
			},
			{
				desc:                 "skip directory object",
				skipDirectoryObjects: true,
				query:                storage.Query{Prefix: "", Delimiter: ""},
				// Skip directory object "pre/"
				want: 20,
			},
			{
				desc:                 "objects in prefix and delimiter /",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: prefix, Delimiter: "/"},
				// List all objects in pre/, prefix: pre/, object: pre/.
				want: 12,
			},
			{
				desc:                 "objects in prefix",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: prefix, Delimiter: ""},
				want:                 21,
			},
		}
		for _, tc := range testcase {
			t.Run(tc.desc, func(t *testing.T) {
				pageResult, err := nextPage(ctx, nextPageOpts{
					bucketHandle:         bucketHandle,
					query:                tc.query,
					skipDirectoryObjects: tc.skipDirectoryObjects,
				})
				if err != nil {
					t.Fatalf("NextBatch() failed: %v", err)
				}
				if len(pageResult.items) != tc.want || !pageResult.doneListing {
					t.Errorf("NextBatch() got = (%d, %v), want (%d, true)", len(pageResult.items), pageResult.doneListing, tc.want)
				}
			})
		}
	})
}
