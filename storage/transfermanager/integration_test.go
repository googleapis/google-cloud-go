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
	"hash/crc32"
	"io"
	"sync"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/storage"
)

const (
	testPrefix     = "go-integration-test"
	grpcTestPrefix = "golang-grpc-test"
)

var uidSpace = uid.NewSpace("", &uid.Options{Short: true})

func TestIntegration_DownloaderSynchronous(t *testing.T) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	bucketName := testPrefix + uidSpace.New()
	b := client.Bucket(bucketName)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		t.Fatalf("bucket.Create: %v", err)
	}
	t.Cleanup(func() { b.Delete(ctx) })

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
		"obj/",
		"./obj",
		"!#$&'()*+,/:;=,?@,[] and spaces",
	}
	contentHashes := make(map[string]uint32)

	// Write objects.
	objectSize := int64(507)
	for _, obj := range objects {
		crc, err := generateFileInGCS(ctx, b.Object(obj), objectSize)
		if err != nil {
			t.Fatalf("generateFileInGCS: %v", err)
		}
		contentHashes[obj] = crc

		t.Cleanup(func() { b.Object(obj).Delete(ctx) })
	}

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
			Bucket:      bucketName,
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

	results := d.Results()

	for _, got := range results {
		writerIdx := objToWriter[got.Object]

		if got.Err != nil {
			t.Errorf("result.Err: %v", got.Err)
			continue
		}

		if want, got := contentHashes[got.Object], writers[writerIdx].crc32c; got != want {
			t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
		}

		if got.Attrs.Size != objectSize {
			t.Errorf("expected object size %d, got %d", objectSize, got.Attrs.Size)
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

	bucketName := testPrefix + uidSpace.New()
	b := client.Bucket(bucketName)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		t.Fatalf("bucket.Create: %v", err)
	}
	t.Cleanup(func() { b.Delete(ctx) })

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
	}
	contentHashes := make(map[string]uint32)

	// Write objects.
	objectSize := int64(507)
	for _, obj := range objects {
		crc, err := generateFileInGCS(ctx, b.Object(obj), objectSize)
		if err != nil {
			t.Fatalf("generateFileInGCS: %v", err)
		}
		contentHashes[obj] = crc

		t.Cleanup(func() { b.Object(obj).Delete(ctx) })
	}

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

	// Download several objects.
	writers := make([]*testWriter, len(objects))
	objToWriter := make(map[string]int) // so we can map the resulting content back to the correct object
	for i, obj := range objects {
		writers[i] = &testWriter{}
		objToWriter[obj] = i
		d.DownloadObject(ctx, &DownloadObjectInput{
			Bucket:      bucketName,
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

		if want, got := contentHashes[got.Object], writers[writerIdx].crc32c; got != want {
			t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
		}

		if got.Attrs.Size != objectSize {
			t.Errorf("expected object size %d, got %d", objectSize, got.Attrs.Size)
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

	bucketName := testPrefix + uidSpace.New()
	b := client.Bucket(bucketName)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		t.Fatalf("bucket.Create: %v", err)
	}
	t.Cleanup(func() { b.Delete(ctx) })

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
		"obj/",
		"./obj",
		"!#$&'()*+,/:;=,?@,[] and spaces",
	}
	contentHashes := make(map[string]uint32)

	// Write objects.
	objectSize := int64(507)
	for _, obj := range objects {
		crc, err := generateFileInGCS(ctx, b.Object(obj), objectSize)
		if err != nil {
			t.Fatalf("generateFileInGCS: %v", err)
		}
		contentHashes[obj] = crc

		t.Cleanup(func() { b.Object(obj).Delete(ctx) })
	}

	// Start a downloader. Give it a smaller amount of workers than objects, to
	// make sure we aren't blocking anywhere.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	numCallbacks := 0
	callbackMu := sync.Mutex{}

	// Download several objects.
	writers := make([]*testWriter, len(objects))
	for i, obj := range objects {
		i := i
		writers[i] = &testWriter{}
		d.DownloadObjectWithCallback(ctx, &DownloadObjectInput{
			Bucket:      bucketName,
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

			if want, got := contentHashes[got.Object], writers[i].crc32c; got != want {
				t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
			}

			if got.Attrs.Size != objectSize {
				t.Errorf("expected object size %d, got %d", objectSize, got.Attrs.Size)
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

	bucketName := testPrefix + uidSpace.New()
	b := client.Bucket(bucketName)
	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
		t.Fatalf("bucket.Create: %v", err)
	}
	t.Cleanup(func() { b.Delete(ctx) })

	// Populate object names and make a map for their contents.
	objects := []string{
		"obj1",
		"obj2",
		"obj/with/slashes",
		"obj/",
		"./obj",
		"!#$&'()*+,/:;=,?@,[] and spaces",
	}
	contentHashes := make(map[string]uint32)

	// Write objects.
	objectSize := int64(507)
	for _, obj := range objects {
		crc, err := generateFileInGCS(ctx, b.Object(obj), objectSize)
		if err != nil {
			t.Fatalf("generateFileInGCS: %v", err)
		}
		contentHashes[obj] = crc

		t.Cleanup(func() { b.Object(obj).Delete(ctx) })
	}

	// Start a downloader. Give it a smaller amount of workers than objects, to
	// make sure we aren't blocking anywhere.
	d, err := NewDownloader(client, WithWorkers(2))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	numCallbacks := 0
	callbackMu := sync.Mutex{}

	// Download a non-existent object.
	nonexistentObject := "not-written"
	w := &testWriter{}

	d.DownloadObjectWithCallback(ctx, &DownloadObjectInput{
		Bucket:      bucketName,
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
			Bucket:      bucketName,
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

			if want, got := contentHashes[got.Object], writers[i].crc32c; got != want {
				t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
			}

			if got.Attrs.Size != objectSize {
				t.Errorf("expected object size %d, got %d", objectSize, got.Attrs.Size)
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

// test ctx cancel and per op timeout

// func TestIntegration_DownloadShard(t *testing.T) {
// 	ctx := context.Background()
// 	client, err := storage.NewClient(ctx)
// 	if err != nil {
// 		t.Fatalf("NewClient: %v", err)
// 	}

// 	bucketName := testPrefix + uidSpace.New()
// 	b := client.Bucket(bucketName)
// 	if err := b.Create(ctx, testutil.ProjID(), nil); err != nil {
// 		t.Fatalf("bucket.Create: %v", err)
// 	}
// 	t.Cleanup(func() { b.Delete(ctx) })

// 	// Write object.
// 	objectName := "obj" + uidSpace.New()
// 	contentHashes := make(map[string]uint32)

// 	objectSize := int64(2 * 1024 * 1024)
// 	crc, err := generateFileInGCS(ctx, b.Object(objectName), objectSize)
// 	if err != nil {
// 		t.Fatalf("generateFileInGCS: %v", err)
// 	}

// 	t.Cleanup(func() { b.Object(objectName).Delete(ctx) })

// 	// Start a downloader.
// 	d, err := NewDownloader(client)
// 	if err != nil {
// 		t.Fatalf("NewDownloader: %v", err)
// 	}

// 	// Download the object normally ?

// 	for _, test := range []struct {
// 		desc string
// 		in   *DownloadObjectInput
// 		want *DownloadOutput
// 	}{
// 		{
// 			desc: "basic input",
// 			in: &DownloadObjectInput{
// 				Bucket: bucketName,
// 				Object: objectName,
// 				//Destination: w,
// 			},
// 			want: &DownloadOutput{
// 				Bucket: bucketName,
// 				Object: objectName,
// 				Attrs: &storage.ReaderObjectAttrs{
// 					Size:            objectSize,
// 					StartOffset:     0,
// 					ContentType:     "",
// 					ContentEncoding: "",
// 					CacheControl:    "",
// 					//LastModified:    time.Time{},
// 					Generation:     0,
// 					Metageneration: 0,
// 				},
// 			},
// 		},
// 		{
// 			desc: "range",
// 		},
// 		{
// 			desc: "incorrect generation",
// 		},
// 		{
// 			desc: "conditions: generationmatch",
// 		},
// 	} {
// 		t.Run(test.desc, func(t *testing.T) {
// 			w := &testWriter{}

// 			// Download a single object.

// 			// in.downloadShard()

// 			// it := d.Results()

// 			// got, err := it.Next()
// 			// if err != nil {
// 			// 	t.Fatalf("it.Next: %v", err)
// 			// }

// 			// // Close the writer so we can check the contents.
// 			// if err := w.Close(); err != nil {
// 			// 	t.Fatalf("testWriter.Close: %v", err)
// 			// }

// 			// if got.Err != nil {
// 			// 	t.Errorf("result.Err: %v", got.Err)
// 			// }

// 			// if got.Object != objects[0] || got.Bucket != bucketName {
// 			// 	t.Errorf("expected Bucket(%q).Object(%q), got %q.%q", bucketName, objects[0], got.Bucket, got.Object)
// 			// }

// 			// if want, got := contentHashes[objects[0]], w.crc32c; got != want {
// 			// 	t.Fatalf("content crc32c does not match; got: %v, expected: %v", got, want)
// 			// }

// 			// if got.Attrs.Size != objectSize {
// 			// 	t.Errorf("expected object size %d, got %d", objectSize, got.Attrs.Size)
// 			// }
// 		})
// 	}

// }

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
