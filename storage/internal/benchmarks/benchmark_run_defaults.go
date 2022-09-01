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
	"runtime"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

func benchmarkRunWithDefaultClient(ctx context.Context, opts *benchmarkOptions, bucketName string) error {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	// Select randomized parameters
	_, doMD5, doCRC32C := randomOf3()
	objectSize := randomInt64(opts.minObjectSize, opts.maxObjectSize)

	// Create contents
	objectName, err := generateRandomFile(objectSize)
	if err != nil {
		return fmt.Errorf("generateRandomFile: %w", err)
	}

	// Select client
	api := opts.api
	if api == mixedAPIs {
		if randomBool() {
			api = xmlAPI
		} else {
			api = grpcAPI
		}
	}

	var client *storage.Client
	var readAPI, writeAPI benchmarkAPI

	switch api {
	case xmlAPI, jsonAPI:
		clientMu.Lock()
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		clientMu.Unlock()
		readAPI, writeAPI = xmlAPI, jsonAPI
	case grpcAPI:
		clientMu.Lock()
		os.Setenv("STORAGE_USE_GRPC", "true")
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
		os.Unsetenv("STORAGE_USE_GRPC")
		clientMu.Unlock()
		readAPI, writeAPI = grpcAPI, grpcAPI
	default:
		log.Fatalf("%s API not supported.\n", opts.api)
	}

	o := client.Bucket(bucketName).Object(objectName)
	defer o.Delete(context.Background())

	// Upload
	runtime.ReadMemStats(memStats)
	prevHeapAlloc := memStats.HeapAlloc
	prevMallocs := memStats.Mallocs

	start := time.Now()
	timeTaken, err := uploadBenchmarkDefaults(ctx, uploadOpts{
		o:         o,
		fileName:  objectName,
		chunkSize: 16777216,
		md5:       doMD5,
		crc32c:    doCRC32C,
	})
	runtime.ReadMemStats(memStats)
	results <- benchmarkResult{
		objectSize:    objectSize,
		appBufferSize: -1, // default
		chunkSize:     -1, // default
		crc32Enabled:  doCRC32C,
		md5Enabled:    doMD5,
		API:           writeAPI,
		elapsedTime:   timeTaken,
		completed:     err == nil,
		isRead:        false,
		heapSys:       memStats.HeapSys,
		heapAlloc:     memStats.HeapAlloc,
		stackInUse:    memStats.StackInuse,
		heapAllocDiff: memStats.HeapAlloc - prevHeapAlloc,
		mallocsDiff:   memStats.Mallocs - prevMallocs,
		start:         start,
	}
	os.Remove(objectName)
	// Do not attempt to read from a failed upload
	if err != nil {
		return fmt.Errorf("failed upload: %w", err)
	}

	// Wait for the object to be available.
	timedCtx, cancelTimedCtx := context.WithTimeout(ctx, time.Second*3)
	defer cancelTimedCtx()
	if _, err := o.Retryer(storage.WithPolicy(storage.RetryAlways)).Attrs(timedCtx); err != nil {
		return fmt.Errorf("object.Attrs: %w", err)
	}

	// Read.
	for i := 0; i < 3; i++ {
		runtime.ReadMemStats(memStats)
		prevHeapAlloc = memStats.HeapAlloc
		prevMallocs = memStats.Mallocs

		start := time.Now()
		timeTaken, err := downloadBenchmark(ctx, downloadOpts{
			o:          o,
			objectSize: objectSize,
		})
		runtime.ReadMemStats(memStats)
		results <- benchmarkResult{
			objectSize:    objectSize,
			appBufferSize: -1,
			crc32Enabled:  true,  // internally verified for us
			md5Enabled:    false, // we only need one integrity validation
			API:           readAPI,
			elapsedTime:   timeTaken,
			completed:     err == nil,
			isRead:        true,
			readIteration: i,
			heapSys:       memStats.HeapSys,
			heapAlloc:     memStats.HeapAlloc,
			stackInUse:    memStats.StackInuse,
			heapAllocDiff: memStats.HeapAlloc - prevHeapAlloc,
			mallocsDiff:   memStats.Mallocs - prevMallocs,
			start:         start,
		}
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
