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
	logger   *log.Logger
	meter    metric.Meter
	lanes    [2]string
	fallback *directPathFallback

	registration    metric.Registration
	channelLoad     metric.Float64Histogram
	channelLoadOpts [2][]metric.RecordOption
}

func newChannelPoolBuiltinMetrics(logger *log.Logger, cfg channelPoolBuiltinMetricsConfig, fallback *directPathFallback) *channelPoolBuiltinMetrics {
	if cfg.meterProvider == nil {
		return nil
	}
	m := &channelPoolBuiltinMetrics{
		logger:   logger,
		meter:    cfg.meterProvider.Meter(builtInMetricsMeterName, metric.WithInstrumentationVersion(internal.Version)),
		lanes:    [2]string{cfg.lane, laneCloudPath},
		fallback: fallback,
	}
	m.registerChannelLoad()
	return m
}

func (m *channelPoolBuiltinMetrics) laneIndex() int {
	if m.fallback != nil && m.fallback.current().fallback {
		return 1
	}
	return 0
}

func (m *channelPoolBuiltinMetrics) registerChannelCount(p *dynamicChannelPool) {
	if m == nil {
		return
	}
	name := nativeMetricsPrefix + metricNameChannelPoolChannelCount
	channelCount, err := m.meter.Int64ObservableUpDownCounter(
		name,
		metric.WithDescription("The number of channels in the pool, by state."),
		metric.WithUnit(metricUnitCount),
	)
	if err != nil {
		logf(m.logger, "spanner_dcp: failed to create built-in metric %s: %v", name, err)
		return
	}
	var activeAttrs, drainingAttrs [2]metric.ObserveOption
	for i, lane := range m.lanes {
		activeAttrs[i] = metric.WithAttributeSet(attribute.NewSet(
			attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
			attribute.String(metricLabelKeyState, channelStateActive),
			attribute.String(metricLabelKeyLane, lane),
		))
		drainingAttrs[i] = metric.WithAttributeSet(attribute.NewSet(
			attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
			attribute.String(metricLabelKeyState, channelStateDraining),
			attribute.String(metricLabelKeyLane, lane),
		))
	}
	reg, err := m.meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			i := m.laneIndex()
			o.ObserveInt64(channelCount, int64(p.Num()), activeAttrs[i])
			o.ObserveInt64(channelCount, p.DrainingCount.Load(), drainingAttrs[i])
			return nil
		},
		channelCount,
	)
	if err != nil {
		logf(m.logger, "spanner_dcp: failed to register built-in channel pool metric callback: %v", err)
		return
	}
	m.registration = reg
}

func (m *channelPoolBuiltinMetrics) registerChannelLoad() {
	name := nativeMetricsPrefix + metricNameChannelPoolChannelLoad
	channelLoad, err := m.meter.Float64Histogram(
		name,
		metric.WithDescription("The number of calls in flight on the channel that a call starts on, including that call."),
		metric.WithUnit(metricUnitCount),
		metric.WithExplicitBucketBoundaries(channelLoadBucketBounds...),
	)
	if err != nil {
		logf(m.logger, "spanner_dcp: failed to create built-in metric %s: %v", name, err)
		return
	}
	m.channelLoad = channelLoad
	for i, lane := range m.lanes {
		m.channelLoadOpts[i] = []metric.RecordOption{metric.WithAttributeSet(attribute.NewSet(
			attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
			attribute.String(metricLabelKeyLane, lane),
		))}
	}
}

func (m *channelPoolBuiltinMetrics) recordChannelLoad(ctx context.Context, load int32) {
	if m == nil || m.channelLoad == nil {
		return
	}
	m.channelLoad.Record(ctx, float64(load), m.channelLoadOpts[m.laneIndex()]...)
}

func (m *channelPoolBuiltinMetrics) close() {
	if m == nil || m.registration == nil {
		return
	}
	if err := m.registration.Unregister(); err != nil {
		logf(m.logger, "spanner_dcp: failed to unregister built-in channel pool metric callback: %v", err)
	}
	m.registration = nil
}
