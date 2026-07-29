/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// fixedAttributeMeterProvider adds client-scoped attributes to the counter
// instrument kinds used by grpcMetricsToEnable. Spanner passes this provider
// only to gRPC's built-in metrics plugin.
type fixedAttributeMeterProvider struct {
	metric.MeterProvider
	attributes metric.MeasurementOption
}

func newFixedAttributeMeterProvider(provider metric.MeterProvider, attributes []attribute.KeyValue) metric.MeterProvider {
	return &fixedAttributeMeterProvider{
		MeterProvider: provider,
		attributes:    metric.WithAttributeSet(attribute.NewSet(attributes...)),
	}
}

func (p *fixedAttributeMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return &fixedAttributeMeter{
		Meter:      p.MeterProvider.Meter(name, opts...),
		attributes: p.attributes,
	}
}

type fixedAttributeMeter struct {
	metric.Meter
	attributes metric.MeasurementOption
}

func (m *fixedAttributeMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	counter, err := m.Meter.Int64Counter(name, opts...)
	if err != nil {
		return nil, err
	}
	return &fixedAttributeInt64Counter{
		Int64Counter: counter,
		attributes:   m.attributes,
	}, nil
}

func (m *fixedAttributeMeter) Int64UpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	counter, err := m.Meter.Int64UpDownCounter(name, opts...)
	if err != nil {
		return nil, err
	}
	return &fixedAttributeInt64UpDownCounter{
		Int64UpDownCounter: counter,
		attributes:         m.attributes,
	}, nil
}

type fixedAttributeInt64Counter struct {
	metric.Int64Counter
	attributes metric.AddOption
}

func (c *fixedAttributeInt64Counter) Add(ctx context.Context, value int64, opts ...metric.AddOption) {
	c.Int64Counter.Add(ctx, value, append(opts, c.attributes)...)
}

type fixedAttributeInt64UpDownCounter struct {
	metric.Int64UpDownCounter
	attributes metric.AddOption
}

func (c *fixedAttributeInt64UpDownCounter) Add(ctx context.Context, value int64, opts ...metric.AddOption) {
	c.Int64UpDownCounter.Add(ctx, value, append(opts, c.attributes)...)
}
