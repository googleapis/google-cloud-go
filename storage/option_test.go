// Copyright 2023 Google LLC
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

package storage

import (
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
)

func TestApplyStorageOpt(t *testing.T) {
	for _, test := range []struct {
		desc string
		opts []option.ClientOption
		want storageConfig
	}{
		{
			desc: "set JSON option",
			opts: []option.ClientOption{WithJSONReads()},
			want: storageConfig{
				useJSONforReads:      true,
				readAPIWasSet:        true,
				disableClientMetrics: false,
			},
		},
		{
			desc: "set XML option",
			opts: []option.ClientOption{WithXMLReads()},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        true,
				disableClientMetrics: false,
			},
		},
		{
			desc: "set conflicting options, last option set takes precedence",
			opts: []option.ClientOption{WithJSONReads(), WithXMLReads()},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        true,
				disableClientMetrics: false,
			},
		},
		{
			desc: "empty options",
			opts: []option.ClientOption{},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        false,
				disableClientMetrics: false,
			},
		},
		{
			desc: "set Google API option",
			opts: []option.ClientOption{option.WithEndpoint("")},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        false,
				disableClientMetrics: false,
			},
		},
		{
			desc: "disable metrics option",
			opts: []option.ClientOption{WithDisabledClientMetrics()},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        false,
				disableClientMetrics: true,
			},
		},
		{
			desc: "set dynamic read req stall timeout option",
			opts: []option.ClientOption{withReadStallTimeout(&experimental.ReadStallTimeoutConfig{
				TargetPercentile: 0.99,
				Min:              time.Second,
			})},
			want: storageConfig{
				useJSONforReads:      false,
				readAPIWasSet:        false,
				disableClientMetrics: false,
				ReadStallTimeoutConfig: &experimental.ReadStallTimeoutConfig{
					TargetPercentile: 0.99,
					Min:              time.Second,
				},
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var got storageConfig
			for _, opt := range test.opts {
				if storageOpt, ok := opt.(storageClientOption); ok {
					storageOpt.ApplyStorageOpt(&got)
				}
			}
			if !cmp.Equal(got, test.want, cmp.AllowUnexported(storageConfig{}, experimental.ReadStallTimeoutConfig{})) {
				t.Errorf(cmp.Diff(got, test.want, cmp.AllowUnexported(storageConfig{}, experimental.ReadStallTimeoutConfig{})))
			}
		})
	}
}

func TestGetDynamicReadReqInitialTimeoutSecFromEnv(t *testing.T) {
	defaultValue := 10 * time.Second

	tests := []struct {
		name     string
		envValue string
		want     time.Duration
	}{
		{"env variable not set", "", 10 * time.Second},
		{"valid duration string", "5s", 5 * time.Second},
		{"invalid duration string", "invalid", 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(dynamicReadReqInitialTimeoutEnv, tt.envValue)
			if got := getDynamicReadReqInitialTimeoutSecFromEnv(defaultValue); got != tt.want {
				t.Errorf("getDynamicReadReqInitialTimeoutSecFromEnv(defaultValue) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDynamicReadReqIncreaseRateFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     float64
	}{
		{"env variable not set", "", defaultDynamicReadReqIncreaseRate},
		{"valid float string", "1.5", 1.5},
		{"invalid float string", "abc", defaultDynamicReadReqIncreaseRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(dynamicReadReqIncreaseRateEnv, tt.envValue)
			if got := getDynamicReadReqIncreaseRateFromEnv(); got != tt.want {
				t.Errorf("getDynamicReadReqIncreaseRateFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
