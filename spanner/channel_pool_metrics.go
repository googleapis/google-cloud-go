// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"context"
	"log"
	"os"
	"strconv"

	"cloud.google.com/go/spanner/internal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	metricNameChannelPoolChannelCount = "channel_pool/channel_count"

	metricLabelKeyPoolMode = "pool_mode"
	metricLabelKeyState    = "state"
	metricLabelKeyLane     = "lane"

	poolModeDynamic = "dynamic"

	channelStateActive   = "active"
	channelStateDraining = "draining"

	laneDirectPath = "directpath"
	laneCloudPath  = "cloudpath"

	disableChannelPoolBuiltinMetricsEnvVar = "SPANNER_DISABLE_CHANNEL_POOL_BUILTIN_METRICS"
)

// channelPoolBuiltinMetricsConfig holds the settings a channel pool needs to
// report its metrics to the built-in Cloud Monitoring pipeline.
type channelPoolBuiltinMetricsConfig struct {
	// meterProvider is the built-in metrics meter provider. A nil value
	// disables the channel pool built-in metrics.
	meterProvider metric.MeterProvider
	lane          string
}

func newChannelPoolBuiltinMetricsConfig(tf *builtinMetricsTracerFactory, isDirectPathEnabled bool) channelPoolBuiltinMetricsConfig {
	cfg := channelPoolBuiltinMetricsConfig{lane: laneCloudPath}
	if isDirectPathEnabled {
		cfg.lane = laneDirectPath
	}
	if tf == nil || tf.meterProvider == nil {
		return cfg
	}
	if disabled, _ := strconv.ParseBool(os.Getenv(disableChannelPoolBuiltinMetricsEnvVar)); disabled {
		return cfg
	}
	cfg.meterProvider = tf.meterProvider
	return cfg
}

// channelPoolBuiltinMetrics reports the channel pool metrics that are only
// sent to the built-in Cloud Monitoring pipeline.
type channelPoolBuiltinMetrics struct {
	registration metric.Registration
}

func newChannelPoolBuiltinMetrics(p *dynamicChannelPool, cfg channelPoolBuiltinMetricsConfig) *channelPoolBuiltinMetrics {
	if cfg.meterProvider == nil {
		return nil
	}
	meter := cfg.meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	name := nativeMetricsPrefix + metricNameChannelPoolChannelCount
	channelCount, err := meter.Int64ObservableUpDownCounter(
		name,
		metric.WithDescription("The number of channels in the pool, by state."),
		metric.WithUnit(metricUnitCount),
	)
	if err != nil {
		logf(p.sc.logger, "Error during registering instrument for metric %s, error: %v", name, err)
		return nil
	}
	activeAttrs := metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyState, channelStateActive),
		attribute.String(metricLabelKeyLane, cfg.lane),
	))
	drainingAttrs := metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyState, channelStateDraining),
		attribute.String(metricLabelKeyLane, cfg.lane),
	))
	reg, err := meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			o.ObserveInt64(channelCount, int64(p.Num()), activeAttrs)
			o.ObserveInt64(channelCount, p.drainingCount.Load(), drainingAttrs)
			return nil
		},
		channelCount,
	)
	if err != nil {
		logf(p.sc.logger, "spanner_dcp: failed to register built-in channel pool metric callback: %v", err)
		return nil
	}
	return &channelPoolBuiltinMetrics{registration: reg}
}

func (m *channelPoolBuiltinMetrics) close(logger *log.Logger) {
	if m == nil || m.registration == nil {
		return
	}
	if err := m.registration.Unregister(); err != nil {
		logf(logger, "Failed to unregister built-in channel pool metric callback, error: %v", err)
	}
	m.registration = nil
}
