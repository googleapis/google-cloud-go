/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics owns the OpenTelemetry tracer machinery for the
// bigtable client — per-operation Tracer, per-attempt AttemptTracer,
// the gRPC stats.Handler that drives attempt boundaries, and the
// Cloud Monitoring exporter wiring. Split from the bigtable package
// so the internal/session data-plane can stamp per-attempt attributes
// (cluster_id, zone_id, transport labels, client-blocking latency,
// server latency) on session-path calls without an import cycle.
package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigtable/internal"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

const (
	BuiltInMetricsMeterName = "bigtable.googleapis.com/internal/client/"

	metricsPrefix         = "bigtable/"
	LocationMDKey         = "x-goog-ext-425905942-bin"
	ServerTimingMDKey     = "server-timing"
	serverTimingValPrefix = "gfet4t7; dur="
	metricMethodPrefix    = "Bigtable."

	// Monitored resource labels
	MonitoredResLabelKeyProject  = "project_id"
	MonitoredResLabelKeyInstance = "instance"
	MonitoredResLabelKeyTable    = "table"
	MonitoredResLabelKeyCluster  = "cluster"
	MonitoredResLabelKeyZone     = "zone"

	// Metric labels
	MetricLabelKeyAppProfile         = "app_profile"
	MetricLabelKeyMethod             = "method"
	MetricLabelKeyStatus             = "status"
	MetricLabelKeyTag                = "tag"
	MetricLabelKeyStreamingOperation = "streaming"
	MetricLabelKeyClientName         = "client_name"
	MetricLabelKeyClientUID          = "client_uid"

	// Peer-info-derived attributes (attempt_latencies2 only). Populated from
	// the bigtable-peer-info sideband metadata via ExtractPeerInfo.
	MetricTransportType    = "transport_type"
	MetricTransportRegion  = "transport_region"
	MetricTransportSubZone = "transport_subzone"
	MetricTransportZone    = "transport_zone"

	// Metric names
	MetricNameOperationLatencies      = "operation_latencies"
	MetricNameAttemptLatencies        = "attempt_latencies"
	MetricNameAttemptLatencies2       = "attempt_latencies2"
	MetricNameServerLatencies         = "server_latencies"
	MetricNameAppBlockingLatencies    = "application_latencies"
	MetricNameClientBlockingLatencies = "throttling_latencies"
	MetricNameFirstRespLatencies      = "first_response_latencies"
	MetricNameRetryCount              = "retry_count"
	MetricNameDebugTags               = "debug_tags"
	MetricNameConnErrCount            = "connectivity_error_count"

	// Metric units
	metricUnitMS    = "ms"
	metricUnitCount = "1"
	maxAttrsLen     = 16 // Monitored resource labels + Metric labels (incl. 4 transport labels on attempt_latencies2)
)

type contextKey string

const (
	statsContextKey         contextKey = "bigtable/clientBlockingLatencyTracker"
	t4t7ContextKey          contextKey = "bigtable/t4t7Tracker"
	metricsTracerContextKey contextKey = "bigtable/metricsTracer"
)

func NewContext(ctx context.Context, mt *Tracer) context.Context {
	return context.WithValue(ctx, metricsTracerContextKey, mt)
}

func FromContext(ctx context.Context) *Tracer {
	if mt, ok := ctx.Value(metricsTracerContextKey).(*Tracer); ok {
		return mt
	}
	return &Tracer{
		BuiltInEnabled: false,
		currOp: OpTracer{
			cookies: make(map[string]string),
		},
	}
}

// These are effectively constant, but for testing purposes they are mutable
var (
	// duration between two metric exports
	DefaultSamplePeriod = time.Minute

	disabledMetricsTracerFactory = &Factory{
		Enabled:  false,
		Shutdown: func() {},
	}

	// MetricsErrorPrefix wraps every metrics-subsystem error surfaced to
	// the OTel error handler. Exposed so tests can assert that exporter
	// / handler failures make it into the error stream.
	MetricsErrorPrefix = "bigtable-metrics: "

	clientName = fmt.Sprintf("go-bigtable/%v", internal.Version)

	bucketBounds = []float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0, 20.0, 25.0, 30.0, 40.0,
		50.0, 65.0, 80.0, 100.0, 130.0, 160.0, 200.0, 250.0, 300.0, 400.0, 500.0, 650.0,
		800.0, 1000.0, 2000.0, 5000.0, 10000.0, 20000.0, 50000.0, 100000.0, 200000.0,
		400000.0, 800000.0, 1600000.0, 3200000.0}

	// clientBlockingBucketBounds bounds optimized for microsecond-scale
	// latencies (expressed in milliseconds), ranging from 10µs to 10s.
	clientBlockingBucketBounds = []float64{
		0.0, 0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.08, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.8, 1.0,
		2.0, 5.0, 10.0, 20.0, 50.0, 100.0, 500.0, 1000.0, 5000.0, 10000.0,
	}

	// All the built-in metrics have same attributes except 'tag', 'status' and 'streaming'
	// These attributes need to be added to only few of the metrics
	MetricsDetails = map[string]metricInfo{
		MetricNameOperationLatencies: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
				MetricLabelKeyStreamingOperation,
			},
			recordedPerAttempt: false,
		},
		MetricNameAttemptLatencies: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
				MetricLabelKeyStreamingOperation,
			},
			recordedPerAttempt: true,
		},
		MetricNameAttemptLatencies2: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
				MetricLabelKeyStreamingOperation,
				MetricTransportType,
				MetricTransportRegion,
				MetricTransportSubZone,
				MetricTransportZone,
			},
			recordedPerAttempt: true,
		},
		MetricNameServerLatencies: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
				MetricLabelKeyStreamingOperation,
			},
			recordedPerAttempt: true,
		},
		MetricNameFirstRespLatencies: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
			},
			recordedPerAttempt: false,
		},
		MetricNameAppBlockingLatencies: {},
		MetricNameClientBlockingLatencies: {
			recordedPerAttempt: true,
		},
		MetricNameRetryCount: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
			},
			recordedPerAttempt: true,
		},
		MetricNameConnErrCount: {
			additionalAttrs: []string{
				MetricLabelKeyStatus,
			},
			recordedPerAttempt: true,
		},
	}

	// Generates unique client ID in the format go-<random UUID>@<hostname>
	GenerateClientUID = func() (string, error) {
		hostname, err := os.Hostname()
		if err != nil {
			return "", err
		}
		return "go-" + uuid.NewString() + "@" + hostname, nil
	}

	endpointOptionType = reflect.TypeOf(option.WithEndpoint(""))

	// CreateExporterOptions takes Bigtable client options and returns exporter
	// options, filtering out any WithEndpoint option to ensure the metrics
	// exporter uses its default endpoint. Overridden in tests to redirect
	// the exporter to a fake Cloud Monitoring server.
	CreateExporterOptions = func(btOpts ...option.ClientOption) []option.ClientOption {
		filteredOptions := []option.ClientOption{}
		for _, opt := range btOpts {
			if reflect.TypeOf(opt) != endpointOptionType {
				filteredOptions = append(filteredOptions, opt)
			}
		}
		return filteredOptions
	}

	SharedStatsHandler = &StatsHandler{}

	// canonicalStatusStrings maps the standard gRPC status codes to their
	// canonical SCREAMING_SNAKE_CASE string form. Indexed by codes.Code so
	// CanonicalString is an allocation-free lookup on the status-recording
	// hot path.
	//
	// Hand-rolled rather than delegating to grpc-go's canonicalString
	// (unexported: only reachable via grpc/internal/CanonicalString) or
	// google.golang.org/genproto/googleapis/rpc/code.Code_name (exported
	// but emits "CANCELLED" for Canceled). The bigtable metrics label
	// history uses "CANCELED" (single L) — matching the pre-refactor
	// upstream code that ran `strings.ToUpper` over grpc-go's
	// `codes.Canceled.String()` = "Canceled". Changing to either helper
	// would flip the emitted label and break downstream dashboards.
	canonicalStatusStrings = [...]string{
		codes.OK:                 "OK",
		codes.Canceled:           "CANCELED",
		codes.Unknown:            "UNKNOWN",
		codes.InvalidArgument:    "INVALID_ARGUMENT",
		codes.DeadlineExceeded:   "DEADLINE_EXCEEDED",
		codes.NotFound:           "NOT_FOUND",
		codes.AlreadyExists:      "ALREADY_EXISTS",
		codes.PermissionDenied:   "PERMISSION_DENIED",
		codes.ResourceExhausted:  "RESOURCE_EXHAUSTED",
		codes.FailedPrecondition: "FAILED_PRECONDITION",
		codes.Aborted:            "ABORTED",
		codes.OutOfRange:         "OUT_OF_RANGE",
		codes.Unimplemented:      "UNIMPLEMENTED",
		codes.Internal:           "INTERNAL",
		codes.Unavailable:        "UNAVAILABLE",
		codes.DataLoss:           "DATA_LOSS",
		codes.Unauthenticated:    "UNAUTHENTICATED",
	}
)

type metricInfo struct {
	additionalAttrs    []string
	recordedPerAttempt bool
}

type Factory struct {
	Enabled bool

	ClientOpts []option.ClientOption

	// To be called on client close
	Shutdown func()

	// attributes that are specific to a client instance and
	// do not change across different function calls on client
	clientAttributes []attribute.KeyValue

	// OtelMeterProvider
	OtelMeterProvider metric.MeterProvider

	operationLatencies      metric.Float64Histogram
	serverLatencies         metric.Float64Histogram
	attemptLatencies        metric.Float64Histogram
	attemptLatencies2       metric.Float64Histogram
	firstRespLatencies      metric.Float64Histogram
	appBlockingLatencies    metric.Float64Histogram
	clientBlockingLatencies metric.Float64Histogram
	retryCount              metric.Int64Counter
	connErrCount            metric.Int64Counter
	debugTags               metric.Int64Counter
}

// Returns error only if MetricsProvider is of unknown type. Rest all errors are swallowed
func NewFactory(ctx context.Context, project, instance, appProfile string, metricsProvider MetricsProvider, opts ...option.ClientOption) (*Factory, error) {
	if metricsProvider != nil {
		switch metricsProvider.(type) {
		case NoopMetricsProvider:
			return disabledMetricsTracerFactory, nil
		default:
			return disabledMetricsTracerFactory, errors.New("bigtable: unknown MetricsProvider type")
		}
	}

	// Metrics are Enabled.
	clientUID, err := GenerateClientUID()
	if err != nil {
		// Swallow the error and disable metrics
		return disabledMetricsTracerFactory, nil
	}

	tracerFactory := &Factory{
		Enabled: true,
		clientAttributes: []attribute.KeyValue{
			attribute.String(MonitoredResLabelKeyProject, project),
			attribute.String(MonitoredResLabelKeyInstance, instance),
			attribute.String(MetricLabelKeyAppProfile, appProfile),
			attribute.String(MetricLabelKeyClientUID, clientUID),
			attribute.String(MetricLabelKeyClientName, clientName),
		},
		Shutdown: func() {},
	}

	// Create default meter provider
	mpOptions, err := builtInMeterProviderOptions(project, opts...)
	if err != nil {
		// Swallow the error and disable metrics
		return disabledMetricsTracerFactory, nil
	}
	meterProvider := sdkmetric.NewMeterProvider(mpOptions...)
	// Enable Otel metrics collection
	otelContext, err := newOtelMetricsContext(ctx, metricsConfig{
		project:         project,
		instance:        instance,
		appProfile:      appProfile,
		clientName:      clientName,
		clientUID:       clientUID,
		interval:        DefaultSamplePeriod,
		customExporter:  nil,
		manualReader:    nil,
		disableExporter: false,
		resourceOpts:    nil,
	})

	// the error from newOtelMetricsContext is silently ignored since metrics are not critical to client creation.
	if err == nil {
		tracerFactory.ClientOpts = otelContext.ClientOpts
		tracerFactory.OtelMeterProvider = otelContext.OtelMeterProvider
	}
	tracerFactory.Shutdown = func() {
		if otelContext != nil {
			otelContext.close()
		}
		meterProvider.Shutdown(ctx)
	}

	// Create meter and instruments
	meter := meterProvider.Meter(BuiltInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	err = tracerFactory.createInstruments(meter)
	if err != nil {
		// Swallow the error and disable metrics
		return disabledMetricsTracerFactory, nil
	}
	return tracerFactory, nil
}

// NewFactoryForTest constructs an enabled Factory backed by the supplied
// MeterProvider. Test-only: skips the built-in Cloud Monitoring exporter
// setup NewFactory does, so callers can inject a ManualReader-backed
// provider and assert on the emitted data points. Production code must
// use NewFactory.
func NewFactoryForTest(project, instance, appProfile string, mp metric.MeterProvider) (*Factory, error) {
	clientUID, err := GenerateClientUID()
	if err != nil {
		return nil, err
	}
	tf := &Factory{
		Enabled: true,
		clientAttributes: []attribute.KeyValue{
			attribute.String(MonitoredResLabelKeyProject, project),
			attribute.String(MonitoredResLabelKeyInstance, instance),
			attribute.String(MetricLabelKeyAppProfile, appProfile),
			attribute.String(MetricLabelKeyClientUID, clientUID),
			attribute.String(MetricLabelKeyClientName, clientName),
		},
		OtelMeterProvider: mp,
		Shutdown:          func() {},
	}
	meter := mp.Meter(BuiltInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	if err := tf.createInstruments(meter); err != nil {
		return nil, err
	}
	return tf, nil
}

func builtInMeterProviderOptions(project string, opts ...option.ClientOption) ([]sdkmetric.Option, error) {
	allOpts := CreateExporterOptions(opts...)
	defaultExporter, err := newMonitoringExporter(context.Background(), project, allOpts...)
	if err != nil {
		return nil, err
	}

	return []sdkmetric.Option{sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(
			defaultExporter,
			sdkmetric.WithInterval(DefaultSamplePeriod),
		),
	)}, nil
}

func (tf *Factory) NewAsyncRefreshErrHandler() func() {
	if !tf.Enabled {
		return func() {}
	}

	asyncRefreshMetricAttrs := tf.clientAttributes
	asyncRefreshMetricAttrs = append(asyncRefreshMetricAttrs,
		attribute.String(MetricLabelKeyTag, "async_refresh_dry_run"),
		// Table, cluster and zone are unknown at this point
		// Use default values
		attribute.String(MonitoredResLabelKeyTable, defaultTable),
		attribute.String(MonitoredResLabelKeyCluster, defaultCluster),
		attribute.String(MonitoredResLabelKeyZone, defaultZone),
	)
	return func() {
		tf.debugTags.Add(context.Background(), 1,
			metric.WithAttributes(asyncRefreshMetricAttrs...))
	}
}

func (tf *Factory) createInstruments(meter metric.Meter) error {
	var err error

	// Create operation_latencies
	tf.operationLatencies, err = meter.Float64Histogram(
		MetricNameOperationLatencies,
		metric.WithDescription("Total time until final operation success or failure, including retries and backoff."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create attempt_latencies
	tf.attemptLatencies, err = meter.Float64Histogram(
		MetricNameAttemptLatencies,
		metric.WithDescription("Client observed latency per RPC attempt."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create attempt_latencies2 — same latency value as attempt_latencies,
	// broken out with transport_type/region/zone/subzone attributes sourced
	// from the bigtable-peer-info sideband metadata. Uses the shared
	// java-parity FineGrainLatencyBounds so sub-ms DirectPath samples
	// don't collapse into a single [0,1)ms bucket.
	tf.attemptLatencies2, err = meter.Float64Histogram(
		MetricNameAttemptLatencies2,
		metric.WithDescription("Client observed latency per RPC attempt, labeled by transport type and AFE location."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(btransport.FineGrainLatencyBounds...),
	)
	if err != nil {
		return err
	}

	// Create server_latencies
	tf.serverLatencies, err = meter.Float64Histogram(
		MetricNameServerLatencies,
		metric.WithDescription("The latency measured from the moment that the RPC entered the Google data center until the RPC was completed."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create first_response_latencies
	tf.firstRespLatencies, err = meter.Float64Histogram(
		MetricNameFirstRespLatencies,
		metric.WithDescription("Latency from operation start until the response headers were received. The publishing of the measurement will be delayed until the attempt response has been received."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create application_latencies
	tf.appBlockingLatencies, err = meter.Float64Histogram(
		MetricNameAppBlockingLatencies,
		metric.WithDescription("The latency of the client application consuming available response data."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create client_blocking_latencies
	tf.clientBlockingLatencies, err = meter.Float64Histogram(
		MetricNameClientBlockingLatencies,
		metric.WithDescription("The latencies of requests queued on gRPC channels."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(clientBlockingBucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create retry_count
	tf.retryCount, err = meter.Int64Counter(
		MetricNameRetryCount,
		metric.WithDescription("The number of additional RPCs sent after the initial attempt."),
		metric.WithUnit(metricUnitCount),
	)
	if err != nil {
		return err
	}

	// Create connectivity_error_count
	tf.connErrCount, err = meter.Int64Counter(
		MetricNameConnErrCount,
		metric.WithDescription("Number of requests that failed to reach the Google datacenter. (Requests without google response headers"),
		metric.WithUnit(metricUnitCount),
	)
	if err != nil {
		return err
	}

	// Create debug_tags
	tf.debugTags, err = meter.Int64Counter(
		MetricNameDebugTags,
		metric.WithDescription("A counter of internal client events used for debugging."),
		metric.WithUnit(metricUnitCount),
	)
	return err
}

// Tracer is created one per operation
// It is used to store metric instruments, attribute values
// and other data required to obtain and record them
type Tracer struct {
	ctx            context.Context
	BuiltInEnabled bool

	// attributes that are specific to a client instance and
	// do not change across different operations on client
	clientAttributes []attribute.KeyValue

	instrumentOperationLatencies      metric.Float64Histogram
	instrumentServerLatencies         metric.Float64Histogram
	instrumentAttemptLatencies        metric.Float64Histogram
	instrumentAttemptLatencies2       metric.Float64Histogram
	instrumentFirstRespLatencies      metric.Float64Histogram
	instrumentAppBlockingLatencies    metric.Float64Histogram
	instrumentClientBlockingLatencies metric.Float64Histogram
	instrumentRetryCount              metric.Int64Counter
	instrumentConnErrCount            metric.Int64Counter
	instrumentDebugTags               metric.Int64Counter

	tableName   string
	method      string
	isStreaming bool

	currOp OpTracer
}

// OpTracer is used to record metrics for the entire operation, including retries.
// Operation is a logical unit that represents a single method invocation on client.
// The method might require multiple attempts/rpcs and backoff logic to complete
type OpTracer struct {
	attemptCount int64

	startTime time.Time

	// Only for ReadRows. Time when the response headers are received in a streaming RPC.
	firstRespTime time.Time

	// gRPC status code of last completed attempt
	status string

	currAttempt AttemptTracer

	appBlockingLatency float64

	// For routing cookie and gRPC attempt number
	cookies map[string]string

	// Last known location details across all attempts
	lastClusterID string
	lastZoneID    string
}

func (o *OpTracer) SetStartTime(t time.Time) {
	o.startTime = t
}

func (o *OpTracer) setFirstRespTime(t time.Time) {
	o.firstRespTime = t
}

func (o *OpTracer) setStatus(status string) {
	o.status = status
}

func (o *OpTracer) incrementAttemptCount() {
	o.attemptCount++
}

func (o *OpTracer) IncrementAppBlockingLatency(latency float64) {
	o.appBlockingLatency += latency
}

// AttemptTracer is used to record metrics for each individual attempt of the operation.
// Attempt corresponds to an attempt of an RPC.
type AttemptTracer struct {
	startTime time.Time
	clusterID string
	zoneID    string

	// Peer-info-derived attributes (feed attempt_latencies2 only). Populated
	// from the bigtable-peer-info sideband metadata; empty when the server
	// didn't emit the header (older servers, or PeerInfo feature flag off).
	transportType    string
	transportRegion  string
	transportZone    string
	transportSubZone string

	// gRPC status code
	status string

	// Server latency in ms
	serverLatency float64

	// Error seen while getting server latency from headers/trailers
	serverLatencyErr error

	// Tracker for client blocking latency
	blockingLatencyTracker *blockingLatencyTracker

	// Client blocking latency in ms
	clientBlockingLatency float64

	// Tracker for t4t7
	t4t7Tracker *t4t7Tracker

	// Response header and trailer metadata captured by the stats handler.
	headerMD  metadata.MD
	trailerMD metadata.MD
}

func (a *AttemptTracer) SetStartTime(t time.Time) {
	a.startTime = t
}

func (a *AttemptTracer) SetClusterID(clusterID string) {
	a.clusterID = clusterID
}

func (a *AttemptTracer) SetZoneID(zoneID string) {
	a.zoneID = zoneID
}

func (a *AttemptTracer) setStatus(status string) {
	a.status = status
}

func (a *AttemptTracer) SetServerLatency(latency float64) {
	a.serverLatency = latency
}

func (a *AttemptTracer) setServerLatencyErr(err error) {
	a.serverLatencyErr = err
}

// SetClientBlockingLatency stamps the per-attempt client-blocking
// latency. The session data plane uses this because it computes the
// value from btransport.InvokeResult.SentAt rather than relying on the
// gRPC OutPayload stats event that never fires for vRPC frames.
func (a *AttemptTracer) SetClientBlockingLatency(ms float64) {
	a.clientBlockingLatency = ms
}

// SetTransportType stamps the transport_type label used on
// attempt_latencies2. Session-path callers populate this from the
// serving session's parsed PeerInfo.
func (a *AttemptTracer) SetTransportType(v string) { a.transportType = v }

// SetTransportRegion stamps the transport_region label.
func (a *AttemptTracer) SetTransportRegion(v string) { a.transportRegion = v }

// SetTransportZone stamps the transport_zone label.
func (a *AttemptTracer) SetTransportZone(v string) { a.transportZone = v }

// SetTransportSubZone stamps the transport_subzone label.
func (a *AttemptTracer) SetTransportSubZone(v string) { a.transportSubZone = v }

// StartTime returns when the attempt started — session-path callers
// need it to compute (result.SentAt - startTime) for
// clientBlockingLatency stamping.
func (a *AttemptTracer) StartTime() time.Time { return a.startTime }

func (tf *Factory) CreateTracer(ctx context.Context, tableName string, isStreaming bool) Tracer {
	// Operation has started but not the attempt.
	// So, create only operation tracer and not attempt tracer
	currOpTracer := OpTracer{
		cookies: make(map[string]string),
	}
	currOpTracer.SetStartTime(time.Now())

	if !tf.Enabled {
		return Tracer{
			BuiltInEnabled: false,
			currOp:         currOpTracer,
		}
	}

	return Tracer{
		ctx:            ctx,
		BuiltInEnabled: tf.Enabled,

		currOp:           currOpTracer,
		clientAttributes: tf.clientAttributes,

		instrumentOperationLatencies:      tf.operationLatencies,
		instrumentServerLatencies:         tf.serverLatencies,
		instrumentAttemptLatencies:        tf.attemptLatencies,
		instrumentAttemptLatencies2:       tf.attemptLatencies2,
		instrumentFirstRespLatencies:      tf.firstRespLatencies,
		instrumentAppBlockingLatencies:    tf.appBlockingLatencies,
		instrumentClientBlockingLatencies: tf.clientBlockingLatencies,
		instrumentRetryCount:              tf.retryCount,
		instrumentConnErrCount:            tf.connErrCount,
		instrumentDebugTags:               tf.debugTags,

		tableName:   tableName,
		isStreaming: isStreaming,
	}
}

func (mt *Tracer) SetMethod(m string) {
	mt.method = metricMethodPrefix + m
}

// toOtelMetricAttrs:
// - converts metric attributes values captured throughout the operation / attempt
// to OpenTelemetry attributes format,
// - combines these with common client attributes and returns
func (mt *Tracer) toOtelMetricAttrs(metricName string) (attribute.Set, error) {
	// Get metric details
	mDetails, found := MetricsDetails[metricName]
	if !found {
		return attribute.Set{}, fmt.Errorf("unable to create attributes list for unknown metric: %v", metricName)
	}

	clusterID := mt.currOp.currAttempt.clusterID
	zoneID := mt.currOp.currAttempt.zoneID
	status := mt.currOp.status

	if mDetails.recordedPerAttempt {
		status = mt.currOp.currAttempt.status
	} else {
		clusterID = FallbackString(clusterID, mt.currOp.lastClusterID)
		zoneID = FallbackString(zoneID, mt.currOp.lastZoneID)
	}

	attrKeyValues := make([]attribute.KeyValue, 0, maxAttrsLen)
	// Create attribute key value pairs for attributes common to all metricss
	attrKeyValues = append(attrKeyValues,
		attribute.String(MetricLabelKeyMethod, mt.method),

		// Add resource labels to otel metric labels.
		// These will be used for creating the monitored resource but exporter
		// will not add them to Google Cloud Monitoring metric labels
		attribute.String(MonitoredResLabelKeyTable, mt.tableName),

		attribute.String(MonitoredResLabelKeyCluster, clusterID),
		attribute.String(MonitoredResLabelKeyZone, zoneID),
	)
	attrKeyValues = append(attrKeyValues, mt.clientAttributes...)

	// Add additional attributes to metrics
	for _, attrKey := range mDetails.additionalAttrs {
		switch attrKey {
		case MetricLabelKeyStatus:
			attrKeyValues = append(attrKeyValues, attribute.String(MetricLabelKeyStatus, status))
		case MetricLabelKeyStreamingOperation:
			attrKeyValues = append(attrKeyValues, attribute.Bool(MetricLabelKeyStreamingOperation, mt.isStreaming))
		case MetricTransportType:
			attrKeyValues = append(attrKeyValues, attribute.String(MetricTransportType, mt.currOp.currAttempt.transportType))
		case MetricTransportRegion:
			attrKeyValues = append(attrKeyValues, attribute.String(MetricTransportRegion, mt.currOp.currAttempt.transportRegion))
		case MetricTransportSubZone:
			attrKeyValues = append(attrKeyValues, attribute.String(MetricTransportSubZone, mt.currOp.currAttempt.transportSubZone))
		case MetricTransportZone:
			attrKeyValues = append(attrKeyValues, attribute.String(MetricTransportZone, mt.currOp.currAttempt.transportZone))
		default:
			return attribute.Set{}, fmt.Errorf("unknown additional attribute: %v", attrKey)
		}
	}

	attrSet := attribute.NewSet(attrKeyValues...)
	return attrSet, nil
}

func (mt *Tracer) RecordAttemptStart() {
	if !mt.BuiltInEnabled {
		return
	}

	// Increment number of attempts
	mt.currOp.incrementAttemptCount()

	mt.currOp.currAttempt = AttemptTracer{}

	// record start time
	mt.currOp.currAttempt.SetStartTime(time.Now())
}

// RecordAttemptCompletionWithMetadata extracts location, server latency (with t4t7 fallback),
// and client blocking latency from headers, trailers, and active trackers, saves them to
// the current attempt tracer, and then records the attempt metrics.
func (mt *Tracer) RecordAttemptCompletionWithMetadata(attemptHeaderMD, attempTrailerMD metadata.MD, err error) {
	if !mt.BuiltInEnabled {
		return
	}

	// 1. Calculate client blocking latency
	if mt.currOp.currAttempt.blockingLatencyTracker != nil {
		messageSentNanos := mt.currOp.currAttempt.blockingLatencyTracker.getMessageSentNanos()
		if messageSentNanos > 0 {
			mt.currOp.currAttempt.clientBlockingLatency = ConvertToMs(time.Unix(0, messageSentNanos).Sub(mt.currOp.currAttempt.startTime))
		}
	}

	// 2. Extract server latency and apply t4t7 fallback
	serverLatency, serverLatencyErr := ExtractServerLatency(attemptHeaderMD, attempTrailerMD)
	if serverLatency == 0 && mt.currOp.currAttempt.t4t7Tracker != nil {
		fallbackLatency := mt.currOp.currAttempt.t4t7Tracker.getLatencyMs()
		if fallbackLatency > 0 {
			serverLatency = fallbackLatency
			serverLatencyErr = nil
		}
	}
	mt.currOp.currAttempt.serverLatency = serverLatency
	mt.currOp.currAttempt.serverLatencyErr = serverLatencyErr

	// 3. Call RecordAttemptCompletion
	mt.RecordAttemptCompletion(attemptHeaderMD, attempTrailerMD, err)
}

// RecordAttemptCompletion records as many attempt specific metrics as it can
// Ignore errors seen while creating metric attributes since metric can still
// be recorded with rest of the attributes
func (mt *Tracer) RecordAttemptCompletion(attemptHeaderMD, attempTrailerMD metadata.MD, err error) {
	if !mt.BuiltInEnabled {
		return
	}

	// Set attempt status
	statusCode, _ := convertToGrpcStatusErr(err)
	mt.currOp.currAttempt.setStatus(statusCode.String())

	// Get location attributes from metadata and set it in tracer.
	// Ignore get location error since the metric can still be recorded with
	// rest of the attributes. Don't overwrite a cluster/zone that the vRPC
	// path has already populated (SessionTable sets these directly from the
	// ClusterInformation payload); only fill in if the attempt's value is
	// missing or the sentinel default. lastClusterID/lastZoneID always track
	// the freshest real value so operation-level metrics get a sensible
	// fallback.
	clusterID, zoneID, _ := ExtractLocation(attemptHeaderMD, attempTrailerMD)
	if clusterID != "" {
		if existing := mt.currOp.currAttempt.clusterID; existing == "" || existing == defaultCluster {
			mt.currOp.currAttempt.SetClusterID(clusterID)
		}
		if clusterID != defaultCluster {
			mt.currOp.lastClusterID = clusterID
		}
	}
	if zoneID != "" {
		if existing := mt.currOp.currAttempt.zoneID; existing == "" || existing == defaultZone {
			mt.currOp.currAttempt.SetZoneID(zoneID)
		}
		if zoneID != defaultZone {
			mt.currOp.lastZoneID = zoneID
		}
	}

	// Extract transport labels from the bigtable-peer-info sideband metadata
	// (populated by the server when the PeerInfo feature flag is negotiated
	// on). Feeds the attempt_latencies2 metric only; other metrics stay on
	// the classic label set. No-op when the header is absent.
	if peerInfo, _ := ExtractPeerInfo(attemptHeaderMD, attempTrailerMD); peerInfo != nil {
		mt.currOp.currAttempt.transportType = btransport.TransportTypeName(peerInfo.GetTransportType())
		mt.currOp.currAttempt.transportRegion = peerInfo.GetApplicationFrontendRegion()
		mt.currOp.currAttempt.transportZone = peerInfo.GetApplicationFrontendZone()
		mt.currOp.currAttempt.transportSubZone = peerInfo.GetApplicationFrontendSubzone()
	}

	// Calculate elapsed time
	elapsedTime := ConvertToMs(time.Since(mt.currOp.currAttempt.startTime))

	// Record attempt_latencies
	attemptLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameAttemptLatencies)
	mt.instrumentAttemptLatencies.Record(mt.ctx, elapsedTime, metric.WithAttributeSet(attemptLatAttrs))

	// Record attempt_latencies2 — same value, but broken out by transport
	// labels from the peer-info sideband metadata.
	if mt.instrumentAttemptLatencies2 != nil {
		attemptLat2Attrs, _ := mt.toOtelMetricAttrs(MetricNameAttemptLatencies2)
		mt.instrumentAttemptLatencies2.Record(mt.ctx, elapsedTime, metric.WithAttributeSet(attemptLat2Attrs))
	}

	// Record client_blocking_latencies
	clientBlockingLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameClientBlockingLatencies)
	mt.instrumentClientBlockingLatencies.Record(mt.ctx, mt.currOp.currAttempt.clientBlockingLatency, metric.WithAttributeSet(clientBlockingLatAttrs))

	// Record server_latencies
	serverLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameServerLatencies)
	if mt.currOp.currAttempt.serverLatencyErr == nil {
		mt.instrumentServerLatencies.Record(mt.ctx, mt.currOp.currAttempt.serverLatency, metric.WithAttributeSet(serverLatAttrs))
	}

	// Record connectivity_error_count
	connErrCountAttrs, _ := mt.toOtelMetricAttrs(MetricNameConnErrCount)
	// Determine if connection error should be incremented.
	// A true connectivity error occurs only when we receive NO server-side signals.
	// 1. Server latency (from server-timing header) is a signal, but absent in DirectPath.
	// 2. Location (from x-goog-ext header) is a signal present in both paths.
	// Therefore, we only count an error if BOTH signals are missing.
	isServerLatencyEffectivelyEmpty := mt.currOp.currAttempt.serverLatencyErr != nil || mt.currOp.currAttempt.serverLatency == 0
	isLocationEmpty := mt.currOp.currAttempt.clusterID == defaultCluster
	if isServerLatencyEffectivelyEmpty && isLocationEmpty {
		// This is a connectivity error: the request likely never reached Google's network.
		mt.instrumentConnErrCount.Add(mt.ctx, 1, metric.WithAttributeSet(connErrCountAttrs))
	} else {
		mt.instrumentConnErrCount.Add(mt.ctx, 0, metric.WithAttributeSet(connErrCountAttrs))
	}
}

// RecordOperationCompletion records as many operation specific metrics as it can
// Ignores error seen while creating metric attributes since metric can still
// be recorded with rest of the attributes
func (mt *Tracer) RecordOperationCompletion() {
	if !mt.BuiltInEnabled {
		return
	}

	// Calculate elapsed time
	elapsedTimeMs := ConvertToMs(time.Since(mt.currOp.startTime))

	// Record operation_latencies
	opLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameOperationLatencies)
	mt.instrumentOperationLatencies.Record(mt.ctx, elapsedTimeMs, metric.WithAttributeSet(opLatAttrs))

	// Record first_reponse_latencies
	firstRespLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameFirstRespLatencies)
	if mt.method == metricMethodPrefix+methodNameReadRows {
		elapsedTimeMs = ConvertToMs(mt.currOp.firstRespTime.Sub(mt.currOp.startTime))
		mt.instrumentFirstRespLatencies.Record(mt.ctx, elapsedTimeMs, metric.WithAttributeSet(firstRespLatAttrs))
	}

	// Record retry_count
	retryCntAttrs, _ := mt.toOtelMetricAttrs(MetricNameRetryCount)
	if mt.currOp.attemptCount > 1 {
		// Only record when retry count is greater than 0 so the retry
		// graph will be less confusing
		mt.instrumentRetryCount.Add(mt.ctx, mt.currOp.attemptCount-1, metric.WithAttributeSet(retryCntAttrs))
	}

	// Record application_latencies
	appBlockingLatAttrs, _ := mt.toOtelMetricAttrs(MetricNameAppBlockingLatencies)
	mt.instrumentAppBlockingLatencies.Record(mt.ctx, mt.currOp.appBlockingLatency, metric.WithAttributeSet(appBlockingLatAttrs))
}

func (mt *Tracer) SetCurrOpStatus(code codes.Code) {
	if !mt.BuiltInEnabled {
		return
	}

	mt.currOp.setStatus(CanonicalString(code))
}

// SetFirstRespTime stamps the first-response timestamp used by the
// first_response_latencies histogram (ReadRows only). Exposed as a
// method so external callers don't need direct OpTracer field access.
func (mt *Tracer) SetFirstRespTime(t time.Time) {
	if !mt.BuiltInEnabled {
		return
	}
	mt.currOp.setFirstRespTime(t)
}

// Cookies returns the operation-scoped routing-cookie map (populated
// from response headers/trailers by ExtractCookiesFromMD). Callers
// iterate it to append cookies to the next outgoing attempt's metadata.
func (mt *Tracer) Cookies() map[string]string {
	return mt.currOp.cookies
}

// ExtractCookiesFromMD stores any headers in md whose key starts with
// cookiePrefix into the tracer's operation-scoped cookie map. Called by
// the classic path's gaxInvokeWithRecorder after each attempt so
// routing cookies persist across retries.
func (mt *Tracer) ExtractCookiesFromMD(md metadata.MD, cookiePrefix string) {
	for k, v := range md {
		if strings.HasPrefix(k, cookiePrefix) {
			mt.currOp.cookies[k] = v[len(v)-1]
		}
	}
}

// CurrAttempt returns a pointer to the in-progress AttemptTracer so
// external callers (e.g. the session-path data plane) can stamp
// per-attempt attributes — cluster_id, zone_id, transport labels,
// client-blocking latency, server latency. Returns nil when metrics
// are disabled so callers can bail cheaply.
func (mt *Tracer) CurrAttempt() *AttemptTracer {
	if !mt.BuiltInEnabled {
		return nil
	}
	return &mt.currOp.currAttempt
}

func CanonicalString(c codes.Code) string {
	if int(c) >= 0 && int(c) < len(canonicalStatusStrings) {
		if s := canonicalStatusStrings[c]; s != "" {
			return s
		}
	}
	return "UNKNOWN"
}

func (mt *Tracer) IncrementAppBlockingLatency(latency float64) {
	if !mt.BuiltInEnabled {
		return
	}

	mt.currOp.IncrementAppBlockingLatency(latency)
}

// RecordClientBlockingLatency stamps the per-attempt client-blocking latency
// as the elapsed time since the attempt started. The vRPC path calls this
// when it dispatches a request because there is no gRPC OutPayload stats
// event to drive blockingLatencyTracker — without this stamp, the stats
// handler would never populate clientBlockingLatency for vRPC attempts.
func (mt *Tracer) RecordClientBlockingLatency() {
	if !mt.BuiltInEnabled {
		return
	}
	startTime := mt.currOp.currAttempt.startTime
	if !startTime.IsZero() {
		mt.currOp.currAttempt.clientBlockingLatency = ConvertToMs(time.Since(startTime))
	}
}

// blockingLatencyTracker is used to calculate the time between stream creation and the first message send.
type blockingLatencyTracker struct {
	endNanos atomic.Int64
}

func (t *blockingLatencyTracker) recordLatency(end time.Time) {
	endN := end.UnixNano()
	// Ensure that only the time of the first OutPayload event is recorded.
	t.endNanos.CompareAndSwap(0, endN)
}

func (t *blockingLatencyTracker) getMessageSentNanos() int64 {
	return t.endNanos.Load()
}

// t4t7Tracker measures the time between sending the client
// request headers and receiving the initial metadata (InHeader) from the server.
type t4t7Tracker struct {
	outHeaderSentNanos atomic.Int64
	inHeaderRecvNanos  atomic.Int64
}

func (t *t4t7Tracker) recordOutHeaderSent(start time.Time) {
	// Ensure we only record the very first time headers are sent
	t.outHeaderSentNanos.CompareAndSwap(0, start.UnixNano())
}

func (t *t4t7Tracker) recordInHeaderRecv(end time.Time) {
	// Ensure we only record the very first time headers are received
	t.inHeaderRecvNanos.CompareAndSwap(0, end.UnixNano())
}

// getLatencyMs returns the calculated latency in milliseconds.
func (t *t4t7Tracker) getLatencyMs() float64 {
	start := t.outHeaderSentNanos.Load()
	end := t.inHeaderRecvNanos.Load()
	if start == 0 || end == 0 {
		return 0
	}
	return float64(end-start) / float64(time.Millisecond)
}

// StatsHandler is the gRPC stats.Handler that drives per-attempt metrics
// recording. It is the single source of truth for attempt boundaries: TagRPC
// starts a new attempt, HandleRPC observes the OutPayload/Header/Trailer events
// to feed the blocking-latency and t4t7 trackers, and the End event records
// attempt completion with the final status from gRPC (no io.EOF translation
// needed because stats.End.Error is nil on successful stream close).
//
// A *Tracer is plumbed through the call context by the public
// entry points (ReadRows, Apply, etc.) via NewContext. RPCs that
// don't carry a tracer (or carry a disabled one) are observed only for the
// existing blocking/t4t7 trackers if present, so non-Bigtable RPCs on the same
// channel emit no metrics.
type StatsHandler struct{}

var _ stats.Handler = (*StatsHandler)(nil)

func (h *StatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	mt := FromContext(ctx)
	if !mt.BuiltInEnabled {
		return ctx
	}

	mt.RecordAttemptStart()

	// Set method name if a caller (e.g. gaxInvokeWithRecorder) hasn't already.
	// strings.LastIndex avoids the slice allocation strings.Split would incur
	// on this per-attempt hot path.
	if mt.method == "" {
		if idx := strings.LastIndex(info.FullMethodName, "/"); idx != -1 {
			mt.SetMethod(info.FullMethodName[idx+1:])
		} else {
			mt.SetMethod(info.FullMethodName)
		}
	}

	blockTracker := &blockingLatencyTracker{}
	mt.currOp.currAttempt.blockingLatencyTracker = blockTracker
	ctx = context.WithValue(ctx, statsContextKey, blockTracker)

	t4t7 := &t4t7Tracker{}
	mt.currOp.currAttempt.t4t7Tracker = t4t7
	ctx = context.WithValue(ctx, t4t7ContextKey, t4t7)

	return ctx
}

func (h *StatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	if tracker, ok := ctx.Value(statsContextKey).(*blockingLatencyTracker); ok {
		if op, ok := s.(*stats.OutPayload); ok {
			tracker.recordLatency(op.SentTime)
		}
	}

	if t4t7, ok := ctx.Value(t4t7ContextKey).(*t4t7Tracker); ok {
		switch s.(type) {
		case *stats.OutHeader:
			// The client has sent the request headers.
			t4t7.recordOutHeaderSent(time.Now())
		case *stats.InHeader:
			// The client has received the initial metadata from the server.
			t4t7.recordInHeaderRecv(time.Now())
		}
	}

	mt := FromContext(ctx)
	if !mt.BuiltInEnabled {
		return
	}
	switch ev := s.(type) {
	case *stats.InHeader:
		mt.currOp.currAttempt.headerMD = ev.Header
	case *stats.InTrailer:
		mt.currOp.currAttempt.trailerMD = ev.Trailer
	case *stats.End:
		// stats.End fires after InTrailer and before the caller's final
		// RecvMsg returns, so currAttempt.{header,trailer}MD are populated.
		// ev.Error is nil on graceful stream close, so attempt status maps
		// to OK without any io.EOF special-casing.
		mt.RecordAttemptCompletionWithMetadata(
			mt.currOp.currAttempt.headerMD,
			mt.currOp.currAttempt.trailerMD,
			ev.Error,
		)
	}
}

func (h *StatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *StatsHandler) HandleConn(context.Context, stats.ConnStats) {}

func FallbackString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
