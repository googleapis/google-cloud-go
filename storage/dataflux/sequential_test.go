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

func TestDoSeqListingEmulated(t *testing.T) {
	transportClientTest(context.Background(), t, func(t *testing.T, ctx context.Context, project, bucket string, client *storage.Client) {

		attrs := &storage.BucketAttrs{
			Name: bucket,
		}
		bucketHandle := client.Bucket(bucket)
		if err := bucketHandle.Create(ctx, project, attrs); err != nil {
			t.Fatal(err)
		}
		wantObjects := 10
		if err := createObject(ctx, bucketHandle, wantObjects, "object/"); err != nil {
			t.Fatalf("unable to create objects: %v", err)
		}
		objectIterator := bucketHandle.Objects(ctx, nil)
		objects, nextToken, pageSize, err := doSeqListing(objectIterator, false)
		if err != nil {
			t.Fatalf("failed to call doSeqListing() : %v", err)
		}
		if len(objects) != wantObjects {
			t.Errorf("doSeqListing()  got %d objects, want %d objects ", len(objects), wantObjects)
		}
		if nextToken != "" {
			t.Errorf("doSequential() got %q token, want empty string ", nextToken)
		}
		if pageSize > seqDefaultPageSize {
			t.Errorf("doSequential() got %d pagesize, want less than %d pagesize", pageSize, seqDefaultPageSize)
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
		if err := createObject(ctx, bucketHandle, wantObjects, ""); err != nil {
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
			t.Fatalf("failed to call doSeqListing() : %v", err)
		}
		if len(objects) != wantObjects {
			t.Errorf("sequentialListing() expected to receive %d results, got %d results", len(objects), wantObjects)
		}
		if nextToken != "" {
			t.Errorf("sequentialListing() expected to receive empty token, got %q", nextToken)
		}
	})
}

func TestSequentialWithQueryEmulated(t *testing.T) {
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
				desc:                 "list all objects using worksteal",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: "", Delimiter: ""},
				want:                 21,
			},
			{
				desc:                 "skip /prefix object  with worksteal",
				skipDirectoryObjects: true,
				query:                storage.Query{Prefix: "", Delimiter: ""},
				want:                 20,
			},
			{
				desc:                 "objects in prefix/",
				skipDirectoryObjects: false,
				query:                storage.Query{Prefix: prefix, Delimiter: "/"},
				// List all objects in pre/, prefix: pre/, object: pre/.
				want: 12,
			},
			{
				desc:                 "objects in prefix/, skipDirectoryObjects ",
				skipDirectoryObjects: true,
				query:                storage.Query{Prefix: prefix, Delimiter: "/"},
				// List all objects in pre/, prefix: pre/ and skip object : pre/.
				want: 11,
			},
		}
		for _, tc := range testcase {
			t.Run(tc.desc, func(t *testing.T) {
				c := &Lister{
					method:               sequential,
					bucket:               bucketHandle,
					query:                tc.query,
					skipDirectoryObjects: tc.skipDirectoryObjects,
				}

				objects, nextToken, err := c.sequentialListing(ctx)
				if err != nil {
					t.Fatalf("failed to call doSeqListing() : %v", err)
				}
				if len(objects) != tc.want || nextToken != "" {
					t.Errorf("sequentialListing() got = (%d, %q), want (%d, empty string)", len(objects), nextToken, tc.want)
				}
				c.Close()
			})
		}
	})
}
