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
	metricNameChannelPoolChannelLoad  = "channel_pool/channel_load"

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

var channelLoadBucketBounds = []float64{1, 2, 3, 5, 8, 12, 16, 20, 25, 30, 40, 50, 75, 100, 150, 200}

type channelPoolBuiltinMetricsConfig struct {
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

type channelPoolBuiltinMetrics struct {
	registration     metric.Registration
	channelLoad      metric.Float64Histogram
	channelLoadAttrs metric.RecordOption
}

func newChannelPoolBuiltinMetrics(p *dynamicChannelPool, cfg channelPoolBuiltinMetricsConfig) *channelPoolBuiltinMetrics {
	if cfg.meterProvider == nil {
		return nil
	}
	meter := cfg.meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version))
	m := &channelPoolBuiltinMetrics{}
	m.registerChannelCount(p, meter, cfg.lane)
	m.registerChannelLoad(p, meter, cfg.lane)
	return m
}

func (m *channelPoolBuiltinMetrics) registerChannelCount(p *dynamicChannelPool, meter metric.Meter, lane string) {
	name := nativeMetricsPrefix + metricNameChannelPoolChannelCount
	channelCount, err := meter.Int64ObservableUpDownCounter(
		name,
		metric.WithDescription("The number of channels in the pool, by state."),
		metric.WithUnit(metricUnitCount),
	)
	if err != nil {
		logf(p.sc.logger, "spanner_dcp: failed to create built-in metric %s: %v", name, err)
		return
	}
	activeAttrs := metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyState, channelStateActive),
		attribute.String(metricLabelKeyLane, lane),
	))
	drainingAttrs := metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyState, channelStateDraining),
		attribute.String(metricLabelKeyLane, lane),
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
		return
	}
	m.registration = reg
}

func (m *channelPoolBuiltinMetrics) registerChannelLoad(p *dynamicChannelPool, meter metric.Meter, lane string) {
	name := nativeMetricsPrefix + metricNameChannelPoolChannelLoad
	channelLoad, err := meter.Float64Histogram(
		name,
		metric.WithDescription("The number of calls in flight on the channel that a call starts on, including that call."),
		metric.WithUnit(metricUnitCount),
		metric.WithExplicitBucketBoundaries(channelLoadBucketBounds...),
	)
	if err != nil {
		logf(p.sc.logger, "spanner_dcp: failed to create built-in metric %s: %v", name, err)
		return
	}
	m.channelLoad = channelLoad
	m.channelLoadAttrs = metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyLane, lane),
	))
}

func (m *channelPoolBuiltinMetrics) recordChannelLoad(ctx context.Context, load int32) {
	if m == nil || m.channelLoad == nil {
		return
	}
	m.channelLoad.Record(ctx, float64(load), m.channelLoadAttrs)
}

func (m *channelPoolBuiltinMetrics) close(logger *log.Logger) {
	if m == nil || m.registration == nil {
		return
	}
	if err := m.registration.Unregister(); err != nil {
		logf(logger, "spanner_dcp: failed to unregister built-in channel pool metric callback: %v", err)
	}
	m.registration = nil
}
