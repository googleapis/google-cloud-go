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

package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
)

func TestDirectPathDiagnostic(t *testing.T) {
	// Start a mock metadata server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/computeMetadata/v1/instance/service-accounts/"+defaultKey+"/email" {
			w.Write([]byte("default-compute@developer.gserviceaccount.com"))
		} else if r.URL.Path == "/computeMetadata/v1/"+serviceAccountTokenKey {
			w.Write([]byte(`{"access_token": "mock-token", "expires_in": 3600, "token_type": "Bearer"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	os.Setenv("GCE_METADATA_HOST", ts.URL[7:])
	defer os.Unsetenv("GCE_METADATA_HOST")

	tests := []struct {
		name       string
		opts       []option.ClientOption
		disableEnv bool
		want       string
	}{
		{
			name:       "disabled via env var",
			opts:       []option.ClientOption{internaloption.EnableDirectPath(true)},
			disableEnv: true,
			want:       reasonEnvVarDisabled,
		},
		{
			name: "option disabled",
			opts: []option.ClientOption{internaloption.EnableDirectPath(false)},
			want: reasonOptionDisabled,
		},
		{
			name: "unsupported endpoint",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("https://storage.googleapis.com"),
			},
			want: reasonUnsupportedEndpoint,
		},
		{
			name: "xds not enabled",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
			},
			want: reasonXDSNotEnabled,
		},
		{
			name: "custom grpc conn",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithGRPCConn(&grpc.ClientConn{}),
			},
			want: reasonCustomGRPCConn,
		},
		{
			name: "custom http client",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithHTTPClient(&http.Client{}),
			},
			want: reasonCustomHTTPClient,
		},
		{
			name: "no auth",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithoutAuthentication(),
			},
			want: reasonNoAuth,
		},
		{
			name: "with api key",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithAPIKey("fake-api-key"),
			},
			want: reasonAPIKey,
		},
		{
			name: "undetermined",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
			},
			want: reasonUndetermined,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(directPathDisableEnvVar, "false") // ensure env var is not set unless specified in test case.
			if tc.disableEnv {
				os.Setenv(directPathDisableEnvVar, "true")
			}

			got := directPathDiagnostic(context.Background(), tc.opts...)
			if got != tc.want {
				t.Errorf("directPathDiagnostic() = %v; want %v", got, tc.want)
			}
		})
	}
}
