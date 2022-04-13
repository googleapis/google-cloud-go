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
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
)

const codeVersion = 3.1 // to keep track of which version of the code a benchmark ran on

var opts = &benchmarkOptions{}
var projectID, credentialsFile, outputFile string

var results chan benchmarkResult
var workers chan struct{}

type benchmarkOptions struct {
	api           benchmarkAPI
	region        string
	timeout       time.Duration
	minObjectSize int64
	maxObjectSize int64
	readQuantum   int
	writeQuantum  int
	minWriteSize  int
	maxWriteSize  int
	minReadSize   int
	maxReadSize   int
	minSamples    int
	maxSamples    int
	minChunkSize  int64
	maxChunkSize  int64
	forceGC       bool
	numWorkers    int
	connPoolSize  int
}

func Init() {
	flag.StringVar((*string)(&opts.api), "api", string(mixed), "api used to upload/download objects; JSON or XML values will use JSON to uplaod and XML to download")
	flag.StringVar(&opts.region, "r", "US-WEST1", "region")
	flag.DurationVar(&opts.timeout, "t", time.Hour, "timeout")
	minSize := flag.Int64("min_size", 512, "minimum object size in kib")
	maxSize := flag.Int64("max_size", 2097152, "maximum object size in kib")
	flag.IntVar(&opts.minWriteSize, "min_w_size", 4000, "minimum write size in bytes")
	flag.IntVar(&opts.maxWriteSize, "max_w_size", 4000, "maximum write size in bytes")
	flag.IntVar(&opts.minReadSize, "min_r_size", 4000, "minimum read size in bytes")
	flag.IntVar(&opts.maxReadSize, "max_r_size", 4000, "maximum read size in bytes")
	flag.IntVar(&opts.readQuantum, "q_read", 1, "read quantum for app buffer size")
	flag.IntVar(&opts.writeQuantum, "q_write", 1, "write quantum for app buffer size")
	minChunkSize := flag.Int64("min_cs", 16*1024, "min chunksize in kib")
	maxChunkSize := flag.Int64("max_cs", 16*1024, "max chunksize in kib")
	flag.IntVar(&opts.minSamples, "min_samples", 10, "minimum number of objects to upload")
	flag.IntVar(&opts.maxSamples, "max_samples", 10000, "maximum number of objects to upload")
	flag.StringVar(&outputFile, "o", "res.csv", "file to output results to")
	flag.BoolVar(&opts.forceGC, "gc_f", false, "force garbage collection at the beginning of each upload")
	flag.IntVar(&opts.numWorkers, "workers", 16, "number of concurrent workers")
	flag.IntVar(&opts.connPoolSize, "conn_pool", 4, "GRPC connection pool size")

	flag.StringVar(&projectID, "p", projectID, "projectID")
	flag.StringVar(&credentialsFile, "creds", credentialsFile, "path to credentials file")

	flag.Parse()

	opts.minObjectSize = (*minSize) * 1024
	opts.maxObjectSize = (*maxSize) * 1024
	opts.minChunkSize = *minChunkSize * 1024
	opts.maxChunkSize = *maxChunkSize * 1024

	if len(projectID) < 1 {
		fmt.Println("Must set a project ID. Use flag -p to specify it.")
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())
}

func main() {
	start := time.Now()
	fmt.Printf("Benchmarking started: %s\n", start)
	Init()

	bucketName := randomName(bucketPrefix)
	cleanUp := createBenchmarkBucket(bucketName, opts)
	defer cleanUp()

	fmt.Printf("Results file: %s\n", outputFile)
	fmt.Printf("Benchmarking bucket: %s\n", bucketName)
	fmt.Printf("Benchmarking options: %+v\n", opts)
	fmt.Printf("Code version: %0.2f\n", codeVersion)

	// Create output file
	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", outputFile, err)
	}
	defer file.Close()

	recordResultGroup, _ := errgroup.WithContext(context.Background())
	startRecordingResults(file, recordResultGroup)

	var benchmarkRunGroup sync.WaitGroup
	workers = make(chan struct{}, opts.numWorkers)

	for i := 0; i < opts.maxSamples && (i < opts.minSamples || time.Since(start) < opts.timeout); i++ {
		benchmarkRunGroup.Add(1)

		go func() {
			defer benchmarkRunGroup.Done()
			workers <- struct{}{}        // use up a worker; if this blocks, numWorkers are already active
			defer func() { <-workers }() // free up a worker

			if err := benchmarkRun(context.Background(), opts, bucketName); err != nil {
				log.Printf("run failed: %v", err)
			}
		}()
	}
	benchmarkRunGroup.Wait()
	close(results)

	if err := recordResultGroup.Wait(); err != nil {
		fmt.Printf("recordResultGroup error: %v", err)
	}

	fmt.Printf("\nTotal time running: %s\n", time.Since(start).String())
}

type benchmarkResult struct {
	objectSize    int64
	appBufferSize int
	chunkSize     int
	crc32Enabled  bool
	md5Enabled    bool
	API           benchmarkAPI
	elapsedTime   time.Duration
	completed     bool
	isRead        bool
	readIteration int
	heapSys       uint64
	heapAlloc     uint64
	stackInUse    uint64
	heapAllocDiff uint64
	mallocsDiff   uint64
	start         time.Time
}

func benchmarkRun(ctx context.Context, opts *benchmarkOptions, bucketName string) error {
	var memStats *runtime.MemStats = &runtime.MemStats{}

	doMD5 := randomBool()
	doCRC32C := randomBool()

	objectName := randomName("obj")
	objectSize := randomInt64(opts.minObjectSize, opts.maxObjectSize)

	appWriteBufferSize := opts.writeQuantum * randomInt(opts.minWriteSize/opts.writeQuantum, opts.maxWriteSize/opts.writeQuantum)
	appReadBufferSize := opts.readQuantum * randomInt(opts.minReadSize/opts.readQuantum, opts.maxReadSize/opts.readQuantum)

	writeChunkSize := randomInt64(opts.minChunkSize, opts.maxChunkSize)

	// Select client
	client, readAPI, writeAPI, err := initializeClient(ctx, opts.api, appWriteBufferSize, appReadBufferSize, opts.connPoolSize)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}

	// Create contents to write
	err = generateRandomFile(objectName, objectSize)
	if err != nil {
		return fmt.Errorf("generateRandomFile: %v", err)
	}

	o := client.Bucket(bucketName).Object(objectName)
	defer o.Delete(ctx)

	// Upload
	if opts.forceGC {
		runtime.GC()
		// debug.FreeOSMemory()
	}
	runtime.ReadMemStats(memStats)
	prevHeapAlloc := memStats.HeapAlloc
	prevMallocs := memStats.Mallocs
	start := time.Now()

	timeTaken, err := uploadBenchmark(ctx, uploadOpts{
		o:         o,
		fileName:  objectName,
		chunkSize: int(writeChunkSize),
		md5:       doMD5,
		crc32c:    doCRC32C,
	})
	runtime.ReadMemStats(memStats)
	results <- benchmarkResult{
		objectSize:    objectSize,
		appBufferSize: appWriteBufferSize,
		chunkSize:     int(writeChunkSize),
		crc32Enabled:  doCRC32C,
		md5Enabled:    doMD5,
		API:           readAPI,
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
		return fmt.Errorf("failed upload: %v", err)
	}

	// Wait for the object to be available.
	timedCtx, cancelTimedCtx := context.WithTimeout(ctx, time.Second*10)
	defer cancelTimedCtx()
	for {
		if _, err := o.Attrs(timedCtx); err != nil {
			// keep trying if the object is not found, otherwise return err
			if !errors.Is(err, storage.ErrObjectNotExist) {
				return fmt.Errorf("object.Attrs: %v", err)
			}

		} else {
			break
		}
		// give some time before checking again
		time.Sleep(time.Millisecond * 100)
	}

	// Read.
	for i := 0; i < 3; i++ {
		if opts.forceGC {
			runtime.GC()
			// debug.FreeOSMemory()
		}
		runtime.ReadMemStats(memStats)
		prevHeapAlloc = memStats.HeapAlloc
		prevMallocs = memStats.Mallocs

		start := time.Now()
		timeTaken, err := downloadBenchmark(ctx, downloadOpts{
			o:          o,
			objectSize: objectSize,
			md5:        doMD5,
		})
		runtime.ReadMemStats(memStats)
		results <- benchmarkResult{
			objectSize:    objectSize,
			appBufferSize: int(appReadBufferSize),
			crc32Enabled:  true, // internally verified for us
			md5Enabled:    doMD5,
			API:           writeAPI,
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
			log.Printf("read error: %v", err)
		}
	}

	return nil
}

type benchmarkAPI string

const (
	json  benchmarkAPI = "JSON"
	xml   benchmarkAPI = "XML"
	grpc  benchmarkAPI = "GRPC"
	mixed benchmarkAPI = "MIXED"
)

var csvHeaders = []string{
	"Op", "ObjectSize", "AppBufferSize", "LibBufferSize",
	"Crc32cEnabled", "MD5Enabled", "ApiName",
	"ElapsedTimeUs", "CpuTimeUs", "Status",
	"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
	"StartTime", "EndTime", "NumWorkers",
	"CodeVersion",
}

// converts result to csv writing format (ie. a slice of strings)
func (br benchmarkResult) csv() []string {
	op := "WRITE"
	if br.isRead {
		op = fmt.Sprintf("READ[%d]", br.readIteration)
	}
	status := "[OK]"
	if !br.completed {
		status = "[FAIL]"
	}

	return []string{
		op,
		strconv.FormatInt(br.objectSize, 10),
		strconv.Itoa(br.appBufferSize),
		strconv.Itoa(br.chunkSize),
		strconv.FormatBool(br.crc32Enabled),
		strconv.FormatBool(br.md5Enabled),
		string(br.API),
		strconv.FormatInt(br.elapsedTime.Microseconds(), 10),
		"-1", // TODO: record cpu time
		status,
		strconv.FormatUint(br.heapSys, 10),
		strconv.FormatUint(br.heapAlloc, 10),
		strconv.FormatUint(br.stackInUse, 10),
		strconv.FormatUint(br.heapAllocDiff, 10),
		strconv.FormatUint(br.mallocsDiff, 10),
		strconv.FormatInt(br.start.UnixNano(), 10),
		strconv.FormatInt(br.start.Add(br.elapsedTime).UnixNano(), 10),
		strconv.Itoa(opts.numWorkers),
		fmt.Sprintf("%.2f", codeVersion),
	}
}

func startRecordingResults(f *os.File, g *errgroup.Group) *csv.Writer {
	// buffer channel so we don't block while printing results
	results = make(chan benchmarkResult, 100)

	// write header
	w := csv.NewWriter(f)
	w.Write(csvHeaders)
	w.Flush()

	// start recording results
	g.Go(func() error {
		for {
			result, ok := <-results
			if !ok {
				break
			}

			err := w.Write(result.csv())
			if err != nil {
				return fmt.Errorf("error writing to csv file: %v", err)
			}
			w.Flush()
		}
		return nil
	})

	return w
}
