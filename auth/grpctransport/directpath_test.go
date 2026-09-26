// Copyright 2024 Google LLC
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

package grpctransport

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/compute/metadata"
)

func TestIsTokenProviderDirectPathCompatible(t *testing.T) {
	for _, tt := range []struct {
		name string
		tp   auth.TokenProvider
		opts *Options
		want bool
	}{
		{
			name: "empty TokenProvider",
			opts: &Options{},
		},
		{
			name: "err TokenProvider.Token",
			tp:   &errTP{},
			opts: &Options{},
			want: false,
		},
		{
			name: "EnableNonDefaultSAForDirectPath",
			tp:   &staticTP{tok: &auth.Token{Value: "fakeToken"}},
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableNonDefaultSAForDirectPath: true,
				},
			},
			want: true,
		},
		{
			name: "non-compute token source",
			tp:   &staticTP{tok: token(map[string]interface{}{"auth.google.tokenSource": "NOT-compute-metadata"})},
			opts: &Options{},
			want: false,
		},
		{
			name: "compute-metadata but non default SA",
			tp: &staticTP{
				tok: token(map[string]interface{}{
					"auth.google.tokenSource":    "compute-metadata",
					"auth.google.serviceAccount": "NON-default",
				}),
			},
			opts: &Options{},
			want: false,
		},
		{
			name: "non-default service account",
			tp:   &staticTP{tok: token(map[string]interface{}{"auth.google.serviceAccount": "NOT-default"})},
			opts: &Options{},
			want: false,
		},
		{
			name: "default service account on compute",
			tp: &staticTP{
				tok: token(map[string]interface{}{
					"auth.google.tokenSource":    "compute-metadata",
					"auth.google.serviceAccount": "default",
				}),
			},
			opts: &Options{},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTokenProviderDirectPathCompatible(tt.tp, tt.opts); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDirectPathBoundTokenEnabled(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts *InternalOptions
		want bool
	}{
		{
			name: "empty list",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{},
			},
		},
		{
			name: "nil list",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{},
			},
		},
		{
			name: "list does not contain ALTS",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"MTLS_S2A"},
			},
		},
		{
			name: "list only contains ALTS",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"ALTS"},
			},
			want: true,
		},
		{
			name: "list contains ALTS and others",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"ALTS", "MTLS_S2A"},
			},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDirectPathBoundTokenEnabled(tt.opts); got != tt.want {
				t.Fatalf("isDirectPathBoundTokenEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

type errTP struct {
}

func (tp *errTP) Token(context.Context) (*auth.Token, error) {
	return nil, errors.New("error fetching Token")
}

func token(metadata map[string]interface{}) *auth.Token {
	tok := &auth.Token{Value: "fakeToken"}
	tok.Metadata = metadata
	return tok
}

func TestLogDirectPathMisconfigDirectPathNotSet(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds, err := credentials.DetectDefault(opts.resolveDetectOptions())
	if err != nil {
		t.Fatalf("failed to create creds")
	}

	logDirectPathMisconfig(endpoint, creds, opts)

	wantedLog := "DirectPath is disabled. To enable, please set the EnableDirectPath option along with the EnableDirectPathXds option."
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigWrongCredential(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	opts := &Options{InternalOptions: &InternalOptions{
		EnableDirectPathXds: true,
		EnableDirectPath:    true,
	}}

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
		JSON:          []byte("test"),
	})
	logDirectPathMisconfig(endpoint, creds, opts)

	wantedLog := "DirectPath is disabled. Please make sure the token source is fetched from GCE metadata server and the default service account is used."
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigNotOnGCE(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPath = true
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"

	creds, err := credentials.DetectDefault(opts.resolveDetectOptions())
	if err != nil {
		t.Fatalf("failed to create creds")
	}

	logDirectPathMisconfig(endpoint, creds, opts)

	if !metadata.OnGCE() {
		wantedLog := "DirectPath is disabled. DirectPath is only available in a GCE environment."
		if !strings.Contains(logOutput.String(), wantedLog) {
			t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
		}
	}
}

func TestConfigureDirectPath_Interconnect(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	opts := &Options{
		InternalOptions: &InternalOptions{
			EnableDirectPath:                    true,
			EnableDirectPathXds:                 true,
			EnableDirectPathXdsOverInterconnect: true,
			AllowHardBoundTokens:                []string{"ALTS"},
		},
	}
	// Use errTP to also verify that configureDirectPath does not synchronously
	// fetch a token for ALTS hard-bound token probing when interconnect is enabled.
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &errTP{},
	})

	for _, tc := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "standard googleapis.com endpoint is rewritten to -direct.googleapis.com",
			endpoint: "storage.googleapis.com:443",
			want:     "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
		{
			name:     "pre-rewritten -direct.googleapis.com endpoint",
			endpoint: "storage-direct.googleapis.com:443",
			want:     "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grpcOpts, gotEndpoint, err := configureDirectPath(nil, opts, tc.endpoint, creds)
			if err != nil {
				t.Fatalf("configureDirectPath(%q) unexpected error: %v", tc.endpoint, err)
			}
			if gotEndpoint != tc.want {
				t.Errorf("configureDirectPath(%q) endpoint = %q, want %q", tc.endpoint, gotEndpoint, tc.want)
			}
			if len(grpcOpts) < 2 {
				t.Errorf("configureDirectPath(%q) len(grpcOpts) = %d, want >= 2 (credentials bundle + WithAuthority)", tc.endpoint, len(grpcOpts))
			}
		})
	}
}

func TestConfigureDirectPath_Interconnect_EnvOverride(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})

	t.Run("env true enables interconnect when option is false", func(t *testing.T) {
		t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "true")
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: false,
			},
		}
		grpcOpts, gotEndpoint, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		wantEndpoint := "google-c2p:///storage-direct.googleapis.com?force-xds"
		if gotEndpoint != wantEndpoint {
			t.Errorf("configureDirectPath() endpoint = %q, want %q", gotEndpoint, wantEndpoint)
		}
		if len(grpcOpts) < 2 {
			t.Errorf("configureDirectPath() len(grpcOpts) = %d, want >= 2 (credentials bundle + WithAuthority)", len(grpcOpts))
		}
	})

	t.Run("env false disables interconnect even when option is true", func(t *testing.T) {
		t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "false")
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		_, gotEndpoint, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		wantEndpoint := "storage.googleapis.com:443"
		if gotEndpoint != wantEndpoint {
			t.Errorf("configureDirectPath() fallback endpoint = %q, want %q", gotEndpoint, wantEndpoint)
		}
	})
}

func TestConfigureDirectPath_Interconnect_CustomURI(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	opts := &Options{
		InternalOptions: &InternalOptions{
			EnableDirectPath:                    true,
			EnableDirectPathXds:                 true,
			EnableDirectPathXdsOverInterconnect: true,
		},
	}
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "preserves google-c2p with force-xds",
			input: "google-c2p:///storage-direct.googleapis.com?force-xds",
			want:  "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
		{
			name:  "appends force-xds to google-c2p without query",
			input: "google-c2p:///storage-direct.googleapis.com",
			want:  "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
		{
			name:  "appends force-xds to google-c2p with existing query",
			input: "google-c2p:///storage-direct.googleapis.com?foo=bar",
			want:  "google-c2p:///storage-direct.googleapis.com?foo=bar&force-xds",
		},
		{
			name:  "rewrites google-c2p standard host to -direct and appends force-xds",
			input: "google-c2p:///storage.googleapis.com",
			want:  "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := configureDirectPath(nil, opts, tc.input, creds)
			if err != nil {
				t.Fatalf("configureDirectPath() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("configureDirectPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestConfigureDirectPath_Interconnect_FallbackPreservesEndpoint(t *testing.T) {
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})

	t.Run("EnableDirectPath false preserves original endpoint", func(t *testing.T) {
		t.Setenv(disableDirectPathEnvVar, "")
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    false,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		_, got, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		if got != "storage.googleapis.com:443" {
			t.Errorf("configureDirectPath() = %q, want %q", got, "storage.googleapis.com:443")
		}
	})

	t.Run("GOOGLE_CLOUD_DISABLE_DIRECT_PATH=true preserves original endpoint", func(t *testing.T) {
		t.Setenv(disableDirectPathEnvVar, "true")
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		_, got, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		if got != "storage.googleapis.com:443" {
			t.Errorf("configureDirectPath() = %q, want %q", got, "storage.googleapis.com:443")
		}
	})

	t.Run("non-GDU universe domain preserves original endpoint", func(t *testing.T) {
		t.Setenv(disableDirectPathEnvVar, "")
		opts := &Options{
			UniverseDomain: "apis-tpclp.goog",
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		_, got, err := configureDirectPath(nil, opts, "storage.apis-tpclp.goog:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		if got != "storage.apis-tpclp.goog:443" {
			t.Errorf("configureDirectPath() = %q, want %q", got, "storage.apis-tpclp.goog:443")
		}
	})

	t.Run("lookalike domain rejected for interconnect DirectPath", func(t *testing.T) {
		t.Setenv(disableDirectPathEnvVar, "")
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		_, got, err := configureDirectPath(nil, opts, "storage.googleapis.com.evil.com:443", creds)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		if got != "storage.googleapis.com.evil.com:443" {
			t.Errorf("configureDirectPath() = %q, want %q", got, "storage.googleapis.com.evil.com:443")
		}
	})
}

func TestLogDirectPathMisconfig_Interconnect(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	opts := &Options{
		InternalOptions: &InternalOptions{
			EnableDirectPath:                    true,
			EnableDirectPathXds:                 true,
			EnableDirectPathXdsOverInterconnect: true,
		},
	}
	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})

	logDirectPathMisconfig("storage-direct.googleapis.com:443", creds, opts)
	if logOutput.Len() > 0 {
		t.Errorf("expected no misconfig warning when Interconnect is enabled, got: %s", logOutput.String())
	}
}
