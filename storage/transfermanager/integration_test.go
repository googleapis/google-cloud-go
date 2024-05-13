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

package transfermanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

const (
	testPrefix      = "go-integration-test-tm"
	grpcTestPrefix  = "golang-grpc-test-tm"
	bucketExpiryAge = 24 * time.Hour
)

var (
	uidSpace   = uid.NewSpace("", &uid.Options{Short: true})
	testBucket = downloadTestBucket{} // This bucket is shared amongst download tests. It is created with populated objects and cleaned up in TestMain.
)

func TestMain(m *testing.M) {
	if err := testBucket.Create(testPrefix); err != nil {
		log.Fatalf("test bucket creation failed: %v", err)
	}

	exit := m.Run()

	if err := testBucket.Cleanup(); err != nil {
		log.Printf("test bucket cleanup failed: %v", err)
	}
	if err := deleteExpiredBuckets(testPrefix); err != nil {
		// Don't fail the test if cleanup fails.
		log.Printf("Post-test cleanup failed: %v", err)
	}

	os.Exit(exit)
}

func TestIntegration_DownloaderSynchronous(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	objects := testBucket.objects

	// Start a downloader. Give it a smaller amount of workers than objects, to
	// make sure we aren't blocking anywhere.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	// Download several objects.
	writers := make([]*testWriter, len(objects))
	objToWriter := make(map[string]int) // so we can map the resulting content back to the correct object
	for i, obj := range objects {
		writers[i] = &testWriter{}
		objToWriter[obj] = i
		d.DownloadObject(ctx, &DownloadObjectInput{
			Bucket:      testBucket.bucket,
			Object:      obj,
			Destination: writers[i],
		})
	}

	if err := d.WaitAndClose(); err != nil {
		t.Fatalf("d.WaitAndClose: %v", err)
	}

	// Close the writers so we can check the contents. This should be fine,
	// since the downloads should all be complete after WaitAndClose.
	for i := range objects {
		if err := writers[i].Close(); err != nil {
			t.Fatalf("testWriter.Close: %v", err)
		}
	}

	// Check the results.
	results := d.Results()
	for _, got := range results {
		writerIdx := objToWriter[got.Object]

		if got.Err != nil {
			t.Errorf("result.Err: %v", got.Err)
			continue
		}

		if want, got := testBucket.contentHashes[got.Object], writers[writerIdx].crc32c; got != want {
			t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
		}

		if got.Attrs.Size != testBucket.objectSize {
			t.Errorf("expected object size %d, got %d", testBucket.objectSize, got.Attrs.Size)
		}
	}

	if len(results) != len(objects) {
		t.Errorf("expected to receive %d results, got %d results", len(objects), len(results))
	}
}

// Tests that a single error does not affect the rest of the downloads.
func TestIntegration_DownloaderErrorSync(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Make a copy of the objects slice.
	objects := make([]string, len(testBucket.objects))
	copy(objects, testBucket.objects)

	// Add another object to attempt to download; since it hasn't been written,
	// this one will fail. Append to the start so that it will (likely) be
	// attempted in the first 2 downloads.
	nonexistentObject := "not-written"
	objects = append([]string{nonexistentObject}, objects...)

	// Start a downloader.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	// Download objects.
	writers := make([]*testWriter, len(objects))
	objToWriter := make(map[string]int) // so we can map the resulting content back to the correct object
	for i, obj := range objects {
		writers[i] = &testWriter{}
		objToWriter[obj] = i
		d.DownloadObject(ctx, &DownloadObjectInput{
			Bucket:      testBucket.bucket,
			Object:      obj,
			Destination: writers[i],
		})
	}

	// WaitAndClose should return an error since one of our downloads should have failed.
	if err := d.WaitAndClose(); err == nil {
		t.Error("d.WaitAndClose should return an error, instead got nil")
	}

	// Close the writers so we can check the contents. This should be fine,
	// since the downloads should all be complete after WaitAndClose.
	for i := range objects {
		if err := writers[i].Close(); err != nil {
			t.Fatalf("testWriter.Close: %v", err)
		}
	}

	// Check the results.
	results := d.Results()
	for _, got := range results {
		writerIdx := objToWriter[got.Object]

		// Check that the nonexistent object returned an error.
		if got.Object == nonexistentObject {
			if got.Err != storage.ErrObjectNotExist {
				t.Errorf("Object(%q) should not exist, err found to be %v", got.Object, got.Err)
			}
			continue
		}

		// All other objects should complete correctly.
		if got.Err != nil {
			t.Errorf("result.Err: %v", got.Err)
			continue
		}

		if want, got := testBucket.contentHashes[got.Object], writers[writerIdx].crc32c; got != want {
			t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
		}

		if got.Attrs.Size != testBucket.objectSize {
			t.Errorf("expected object size %d, got %d", testBucket.objectSize, got.Attrs.Size)
		}
	}

	if len(results) != len(objects) {
		t.Errorf("expected to receive %d results, got %d results", len(objects), len(results))
	}
}

func TestIntegration_DownloaderAsynchronous(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	objects := testBucket.objects

	// Start a downloader. Give it a smaller amount of workers than objects, to
	// make sure we aren't blocking anywhere.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	numCallbacks := 0
	callbackMu := sync.Mutex{}

	// Download objects.
	writers := make([]*testWriter, len(objects))
	for i, obj := range objects {
		i := i
		writers[i] = &testWriter{}
		d.DownloadObjectWithCallback(ctx, &DownloadObjectInput{
			Bucket:      testBucket.bucket,
			Object:      obj,
			Destination: writers[i],
		}, func(got *DownloadOutput) {
			callbackMu.Lock()
			numCallbacks++
			callbackMu.Unlock()

			if got.Err != nil {
				t.Errorf("result.Err: %v", got.Err)
			}

			// Close the writer so we can check the contents.
			if err := writers[i].Close(); err != nil {
				t.Fatalf("testWriter.Close: %v", err)
			}

			if want, got := testBucket.contentHashes[got.Object], writers[i].crc32c; got != want {
				t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
			}

			if got.Attrs.Size != testBucket.objectSize {
				t.Errorf("expected object size %d, got %d", testBucket.objectSize, got.Attrs.Size)
			}
		})
	}

	if err := d.WaitAndClose(); err != nil {
		t.Fatalf("d.WaitAndClose: %v", err)
	}

	if numCallbacks != len(objects) {
		t.Errorf("expected to receive %d results, got %d callbacks", len(objects), numCallbacks)
	}
}

func TestIntegration_DownloaderErrorAsync(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	objects := testBucket.objects

	// Start a downloader. Give it a smaller amount of workers than objects, to
	// make sure we aren't blocking anywhere.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	// Keep track of the number of callbacks. Since the callbacks may happen
	// in parallel, we sync access to this variable.
	numCallbacks := 0
	callbackMu := sync.Mutex{}

	// Download a non-existent object.
	nonexistentObject := "not-written"
	w := &testWriter{}

	d.DownloadObjectWithCallback(ctx, &DownloadObjectInput{
		Bucket:      testBucket.bucket,
		Object:      nonexistentObject,
		Destination: w,
	}, func(got *DownloadOutput) {
		callbackMu.Lock()
		numCallbacks++
		callbackMu.Unlock()

		// Check that the nonexistent object returned an error.
		if got.Err != storage.ErrObjectNotExist {
			t.Errorf("Object(%q) should not exist, err found to be %v", got.Object, got.Err)
		}
	})

	// Download remaining objects.
	writers := make([]*testWriter, len(objects))
	for i, obj := range objects {
		i := i
		writers[i] = &testWriter{}
		d.DownloadObjectWithCallback(ctx, &DownloadObjectInput{
			Bucket:      testBucket.bucket,
			Object:      obj,
			Destination: writers[i],
		}, func(got *DownloadOutput) {
			callbackMu.Lock()
			numCallbacks++
			callbackMu.Unlock()

			if got.Err != nil {
				t.Errorf("result.Err: %v", got.Err)
			}

			// Close the writer so we can check the contents.
			if err := writers[i].Close(); err != nil {
				t.Fatalf("testWriter.Close: %v", err)
			}

			if want, got := testBucket.contentHashes[got.Object], writers[i].crc32c; got != want {
				t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
			}

			if got.Attrs.Size != testBucket.objectSize {
				t.Errorf("expected object size %d, got %d", testBucket.objectSize, got.Attrs.Size)
			}
		})
	}

	// WaitAndClose should return an error since one of our downloads should have failed.
	if err := d.WaitAndClose(); err == nil {
		t.Error("d.WaitAndClose should return an error, instead got nil")
	}

	// We expect num objects callbacks + 1 for the errored call.
	if numCallbacks != len(objects)+1 {
		t.Errorf("expected to receive %d results, got %d callbacks", len(objects)+1, numCallbacks)
	}
}

func TestIntegration_DownloaderTimeout(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Start a downloader.
	d, err := NewDownloader(client, WithPerOpTimeout(time.Nanosecond))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	// Download an object.
	d.DownloadObject(ctx, &DownloadObjectInput{
		Bucket:      testBucket.bucket,
		Object:      testBucket.objects[0],
		Destination: &testWriter{},
	})

	// WaitAndClose should return an error since the timeout is too short.
	if err := d.WaitAndClose(); err == nil {
		t.Error("d.WaitAndClose should return an error, instead got nil")
	}

	// Check the result.
	results := d.Results()
	got := results[0]

	// Check that the nonexistent object returned an error.
	if got.Err != context.DeadlineExceeded {
		t.Errorf("expected deadline exceeded error, got: %v", got.Err)
	}
}

func TestIntegration_DownloadShard(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Use an object from the test bucket.
	objectName := testBucket.objects[0]

	// Get expected Attrs.
	o := client.Bucket(testBucket.bucket).Object(objectName)
	r, err := o.NewReader(ctx)
	if err != nil {
		t.Fatalf("o.Attrs: %v", err)
	}

	incorrectGen := r.Attrs.Generation - 1

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	for _, test := range []struct {
		desc    string
		timeout time.Duration
		in      *DownloadObjectInput
		want    *DownloadOutput
	}{
		{
			desc: "basic input",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
			},
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Attrs:  &r.Attrs,
			},
		},
		{
			desc: "range",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Range: &DownloadRange{
					Offset: testBucket.objectSize - 5,
					Length: -1,
				},
			},
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Attrs: &storage.ReaderObjectAttrs{
					Size:            testBucket.objectSize,
					StartOffset:     testBucket.objectSize - 5,
					ContentType:     r.Attrs.ContentType,
					ContentEncoding: r.Attrs.ContentEncoding,
					CacheControl:    r.Attrs.CacheControl,
					LastModified:    r.Attrs.LastModified,
					Generation:      r.Attrs.Generation,
					Metageneration:  r.Attrs.Metageneration,
				},
			},
		},
		{
			desc: "incorrect generation",
			in: &DownloadObjectInput{
				Bucket:     testBucket.bucket,
				Object:     objectName,
				Generation: &incorrectGen,
			},
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Err:    storage.ErrObjectNotExist,
			},
		},
		{
			desc: "conditions: generationmatch",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Conditions: &storage.Conditions{
					GenerationMatch: r.Attrs.Generation,
				},
			},
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Attrs:  &r.Attrs,
			},
		},
		{
			desc: "conditions do not hold",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Conditions: &storage.Conditions{
					GenerationMatch: incorrectGen,
				},
			},
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Err:    &googleapi.Error{Code: 412},
			},
		},
		{
			desc: "timeout",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
			},
			timeout: time.Nanosecond,
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Err:    context.DeadlineExceeded,
			},
		},
		{
			desc: "cancelled ctx",
			in: &DownloadObjectInput{
				Bucket: testBucket.bucket,
				Object: objectName,
				ctx:    cancelledCtx,
			},
			timeout: time.Nanosecond,
			want: &DownloadOutput{
				Bucket: testBucket.bucket,
				Object: objectName,
				Err:    context.Canceled,
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			w := &testWriter{}

			test.in.Destination = w

			if test.in.ctx == nil {
				test.in.ctx = ctx
			}

			got := test.in.downloadShard(client, test.timeout)

			if got.Bucket != test.want.Bucket || got.Object != test.want.Object {
				t.Errorf("wanted bucket %q, object %q, got: %q, %q", test.want.Bucket, test.want.Object, got.Bucket, got.Object)
			}

			if diff := cmp.Diff(got.Attrs, test.want.Attrs); diff != "" {
				t.Errorf("diff got(-) vs. want(+): %v", diff)
			}

			if !errorIs(got.Err, test.want.Err) {
				t.Errorf("mismatching errors: got %v, want %v", got.Err, test.want.Err)
			}
		})
	}

}

// test ctx cancel and per op timeout

// errorIs is equivalent to errors.Is, except that it additionally will return
// true if err and targetErr are googleapi.Errors with identical error codes.
func errorIs(err error, targetErr error) bool {
	var e, targetE *googleapi.Error
	if errors.As(err, &e) && errors.As(targetErr, &targetE) {
		return e.Code == targetE.Code
	}

	// fallback to regular check
	return errors.Is(err, targetErr)
}

// generateRandomFileInGCS uploads a file with random contents to GCS and returns
// the crc32c hash of the contents.
func generateFileInGCS(ctx context.Context, o *storage.ObjectHandle, size int64) (uint32, error) {
	w := o.Retryer(storage.WithPolicy(storage.RetryAlways)).NewWriter(ctx)

	crc32cHash := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	mw := io.MultiWriter(w, crc32cHash)

	if _, err := io.CopyN(mw, rand.Reader, size); err != nil {
		w.Close()
		return 0, err
	}
	return crc32cHash.Sum32(), w.Close()
}

// TODO: once we provide a DownloaderBuffer that implements WriterAt in the
// library, we can use that instead.
type testWriter struct {
	b      []byte
	crc32c uint32
	bufs   [][]byte // temp bufs that will be joined on Close()
}

// Close must be called to finalize the buffer
func (tw *testWriter) Close() error {
	tw.b = bytes.Join(tw.bufs, nil)
	crc := crc32.New(crc32.MakeTable(crc32.Castagnoli))

	_, err := io.Copy(crc, bytes.NewReader(tw.b))
	tw.crc32c = crc.Sum32()
	return err
}

func (tw *testWriter) WriteAt(b []byte, offset int64) (n int, err error) {
	// TODO: use the offset. This is fine for now since reads are not yet sharded.
	copiedB := make([]byte, len(b))
	copy(copiedB, b)
	tw.bufs = append(tw.bufs, copiedB)

	return len(b), nil
}

func deleteExpiredBuckets(prefix string) error {
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
	objectSize    int64
}

// Create initializes the downloadTestBucket, creating a bucket and populating
// objects in it. All objects are of the same size but with different contents
// and can be mapped to their respective crc32c hash in contentHashes.
func (tb *downloadTestBucket) Create(prefix string) error {
	ctx := context.Background()

	tb.bucket = prefix + uidSpace.New()
	tb.objectSize = int64(507)
	tb.objects = []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
		"obj/",
		"./obj",
		"!#$&'()*+,/:;=,?@,[] and spaces",
	}
	tb.contentHashes = make(map[string]uint32)

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}
	defer client.Close()

	b := client.Bucket(tb.bucket)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		return fmt.Errorf("bucket.Create: %v", err)
	}

	// Write objects.
	for _, obj := range tb.objects {
		crc, err := generateFileInGCS(ctx, b.Object(obj), tb.objectSize)
		if err != nil {
			return fmt.Errorf("generateFileInGCS: %v", err)
		}
		tb.contentHashes[obj] = crc

	}
	return nil
}

// Cleanup deletes the objects and bucket created in Create.
func (tb *downloadTestBucket) Cleanup() error {
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
