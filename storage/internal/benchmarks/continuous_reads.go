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

package main

import (
	"context"
	"math/rand"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

// continuousReads is a benchmark that continuously downloads the same set of
// objects from GCS until the timeout is reached.
//
// SET UP
//   - Initialize structs to populate with the benchmark results.
//   - Select a random API to use to upload and download the object, unless an
//     API is set in the command-line,
//   - Select a random size for the objects; this size will
//     be between two values configured in the command-line.
//   - Create a directory of objects of that size in GCS, uploading random
//     contents directly from memory. The number of objects created is equal to
//     the command-line configuration for the number of objects in a directory.
//
// BENCHMARK
//   - Grab a storage client from the pool.
//   - Check if the timeout is exceeded; if so, compile p50, p90 and p99 and return.
//   - Queue a goroutine that performs a download. This goroutine will run as
//     soon as there is an available worker. The number of workers that run
//     concurrently is set in the command-line.
//   - This goroutine chooses the object to download by receiving an object name
//     from the channel. At the end of the download, it will return that name to
//     the channel. That way, no goroutines are reading from the same object at
//     the same time.
//     Note that since the same objects are downloaded continuously, the downloads
//     will overwrite previous downloads of the same object.
//   - The time taken is saved in a slice that is used to compile the percentiles
//     once the timeout is exceeded.
type continuousReads struct {
	opts          *benchmarkOptions
	bucketName    string
	directoryPath string
	numWorkers    int
	objects       chan string // list of objects to synchronize reads
	results       []time.Duration
	resultMu      sync.Mutex
	api           benchmarkAPI
	objectSize    int64 // every object must be of the same size
}

func (r *continuousReads) setup(ctx context.Context) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil // don't error out here since we expect to finish with a cancellation
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

				var span trace.Span
				ctx, span = otel.GetTracerProvider().Tracer(tracerName).Start(ctx, "continuous_reads")
				span.SetAttributes(attribute.KeyValue{Key: "workload", Value: attribute.StringValue("9")},
					attribute.KeyValue{Key: "api", Value: attribute.StringValue(string(r.api))},
					attribute.KeyValue{Key: "object_size", Value: attribute.Int64Value(r.objectSize)})
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
					r.resultMu.Lock()
					r.results = append(r.results, timeTaken)
					r.resultMu.Unlock()
				}
				r.objects <- object

				return err
			})
		}
	}
}

func (r *continuousReads) compileResults() {
	// TO-DO: switch to slices.Sort(r.results) when Go<1.21 support is dropped
	sort.Slice(r.results, func(i, j int) bool {
		return r.results[i] < r.results[j]
	})
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
