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

// All options in this package are experimental.

package experimental

import (
	"time"

	"cloud.google.com/go/storage/internal"
	"go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
)

// Configure how often to emit metrics when using NewPeriodicReader
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#NewPeriodicReader
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#WithInterval
func WithMetricInterval(metricInterval time.Duration) option.ClientOption {
	return internal.WithMetricInterval.(func(time.Duration) option.ClientOption)(metricInterval)
}

// Configure alternate client-side metric Open Telemetry exporter
// to emit metrics through.
// Exporter must implement interface metric.Exporter:
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#Exporter
//
// Only WithMetricOptions or WithMetricExporter option can be used at a time.
func WithMetricExporter(ex *metric.Exporter) option.ClientOption {
	return internal.WithMetricInterval.(func(*metric.Exporter) option.ClientOption)(ex)
}
