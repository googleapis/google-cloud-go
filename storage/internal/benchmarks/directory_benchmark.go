// Copyright 2023 Google LLC
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
//   - Select a random API to upload/download the directory, unless an API is
//     set in the command-line,
//   - Select a random size for the objects that will be uploaded; this size will
//     be between two values configured in the command-line.
//   - Create a directory with objects_per_directory number of files of that size.
//   - Select, for the upload and the download separately, the following parameters:
//   - the application buffer size set in the command-line
//   - the chunksize (only for uploads) set in the command-line,
//   - if the client library will perform CRC32C and/or MD5 hashes on the data.
//
// BENCHMARK
//
//   - Take a snapshot of the current memory stats.
//
//   - Walk through the entire directory and upload, capturing the time taken.
//     For each file:
//     1. Extract the object name from the path.
//     2. Grab a storage client from the pool.
//     3. Initiate a goroutine to upload the object. If it can run without
//     causing the number of goroutines to exceed numWorkers, it starts uploading
//     right away. Otherwise, it waits until less than numWorkers goroutines are
//     currently uploading a file in the directory to start.
//
//   - Take another snapshot of memory stats.
//
//   - After the entire directory is uploaded, it starts downloading the same
//     to a copy of the directory, tracking time to completion:
//
//   - Get an iterator over the bucket, using the directory path as a filter.
//
//   - Iterate over all objects, doing the same process as the upload:
//     1. Specify range size, if applicable.
//     2. Grab a storage client from the pool.
//     3. Initiate a goroutine to download the object using that client. As
//     with uploads, the number of concurrently running goroutines is
//     always kept at or below -workers.
//
//   - Another memory snapshot is taken.
type directoryBenchmark struct {
	opts                  *benchmarkOptions
	bucketName            string
	uploadDirectoryPath   string
	downloadDirectoryPath string
	writeResult           *benchmarkResult
	readResult            *benchmarkResult
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

	// Select write params
	r.writeResult = &benchmarkResult{}
	r.writeResult.selectWriteParams(*r.opts, api)

	// Select read params
	r.readResult = &benchmarkResult{isRead: true}
	r.readResult.selectReadParams(*r.opts, api)

	// Make a temp dir for this run
	dir, err := os.MkdirTemp("", "benchmark-experiment-")
	if err != nil {
		return err
	}

	// Create contents
	totalBytes, err := fillDirectory(dir)
	if err != nil {
		return err
	}
	r.writeResult.directorySize = totalBytes
	r.readResult.directorySize = totalBytes

	r.uploadDirectoryPath = dir
	r.downloadDirectoryPath = dir + "-copy"

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
	return deleteDirectoryFromGCS(r.bucketName, path.Base(r.uploadDirectoryPath))
}

func (r *directoryBenchmark) uploadDirectory(ctx context.Context, numWorkers int) (elapsedTime time.Duration, err error) {
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
			return nil // skip directories
		}

		objectName, err := filepath.Rel(path.Dir(r.uploadDirectoryPath), filePath)
		if err != nil {
			return err
		}

		client := getClient(ctx, r.writeResult.params.api)

		benchGroup.Go(func() error {
			// Do the upload
			_, err := uploadBenchmark(ctx, uploadOpts{
				client:              client,
				params:              r.writeResult.params,
				bucket:              r.bucketName,
				object:              objectName,
				useDefaultChunkSize: opts.minChunkSize == useDefault || opts.maxChunkSize == useDefault,
				objectPath:          filePath,
				timeout:             r.opts.timeoutPerOp,
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

func (r *directoryBenchmark) downloadDirectory(ctx context.Context, numWorkers int) (elapsedTime time.Duration, err error) {
	benchGroup, ctx := errgroup.WithContext(ctx)
	benchGroup.SetLimit(numWorkers)

	// Set timer
	start := time.Now()
	defer func() { elapsedTime = time.Since(start) }()

	client := nonBenchmarkingClients.Get()

	// Get an iterator to list all objects under the directory
	query := &storage.Query{
		Prefix: path.Base(r.uploadDirectoryPath),
	}
	err = query.SetAttrSelection([]string{"Name"})
	if err != nil {
		return
	}
	it := client.Bucket(r.bucketName).Objects(context.Background(), query)

	attrs, err := it.Next()

	for err == nil {
		object := attrs.Name

		// first, make sure all folders in path exist
		fullPathToObj := path.Join(r.downloadDirectoryPath, filepath.Dir(object))
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

		// get the expected size by checking the size in the upload directory
		var fs fs.FileInfo
		fs, err = os.Stat(path.Join(r.uploadDirectoryPath, filepath.Base(object)))
		if err != nil {
			return
		}
		objectSize := fs.Size()

		// download the object
		benchGroup.Go(func() error {
			_, err = downloadBenchmark(ctx, downloadOpts{
				client:              getClient(ctx, r.readResult.params.api),
				objectSize:          objectSize,
				bucket:              r.bucketName,
				object:              object,
				downloadToDirectory: r.downloadDirectoryPath,
				rangeStart:          rangeStart,
				rangeLength:         rangeLength,
				timeout:             r.opts.timeoutPerOp,
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
	// Upload
	err := runOneSample(r.writeResult, func() (time.Duration, error) {
		return r.uploadDirectory(ctx, r.numWorkers)
	}, false)

	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("upload directory: %w", err)
	}

	// Download
	err = runOneSample(r.readResult, func() (time.Duration, error) {
		return r.downloadDirectory(ctx, r.numWorkers)
	}, false)
	if err != nil {
		return fmt.Errorf("download directory: %w", err)
	}
	return nil
}
