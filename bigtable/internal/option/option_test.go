/*
Copyright 2026 Google LLC

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

package option

import (
	"testing"
)

func TestEnableBigtableConnectionPool(t *testing.T) {
	tests := []struct {
		desc         string
		emulatorHost string
		connPoolEnv  string
		want         bool
	}{
		{
			desc:         "emulator host set, should return false",
			emulatorHost: "localhost:8086",
			want:         false,
		},
		{
			desc:         "emulator host set, conn pool env true, should return false",
			emulatorHost: "localhost:8086",
			connPoolEnv:  "true",
			want:         false,
		},
		{
			desc:         "emulator host not set, conn pool env not set, should return true",
			emulatorHost: "",
			connPoolEnv:  "",
			want:         true,
		},
		{
			desc:         "emulator host not set, conn pool env true, should return true",
			emulatorHost: "",
			connPoolEnv:  "true",
			want:         true,
		},
		{
			desc:         "emulator host not set, conn pool env false, should return false",
			emulatorHost: "",
			connPoolEnv:  "false",
			want:         false,
		},
		{
			desc:         "emulator host not set, conn pool env invalid, should return true",
			emulatorHost: "",
			connPoolEnv:  "invalid",
			want:         true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.emulatorHost != "" {
				t.Setenv("BIGTABLE_EMULATOR_HOST", test.emulatorHost)
			} else {
				t.Setenv("BIGTABLE_EMULATOR_HOST", "")
			}

			if test.connPoolEnv != "" {
				t.Setenv(BigtableConnectionPoolEnvVar, test.connPoolEnv)
			} else {
				t.Setenv(BigtableConnectionPoolEnvVar, "")
			}

			got := EnableBigtableConnectionPool()
			if got != test.want {
				t.Errorf("EnableBigtableConnectionPool() = %v, want %v", got, test.want)
			}
		})
	}
}
