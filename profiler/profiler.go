// Copyright 2017 Google LLC
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

// Package profiler is a client for the Cloud Profiler service.
//
// Usage example:
//
//	import "cloud.google.com/go/profiler"
//	...
//	if err := profiler.Start(profiler.Config{Service: "my-service"}); err != nil {
//	    // TODO: Handle error.
//	}
//
// Calling Start will start a goroutine to collect profiles and upload to
// the profiler server, at the rhythm specified by the server.
//
// The caller must provide the service string in the config, and may provide
// other information as well. See Config for details.
//
// Profiler has CPU, heap and goroutine profiling enabled by default. Mutex
// profiling can be enabled in the config. Note that goroutine and mutex
// profiles are shown as "threads" and "contention" profiles in the profiler
// UI.
package profiler

import (
	// Standard library imports
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"       // Used for Go version and GC
	"runtime/pprof" // For profile collection functions (StartCPUProfile etc.)
	"strings"
	"sync"
	"time"

	// Google Cloud & API related imports
	"cloud.google.com/go/bigquery"
	gcemd "cloud.google.com/go/compute/metadata" // GCE metadata client
	"github.com/google/uuid"                     // For UUID generation
	"github.com/googleapis/gax-go/v2"            // Google API Extensions
	"google.golang.org/api/option"               // Google API client options
	gtransport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/devtools/cloudprofiler/v2" // Profiler API protobuf definitions
	edpb "google.golang.org/genproto/googleapis/rpc/errdetails"          // Error details protobuf definitions

	// Error details protobuf definitions
	"google.golang.org/grpc" // gRPC library
	// gRPC status codes
	"google.golang.org/grpc/codes"
	grpcmd "google.golang.org/grpc/metadata" // gRPC metadata handling
	"google.golang.org/grpc/status"

	// gRPC status handling
	// Core protobuf library
	"google.golang.org/protobuf/proto"
	_ "google.golang.org/protobuf/types/known/durationpb"
	_ "google.golang.org/protobuf/types/known/timestamppb"

	// PProf imports
	pprof_pb "github.com/google/pprof/profile" // Contains generated Profile struct and funcs like Parse, Merge
)

// --- ADDED: BigQuery Target Configuration ---
const (
	bqDenormDatasetIDDefault = "jatinagarwala"              // Your Dataset ID
	bqDenormTableIDDefault   = "profiler_denormalised_data" // Your Table ID
	bqUploadTimeout          = 30 * time.Second             // Timeout for BQ upload attempts
	agentVersion             = "0.1.0-bq"                   // Placeholder version for User-Agent
)

// --- ADDED: BigQuery Schema Mirror Structs ---
// (Struct definitions remain the same)
type BigQueryProfileRow struct {
	ProfileUUID         string                 `bigquery:"profile_uuid"`
	UploadTimestamp     time.Time              `bigquery:"upload_timestamp"`
	ProfileType         string                 `bigquery:"profile_type"`
	TimeNanos           bigquery.NullTimestamp `bigquery:"time_nanos"`
	DurationNanos       int64                  `bigquery:"duration_nanos"`
	PeriodType          *BQValueType           `bigquery:"period_type"`
	Period              int64                  `bigquery:"period"`
	Comment             []string               `bigquery:"comment"` // NOTE: Populated as empty due to parsing limitations
	DefaultSampleType   string                 `bigquery:"default_sample_type"`
	DocURL              string                 `bigquery:"doc_url"` // NOTE: Populated as empty due to parsing limitations
	DropFramesPattern   string                 `bigquery:"drop_frames_pattern"`
	KeepFramesPattern   string                 `bigquery:"keep_frames_pattern"`
	SampleType          []BQValueType          `bigquery:"sample_type"`
	Sample              []BQSample             `bigquery:"sample"`
	Mapping             []BQMapping            `bigquery:"mapping"`
	Location            []BQLocation           `bigquery:"location"`
	Function            []BQFunction           `bigquery:"function"`
	DeploymentProjectID string                 `bigquery:"deployment_project_id"`
	DeploymentTarget    string                 `bigquery:"deployment_target"`
	DeploymentLabels    []BQKeyValue           `bigquery:"deployment_labels"`
	ProfileLabels       []BQKeyValue           `bigquery:"profile_labels"`
}
type BQValueType struct {
	Type string `bigquery:"type"`
	Unit string `bigquery:"unit"`
}
type BQSample struct {
	LocationID []int64   `bigquery:"location_id"`
	Value      []int64   `bigquery:"value"`
	Label      []BQLabel `bigquery:"label"`
}
type BQLabel struct {
	Key     string `bigquery:"key"`
	Str     string `bigquery:"str"`
	Num     int64  `bigquery:"num"`
	NumUnit string `bigquery:"num_unit"`
}
type BQMapping struct {
	ID              int64  `bigquery:"id"`
	MemoryStart     int64  `bigquery:"memory_start"`
	MemoryLimit     int64  `bigquery:"memory_limit"`
	FileOffset      int64  `bigquery:"file_offset"`
	Filename        string `bigquery:"filename"`
	BuildID         string `bigquery:"build_id"`
	HasFunctions    bool   `bigquery:"has_functions"`
	HasFilenames    bool   `bigquery:"has_filenames"`
	HasLineNumbers  bool   `bigquery:"has_line_numbers"`
	HasInlineFrames bool   `bigquery:"has_inline_frames"`
}
type BQLocation struct {
	ID        int64    `bigquery:"id"`
	MappingID int64    `bigquery:"mapping_id"`
	Address   int64    `bigquery:"address"`
	Line      []BQLine `bigquery:"line"`
	IsFolded  bool     `bigquery:"is_folded"`
}
type BQLine struct {
	FunctionID int64 `bigquery:"function_id"`
	Line       int64 `bigquery:"line"`
	Column     int64 `bigquery:"column"`
}
type BQFunction struct {
	ID         int64  `bigquery:"id"`
	Name       string `bigquery:"name"`
	SystemName string `bigquery:"system_name"`
	Filename   string `bigquery:"filename"`
	StartLine  int64  `bigquery:"start_line"`
}
type BQKeyValue struct {
	Key   string `bigquery:"key"`
	Value string `bigquery:"value"`
}

// --- END ADDED: BigQuery Schema Mirror Structs ---

var (
	config       Config
	startOnce    allowUntilSuccess
	mutexEnabled bool
	logger       *log.Logger
	// The functions below are stubbed to be overrideable for testing.
	getProjectID     = gcemd.ProjectID
	getInstanceName  = gcemd.InstanceName
	getZone          = gcemd.Zone
	startCPUProfile  = pprof.StartCPUProfile
	stopCPUProfile   = pprof.StopCPUProfile
	writeHeapProfile = pprof.WriteHeapProfile
	sleep            = gax.Sleep
	dialGRPC         = gtransport.DialPool
	onGCE            = gcemd.OnGCE
	serviceRegexp    = regexp.MustCompile(`^[a-z]([-a-z0-9_.]{0,253}[a-z0-9])?$`)

	// For testing only.
	profilingDone chan bool
)

const (
	apiAddress       = "cloudprofiler.googleapis.com:443"
	xGoogAPIMetadata = "x-goog-api-client"
	zoneNameLabel    = "zone"
	versionLabel     = "version"
	languageLabel    = "language"
	instanceLabel    = "instance"
	scope            = "https://www.googleapis.com/auth/monitoring.write"

	initialBackoff    = time.Minute
	maxBackoff        = time.Hour
	backoffMultiplier = 1.3
	retryInfoMetadata = "google.rpc.retryinfo-bin"
)

// Config is the profiler configuration.
// ... (Config struct definition remains the same) ...
type Config struct {
	Service              string
	ServiceVersion       string
	DebugLogging         bool
	MutexProfiling       bool
	NoCPUProfiling       bool
	NoAllocProfiling     bool
	AllocForceGC         bool
	NoHeapProfiling      bool
	NoGoroutineProfiling bool
	EnableOCTelemetry    bool
	ProjectID            string
	APIAddr              string
	Instance             string
	Zone                 string
	numProfiles          int
}

// allowUntilSuccess is an object that will perform action till
// it succeeds once.
// ... (allowUntilSuccess struct and do method remain the same) ...
type allowUntilSuccess struct {
	m    sync.Mutex
	done uint32
}

func (o *allowUntilSuccess) do(f func() error) (err error) {
	o.m.Lock()
	defer o.m.Unlock()
	if o.done == 0 {
		if err = f(); err == nil {
			o.done = 1
		}
	} else {
		debugLog("profiler.Start() called again after it was previously called")
		err = nil
	}
	return err
}

// Start starts a goroutine to collect and upload profiles.
// ... (Start function remains the same) ...
func Start(cfg Config, options ...option.ClientOption) error {
	startError := startOnce.do(func() error {
		return start(cfg, options...)
	})
	return startError
}

// start initializes the agent and starts the background polling goroutine.
// ... (start function remains the same as previous corrected version) ...
func start(cfg Config, options ...option.ClientOption) error {
	logger = log.New(os.Stderr, "Cloud Profiler: ", log.LstdFlags)
	if err := initializeConfig(cfg); err != nil {
		debugLog("failed to initialize config: %v", err)
		return err
	}
	if config.MutexProfiling {
		if runtime.Version() < "go1.8" {
			log.Println("Cloud Profiler: Mutex profiling requested but requires Go 1.8 or later. Disabling.")
			config.MutexProfiling = false
			mutexEnabled = false
		} else {
			runtime.SetMutexProfileFraction(5) // Example rate
			mutexEnabled = true
			debugLog("Mutex profiling enabled with rate %d.", 5)
		}
	}

	ctx := context.Background()

	opts := []option.ClientOption{
		option.WithEndpoint(config.APIAddr),
		option.WithScopes(scope),
		option.WithUserAgent(fmt.Sprintf("gcloud-go-profiler/%s", agentVersion)),
	}
	if !config.EnableOCTelemetry {
		opts = append(opts, option.WithTelemetryDisabled())
	}
	opts = append(opts, options...)

	connPool, err := dialGRPC(ctx, opts...)
	if err != nil {
		debugLog("failed to dial GRPC: %v", err)
		return err
	}

	profilerClient := pb.NewProfilerServiceClient(connPool)

	a, err := initializeAgent(profilerClient)
	if err != nil {
		debugLog("failed to start the profiling agent: %v", err)
		return err
	}
	go pollProfilerService(withXGoogHeader(ctx), a)
	return nil
}

// debugLog logs a message if DebugLogging is enabled.
// ... (debugLog function remains the same) ...
func debugLog(format string, e ...interface{}) {
	if config.DebugLogging {
		logger.Printf(format, e...)
	}
}

// agent polls the profiler server for instructions on behalf of a task,
// and collects and uploads profiles as requested.
// --- agent Struct (remains the same) ---
type agent struct {
	client          pb.ProfilerServiceClient
	deployment      *pb.Deployment
	profileLabels   map[string]string
	profileTypes    []pb.ProfileType
	bqClient        *bigquery.Client
	bqProjectID     string
	bqDatasetID     string
	bqDenormTableID string
}

// abortedBackoffDuration retrieves the retry duration from gRPC trailing metadata.
// ... (abortedBackoffDuration function remains the same as previous corrected version) ...
func abortedBackoffDuration(md grpcmd.MD) (time.Duration, error) {
	elem := md[retryInfoMetadata]
	if len(elem) <= 0 {
		return 0, errors.New("no retry info")
	}

	var retryInfo edpb.RetryInfo
	if err := proto.Unmarshal([]byte(elem[0]), &retryInfo); err != nil {
		return 0, err
	}
	d := retryInfo.GetRetryDelay().AsDuration()
	if d < 0 {
		return 0, errors.New("negative retry duration")
	}
	return d, nil
}

// retryer implements gax.Retryer for handling server-specified backoff.
// ... (retryer struct and Retry method remain the same) ...
type retryer struct {
	backoff gax.Backoff
	md      *grpcmd.MD
}

func (r *retryer) Retry(err error) (time.Duration, bool) {
	st, _ := status.FromError(err)
	if st != nil && st.Code() == codes.Aborted {
		dur, err := abortedBackoffDuration(*r.md)
		if err == nil {
			return dur, true
		}
		debugLog("failed to get backoff duration: %v", err)
	}
	return r.backoff.Pause(), true
}

// createProfile talks to the profiler server to create profile.
// ... (createProfile function remains the same as previous corrected version) ...
func (a *agent) createProfile(ctx context.Context) *pb.Profile {
	req := &pb.CreateProfileRequest{
		Parent:      "projects/" + a.deployment.ProjectId,
		Deployment:  a.deployment,
		ProfileType: a.profileTypes,
	}

	var p *pb.Profile
	md := grpcmd.New(nil)

	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		debugLog("creating a new profile via profiler service")
		var err error
		p, err = a.client.CreateProfile(ctx, req, grpc.Trailer(&md))
		if err != nil {
			debugLog("failed to create profile, will retry: %v", err)
			st, _ := status.FromError(err)
			if st != nil && strings.Contains(st.Message(), "x509: certificate signed by unknown authority") {
				err = fmt.Errorf("retry the certificate error: %w", err)
			}
		}
		return err
	}, gax.WithRetry(func() gax.Retryer {
		return &retryer{
			backoff: gax.Backoff{
				Initial:    initialBackoff,
				Max:        maxBackoff,
				Multiplier: backoffMultiplier,
			},
			md: &md,
		}
	}))

	if err != nil {
		log.Printf("Cloud Profiler: Failed to create profile via API after retries: %v", err)
		return nil
	}

	debugLog("successfully created profile %v", p.GetProfileType())
	return p
}

// profileAndUpload generates the profile, uploads to BQ (async), and uploads to Profiler API.
// --- profileAndUpload Function (remains the same as previous corrected version) ---
func (a *agent) profileAndUpload(ctx context.Context, p *pb.Profile) {
	uploadTime := time.Now().UTC() // Capture time for BQ record
	var profBuffer bytes.Buffer    // Buffer to hold profile bytes (assumed gzipped by pprof funcs)
	pt := p.GetProfileType()       // Use pb.ProfileType enum

	ptEnabled := false
	for _, enabled := range a.profileTypes {
		if enabled == pt {
			ptEnabled = true
			break
		}
	}

	if !ptEnabled {
		debugLog("skipping collection of disabled profile type: %v", pt)
		return
	}

	// --- Profile Generation ---
	var profileGenErr error
	var profileDuration time.Duration
	protoDuration := p.GetDuration()
	if protoDuration != nil {
		if err := protoDuration.CheckValid(); err != nil {
			debugLog("invalid duration format for profile type %v: %v", pt, err)
		} else {
			profileDuration = protoDuration.AsDuration()
			if profileDuration <= 0 && (pt == pb.ProfileType_CPU || pt == pb.ProfileType_HEAP_ALLOC || pt == pb.ProfileType_CONTENTION) {
				profileGenErr = fmt.Errorf("non-positive duration (%v) for profile type %v", profileDuration, pt)
				debugLog(profileGenErr.Error())
			}
		}
	} else if pt == pb.ProfileType_CPU || pt == pb.ProfileType_HEAP_ALLOC || pt == pb.ProfileType_CONTENTION {
		profileGenErr = fmt.Errorf("missing duration for profile type %v", pt)
		debugLog(profileGenErr.Error())
	}

	if profileGenErr == nil {
		switch pt {
		case pb.ProfileType_CPU:
			if err := startCPUProfile(&profBuffer); err != nil {
				profileGenErr = fmt.Errorf("failed to start CPU profile: %w", err)
			} else {
				sleep(ctx, profileDuration)
				stopCPUProfile()
			}
		case pb.ProfileType_HEAP:
			if err := writeHeapProfile(&profBuffer); err != nil {
				profileGenErr = fmt.Errorf("failed to write heap profile: %w", err)
			}
		case pb.ProfileType_HEAP_ALLOC:
			// ASSUMPTION: deltaAllocProfile exists elsewhere in the package
			if err := deltaAllocProfile(ctx, profileDuration, config.AllocForceGC, &profBuffer); err != nil {
				profileGenErr = fmt.Errorf("failed to collect allocation profile: %w", err)
			}
		case pb.ProfileType_THREADS:
			if err := pprof.Lookup("goroutine").WriteTo(&profBuffer, 0); err != nil {
				profileGenErr = fmt.Errorf("failed to collect goroutine profile: %w", err)
			}
		case pb.ProfileType_CONTENTION:
			if !mutexEnabled {
				profileGenErr = errors.New("mutex profiling is not enabled for contention profile type")
			} else if err := deltaMutexProfile(ctx, profileDuration, &profBuffer); err != nil {
				profileGenErr = fmt.Errorf("failed to collect mutex profile: %w", err)
			}
		default:
			profileGenErr = fmt.Errorf("unexpected profile type: %v", pt)
		}
	}

	if profileGenErr != nil {
		debugLog("Profile generation failed for type %v: %v. Skipping uploads.", pt, profileGenErr)
		return
	}

	gzippedProfileBytes := profBuffer.Bytes()
	if len(gzippedProfileBytes) == 0 {
		debugLog("Profile generation resulted in empty bytes for type %v. Skipping uploads.", pt)
		return
	}

	if a.bqClient != nil && a.bqDatasetID != "" && a.bqDenormTableID != "" && a.bqProjectID != "" {
		go a.parseAndUploadToBigQuery(ctx, pt.String(), uploadTime, gzippedProfileBytes)
	} else {
		debugLog("BigQuery client/config not set for table %s, skipping BQ upload.", a.bqDenormTableID)
	}

	finalLabels := a.profileLabels
	if p.GetLabels() != nil {
		if finalLabels == nil {
			finalLabels = make(map[string]string)
		}
		for k, v := range p.GetLabels() {
			finalLabels[k] = v
		}
	}

	p.ProfileBytes = gzippedProfileBytes
	p.Labels = finalLabels

	req := &pb.UpdateProfileRequest{Profile: p}

	debugLog("Starting upload profile via Cloud Profiler API...")
	if _, err := a.client.UpdateProfile(ctx, req); err != nil {
		debugLog("Failed to upload profile via Cloud Profiler API: %v", err)
	} else {
		debugLog("Successfully uploaded profile via Cloud Profiler API")
	}
}

// parseAndUploadToBigQuery runs in a goroutine to handle BQ processing.
// ... (parseAndUploadToBigQuery function remains the same as previous corrected version) ...
func (a *agent) parseAndUploadToBigQuery(ctx context.Context, profileTypeStr string, uploadTime time.Time, gzippedBytes []byte) {
	// 1. Decompress
	gzReader, err := gzip.NewReader(bytes.NewReader(gzippedBytes))
	if err != nil {
		debugLog("[BQ Upload] Failed to create gzip reader: %v. Skipping BQ upload.", err)
		return
	}
	decompressedBytes, err := io.ReadAll(gzReader)
	gzReader.Close()
	if err != nil {
		debugLog("[BQ Upload] Failed to decompress profile bytes: %v. Skipping BQ upload.", err)
		return
	}
	if len(decompressedBytes) == 0 {
		debugLog("[BQ Upload] Decompressed profile bytes are empty. Skipping BQ upload.")
		return
	}

	// 2. Unmarshal using pprof_pb.ParseData
	parsedProfile, err := pprof_pb.ParseData(decompressedBytes)
	if err != nil {
		debugLog("[BQ Upload] Failed to parse decompressed profile data: %v. Skipping BQ upload.", err)
		return
	}
	debugLog("[BQ Upload] Successfully parsed profile proto for type %s.", profileTypeStr)

	// 3. Denormalize into BigQueryProfileRow
	bqRow, err := a.denormalizeProfile(parsedProfile, profileTypeStr, uploadTime)
	if err != nil {
		debugLog("[BQ Upload] Failed during denormalization: %v. Skipping BQ upload.", err)
		return
	}

	// 4. Upload to BigQuery
	inserter := a.bqClient.DatasetInProject(a.bqProjectID, a.bqDatasetID).Table(a.bqDenormTableID).Inserter()
	itemsToInsert := []*BigQueryProfileRow{bqRow}

	insertCtx, cancel := context.WithTimeout(context.Background(), bqUploadTimeout)
	defer cancel()

	if err := inserter.Put(insertCtx, itemsToInsert); err != nil {
		logBigQueryError(err, a.bqDenormTableID)
	} else {
		debugLog("[BQ Upload] Successfully uploaded denormalized profile %s to BigQuery table %s", bqRow.ProfileUUID, a.bqDenormTableID)
	}
}

// denormalizeProfile converts a parsed pprof_pb.Profile into a BigQueryProfileRow.
// --- MODIFIED denormalizeProfile Function ---
func (a *agent) denormalizeProfile(p *pprof_pb.Profile, profileTypeStr string, uploadTime time.Time) (*BigQueryProfileRow, error) {
	profileUUID := uuid.New().String()

	row := &BigQueryProfileRow{
		ProfileUUID:         profileUUID,
		UploadTimestamp:     uploadTime,
		ProfileType:         profileTypeStr,
		DurationNanos:       p.DurationNanos,
		Period:              p.Period,
		DeploymentProjectID: a.deployment.GetProjectId(),
		DeploymentTarget:    a.deployment.GetTarget(),
		DeploymentLabels:    mapToLabelPairs(a.deployment.GetLabels()),
		ProfileLabels:       mapToLabelPairs(a.profileLabels),

		// Access already resolved string fields from parsed profile
		// Comment field is not directly accessible/exported in parsed profile.Profile
		Comment:           []string{},          // Assign empty slice
		DefaultSampleType: p.DefaultSampleType, // Direct access (string)
		DropFramesPattern: p.DropFrames,        // Direct access (string)
		KeepFramesPattern: p.KeepFrames,        // Direct access (string)
		DocURL:            "",                  // No direct DocURL field
	}

	// TimeNanos
	if p.TimeNanos != 0 {
		t := time.Unix(0, p.TimeNanos).UTC()
		row.TimeNanos = bigquery.NullTimestamp{Timestamp: t, Valid: true}
	} else {
		row.TimeNanos = bigquery.NullTimestamp{Valid: false}
	}

	// PeriodType
	if p.PeriodType != nil {
		row.PeriodType = &BQValueType{
			Type: p.PeriodType.Type,
			Unit: p.PeriodType.Unit,
		}
	}

	// SampleType
	if len(p.SampleType) > 0 {
		row.SampleType = make([]BQValueType, len(p.SampleType))
		for i, st := range p.SampleType {
			row.SampleType[i] = BQValueType{
				Type: st.Type,
				Unit: st.Unit,
			}
		}
	} else {
		row.SampleType = []BQValueType{}
	}

	// Sample
	if len(p.Sample) > 0 {
		row.Sample = make([]BQSample, len(p.Sample))
		for i, s := range p.Sample {
			bqSample := BQSample{
				Value: s.Value,
			}
			// Location IDs
			if len(s.Location) > 0 {
				bqSample.LocationID = make([]int64, len(s.Location))
				for k, loc := range s.Location {
					if loc != nil {
						bqSample.LocationID[k] = int64(loc.ID)
					}
				}
			} else {
				bqSample.LocationID = []int64{}
			}
			// Value
			if s.Value == nil {
				bqSample.Value = []int64{}
			}
			// Label
			if len(s.Label) > 0 {
				bqSample.Label = make([]BQLabel, 0, len(s.Label))
				for key, values := range s.Label {
					if len(values) > 0 {
						bqSample.Label = append(bqSample.Label, BQLabel{
							Key: key,
							Str: values[0],
						})
					}
				}
			} else {
				bqSample.Label = []BQLabel{}
			}
			row.Sample[i] = bqSample
		}
	} else {
		row.Sample = []BQSample{}
	}

	// Mapping
	if len(p.Mapping) > 0 {
		row.Mapping = make([]BQMapping, len(p.Mapping))
		for i, m := range p.Mapping {
			row.Mapping[i] = BQMapping{
				ID:              int64(m.ID),
				MemoryStart:     int64(m.Start),
				MemoryLimit:     int64(m.Limit),
				FileOffset:      int64(m.Offset),
				Filename:        m.File,
				BuildID:         m.BuildID,
				HasFunctions:    m.HasFunctions,
				HasFilenames:    m.HasFilenames,
				HasLineNumbers:  m.HasLineNumbers, // Correct field name
				HasInlineFrames: m.HasInlineFrames,
			}
		}
	} else {
		row.Mapping = []BQMapping{}
	}

	// Location
	if len(p.Location) > 0 {
		row.Location = make([]BQLocation, len(p.Location))
		for i, l := range p.Location {
			bqLocation := BQLocation{
				ID:        int64(l.ID),
				MappingID: 0,
				Address:   int64(l.Address),
				IsFolded:  l.IsFolded,
			}
			if l.Mapping != nil {
				bqLocation.MappingID = int64(l.Mapping.ID)
			}
			// Line
			if len(l.Line) > 0 {
				bqLocation.Line = make([]BQLine, len(l.Line))
				for j, ln := range l.Line {
					bqLine := BQLine{
						FunctionID: 0,
						Line:       ln.Line,
						Column:     0,
					}
					if ln.Function != nil {
						bqLine.FunctionID = int64(ln.Function.ID)
					}
					bqLocation.Line[j] = bqLine
				}
			} else {
				bqLocation.Line = []BQLine{}
			}
			row.Location[i] = bqLocation
		}
	} else {
		row.Location = []BQLocation{}
	}

	// Function
	if len(p.Function) > 0 {
		row.Function = make([]BQFunction, len(p.Function))
		for i, f := range p.Function {
			row.Function[i] = BQFunction{
				ID:         int64(f.ID),
				Name:       f.Name,
				SystemName: f.SystemName,
				Filename:   f.Filename,
				StartLine:  f.StartLine,
			}
		}
	} else {
		row.Function = []BQFunction{}
	}

	if row.DeploymentLabels == nil {
		row.DeploymentLabels = []BQKeyValue{}
	}
	if row.ProfileLabels == nil {
		row.ProfileLabels = []BQKeyValue{}
	}

	debugLog("[Denormalize] Denormalization complete for profile %s.", profileUUID)
	return row, nil
}

// logBigQueryError logs BQ insertion errors.
// ... (logBigQueryError function remains the same) ...
func logBigQueryError(err error, tableID string) {
	if multiErr, ok := err.(bigquery.PutMultiError); ok {
		for _, rowErr := range multiErr {
			debugLog("[BQ Upload] BigQuery row insertion error for table %s: index %d, errors: %v", tableID, rowErr.RowIndex, rowErr.Errors)
		}
		debugLog("[BQ Upload] Failed to insert some rows into BigQuery table %s.", tableID)
	} else {
		debugLog("[BQ Upload] Failed to upload profile to BigQuery table %s: %v", tableID, err)
	}
}

// mapToLabelPairs converts a map to BQ KeyValue slice.
// ... (mapToLabelPairs function remains the same) ...
func mapToLabelPairs(m map[string]string) []BQKeyValue {
	if len(m) == 0 {
		return []BQKeyValue{}
	}
	pairs := make([]BQKeyValue, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, BQKeyValue{Key: k, Value: v})
	}
	return pairs
}

// deltaMutexProfile computes and writes the delta mutex profile.
// ... (deltaMutexProfile function remains the same as previous corrected version) ...
func deltaMutexProfile(ctx context.Context, duration time.Duration, prof *bytes.Buffer) error {
	if !mutexEnabled {
		return errors.New("mutex profiling is not enabled")
	}
	p0, err := mutexProfile()
	if err != nil {
		return fmt.Errorf("failed to get initial mutex profile: %w", err)
	}
	sleep(ctx, duration)
	p, err := mutexProfile()
	if err != nil {
		return fmt.Errorf("failed to get final mutex profile: %w", err)
	}

	p0.Scale(-1)
	mergedProfile, err := pprof_pb.Merge([]*pprof_pb.Profile{p0, p})
	if err != nil {
		return fmt.Errorf("failed to merge mutex profiles: %w", err)
	}
	if err := mergedProfile.Write(prof); err != nil {
		return fmt.Errorf("failed to write merged mutex profile: %w", err)
	}
	return nil
}

// mutexProfile collects and parses the current mutex profile using pprof_pb.Parse.
// ... (mutexProfile function remains the same as previous corrected version) ...
func mutexProfile() (*pprof_pb.Profile, error) {
	lookup := pprof.Lookup("mutex")
	if lookup == nil {
		return nil, errors.New("mutex profiling is not supported by this Go version or is disabled")
	}
	var buf bytes.Buffer
	if err := lookup.WriteTo(&buf, 0); err != nil {
		return nil, fmt.Errorf("failed to write mutex profile data: %w", err)
	}
	parsedProfile, err := pprof_pb.Parse(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mutex profile data: %w", err)
	}
	return parsedProfile, nil
}

// REMOVED deltaAllocProfile definition (assuming it exists elsewhere)

// withXGoogHeader sets the x-goog-api-client header.
// --- MODIFIED withXGoogHeader Function ---
func withXGoogHeader(ctx context.Context, keyval ...string) context.Context {
	// Use runtime.Version() for Go version
	kv := append([]string{"gl-go", runtime.Version(), "gccl", agentVersion}, keyval...)
	kv = append(kv, "gax", gax.Version, "grpc", grpc.Version)

	md, _ := grpcmd.FromOutgoingContext(ctx)
	md = md.Copy()
	md[xGoogAPIMetadata] = []string{gax.XGoogHeader(kv...)}
	return grpcmd.NewOutgoingContext(ctx, md)
}

// initializeAgent initializes the agent state, including BQ client.
// ... (initializeAgent function remains the same as previous corrected version) ...
func initializeAgent(c pb.ProfilerServiceClient) (*agent, error) {
	labels := map[string]string{languageLabel: "go"}
	if config.Zone != "" {
		labels[zoneNameLabel] = config.Zone
	}
	if config.ServiceVersion != "" {
		labels[versionLabel] = config.ServiceVersion
	}
	d := &pb.Deployment{
		ProjectId: config.ProjectID,
		Target:    config.Service,
		Labels:    labels,
	}

	profileLabels := map[string]string{}
	if config.Instance != "" {
		profileLabels[instanceLabel] = config.Instance
	}

	var profileTypes []pb.ProfileType
	if !config.NoCPUProfiling {
		profileTypes = append(profileTypes, pb.ProfileType_CPU)
	}
	if !config.NoHeapProfiling {
		profileTypes = append(profileTypes, pb.ProfileType_HEAP)
	}
	if !config.NoGoroutineProfiling {
		profileTypes = append(profileTypes, pb.ProfileType_THREADS)
	}
	if !config.NoAllocProfiling {
		profileTypes = append(profileTypes, pb.ProfileType_HEAP_ALLOC)
	}
	if mutexEnabled {
		profileTypes = append(profileTypes, pb.ProfileType_CONTENTION)
	}

	if len(profileTypes) == 0 {
		return nil, fmt.Errorf("collection is not enabled for any profile types")
	}

	var bqClient *bigquery.Client
	var bqErr error
	bqProjectID := config.ProjectID

	if bqProjectID == "" {
		log.Println("Cloud Profiler: Warning: ProjectID is empty. BigQuery uploads will be disabled.")
		bqClient = nil
	} else {
		bqClient, bqErr = bigquery.NewClient(context.Background(), bqProjectID)
		if bqErr != nil {
			log.Printf("Cloud Profiler: Warning: Failed to initialize BigQuery client for project %s: %v. BigQuery uploads will be disabled.", bqProjectID, bqErr)
			bqClient = nil
		} else {
			debugLog("BigQuery client initialized successfully for project %s.", bqProjectID)
		}
	}

	bqDatasetID := bqDenormDatasetIDDefault
	bqDenormTableID := bqDenormTableIDDefault

	return &agent{
		client:          c,
		deployment:      d,
		profileLabels:   profileLabels,
		profileTypes:    profileTypes,
		bqClient:        bqClient,
		bqProjectID:     bqProjectID,
		bqDatasetID:     bqDatasetID,
		bqDenormTableID: bqDenormTableID,
	}, nil
}

// initializeConfig resolves configuration details.
// ... (initializeConfig function remains the same as previous corrected version) ...
func initializeConfig(cfg Config) error {
	config = cfg

	if config.Service == "" {
		for _, ev := range []string{"GAE_SERVICE", "K_SERVICE"} {
			if val := os.Getenv(ev); val != "" {
				config.Service = val
				break
			}
		}
	}
	if config.Service == "" {
		return errors.New("service name must be configured")
	}
	if !serviceRegexp.MatchString(config.Service) {
		return fmt.Errorf("service name %q does not match regular expression %q", config.Service, serviceRegexp.String())
	}

	if config.ServiceVersion == "" {
		for _, ev := range []string{"GAE_VERSION", "K_REVISION"} {
			if val := os.Getenv(ev); val != "" {
				config.ServiceVersion = val
				break
			}
		}
	}

	if config.ProjectID == "" {
		if projectIDEnv := os.Getenv("GOOGLE_CLOUD_PROJECT"); projectIDEnv != "" {
			config.ProjectID = projectIDEnv
			log.Printf("Cloud Profiler: Using ProjectID from GOOGLE_CLOUD_PROJECT env var: %s", config.ProjectID)
		}
	}

	if onGCE() {
		var err error
		if config.ProjectID == "" {
			config.ProjectID, err = getProjectID()
			if err != nil {
				return fmt.Errorf("failed to get project ID from Compute Engine metadata (and not set via config/env): %w", err)
			}
			log.Printf("Cloud Profiler: Using ProjectID from GCE metadata server: %s", config.ProjectID)
		}

		if config.Zone == "" {
			config.Zone, err = getZone()
			if err != nil {
				log.Printf("Cloud Profiler: Warning: Failed to get zone from Compute Engine metadata: %v", err)
				config.Zone = "unknown"
			}
		}

		if config.Instance == "" {
			if instance, err := getInstanceName(); err != nil {
				if _, ok := err.(gcemd.NotDefinedError); !ok && !strings.Contains(err.Error(), "not defined") { // Check common variations
					log.Printf("Cloud Profiler: Warning: Failed to get instance name from Compute Engine metadata: %v", err)
				}
			} else {
				config.Instance = instance
			}
		}
	} else {
		if config.ProjectID == "" {
			return fmt.Errorf("project ID must be specified in the configuration or via GOOGLE_CLOUD_PROJECT env var if running outside of GCP")
		}
	}

	if config.APIAddr == "" {
		config.APIAddr = apiAddress
	}

	return nil
}

// pollProfilerService runs the main agent loop.
// --- MODIFIED pollProfilerService Function ---
func pollProfilerService(ctx context.Context, a *agent) {
	debugLog("Cloud Profiler Go Agent version: %s", agentVersion)
	debugLog("Profiler has started polling loop.")

	profileCount := 0
	for i := 0; config.numProfiles == 0 || i < config.numProfiles; i++ {
		if ctx.Err() != nil {
			debugLog("Context cancelled before creating profile, exiting poll loop.")
			break
		}

		p := a.createProfile(ctx)
		if p != nil {
			if ctx.Err() != nil {
				debugLog("Context cancelled before uploading profile, exiting poll loop.")
				break
			}
			a.profileAndUpload(ctx, p)
			profileCount = i + 1
		} else {
			log.Printf("Cloud Profiler: Failed to create profile after retries. Continuing poll loop.")
			select {
			case <-time.After(initialBackoff):
			case <-ctx.Done():
				debugLog("Context cancelled during backoff sleep after createProfile failure.")
				break // Exit inner select and outer loop check will catch cancellation
			}
		}
		if ctx.Err() != nil {
			debugLog("Context cancelled, exiting poll loop.")
			break
		}
	}

	if a.bqClient != nil {
		debugLog("Closing BigQuery client...")
		// Remove unused context for Close()
		if err := a.bqClient.Close(); err != nil {
			log.Printf("Cloud Profiler: Error closing BigQuery client: %v", err)
		} else {
			debugLog("BigQuery client closed.")
		}
	}

	logMsg := "Profiler has stopped polling loop."
	if config.numProfiles > 0 {
		logMsg = fmt.Sprintf("Profiler has stopped polling loop after %d profiles.", profileCount)
	}
	debugLog(logMsg)

	if profilingDone != nil {
		profilingDone <- true
	}
}
