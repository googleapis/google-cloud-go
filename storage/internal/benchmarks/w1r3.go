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
		c := nonBenchmarkingClients.Get().(*storage.Client)
		o := c.Bucket(r.bucketName).Object(r.objectName).Retryer(storage.WithPolicy(storage.RetryAlways))
		o.Delete(context.Background())
		nonBenchmarkingClients.Put(c)
	}()

	// Upload

	// If the option is specified, run a garbage collector before collecting
	// memory statistics and starting the timer on the benchmark. This can be
	// used to compare between running each benchmark "on a blank slate" vs organically.
	forceGarbageCollection(opts.forceGC)

	client, err := getClient(ctx, opts, *r.writeResult)
	if err != nil {
		return fmt.Errorf("getClient: %w", err)
	}

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

	returnToPool(r.writeResult.params.api, client)
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
		forceGarbageCollection(opts.forceGC)

		client, err := getClient(ctx, opts, *r.readResults[i])
		if err != nil {
			return fmt.Errorf("getClient: %w", err)
		}

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

		returnToPool(r.readResults[i].params.api, client)
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

func returnToPool(api benchmarkAPI, client *storage.Client) {
	if api == grpcAPI {
		gRPCClients.Put(client)
	} else {
		httpClients.Put(client)
	}
}
