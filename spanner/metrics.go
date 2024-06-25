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

	"cloud.google.com/go/spanner/internal"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
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
	metricLabelKeyDatabase          = "database"
	metricLabelKeyClientUID         = "client_uid"
	metricLabelKeyClientName        = "client_name"
	metricLabelKeyMethod            = "method"
	metricLabelKeyOperationStatus   = "status"
	metricLabelKeyDirectPathEnabled = "directpath_enabled"
	metricLabelKeyDirectPathUsed    = "directpath_used"

	// Metric names
	metricNameOperationLatencies = "operation_latencies"
	metricNameAttemptLatencies   = "attempt_latencies"
	metricNameOperationCount     = "operation_count"
	metricNameAttemptCount       = "attempt_count"
)

var (
	// duration between two metric exports
	defaultSamplePeriod = 5 * time.Minute

	clientName = fmt.Sprintf("go-spanner v%v", internal.Version)

	builtInEnabledDefault = true

	// Generates unique client ID in the format go-<random UUID>@<>hostname-processID
	generateClientUID = func() (string, error) {
		hostname := "localhost"
		hostname, err := os.Hostname()
		if err != nil {
			return "", err
		}
		return "go-" + uuid.NewString() + "@" + hostname + "-" + strconv.Itoa(os.Getpid()), nil
	}
	exporterOpts = []option.ClientOption{}
)

// MetricsProvider is a wrapper for built in metrics meter provider
type MetricsProvider interface {
	isMetricsProvider()
}

// NoopMetricsProvider can be used to disable built in metrics
type NoopMetricsProvider struct{}

func (NoopMetricsProvider) isMetricsProvider() {}

// CustomOpenTelemetryMetricsProvider can be used to collect and export builtin metric with custom meter provider
type CustomOpenTelemetryMetricsProvider struct {
	MeterProvider *sdkmetric.MeterProvider
}

func (CustomOpenTelemetryMetricsProvider) isMetricsProvider() {}

// createBuiltInMeterProviderOptions returns meter provider options, shutdown function and error
func createBuiltInMeterProviderOptions(ctx context.Context, project string) (sdkmetric.Option, error) {
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

type builtinMetricsFactory struct {
	builtinEnabled bool

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

func newBuiltinMetricsTracerFactory(ctx context.Context, project, instance, instanceConfig string, metricsProvider MetricsProvider) (*builtinMetricsFactory, error) {
	clientUID, err := generateClientUID()
	if err != nil {
		log.Printf("built-in metrics: generateClientUID failed: %v. Using empty string in the %v metric atteribute", err, metricLabelKeyClientUID)
	}

	metricsFactory := &builtinMetricsFactory{
		builtinEnabled: false,
		clientAttributes: []attribute.KeyValue{
			attribute.String(monitoredResLabelKeyProject, project),
			attribute.String(monitoredResLabelKeyInstance, instance),
			attribute.String(monitoredResLabelKeyInstanceConfig, instanceConfig),
			attribute.String(metricLabelKeyClientUID, clientUID),
			attribute.String(metricLabelKeyClientName, clientName),
		},
		shutdown: func() {},
	}

	if emulatorAddr := os.Getenv("BIGTABLE_EMULATOR_HOST"); emulatorAddr != "" {
		// Do not emit metrics when emulator is being used
		return metricsFactory, nil
	}

	var meterProvider *sdkmetric.MeterProvider
	if metricsProvider == nil {
		// Create default meter provider
		mpOptions, err := createBuiltInMeterProviderOptions(ctx, project)
		if err != nil {
			return metricsFactory, err
		}
		meterProvider = sdkmetric.NewMeterProvider(mpOptions)

		metricsFactory.builtinEnabled = true
		metricsFactory.shutdown = func() { meterProvider.Shutdown(ctx) }
	} else {
		switch v := metricsProvider.(type) {
		case CustomOpenTelemetryMetricsProvider:
			// User provided meter provider
			metricsFactory.builtinEnabled = true
			meterProvider = v.MeterProvider
		case NoopMetricsProvider:
			metricsFactory.builtinEnabled = false
			return metricsFactory, nil
		default:
			metricsFactory.builtinEnabled = false
			return metricsFactory, errors.New("Unknown MetricsProvider type")
		}
	}

	// Create meter and instruments
	meter := meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	err = metricsFactory.createInstruments(meter)
	return metricsFactory, err
}

func (mf *builtinMetricsFactory) createInstruments(meter metric.Meter) error {
	var err error

	// Create operation_latencies
	mf.operationLatencies, err = meter.Float64Histogram(
		nativeMetricsPrefix+metricNameOperationLatencies,
		metric.WithDescription("Total time until final operation success or failure, including retries and backoff."),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create attempt_latencies
	mf.attemptLatencies, err = meter.Float64Histogram(
		nativeMetricsPrefix+metricNameAttemptLatencies,
		metric.WithDescription("Client observed latency per RPC attempt."),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(bucketBounds...),
	)
	if err != nil {
		return err
	}

	// Create operation_count
	mf.operationCount, err = meter.Int64Counter(
		nativeMetricsPrefix+metricNameOperationCount,
		metric.WithDescription("The number of RPC that represents a single method invocation. The method might require multiple attempts/rpcs and backoff logic to complete"),
	)

	// Create attempt_count
	mf.attemptCount, err = meter.Int64Counter(
		nativeMetricsPrefix+metricNameAttemptCount,
		metric.WithDescription("The number of additional RPCs sent after the initial attempt."),
	)
	return err
}
