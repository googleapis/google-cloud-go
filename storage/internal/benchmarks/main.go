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
	"os"
	"strconv"
	"strings"
	"time"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	epb "github.com/cloudprober/cloudprober/probes/external/proto"
	"github.com/cloudprober/cloudprober/probes/external/serverutils"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"golang.org/x/sync/errgroup"

	// Install google-c2p resolver, which is required for direct path.
	_ "google.golang.org/grpc/xds/googledirectpath"
	"google.golang.org/protobuf/proto"

	// Install RLS load balancer policy, which is needed for gRPC RLS.
	_ "google.golang.org/grpc/balancer/rls"
)

const (
	codeVersion = "0.11.0" // to keep track of which version of the code a benchmark ran on
	useDefault  = -1
	tracerName  = "storage-benchmark"
)

var (
	projectID, outputFile string

	opts       = &benchmarkOptions{}
	results    chan benchmarkResult
	serverMode bool
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

	readBufferSize  int
	writeBufferSize int

	minChunkSize int64
	maxChunkSize int64

	forceGC      bool
	connPoolSize int

	timeout      time.Duration
	timeoutPerOp time.Duration

	continueOnFail bool

	numClients             int
	workload               int
	numObjectsPerDirectory int

	useGCSFuseConfig bool
	endpoint         string

	enableTracing   bool
	traceSampleRate float64
	warmup          time.Duration
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

	minObjSize := b.objectSize
	if minObjSize == 0 {
		minObjSize = b.minObjectSize
	}

	if b.maxReadOffset > minObjSize-b.rangeSize {
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

	flag.BoolVar(&opts.useGCSFuseConfig, "gcs_fuse", false, "use GCSFuse configs on HTTP client creation")
	flag.StringVar(&opts.endpoint, "endpoint", "", "endpoint to set on Storage Client")

	flag.BoolVar(&opts.enableTracing, "tracing", false, "enable trace exporter to Cloud Trace")
	flag.Float64Var(&opts.traceSampleRate, "sample_rate", 1.0, "sample rate for traces")

	flag.IntVar(&opts.readBufferSize, "read_buffer_size", useDefault, "read buffer size in bytes")
	flag.IntVar(&opts.writeBufferSize, "write_buffer_size", useDefault, "write buffer size in bytes")

	flag.Int64Var(&opts.minChunkSize, "min_chunksize", useDefault, "min chunksize in bytes")
	flag.Int64Var(&opts.maxChunkSize, "max_chunksize", useDefault, "max chunksize in bytes")

	flag.IntVar(&opts.connPoolSize, "connection_pool_size", 4, "GRPC connection pool size")

	flag.BoolVar(&opts.forceGC, "force_garbage_collection", false, "force garbage collection at the beginning of each upload")

	flag.DurationVar(&opts.timeout, "timeout", time.Hour, "timeout")
	flag.DurationVar(&opts.timeoutPerOp, "timeout_per_op", time.Minute*5, "timeout per upload/download")
	flag.StringVar(&outputFile, "o", "", "file to output results to - if empty, will output to stdout")

	flag.BoolVar(&opts.continueOnFail, "continue_on_fail", false, "continue even if a run fails")

	flag.IntVar(&opts.numClients, "clients", 1, "number of storage clients to be used; if Mixed APIs, then twice the clients are created")

	flag.IntVar(&opts.workload, "workload", 1, "which workload to run")
	flag.IntVar(&opts.numObjectsPerDirectory, "directory_num_objects", 1000, "total number of objects in directory")

	flag.DurationVar(&opts.warmup, "warmup", 0, "time to warmup benchmarks; w1r3 benchmarks will be run for this duration without recording any results")

	flag.BoolVar(&serverMode, "server", false, "if true, script runs in cloudprober server mode")

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

	start := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(opts.timeout))
	defer cancel()

	// Print a message once deadline is exceeded
	go func() {
		<-ctx.Done()
		log.Printf("total configured timeout exceeded")
	}()

	// Create bucket if necessary
	if len(opts.bucket) < 1 {
		opts.bucket = randomName(bucketPrefix)
		cleanUp := createBenchmarkBucket(opts.bucket, opts)
		defer cleanUp()
	}

	w := os.Stdout

	// Create output file if necessary
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create file %s: %v", outputFile, err)
		}
		defer f.Close()
		w = f
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

	if err := populateDependencyVersions(); err != nil {
		log.Fatalf("populateDependencyVersions: %v", err)
	}

	if err := warmupW1R3(ctx, opts); err != nil {
		log.Fatal(err)
	}

	if opts.enableTracing {
		cleanup := enableTracing(ctx, opts.traceSampleRate)
		defer cleanup()
	}

	if serverMode {
		// Server mode blocks forever servicing probe requests
		runInServerMode(ctx, opts)
		log.Fatalln("server mode exited unexpectedly")
	}

	err := runSamples(ctx, opts, w)

	if outputFile != "" {
		// if sending output to a file, we can use stdout for informational logs
		fmt.Printf("\nTotal time running: %s\n", time.Since(start).Round(time.Second))
	}

	if err != nil {
		log.Fatalln(err)
	}
}

type benchmark interface {
	setup(context.Context) error
	run(context.Context) error
	cleanup() error
}

type randomizedParams struct {
	appBufferSize int
	chunkSize     int64
	crc32cEnabled bool
	md5Enabled    bool
	api           benchmarkAPI
	rangeOffset   int64
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
	header := selectHeader()
	cw := csv.NewWriter(w)
	if err := cw.Write(*header); err != nil {
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

// enableTracing turns on Open Telemetry tracing with export to Cloud Trace.
func enableTracing(ctx context.Context, sampleRate float64) func() {
	exporter, err := texporter.New(texporter.WithProjectID(projectID))
	if err != nil {
		log.Fatalf("texporter.New: %v", err)
	}

	// Identify your application using resource detection
	res, err := resource.New(ctx,
		// Use the GCP resource detector to detect information about the GCP platform
		resource.WithDetectors(gcp.NewDetector()),
		// Keep the default detectors
		resource.WithTelemetrySDK(),
		// Add your own custom attributes to identify your application
		resource.WithAttributes(
			semconv.ServiceName(tracerName),
		),
	)
	if err != nil {
		log.Fatalf("resource.New: %v", err)
	}

	// Create trace provider with the exporter.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRate)),
	)

	otel.SetTracerProvider(tp)

	return func() {
		tp.ForceFlush(ctx)
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatal(err)
		}
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

type multiError []error

func (e multiError) Error() string {
	var sb strings.Builder

	for _, err := range e {
		sb.WriteString(err.Error())
		sb.WriteRune('\n')
	}

	return sb.String()
}

// runSamples will run benchmarks until the required number of samples is
// collected and recorded, there is a setup failure, or we hit the deadline
func runSamples(ctx context.Context, opts *benchmarkOptions, out io.Writer) error {
	multiErr := multiError{}
	deadline, _ := ctx.Deadline()

	// Record results async
	recordResultGroup, _ := errgroup.WithContext(ctx)
	startRecordingResults(out, recordResultGroup, opts.outType)

	// Create a group to run benchmarks
	benchGroup, ctx := errgroup.WithContext(ctx)

	// Select concurrency
	var concurrentBenchmarkRuns int
	switch opts.workload {
	default:
		concurrentBenchmarkRuns = opts.numWorkers
	case 6, 9:
		// Directory benchmarks parallelize on the object level, so only run one
		// benchmark at a time
		concurrentBenchmarkRuns = 1
	}

	benchGroup.SetLimit(concurrentBenchmarkRuns)

	// Run benchmarks until the required number of samples is collected, or we
	// hit the deadline
	for i := 0; i < opts.numSamples && !time.Now().After(deadline); i++ {
		benchGroup.Go(func() error {
			var benchmark benchmark
			switch opts.workload {
			default:
				benchmark = &w1r3{opts: opts, bucketName: opts.bucket}
			case 6:
				benchmark = &directoryBenchmark{opts: opts, bucketName: opts.bucket, numWorkers: opts.numWorkers}
			case 9:
				benchmark = &continuousReads{opts: opts, bucketName: opts.bucket, numWorkers: opts.numWorkers}
			}

			if err := benchmark.setup(ctx); err != nil {
				// If setup fails, it will probably continue failing.
				// Returning the error here will cancel the context, which will
				// stop the benchmarking.
				return fmt.Errorf("run setup failed: %v", err)
			}
			if err := benchmark.run(ctx); err != nil {
				// If a run fails, we continue, as it could be a temporary issue.
				multiErr = append(multiErr, fmt.Errorf("run failed: %v", err))
			}
			if err := benchmark.cleanup(); err != nil {
				// If cleanup fails, we continue, as a failure here is not critical.
				// Cleanup may be expected to fail if there is an issue with the run.
				multiErr = append(multiErr, fmt.Errorf("run cleanup failed: %v", err))
			}
			return nil
		})
	}

	if err := benchGroup.Wait(); err != nil {
		multiErr = append(multiErr, err)
	}

	close(results)
	recordResultGroup.Wait()

	if len(multiErr) > 0 {
		return multiErr
	}
	return nil
}

// runInServerMode blocks forever servicing probe requests.
// Number of samples requested is ignored in this mode.
// Timeouts must be properly set at the probe level to ensure requests aren't
// being serviced concurrently
func runInServerMode(ctx context.Context, opts *benchmarkOptions) {
	// serverutils.Serve processes one request at a time sequentially; so we
	// always output the results of a sample before beginning another.
	// Therefore, this channel contains a maximum of 4 results (for w1r3) at once.
	results = make(chan benchmarkResult, 4)

	serverutils.Serve(func(request *epb.ProbeRequest, reply *epb.ProbeReply) {
		// Set the ctx so that we stop processing this sample if we reach the
		// timeout set in the prober's configuration.
		timeLimit := time.Millisecond * time.Duration(request.GetTimeLimit())
		ctx, cancel := context.WithTimeout(ctx, timeLimit)
		defer cancel()

		var benchmark benchmark
		benchmark = &w1r3{opts: opts, bucketName: opts.bucket}

		if opts.workload == 6 {
			benchmark = &directoryBenchmark{opts: opts, bucketName: opts.bucket, numWorkers: opts.numWorkers}
		}

		if err := benchmark.setup(ctx); err != nil {
			reply.ErrorMessage = proto.String(err.Error())
			return
		}
		if err := benchmark.run(ctx); err != nil {
			reply.ErrorMessage = proto.String(fmt.Errorf("run failed: %v", err).Error())
			benchmark.cleanup()
			return
		}
		if err := benchmark.cleanup(); err != nil {
			reply.ErrorMessage = proto.String(fmt.Errorf("run cleanup failed: %v", err).Error())
			return
		}

		numResults := 4 // w1r3 will output 1 result per read/write (1 write, 3 reads)

		if opts.workload == 6 {
			numResults = 2 // workload 6 only outputs 2 results (1 directory read, 1 write)
		}

		// Synchronously gather results
		payload := new(strings.Builder)

		for i := 0; i < numResults; i++ {
			result := <-results
			writeResultAsCloudMonitoring(payload, &result)
		}

		reply.Payload = proto.String(payload.String())
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
