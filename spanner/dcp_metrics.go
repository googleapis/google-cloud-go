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
	"math"

	"cloud.google.com/go/spanner/internal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const dcpMetricsPrefix = metricsPrefix + "dynamic_channel_pool/"

var attributeKeyDCPDirection = attribute.Key("direction")

type dcpMetrics struct {
	attrs []attribute.KeyValue

	numChannels          metric.Int64ObservableGauge
	drainingChannelCount metric.Int64ObservableGauge
	maxAllowedChannels   metric.Int64ObservableGauge
	activeRPCCount       metric.Int64ObservableGauge
	maxRPCPerChannel     metric.Int64ObservableGauge
	channelPoolScaling   metric.Int64Counter
	registration         metric.Registration
}

func newDCPMetrics(p *dynamicChannelPool, mp metric.MeterProvider) *dcpMetrics {
	if !IsOpenTelemetryMetricsEnabled() {
		return nil
	}
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	_, instance, database, err := parseDatabaseName(p.sc.database)
	if err != nil {
		logf(p.sc.logger, "spanner_dcp: failed to parse database name for OpenTelemetry metrics: %v", err)
		return nil
	}
	m := &dcpMetrics{
		attrs: []attribute.KeyValue{
			attributeKeyClientID.String(p.sc.id),
			attributeKeyDatabase.String(database),
			attributeKeyInstance.String(instance),
			attributeKeyLibVersion.String(internal.Version),
		},
	}
	meter := mp.Meter(OtInstrumentationScope, metric.WithInstrumentationVersion(internal.Version))
	if m.numChannels, err = dcpInt64ObservableGauge(meter, p.sc.logger, "num_channels", "Number of active channels currently in the dynamic channel pool.", "{channel}"); err != nil {
		return nil
	}
	if m.drainingChannelCount, err = dcpInt64ObservableGauge(meter, p.sc.logger, "draining_channel_count", "Number of channels currently draining in the dynamic channel pool.", "{channel}"); err != nil {
		return nil
	}
	if m.maxAllowedChannels, err = dcpInt64ObservableGauge(meter, p.sc.logger, "max_allowed_channels", "Maximum number of channels allowed in the dynamic channel pool.", "{channel}"); err != nil {
		return nil
	}
	if m.activeRPCCount, err = dcpInt64ObservableGauge(meter, p.sc.logger, "active_rpc_count", "Number of RPCs currently active on the dynamic channel pool.", "{rpc}"); err != nil {
		return nil
	}
	if m.maxRPCPerChannel, err = dcpInt64ObservableGauge(meter, p.sc.logger, "max_rpc_per_channel", "Maximum number of active RPCs allowed per channel before dynamic channel pool scale-up.", "{rpc}"); err != nil {
		return nil
	}

	m.channelPoolScaling, err = meter.Int64Counter(
		dcpMetricsPrefix+"channel_pool_scaling",
		metric.WithDescription("Number of channels added or removed by dynamic channel pool scaling."),
		metric.WithUnit("{channel}"),
	)
	if err != nil {
		logf(p.sc.logger, "Error during registering instrument for metric %s, error: %v", dcpMetricsPrefix+"channel_pool_scaling", err)
		return nil
	}

	reg, err := meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			attrs := metric.WithAttributes(m.attrs...)
			o.ObserveInt64(m.numChannels, int64(p.Num()), attrs)
			o.ObserveInt64(m.drainingChannelCount, p.drainingCount.Load(), attrs)
			o.ObserveInt64(m.maxAllowedChannels, int64(p.cfg.DCPMaxChannels), attrs)
			o.ObserveInt64(m.activeRPCCount, int64(p.totalRPCLoad.Load()), attrs)
			o.ObserveInt64(m.maxRPCPerChannel, int64(math.Ceil(p.cfg.DCPMaxRPCPerChannel)), attrs)
			return nil
		},
		m.numChannels,
		m.drainingChannelCount,
		m.maxAllowedChannels,
		m.activeRPCCount,
		m.maxRPCPerChannel,
	)
	if err != nil {
		logf(p.sc.logger, "spanner_dcp: failed to register OpenTelemetry metric callback: %v", err)
		return m
	}
	m.registration = reg
	return m
}

func dcpInt64ObservableGauge(meter metric.Meter, logger *log.Logger, name, desc, unit string) (metric.Int64ObservableGauge, error) {
	fullName := dcpMetricsPrefix + name
	instrument, err := meter.Int64ObservableGauge(fullName, metric.WithDescription(desc), metric.WithUnit(unit))
	if err != nil {
		logf(logger, "Error during registering instrument for metric %s, error: %v", fullName, err)
	}
	return instrument, err
}

func (m *dcpMetrics) recordScaleUp(ctx context.Context, channels int64) {
	m.recordScaling(ctx, channels, "up")
}

func (m *dcpMetrics) recordScaleDown(ctx context.Context, channels int64) {
	m.recordScaling(ctx, channels, "down")
}

func (m *dcpMetrics) recordScaling(ctx context.Context, channels int64, direction string) {
	if m == nil || m.channelPoolScaling == nil || channels <= 0 {
		return
	}
	attrs := append([]attribute.KeyValue{}, m.attrs...)
	attrs = append(attrs, attributeKeyDCPDirection.String(direction))
	m.channelPoolScaling.Add(ctx, channels, metric.WithAttributes(attrs...))
}

func (m *dcpMetrics) close(logger *log.Logger) {
	if m == nil || m.registration == nil {
		return
	}
	if err := m.registration.Unregister(); err != nil {
		logf(logger, "Failed to unregister callback from the OpenTelemetry meter, error : %v", err)
	}
	m.registration = nil
}
