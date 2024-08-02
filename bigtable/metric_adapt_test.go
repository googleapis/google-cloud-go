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
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestOtelSdkMetricOptions(t *testing.T) {
	tests := []struct {
		name        string
		btOptions   []MetricOption
		wantOtelOpt []sdkmetric.Option
	}{
		{
			name:        "nil input",
			btOptions:   nil,
			wantOtelOpt: nil,
		},
		{
			name:        "empty input",
			btOptions:   []MetricOption{},
			wantOtelOpt: []sdkmetric.Option{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOtelOpt := OtelSdkMetricOptions(tt.btOptions)
			if len(gotOtelOpt) != len(tt.wantOtelOpt) {
				t.Errorf("OtelSdkMetricOptions() got %v options, want %v options", len(gotOtelOpt), len(tt.wantOtelOpt))
				return
			}
			for i := range gotOtelOpt {
				if got, want := gotOtelOpt[i], tt.wantOtelOpt[i]; got != want {
					t.Errorf("OtelSdkMetricOptions() got %v, want %v", got, want)
				}
			}
		})
	}
}
