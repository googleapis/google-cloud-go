/*
Copyright 2024 Google LLC

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

package bigtable

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigtable/internal"
	"github.com/google/uuid"
	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	// instrumentationScope is the instrumentation name that will be associated with the emitted telemetry.
	instrumentationScope = "cloud.google.com/go"

	metricsPrefix     = "bigtable/"
	locationMDKey     = "x-goog-ext-425905942-bin"
	serverTimingMDKey = "server-timing"

	// Monitored resource labels
	monitoredResLabelKeyProject  = "project_id"
	monitoredResLabelKeyInstance = "instance"
	monitoredResLabelKeyTable    = "table"
	monitoredResLabelKeyCluster  = "cluster"
	monitoredResLabelKeyZone     = "zone"

	// Metric labels
	metricLabelKeyAppProfile         = "app_profile"
	metricLabelKeyMethod             = "method"
	metricLabelKeyOperationStatus    = "status"
	metricLabelKeyStreamingOperation = "streaming"
	metricLabelKeyClientName         = "client_name"
	metricLabelKeyClientUID          = "client_uid"

	// Metric names
	metricNameOperationLatencies = "operation_latencies"
	metricNameAttemptLatencies   = "attempt_latencies"
	metricNameRetryCount         = "retry_count"
	metricNameServerLatencies    = "server_latencies"
)

var (
	// duration between two metric exports
	defaultSamplePeriod = 5 * time.Minute

	clientName            = fmt.Sprintf("cloud.google.com/go/bigtable v%v", internal.Version)
	builtInEnabledDefault = true

	// All the built-in metrics have same attributes except 'status' and 'streaming'
	// These attributes need to be added to only few of the metrics
	builtinMetrics = map[string]metricInfo{
		metricNameOperationLatencies: {
			desc:       "Total time until final operation success or failure, including retries and backoff.",
			metricType: otelmetric.InstrumentKindHistogram,
			unit:       "ns",
			additionalAttributes: []string{
				metricLabelKeyOperationStatus,
				metricLabelKeyStreamingOperation,
			},
		},
		metricNameAttemptLatencies: {
			desc:       "Client observed latency per RPC attempt.",
			metricType: otelmetric.InstrumentKindHistogram,
			unit:       "ns",
			additionalAttributes: []string{
				metricLabelKeyOperationStatus,
				metricLabelKeyStreamingOperation,
			},
		},
		metricNameServerLatencies: {
			desc:       "The latency measured from the moment that the RPC entered the Google data center until the RPC was completed.",
			metricType: otelmetric.InstrumentKindHistogram,
			unit:       "ns",
			additionalAttributes: []string{
				metricLabelKeyOperationStatus,
				metricLabelKeyStreamingOperation,
			},
		},
		metricNameRetryCount: {
			desc:       "The number of additional RPCs sent after the initial attempt.",
			metricType: otelmetric.InstrumentKindCounter,
			additionalAttributes: []string{
				metricLabelKeyOperationStatus,
			},
		},
	}

	noOpRecordFn = func() {}
)

type metricInfo struct {
	desc                 string
	metricType           otelmetric.InstrumentKind
	unit                 string
	additionalAttributes []string
}

type builtInMetricInstruments struct {
	distributions map[string]metric.Float64Histogram // Key is metric name e.g. operation_latencies
	counters      map[string]metric.Int64Counter     // Key is metric name e.g. retry_count
}

// Generates unique client ID in the format go-<random UUID>@<>hostname
func generateClientUID() (string, error) {
	hostname := "localhost"
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return "go-" + uuid.NewString() + "@" + hostname, nil
}

type metricsConfigInternal struct {
	exporter otelmetric.Exporter

	project string

	// Flag to indicate whether built-in metrics should be recorded and exported to GCM
	builtInEnabled bool

	// attributes that are specific to a client instance and
	// do not change across different function calls on client
	clientAttributes []attribute.KeyValue

	// Contains one entry per meter provider
	instruments []builtInMetricInstruments
}

func newMetricsConfigInternal(ctx context.Context, project, instance string, userProvidedConfig *metricsConfig, opts ...option.ClientOption) (*metricsConfigInternal, error) {
	clientUID, err := generateClientUID()
	if err != nil {
		log.Printf("built-in metrics: generateClientUID failed: %v. Using empty string in the %v metric atteribute", err, metricLabelKeyClientUID)
	}

	// Create default meter provider
	internalMetricsConfig := &metricsConfigInternal{
		project: project,
		clientAttributes: []attribute.KeyValue{
			attribute.String(monitoredResLabelKeyProject, project),
			attribute.String(monitoredResLabelKeyInstance, instance),
			attribute.String(metricLabelKeyClientUID, clientUID),
			attribute.String(metricLabelKeyClientName, clientName),
		},
		instruments:    []builtInMetricInstruments{},
		builtInEnabled: builtInEnabledDefault,
	}

	defaultExporter, err := newMonitoringExporter(ctx, internalMetricsConfig.project, opts...)
	if err != nil {
		return nil, err
	}
	internalMetricsConfig.exporter = defaultExporter

	// Create default meter provider
	defaultMp := otelmetric.NewMeterProvider(
		otelmetric.WithReader(
			otelmetric.NewPeriodicReader(
				defaultExporter,
				otelmetric.WithInterval(defaultSamplePeriod),
			),
		),
	)

	userProvidedMeterProviders := []*otelmetric.MeterProvider{}
	if addr := os.Getenv("BIGTABLE_EMULATOR_HOST"); addr != "" {
		// Do not emit metrics when emulator is being used
		internalMetricsConfig.builtInEnabled = false
	} else if userProvidedConfig != nil {
		internalMetricsConfig.builtInEnabled = userProvidedConfig.builtInEnabled
		userProvidedMeterProviders = userProvidedConfig.meterProviders
	}

	if !internalMetricsConfig.builtInEnabled {
		return internalMetricsConfig, nil
	}

	// Create instruments on all meter providers
	allMeterProviders := append(userProvidedMeterProviders, defaultMp)
	for _, mp := range allMeterProviders {
		builtInMetricInstruments := builtInMetricInstruments{
			distributions: make(map[string]metric.Float64Histogram),
			counters:      make(map[string]metric.Int64Counter),
		}

		// Create meter
		meter := mp.Meter(instrumentationScope, metric.WithInstrumentationVersion(internal.Version))

		// Create instruments
		for metricName, metricDetails := range builtinMetrics {
			if metricDetails.metricType == otelmetric.InstrumentKindHistogram {
				builtInMetricInstruments.distributions[metricName], err = meter.Float64Histogram(
					metricName,
					metric.WithDescription(metricDetails.desc),
					metric.WithUnit(metricDetails.unit),
				)
				if err != nil {
					return internalMetricsConfig, err
				}
			} else if metricDetails.metricType == otelmetric.InstrumentKindCounter {
				builtInMetricInstruments.counters[metricName], err = meter.Int64Counter(
					metricName,
					metric.WithDescription(metricDetails.desc),
					metric.WithUnit(metricDetails.unit),
				)
				if err != nil {
					return internalMetricsConfig, err
				}
			}
			internalMetricsConfig.instruments = append(internalMetricsConfig.instruments, builtInMetricInstruments)
		}
	}
	return internalMetricsConfig, nil
}

func (config *metricsConfigInternal) getOperationRecorder(ctx context.Context, table string, appProfile string, method string, isStreaming bool) (*builtinMetricsTracer, func()) {
	mt := newBuiltinMetricsTracer(method, table, appProfile, isStreaming)
	return &mt, config.recordOperationCompletion(ctx, &mt)
}

func (config *metricsConfigInternal) gaxInvokeWithRecorder(ctx context.Context, mt *builtinMetricsTracer,
	f func(ctx context.Context, _ gax.CallSettings) error, opts ...gax.CallOption) error {

	callWrapper := func(ctx context.Context, callSettings gax.CallSettings) error {
		// Increment number of attempts
		mt.attemptCount++

		recorder := config.recordAttemptCompletion(ctx, mt)
		defer recorder()

		err := f(ctx, callSettings)

		// Record attempt status
		statusCode, _ := convertToGrpcStatusErr(err)
		mt.status = statusCode.String()
		return err
	}
	return gax.Invoke(ctx, callWrapper, opts...)
}

// recordAttemptCompletion returns a function that should be executed to record attempt specific metrics
// It records as many metrics as it can and does not return error
func (config *metricsConfigInternal) recordAttemptCompletion(ctx context.Context, mt *builtinMetricsTracer) func() {
	if !config.builtInEnabled {
		return noOpRecordFn
	}
	startTime := time.Now()

	return func() {
		// Calculate elapsed time
		elapsedTime := time.Since(startTime).Nanoseconds()

		// Attributes for attempt_latencies
		attemptLatCurrCallAttrs, attemptLatErr := mt.toOtelMetricAttrs(metricNameAttemptLatencies)
		attemptLatAllAttrs := metric.WithAttributes(append(config.clientAttributes, attemptLatCurrCallAttrs...)...)

		// Attributes for server_latencies
		serverLatCurrCallAttrs, serverLatErr := mt.toOtelMetricAttrs(metricNameServerLatencies)
		serverLatAllAttres := metric.WithAttributes(append(config.clientAttributes, serverLatCurrCallAttrs...)...)

		for _, builtInMetricInstruments := range config.instruments {
			if attemptLatErr == nil {
				builtInMetricInstruments.distributions[metricNameAttemptLatencies].Record(ctx, float64(elapsedTime), attemptLatAllAttrs)
			}

			if serverLatErr == nil {
				serverLatency, serverLatErr := mt.getServerLatency()
				if serverLatErr == nil {
					builtInMetricInstruments.distributions[metricNameServerLatencies].Record(ctx, float64(serverLatency), serverLatAllAttres)
				}
			}
		}
	}
}

// recordOperationCompletion returns a function that should be executed to record total operation metrics
// It records as many metrics as it can and does not return error on first failure
func (config *metricsConfigInternal) recordOperationCompletion(ctx context.Context, mt *builtinMetricsTracer) func() {
	if !config.builtInEnabled {
		return noOpRecordFn
	}
	startTime := time.Now()

	return func() {
		// Calculate elapsed time
		elapsedTime := time.Since(startTime).Nanoseconds()

		// Attributes for operation_latencies
		opLatCurrCallAttrs, opLatErr := mt.toOtelMetricAttrs(metricNameOperationLatencies)
		opLatAllAttrs := metric.WithAttributes(append(config.clientAttributes, opLatCurrCallAttrs...)...)

		// Attributes for retry_count
		retryCntCurrCallAttrs, retryCountErr := mt.toOtelMetricAttrs(metricNameRetryCount)
		retryCntAllAttrs := metric.WithAttributes(append(config.clientAttributes, retryCntCurrCallAttrs...)...)

		for _, builtInMetricInstruments := range config.instruments {
			if opLatErr == nil {
				builtInMetricInstruments.distributions[metricNameOperationLatencies].Record(ctx, float64(elapsedTime), opLatAllAttrs)
			}

			// Only record when retry count is greater than 0 so the retry
			// graph will be less confusing
			if mt.attemptCount > 1 {
				if retryCountErr == nil {
					builtInMetricInstruments.counters[metricNameRetryCount].Add(ctx, mt.attemptCount-1, retryCntAllAttrs)
				}
			}
		}
	}
}

// builtinMetricsTracer is created one per function call
// It is used to store metric attribute values and other data required to obtain them
type builtinMetricsTracer struct {
	tableName    string
	appProfileID string
	method       string
	isStreaming  bool

	// gRPC status code
	status string

	// Contains the header response metadata which is used to extract cluster and zone
	headerMD *metadata.MD

	// Contains the trailer response metadata which is used to extract cluster and zone
	trailerMD *metadata.MD

	attemptCount int64
}

func newBuiltinMetricsTracer(method, tableName, appProfile string, isStreaming bool) builtinMetricsTracer {
	headerMD := metadata.New(nil)
	trailerMD := metadata.New(nil)
	return builtinMetricsTracer{
		tableName:    tableName,
		appProfileID: appProfile,
		method:       method,
		isStreaming:  isStreaming,
		status:       "",
		headerMD:     &headerMD,
		trailerMD:    &trailerMD,
		attemptCount: 0,
	}
}

func (mt *builtinMetricsTracer) recordAndConvertErr(err error) error {
	statusCode, statusErr := convertToGrpcStatusErr(err)
	mt.status = statusCode.String()
	return statusErr
}

// mt.toOtelMetricAttrs converts recorded metric attributes values to OpenTelemetry attributes format
func (mt *builtinMetricsTracer) toOtelMetricAttrs(metricName string) ([]attribute.KeyValue, error) {
	clusterID, zoneID, _ := mt.getLocation()

	// Create attribute key value pairs for attributes common to all metricss
	attrKeyValues := []attribute.KeyValue{
		attribute.String(metricLabelKeyAppProfile, mt.appProfileID),
		attribute.String(metricLabelKeyMethod, mt.method),

		// Add resource labels to otel metric labels.
		// These will be used for creating the monitored resource but exporter
		// will not add them to Google Cloud Monitoring metric labels
		attribute.String(monitoredResLabelKeyTable, mt.tableName),
		attribute.String(monitoredResLabelKeyCluster, clusterID),
		attribute.String(monitoredResLabelKeyZone, zoneID),
	}

	metricDetails, found := builtinMetrics[metricName]
	if !found {
		return nil, fmt.Errorf("Unable to create attributes list for unknown metric: %v", metricName)
	}

	// Add additional attributes to metrics
	for _, attrKey := range metricDetails.additionalAttributes {
		switch attrKey {
		case metricLabelKeyOperationStatus:
			attrKeyValues = append(attrKeyValues, attribute.String(metricLabelKeyOperationStatus, mt.status))
		case metricLabelKeyStreamingOperation:
			attrKeyValues = append(attrKeyValues, attribute.Bool(metricLabelKeyStreamingOperation, mt.isStreaming))
		default:
			return nil, fmt.Errorf("Unknown additional attribute: %v", attrKey)
		}
	}

	return attrKeyValues, nil
}

// get GFE latency from response metadata
func (mt *builtinMetricsTracer) getServerLatency() (int, error) {
	serverLatencyNano := 0
	serverTimingStr := ""

	// Check whether server latency available in response header metadata
	if mt.headerMD != nil {
		headerMDValues := mt.headerMD.Get(serverTimingMDKey)
		if len(headerMDValues) != 0 {
			serverTimingStr = headerMDValues[0]
		}
	}

	if len(serverTimingStr) == 0 {
		// Check whether server latency available in response trailer metadata
		if mt.trailerMD != nil {
			trailerMDValues := mt.trailerMD.Get(serverTimingMDKey)
			if len(trailerMDValues) != 0 {
				serverTimingStr = trailerMDValues[0]
			}
		}
	}

	serverTimingValPrefix := "gfet4t7; dur="
	serverLatencyMillis, err := strconv.Atoi(strings.TrimPrefix(serverTimingStr, serverTimingValPrefix))
	if !strings.HasPrefix(serverTimingStr, serverTimingValPrefix) || err != nil {
		return serverLatencyNano, err
	}

	serverLatencyNano = serverLatencyMillis * 1000000
	return serverLatencyNano, nil
}

// Obtain cluster and zone from response metadata
func (mt *builtinMetricsTracer) getLocation() (string, string, error) {
	var locationMetadata []string

	// Check whether location metadata available in response header metadata
	if mt.headerMD != nil {
		locationMetadata = mt.headerMD.Get(locationMDKey)
	}

	if locationMetadata == nil {
		// Check whether location metadata available in response trailer metadata
		// if none found in response header metadata
		if mt.trailerMD != nil {
			locationMetadata = mt.trailerMD.Get(locationMDKey)
		}
	}

	if len(locationMetadata) < 1 {
		return "", "", fmt.Errorf("Failed to get location metadata")
	}

	// Unmarshal binary location metadata
	responseParams := &btpb.ResponseParams{}
	err := proto.Unmarshal([]byte(locationMetadata[0]), responseParams)
	if err != nil {
		return "", "", err
	}

	return responseParams.GetClusterId(), responseParams.GetZoneId(), nil
}
