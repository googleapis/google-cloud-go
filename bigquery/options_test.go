// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
)

func TestCustomClientOptions(t *testing.T) {
	testCases := []struct {
		desc    string
		options []option.ClientOption
		want    *customClientConfig
	}{
		{
			desc: "no options",
			want: &customClientConfig{
				jobCreationMode: "",
			},
		},
		{
			desc: "jobmode required",
			options: []option.ClientOption{
				WithDefaultJobCreationMode(JobCreationModeRequired),
			},
			want: &customClientConfig{
				jobCreationMode: JobCreationModeRequired,
			},
		},
		{
			desc: "jobmode optional",
			options: []option.ClientOption{
				WithDefaultJobCreationMode(JobCreationModeOptional),
			},
			want: &customClientConfig{
				jobCreationMode: JobCreationModeOptional,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gotCfg := newCustomClientConfig(tc.options...)
			if diff := cmp.Diff(gotCfg, tc.want, cmp.AllowUnexported(customClientConfig{})); diff != "" {
				t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
			}
		})
	}
}
