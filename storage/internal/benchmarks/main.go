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

const codeVersion = "0.7.0" // to keep track of which version of the code a benchmark ran on

var (
	projectID, outputFile string

	opts    = &benchmarkOptions{}
	results chan benchmarkResult
)

type benchmarkOptions struct {
	// all sizes are in bytes
	bucket     string
	region     string
	outType    outputType
	numSamples int
	numWorkers int
	api        benchmarkAPI

	objectSize    int64
	minObjectSize int64
	maxObjectSize int64

	rangeSize     int64
	minReadOffset int64
	maxReadOffset int64

	allowCustomClient bool
	readBufferSize    int
	writeBufferSize   int

	minChunkSize int64
	maxChunkSize int64

	forceGC      bool
	connPoolSize int

	timeout         time.Duration
	appendToResults string

	numClients int
}

func (b *benchmarkOptions) validate() error {
	if err := b.api.validate(); err != nil {
		return err
	}
	if err := b.outType.validate(); err != nil {
		return err
	}

	if (b.maxReadOffset != 0 || b.minReadOffset != 0) && b.rangeSize == 0 {
		return fmt.Errorf("read offset specified but no range size specified")
	}

	if b.maxReadOffset > b.minObjectSize-b.rangeSize {
		return fmt.Errorf("read offset (%d) is too large for the selected range size (%d) - object might run out of bytes before reading complete rangeSize", b.maxReadOffset, b.rangeSize)
	}
	return nil
}

func (b *benchmarkOptions) String() string {
	var sb strings.Builder

	stringifiedOpts := []string{
		fmt.Sprintf("api:\t\t\t%s", b.api),
		fmt.Sprintf("region:\t\t\t%s", b.region),
		fmt.Sprintf("timeout:\t\t%s", b.timeout),
		fmt.Sprintf("number of samples:\t%d", b.numSamples),
		fmt.Sprintf("object size:\t\t%d kib", b.objectSize/kib),
		fmt.Sprintf("object size (if none above):\t%d - %d kib", b.minObjectSize/kib, b.maxObjectSize/kib),
		fmt.Sprintf("write size:\t\t%d bytes (app buffer for uploads)", b.writeBufferSize),
		fmt.Sprintf("read size:\t\t%d bytes (app buffer for downloads)", b.readBufferSize),
		fmt.Sprintf("chunk size:\t\t%d - %d kib (library buffer for uploads)", b.minChunkSize/kib, b.maxChunkSize/kib),
		fmt.Sprintf("range offset:\t\t%d - %d bytes ", b.minReadOffset, b.maxReadOffset),
		fmt.Sprintf("range size:\t\t%d bytes (0 -> full object)", b.rangeSize),
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
	flag.StringVar(&projectID, "project", projectID, "GCP project identifier")

	flag.StringVar(&opts.bucket, "bucket", "", "name of bucket to use; will create a bucket if not provided")
	flag.StringVar(&opts.region, "bucket_region", "US-WEST1", "region")
	flag.StringVar((*string)(&opts.outType), "output_type", string(outputCloudMonitoring), "output as csv or cloud monitoring format")
	flag.IntVar(&opts.numSamples, "samples", 8000, "number of samples to report")
	flag.IntVar(&opts.numWorkers, "workers", 16, "number of concurrent workers")
	flag.StringVar((*string)(&opts.api), "api", string(mixedAPIs), "api used to upload/download objects; JSON or XML values will use JSON to uplaod and XML to download")

	objectRange := flag.String("object_size", fmt.Sprint(1024*kib), "object size in bytes")

	flag.Int64Var(&opts.rangeSize, "range_read_size", 0, "size of the range to read in bytes")
	flag.Int64Var(&opts.minReadOffset, "minimum_read_offset", 0, "minimum read offset in bytes")
	flag.Int64Var(&opts.maxReadOffset, "maximum_read_offset", 0, "maximum read offset in bytes")

	flag.BoolVar(&opts.allowCustomClient, "allow_custom_HTTP_client", false, "allow custom client configuration")
	flag.IntVar(&opts.readBufferSize, "read_buffer_size", 4000, "read buffer size in bytes")
	flag.IntVar(&opts.writeBufferSize, "write_buffer_size", 4000, "write buffer size in bytes")

	flag.Int64Var(&opts.minChunkSize, "min_chunksize", 16*1024*1024, "min chunksize in bytes")
	flag.Int64Var(&opts.maxChunkSize, "max_chunksize", 16*1024*1024, "max chunksize in bytes")

	flag.IntVar(&opts.connPoolSize, "connection_pool_size", 4, "GRPC connection pool size")

	flag.BoolVar(&opts.forceGC, "force_garbage_collection", false, "force garbage collection at the beginning of each upload")

	flag.DurationVar(&opts.timeout, "timeout", time.Hour, "timeout")
	flag.StringVar(&outputFile, "o", "", "file to output results to - if empty, will output to stdout")
	flag.StringVar(&opts.appendToResults, "append_labels", "", "labels added to cloud monitoring output")

	flag.IntVar(&opts.numClients, "clients", 1, "number of storage clients to be used; if Mixed APIs, then twice the clients are created")

	flag.Parse()

	if len(projectID) < 1 {
		log.Fatalln("Must set a project ID. Use flag -project to specify it.")
	}

	min, max, isRange := strings.Cut(*objectRange, "..")
	var err error
	if isRange {
		opts.minObjectSize, err = strconv.ParseInt(min, 10, 64)
		if err != nil {
			log.Fatalln("Could not parse object size")
		}
		opts.maxObjectSize, err = strconv.ParseInt(max, 10, 64)
		if err != nil {
			log.Fatalln("Could not parse object size")
		}
	} else {
		opts.objectSize, err = strconv.ParseInt(min, 10, 64)
		if err != nil {
			log.Fatalln("Could not parse object size")
		}
	}
}

func main() {
	log.SetOutput(os.Stderr)
	parseFlags()
	rand.Seed(time.Now().UnixNano())

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

	if err := opts.validate(); err != nil {
		log.Fatal(err)
	}

	closePools := initializeClientPools(ctx, opts)
	defer closePools()

	w := os.Stdout

	if outputFile != "" {
		w = file
		// The output file is only for benchmarking data points; if sending
		// output to a file, we can use stdout for informational logs
		fmt.Printf("Benchmarking started: %s\n", start.UTC().Format(time.ANSIC))
		fmt.Printf("Code version: %s\n", codeVersion)
		fmt.Printf("Results file: %s\n", outputFile)
		fmt.Printf("Bucket:  %s\n", opts.bucket)
		fmt.Printf("Benchmarking options: %+v\n", opts)
	}

	if err := populateDependencyVersions(); err != nil {
		log.Fatalf("populateDependencyVersions: %v", err)
	}

	recordResultGroup, _ := errgroup.WithContext(ctx)
	startRecordingResults(w, recordResultGroup, opts.outType)

	benchGroup, _ := errgroup.WithContext(ctx)
	benchGroup.SetLimit(opts.numWorkers)

	// Run benchmarks
	for i := 0; i < opts.numSamples && time.Since(start) < opts.timeout; i++ {
		benchGroup.Go(func() error {
			benchmark := w1r3{opts: opts, bucketName: opts.bucket}
			if err := benchmark.setup(); err != nil {
				log.Fatalf("run setup failed: %v", err)
			}
			if err := benchmark.run(ctx); err != nil {
				log.Fatalf("run failed: %v", err)
			}
			return nil
		})
	}

	benchGroup.Wait()
	close(results)
	recordResultGroup.Wait()

	if outputFile != "" {
		// if sending output to a file, we can use stdout for informational logs
		fmt.Printf("\nTotal time running: %s\n", time.Since(start).Round(time.Second))
	}
}

type randomizedParams struct {
	appBufferSize int
	chunkSize     int64
	crc32cEnabled bool
	md5Enabled    bool
	api           benchmarkAPI
	rangeOffset   int64
}

type benchmarkResult struct {
	objectSize    int64
	readOffset    int64
	params        randomizedParams
	isRead        bool
	readIteration int
	start         time.Time
	elapsedTime   time.Duration
	err           error
	timedOut      bool
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
			appBufferSize: opts.readBufferSize,
			chunkSize:     -1,    // not used for reads
			crc32cEnabled: true,  // crc32c is always verified in the Go GCS library
			md5Enabled:    false, // we only need one integrity validation
			api:           api,
			rangeOffset:   randomInt64(opts.minReadOffset, opts.maxReadOffset),
		}

		if !opts.allowCustomClient {
			br.params.appBufferSize = 4000 // default for HTTP

			if api == grpcAPI {
				br.params.appBufferSize = 32000 // default for GRPC
			}
			return
		}
	}

	if api == xmlAPI {
		api = jsonAPI
	}

	_, doMD5, doCRC32C := randomOf3()
	br.params = randomizedParams{
		appBufferSize: opts.writeBufferSize,
		chunkSize:     randomInt64(opts.minChunkSize, opts.maxChunkSize),
		crc32cEnabled: doCRC32C,
		md5Enabled:    doMD5,
		api:           api,
	}

	if !opts.allowCustomClient {
		// get a writer on an object to check the default chunksize
		// object does not need to exist
		if c, err := storage.NewClient(context.Background()); err != nil {
			log.Printf("storage.NewClient: %v", err)
		} else {
			w := c.Bucket("").Object("").NewWriter(context.Background())
			br.params.chunkSize = int64(w.ChunkSize)
		}

		br.params.appBufferSize = 4000 // default for HTTP
		if api == grpcAPI {
			br.params.appBufferSize = 32000 // default for GRPC
		}
	}
}

func (br *benchmarkResult) copyParams(from *benchmarkResult) {
	br.params = randomizedParams{
		appBufferSize: from.params.appBufferSize,
		chunkSize:     from.params.chunkSize,
		crc32cEnabled: from.params.crc32cEnabled,
		md5Enabled:    from.params.md5Enabled,
		api:           from.params.api,
		rangeOffset:   from.params.rangeOffset,
	}
}

// converts result to cloud monitoring format
func (br *benchmarkResult) cloudMonitoring() []byte {
	var sb strings.Builder
	op := "WRITE"
	if br.isRead {
		op = fmt.Sprintf("READ[%d]", br.readIteration)
	}
	status := "OK"
	if br.err != nil {
		status = "FAIL"

		if br.timedOut {
			status = "TIMEOUT"
		}
	}

	throughput := float64(br.objectSize) / float64(br.elapsedTime.Seconds())

	// Cloud monitoring only allows letters, numbers and underscores
	sanitizeKey := func(key string) string {
		key = strings.Replace(key, ".", "", -1)
		return strings.Replace(key, "/", "_", -1)
	}

	sanitizeValue := func(key string) string {
		return strings.Replace(key, "\"", "", -1)
	}

	// For values of type string
	makeStringQuoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=\"%v\"", parameter, value)
	}
	// For values of type int, bool
	makeStringUnquoted := func(parameter string, value any) string {
		return fmt.Sprintf("%s=%v", parameter, value)
	}

	sb.Grow(380)
	sb.WriteString("throughput{")
	sb.WriteString(makeStringQuoted("library", "go"))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("api", br.params.api))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("op", op))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("object_size", br.objectSize))
	sb.WriteString(",")

	if op != "WRITE" && opts.rangeSize > 0 {
		sb.WriteString(makeStringUnquoted("transfer_size", opts.rangeSize))
		sb.WriteString(",")
		sb.WriteString(makeStringUnquoted("transfer_offset", br.params.rangeOffset))
		sb.WriteString(",")
	}

	if op == "WRITE" {
		sb.WriteString(makeStringUnquoted("chunksize", br.params.chunkSize))
		sb.WriteString(",")
	}

	sb.WriteString(makeStringUnquoted("workers", opts.numWorkers))
	sb.WriteString(",")

	sb.WriteString(makeStringUnquoted("crc32c_enabled", br.params.crc32cEnabled))
	sb.WriteString(",")
	sb.WriteString(makeStringUnquoted("md5_enabled", br.params.md5Enabled))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("bucket_name", opts.bucket))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("status", status))
	sb.WriteString(",")
	if br.err != nil {
		sb.WriteString(makeStringQuoted("failure_msg", sanitizeValue(br.err.Error())))
		sb.WriteString(",")
	}

	sb.WriteString(makeStringUnquoted("app_buffer_size", br.params.appBufferSize))
	sb.WriteString(",")

	sb.WriteString(makeStringQuoted("code_version", codeVersion))
	sb.WriteString(",")
	sb.WriteString(makeStringQuoted("go_version", goVersion))
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
	status := "OK"
	if br.err != nil {
		status = "FAIL"
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
	mixedAPIs  benchmarkAPI = "Mixed"
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
