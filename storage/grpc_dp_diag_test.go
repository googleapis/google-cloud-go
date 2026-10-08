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

	"cloud.google.com/go/storage/experimental"
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

func stringPtr(s string) *string {
	return &s
}

func TestDirectPathDiagnostic_Interconnect(t *testing.T) {
	origOnGCE := onGCE
	onGCE = func() bool { return false }
	t.Cleanup(func() { onGCE = origOnGCE })

	t.Setenv(directPathDisableEnvVar, "false")

	for _, tc := range []struct {
		name   string
		envVal *string
		opts   []option.ClientOption
		want   string
	}{
		{
			name: "interconnect enabled off-GCE with standard endpoint bypasses GCE check",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				experimental.WithDirectPathXdsOverInterconnect(),
				option.WithEndpoint("storage.googleapis.com:443"),
			},
			want: reasonUndetermined,
		},
		{
			name: "interconnect enabled off-GCE with google-c2p endpoint bypasses GCE check",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				experimental.WithDirectPathXdsOverInterconnect(),
				option.WithEndpoint("google-c2p:///storage-direct.googleapis.com?force-xds"),
			},
			want: reasonUndetermined,
		},
		{
			name:   "env var alone enables bypass off-GCE",
			envVal: stringPtr("true"),
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				option.WithEndpoint("storage.googleapis.com:443"),
			},
			want: reasonUndetermined,
		},
		{
			name: "endpoint with -direct. enables bypass off-GCE",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				option.WithEndpoint("storage-direct.googleapis.com:443"),
			},
			want: reasonUndetermined,
		},
		{
			name: "endpoint with force-xds enables bypass off-GCE",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				option.WithEndpoint("google-c2p:///storage.googleapis.com?force-xds"),
			},
			want: reasonUndetermined,
		},
		{
			name: "interconnect disabled off-GCE returns not_on_gce",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				option.WithEndpoint("storage.googleapis.com:443"),
			},
			want: reasonNotOnGCE,
		},
		{
			name: "interconnect enabled with unsupported scheme endpoint returns unsupported_endpoint",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				experimental.WithDirectPathXdsOverInterconnect(),
				option.WithEndpoint("https://storage.googleapis.com"),
			},
			want: reasonUnsupportedEndpoint,
		},
		{
			name: "interconnect enabled without authentication returns no_auth",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				experimental.WithDirectPathXdsOverInterconnect(),
				option.WithEndpoint("storage.googleapis.com:443"),
				option.WithoutAuthentication(),
			},
			want: reasonNoAuth,
		},
		{
			name: "interconnect enabled with api key returns api_key",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				internaloption.EnableDirectPathXds(),
				experimental.WithDirectPathXdsOverInterconnect(),
				option.WithEndpoint("storage.googleapis.com:443"),
				option.WithAPIKey("fake-api-key"),
			},
			want: reasonAPIKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envVal != nil {
				t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, *tc.envVal)
			} else {
				t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			}
			if got := directPathDiagnostic(context.Background(), tc.opts...); got != tc.want {
				t.Errorf("directPathDiagnostic() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInterconnectRequested(t *testing.T) {
	for _, tc := range []struct {
		name     string
		envVal   *string
		resOpt   bool
		endpoint string
		want     bool
	}{
		{name: "option true, env unset", envVal: nil, resOpt: true, endpoint: "storage.googleapis.com:443", want: true},
		{name: "option false, env unset", envVal: nil, resOpt: false, endpoint: "storage.googleapis.com:443", want: false},
		{name: "env true overrides option false", envVal: stringPtr("true"), resOpt: false, endpoint: "storage.googleapis.com:443", want: true},
		{name: "env 1 overrides option false", envVal: stringPtr("1"), resOpt: false, endpoint: "storage.googleapis.com:443", want: true},
		{name: "env false overrides option true", envVal: stringPtr("false"), resOpt: true, endpoint: "storage.googleapis.com:443", want: false},
		{name: "env 0 overrides option true", envVal: stringPtr("0"), resOpt: true, endpoint: "storage.googleapis.com:443", want: false},
		{name: "invalid env falls back to option true", envVal: stringPtr("invalid"), resOpt: true, endpoint: "storage.googleapis.com:443", want: true},
		{name: "empty env falls back to option true", envVal: stringPtr(""), resOpt: true, endpoint: "storage.googleapis.com:443", want: true},
		{name: "endpoint with directPathInterconnectInfix enables interconnect", envVal: nil, resOpt: false, endpoint: "storage-direct.googleapis.com:443", want: true},
		{name: "endpoint with force-xds enables interconnect", envVal: nil, resOpt: false, endpoint: "google-c2p:///storage.googleapis.com?force-xds", want: true},
		{name: "env false overrides endpoint infix", envVal: stringPtr("false"), resOpt: false, endpoint: "storage-direct.googleapis.com:443", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envVal != nil {
				t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, *tc.envVal)
			} else {
				origVal, had := os.LookupEnv(enableDirectPathXdsOverInterconnectEnvVar)
				os.Unsetenv(enableDirectPathXdsOverInterconnectEnvVar)
				t.Cleanup(func() {
					if had {
						os.Setenv(enableDirectPathXdsOverInterconnectEnvVar, origVal)
					} else {
						os.Unsetenv(enableDirectPathXdsOverInterconnectEnvVar)
					}
				})
			}

			var opts []option.ClientOption
			if tc.resOpt {
				opts = append(opts, internaloption.EnableDirectPathXdsOverInterconnect())
			}
			res, err := internaloption.NewUnsafeResolver(opts...)
			if err != nil {
				t.Fatalf("internaloption.NewUnsafeResolver() failed: %v", err)
			}

			if got := interconnectRequested(res, tc.endpoint); got != tc.want {
				t.Errorf("interconnectRequested() = %v, want %v", got, tc.want)
			}
		})
	}
}
