// Copyright 2026 Google LLC
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

package bigtable

import (
	"os"
	"testing"
)

func TestIsDirectAccessDisabled(t *testing.T) {
	tests := []struct {
		name          string
		configDisable bool
		envValue      string
		envSet        bool
		want          bool
	}{
		{
			name:          "config_disabled_env_unset",
			configDisable: true,
			envSet:        false,
			want:          true,
		},
		{
			name:          "config_disabled_env_false",
			configDisable: true,
			envSet:        true,
			envValue:      "false",
			want:          true,
		},
		{
			name:          "config_disabled_env_true",
			configDisable: true,
			envSet:        true,
			envValue:      "true",
			want:          true,
		},
		{
			name:          "config_enabled_env_unset",
			configDisable: false,
			envSet:        false,
			want:          false,
		},
		{
			name:          "config_enabled_env_false_lowercase",
			configDisable: false,
			envSet:        true,
			envValue:      "false",
			want:          true,
		},
		{
			name:          "config_enabled_env_false_uppercase",
			configDisable: false,
			envSet:        true,
			envValue:      "FALSE",
			want:          true,
		},
		{
			name:          "config_enabled_env_false_mixedcase",
			configDisable: false,
			envSet:        true,
			envValue:      "False",
			want:          true,
		},
		{
			name:          "config_enabled_env_true_lowercase",
			configDisable: false,
			envSet:        true,
			envValue:      "true",
			want:          false,
		},
		{
			name:          "config_enabled_env_true_uppercase",
			configDisable: false,
			envSet:        true,
			envValue:      "TRUE",
			want:          false,
		},
		{
			name:          "config_enabled_env_true_mixedcase",
			configDisable: false,
			envSet:        true,
			envValue:      "True",
			want:          false,
		},
		{
			// 't' is not respected, defaults to not disabled (false)
			name:          "config_enabled_env_t",
			configDisable: false,
			envSet:        true,
			envValue:      "t",
			want:          false,
		},
		{
			// 'f' is not respected, defaults to not disabled (false)
			name:          "config_enabled_env_f",
			configDisable: false,
			envSet:        true,
			envValue:      "f",
			want:          false,
		},
		{
			name:          "config_enabled_env_invalid",
			configDisable: false,
			envSet:        true,
			envValue:      "invalid",
			want:          false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envSet {
				os.Setenv("CBT_ENABLE_DIRECTPATH", tc.envValue)
				defer os.Unsetenv("CBT_ENABLE_DIRECTPATH")
			} else {
				os.Unsetenv("CBT_ENABLE_DIRECTPATH")
			}
			config := ClientConfig{
				DisableDirectAccess: tc.configDisable,
			}
			got := isDirectAccessDisabled(config)
			if got != tc.want {
				t.Errorf("isDirectAccessDisabled(%+v) = %v; want %v", config, got, tc.want)
			}
		})
	}
}
