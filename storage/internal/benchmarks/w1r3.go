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
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"runtime"
	"time"

	"cloud.google.com/go/storage"
)

// w1r3 or "write one, read three" is a benchmark that uploads a randomly generated
// object once and then downloads in three times. The object is downloaded more
// than once to compare "cold" (just uploaded) vs. "hot" data.
//
// SET UP
//   - Initialize structs to populate with the benchmark results.
//   - Select a random size for the object that will be uploaded; this size will
//     be between two values configured in the command-line.
//   - Create an object of that size on disk, and fill with random contents.
//   - Select, for the upload and each download separately, the following parameters:
//   - a random API to use to upload/download the object, unless it is set in
//     the command-line,
//   - the application buffer size set in the command-line
//   - the chunksize (only for uploads) set in the command-line,
//   - if the client library will perform CRC32C and/or MD5 hashes on the data.
//
// BENCHMARK
//   - Grab a storage client from the pool.
//   - Take a snapshot of the current memory stats.
//   - Upload the object that was created in the set up, capturing the time taken.
//     This includes opening the file, writing the object, and verifying the hash
//     (if applicable).
//   - Take another snapshot of memory stats.
//   - Delete the file from the OS.
//   - Then the program downloads the same object (3 times) with the same client,
//     capturing the elapsed time and taking memory snapshots before and after
//     each download.
//   - Delete the object and return the client to the pool.
type w1r3 struct {
	opts                   *benchmarkOptions
	bucketName, objectName string
	objectPath             string
	writeResult            *benchmarkResult
	readResults            []*benchmarkResult
}

func (r *w1r3) setup() error {
	// Select API first as it will be the same for all writes/reads
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
	r.readResults = []*benchmarkResult{{objectSize: objectSize}, {objectSize: objectSize}, {objectSize: objectSize}}

	// Select write params
	r.writeResult.selectWriteParams(*r.opts, api)

	// Select read params
	firstRead := r.readResults[0]
	firstRead.isRead = true
	firstRead.readIteration = 0
	firstRead.selectReadParams(*r.opts, api)

	// We want the reads to be similar, so we copy the params from the first read
	for i, res := range r.readResults[1:] {
		res.isRead = true
		res.readIteration = i + 1
		res.copyParams(firstRead)
	}

	// Create contents
	objectPath, err := generateRandomFile(objectSize)
	if err != nil {
		return fmt.Errorf("generateRandomFile: %w", err)
	}

	r.objectPath = objectPath
	r.objectName = path.Base(objectPath)
	return nil
}

func (r *w1r3) run(ctx context.Context) error {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	defer func() {
		c := nonBenchmarkingClients.Get()
		o := c.Bucket(r.bucketName).Object(r.objectName).Retryer(storage.WithPolicy(storage.RetryAlways))
		o.Delete(context.Background())
	}()

	// Use the same client for write and reads as the api is the same
	client := getClient(ctx, r.writeResult.params.api)

	// Upload

	// If the option is specified, run the garbage collector before collecting
	// memory statistics and starting the timer on the benchmark. This can be
	// used to compare between running each benchmark "on a blank slate" vs organically.
	if opts.forceGC {
		runtime.GC()
	}

	runtime.ReadMemStats(memStats)
	r.writeResult.startMem = *memStats
	r.writeResult.start = time.Now()

	timeTaken, err := uploadBenchmark(ctx, uploadOpts{
		client:              client,
		params:              r.writeResult.params,
		bucket:              r.bucketName,
		object:              r.objectName,
		useDefaultChunkSize: opts.minChunkSize == useDefault || opts.maxChunkSize == useDefault,
		objectPath:          r.objectPath,
	})

	runtime.ReadMemStats(memStats)
	r.writeResult.endMem = *memStats
	r.writeResult.err = err
	r.writeResult.elapsedTime = timeTaken

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		r.writeResult.timedOut = true
	}

	results <- *r.writeResult
	os.Remove(r.objectPath)

	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("failed upload: %w", err)
	}

	// Read
	for i := 0; i < 3; i++ {
		// Get full object if no rangeSize is specified
		rangeStart := int64(0)
		rangeLength := int64(-1)

		if opts.rangeSize > 0 {
			rangeStart = r.readResults[i].params.rangeOffset
			rangeLength = opts.rangeSize
		}
		r.readResults[i].readOffset = rangeStart

		// If the option is specified, run a garbage collector before collecting
		// memory statistics and starting the timer on the benchmark. This can be
		// used to compare between running each benchmark "on a blank slate" vs organically.
		if opts.forceGC {
			runtime.GC()
		}

		runtime.ReadMemStats(memStats)
		r.readResults[i].startMem = *memStats
		r.readResults[i].start = time.Now()

		timeTaken, err := downloadBenchmark(ctx, downloadOpts{
			client:      client,
			objectSize:  r.readResults[i].objectSize,
			bucket:      r.bucketName,
			object:      r.objectName,
			rangeStart:  rangeStart,
			rangeLength: rangeLength,
		})

		runtime.ReadMemStats(memStats)
		r.readResults[i].endMem = *memStats
		r.readResults[i].err = err
		r.readResults[i].elapsedTime = timeTaken

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			r.readResults[i].timedOut = true
		}

		results <- *r.readResults[i]

		if err != nil {
			return fmt.Errorf("read failed: %v", err)
		}
	}

	return nil
}
