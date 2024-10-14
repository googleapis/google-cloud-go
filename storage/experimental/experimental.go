// Copyright 2024 Google LLC
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

// Package experimental is a collection of experimental features that might
// have some rough edges to them. Housing experimental features in this package
// results in a user accessing these APIs as `experimental.Foo`, thereby making
// it explicit that the feature is experimental and using them in production
// code is at their own risk.
//
// All APIs in this package are experimental.
package experimental

import (
	"time"

	"cloud.google.com/go/storage/internal"
	"go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
)

// WithMetricInterval provides a [ClientOption] that may be passed to [storage.NewGrpcClient].
// It sets how often to emit metrics when using NewPeriodicReader and must be
// greater than 1 minute.
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#NewPeriodicReader
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#WithInterval
func WithMetricInterval(metricInterval time.Duration) option.ClientOption {
	return internal.WithMetricInterval.(func(time.Duration) option.ClientOption)(metricInterval)
}

// WithMetricExporter provides a [ClientOption] that may be passed to [storage.NewGrpcClient].
// Set an alternate client-side metric Exporter to emit metrics through.
// Must implement interface metric.Exporter:
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#Exporter
func WithMetricExporter(ex *metric.Exporter) option.ClientOption {
	return internal.WithMetricExporter.(func(*metric.Exporter) option.ClientOption)(ex)
}

// WithReadStallTimeout provides a [ClientOption] that may be passed to [storage.NewClient].
// It enables the client to retry stalled requests when starting a download from
// Cloud Storage. If the timeout elapses with no response from the server, the request
// is automatically retried.
// The timeout is initially set to ReadStallTimeoutConfig.Min. The client tracks
// latency across all read requests from the client, and can adjust the timeout higher
// to the target percentile when latency from the server is high.
// Currently, this is supported only for downloads ([storage.NewReader] and
// [storage.NewRangeReader] calls) and only for the XML API. Other read APIs (gRPC & JSON)
// will be supported soon.
func WithReadStallTimeout(rstc *ReadStallTimeoutConfig) option.ClientOption {
	// TODO (raj-prince): To keep separate dynamicDelay instance for different BucketHandle.
	// Currently, dynamicTimeout is kept at the client and hence shared across all the
	// BucketHandle, which is not the ideal state. As latency depends on location of VM
	// and Bucket, and read latency of different buckets may lie in different range.
	// Hence having a separate dynamicTimeout instance at BucketHandle level will
	// be better.
	return internal.WithReadStallTimeout.(func(config *ReadStallTimeoutConfig) option.ClientOption)(rstc)
}

// ReadStallTimeoutConfig defines the timeout which is adjusted dynamically based on
// past observed latencies.
type ReadStallTimeoutConfig struct {
	// Min is the minimum duration of the timeout. The default value is 500ms. Requests
	// taking shorter than this value to return response headers will never time out.
	// In general, you should choose a Min value that is greater than the typical value
	// for the target percentile.
	Min time.Duration

	// TargetPercentile is the percentile to target for the dynamic timeout. The default
	// value is 0.99. At the default percentile, at most 1% of requests will be timed out
	// and retried.
	TargetPercentile float64
}
