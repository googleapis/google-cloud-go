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

package transport

import (
	"testing"
)

const (
	testS2AAddr     = "testS2AAddress:port"
	testMTLSS2AAddr = "testMTLSS2AAddress:port"
)

func TestGetS2AAddress(t *testing.T) {
	testCases := []struct {
		name               string
		respFn             func() (string, error)
		wantErr            bool
		wantS2AAddress     string
		wantMTLSS2AAddress string
	}{
		{
			name:               "test valid config with plaintext S2A address",
			respFn:             validConfigResp,
			wantErr:            false,
			wantS2AAddress:     testS2AAddr,
			wantMTLSS2AAddress: "",
		},
		{
			name:               "test valid config with MTLS S2A address",
			respFn:             validConfigRespMTLSS2A,
			wantErr:            false,
			wantS2AAddress:     "",
			wantMTLSS2AAddress: testMTLSS2AAddr,
		},
		{
			name:               "test error when getting config",
			respFn:             errorConfigResp,
			wantErr:            true,
			wantS2AAddress:     "",
			wantMTLSS2AAddress: "",
		},
		{
			name:               "test invalid config",
			respFn:             invalidConfigResp,
			wantErr:            true,
			wantS2AAddress:     "",
			wantMTLSS2AAddress: "",
		},
		{
			name:               "test invalid JSON response",
			respFn:             invalidJSONResp,
			wantErr:            true,
			wantS2AAddress:     "",
			wantMTLSS2AAddress: "",
		},
	}

	defer setupTest(t)()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			httpGetMetadataMTLSConfig = tc.respFn
			mtlsConfiguration, err = queryConfig()
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("queryConfig() got error: %v, want error: %v", gotErr, tc.wantErr)
			}
			if want, got := tc.wantS2AAddress, GetS2AAddress(); got != want {
				t.Errorf("want S2A address [%s], got address [%s]", want, got)
			}
			if want, got := tc.wantMTLSS2AAddress, GetMTLSS2AAddress(); got != want {
				t.Errorf("want MTLS S2A address [%s], got address [%s]", want, got)
			}
		})
	}
}

func TestIsGoogleS2AEnabled(t *testing.T) {
	testCases := []struct {
		name      string
		useS2AEnv string
		want      bool
	}{
		{
			name:      "true",
			useS2AEnv: "true",
			want:      true,
		},
		{
			name:      "false",
			useS2AEnv: "false",
			want:      false,
		},
		{
			name:      "empty",
			useS2AEnv: "",
			want:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useS2AEnv != "" {
				t.Setenv(googleAPIUseS2AEnv, tc.useS2AEnv)
			}

			if got := isGoogleS2AEnabled(); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}
}
