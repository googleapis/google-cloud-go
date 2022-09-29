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

	// Install google-c2p resolver, which is required for direct path.
	_ "google.golang.org/grpc/xds/googledirectpath"
	// Install RLS load balancer policy, which is needed for gRPC RLS.
	_ "google.golang.org/grpc/balancer/rls"
)

const codeVersion = "0.4.1" // to keep track of which version of the code a benchmark ran on

var opts = &benchmarkOptions{}
var projectID, credentialsFile, outputFile string

var results chan benchmarkResult

type benchmarkOptions struct {
	// all sizes are in bytes
	api           benchmarkAPI
	bucket        string
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
	useDefaults   bool
	directPath    bool
}

func (b benchmarkOptions) String() string {
	var sb strings.Builder

	stringifiedOpts := []string{
		fmt.Sprintf("api:\t\t\t%s", b.api),
		fmt.Sprintf("region:\t\t\t%s", b.region),
		fmt.Sprintf("timeout:\t\t%s", b.timeout),
		fmt.Sprintf("number of samples:\tbetween %d - %d", b.minSamples, b.maxSamples),
		fmt.Sprintf("object size:\t\t%d - %d kib", b.minObjectSize/kib, b.maxObjectSize/kib),
		fmt.Sprintf("write size:\t\t%d - %d bytes (app buffer for uploads)", b.minWriteSize, b.maxWriteSize),
		fmt.Sprintf("read size:\t\t%d - %d bytes (app buffer for downloads)", b.minReadSize, b.maxReadSize),
		fmt.Sprintf("chunk size:\t\t%d - %d kib (library buffer for uploads)", b.minChunkSize/kib, b.maxChunkSize/kib),
		fmt.Sprintf("connection pool size:\t%d (GRPC)", b.connPoolSize),
		fmt.Sprintf("directpath:\t\t%t (GRPC)", b.directPath),
		fmt.Sprintf("num workers:\t\t%d (max number of concurrent benchmark runs at a time)", b.numWorkers),
		fmt.Sprintf("force garbage collection:%t", b.forceGC),
	}

	for _, s := range stringifiedOpts {
		sb.WriteByte('\n')
		sb.WriteByte('\t')
		sb.WriteString(s)
	}

	return sb.String()
}

func parseFlags() {
	flag.StringVar((*string)(&opts.api), "api", string(mixedAPIs), "api used to upload/download objects; JSON or XML values will use JSON to uplaod and XML to download")
	flag.StringVar(&opts.region, "r", "US-WEST1", "region")
	flag.DurationVar(&opts.timeout, "t", time.Hour, "timeout")
	flag.Int64Var(&opts.minObjectSize, "min_size", 512*kib, "minimum object size in bytes")
	flag.Int64Var(&opts.maxObjectSize, "max_size", 2097152*kib, "maximum object size in bytes")
	flag.IntVar(&opts.minWriteSize, "min_w_size", 4000, "minimum write size in bytes")
	flag.IntVar(&opts.maxWriteSize, "max_w_size", 4000, "maximum write size in bytes")
	flag.IntVar(&opts.minReadSize, "min_r_size", 4000, "minimum read size in bytes")
	flag.IntVar(&opts.maxReadSize, "max_r_size", 4000, "maximum read size in bytes")
	flag.IntVar(&opts.readQuantum, "q_read", 1, "read quantum for app buffer size")
	flag.IntVar(&opts.writeQuantum, "q_write", 1, "write quantum for app buffer size")
	flag.Int64Var(&opts.minChunkSize, "min_cs", 16*1024*1024, "min chunksize in bytes")
	flag.Int64Var(&opts.maxChunkSize, "max_cs", 16*1024*1024, "max chunksize in bytes")
	flag.IntVar(&opts.minSamples, "min_samples", 10, "minimum number of objects to upload")
	flag.IntVar(&opts.maxSamples, "max_samples", 10000, "maximum number of objects to upload")
	flag.StringVar(&outputFile, "o", "res.csv", "file to output results to")
	flag.BoolVar(&opts.forceGC, "gc_f", false, "force garbage collection at the beginning of each upload")
	flag.IntVar(&opts.numWorkers, "workers", 16, "number of concurrent workers")
	flag.IntVar(&opts.connPoolSize, "conn_pool", 4, "GRPC connection pool size")
	flag.BoolVar(&opts.useDefaults, "defaults", false, "use default client configuration")
	flag.BoolVar(&opts.directPath, "directpath", false, "use direct path")

	flag.StringVar(&projectID, "p", projectID, "projectID")
	flag.StringVar(&credentialsFile, "creds", credentialsFile, "path to credentials file")
	flag.StringVar(&opts.bucket, "bucket", "", "name of bucket to use; will create a bucket if not provided")

	flag.Parse()

	if len(projectID) < 1 {
		fmt.Println("Must set a project ID. Use flag -p to specify it.")
		os.Exit(1)
	}
}

func main() {
	parseFlags()
	rand.Seed(time.Now().UnixNano())
	closePools := initializeClientPools(opts)
	defer closePools()

	start := time.Now()
	fmt.Printf("Benchmarking started: %s\n", start.UTC().Format(time.ANSIC))
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(opts.timeout))
	defer cancel()

	// Create bucket if necessary
	if len(opts.bucket) < 1 {
		opts.bucket = randomName(bucketPrefix)
		cleanUp := createBenchmarkBucket(opts.bucket, opts)
		defer cleanUp()
	}

	// Create output file
	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", outputFile, err)
	}
	defer file.Close()

	// Enable direct path
	if opts.directPath {
		if err := os.Setenv("GOOGLE_CLOUD_ENABLE_DIRECT_PATH_XDS", "true"); err != nil {
			log.Fatalf("error setting direct path env var: %v", err)
		}
	}

	// Print benchmarking options
	fmt.Printf("Code version: %s\n", codeVersion)
	fmt.Printf("Results file: %s\n", outputFile)
	fmt.Printf("Bucket:  %s\n", opts.bucket)
	fmt.Printf("Benchmarking options: %+v\n", opts)

	recordResultGroup, _ := errgroup.WithContext(ctx)
	startRecordingResults(file, recordResultGroup)

	benchGroup, _ := errgroup.WithContext(ctx)
	benchGroup.SetLimit(opts.numWorkers)

	// Run benchmarks
	for i := 0; i < opts.maxSamples && (i < opts.minSamples || time.Since(start) < opts.timeout); i++ {
		benchGroup.Go(func() error {
			benchmark := w1r3{opts: opts, bucketName: opts.bucket}
			if err := benchmark.setup(); err != nil {
				// We don't want to stop benchmarking on a single run's error, so just log
				log.Printf("run setup failed: %v", err)
				return nil
			}
			if err := benchmark.run(ctx); err != nil {
				log.Printf("run failed: %v", err)
			}
			return nil
		})
	}

	benchGroup.Wait()
	close(results)
	recordResultGroup.Wait()

	fmt.Printf("\nTotal time running: %s\n", time.Since(start).Round(time.Second))
}

type randomizedParams struct {
	appBufferSize int
	chunkSize     int64
	crc32cEnabled bool
	md5Enabled    bool
	api           benchmarkAPI
}

type benchmarkResult struct {
	objectSize    int64
	params        randomizedParams
	isRead        bool
	readIteration int
	start         time.Time
	elapsedTime   time.Duration
	completed     bool
	startMem      runtime.MemStats
	endMem        runtime.MemStats
}

func (br *benchmarkResult) selectParams(opts benchmarkOptions) {
	api := opts.api
	if api == mixedAPIs {
		if randomBool() {
			api = xmlAPI
		} else {
			api = grpcAPI
		}
	}

	if br.isRead {
		if api == jsonAPI {
			api = xmlAPI
		}

		br.params = randomizedParams{
			appBufferSize: opts.readQuantum * randomInt(opts.minReadSize/opts.readQuantum, opts.maxReadSize/opts.readQuantum),
			chunkSize:     -1,    // not used for reads
			crc32cEnabled: true,  // crc32c is always verified in the Go GCS library
			md5Enabled:    false, // we only need one integrity validation
			api:           api,
		}

		if opts.useDefaults {
			br.params.appBufferSize = -1 // use -1 to indicate default; if we give it a value any change to defaults would not be reflected
		}

		return
	}

	if api == xmlAPI {
		api = jsonAPI
	}

	_, doMD5, doCRC32C := randomOf3()
	br.params = randomizedParams{
		appBufferSize: opts.writeQuantum * randomInt(opts.minWriteSize/opts.writeQuantum, opts.maxWriteSize/opts.writeQuantum),
		chunkSize:     randomInt64(opts.minChunkSize, opts.maxChunkSize),
		crc32cEnabled: doCRC32C,
		md5Enabled:    doMD5,
		api:           api,
	}

	if opts.useDefaults {
		// get a writer on an object to check the default chunksize
		// object does not need to exist
		c, _ := storage.NewClient(context.Background())
		ow := c.Bucket("").Object("").NewWriter(context.Background())
		br.params.chunkSize = int64(ow.ChunkSize)

		br.params.appBufferSize = -1 // use -1 to indicate default; if we give it a value any change to defaults would not be reflected
	}
}

// converts result to csv writing format (ie. a slice of strings)
func (br *benchmarkResult) csv() []string {
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
		strconv.Itoa(br.params.appBufferSize),
		strconv.Itoa(int(br.params.chunkSize)),
		strconv.FormatBool(br.params.crc32cEnabled),
		strconv.FormatBool(br.params.md5Enabled),
		string(br.params.api),
		strconv.FormatBool(opts.directPath),
		strconv.FormatInt(br.elapsedTime.Microseconds(), 10),
		"-1", // TODO: record cpu time
		status,
		strconv.FormatUint(br.startMem.HeapSys, 10),
		strconv.FormatUint(br.startMem.HeapAlloc, 10),
		strconv.FormatUint(br.startMem.StackInuse, 10),
		// commented out to avoid large numbers messing up BigQuery imports
		// TODO: revisit later
		"-1", //strconv.FormatUint(br.endMem.HeapAlloc-br.startMem.HeapAlloc, 10),
		strconv.FormatUint(br.endMem.Mallocs-br.startMem.Mallocs, 10),
		strconv.FormatInt(br.start.Unix(), 10),
		strconv.FormatInt(br.start.Add(br.elapsedTime).Unix(), 10),
		strconv.Itoa(opts.numWorkers),
		codeVersion,
		opts.bucket,
	}
}

var csvHeaders = []string{
	"Op", "ObjectSize", "AppBufferSize", "LibBufferSize",
	"Crc32cEnabled", "MD5Enabled", "ApiName", "DirectPath",
	"ElapsedTimeUs", "CpuTimeUs", "Status",
	"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
	"StartTime", "EndTime", "NumWorkers",
	"CodeVersion", "BucketName",
}

type benchmarkAPI string

const (
	jsonAPI   benchmarkAPI = "JSON"
	xmlAPI    benchmarkAPI = "XML"
	grpcAPI   benchmarkAPI = "GRPC"
	mixedAPIs benchmarkAPI = "MIXED"
)

func startRecordingResults(f *os.File, g *errgroup.Group) {
	// buffer channel so we don't block on printing results
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
				log.Fatalf("error writing to csv file: %v", err)
			}
			w.Flush()
		}
		return nil
	})
}
