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
	"io"
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

const codeVersion = "0.5.1" // to keep track of which version of the code a benchmark ran on

var (
	projectID, credentialsFile, outputFile string

	opts    = &benchmarkOptions{}
	results chan benchmarkResult
)

type benchmarkOptions struct {
	// all sizes are in bytes
	api             benchmarkAPI
	bucket          string
	region          string
	timeout         time.Duration
	minObjectSize   int64
	maxObjectSize   int64
	readQuantum     int
	writeQuantum    int
	minWriteSize    int
	maxWriteSize    int
	minReadSize     int
	maxReadSize     int
	minSamples      int
	maxSamples      int
	minChunkSize    int64
	maxChunkSize    int64
	forceGC         bool
	numWorkers      int
	connPoolSize    int
	useDefaults     bool
	outType         outputType
	workload        string
	appendToResults string
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
	flag.StringVar(&outputFile, "o", "", "file to output results to - if empty, will output to stdout")
	flag.BoolVar(&opts.forceGC, "gc_f", false, "force garbage collection at the beginning of each upload")
	flag.IntVar(&opts.numWorkers, "workers", 16, "number of concurrent workers")
	flag.IntVar(&opts.connPoolSize, "conn_pool", 4, "GRPC connection pool size")
	flag.BoolVar(&opts.useDefaults, "defaults", false, "use default client configuration")
	flag.StringVar(&opts.workload, "workload", "", "workload")
	flag.StringVar(&opts.appendToResults, "labels", "", "labels added to cloud monitoring output")

	flag.StringVar((*string)(&opts.outType), "output_type", string(outputCloudMonitoring), "output as csv or cloud monitoring format")
	flag.StringVar(&projectID, "p", projectID, "projectID")
	flag.StringVar(&credentialsFile, "creds", credentialsFile, "path to credentials file")
	flag.StringVar(&opts.bucket, "bucket", "", "name of bucket to use; will create a bucket if not provided")

	flag.Parse()

	if len(projectID) < 1 {
		log.Fatalln("Must set a project ID. Use flag -p to specify it.")
	}
}

func main() {
	log.SetOutput(os.Stderr)
	parseFlags()
	rand.Seed(time.Now().UnixNano())
	closePools := initializeClientPools(opts)
	defer closePools()

	start := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(opts.timeout))
	defer cancel()

	// Create bucket if necessary
	if len(opts.bucket) < 1 {
		opts.bucket = randomName(bucketPrefix)
		cleanUp := createBenchmarkBucket(opts.bucket, opts)
		defer cleanUp()
	}

	// Create output file
	var file *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create file %s: %v", outputFile, err)
		}
		defer f.Close()
		file = f
	}

	// Enable direct path
	if opts.api == directPath {
		if err := os.Setenv("GOOGLE_CLOUD_ENABLE_DIRECT_PATH_XDS", "true"); err != nil {
			log.Fatalf("error setting direct path env var: %v", err)
		}
	}

	if err := opts.api.validate(); err != nil {
		log.Fatal(err)
	}
	if err := opts.outType.validate(); err != nil {
		log.Fatal(err)
	}

	w := os.Stdout

	if outputFile != "" {
		w = file
		// Print benchmarking options
		fmt.Printf("Benchmarking started: %s\n", start.UTC().Format(time.ANSIC))
		fmt.Printf("Code version: %s\n", codeVersion)
		fmt.Printf("Results file: %s\n", outputFile)
		fmt.Printf("Bucket:  %s\n", opts.bucket)
		fmt.Printf("Benchmarking options: %+v\n", opts)
	}

	if err := populateDependencyVersions(); err != nil {
		log.Printf("populateDependencyVersions: %v", err)
	}

	recordResultGroup, _ := errgroup.WithContext(ctx)
	startRecordingResults(w, recordResultGroup, opts.outType)

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

	if outputFile != "" {
		fmt.Printf("\nTotal time running: %s\n", time.Since(start).Round(time.Second))
	}
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
		if c, err := storage.NewClient(context.Background()); err != nil {
			log.Printf("storage.NewClient: %v", err)
		} else {
			w := c.Bucket("").Object("").NewWriter(context.Background())
			br.params.chunkSize = int64(w.ChunkSize)
		}

		br.params.appBufferSize = -1 // use -1 to indicate default; if we give it a value any change to defaults would not be reflected
	}
}

// converts result to cloud monitoring format
func (br *benchmarkResult) cloudMonitoring() []byte {
	var sb strings.Builder
	op := "WRITE"
	if br.isRead {
		op = fmt.Sprintf("READ[%d]", br.readIteration)
	}
	status := "[OK]"
	if !br.completed {
		status = "[FAIL]"
	}

	throughput := float64(br.objectSize) / float64(br.elapsedTime.Seconds())

	// Cloud monitoring only allows letters, numbers and underscores
	sanitizeKey := func(key string) string {
		key = strings.Replace(key, ".", "", -1)
		return strings.Replace(key, "/", "_", -1)
	}

	makeStringQuoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=\"%v\"", parameter, value)
	}
	makeStringUnquoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=%v", parameter, value)
	}

	sb.Grow(380)
	sb.WriteString("throughput{")
	sb.WriteString(makeStringQuoted("workload", opts.workload))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("Op", op))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("ObjectSize", br.objectSize))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("AppBufferSize", br.params.appBufferSize))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("LibBufferSize", br.params.chunkSize))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("Crc32cEnabled", br.params.crc32cEnabled))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("MD5Enabled", br.params.md5Enabled))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("Api", br.params.api))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("ElapsedMicroseconds", br.elapsedTime.Microseconds()))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("Status", status))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("HeapSys", br.startMem.HeapSys))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("HeapAlloc", br.startMem.HeapAlloc))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("StackInUse", br.startMem.StackInuse))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("StartTime", br.start.Unix()))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("EndTime", br.start.Add(br.elapsedTime).Unix()))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("NumWorkers", opts.numWorkers))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("CodeVersion", codeVersion))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("BucketName", opts.bucket))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("GoVersion", goVersion))
	for dep, ver := range dependencyVersions {
		sb.WriteString(",")
		sb.WriteString(makeStringQuoted(sanitizeKey(dep), ver))
	}

	if opts.appendToResults != "" {
		sb.WriteString(",")
		sb.WriteString(opts.appendToResults)
	}

	sb.WriteString("} ")
	sb.WriteString(strconv.FormatFloat(throughput, 'f', 2, 64))

	return []byte(sb.String())
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

	record := make(map[string]string)

	record["Op"] = op
	record["ObjectSize"] = strconv.FormatInt(br.objectSize, 10)
	record["AppBufferSize"] = strconv.Itoa(br.params.appBufferSize)
	record["LibBufferSize"] = strconv.Itoa(int(br.params.chunkSize))
	record["Crc32cEnabled"] = strconv.FormatBool(br.params.crc32cEnabled)
	record["MD5Enabled"] = strconv.FormatBool(br.params.md5Enabled)
	record["ApiName"] = string(br.params.api)
	record["ElapsedTimeUs"] = strconv.FormatInt(br.elapsedTime.Microseconds(), 10)
	record["CpuTimeUs"] = "-1" // TODO: record cpu time
	record["Status"] = status
	record["HeapSys"] = strconv.FormatUint(br.startMem.HeapSys, 10)
	record["HeapAlloc"] = strconv.FormatUint(br.startMem.HeapAlloc, 10)
	record["StackInUse"] = strconv.FormatUint(br.startMem.StackInuse, 10)
	// commented out to avoid large numbers messing up BigQuery imports
	record["HeapAllocDiff"] = "-1" //strconv.FormatUint(br.endMem.HeapAlloc-br.startMem.HeapAlloc, 10),
	record["MallocsDiff"] = strconv.FormatUint(br.endMem.Mallocs-br.startMem.Mallocs, 10)
	record["StartTime"] = strconv.FormatInt(br.start.Unix(), 10)
	record["EndTime"] = strconv.FormatInt(br.start.Add(br.elapsedTime).Unix(), 10)
	record["NumWorkers"] = strconv.Itoa(opts.numWorkers)
	record["CodeVersion"] = codeVersion
	record["BucketName"] = opts.bucket

	var result []string

	for _, h := range csvHeader {
		result = append(result, record[h])
	}

	return result
}

var csvHeader = []string{
	"Op", "ObjectSize", "AppBufferSize", "LibBufferSize",
	"Crc32cEnabled", "MD5Enabled", "ApiName",
	"ElapsedTimeUs", "CpuTimeUs", "Status",
	"HeapSys", "HeapAlloc", "StackInUse", "HeapAllocDiff", "MallocsDiff",
	"StartTime", "EndTime", "NumWorkers",
	"CodeVersion", "BucketName",
}

type benchmarkAPI string

const (
	jsonAPI    benchmarkAPI = "JSON"
	xmlAPI     benchmarkAPI = "XML"
	grpcAPI    benchmarkAPI = "GRPC"
	mixedAPIs  benchmarkAPI = "MIXED"
	directPath benchmarkAPI = "DirectPath"
)

func (api benchmarkAPI) validate() error {
	switch api {
	case jsonAPI, grpcAPI, xmlAPI, directPath, mixedAPIs:
		return nil
	default:
		return fmt.Errorf("no such api: %s", api)
	}
}

func writeHeader(w io.Writer) {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		log.Fatalf("error writing csv header: %v", err)
	}
	cw.Flush()
}

func writeResultAsCSV(w io.Writer, result *benchmarkResult) {
	cw := csv.NewWriter(w)
	if err := cw.Write(result.csv()); err != nil {
		log.Fatalf("error writing csv: %v", err)
	}
	cw.Flush()
}

func writeResultAsCloudMonitoring(w io.Writer, result *benchmarkResult) {
	_, err := w.Write(result.cloudMonitoring())
	if err != nil {
		log.Fatalf("cloud monitoring w.Write: %v", err)
	}
	_, err = w.Write([]byte{'\n'})
	if err != nil {
		log.Fatalf("cloud monitoring w.Write: %v", err)
	}
}

func startRecordingResults(w io.Writer, g *errgroup.Group, oType outputType) {
	// buffer channel so we don't block on printing results
	results = make(chan benchmarkResult, 100)

	if oType == outputCSV {
		writeHeader(w)
	}

	// start recording results
	g.Go(func() error {
		for {
			result, ok := <-results
			if !ok {
				break
			}

			if oType == outputCSV {
				writeResultAsCSV(w, &result)
			} else if oType == outputCloudMonitoring {
				writeResultAsCloudMonitoring(w, &result)
			}
		}
		return nil
	})
}

type outputType string

const (
	outputCSV             outputType = "csv"
	outputCloudMonitoring outputType = "cloud-monitoring"
)

func (o outputType) validate() error {
	switch o {
	case outputCSV, outputCloudMonitoring:
		return nil
	default:
		return fmt.Errorf("could not parse output type: %s", o)
	}
}
