// package profiler

// import (
// 	"bytes" // Added: potentially needed if we need to decompress bytes first
// 	"compress/gzip"
// 	"context"
// 	"errors"
// 	"fmt"
// 	"io"
// 	"log"
// 	"os"
// 	"regexp"
// 	"runtime"
// 	"runtime/pprof" // Correct import for pprof functions
// 	"strings"
// 	"sync"
// 	"time"

// 	"cloud.google.com/go/bigquery"
// 	gcemd "cloud.google.com/go/compute/metadata" // Renamed for clarity
// 	"cloud.google.com/go/internal/version"
// 	"cloud.google.com/go/profiler/internal"

// 	// --- ADDED: Import pprof profile proto definitions ---
// 	// "github.com/google/pprof/profile" // For parsing profile bytes
// 	pprof_pb "cloud.google.com/go/profiler/internal/pprof_proto" // Check this line carefully

// 	"github.com/google/pprof/profile"
// 	"github.com/googleapis/gax-go/v2"
// 	"google.golang.org/api/option"
// 	gtransport "google.golang.org/api/transport/grpc" // Renamed for clarity
// 	pb "google.golang.org/genproto/googleapis/devtools/cloudprofiler/v2"
// 	edpb "google.golang.org/genproto/googleapis/rpc/errdetails"
// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/codes"
// 	grpcmd "google.golang.org/grpc/metadata" // Renamed for clarity
// 	"google.golang.org/grpc/status"

// 	// --- ADDED: Import for protojson ---
// 	"google.golang.org/protobuf/encoding/protojson"
// 	"google.golang.org/protobuf/proto"
// 	"google.golang.org/protobuf/types/known/durationpb"
// )

// // --- BigQuery Struct Definitions ---

// type LabelPair struct {
// 	Key   string `bigquery:"key"`
// 	Value string `bigquery:"value"`
// }

// type DeploymentBQRecord struct {
// 	ProjectID string      `bigquery:"project_id"`
// 	Target    string      `bigquery:"target"`
// 	Labels    []LabelPair `bigquery:"labels"`
// }

// type DurationBQRecord struct {
// 	Seconds int64 `bigquery:"seconds"`
// 	Nanos   int32 `bigquery:"nanos"`
// }

// // ProfileBQRecord matches the 'profiler_data' BigQuery table schema.
// type ProfileBQRecord struct {
// 	Name         string              `bigquery:"name"`
// 	ProfileType  string              `bigquery:"profile_type"`
// 	Deployment   *DeploymentBQRecord `bigquery:"deployment"`
// 	Duration     *DurationBQRecord   `bigquery:"duration"`
// 	ProfileBytes []byte              `bigquery:"profile_bytes"` // Gzipped profile bytes
// 	Labels       []LabelPair         `bigquery:"labels"`
// 	StartTime    time.Time           `bigquery:"start_time"`
// }

// // ProfileJsonBQRecord matches the 'profiler_json_data' BigQuery table schema.
// type ProfileJsonBQRecord struct {
// 	Name        string              `bigquery:"name"`
// 	ProfileType string              `bigquery:"profile_type"`
// 	Deployment  *DeploymentBQRecord `bigquery:"deployment"`
// 	Duration    *DurationBQRecord   `bigquery:"duration"`
// 	Labels      []LabelPair         `bigquery:"labels"`
// 	StartTime   time.Time           `bigquery:"start_time"`
// 	ProfileJson string              `bigquery:"profile_json"` // Profile as JSON string
// }

// // --- CORRECTED: Struct Definitions for Nested BigQuery Schema using Null Types ---

// // ProfileNestedBQRecord remains the same top-level structure
// type ProfileNestedBQRecord struct {
// 	Name           string                  `bigquery:"name"`
// 	ProfileType    string                  `bigquery:"profile_type"`
// 	Deployment     *DeploymentBQRecord     `bigquery:"deployment"`
// 	Duration       *DurationBQRecord       `bigquery:"duration"`
// 	Labels         []LabelPair             `bigquery:"labels"`
// 	StartTime      time.Time               `bigquery:"start_time"` // time.Time maps directly to NULLABLE TIMESTAMP
// 	ProfileDetails *ProfileDetailsBQRecord `bigquery:"profile_details"`
// }

// // ProfileDetailsBQRecord mirrors the structure of profile.proto:Profile message.
// type ProfileDetailsBQRecord struct {
// 	SampleType        []ValueTypeBQRecord `bigquery:"sample_type"`
// 	Sample            []SampleBQRecord    `bigquery:"sample"`
// 	Mapping           []MappingBQRecord   `bigquery:"mapping"`
// 	Location          []LocationBQRecord  `bigquery:"location"`
// 	Function          []FunctionBQRecord  `bigquery:"function"`
// 	StringTable       []string            `bigquery:"string_table"` // Slice handles REPEATED
// 	DropFrames        bigquery.NullInt64  `bigquery:"drop_frames"`  // Use NullInt64 for NULLABLE INTEGER
// 	KeepFrames        bigquery.NullInt64  `bigquery:"keep_frames"`
// 	TimeNanos         bigquery.NullInt64  `bigquery:"time_nanos"`
// 	DurationNanos     bigquery.NullInt64  `bigquery:"duration_nanos"`
// 	PeriodType        *ValueTypeBQRecord  `bigquery:"period_type"` // Pointer for NULLABLE RECORD is fine
// 	Period            bigquery.NullInt64  `bigquery:"period"`
// 	Comment           []int64             `bigquery:"comment"` // Slice handles REPEATED
// 	DefaultSampleType bigquery.NullInt64  `bigquery:"default_sample_type"`
// 	DocURL            bigquery.NullInt64  `bigquery:"doc_url"`
// }

// // ValueTypeBQRecord mirrors profile.proto:ValueType.
// type ValueTypeBQRecord struct {
// 	Type bigquery.NullInt64 `bigquery:"type"` // Index into string_table
// 	Unit bigquery.NullInt64 `bigquery:"unit"` // Index into string_table
// }

// // SampleBQRecord mirrors profile.proto:Sample.
// type SampleBQRecord struct {
// 	LocationID []int64         `bigquery:"location_id"` // Slice handles REPEATED
// 	Value      []int64         `bigquery:"value"`       // Slice handles REPEATED
// 	Label      []LabelBQRecord `bigquery:"label"`       // Slice handles REPEATED RECORD
// }

// // LabelBQRecord mirrors profile.proto:Label.
// type LabelBQRecord struct {
// 	Key     bigquery.NullInt64 `bigquery:"key"` // Index into string_table
// 	Str     bigquery.NullInt64 `bigquery:"str"` // Index into string_table
// 	Num     bigquery.NullInt64 `bigquery:"num"`
// 	NumUnit bigquery.NullInt64 `bigquery:"num_unit"` // Index into string_table
// }

// // MappingBQRecord mirrors profile.proto:Mapping.
// type MappingBQRecord struct {
// 	ID              bigquery.NullInt64 `bigquery:"id"`
// 	MemoryStart     bigquery.NullInt64 `bigquery:"memory_start"`
// 	MemoryLimit     bigquery.NullInt64 `bigquery:"memory_limit"`
// 	FileOffset      bigquery.NullInt64 `bigquery:"file_offset"`
// 	Filename        bigquery.NullInt64 `bigquery:"filename"`      // Index into string_table
// 	BuildID         bigquery.NullInt64 `bigquery:"build_id"`      // Index into string_table
// 	HasFunctions    bigquery.NullBool  `bigquery:"has_functions"` // Use NullBool for NULLABLE BOOLEAN
// 	HasFilenames    bigquery.NullBool  `bigquery:"has_filenames"`
// 	HasLineNumbers  bigquery.NullBool  `bigquery:"has_line_numbers"`
// 	HasInlineFrames bigquery.NullBool  `bigquery:"has_inline_frames"`
// }

// // LocationBQRecord mirrors profile.proto:Location.
// type LocationBQRecord struct {
// 	ID        bigquery.NullInt64 `bigquery:"id"`
// 	MappingID bigquery.NullInt64 `bigquery:"mapping_id"`
// 	Address   bigquery.NullInt64 `bigquery:"address"`
// 	Line      []LineBQRecord     `bigquery:"line"` // Slice handles REPEATED RECORD
// 	IsFolded  bigquery.NullBool  `bigquery:"is_folded"`
// }

// // LineBQRecord mirrors profile.proto:Line.
// type LineBQRecord struct {
// 	FunctionID bigquery.NullInt64 `bigquery:"function_id"`
// 	Line       bigquery.NullInt64 `bigquery:"line"`
// 	Column     bigquery.NullInt64 `bigquery:"column"`
// }

// // FunctionBQRecord mirrors profile.proto:Function.
// type FunctionBQRecord struct {
// 	ID         bigquery.NullInt64 `bigquery:"id"`
// 	Name       bigquery.NullInt64 `bigquery:"name"`        // Index into string_table
// 	SystemName bigquery.NullInt64 `bigquery:"system_name"` // Index into string_table
// 	Filename   bigquery.NullInt64 `bigquery:"filename"`    // Index into string_table
// 	StartLine  bigquery.NullInt64 `bigquery:"start_line"`
// }

// // Helper function to convert map[string]string to the BigQuery []LabelPair format.
// func mapToLabelPairs(m map[string]string) []LabelPair {
// 	if len(m) == 0 {
// 		return nil
// 	}
// 	pairs := make([]LabelPair, 0, len(m))
// 	for k, v := range m {
// 		pairs = append(pairs, LabelPair{Key: k, Value: v})
// 	}
// 	// sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key }) // Optional sort
// 	return pairs
// }

// // --- Global Variables & Constants ---

// var (
// 	config           Config
// 	startOnce        allowUntilSuccess
// 	mutexEnabled     bool
// 	logger           *log.Logger
// 	getProjectID     = gcemd.ProjectID
// 	getInstanceName  = gcemd.InstanceName
// 	getZone          = gcemd.Zone
// 	startCPUProfile  = pprof.StartCPUProfile
// 	stopCPUProfile   = pprof.StopCPUProfile
// 	writeHeapProfile = pprof.WriteHeapProfile
// 	sleep            = gax.Sleep
// 	dialGRPC         = gtransport.DialPool
// 	onGCE            = gcemd.OnGCE
// 	serviceRegexp    = regexp.MustCompile(`^[a-z0-9]([-a-z0-9_.]{0,253}[a-z0-9])?$`)
// 	profilingDone    chan bool // For testing only
// )

// const (
// 	apiAddress             = "cloudprofiler.googleapis.com:443"
// 	xGoogAPIMetadata       = "x-goog-api-client"
// 	zoneNameLabel          = "zone"
// 	versionLabel           = "version"
// 	languageLabel          = "language"
// 	instanceLabel          = "instance"
// 	scope                  = "https://www.googleapis.com/auth/monitoring.write"
// 	initialBackoff         = time.Minute
// 	maxBackoff             = time.Hour
// 	backoffMultiplier      = 1.3
// 	retryInfoMetadata      = "google.rpc.retryinfo-bin"
// 	bqDatasetIDDefault     = "jatinagarwala"             // Default dataset
// 	bqTableIDDefault       = "profiler_data"             // Default table for raw bytes
// 	bqJsonTableIDDefault   = "profiler_json_data"        // Default table for JSON
// 	bqNestedTableIDDefault = "profiler_nested_json_data" // Default table for nested structure
// 	bqInsertTimeout        = 30 * time.Second            // Timeout for BQ insert calls
// )

// // --- Configuration ---

// type Config struct {
// 	Service              string    // Mandatory service name
// 	ServiceVersion       string    // Optional service version
// 	DebugLogging         bool      // Enable detailed agent logging
// 	DebugLoggingOutput   io.Writer // Where to write debug logs (default: os.Stderr)
// 	MutexProfiling       bool      // Enable mutex profiling (requires Go 1.8+)
// 	NoCPUProfiling       bool      // Disable CPU profiling
// 	NoAllocProfiling     bool      // Disable allocation profiling
// 	AllocForceGC         bool      // Force GC before alloc profile collection
// 	NoHeapProfiling      bool      // Disable heap profiling
// 	NoGoroutineProfiling bool      // Disable goroutine profiling
// 	EnableOCTelemetry    bool      // Enable OpenCensus telemetry (default: false)
// 	ProjectID            string    // GCP Project ID (override auto-detection)
// 	APIAddr              string    // Profiler API endpoint override (for testing)
// 	Instance             string    // Instance name override (for testing)
// 	Zone                 string    // Zone name override (for testing)
// 	numProfiles          int       // Number of profiles to collect before exiting (for testing)
// }

// // --- Initialization and Control ---

// type allowUntilSuccess struct {
// 	m    sync.Mutex
// 	done uint32
// }

// // do ensures f is called only once successfully.
// func (o *allowUntilSuccess) do(f func() error) (err error) {
// 	o.m.Lock()
// 	defer o.m.Unlock()
// 	if o.done == 0 {
// 		if err = f(); err == nil {
// 			o.done = 1
// 		}
// 	} else {
// 		debugLog("profiler.Start() called again after successful start")
// 	}
// 	return err
// }

// // Start initializes and starts the profiling agent in a background goroutine.
// // It should only be called once; subsequent calls are ignored.
// func Start(cfg Config, options ...option.ClientOption) error {
// 	startError := startOnce.do(func() error {
// 		return start(cfg, options...)
// 	})
// 	return startError
// }

// // start performs the actual initialization and starts the agent goroutine.
// func start(cfg Config, options ...option.ClientOption) error {
// 	if cfg.DebugLoggingOutput == nil {
// 		cfg.DebugLoggingOutput = os.Stderr
// 	}
// 	if cfg.DebugLogging {
// 		logger = log.New(cfg.DebugLoggingOutput, "Cloud Profiler [Debug]: ", log.LstdFlags|log.Lmicroseconds)
// 	}

// 	if err := initializeConfig(cfg); err != nil {
// 		debugLog("failed to initialize config: %v", err)
// 		return err
// 	}
// 	if config.MutexProfiling {
// 		if mutexEnabled = enableMutexProfiling(); !mutexEnabled {
// 			return fmt.Errorf("mutex profiling is not supported by %s, requires Go 1.8 or later", runtime.Version())
// 		}
// 	}

// 	ctx := context.Background()
// 	opts := buildGRPCOptions(options...)
// 	connPool, err := dialGRPC(ctx, opts...)
// 	if err != nil {
// 		debugLog("failed to dial GRPC: %v", err)
// 		return err
// 	}

// 	a, err := initializeAgent(pb.NewProfilerServiceClient(connPool))
// 	if err != nil {
// 		debugLog("failed to start the profiling agent: %v", err)
// 		// Close BQ client if agent init failed after client was created
// 		if a != nil && a.bqClient != nil {
// 			a.bqClient.Close()
// 		}
// 		return err
// 	}

// 	go pollProfilerService(withXGoogHeader(ctx), a)
// 	return nil
// }

// // buildGRPCOptions creates the list of gRPC client options.
// func buildGRPCOptions(additionalOpts ...option.ClientOption) []option.ClientOption {
// 	opts := []option.ClientOption{
// 		option.WithEndpoint(config.APIAddr),
// 		option.WithScopes(scope),
// 		option.WithUserAgent(fmt.Sprintf("gcloud-go-profiler/%s", internal.Version)),
// 	}
// 	if !config.EnableOCTelemetry {
// 		opts = append(opts, option.WithTelemetryDisabled())
// 	}
// 	opts = append(opts, additionalOpts...)
// 	return opts
// }

// // --- Modified debugLog Function ---
// // (Added check for logger != nil)
// func debugLog(format string, e ...interface{}) {
// 	// Check if DebugLogging is enabled AND logger is initialized
// 	if config.DebugLogging && logger != nil {
// 		logger.Printf(format, e...)
// 	}
// }

// // --- Agent Logic ---

// type agent struct {
// 	client        pb.ProfilerServiceClient
// 	deployment    *pb.Deployment
// 	profileLabels map[string]string
// 	profileTypes  []pb.ProfileType

// 	bqClient        *bigquery.Client
// 	bqProjectID     string
// 	bqDatasetID     string
// 	bqTableID       string // For raw bytes table
// 	bqJsonTableID   string // For JSON table
// 	bqNestedTableID string // Table ID for the new nested table
// }

// // pollProfilerService is the main loop that polls the Profiler API,
// // collects profiles, and uploads them.
// func pollProfilerService(ctx context.Context, a *agent) {
// 	debugLog("Cloud Profiler Go Agent version: %s, targeting project: %s", internal.Version, a.bqProjectID)
// 	debugLog("profiler has started")

// 	for i := 0; config.numProfiles == 0 || i < config.numProfiles; i++ {
// 		select {
// 		case <-ctx.Done():
// 			debugLog("profiler poll loop cancelled: %v", ctx.Err())
// 			goto cleanup
// 		default:
// 			// Continue polling
// 		}

// 		p := a.createProfile(ctx)
// 		if p == nil {
// 			debugLog("createProfile returned nil, potentially due to unrecoverable error during retry")
// 			// Depending on Invoke behavior, this might indicate context cancellation too
// 			continue // Or break? Needs check on how createProfile handles ctx cancellation.
// 		}
// 		a.collectAndUploadProfile(ctx, p)
// 	}

// cleanup:
// 	// Close BigQuery client gracefully on exit
// 	if a.bqClient != nil {
// 		debugLog("Closing BigQuery client...")
// 		if err := a.bqClient.Close(); err != nil {
// 			log.Printf("Cloud Profiler: Error closing BigQuery client: %v", err) // Use standard logger for shutdown errors
// 		} else {
// 			debugLog("BigQuery client closed.")
// 		}
// 	}

// 	numProfilesCollected := config.numProfiles
// 	if config.numProfiles == 0 {
// 		numProfilesCollected = -1 // Indicate indefinite run ended
// 	}
// 	debugLog("profiler has stopped. Profiles collected (approx): %d", numProfilesCollected)
// 	if profilingDone != nil {
// 		profilingDone <- true
// 	}
// }

// // createProfile requests profiling instructions from the Profiler API with retry logic.
// func (a *agent) createProfile(ctx context.Context) *pb.Profile {
// 	req := pb.CreateProfileRequest{
// 		Parent:      "projects/" + a.deployment.ProjectId,
// 		Deployment:  a.deployment,
// 		ProfileType: a.profileTypes,
// 	}

// 	var p *pb.Profile
// 	md := grpcmd.New(nil) // For capturing trailing metadata

// 	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
// 		debugLog("creating a new profile via profiler service")
// 		var createErr error
// 		p, createErr = a.client.CreateProfile(ctx, &req, grpc.Trailer(&md)) // Capture trailer
// 		if createErr != nil {
// 			debugLog("failed to create profile, will retry: %v", createErr)
// 			// gax.Invoke handles standard retryable codes. Force retry for specific cert errors.
// 			if strings.Contains(createErr.Error(), "x509: certificate signed by unknown authority") {
// 				return errors.New("retry the certificate error")
// 			}
// 		}
// 		return createErr // Return original error for gax retry logic
// 	}, gax.WithRetry(func() gax.Retryer {
// 		return &retryer{ // Custom retryer to handle Aborted code with server-provided backoff
// 			backoff: gax.Backoff{
// 				Initial:    initialBackoff,
// 				Max:        maxBackoff,
// 				Multiplier: backoffMultiplier,
// 			},
// 			md: &md,
// 		}
// 	}))

// 	// If Invoke fails after retries (e.g., context cancelled, non-retryable error)
// 	if err != nil {
// 		debugLog("failed to create profile after retries: %v", err)
// 		return nil // Indicate failure
// 	}

// 	debugLog("successfully created profile %v", p.GetProfileType())
// 	return p
// }

// // --- CORRECTED: Mapping Functions using Null Types ---

// func buildNestedProfileRecord(p *pprof_pb.Profile) *ProfileDetailsBQRecord {
// 	if p == nil {
// 		return nil
// 	}

// 	details := &ProfileDetailsBQRecord{
// 		// Use Null types for scalar fields, setting Valid: true
// 		DropFrames:        bigquery.NullInt64{Int64: p.GetDropFrames(), Valid: true},
// 		KeepFrames:        bigquery.NullInt64{Int64: p.GetKeepFrames(), Valid: true},
// 		TimeNanos:         bigquery.NullInt64{Int64: p.GetTimeNanos(), Valid: true},
// 		DurationNanos:     bigquery.NullInt64{Int64: p.GetDurationNanos(), Valid: true},
// 		Period:            bigquery.NullInt64{Int64: p.GetPeriod(), Valid: true},
// 		DefaultSampleType: bigquery.NullInt64{Int64: p.GetDefaultSampleType(), Valid: true},
// 		DocURL:            bigquery.NullInt64{Int64: p.GetDocUrl(), Valid: true},
// 		// Slices are allocated as before
// 		StringTable: make([]string, len(p.GetStringTable())),
// 		Comment:     make([]int64, len(p.GetComment())),
// 		SampleType:  make([]ValueTypeBQRecord, len(p.GetSampleType())),
// 		Sample:      make([]SampleBQRecord, len(p.GetSample())),
// 		Mapping:     make([]MappingBQRecord, len(p.GetMapping())),
// 		Location:    make([]LocationBQRecord, len(p.GetLocation())),
// 		Function:    make([]FunctionBQRecord, len(p.GetFunction())),
// 	}

// 	// Copy slices/repeated fields
// 	copy(details.StringTable, p.GetStringTable())
// 	copy(details.Comment, p.GetComment())

// 	for i, v := range p.GetSampleType() {
// 		details.SampleType[i] = mapProtoValueTypeToBQ(v)
// 	}
// 	// Handle nullable PeriodType record
// 	if p.GetPeriodType() != nil {
// 		details.PeriodType = new(ValueTypeBQRecord)
// 		*details.PeriodType = mapProtoValueTypeToBQ(p.GetPeriodType())
// 	} // If nil, details.PeriodType remains nil, correctly mapping to NULL

// 	for i, v := range p.GetSample() {
// 		details.Sample[i] = mapProtoSampleToBQ(v)
// 	}
// 	for i, v := range p.GetMapping() {
// 		details.Mapping[i] = mapProtoMappingToBQ(v)
// 	}
// 	for i, v := range p.GetLocation() {
// 		details.Location[i] = mapProtoLocationToBQ(v)
// 	}
// 	for i, v := range p.GetFunction() {
// 		details.Function[i] = mapProtoFunctionToBQ(v)
// 	}

// 	return details
// }

// func mapProtoValueTypeToBQ(vt *pprof_pb.ValueType) ValueTypeBQRecord {
// 	if vt == nil {
// 		// Return struct with Valid: false for Null types if source is nil
// 		return ValueTypeBQRecord{}
// 	}
// 	return ValueTypeBQRecord{
// 		Type: bigquery.NullInt64{Int64: vt.GetType(), Valid: true},
// 		Unit: bigquery.NullInt64{Int64: vt.GetUnit(), Valid: true},
// 	}
// }

// func mapProtoSampleToBQ(s *pprof_pb.Sample) SampleBQRecord {
// 	if s == nil {
// 		return SampleBQRecord{}
// 	}
// 	bqSample := SampleBQRecord{
// 		LocationID: make([]int64, len(s.GetLocationId())),
// 		Value:      make([]int64, len(s.GetValue())),
// 		Label:      make([]LabelBQRecord, len(s.GetLabel())),
// 	}
// 	for i, id := range s.GetLocationId() {
// 		bqSample.LocationID[i] = int64(id)
// 	}
// 	copy(bqSample.Value, s.GetValue())
// 	for i, l := range s.GetLabel() {
// 		bqSample.Label[i] = mapProtoLabelToBQ(l)
// 	}
// 	return bqSample
// }

// func mapProtoLabelToBQ(l *pprof_pb.Label) LabelBQRecord {
// 	if l == nil {
// 		return LabelBQRecord{}
// 	}
// 	return LabelBQRecord{
// 		Key:     bigquery.NullInt64{Int64: l.GetKey(), Valid: true},
// 		Str:     bigquery.NullInt64{Int64: l.GetStr(), Valid: true},
// 		Num:     bigquery.NullInt64{Int64: l.GetNum(), Valid: true},
// 		NumUnit: bigquery.NullInt64{Int64: l.GetNumUnit(), Valid: true},
// 	}
// }

// func mapProtoMappingToBQ(m *pprof_pb.Mapping) MappingBQRecord {
// 	if m == nil {
// 		return MappingBQRecord{}
// 	}
// 	return MappingBQRecord{
// 		ID:              bigquery.NullInt64{Int64: int64(m.GetId()), Valid: true},
// 		MemoryStart:     bigquery.NullInt64{Int64: int64(m.GetMemoryStart()), Valid: true},
// 		MemoryLimit:     bigquery.NullInt64{Int64: int64(m.GetMemoryLimit()), Valid: true},
// 		FileOffset:      bigquery.NullInt64{Int64: int64(m.GetFileOffset()), Valid: true},
// 		Filename:        bigquery.NullInt64{Int64: m.GetFilename(), Valid: true},
// 		BuildID:         bigquery.NullInt64{Int64: m.GetBuildId(), Valid: true},
// 		HasFunctions:    bigquery.NullBool{Bool: m.GetHasFunctions(), Valid: true},
// 		HasFilenames:    bigquery.NullBool{Bool: m.GetHasFilenames(), Valid: true},
// 		HasLineNumbers:  bigquery.NullBool{Bool: m.GetHasLineNumbers(), Valid: true},
// 		HasInlineFrames: bigquery.NullBool{Bool: m.GetHasInlineFrames(), Valid: true},
// 	}
// }

// func mapProtoLocationToBQ(l *pprof_pb.Location) LocationBQRecord {
// 	if l == nil {
// 		return LocationBQRecord{}
// 	}
// 	bqLocation := LocationBQRecord{
// 		ID:        bigquery.NullInt64{Int64: int64(l.GetId()), Valid: true},
// 		MappingID: bigquery.NullInt64{Int64: int64(l.GetMappingId()), Valid: true},
// 		Address:   bigquery.NullInt64{Int64: int64(l.GetAddress()), Valid: true},
// 		IsFolded:  bigquery.NullBool{Bool: l.GetIsFolded(), Valid: true},
// 		Line:      make([]LineBQRecord, len(l.GetLine())),
// 	}
// 	for i, ln := range l.GetLine() {
// 		bqLocation.Line[i] = mapProtoLineToBQ(ln)
// 	}
// 	return bqLocation
// }

// func mapProtoLineToBQ(ln *pprof_pb.Line) LineBQRecord {
// 	if ln == nil {
// 		return LineBQRecord{}
// 	}
// 	return LineBQRecord{
// 		FunctionID: bigquery.NullInt64{Int64: int64(ln.GetFunctionId()), Valid: true},
// 		Line:       bigquery.NullInt64{Int64: ln.GetLine(), Valid: true},
// 		Column:     bigquery.NullInt64{Int64: ln.GetColumn(), Valid: true},
// 	}
// }

// func mapProtoFunctionToBQ(f *pprof_pb.Function) FunctionBQRecord {
// 	if f == nil {
// 		return FunctionBQRecord{}
// 	}
// 	return FunctionBQRecord{
// 		ID:         bigquery.NullInt64{Int64: int64(f.GetId()), Valid: true},
// 		Name:       bigquery.NullInt64{Int64: f.GetName(), Valid: true},
// 		SystemName: bigquery.NullInt64{Int64: f.GetSystemName(), Valid: true},
// 		Filename:   bigquery.NullInt64{Int64: f.GetFilename(), Valid: true},
// 		StartLine:  bigquery.NullInt64{Int64: f.GetStartLine(), Valid: true},
// 	}
// }

// // --- ADDED: Upload function for Nested BQ Table ---

// // uploadNestedToBigQuery populates and uploads a record with the fully nested structure.
// func (a *agent) uploadNestedToBigQuery(ctx context.Context, name string, pt pb.ProfileType, captureTime time.Time, labels map[string]string, deployment *pb.Deployment, duration *durationpb.Duration, parsedProfile *pprof_pb.Profile) {
// 	if a.bqClient == nil || a.bqNestedTableID == "" {
// 		debugLog("Skipping BigQuery nested upload (client/table not configured). TableID: %s", a.bqNestedTableID)
// 		return
// 	}
// 	if parsedProfile == nil {
// 		debugLog("Skipping BigQuery nested upload (parsed profile is nil).")
// 		return
// 	}

// 	// Map the parsed proto to the nested BQ structure
// 	profileDetails := buildNestedProfileRecord(parsedProfile)
// 	if profileDetails == nil { // Should not happen if parsedProfile is not nil, but check
// 		debugLog("Skipping BigQuery nested upload (failed to build nested record).")
// 		return
// 	}

// 	// Create the top-level record
// 	bqNestedRecord := ProfileNestedBQRecord{
// 		Name:           name,
// 		ProfileType:    pt.String(),
// 		StartTime:      captureTime,
// 		Labels:         mapToLabelPairs(labels),
// 		Deployment:     buildDeploymentBQRecord(deployment), // Reuse helper
// 		Duration:       buildDurationBQRecord(duration),     // Reuse helper
// 		ProfileDetails: profileDetails,
// 	}

// 	inserter := a.bqClient.DatasetInProject(a.bqProjectID, a.bqDatasetID).Table(a.bqNestedTableID).Inserter()
// 	items := []*ProfileNestedBQRecord{&bqNestedRecord}

// 	if err := insertBigQueryItems(ctx, inserter, items, a.bqNestedTableID); err != nil {
// 		logBigQueryError(err, a.bqNestedTableID) // Use helper for logging
// 	} else {
// 		debugLog("Successfully uploaded profile nested data to BigQuery table %s", a.bqNestedTableID)
// 	}
// }

// // --- Modified collectAndUploadProfile function ---
// func (a *agent) collectAndUploadProfile(ctx context.Context, p *pb.Profile) {
// 	captureTime := time.Now().UTC()
// 	profileType := p.GetProfileType()

// 	if !a.isProfileTypeEnabled(profileType) {
// 		debugLog("skipping collection of disabled profile type: %v", profileType)
// 		return
// 	}

// 	gzippedProfileBytes, err := a.generateProfileBytes(ctx, p)
// 	if err != nil {
// 		debugLog("Profile generation failed for type %v: %v. Skipping uploads.", profileType, err)
// 		return
// 	}

// 	// --- Decompress and Parse (Result needed for JSON and Nested uploads) ---
// 	var parsedProfile *pprof_pb.Profile // Store pointer to parsed profile
// 	var profileJsonString string
// 	var conversionErr error

// 	profileJsonString, parsedProfile, conversionErr = convertProfileBytesToJsonAndProto(gzippedProfileBytes) // Modified helper
// 	if conversionErr != nil {
// 		debugLog("Failed to convert profile bytes for type %v: %v. JSON/Nested BQ uploads will be skipped.", profileType, conversionErr)
// 		// Set parsedProfile to nil to prevent nested upload attempt if parsing failed
// 		parsedProfile = nil
// 	}
// 	// --- End Decompress and Parse ---

// 	// Prepare common metadata for uploads
// 	profileName := p.GetName()
// 	profileLabelsMap := mergeLabels(a.profileLabels, p.GetLabels())
// 	commonDeployment := p.GetDeployment()
// 	// Use the duration potentially modified by generateProfileBytes
// 	commonDurationProto := p.GetDuration()

// 	// --- Upload to BigQuery Table 1 (Raw Bytes) ---
// 	a.uploadBytesToBigQuery(ctx, profileName, profileType, captureTime, profileLabelsMap, commonDeployment, commonDurationProto, gzippedProfileBytes)

// 	// --- Upload to BigQuery Table 2 (JSON String) ---
// 	if profileJsonString != "" { // Only upload if JSON string conversion succeeded
// 		a.uploadJsonToBigQuery(ctx, profileName, profileType, captureTime, profileLabelsMap, commonDeployment, commonDurationProto, profileJsonString)
// 	}

// 	// --- ADDED: Upload to BigQuery Table 3 (Nested Structure) ---
// 	if parsedProfile != nil { // Only upload if parsing to proto struct succeeded
// 		a.uploadNestedToBigQuery(ctx, profileName, profileType, captureTime, profileLabelsMap, commonDeployment, commonDurationProto, parsedProfile)
// 	}
// 	// --- END ADDED ---

// 	// --- Upload to Cloud Profiler API ---
// 	a.uploadProfileToAPI(ctx, p, profileLabelsMap, gzippedProfileBytes)
// }

// // --- MODIFIED Helper: convertProfileBytesToJsonAndProto ---
// // Now returns the parsed proto as well, or nil if parsing fails.
// func convertProfileBytesToJsonAndProto(gzippedBytes []byte) (string, *pprof_pb.Profile, error) {
// 	gzippedReader := bytes.NewReader(gzippedBytes)
// 	gzipReader, err := gzip.NewReader(gzippedReader)
// 	if err != nil {
// 		return "", nil, fmt.Errorf("failed to create gzip reader: %w", err)
// 	}
// 	defer gzipReader.Close()

// 	decompressedBytes, err := io.ReadAll(gzipReader)
// 	if err != nil {
// 		return "", nil, fmt.Errorf("failed to decompress profile bytes: %w", err)
// 	}

// 	var parsedProfile pprof_pb.Profile // Use the generated struct
// 	if err := proto.Unmarshal(decompressedBytes, &parsedProfile); err != nil {
// 		// Return error, indicating parsing failed
// 		return "", nil, fmt.Errorf("failed to unmarshal decompressed profile: %w", err)
// 	}

// 	// Parsing succeeded, now try marshalling to JSON
// 	jsonMarshaler := protojson.MarshalOptions{
// 		Multiline:       false,
// 		UseProtoNames:   true,
// 		EmitUnpopulated: false,
// 	}
// 	jsonBytes, err := jsonMarshaler.Marshal(&parsedProfile)
// 	if err != nil {
// 		// Return error, but also return the successfully parsed profile
// 		return "", &parsedProfile, fmt.Errorf("failed to marshal parsed profile to JSON: %w", err)
// 	}

// 	// Both parsing and JSON marshalling succeeded
// 	return string(jsonBytes), &parsedProfile, nil
// }

// // generateProfileBytes collects the specified profile type and returns gzipped bytes.
// // It may modify p.Duration if the profile type doesn't have a duration (e.g., HEAP).
// func (a *agent) generateProfileBytes(ctx context.Context, p *pb.Profile) ([]byte, error) {
// 	var prof bytes.Buffer // Target buffer for pprof output (assumed gzipped)
// 	profileType := p.GetProfileType()
// 	durationProto := p.GetDuration()

// 	switch profileType {
// 	case pb.ProfileType_CPU:
// 		duration, err := validateDuration(durationProto)
// 		if err != nil {
// 			return nil, fmt.Errorf("invalid duration for CPU profile: %w", err)
// 		}
// 		if err := startCPUProfile(&prof); err != nil {
// 			return nil, fmt.Errorf("failed to start CPU profile: %w", err)
// 		}
// 		sleep(ctx, duration)
// 		stopCPUProfile()

// 	case pb.ProfileType_HEAP:
// 		if err := writeHeapProfile(&prof); err != nil {
// 			return nil, fmt.Errorf("failed to write heap profile: %w", err)
// 		}
// 		p.Duration = nil // Duration doesn't apply

// 	case pb.ProfileType_HEAP_ALLOC:
// 		duration, err := validateDuration(durationProto)
// 		if err != nil {
// 			return nil, fmt.Errorf("invalid duration for allocation profile: %w", err)
// 		}
// 		if err := deltaAllocProfile(ctx, duration, config.AllocForceGC, &prof); err != nil {
// 			return nil, fmt.Errorf("failed to collect allocation profile: %w", err)
// 		}

// 	case pb.ProfileType_THREADS:
// 		if err := pprof.Lookup("goroutine").WriteTo(&prof, 0); err != nil {
// 			return nil, fmt.Errorf("failed to collect goroutine profile: %w", err)
// 		}
// 		p.Duration = nil // Duration doesn't apply

// 	case pb.ProfileType_CONTENTION:
// 		if !mutexEnabled {
// 			return nil, errors.New("mutex profiling is not enabled")
// 		}
// 		duration, err := validateDuration(durationProto)
// 		if err != nil {
// 			return nil, fmt.Errorf("invalid duration for contention profile: %w", err)
// 		}
// 		if err := deltaMutexProfile(ctx, duration, &prof); err != nil {
// 			return nil, fmt.Errorf("failed to collect mutex profile: %w", err)
// 		}

// 	default:
// 		return nil, fmt.Errorf("unexpected profile type: %v", profileType)
// 	}

// 	return prof.Bytes(), nil
// }

// // convertProfileBytesToJson decompresses gzipped profile bytes, parses them,
// // and marshals them into a compact JSON string.
// func convertProfileBytesToJson(gzippedBytes []byte) (string, error) {
// 	gzippedReader := bytes.NewReader(gzippedBytes)
// 	gzipReader, err := gzip.NewReader(gzippedReader)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create gzip reader: %w", err)
// 	}
// 	defer gzipReader.Close()

// 	decompressedBytes, err := io.ReadAll(gzipReader)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to decompress profile bytes: %w", err)
// 	}

// 	var parsedProfile pprof_pb.Profile // Use the generated struct
// 	if err := proto.Unmarshal(decompressedBytes, &parsedProfile); err != nil {
// 		// Optionally save files here if debugging is still needed
// 		// saveDebugFiles(pt, gzippedProfileBytes, decompressedBytes)
// 		return "", fmt.Errorf("failed to unmarshal decompressed profile: %w", err)
// 	}

// 	jsonMarshaler := protojson.MarshalOptions{
// 		Multiline:       false,
// 		UseProtoNames:   true,
// 		EmitUnpopulated: false,
// 	}
// 	jsonBytes, err := jsonMarshaler.Marshal(&parsedProfile)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to marshal parsed profile to JSON: %w", err)
// 	}

// 	return string(jsonBytes), nil
// }

// // uploadBytesToBigQuery populates and uploads a record containing raw profile bytes.
// func (a *agent) uploadBytesToBigQuery(ctx context.Context, name string, pt pb.ProfileType, captureTime time.Time, labels map[string]string, deployment *pb.Deployment, duration *durationpb.Duration, gzippedBytes []byte) {
// 	if a.bqClient == nil || a.bqTableID == "" {
// 		debugLog("Skipping BigQuery raw bytes upload (client/table not configured). TableID: %s", a.bqTableID)
// 		return
// 	}

// 	bqRecord := ProfileBQRecord{
// 		Name:         name,
// 		ProfileType:  pt.String(),
// 		ProfileBytes: gzippedBytes,
// 		StartTime:    captureTime,
// 		Labels:       mapToLabelPairs(labels),
// 		Deployment:   buildDeploymentBQRecord(deployment),
// 		Duration:     buildDurationBQRecord(duration),
// 	}

// 	inserter := a.bqClient.DatasetInProject(a.bqProjectID, a.bqDatasetID).Table(a.bqTableID).Inserter()
// 	items := []*ProfileBQRecord{&bqRecord}

// 	if err := insertBigQueryItems(ctx, inserter, items, a.bqTableID); err != nil {
// 		logBigQueryError(err, a.bqTableID) // Use helper for logging
// 	} else {
// 		debugLog("Successfully uploaded profile bytes to BigQuery table %s", a.bqTableID)
// 	}
// }

// // uploadJsonToBigQuery populates and uploads a record containing profile JSON.
// func (a *agent) uploadJsonToBigQuery(ctx context.Context, name string, pt pb.ProfileType, captureTime time.Time, labels map[string]string, deployment *pb.Deployment, duration *durationpb.Duration, jsonString string) {
// 	if a.bqClient == nil || a.bqJsonTableID == "" {
// 		debugLog("Skipping BigQuery JSON upload (client/table not configured). TableID: %s", a.bqJsonTableID)
// 		return
// 	}

// 	bqJsonRecord := ProfileJsonBQRecord{
// 		Name:        name,
// 		ProfileType: pt.String(),
// 		ProfileJson: jsonString,
// 		StartTime:   captureTime,
// 		Labels:      mapToLabelPairs(labels),
// 		Deployment:  buildDeploymentBQRecord(deployment),
// 		Duration:    buildDurationBQRecord(duration),
// 	}

// 	inserter := a.bqClient.DatasetInProject(a.bqProjectID, a.bqDatasetID).Table(a.bqJsonTableID).Inserter()
// 	items := []*ProfileJsonBQRecord{&bqJsonRecord}

// 	if err := insertBigQueryItems(ctx, inserter, items, a.bqJsonTableID); err != nil {
// 		logBigQueryError(err, a.bqJsonTableID) // Use helper for logging
// 	} else {
// 		debugLog("Successfully uploaded profile JSON to BigQuery table %s", a.bqJsonTableID)
// 	}
// }

// // insertBigQueryItems handles the actual insertion call with timeout and basic retry setup.
// // Note: The 'items' parameter uses interface{} for generics, caller ensures correct type.
// func insertBigQueryItems(ctx context.Context, inserter *bigquery.Inserter, items interface{}, tableID string) error {
// 	insertCtx, cancel := context.WithTimeout(ctx, bqInsertTimeout)
// 	defer cancel()
// 	return inserter.Put(insertCtx, items) // Let caller handle specific PutMultiError if needed via logBigQueryError
// }

// // uploadProfileToAPI uploads the profile proto to the Cloud Profiler service API.
// func (a *agent) uploadProfileToAPI(ctx context.Context, p *pb.Profile, labels map[string]string, gzippedBytes []byte) {
// 	// Ensure the proto being sent to the API has the correct bytes and labels
// 	p.ProfileBytes = gzippedBytes
// 	p.Labels = labels

// 	req := &pb.UpdateProfileRequest{Profile: p}
// 	debugLog("Starting upload profile via Cloud Profiler API...")
// 	_, err := a.client.UpdateProfile(ctx, req)
// 	if err != nil {
// 		// Consider adding retry for transient API errors if needed
// 		debugLog("Failed to upload profile via Cloud Profiler API: %v", err)
// 	} else {
// 		debugLog("Successfully uploaded profile via Cloud Profiler API")
// 	}
// }

// // --- Helper Functions ---

// func (a *agent) isProfileTypeEnabled(pt pb.ProfileType) bool {
// 	for _, enabled := range a.profileTypes {
// 		if enabled == pt {
// 			return true
// 		}
// 	}
// 	return false
// }

// // mergeLabels combines base labels with profile-specific labels, with profile labels overriding.
// func mergeLabels(base, profile map[string]string) map[string]string {
// 	if len(base) == 0 && len(profile) == 0 {
// 		return nil
// 	}
// 	merged := make(map[string]string)
// 	for k, v := range base {
// 		merged[k] = v
// 	}
// 	for k, v := range profile { // Override base with profile labels if keys clash
// 		merged[k] = v
// 	}
// 	return merged
// }

// // buildDeploymentBQRecord converts the proto deployment to the BQ record struct.
// func buildDeploymentBQRecord(dep *pb.Deployment) *DeploymentBQRecord {
// 	if dep == nil {
// 		return nil
// 	}
// 	return &DeploymentBQRecord{
// 		ProjectID: dep.GetProjectId(),
// 		Target:    dep.GetTarget(),
// 		Labels:    mapToLabelPairs(dep.GetLabels()),
// 	}
// }

// // buildDurationBQRecord converts the proto duration to the BQ record struct.
// func buildDurationBQRecord(dur *durationpb.Duration) *DurationBQRecord {
// 	if dur == nil {
// 		return nil
// 	}
// 	// Ensure duration is valid before converting; CheckValid handles normalization.
// 	if err := dur.CheckValid(); err != nil {
// 		debugLog("Skipping invalid proto duration for BigQuery record: %v", err)
// 		return nil
// 	}
// 	return &DurationBQRecord{
// 		Seconds: dur.GetSeconds(),
// 		Nanos:   dur.GetNanos(),
// 	}
// }

// // validateDuration checks if a proto duration is valid and positive, returning time.Duration.
// func validateDuration(d *durationpb.Duration) (time.Duration, error) {
// 	if err := d.CheckValid(); err != nil {
// 		return 0, err
// 	}
// 	duration := d.AsDuration()
// 	if duration <= 0 {
// 		return 0, fmt.Errorf("non-positive duration: %v", duration)
// 	}
// 	return duration, nil
// }

// // logBigQueryError logs errors from inserter.Put, handling PutMultiError.
// func logBigQueryError(err error, tableID string) {
// 	if multiErr, ok := err.(bigquery.PutMultiError); ok {
// 		for _, rowErr := range multiErr {
// 			debugLog("BigQuery row insertion error for table %s: index %d, errors: %v", tableID, rowErr.RowIndex, rowErr.Errors)
// 		}
// 		// Log a summary error even if only some rows failed
// 		debugLog("Failed to insert some/all rows into BigQuery table %s.", tableID)
// 	} else {
// 		// Log non-multi errors
// 		debugLog("Failed to upload profile to BigQuery table %s: %v", tableID, err)
// 	}
// }

// // withXGoogHeader adds the x-goog-api-client header for tracking.
// func withXGoogHeader(ctx context.Context, keyval ...string) context.Context {
// 	kv := append([]string{"gl-go", version.Go(), "gccl", internal.Version}, keyval...)
// 	kv = append(kv, "gax", gax.Version, "grpc", grpc.Version)
// 	md, _ := grpcmd.FromOutgoingContext(ctx)
// 	md = md.Copy()
// 	md[xGoogAPIMetadata] = []string{gax.XGoogHeader(kv...)}
// 	return grpcmd.NewOutgoingContext(ctx, md)
// }

// // --- Initialization Logic ---

// // initializeAgent sets up the agent struct, including BigQuery client.
// func initializeAgent(c pb.ProfilerServiceClient) (*agent, error) {
// 	deploymentLabels := map[string]string{languageLabel: "go"}
// 	if config.Zone != "" {
// 		deploymentLabels[zoneNameLabel] = config.Zone
// 	}
// 	if config.ServiceVersion != "" {
// 		deploymentLabels[versionLabel] = config.ServiceVersion
// 	}
// 	d := &pb.Deployment{
// 		ProjectId: config.ProjectID,
// 		Target:    config.Service,
// 		Labels:    deploymentLabels,
// 	}

// 	profileLabels := map[string]string{}
// 	if config.Instance != "" {
// 		profileLabels[instanceLabel] = config.Instance
// 	}

// 	profileTypes := determineEnabledProfileTypes()
// 	if len(profileTypes) == 0 {
// 		return nil, errors.New("collection is not enabled for any profile types")
// 	}

// 	// Initialize BigQuery client
// 	var bqClient *bigquery.Client
// 	var err error
// 	bqProjectID := config.ProjectID
// 	if bqProjectID != "" {
// 		bqClient, err = bigquery.NewClient(context.Background(), bqProjectID)
// 		if err != nil {
// 			log.Printf("Cloud Profiler: Warning: Failed to initialize BigQuery client for project %s: %v. BigQuery uploads will be disabled.", bqProjectID, err)
// 			bqClient = nil // Ensure client is nil
// 		} else {
// 			debugLog("BigQuery client initialized successfully for project %s.", bqProjectID)
// 		}
// 	} else {
// 		log.Println("Cloud Profiler: Warning: ProjectID is empty. BigQuery uploads will be disabled.")
// 	}

// 	// Use default or configured table IDs
// 	bqDatasetID := bqDatasetIDDefault
// 	bqTableID := bqTableIDDefault
// 	bqJsonTableID := bqJsonTableIDDefault
// 	bqNestedTableID := bqNestedTableIDDefault
// 	// TODO: Potentially allow these IDs to be overridden via Config struct

// 	return &agent{
// 		client:          c,
// 		deployment:      d,
// 		profileLabels:   profileLabels,
// 		profileTypes:    profileTypes,
// 		bqClient:        bqClient,
// 		bqProjectID:     bqProjectID,
// 		bqDatasetID:     bqDatasetID,
// 		bqTableID:       bqTableID,
// 		bqJsonTableID:   bqJsonTableID,
// 		bqNestedTableID: bqNestedTableID,
// 	}, nil
// }

// // determineEnabledProfileTypes figures out which profile types are active based on config.
// func determineEnabledProfileTypes() []pb.ProfileType {
// 	var profileTypes []pb.ProfileType
// 	if !config.NoCPUProfiling {
// 		profileTypes = append(profileTypes, pb.ProfileType_CPU)
// 	}
// 	if !config.NoHeapProfiling {
// 		profileTypes = append(profileTypes, pb.ProfileType_HEAP)
// 	}
// 	if !config.NoGoroutineProfiling {
// 		profileTypes = append(profileTypes, pb.ProfileType_THREADS)
// 	}
// 	if !config.NoAllocProfiling {
// 		profileTypes = append(profileTypes, pb.ProfileType_HEAP_ALLOC)
// 	}
// 	if mutexEnabled { // mutexEnabled is set during start()
// 		profileTypes = append(profileTypes, pb.ProfileType_CONTENTION)
// 	}
// 	return profileTypes
// }

// // initializeConfig resolves the final configuration values from input, env vars, and metadata.
// func initializeConfig(cfg Config) error {
// 	config = cfg // Assign initial config

// 	// Service Name
// 	if config.Service == "" {
// 		for _, ev := range []string{"GAE_SERVICE", "K_SERVICE"} {
// 			if val := os.Getenv(ev); val != "" {
// 				config.Service = val
// 				break
// 			}
// 		}
// 	}
// 	if config.Service == "" {
// 		return errors.New("service name must be configured via Config or environment variable (GAE_SERVICE, K_SERVICE)")
// 	}
// 	if !serviceRegexp.MatchString(config.Service) {
// 		return fmt.Errorf("service name %q does not match regular expression %q", config.Service, serviceRegexp.String())
// 	}

// 	// Service Version
// 	if config.ServiceVersion == "" {
// 		for _, ev := range []string{"GAE_VERSION", "K_REVISION"} {
// 			if val := os.Getenv(ev); val != "" {
// 				config.ServiceVersion = val
// 				break
// 			}
// 		}
// 	}

// 	// Project ID
// 	if config.ProjectID == "" {
// 		if projectIDEnv := os.Getenv("GOOGLE_CLOUD_PROJECT"); projectIDEnv != "" {
// 			config.ProjectID = projectIDEnv
// 		}
// 	}

// 	// GCP Metadata (only if running on GCE)
// 	if onGCE() {
// 		var err error
// 		if config.ProjectID == "" {
// 			config.ProjectID, err = getProjectID()
// 			if err != nil {
// 				return fmt.Errorf("failed to get project ID from Compute Engine metadata (and not set via Config/env): %w", err)
// 			}
// 		}
// 		if config.Zone == "" {
// 			config.Zone, err = getZone()
// 			if err != nil {
// 				// Non-fatal? Maybe log a warning instead? Let's keep it fatal for now.
// 				return fmt.Errorf("failed to get zone from Compute Engine metadata: %w", err)
// 			}
// 		}
// 		// if config.Instance == "" {
// 		// 	config.Instance, err = getInstanceName()
// 		// 	if err != nil && !errors.As(err, &gcemd.NotDefinedError{}) {
// 		// 		// Non-fatal? Log warning? Keep fatal for now.
// 		// 		return fmt.Errorf("failed to get instance name from Compute Engine metadata: %w", err)
// 		// 	} else if err != nil {
// 		// 		debugLog("Could not determine instance name from metadata: %v", err)
// 		// 	}
// 		// }
// 		if config.Instance == "" {
// 			var err error // Declare err within this scope if not already declared above
// 			config.Instance, err = getInstanceName()

// 			var nfe *gcemd.NotDefinedError // Declare a variable to hold the target error type pointer
// 			if errors.As(err, &nfe) {
// 				// It's specifically a NotDefinedError, which is acceptable. Just log it.
// 				debugLog("Could not determine instance name from metadata (NotDefinedError): %v", err)
// 				// Clear the error variable since we handled this specific case
// 				err = nil
// 			}

// 			// If there was an error, and it wasn't the NotDefinedError we just handled, return it.
// 			if err != nil {
// 				return fmt.Errorf("failed to get instance name from Compute Engine metadata: %w", err)
// 			}
// 			// If err is nil here, getInstanceName succeeded or returned only NotDefinedError.
// 			if config.Instance != "" { // Log if successfully retrieved
// 				debugLog("Using Instance name from GCE metadata server: %s", config.Instance)
// 			}
// 		}

// 	} else if config.ProjectID == "" {
// 		// Must have ProjectID if not on GCE
// 		return errors.New("project ID must be specified via profiler.Config or GOOGLE_CLOUD_PROJECT env var when not running on GCP")
// 	}

// 	// API Address
// 	if config.APIAddr == "" {
// 		config.APIAddr = apiAddress
// 	}

// 	debugLog("Initialized configuration: ProjectID=%q, Service=%q, Version=%q, Zone=%q, Instance=%q",
// 		config.ProjectID, config.Service, config.ServiceVersion, config.Zone, config.Instance)
// 	return nil
// }

// // --- Low-level Profiling Wrappers/Helpers ---

// // abortedBackoffDuration retrieves the retry duration from gRPC trailing metadata.
// func abortedBackoffDuration(md grpcmd.MD) (time.Duration, error) {
// 	elem := md[retryInfoMetadata]
// 	if len(elem) <= 0 {
// 		return 0, errors.New("no retry info")
// 	}
// 	var retryInfo edpb.RetryInfo
// 	if err := proto.Unmarshal([]byte(elem[0]), &retryInfo); err != nil {
// 		return 0, fmt.Errorf("failed to unmarshal retry info: %w", err)
// 	}
// 	if err := retryInfo.RetryDelay.CheckValid(); err != nil {
// 		return 0, fmt.Errorf("invalid retry delay duration: %w", err)
// 	}
// 	duration := retryInfo.RetryDelay.AsDuration()
// 	if duration < 0 {
// 		return 0, errors.New("negative retry duration")
// 	}
// 	return duration, nil
// }

// // retryer implements gax.Retryer for custom handling of Aborted status code.
// type retryer struct {
// 	backoff gax.Backoff
// 	md      *grpcmd.MD
// }

// func (r *retryer) Retry(err error) (time.Duration, bool) {
// 	st, _ := status.FromError(err)
// 	// Use server-suggested backoff for Aborted status, if available.
// 	if st != nil && st.Code() == codes.Aborted {
// 		if dur, err := abortedBackoffDuration(*r.md); err == nil {
// 			debugLog("Retrying on Aborted status with server-suggested backoff: %v", dur)
// 			return dur, true
// 		}
// 		debugLog("Aborted status received, but failed to get backoff duration from server: %v", err)
// 		// Fall through to default backoff for Aborted if server suggestion fails
// 	}
// 	// Use standard exponential backoff for other retryable errors handled by gax.Invoke.
// 	// gax.Invoke determines retryability based on standard gRPC codes.
// 	return r.backoff.Pause(), true // Let gax.Invoke decide if the error code is retryable
// }

// // deltaMutexProfile collects and calculates the delta mutex profile over a duration.
// // Assumes mutex profiling has been enabled.
// func deltaMutexProfile(ctx context.Context, duration time.Duration, prof *bytes.Buffer) error {
// 	p0, err := readMutexProfile()
// 	if err != nil {
// 		return fmt.Errorf("failed to read initial mutex profile: %w", err)
// 	}

// 	if err := sleep(ctx, duration); err != nil {
// 		// If sleep is interrupted (e.g., context canceled), we might not have a full duration.
// 		// Depending on requirements, we could either return error or proceed with partial duration.
// 		// Let's proceed and calculate delta for the time elapsed.
// 		debugLog("Sleep interrupted during mutex profile collection: %v", err)
// 	}

// 	p, err := readMutexProfile()
// 	if err != nil {
// 		return fmt.Errorf("failed to read final mutex profile: %w", err)
// 	}

// 	// Calculate delta: p - p0
// 	p0.Scale(-1)
// 	mergedProfile, err := profile.Merge([]*profile.Profile{p0, p})
// 	if err != nil {
// 		return fmt.Errorf("failed to merge mutex profiles: %w", err)
// 	}

// 	// Write the merged (delta) profile to the buffer
// 	// Assume Write handles gzipping if needed, based on previous findings.
// 	// If Write outputs raw proto, need to gzip here. Let's assume it outputs gzipped.
// 	return mergedProfile.Write(prof)
// }

// // readMutexProfile reads the current mutex profile.
// func readMutexProfile() (*profile.Profile, error) {
// 	p := pprof.Lookup("mutex")
// 	if p == nil {
// 		// Should not happen if mutexEnabled is true, but check defensively.
// 		return nil, errors.New("mutex profile type not found")
// 	}
// 	var buf bytes.Buffer
// 	if err := p.WriteTo(&buf, 0); err != nil {
// 		return nil, fmt.Errorf("failed writing mutex profile: %w", err)
// 	}
// 	// Parse the profile to allow scaling/merging.
// 	parsedProfile, err := profile.Parse(&buf)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed parsing mutex profile: %w", err)
// 	}
// 	return parsedProfile, nil
// }

// // saveDebugFiles is a utility to save profile data locally for offline analysis.
// // (Keep commented out unless active debugging is needed).
// /*
// func saveDebugFiles(pt pb.ProfileType, gzippedBytes, decompressedBytes []byte) {
// 	fileNameSuffix := fmt.Sprintf("%s_%d", pt.String(), time.Now().UnixNano())
// 	fGz := fmt.Sprintf("debug_profile_%s.pb.gz", fileNameSuffix)
// 	fPb := fmt.Sprintf("debug_profile_%s.pb", fileNameSuffix)
// 	errGz := os.WriteFile(fGz, gzippedBytes, 0644)
// 	errPb := os.WriteFile(fPb, decompressedBytes, 0644)
// 	if errGz != nil || errPb != nil {
// 		debugLog("Error saving debug profile files: gzErr=%v, pbErr=%v", errGz, errPb)
// 	} else {
// 		debugLog("Saved debug profile data: %s, %s", fGz, fPb)
// 	}
// }
// */
