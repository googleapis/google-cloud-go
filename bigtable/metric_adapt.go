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

package bigtable

import (
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type MetricOption struct {
	option sdkmetric.Option
}

func OtelSdkMetricOptions(btOptions []MetricOption) []sdkmetric.Option {
	if btOptions == nil {
		return nil
	}
	otelOpts := []sdkmetric.Option{}
	for _, btOpt := range btOptions {

		otelOpts = append(otelOpts, OtelSdkMetricOption(btOpt))
	}
	return otelOpts
}

func OtelSdkMetricOption(btOption MetricOption) sdkmetric.Option {
	return btOption.option
}
