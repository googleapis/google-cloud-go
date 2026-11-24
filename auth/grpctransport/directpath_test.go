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
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/compute/metadata"
	grpccreds "google.golang.org/grpc/credentials"
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
		{
			name:     "preprod sandbox endpoint is rewritten after its first label",
			endpoint: "storage-preprod.sandbox.googleapis.com:443",
			want:     "google-c2p:///storage-preprod-direct.sandbox.googleapis.com?force-xds",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grpcOpts, gotEndpoint, err := configureDirectPath(nil, opts, tc.endpoint, creds, nil)
			if err != nil {
				t.Fatalf("configureDirectPath(%q) unexpected error: %v", tc.endpoint, err)
			}
			if gotEndpoint != tc.want {
				t.Errorf("configureDirectPath(%q) endpoint = %q, want %q", tc.endpoint, gotEndpoint, tc.want)
			}
			if len(grpcOpts) < 1 {
				t.Errorf("configureDirectPath(%q) len(grpcOpts) = %d, want >= 1 (credentials bundle)", tc.endpoint, len(grpcOpts))
			}
		})
	}
}

// spyTP records whether Token was called. It returns tok, or an error if tok is
// nil.
type spyTP struct {
	tok    *auth.Token
	called bool
}

func (tp *spyTP) Token(context.Context) (*auth.Token, error) {
	tp.called = true
	if tp.tok == nil {
		return nil, errors.New("no token configured")
	}
	return tp.tok, nil
}

// tlsCredsType is the dynamic type of plain TLS transport credentials. The
// Google Default transport credentials also report "tls" from Info() while
// selecting ALTS per cluster during the handshake, so tests compare types.
var tlsCredsType = reflect.TypeOf(grpccreds.NewTLS(nil))

// TestDirectPathCredentialsBundle verifies the credentials bundle selected for
// a DirectPath channel. Over Interconnect the bundle uses plain TLS transport
// credentials and the given per-RPC credentials, without probing the token
// source for ALTS hard-bound tokens. Otherwise the Google Default bundle is
// used, adding ALTS hard-bound per-RPC credentials when the token comes from
// the GCE metadata server's default service account.
func TestDirectPathCredentialsBundle(t *testing.T) {
	perRPCCreds := &grpcCredentialsProvider{endpoint: "storage.googleapis.com:443"}
	computeToken := token(map[string]interface{}{
		"auth.google.tokenSource":    "compute-metadata",
		"auth.google.serviceAccount": "default",
	})
	for _, tc := range []struct {
		name         string
		interconnect bool
		tok          *auth.Token
		// wantTokenCalled is whether the token source is probed to decide on
		// ALTS hard-bound tokens.
		wantTokenCalled bool
		// wantALTSPerRPCCreds is whether gRPC wraps perRPCCreds together with
		// the ALTS hard-bound per-RPC credentials.
		wantALTSPerRPCCreds bool
	}{
		{
			name:            "interconnect uses TLS without probing the GCE token source",
			interconnect:    true,
			tok:             computeToken,
			wantTokenCalled: false,
		},
		{
			name:            "directpath without a GCE token omits ALTS hard-bound credentials",
			interconnect:    false,
			wantTokenCalled: true,
		},
		{
			name:                "directpath with a GCE token adds ALTS hard-bound credentials",
			interconnect:        false,
			tok:                 computeToken,
			wantTokenCalled:     true,
			wantALTSPerRPCCreds: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tp := &spyTP{tok: tc.tok}
			creds := auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: tp})
			opts := &Options{
				// Resolves the ALTS hard-bound credentials from a local file,
				// so the test does not reach the metadata server.
				DetectOpts: &credentials.DetectOptions{
					CredentialsFile: "../internal/testdata/sa.json",
				},
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: tc.interconnect,
					AllowHardBoundTokens:                []string{"ALTS"},
				},
			}
			bundle, err := directPathCredentialsBundle(tc.interconnect, opts, "storage.googleapis.com:443", creds, perRPCCreds)
			if err != nil {
				t.Fatalf("directPathCredentialsBundle(interconnect=%v) unexpected error: %v", tc.interconnect, err)
			}
			if tp.called != tc.wantTokenCalled {
				t.Errorf("directPathCredentialsBundle(interconnect=%v) called Token() = %v, want %v", tc.interconnect, tp.called, tc.wantTokenCalled)
			}
			if _, ok := bundle.(*interconnectCredentials); ok != tc.interconnect {
				t.Errorf("directPathCredentialsBundle(interconnect=%v) = %T, want *interconnectCredentials=%v", tc.interconnect, bundle, tc.interconnect)
			}
			if got := reflect.TypeOf(bundle.TransportCredentials()); (got == tlsCredsType) != tc.interconnect {
				t.Errorf("directPathCredentialsBundle(interconnect=%v).TransportCredentials() type = %v, want plain TLS (%v)=%v", tc.interconnect, got, tlsCredsType, tc.interconnect)
			}
			gotPerRPC := bundle.PerRPCCredentials()
			if gotALTS := gotPerRPC != grpccreds.PerRPCCredentials(perRPCCreds); gotALTS != tc.wantALTSPerRPCCreds {
				t.Errorf("directPathCredentialsBundle(interconnect=%v).PerRPCCredentials() = %T, want wrapped with ALTS hard-bound credentials=%v", tc.interconnect, gotPerRPC, tc.wantALTSPerRPCCreds)
			}
		})
	}
}

// TestInterconnectCredentials_NewWithMode verifies that every bundle mode keeps
// TLS transport credentials and the same per-RPC credentials. gRPC's RLS
// control channel requests fallback mode and relies on the per-RPC credentials
// attached to the bundle.
func TestInterconnectCredentials_NewWithMode(t *testing.T) {
	perRPCCreds := &grpcCredentialsProvider{endpoint: "storage.googleapis.com:443"}
	bundle := newInterconnectCredentials(perRPCCreds)
	for _, mode := range []string{"fallback", "balancer", "backend-from-balancer", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			got, err := bundle.NewWithMode(mode)
			if err != nil {
				t.Fatalf("NewWithMode(%q) unexpected error: %v", mode, err)
			}
			if got == grpccreds.Bundle(bundle) {
				t.Fatalf("NewWithMode(%q) returned the receiver, want a distinct copy", mode)
			}
			if gotType := reflect.TypeOf(got.TransportCredentials()); gotType != tlsCredsType {
				t.Errorf("NewWithMode(%q).TransportCredentials() type = %v, want %v", mode, gotType, tlsCredsType)
			}
			if got.TransportCredentials() == bundle.TransportCredentials() {
				t.Errorf("NewWithMode(%q) shares TransportCredentials with the receiver, want a clone", mode)
			}
			if got.PerRPCCredentials() != grpccreds.PerRPCCredentials(perRPCCreds) {
				t.Errorf("NewWithMode(%q).PerRPCCredentials() = %v, want %v", mode, got.PerRPCCredentials(), perRPCCreds)
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
		grpcOpts, gotEndpoint, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds, nil)
		if err != nil {
			t.Fatalf("configureDirectPath() unexpected error: %v", err)
		}
		wantEndpoint := "google-c2p:///storage-direct.googleapis.com?force-xds"
		if gotEndpoint != wantEndpoint {
			t.Errorf("configureDirectPath() endpoint = %q, want %q", gotEndpoint, wantEndpoint)
		}
		if len(grpcOpts) < 1 {
			t.Errorf("configureDirectPath() len(grpcOpts) = %d, want >= 1 (credentials bundle)", len(grpcOpts))
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
		_, gotEndpoint, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds, nil)
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
			_, got, err := configureDirectPath(nil, opts, tc.input, creds, nil)
			if err != nil {
				t.Fatalf("configureDirectPath() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("configureDirectPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestInterconnectHostAndAuthority(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		host                  string
		wantTargetHost        string
		wantAuthorityOverride string
	}{
		{
			name:           "storage host is rewritten and keeps derived authority",
			host:           "storage.googleapis.com",
			wantTargetHost: "storage-direct.googleapis.com",
		},
		{
			name:           "storage-direct host keeps derived authority",
			host:           "storage-direct.googleapis.com",
			wantTargetHost: "storage-direct.googleapis.com",
		},
		{
			name:                  "other service host is rewritten and overridden",
			host:                  "spanner.googleapis.com",
			wantTargetHost:        "spanner-direct.googleapis.com",
			wantAuthorityOverride: "spanner.googleapis.com",
		},
		{
			name:                  "other service -direct host is overridden",
			host:                  "spanner-direct.googleapis.com",
			wantTargetHost:        "spanner-direct.googleapis.com",
			wantAuthorityOverride: "spanner.googleapis.com",
		},
		{
			name:           "preprod storage host is rewritten and keeps derived authority",
			host:           "storage-preprod.sandbox.googleapis.com",
			wantTargetHost: "storage-preprod-direct.sandbox.googleapis.com",
		},
		{
			name:           "preprod storage-direct host keeps derived authority",
			host:           "storage-preprod-direct.sandbox.googleapis.com",
			wantTargetHost: "storage-preprod-direct.sandbox.googleapis.com",
		},
		{
			// -direct is inserted after the first label, mirroring how the
			// override removes it.
			name:                  "other multi-label host is rewritten after its first label and overridden",
			host:                  "foo.sandbox.googleapis.com",
			wantTargetHost:        "foo-direct.sandbox.googleapis.com",
			wantAuthorityOverride: "foo.sandbox.googleapis.com",
		},
		{
			name:           "non-googleapis host is unchanged",
			host:           "localhost",
			wantTargetHost: "localhost",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotTargetHost, gotAuthorityOverride := interconnectHostAndAuthority(tc.host)
			if gotTargetHost != tc.wantTargetHost || gotAuthorityOverride != tc.wantAuthorityOverride {
				t.Errorf("interconnectHostAndAuthority(%q) = (%q, %q), want (%q, %q)", tc.host, gotTargetHost, gotAuthorityOverride, tc.wantTargetHost, tc.wantAuthorityOverride)
			}
		})
	}
}

// TestConfigureDirectPath_Interconnect_Authority verifies that channels to the
// Cloud Storage Interconnect hosts keep the :authority derived from the target,
// while other hosts get a WithAuthority override.
//
// grpc.DialOption values are opaque, so the test detects the override by
// comparing the number of dial options against a host known to receive one.
func TestConfigureDirectPath_Interconnect_Authority(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})
	interconnectOpts := &Options{
		InternalOptions: &InternalOptions{
			EnableDirectPath:                    true,
			EnableDirectPathXds:                 true,
			EnableDirectPathXdsOverInterconnect: true,
		},
	}
	nonInterconnectOpts := &Options{
		InternalOptions: &InternalOptions{
			EnableDirectPath:    true,
			EnableDirectPathXds: true,
		},
	}
	numDialOptions := func(t *testing.T, opts *Options, endpoint string) int {
		t.Helper()
		grpcOpts, _, err := configureDirectPath(nil, opts, endpoint, creds, nil)
		if err != nil {
			t.Fatalf("configureDirectPath(%q) unexpected error: %v", endpoint, err)
		}
		return len(grpcOpts)
	}
	// t.Setenv cannot unset a variable; an empty value fails strconv.ParseBool
	// and falls back to the option and endpoint, matching an unset variable.
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	// Other services receive the WithAuthority override, establishing the baseline
	// dial option count when an override is present.
	withAuthorityOverride := numDialOptions(t, interconnectOpts, "pubsub.googleapis.com:443")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "true")
	if got := numDialOptions(t, nonInterconnectOpts, "spanner.googleapis.com:443"); got != withAuthorityOverride {
		t.Fatalf("configureDirectPath(%q) len(grpcOpts) = %d, want %d", "spanner.googleapis.com:443", got, withAuthorityOverride)
	}

	for _, tc := range []struct {
		name                  string
		env                   string
		opts                  *Options
		endpoint              string
		wantAuthorityOverride bool
	}{
		{
			name:     "option enables interconnect for default endpoint",
			opts:     interconnectOpts,
			endpoint: "storage.googleapis.com:443",
		},
		{
			name:     "env true enables interconnect for default endpoint",
			env:      "true",
			opts:     nonInterconnectOpts,
			endpoint: "storage.googleapis.com:443",
		},
		{
			name:     "invalid env falls back to option",
			env:      "yes",
			opts:     interconnectOpts,
			endpoint: "storage.googleapis.com:443",
		},
		{
			name:     "application-supplied storage-direct endpoint",
			opts:     nonInterconnectOpts,
			endpoint: "storage-direct.googleapis.com:443",
		},
		{
			name:     "application-supplied dns scheme storage-direct endpoint",
			opts:     nonInterconnectOpts,
			endpoint: "dns:///storage-direct.googleapis.com:443",
		},
		{
			name:     "application-supplied google-c2p storage-direct target",
			opts:     nonInterconnectOpts,
			endpoint: "google-c2p:///storage-direct.googleapis.com?force-xds",
		},
		{
			name:     "application-supplied google-c2p storage-direct target with other query",
			opts:     interconnectOpts,
			endpoint: "google-c2p:///storage-direct.googleapis.com?foo=bar",
		},
		{
			// force-xds enables interconnect and rewrites the host to
			// storage-direct.googleapis.com, so no authority override is added.
			name:     "application-supplied force-xds target for storage.googleapis.com",
			opts:     nonInterconnectOpts,
			endpoint: "google-c2p:///storage.googleapis.com?force-xds",
		},
		{
			name:                  "other service keeps authority override",
			opts:                  interconnectOpts,
			endpoint:              "spanner.googleapis.com:443",
			wantAuthorityOverride: true,
		},
		{
			name:                  "other service -direct endpoint keeps authority override",
			opts:                  nonInterconnectOpts,
			endpoint:              "spanner-direct.googleapis.com:443",
			wantAuthorityOverride: true,
		},
		{
			name:     "application-supplied preprod storage-direct endpoint",
			opts:     nonInterconnectOpts,
			endpoint: "storage-preprod-direct.sandbox.googleapis.com:443",
		},
		{
			name:     "option enables interconnect for preprod endpoint",
			opts:     interconnectOpts,
			endpoint: "storage-preprod.sandbox.googleapis.com:443",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, tc.env)
			want := withAuthorityOverride - 1
			if tc.wantAuthorityOverride {
				want = withAuthorityOverride
			}
			if got := numDialOptions(t, tc.opts, tc.endpoint); got != want {
				t.Errorf("configureDirectPath(%q) len(grpcOpts) = %d, want %d (wantAuthorityOverride=%v)", tc.endpoint, got, want, tc.wantAuthorityOverride)
			}
		})
	}
}

func TestConfigureDirectPath_Interconnect_FallbackPreservesEndpoint(t *testing.T) {
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})

	for _, tc := range []struct {
		name            string
		disableEnv      string
		interconnectEnv string
		opts            *Options
		endpoint        string
	}{
		{
			name: "EnableDirectPath false preserves original endpoint",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    false,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "storage.googleapis.com:443",
		},
		{
			name:       "GOOGLE_CLOUD_DISABLE_DIRECT_PATH=true preserves original endpoint",
			disableEnv: "true",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "storage.googleapis.com:443",
		},
		{
			name: "non-GDU universe domain preserves original endpoint",
			opts: &Options{
				UniverseDomain: "apis-tpclp.goog",
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "storage.apis-tpclp.goog:443",
		},
		{
			name: "lookalike domain rejected for interconnect DirectPath",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "storage.googleapis.com.evil.com:443",
		},
		{
			// The env var disables interconnect even for an application-supplied
			// storage-direct endpoint. Regular DirectPath then requires a GCE
			// metadata-server token, which staticTP is not, so the caller's dial
			// options and endpoint are returned unchanged and no authority
			// override is added either.
			name:            "env false preserves original storage-direct endpoint",
			interconnectEnv: "false",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			endpoint: "storage-direct.googleapis.com:443",
		},
		{
			// Only google-c2p:/// and dns:/// targets qualify for DirectPath, so
			// the experimental scheme is passed through without an authority
			// override.
			name: "google-c2p-experimental scheme preserves original endpoint",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "google-c2p-experimental:///storage-direct.googleapis.com?force-xds",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, tc.disableEnv)
			// t.Setenv cannot unset a variable; an empty value fails
			// strconv.ParseBool and falls back to the option and endpoint,
			// matching an unset variable.
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, tc.interconnectEnv)
			grpcOpts, got, err := configureDirectPath(nil, tc.opts, tc.endpoint, creds, nil)
			if err != nil {
				t.Fatalf("configureDirectPath(%q) unexpected error: %v", tc.endpoint, err)
			}
			if got != tc.endpoint {
				t.Errorf("configureDirectPath(%q) = %q, want %q", tc.endpoint, got, tc.endpoint)
			}
			if len(grpcOpts) != 0 {
				t.Errorf("configureDirectPath(%q) len(grpcOpts) = %d, want 0", tc.endpoint, len(grpcOpts))
			}
		})
	}
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

func TestIsDirectPathEnabled(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "true")

	t.Run("opts nil returns false even when interconnect env is true", func(t *testing.T) {
		if got := isDirectPathEnabled("storage.googleapis.com:443", nil); got {
			t.Errorf("isDirectPathEnabled(nil) = true, want false")
		}
	})

	t.Run("InternalOptions nil returns false even when interconnect env is true", func(t *testing.T) {
		opts := &Options{InternalOptions: nil}
		if got := isDirectPathEnabled("storage.googleapis.com:443", opts); got {
			t.Errorf("isDirectPathEnabled(opts with nil InternalOptions) = true, want false")
		}
	})

	t.Run("EnableDirectPath false returns false", func(t *testing.T) {
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath: false,
			},
		}
		if got := isDirectPathEnabled("storage.googleapis.com:443", opts); got {
			t.Errorf("isDirectPathEnabled(EnableDirectPath: false) = true, want false")
		}
	})

	t.Run("EnableDirectPath true returns true for valid endpoint", func(t *testing.T) {
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath: true,
			},
		}
		if got := isDirectPathEnabled("storage.googleapis.com:443", opts); !got {
			t.Errorf("isDirectPathEnabled(EnableDirectPath: true) = false, want true")
		}
	})
}

func TestConfigureDirectPath_PropagatesMetadataAndUniverseDomain(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	opts := &Options{
		UniverseDomain: "googleapis.com",
		InternalOptions: &InternalOptions{
			EnableDirectPath:                    true,
			EnableDirectPathXds:                 true,
			EnableDirectPathXdsOverInterconnect: true,
		},
	}
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
	})
	metadata := map[string]string{
		"x-goog-user-project": "test-project-123",
	}
	grpcOpts, gotEndpoint, err := configureDirectPath(nil, opts, "storage.googleapis.com:443", creds, metadata)
	if err != nil {
		t.Fatalf("configureDirectPath() unexpected error: %v", err)
	}
	if gotEndpoint != "google-c2p:///storage-direct.googleapis.com?force-xds" {
		t.Errorf("gotEndpoint = %q, want google-c2p:///storage-direct.googleapis.com?force-xds", gotEndpoint)
	}
	if len(grpcOpts) < 1 {
		t.Fatalf("configureDirectPath() returned %d dial options, want >= 1", len(grpcOpts))
	}

	// Verify grpcCredentialsProvider correctly propagates metadata and clientUniverseDomain
	cp := &grpcCredentialsProvider{
		creds:                creds,
		endpoint:             "storage.googleapis.com:443",
		metadata:             metadata,
		clientUniverseDomain: opts.UniverseDomain,
	}
	if got := cp.getClientUniverseDomain(); got != "googleapis.com" {
		t.Errorf("clientUniverseDomain = %q, want %q", got, "googleapis.com")
	}
	md, err := cp.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata() unexpected error: %v", err)
	}
	if md["x-goog-user-project"] != "test-project-123" {
		t.Errorf("GetRequestMetadata() missing x-goog-user-project, got: %v", md)
	}
}

func TestParseTarget(t *testing.T) {
	for _, tc := range []struct {
		input     string
		wantHost  string
		wantQuery map[string][]string
	}{
		{
			input:    "storage.googleapis.com:443",
			wantHost: "storage.googleapis.com",
		},
		{
			input:    "storage.googleapis.com",
			wantHost: "storage.googleapis.com",
		},
		{
			input:    "dns:///storage.googleapis.com:443",
			wantHost: "storage.googleapis.com",
		},
		{
			input:    "google-c2p:///storage.googleapis.com",
			wantHost: "storage.googleapis.com",
		},
		{
			input:    "google-c2p:///storage-direct.googleapis.com?force-xds",
			wantHost: "storage-direct.googleapis.com",
			wantQuery: map[string][]string{
				"force-xds": {""},
			},
		},
		{
			input:    "google-c2p:///storage-direct.googleapis.com?foo=bar&force-xds",
			wantHost: "storage-direct.googleapis.com",
			wantQuery: map[string][]string{
				"foo":       {"bar"},
				"force-xds": {""},
			},
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			gotHost, gotQuery, err := parseTarget(tc.input)
			if err != nil {
				t.Fatalf("parseTarget(%q) unexpected error: %v", tc.input, err)
			}
			if gotHost != tc.wantHost {
				t.Errorf("parseTarget(%q) host = %q, want %q", tc.input, gotHost, tc.wantHost)
			}
			for k, wantVs := range tc.wantQuery {
				gotVs := gotQuery[k]
				if !reflect.DeepEqual(gotVs, wantVs) {
					t.Errorf("parseTarget(%q) query[%q] = %v, want %v", tc.input, k, gotVs, wantVs)
				}
			}
		})
	}
}

func TestLogDirectPathMisconfig_Interconnect_SpecificWarnings(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")

	t.Run("non-default universe domain warning", func(t *testing.T) {
		opts := &Options{
			UniverseDomain: "apis-tpclp.goog",
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

		logDirectPathMisconfig("storage.apis-tpclp.goog:443", creds, opts)
		wantMsg := "DirectPath over Interconnect is disabled. Non-default universe domain is not supported."
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})

	t.Run("nil credentials warning", func(t *testing.T) {
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    true,
				EnableDirectPathXds:                 true,
				EnableDirectPathXdsOverInterconnect: true,
			},
		}
		var logOutput bytes.Buffer
		opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

		logDirectPathMisconfig("storage-direct.googleapis.com:443", nil, opts)
		wantMsg := "DirectPath over Interconnect is disabled. Valid credentials are required."
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})

	t.Run("nil token provider warning", func(t *testing.T) {
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
			TokenProvider: nil,
		})

		logDirectPathMisconfig("storage-direct.googleapis.com:443", creds, opts)
		wantMsg := "DirectPath over Interconnect is disabled. Valid credentials are required."
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})

	t.Run("isDirectPathEnabled false warning", func(t *testing.T) {
		opts := &Options{
			InternalOptions: &InternalOptions{
				EnableDirectPath:                    false,
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
		wantMsg := "DirectPath is disabled. To enable, please set the EnableDirectPath option along with the EnableDirectPathXds option."
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})
}
