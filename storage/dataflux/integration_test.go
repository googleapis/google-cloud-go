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
	crand "crypto/rand"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

const (
	testPrefix      = "go-integration-test-tm"
	grpcTestPrefix  = "golang-grpc-test-tm"
	bucketExpiryAge = 24 * time.Hour
	minObjectSize   = 1024
	maxObjectSize   = 1024 * 1024
)

var (
	uidSpace = uid.NewSpace("", nil)
	//  These buckets are shared amongst download tests. They are created,
	// populated with objects and cleaned up in TestMain.
	httpTestBucket = downloadTestBucket{}
)

func TestMain(m *testing.M) {
	flag.Parse()
	fmt.Println("creating bucket")
	if err := httpTestBucket.Create(testPrefix); err != nil {
		log.Fatalf("test bucket creation failed: %v", err)
	}

	m.Run()

	if err := httpTestBucket.Cleanup(); err != nil {
		log.Printf("test bucket cleanup failed: %v", err)
	}

	if err := deleteExpiredBuckets(testPrefix); err != nil {
		log.Printf("expired http bucket cleanup failed: %v", err)
	}
}

// Lists the all the objects in the bucket.
func TestIntegration_NextBatch_All(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	c, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	in := &ListerInput{
		BucketName: httpTestBucket.bucket,
	}

	df := NewLister(c, in)
	defer df.Close()

	objects, err := df.NextBatch(ctx)
	if err != nil && err != iterator.Done {
		t.Errorf("df.NextBatch : %v", err)
	}

	if len(objects) != len(httpTestBucket.objects) {
		t.Errorf("expected to receive %d results, got %d results", len(httpTestBucket.objects), len(objects))
	}
}

func TestIntegration_NextBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	const landsatBucket = "gcp-public-data-landsat"
	const landsatPrefix = "LC08/01/001/00"
	wantObjects := 17225
	ctx := context.Background()
	c, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	in := &ListerInput{
		BucketName: landsatBucket,
		Query:      storage.Query{Prefix: landsatPrefix},
		BatchSize:  2000,
	}

	df := NewLister(c, in)
	defer df.Close()
	totalObjects := 0
	for {
		objects, err := df.NextBatch(ctx)
		if err != nil && err != iterator.Done {
			t.Errorf("df.NextBatch : %v", err)
		}
		totalObjects += len(objects)
		if err == iterator.Done {
			break
		}
		if len(objects) > in.BatchSize {
			t.Errorf("expected to receive %d objects in each batch, got %d objects in a batch", in.BatchSize, len(objects))
		}
	}
	if totalObjects != wantObjects {
		t.Errorf("expected to receive %d objects in results, got %d objects in results", wantObjects, totalObjects)

	}
}

// generateRandomFileInGCS uploads a file with random contents to GCS and returns
// the crc32c hash of the contents.
func generateFileInGCS(ctx context.Context, o *storage.ObjectHandle, size int64) (uint32, error) {
	w := o.Retryer(storage.WithPolicy(storage.RetryAlways)).NewWriter(ctx)

	crc32cHash := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	mw := io.MultiWriter(w, crc32cHash)

	if _, err := io.CopyN(mw, crand.Reader, size); err != nil {
		w.Close()
		return 0, err
	}
	return crc32cHash.Sum32(), w.Close()
}

// randomInt64 returns a value in the closed interval [min, max].
// That is, the endpoints are possible return values.
func randomInt64(min, max int64) int64 {
	if min > max {
		log.Fatalf("min cannot be larger than max; min: %d max: %d", min, max)
	}
	return rand.Int63n(max-min+1) + min
}

func deleteExpiredBuckets(prefix string) error {
	if testing.Short() {
		return nil
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}

	projectID := testutil.ProjID()
	it := client.Buckets(ctx, projectID)
	it.Prefix = prefix
	for {
		bktAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if time.Since(bktAttrs.Created) > bucketExpiryAge {
			log.Printf("deleting bucket %q, which is more than %s old", bktAttrs.Name, bucketExpiryAge)
			if err := killBucket(ctx, client, bktAttrs.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// killBucket deletes a bucket and all its objects.
func killBucket(ctx context.Context, client *storage.Client, bucketName string) error {
	bkt := client.Bucket(bucketName)
	// Bucket must be empty to delete.
	it := bkt.Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if err := bkt.Object(objAttrs.Name).Delete(ctx); err != nil {
			return fmt.Errorf("deleting %q: %v", bucketName+"/"+objAttrs.Name, err)
		}
	}
	// GCS is eventually consistent, so this delete may fail because the
	// replica still sees an object in the bucket. We log the error and expect
	// a later test run to delete the bucket.
	if err := bkt.Delete(ctx); err != nil {
		log.Printf("deleting %q: %v", bucketName, err)
	}
	return nil
}

// downloadTestBucket provides a bucket that can be reused for tests that only
// download from the bucket.
type downloadTestBucket struct {
	bucket        string
	objects       []string
	contentHashes map[string]uint32
	objectSizes   map[string]int64
}

// Create initializes the downloadTestBucket, creating a bucket and populating
// objects in it. All objects are of the same size but with different contents
// and can be mapped to their respective crc32c hash in contentHashes.
func (tb *downloadTestBucket) Create(prefix string) error {
	if testing.Short() {
		return nil
	}
	ctx := context.Background()

	tb.bucket = prefix + uidSpace.New()
	tb.objects = []string{
		"!#$&'()*+,:;=,?@,[] and spaces",
		"./obj",
		"obj1",
		"obj2",
		"dir/file",
		"dir/objA",
		"dir/objB",
		"dir/objC",
		"dir/nested/objA",
		"dir/nested/again/obj1",
		"anotherDir/objC",
	}
	tb.contentHashes = make(map[string]uint32)
	tb.objectSizes = make(map[string]int64)

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}
	defer client.Close()

	b := client.Bucket(tb.bucket)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		return fmt.Errorf("bucket(%q).Create: %v", tb.bucket, err)
	}

	// Write objects.
	for _, obj := range tb.objects {
		size := randomInt64(minObjectSize, maxObjectSize)
		crc, err := generateFileInGCS(ctx, b.Object(obj), size)
		if err != nil {
			return fmt.Errorf("generateFileInGCS: %v", err)
		}
		tb.contentHashes[obj] = crc
		tb.objectSizes[obj] = size
	}
	return nil
}

// Cleanup deletes the objects and bucket created in Create.
func (tb *downloadTestBucket) Cleanup() error {
	if testing.Short() {
		return nil
	}
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}
	defer client.Close()

	b := client.Bucket(tb.bucket)

	for _, obj := range tb.objects {
		if err := b.Object(obj).Delete(ctx); err != nil {
			return fmt.Errorf("object.Delete: %v", err)
		}
	}

	return b.Delete(ctx)
}
