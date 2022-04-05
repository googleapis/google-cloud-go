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
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
)

const (
	CODE_VERSION = 1.0
	prefix       = "golang-grpc-test-" // prefix must be this for GRPC (for now)
)

var opts = &benchmarkOptions{}
var projectID, credentialsFile, resultsFile string

var results chan benchmarkResult

type benchmarkOptions struct {
	api           benchmarkAPI
	region        string
	timeout       time.Duration
	minObjectSize int64
	maxObjectSize int64
	readQuantum   int
	writeQuantum  int
	minSamples    int
	maxSamples    int
	minChunkSize  int64
	maxChunkSize  int64
	forceGC       bool
	concurrent    bool
}

func Init() {
	flag.StringVar((*string)(&opts.api), "api", string(Random), "api used to upload/download objects; JSON or XML values will use JSON to upload and XML to download")
	flag.StringVar(&opts.region, "r", "US-WEST1", "region")
	minSize := flag.Int64("min_size", 0, "minimum object size in kib")
	maxSize := flag.Int64("max_size", 16, "maximum object size in kib")
	flag.IntVar(&opts.readQuantum, "q_read", 16, "read quantum")
	flag.IntVar(&opts.writeQuantum, "q_write", 16, "write quantum")
	minChunkSize := flag.Int64("min_cs", 16*1024, "min chunksize in kib")
	maxChunkSize := flag.Int64("max_cs", 16*1024, "max chunksize in kib")
	flag.DurationVar(&opts.timeout, "t", time.Hour, "timeout")
	flag.IntVar(&opts.minSamples, "min_samples", 10, "minimum number of objects to upload")
	flag.IntVar(&opts.maxSamples, "max_samples", 10000, "maximum number of objects to upload")
	flag.StringVar(&resultsFile, "o", "res.csv", "file to output results to")
	flag.BoolVar(&opts.forceGC, "gc_f", false, "force garbage collection at the beginning of each upload")
	flag.BoolVar(&opts.concurrent, "c", false, "concurrent")

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

	bucketName := randomBucketName(prefix)
	cleanUp := createBenchmarkBucket(bucketName, opts)
	_ = cleanUp
	defer cleanUp()

	fmt.Printf("Results file: %s\n", resultsFile)
	fmt.Printf("Benchmarking bucket: %s\n", bucketName)
	fmt.Printf("Benchmarking options: %+v\n", opts)
	fmt.Printf("Code version: %0.2f\n", CODE_VERSION)

	// Create output file
	file, err := os.Create(resultsFile)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", resultsFile, err)
	}
	defer file.Close()

	recordResultGroup, _ := errgroup.WithContext(context.Background())
	startRecordingResults(file, recordResultGroup)

	benchmarkRunGroup, _ := errgroup.WithContext(context.Background())

	for i := 0; i < opts.maxSamples && (i < opts.minSamples || time.Since(start) < opts.timeout); i++ {
		if opts.concurrent {
			benchmarkRunGroup.Go(func() error {
				if err := benchmarkRun(context.Background(), opts, bucketName); err != nil {
					log.Printf("run failed: %v", err)
				}
				return nil
			})
		} else {
			if err := benchmarkRun(context.Background(), opts, bucketName); err != nil {
				log.Printf("run failed: %v", err)
			}
		}
	}
	benchmarkRunGroup.Wait()
	close(results)

	if err := recordResultGroup.Wait(); err != nil {
		fmt.Printf("go-routine return error: %v", err)
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
	if opts.forceGC {
		runtime.GC()
		// debug.FreeOSMemory()
	}
	var memStats *runtime.MemStats = &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	prevHeapAlloc := memStats.HeapAlloc
	prevMallocs := memStats.Mallocs

	objectName := randomObjectName()

	writeChunkSize := randomValue(opts.minChunkSize, opts.maxChunkSize)

	objectSize := randomValue(opts.minObjectSize, opts.maxObjectSize)
	// The application buffer sizes for read() and write() calls are also selected at random.
	// These sizes are quantized, and the quantum can be configured in the command-line.
	// readBufferSize := 256
	appWriteBufferSize := (opts.writeQuantum * 1024 * 1024) // randomized below at contents := randomString...

	// TODO: CRC32C and/or MD5 hashes

	// Select client
	client, readAPI, writeAPI, err := initializeClient(ctx, opts.api)
	if err != nil {
		return fmt.Errorf("NewClient: %v", err)
	}

	// Create contents to write
	contents := randomString(appWriteBufferSize, ASCIIchars)
	contentsReader := strings.NewReader(contents)

	o := client.Bucket(bucketName).Object(objectName)
	defer o.Delete(ctx)

	// Upload
	start := time.Now()
	timeTaken, err := uploadBenchmark(ctx, uploadParams{
		o:              o,
		contentsReader: contentsReader,
		readerLength:   len(contents),
		objectSize:     objectSize,
		chunkSize:      int(writeChunkSize),
	})
	runtime.ReadMemStats(memStats)
	results <- benchmarkResult{
		objectSize:    objectSize,
		appBufferSize: len(contents),
		chunkSize:     int(writeChunkSize),
		crc32Enabled:  false,
		md5Enabled:    false,
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
		runtime.ReadMemStats(memStats)
		prevHeapAlloc = memStats.HeapAlloc
		prevMallocs = memStats.Mallocs

		start := time.Now()
		timeTaken, err := downloadBenchmark(ctx, o, objectSize)
		runtime.ReadMemStats(memStats)
		results <- benchmarkResult{
			objectSize:    objectSize,
			crc32Enabled:  false,
			md5Enabled:    false,
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
	JSON   benchmarkAPI = "JSON"
	XML    benchmarkAPI = "XML"
	GRPC   benchmarkAPI = "GRPC"
	Random benchmarkAPI = "RANDOM"
)

var csvHeaders = []string{
	"Op", "ObjectSize", "AppBufferSize", "LibBufferSize",
	"Crc32cEnabled", "MD5Enabled", "ApiName",
	"ElapsedTimeUs", "CpuTimeUs", "Status",
	"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
	"StartTime",
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
		br.start.Format(time.UnixDate),
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
				log.Printf("Error writing to csv file: %v", err)
			}
			w.Flush()
		}
		return nil
	})

	return w
}
