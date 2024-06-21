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
	"math/rand"
	"os"
	"path"
	"runtime"
	"time"

	"cloud.google.com/go/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// w1r3 or "write one, read three" is a benchmark that uploads a randomly generated
// object once and then downloads in three times. The object is downloaded more
// than once to compare "cold" (just uploaded) vs. "hot" data.
//
// SET UP
//   - Initialize structs to populate with the benchmark results.
//   - Select a random API to use to upload and download the object, unless an
//     API is set in the command-line,
//   - Select a random size for the object that will be uploaded; this size will
//     be between two values configured in the command-line.
//   - Create an object of that size on disk, and fill with random contents.
//   - Select, for the upload and each download separately, the following parameters:
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
//   - Downloads the same object (3 times) sequentially with the same client,
//     capturing the elapsed time and taking memory snapshots before and after
//     each download.
type w1r3 struct {
	opts                   *benchmarkOptions
	bucketName, objectName string
	directoryPath          string
	objectPath             string
	writeResult            *benchmarkResult
	readResults            []*benchmarkResult
	isWarmup               bool // if true, results should not be recorded
}

func (r *w1r3) setup(ctx context.Context) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

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

	// Make a temp dir for this run
	dir, err := os.MkdirTemp("", "benchmark-experiment-")
	if err != nil {
		return err
	}

	// Create contents
	objectPath, err := generateRandomFile(dir, objectSize)
	if err != nil {
		return fmt.Errorf("generateRandomFile: %w", err)
	}

	r.directoryPath = dir
	r.objectPath = objectPath
	r.objectName = path.Base(objectPath)
	return nil
}

// cleanup deletes objects on disk and in GCS. It does not accept a context as
// it should run to completion to ensure full clean up of resources.
func (r *w1r3) cleanup() error {
	// Clean temp dir
	if err := os.RemoveAll(r.directoryPath); err != nil {
		return err
	}

	// Delete uploaded object
	c := nonBenchmarkingClients.Get()
	o := c.Bucket(r.bucketName).Object(r.objectName).Retryer(storage.WithPolicy(storage.RetryAlways))
	if err := o.Delete(context.Background()); err != nil {
		return err
	}

	return nil
}

func (r *w1r3) run(ctx context.Context) error {
	// Use the same client for write and reads as the api is the same
	client := getClient(ctx, r.readResults[0].params.api)

	var span trace.Span
	ctx, span = otel.GetTracerProvider().Tracer(tracerName).Start(ctx, "w1r3")
	span.SetAttributes(attribute.KeyValue{Key: "workload", Value: attribute.StringValue("w1r3")},
		attribute.KeyValue{Key: "api", Value: attribute.StringValue(string(r.opts.api))},
		attribute.KeyValue{Key: "object_size", Value: attribute.Int64Value(r.opts.objectSize)})
	defer span.End()

	// Upload
	err := runOneSample(r.writeResult, func() (time.Duration, error) {
		return uploadBenchmark(ctx, uploadOpts{
			client:              client,
			params:              r.writeResult.params,
			bucket:              r.bucketName,
			object:              r.objectName,
			useDefaultChunkSize: opts.minChunkSize == useDefault || opts.maxChunkSize == useDefault,
			objectPath:          r.objectPath,
			timeout:             r.opts.timeoutPerOp,
		})
	}, r.isWarmup)

	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	// Downloads
	for i := 0; i < 3; i++ {
		// Get full object if no rangeSize is specified
		rangeStart := int64(0)
		rangeLength := int64(-1)

		if opts.rangeSize > 0 {
			rangeStart = r.readResults[i].params.rangeOffset
			rangeLength = opts.rangeSize
		}
		r.readResults[i].readOffset = rangeStart

		err = runOneSample(r.readResults[i], func() (time.Duration, error) {
			return downloadBenchmark(ctx, downloadOpts{
				client:              client,
				objectSize:          r.readResults[i].objectSize,
				bucket:              r.bucketName,
				object:              r.objectName,
				rangeStart:          rangeStart,
				rangeLength:         rangeLength,
				downloadToDirectory: r.directoryPath,
				timeout:             r.opts.timeoutPerOp,
			})
		}, r.isWarmup)
		if err != nil {
			// We stop additional reads if one fails, as the iteration number would be off
			return fmt.Errorf("download[%d]: %v", i, err)
		}
	}

	return nil
}

func runOneSample(result *benchmarkResult, doOp func() (time.Duration, error), isWarmup bool) error {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	// If the option is specified, run the garbage collector before collecting
	// memory statistics and starting the timer on the benchmark. This can be
	// used to compare between running each benchmark "on a blank slate" vs organically.
	if opts.forceGC {
		runtime.GC()
	}

	runtime.ReadMemStats(memStats)
	result.startMem = *memStats
	result.start = time.Now()

	timeTaken, err := doOp()

	runtime.ReadMemStats(memStats)
	result.endMem = *memStats
	result.err = err
	result.elapsedTime = timeTaken

	if errorIsDeadLineExceeded(err) {
		result.timedOut = true
	}

	if !isWarmup {
		results <- *result
	}

	return err
}
