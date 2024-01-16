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
	"slices"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
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
type continuousReads struct {
	opts          *benchmarkOptions
	bucketName    string
	directoryPath string
	numWorkers    int
	objects       chan string // list of objects to synchronize reads
	results       []time.Duration
	api           benchmarkAPI
	objectSize    int64 // every object must be of the same size
}

func (r *continuousReads) setup(ctx context.Context) error {
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
	r.api = api

	// Select object size
	objectSize := opts.objectSize
	if objectSize == 0 {
		objectSize = randomInt64(opts.minObjectSize, opts.maxObjectSize)
	}
	r.objectSize = objectSize

	// Make a temp dir for this run
	dir, err := os.MkdirTemp("", "benchmark-experiment-")
	if err != nil {
		return err
	}

	// Create contents
	r.directoryPath = dir

	objects, err := generateDirInGCS(ctx, path.Base(dir), objectSize)
	if err != nil {
		return err
	}
	r.objects = *objects

	return nil
}

// cleanup deletes objects on disk and in GCS. It does not accept a context as
// it should run to completion to ensure full clean up of resources.
func (r *continuousReads) cleanup() error {
	// Clean temp dir
	if err := os.RemoveAll(r.directoryPath); err != nil {
		return err
	}

	// Delete uploaded objects
	return deleteDirectoryFromGCS(r.bucketName, path.Base(r.directoryPath))
}

func (r *continuousReads) run(ctx context.Context) error {
	benchGroup, ctx := errgroup.WithContext(ctx)
	benchGroup.SetLimit(r.numWorkers)

	i := 0
	for {
		select {
		case <-ctx.Done():
			err := benchGroup.Wait()
			r.compileResults()
			if errorIsDeadLineExceeded(err) {
				return nil
			}
			return err
		default:
			benchGroup.Go(func() error {
				client := getClient(ctx, r.api)
				object := <-r.objects
				fmt.Println("hi ", i)
				i++

				var span trace.Span
				ctx, span = otel.GetTracerProvider().Tracer(tracerName).Start(ctx, "continuous_reads")
				span.SetAttributes(attribute.KeyValue{"workload", attribute.StringValue("9")},
					attribute.KeyValue{"api", attribute.StringValue(string(r.api))},
					attribute.KeyValue{"object_size", attribute.Int64Value(r.objectSize)})
				defer span.End()

				// Download full object (range is not supported)
				rangeStart := int64(0)
				rangeLength := int64(-1)

				timeTaken, err := downloadBenchmark(ctx, downloadOpts{
					client:              client,
					objectSize:          r.objectSize,
					bucket:              r.bucketName,
					object:              object,
					rangeStart:          rangeStart,
					rangeLength:         rangeLength,
					downloadToDirectory: r.directoryPath,
					timeout:             r.opts.timeoutPerOp,
				})

				if err != nil {
					r.results = append(r.results, timeTaken)
				}
				r.objects <- object

				return err
			})

		}
	}
}

func (r *continuousReads) compileResults() {
	slices.Sort(r.results)
	l := len(r.results)

	percentiles := map[string]time.Duration{
		"p50": r.results[l/2],
		"p90": r.results[l*9/10],
		"p99": r.results[l*99/100],
	}

	for name, value := range percentiles {
		result := benchmarkResult{
			objectSize:  r.objectSize,
			isRead:      true,
			elapsedTime: value,
			metricName:  name,
		}

		result.selectReadParams(*r.opts, r.api)
		result.params.rangeOffset = 0
		results <- result
	}
}
