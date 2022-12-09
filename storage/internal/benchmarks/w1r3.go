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
//   - the application buffer size, unless "default" is set in the command-line.
//     This size is quantized, and the quantum, as well as the range, can be
//     configured in the command-line;
//   - the chunksize (only for uploads); this size will be between two values
//     configured in the command-line,
//   - if the client library will perform CRC32C and/or MD5 hashes on the data.
//
// BENCHMARK
//   - Create a storage client or grab one from the pool. No client will ever process
//     more than one upload or download at a time.
//   - Take a snapshot of the current memory stats.
//   - Upload the object that was created in the set up, capturing the time taken.
//     This includes opening the file, writing the object, and verifying the hash
//     (if applicable).
//   - Take another snapshot of memory stats.
//   - Delete the file from the OS.
//   - Then the program downloads the same object (3 times), getting a different
//     client for each, capturing the elapsed time and taking memory snapshots
//     before and after each download.
//   - Delete the object and return the clients to the pool or close them.
type w1r3 struct {
	opts                   *benchmarkOptions
	bucketName, objectName string
	objectPath             string
	writeResult            *benchmarkResult
	readResults            []*benchmarkResult
}

func (r *w1r3) setup() error {
	objectSize := randomInt64(opts.minObjectSize, opts.maxObjectSize)
	r.writeResult = &benchmarkResult{objectSize: objectSize}
	r.readResults = []*benchmarkResult{{}, {}, {}}

	r.writeResult.selectParams(*r.opts)
	for i, res := range r.readResults {
		res.isRead = true
		res.readIteration = i
		res.objectSize = objectSize
		res.selectParams(*r.opts)
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
		nonBenchmarkingClients.Put(c)
	}()

	// Upload

	// If the option is specified, run the garbage collector before collecting
	// memory statistics and starting the timer on the benchmark. This can be
	// used to compare between running each benchmark "on a blank slate" vs organically.
	if opts.forceGC {
		runtime.GC()
	}

	client, close, err := getClient(ctx, opts, *r.writeResult)
	if err != nil {
		return fmt.Errorf("getClient: %w", err)
	}
	defer func() {
		if err := close(); err != nil {
			log.Printf("close client: %v", err)
		}
	}()

	runtime.ReadMemStats(memStats)
	r.writeResult.startMem = *memStats
	r.writeResult.start = time.Now()

	timeTaken, err := uploadBenchmark(ctx, uploadOpts{
		client:              client,
		params:              r.writeResult.params,
		bucket:              r.bucketName,
		object:              r.objectName,
		useDefaultChunkSize: opts.useDefaults,
		objectPath:          r.objectPath,
	})

	runtime.ReadMemStats(memStats)
	r.writeResult.endMem = *memStats
	r.writeResult.completed = err == nil
	r.writeResult.elapsedTime = timeTaken

	results <- *r.writeResult
	os.Remove(r.objectPath)

	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("failed upload: %w", err)
	}

	// Read
	for i := 0; i < 3; i++ {
		// If the option is specified, run a garbage collector before collecting
		// memory statistics and starting the timer on the benchmark. This can be
		// used to compare between running each benchmark "on a blank slate" vs organically.
		if opts.forceGC {
			runtime.GC()
		}

		client, close, err := getClient(ctx, opts, *r.readResults[i])
		if err != nil {
			return fmt.Errorf("getClient: %w", err)
		}
		defer func() {
			if err := close(); err != nil {
				log.Printf("close client: %v", err)
			}
		}()

		runtime.ReadMemStats(memStats)
		r.readResults[i].startMem = *memStats
		r.readResults[i].start = time.Now()

		timeTaken, err := downloadBenchmark(ctx, downloadOpts{
			client:     client,
			objectSize: r.readResults[i].objectSize,
			bucket:     r.bucketName,
			object:     r.objectName,
		})

		runtime.ReadMemStats(memStats)
		r.readResults[i].endMem = *memStats
		r.readResults[i].completed = err == nil
		r.readResults[i].elapsedTime = timeTaken

		results <- *r.readResults[i]

		// do not return error, continue to attempt to read
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			log.Printf("read error: %v", err)
		}
	}

	return nil
}
