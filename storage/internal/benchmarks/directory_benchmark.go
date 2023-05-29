// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
)

// directory benchmark is a benchmark that uploads a randomly generated
// directory once and then downloads it once. The objects in the directory may
// be uploaded/downloaded using parallelization.
//
// SET UP
//   - Initialize structs to populate with the benchmark results.
//   - Select a random API to use to upload/download the object, unless it is
//     set in the command-line,
//   - Select a random size for the objects that will be uploaded; this size will
//     be between two values configured in the command-line.
//   - Create a directory with 1 to 1000 files of that size and subdirectories.
//   - Select, for the upload and the download separately, the following parameters:
//   - the application buffer size set in the command-line
//   - the chunksize (only for uploads) set in the command-line,
//   - if the client library will perform CRC32C and/or MD5 hashes on the data.
//
// BENCHMARK
//   - Grab a storage client from the pool.
//   - Take a snapshot of the current memory stats.
//   - Upload the entire directory, capturing the time taken.
//   - Take another snapshot of memory stats.
//   - Download the same directory sequentially with the same client, capturing
//     the elapsed time and taking memory snapshots before and after the
//     download.
type directoryBenchmark struct {
	opts                  *benchmarkOptions
	bucketName            string
	uploadDirectoryPath   string
	downloadDirectoryPath string
	writeResult           *benchmarkResult
	readResult            *benchmarkResult
	bytesInDir            int64
	numWorkers            int
}

func (r *directoryBenchmark) setup(ctx context.Context) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Select API
	api := opts.api
	if api == mixedAPIs {
		switch rand.Intn(4) {
		case 0:
			api = xmlAPI
		case 1:
			api = jsonAPI
		case 2:
			api = grpcAPI
		case 3:
			api = directPath
		}
	}

	// Select object size
	objectSize := opts.objectSize
	if objectSize == 0 {
		objectSize = randomInt64(opts.minObjectSize, opts.maxObjectSize)
	}
	r.writeResult = &benchmarkResult{objectSize: objectSize}
	r.readResult = &benchmarkResult{objectSize: objectSize}

	// Select write params
	r.writeResult.selectWriteParams(*r.opts, api)

	// Select read params
	r.readResult.isRead = true
	r.readResult.readIteration = 0
	r.readResult.selectReadParams(*r.opts, api)

	// Make a temp dir for this run
	dir, err := os.MkdirTemp("", "benchmark-experiment-")
	if err != nil {
		return err
	}

	// Create contents
	totalBytes, err := fillDirectoryRandomly(dir, objectSize)
	if err != nil {
		return err
	}
	r.writeResult.directorySize = totalBytes
	r.readResult.directorySize = totalBytes

	r.uploadDirectoryPath = dir
	r.downloadDirectoryPath = dir + "-copy"
	r.bytesInDir = totalBytes
	return nil
}

// cleanup deletes objects on disk and in GCS. It does not accept a context as
// it should run to completion to ensure full clean up of resources.
func (r *directoryBenchmark) cleanup() error {
	// Clean temp dirs
	if err := os.RemoveAll(r.uploadDirectoryPath); err != nil {
		return err
	}
	if err := os.RemoveAll(r.downloadDirectoryPath); err != nil {
		return err
	}

	// Delete uploaded objects
	c := nonBenchmarkingClients.Get()
	// List objects under root and delete all
	root := path.Base(r.uploadDirectoryPath)
	it := c.Bucket(r.bucketName).Objects(context.Background(), &storage.Query{
		Prefix:     root,
		Projection: storage.ProjectionNoACL,
	})

	attrs, err := it.Next()

	for err == nil {
		o := c.Bucket(r.bucketName).Object(attrs.Name).Retryer(storage.WithPolicy(storage.RetryAlways))
		if err := o.Delete(context.Background()); err != nil {
			return err
		}
		attrs, err = it.Next()
	}

	if err != iterator.Done {
		return fmt.Errorf("Bucket.Objects: %w", err)
	}

	return nil
}

func (r *directoryBenchmark) uploadDirectory(ctx context.Context, client *storage.Client, numWorkers int) (elapsedTime time.Duration, err error) {
	benchGroup, ctx := errgroup.WithContext(ctx)
	benchGroup.SetLimit(numWorkers)

	// Set timer
	start := time.Now()
	defer func() { elapsedTime = time.Since(start) }()

	// Walk through directory while uploading files
	err = filepath.WalkDir(r.uploadDirectoryPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil // skip directories for now
		}

		objectName, err := filepath.Rel(path.Dir(r.uploadDirectoryPath), filePath)
		if err != nil {
			return err
		}

		benchGroup.Go(func() error {
			// Do the upload
			_, err := uploadBenchmark(ctx, uploadOpts{
				client:              client,
				params:              r.writeResult.params,
				bucket:              r.bucketName,
				object:              objectName,
				useDefaultChunkSize: opts.minChunkSize == useDefault || opts.maxChunkSize == useDefault,
				objectPath:          filePath,
			})
			return err
		})
		return nil
	})
	if err != nil {
		return
	}

	err = benchGroup.Wait()
	return
}

func (r *directoryBenchmark) downloadDirectory(ctx context.Context, client *storage.Client, numWorkers int) (elapsedTime time.Duration, err error) {
	benchGroup, ctx := errgroup.WithContext(ctx)
	benchGroup.SetLimit(numWorkers)

	// Set timer
	start := time.Now()
	defer func() { elapsedTime = time.Since(start) }()

	// Get an iterator to list all objects under the directory
	it := client.Bucket(r.bucketName).Objects(context.Background(), &storage.Query{
		Prefix:     path.Base(r.uploadDirectoryPath),
		Projection: storage.ProjectionNoACL,
	})

	attrs, err := it.Next()

	for err == nil {
		// first, make sure all folders in path exist
		fullPathToObj := path.Join(r.downloadDirectoryPath, filepath.Dir(attrs.Name))
		err = os.MkdirAll(fullPathToObj, fs.ModeDir|fs.ModePerm)
		if err != nil {
			return
		}

		// Get full object if no rangeSize is specified
		rangeStart := int64(0)
		rangeLength := int64(-1)

		if opts.rangeSize > 0 {
			rangeStart = r.readResult.params.rangeOffset
			rangeLength = opts.rangeSize
		}
		r.readResult.readOffset = rangeStart

		object := attrs.Name

		// download the object
		benchGroup.Go(func() error {
			_, err = downloadBenchmark(ctx, downloadOpts{
				client:              client,
				objectSize:          r.readResult.objectSize,
				bucket:              r.bucketName,
				object:              object,
				downloadToDirectory: r.downloadDirectoryPath,
				rangeStart:          rangeStart,
				rangeLength:         rangeLength,
			})
			return err
		})
		if err != nil {
			return
		}

		attrs, err = it.Next()
	}

	if err != iterator.Done {
		return
	}

	err = benchGroup.Wait()
	return
}

func (r *directoryBenchmark) run(ctx context.Context) error {
	// Use the same client for write and reads as the api is the same
	client := getClient(ctx, r.writeResult.params.api)

	// Upload
	err := runOneOp(ctx, r.writeResult, func() (time.Duration, error) {
		return r.uploadDirectory(ctx, client, r.numWorkers)
	})

	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("upload directory: %w", err)
	}

	// Download
	err = runOneOp(ctx, r.readResult, func() (time.Duration, error) {
		return r.downloadDirectory(ctx, client, r.numWorkers)
	})
	if err != nil {
		return fmt.Errorf("download directory: %w", err)
	}
	return nil
}
