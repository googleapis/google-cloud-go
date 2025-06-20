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

package metric

import (
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Option is a wrapper over sdkmetric.Option
type Option struct {
	sdkmetric.Option
}

// OtelSdkMetricOptions returns the underlying sdkmetric.Option array
func OtelSdkMetricOptions(options []Option) []sdkmetric.Option {
	if options == nil {
		return nil
	}
	otelOpts := []sdkmetric.Option{}
	for _, opt := range options {

		otelOpts = append(otelOpts, OtelSdkMetricOption(opt))
	}
	return otelOpts
}

// OtelSdkMetricOption returns the underlying sdkmetric.Option
func OtelSdkMetricOption(option Option) sdkmetric.Option {
	return option.Option
}

// MeterProvider is a wrapper over sdkmetric.MeterProvider
type MeterProvider struct {
	sdkmetric.MeterProvider
}
