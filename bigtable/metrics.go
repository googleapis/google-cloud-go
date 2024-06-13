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
	"errors"
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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	builtInMetricsMeterName = "bigtable.googleapis.com/internal/client/"

	metricsPrefix         = "bigtable/"
	locationMDKey         = "x-goog-ext-425905942-bin"
	serverTimingMDKey     = "server-timing"
	serverTimingValPrefix = "gfet4t7; dur="

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
	metricNameServerLatencies    = "server_latencies"
	metricNameRetryCount         = "retry_count"
)

var (
	// duration between two metric exports
	defaultSamplePeriod = 5 * time.Minute

	clientName            = fmt.Sprintf("cloud.google.com/go/bigtable v%v", internal.Version)
	builtInEnabledDefault = true

	bucketBounds = []float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0, 20.0, 25.0, 30.0, 40.0,
		50.0, 65.0, 80.0, 100.0, 130.0, 160.0, 200.0, 250.0, 300.0, 400.0, 500.0, 650.0,
		800.0, 1000.0, 2000.0, 5000.0, 10000.0, 20000.0, 50000.0, 100000.0, 200000.0,
		400000.0, 800000.0, 1600000.0, 3200000.0}

	// All the built-in metrics have same attributes except 'status' and 'streaming'
	// These attributes need to be added to only few of the metrics
	additionalAttributes = map[string][]string{
		metricNameOperationLatencies: {
			metricLabelKeyOperationStatus,
			metricLabelKeyStreamingOperation,
		},
		metricNameAttemptLatencies: {
			metricLabelKeyOperationStatus,
			metricLabelKeyStreamingOperation,
		},
		metricNameServerLatencies: {
			metricLabelKeyOperationStatus,
			metricLabelKeyStreamingOperation,
		},
		metricNameRetryCount: {
			metricLabelKeyOperationStatus,
		},
	}

	noOpRecordFn = func() {}

	// Generates unique client ID in the format go-<random UUID>@<>hostname
	generateClientUID = func() (string, error) {
		hostname := "localhost"
		hostname, err := os.Hostname()
		if err != nil {
			return "", err
		}
		return "go-" + uuid.NewString() + "@" + hostname, nil
	}

	exporterOpts = []option.ClientOption{}
)

// getBuiltInMeterProviderOptions returns meter provider options, shutdown function and error
func getBuiltInMeterProviderOptions(ctx context.Context, project string) (sdkmetric.Option, error) {
	defaultExporter, err := newMonitoringExporter(ctx, project, exporterOpts...)
	if err != nil {
		return nil, err
	}

	return sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(
			defaultExporter,
			sdkmetric.WithInterval(defaultSamplePeriod),
		),
	), nil
}

type builtinMetricsTracerFactory struct {
	builtinEnabled bool

	// To be called on client close
	shutdown func()

	// attributes that are specific to a client instance and
	// do not change across different function calls on client
	clientAttributes []attribute.KeyValue

	operationLatencies metric.Float64Histogram
	serverLatencies    metric.Float64Histogram
	attemptLatencies   metric.Float64Histogram
	retryCount         metric.Int64Counter
}

func newBuiltinMetricsTracerFactory(ctx context.Context, project, instance, appProfile string, metricsProvider MetricsProvider) (*builtinMetricsTracerFactory, error) {
	clientUID, err := generateClientUID()
	if err != nil {
		log.Printf("built-in metrics: generateClientUID failed: %v. Using empty string in the %v metric atteribute", err, metricLabelKeyClientUID)
	}

	tracerFactory := &builtinMetricsTracerFactory{
		builtinEnabled: false,
		clientAttributes: []attribute.KeyValue{
			attribute.String(monitoredResLabelKeyProject, project),
			attribute.String(monitoredResLabelKeyInstance, instance),
			attribute.String(metricLabelKeyAppProfile, appProfile),
			attribute.String(metricLabelKeyClientUID, clientUID),
			attribute.String(metricLabelKeyClientName, clientName),
		},
		shutdown: func() {},
	}

	if emulatorAddr := os.Getenv("BIGTABLE_EMULATOR_HOST"); emulatorAddr != "" {
		// Do not emit metrics when emulator is being used
		return tracerFactory, nil
	}

	var meterProvider *sdkmetric.MeterProvider
	if metricsProvider == nil {
		// Create default meter provider
		mpOptions, err := getBuiltInMeterProviderOptions(ctx, project)
		if err != nil {
			return tracerFactory, err
		}
		meterProvider = sdkmetric.NewMeterProvider(mpOptions)

		tracerFactory.builtinEnabled = true
		tracerFactory.shutdown = func() { meterProvider.Shutdown(ctx) }
	} else {
		switch v := metricsProvider.(type) {
		case CustomOpenTelemetryMetricsProvider:
			// User provided meter provider
			tracerFactory.builtinEnabled = true
			meterProvider = v.MeterProvider
		case NoopMetricsProvider:
			tracerFactory.builtinEnabled = false
			return tracerFactory, nil
		default:
			tracerFactory.builtinEnabled = false
			return tracerFactory, errors.New("Unknown MetricsProvider type")
		}
	}

	// Create meter and instruments
	meter := meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	err = tracerFactory.createInstruments(meter)
	return tracerFactory, err
}

func (tf *builtinMetricsTracerFactory) createInstruments(meter metric.Meter) error {
	var err error

	// Create operation_latencies
	tf.operationLatencies, err = meter.Float64Histogram(
		metricNameOperationLatencies,
		metric.WithDescription("Total time until final operation success or failure, including retries and backoff."),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create attempt_latencies
	tf.attemptLatencies, err = meter.Float64Histogram(
		metricNameAttemptLatencies,
		metric.WithDescription("Client observed latency per RPC attempt."),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create server_latencies
	tf.serverLatencies, err = meter.Float64Histogram(
		metricNameServerLatencies,
		metric.WithDescription("The latency measured from the moment that the RPC entered the Google data center until the RPC was completed."),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create retry_count
	tf.retryCount, err = meter.Int64Counter(
		metricNameRetryCount,
		metric.WithDescription("The number of additional RPCs sent after the initial attempt."),
	)
	return err
}

// builtinMetricsTracer is created one per function call
// It is used to store metric instruments, attribute values
// and other data required to obtain and record them
type builtinMetricsTracer struct {
	ctx            context.Context
	builtInEnabled bool
	opStartTime    time.Time

	// attributes that are specific to a client instance and
	// do not change across different function calls on client
	clientAttributes []attribute.KeyValue

	instrumentOperationLatencies metric.Float64Histogram
	instrumentServerLatencies    metric.Float64Histogram
	instrumentAttemptLatencies   metric.Float64Histogram
	instrumentRetryCount         metric.Int64Counter

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

func (tf *builtinMetricsTracerFactory) newBuiltinMetricsTracer(ctx context.Context, tableName, appProfile string, isStreaming bool) builtinMetricsTracer {
	headerMD := metadata.New(nil)
	trailerMD := metadata.New(nil)
	return builtinMetricsTracer{
		ctx:              ctx,
		builtInEnabled:   tf.builtinEnabled,
		opStartTime:      time.Now(),
		clientAttributes: tf.clientAttributes,

		instrumentOperationLatencies: tf.operationLatencies,
		instrumentServerLatencies:    tf.serverLatencies,
		instrumentAttemptLatencies:   tf.attemptLatencies,
		instrumentRetryCount:         tf.retryCount,

		tableName:    tableName,
		appProfileID: appProfile,
		isStreaming:  isStreaming,
		status:       "",
		headerMD:     &headerMD,
		trailerMD:    &trailerMD,
		attemptCount: 0,
	}
}

// recordOperationCompletion records as many metrics as it can and does not return error
func (mt *builtinMetricsTracer) recordOperationCompletion() {
	if !mt.builtInEnabled {
		return
	}

	// Calculate elapsed time
	elapsedTimeMs := float64(time.Since(mt.opStartTime).Nanoseconds()) / 1000000

	// Attributes for operation_latencies
	opLatCurrCallAttrs, opLatErr := mt.toOtelMetricAttrs(metricNameOperationLatencies)
	opLatAllAttrs := metric.WithAttributes(append(mt.clientAttributes, opLatCurrCallAttrs...)...)

	// Attributes for retry_count
	retryCntCurrCallAttrs, retryCountErr := mt.toOtelMetricAttrs(metricNameRetryCount)
	retryCntAllAttrs := metric.WithAttributes(append(mt.clientAttributes, retryCntCurrCallAttrs...)...)

	if opLatErr == nil {
		mt.instrumentOperationLatencies.Record(mt.ctx, elapsedTimeMs, opLatAllAttrs)
	}

	// Only record when retry count is greater than 0 so the retry
	// graph will be less confusing
	if mt.attemptCount > 1 {
		if retryCountErr == nil {
			mt.instrumentRetryCount.Add(mt.ctx, mt.attemptCount-1, retryCntAllAttrs)
		}
	}
}

func (mt *builtinMetricsTracer) gaxInvokeWithRecorder(ctx context.Context, method string,
	f func(ctx context.Context, _ gax.CallSettings) error, opts ...gax.CallOption) error {

	mt.method = method
	callWrapper := func(ctx context.Context, callSettings gax.CallSettings) error {
		// Increment number of attempts
		mt.attemptCount++

		// record start time
		startTime := time.Now()
		defer func() {
			if !mt.builtInEnabled {
				return
			}

			// Calculate elapsed time
			elapsedTime := float64(time.Since(startTime).Nanoseconds()) / 1000000

			// Attributes for attempt_latencies
			attemptLatCurrCallAttrs, attemptLatErr := mt.toOtelMetricAttrs(metricNameAttemptLatencies)
			attemptLatAllAttrs := metric.WithAttributes(append(mt.clientAttributes, attemptLatCurrCallAttrs...)...)

			// Attributes for server_latencies
			serverLatCurrCallAttrs, serverLatErr := mt.toOtelMetricAttrs(metricNameServerLatencies)
			serverLatAllAttres := metric.WithAttributes(append(mt.clientAttributes, serverLatCurrCallAttrs...)...)

			if attemptLatErr == nil {
				mt.instrumentAttemptLatencies.Record(mt.ctx, elapsedTime, attemptLatAllAttrs)
			}

			if serverLatErr == nil {
				serverLatency, serverLatErr := mt.getServerLatency()
				if serverLatErr == nil {
					mt.instrumentServerLatencies.Record(mt.ctx, serverLatency, serverLatAllAttres)
				}
			}
		}()

		// Make call to CBT service
		err := f(ctx, callSettings)

		// Record attempt status
		statusCode, _ := convertToGrpcStatusErr(err)
		mt.status = statusCode.String()
		return err
	}
	return gax.Invoke(ctx, callWrapper, opts...)
}

func (mt *builtinMetricsTracer) recordAndConvertErr(err error) error {
	statusCode, statusErr := convertToGrpcStatusErr(err)
	mt.status = statusCode.String()
	return statusErr
}

// toOtelMetricAttrs converts recorded metric attributes values to OpenTelemetry attributes format
func (mt *builtinMetricsTracer) toOtelMetricAttrs(metricName string) ([]attribute.KeyValue, error) {
	clusterID, zoneID, _ := mt.getLocation()

	// Create attribute key value pairs for attributes common to all metricss
	attrKeyValues := []attribute.KeyValue{
		attribute.String(metricLabelKeyMethod, mt.method),

		// Add resource labels to otel metric labels.
		// These will be used for creating the monitored resource but exporter
		// will not add them to Google Cloud Monitoring metric labels
		attribute.String(monitoredResLabelKeyTable, mt.tableName),
		attribute.String(monitoredResLabelKeyCluster, clusterID),
		attribute.String(monitoredResLabelKeyZone, zoneID),
	}

	attrs, found := additionalAttributes[metricName]
	if !found {
		return nil, fmt.Errorf("Unable to create attributes list for unknown metric: %v", metricName)
	}

	// Add additional attributes to metrics
	for _, attrKey := range attrs {
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
func (mt *builtinMetricsTracer) getServerLatency() (float64, error) {
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

	serverLatencyMillisStr := strings.TrimPrefix(serverTimingStr, serverTimingValPrefix)
	serverLatencyMillis, err := strconv.ParseFloat(strings.TrimSpace(serverLatencyMillisStr), 64)
	if !strings.HasPrefix(serverTimingStr, serverTimingValPrefix) || err != nil {
		return serverLatencyMillis, err
	}

	return serverLatencyMillis, nil
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
