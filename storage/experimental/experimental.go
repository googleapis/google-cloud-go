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

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

type StorageExperimentalConfig struct {
	MetricExporterOptions []mexporter.Option
	MetricExporter        *metric.Exporter
	MetricInterval        time.Duration
}

func NewStorageExperimentalConfig(opts ...option.ClientOption) StorageExperimentalConfig {
	var conf StorageExperimentalConfig
	for _, opt := range opts {
		if storageOpt, ok := opt.(storageExperimentalClientOption); ok {
			storageOpt.ApplyStorageOpt(&conf)
		}
	}
	return conf
}

type storageExperimentalClientOption interface {
	option.ClientOption
	ApplyStorageOpt(*StorageExperimentalConfig)
}

type withMeterOptions struct {
	internaloption.EmbeddableAdapter
	// set sampling interval
	interval time.Duration
}

// Configure how often to emit metrics when using NewPeriodicReader
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#NewPeriodicReader
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#WithInterval
func WithMetricInterval(interval time.Duration) option.ClientOption {
	return &withMeterOptions{interval: interval}
}

func (w *withMeterOptions) ApplyStorageOpt(c *StorageExperimentalConfig) {
	c.MetricInterval = w.interval
}

type withMetricExporterConfig struct {
	internaloption.EmbeddableAdapter
	// client options for exporter
	exporterOptions []mexporter.Option
	// exporter override
	metricExporter *metric.Exporter
}

// Configure Google Cloud Monitoring Exporter options such as Project,
// Credentials and Sampling Rate.
// Only WithMetricOptions or WithMetricExporter option can be used at a time.
func WithMetricOptions(opts []mexporter.Option) option.ClientOption {
	return &withMetricExporterConfig{exporterOptions: opts}
}

// Configure alternate client-side metric Open Telemetry exporter
// to emit metrics through.
// Exporter must implement interface metric.Exporter:
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#Exporter
//
// Only WithMetricOptions or WithMetricExporter option can be used at a time.
func WithMetricExporter(ex *metric.Exporter) option.ClientOption {
	return &withMetricExporterConfig{metricExporter: ex}
}

func (w *withMetricExporterConfig) ApplyStorageOpt(c *StorageExperimentalConfig) {
	c.MetricExporterOptions = w.exporterOptions
	c.MetricExporter = w.metricExporter
}
