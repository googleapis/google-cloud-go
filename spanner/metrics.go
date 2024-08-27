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

package spanner

import (
	"context"
	"errors"
	"fmt"

	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloud.google.com/go/spanner/internal"
)

const (
	builtInMetricsMeterName = "gax-go"

	nativeMetricsPrefix = "spanner.googleapis.com/internal/client/"

	// Monitored resource labels
	monitoredResLabelKeyProject        = "project_id"
	monitoredResLabelKeyInstance       = "instance_id"
	monitoredResLabelKeyInstanceConfig = "instance_config"
	monitoredResLabelKeyLocation       = "location"

	// Metric labels
	metricLabelKeyClientUID         = "client_uid"
	metricLabelKeyClientName        = "client_name"
	metricLabelKeyDatabase          = "database"
	metricLabelKeyMethod            = "method"
	metricLabelKeyStatus            = "status"
	metricLabelKeyDirectPathEnabled = "directpath_enabled"
	metricLabelKeyDirectPathUsed    = "directpath_used"

	// Metric names
	metricNameOperationLatencies = "operation_latencies"
	metricNameAttemptLatencies   = "attempt_latencies"
	metricNameOperationCount     = "operation_count"
	metricNameAttemptCount       = "attempt_count"

	// Metric units
	metricUnitMS    = "ms"
	metricUnitCount = "1"
)

// These are effectively const, but for testing purposes they are mutable
var (
	// duration between two metric exports
	defaultSamplePeriod = 5 * time.Minute

	clientName = fmt.Sprintf("spanner-go/%v", internal.Version)

	bucketBounds = []float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0, 20.0, 25.0, 30.0, 40.0,
		50.0, 65.0, 80.0, 100.0, 130.0, 160.0, 200.0, 250.0, 300.0, 400.0, 500.0, 650.0,
		800.0, 1000.0, 2000.0, 5000.0, 10000.0, 20000.0, 50000.0, 100000.0, 200000.0,
		400000.0, 800000.0, 1600000.0, 3200000.0}

	// All the built-in metrics have same attributes except 'status' and 'streaming'
	// These attributes need to be added to only few of the metrics
	metricsDetails = map[string]metricInfo{
		metricNameOperationCount: {
			additionalAttrs: []string{
				metricLabelKeyStatus,
			},
			recordedPerAttempt: false,
		},
		metricNameOperationLatencies: {
			additionalAttrs: []string{
				metricLabelKeyStatus,
			},
			recordedPerAttempt: false,
		},
		metricNameAttemptLatencies: {
			additionalAttrs: []string{
				metricLabelKeyStatus,
			},
			recordedPerAttempt: true,
		},
		metricNameAttemptCount: {
			additionalAttrs: []string{
				metricLabelKeyStatus,
			},
			recordedPerAttempt: true,
		},
	}

	// Generates unique client ID in the format go-<random UUID>@<hostname>
	generateClientUID = func() (string, error) {
		hostname := "localhost"
		hostname, err := os.Hostname()
		if err != nil {
			return "", err
		}
		return uuid.NewString() + "@" + strconv.FormatInt(int64(os.Getpid()), 10) + "@" + hostname, nil
	}

	exporterOpts = []option.ClientOption{}
)

type metricInfo struct {
	additionalAttrs    []string
	recordedPerAttempt bool
}

type builtinMetricsTracerFactory struct {
	enabled             bool
	isDirectPathEnabled bool

	// To be called on client close
	shutdown func()

	// attributes that are specific to a client instance and
	// do not change across different function calls on client
	clientAttributes []attribute.KeyValue

	operationLatencies metric.Float64Histogram
	attemptLatencies   metric.Float64Histogram
	operationCount     metric.Int64Counter
	attemptCount       metric.Int64Counter
}

func detectClientLocation(ctx context.Context) string {
	resource, err := gcp.NewDetector().Detect(ctx)
	if err != nil {
		return "global"
	}
	for _, attr := range resource.Attributes() {
		if attr.Key == semconv.CloudRegionKey {
			return attr.Value.AsString()
		}
	}
	// If region is not found, return global
	return "global"
}

func newBuiltinMetricsTracerFactory(ctx context.Context, dbpath string, metricsProvider metric.MeterProvider) (*builtinMetricsTracerFactory, error) {
	clientUID, err := generateClientUID()
	if err != nil {
		log.Printf("built-in metrics: generateClientUID failed: %v. Using empty string in the %v metric atteribute", err, metricLabelKeyClientUID)
	}
	project, instance, database, err := parseDatabaseName(dbpath)
	if err != nil {
		return nil, err
	}

	tracerFactory := &builtinMetricsTracerFactory{
		enabled: false,
		clientAttributes: []attribute.KeyValue{
			attribute.String(monitoredResLabelKeyProject, project),
			attribute.String(monitoredResLabelKeyInstance, instance),
			attribute.String(metricLabelKeyDatabase, database),
			attribute.String(metricLabelKeyClientUID, clientUID),
			attribute.String(metricLabelKeyClientName, clientName),
			// Skipping instance config until we have a way to get it
			attribute.String(monitoredResLabelKeyInstanceConfig, "unknown"),
			attribute.String(monitoredResLabelKeyLocation, detectClientLocation(ctx)),
		},
		shutdown: func() {},
	}

	tracerFactory.isDirectPathEnabled = false

	var meterProvider *sdkmetric.MeterProvider
	if metricsProvider == nil {
		// Create default meter provider
		mpOptions, err := builtInMeterProviderOptions(project)
		if err != nil {
			return tracerFactory, err
		}
		meterProvider = sdkmetric.NewMeterProvider(mpOptions...)

		tracerFactory.enabled = true
		tracerFactory.shutdown = func() { meterProvider.Shutdown(ctx) }
	} else {
		switch metricsProvider.(type) {
		case noop.MeterProvider:
			tracerFactory.enabled = false
			return tracerFactory, nil
		default:
			tracerFactory.enabled = false
			return tracerFactory, errors.New("unknown MetricsProvider type")
		}
	}

	// Create meter and instruments
	meter := meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	err = tracerFactory.createInstruments(meter)
	return tracerFactory, err
}

func builtInMeterProviderOptions(project string) ([]sdkmetric.Option, error) {
	defaultExporter, err := newMonitoringExporter(context.Background(), project, exporterOpts...)
	if err != nil {
		return nil, err
	}

	return []sdkmetric.Option{sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(
			defaultExporter,
			sdkmetric.WithInterval(defaultSamplePeriod),
		),
	)}, nil
}

func (tf *builtinMetricsTracerFactory) createInstruments(meter metric.Meter) error {
	var err error

	// Create operation_latencies
	tf.operationLatencies, err = meter.Float64Histogram(
		nativeMetricsPrefix+metricNameOperationLatencies,
		metric.WithDescription("Total time until final operation success or failure, including retries and backoff."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create attempt_latencies
	tf.attemptLatencies, err = meter.Float64Histogram(
		nativeMetricsPrefix+metricNameAttemptLatencies,
		metric.WithDescription("Client observed latency per RPC attempt."),
		metric.WithUnit(metricUnitMS),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create operation_count
	tf.operationCount, err = meter.Int64Counter(
		nativeMetricsPrefix+metricNameOperationCount,
		metric.WithDescription("The count of database operations."),
		metric.WithUnit(metricUnitCount),
	)

	// Create attempt_count
	tf.attemptCount, err = meter.Int64Counter(
		nativeMetricsPrefix+metricNameAttemptCount,
		metric.WithDescription("The number of additional RPCs sent after the initial attempt."),
		metric.WithUnit(metricUnitCount),
	)
	return err
}

// builtinMetricsTracer is created one per operation
// It is used to store metric instruments, attribute values
// and other data required to obtain and record them
type builtinMetricsTracer struct {
	ctx            context.Context
	builtInEnabled bool

	// attributes that are specific to a client instance and
	// do not change across different operations on client
	clientAttributes []attribute.KeyValue

	instrumentOperationLatencies metric.Float64Histogram
	instrumentAttemptLatencies   metric.Float64Histogram
	instrumentOperationCount     metric.Int64Counter
	instrumentAttemptCount       metric.Int64Counter

	method      string
	isStreaming bool

	currOp opTracer
}

// opTracer is used to record metrics for the entire operation, including retries.
// Operation is a logical unit that represents a single method invocation on client.
// The method might require multiple attempts/rpcs and backoff logic to complete
type opTracer struct {
	attemptCount int64

	startTime time.Time

	// gRPC status code of last completed attempt
	status string

	directPathEnabled bool

	currAttempt attemptTracer
}

func (o *opTracer) setStartTime(t time.Time) {
	o.startTime = t
}

func (o *opTracer) setStatus(status string) {
	o.status = status
}

func (o *opTracer) setDirectPathEnabled(directPathEnabled bool) {
	o.directPathEnabled = directPathEnabled
}

func (o *opTracer) incrementAttemptCount() {
	o.attemptCount++
}

// attemptTracer is used to record metrics for each individual attempt of the operation.
// Attempt corresponds to an attempt of an RPC.
type attemptTracer struct {
	startTime time.Time
	// gRPC status code
	status string

	directPathUsed bool
}

func (a *attemptTracer) setStartTime(t time.Time) {
	a.startTime = t
}

func (a *attemptTracer) setStatus(status string) {
	a.status = status
}

func (o *attemptTracer) setDirectPathUsed(directPathUsage bool) {
	o.directPathUsed = directPathUsage
}

func (tf *builtinMetricsTracerFactory) createBuiltinMetricsTracer(ctx context.Context, isStreaming bool) builtinMetricsTracer {
	// Operation has started but not the attempt.
	// So, create only operation tracer and not attempt tracer
	currOpTracer := opTracer{}
	currOpTracer.setStartTime(time.Now())
	currOpTracer.setDirectPathEnabled(tf.isDirectPathEnabled)

	return builtinMetricsTracer{
		ctx:            ctx,
		builtInEnabled: tf.enabled,

		currOp:           currOpTracer,
		clientAttributes: tf.clientAttributes,

		instrumentOperationLatencies: tf.operationLatencies,
		instrumentAttemptLatencies:   tf.attemptLatencies,
		instrumentOperationCount:     tf.operationCount,
		instrumentAttemptCount:       tf.attemptCount,

		isStreaming: isStreaming,
	}
}

// toOtelMetricAttrs:
// - converts metric attributes values captured throughout the operation / attempt
// to OpenTelemetry attributes format,
// - combines these with common client attributes and returns
func (mt *builtinMetricsTracer) toOtelMetricAttrs(metricName string) ([]attribute.KeyValue, error) {
	// Create attribute key value pairs for attributes common to all metricss
	attrKeyValues := []attribute.KeyValue{
		attribute.String(metricLabelKeyMethod, mt.method),
	}
	attrKeyValues = append(attrKeyValues, mt.clientAttributes...)

	// Get metric details
	mDetails, found := metricsDetails[metricName]
	if !found {
		return attrKeyValues, fmt.Errorf("unable to create attributes list for unknown metric: %v", metricName)
	}

	attrKeyValues = append(attrKeyValues, attribute.String(metricLabelKeyDirectPathEnabled, strconv.FormatBool(mt.currOp.directPathEnabled)))
	attrKeyValues = append(attrKeyValues, attribute.String(metricLabelKeyDirectPathUsed, strconv.FormatBool(mt.currOp.currAttempt.directPathUsed)))

	rpcStatus := mt.currOp.status
	if mDetails.recordedPerAttempt {
		rpcStatus = mt.currOp.currAttempt.status
	}

	// Add additional attributes to metrics
	for _, attrKey := range mDetails.additionalAttrs {
		switch attrKey {
		case metricLabelKeyStatus:
			attrKeyValues = append(attrKeyValues, attribute.String(metricLabelKeyStatus, rpcStatus))
		default:
			return attrKeyValues, fmt.Errorf("unknown additional attribute: %v", attrKey)
		}
	}

	return attrKeyValues, nil
}

// Convert error to grpc status error
func convertToGrpcStatusErr(err error) (codes.Code, error) {
	if err == nil {
		return codes.OK, nil
	}

	if errStatus, ok := status.FromError(err); ok {
		return errStatus.Code(), status.Error(errStatus.Code(), errStatus.Message())
	}

	ctxStatus := status.FromContextError(err)
	if ctxStatus.Code() != codes.Unknown {
		return ctxStatus.Code(), status.Error(ctxStatus.Code(), ctxStatus.Message())
	}

	return codes.Unknown, err
}

// recordAttemptCompletion records as many attempt specific metrics as it can
// Ignore errors seen while creating metric attributes since metric can still
// be recorded with rest of the attributes
func recordAttemptCompletion(mt *builtinMetricsTracer) {
	if !mt.builtInEnabled {
		return
	}

	// Calculate elapsed time
	elapsedTime := convertToMs(time.Since(mt.currOp.currAttempt.startTime))

	// Record attempt_latencies
	attemptLatAttrs, _ := mt.toOtelMetricAttrs(metricNameAttemptLatencies)
	mt.instrumentAttemptLatencies.Record(mt.ctx, elapsedTime, metric.WithAttributes(attemptLatAttrs...))
}

// recordOperationCompletion records as many operation specific metrics as it can
// Ignores error seen while creating metric attributes since metric can still
// be recorded with rest of the attributes
func recordOperationCompletion(mt *builtinMetricsTracer) {
	if !mt.builtInEnabled {
		return
	}

	// Calculate elapsed time
	elapsedTimeMs := convertToMs(time.Since(mt.currOp.startTime))

	// Record operation_count
	opCntAttrs, _ := mt.toOtelMetricAttrs(metricNameOperationCount)
	mt.instrumentOperationCount.Add(mt.ctx, 1, metric.WithAttributes(opCntAttrs...))

	// Record operation_latencies
	opLatAttrs, _ := mt.toOtelMetricAttrs(metricNameOperationLatencies)
	mt.instrumentOperationLatencies.Record(mt.ctx, elapsedTimeMs, metric.WithAttributes(opLatAttrs...))

	// Record attempt_count
	attemptCntAttrs, _ := mt.toOtelMetricAttrs(metricNameAttemptCount)
	if mt.currOp.attemptCount > 1 {
		// Only record when retry count is greater than 0 so the retry
		// graph will be less confusing
		mt.instrumentAttemptCount.Add(mt.ctx, mt.currOp.attemptCount-1, metric.WithAttributes(attemptCntAttrs...))
	}
}
func convertToMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1000000
}
