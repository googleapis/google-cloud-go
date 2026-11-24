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
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/internal"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
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
			if got := isTokenProviderDirectPathCompatible(t.Context(), tt.tp, tt.opts); got != tt.want {
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
	t.Setenv(internal.UniverseDomainEnvVar, "")
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: defaultComputeTokenProvider(),
	})

	reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
	logDirectPathMisconfig(opts, endpoint, reason)

	wantedLog := directPathNotEnabledReason
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigWrongCredential(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")
	stubGCEState(t, true, internal.DefaultUniverseDomain, nil)
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
	reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
	logDirectPathMisconfig(opts, endpoint, reason)

	wantedLog := directPathIncompatibleCredReason
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigNotOnGCE(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")
	stubGCEState(t, false, "", nil)
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPath = true
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: defaultComputeTokenProvider(),
	})

	reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
	logDirectPathMisconfig(opts, endpoint, reason)

	wantedLog := directPathNotOnGCEReason
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestConfigureDirectPath_Interconnect(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")
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
			grpcOpts, gotEndpoint, err := configureDirectPath(t.Context(), nil, opts, tc.endpoint, creds, nil)
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
			bundle, err := directPathCredentialsBundle(t.Context(), tc.interconnect, opts, "storage.googleapis.com:443", creds, perRPCCreds)
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
	t.Setenv(internal.UniverseDomainEnvVar, "")
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
		grpcOpts, gotEndpoint, err := configureDirectPath(t.Context(), nil, opts, "storage.googleapis.com:443", creds, nil)
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
		_, gotEndpoint, err := configureDirectPath(t.Context(), nil, opts, "storage.googleapis.com:443", creds, nil)
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
	t.Setenv(internal.UniverseDomainEnvVar, "")
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
			_, got, err := configureDirectPath(t.Context(), nil, opts, tc.input, creds, nil)
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
	t.Setenv(internal.UniverseDomainEnvVar, "")
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
		grpcOpts, _, err := configureDirectPath(t.Context(), nil, opts, endpoint, creds, nil)
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
				UniverseDomain: "example.com",
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			endpoint: "storage.example.com:443",
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
			t.Setenv(internal.UniverseDomainEnvVar, "")
			grpcOpts, got, err := configureDirectPath(t.Context(), nil, tc.opts, tc.endpoint, creds, nil)
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
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")
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

	endpoint := "storage-direct.googleapis.com:443"
	reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
	logDirectPathMisconfig(opts, endpoint, reason)
	if logOutput.Len() > 0 {
		t.Errorf("expected no misconfig warning when Interconnect is enabled, got: %s", logOutput.String())
	}
}

func TestIsDirectPathEnabled(t *testing.T) {
	tests := []struct {
		name            string
		endpoint        string
		opts            *Options
		disableEnv      string
		interconnectEnv string
		udEnv           string
		want            bool
	}{
		{
			name:            "opts nil returns false even when interconnect env is true",
			endpoint:        "storage.googleapis.com:443",
			interconnectEnv: "true",
			want:            false,
		},
		{
			name:            "InternalOptions nil returns false even when interconnect env is true",
			endpoint:        "storage.googleapis.com:443",
			opts:            &Options{InternalOptions: nil},
			interconnectEnv: "true",
			want:            false,
		},
		{
			name:            "EnableDirectPath false returns false",
			endpoint:        "storage.googleapis.com:443",
			opts:            &Options{InternalOptions: &InternalOptions{EnableDirectPath: false}},
			interconnectEnv: "true",
			want:            false,
		},
		{
			name:     "EnableDirectPath true returns true for valid GDU endpoint",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     true,
		},
		{
			name:     "dns scheme GDU endpoint returns true",
			endpoint: "dns:///storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     true,
		},
		{
			name:     "google-c2p scheme GDU endpoint returns true",
			endpoint: "google-c2p:///storage.googleapis.com",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     true,
		},
		{
			name:     "explicit GDU universe domain returns true",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				UniverseDomain:  "googleapis.com",
				InternalOptions: &InternalOptions{EnableDirectPath: true},
			},
			want: true,
		},
		{
			name:       "disabled by env returns false",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			disableEnv: "true",
			want:       false,
		},
		{
			name:     "non-GDU universe domain option returns false",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				UniverseDomain:  "example.com",
				InternalOptions: &InternalOptions{EnableDirectPath: true},
			},
			want: false,
		},
		{
			name:     "non-GDU universe domain env returns false",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			udEnv:    "example.com",
			want:     false,
		},
		{
			name:     "non-GDU endpoint without universe domain returns false",
			endpoint: "storage.example.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     false,
		},
		{
			name:     "non-GDU dns endpoint without universe domain returns false",
			endpoint: "dns:///storage.example.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     false,
		},
		{
			name:     "lookalike endpoint returns false",
			endpoint: "storage.googleapis.com.evil.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     false,
		},
		{
			name:     "non-dns scheme returns false",
			endpoint: "https://storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, tc.disableEnv)
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, tc.interconnectEnv)
			t.Setenv(internal.UniverseDomainEnvVar, tc.udEnv)

			if got := isDirectPathEnabled(tc.endpoint, tc.opts); got != tc.want {
				t.Errorf("isDirectPathEnabled(%q, %+v) = %v, want %v", tc.endpoint, tc.opts, got, tc.want)
			}
		})
	}
}

func TestConfigureDirectPath_PropagatesMetadataAndUniverseDomain(t *testing.T) {
	t.Setenv(disableDirectPathEnvVar, "")
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")
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
	grpcOpts, gotEndpoint, err := configureDirectPath(t.Context(), nil, opts, "storage.googleapis.com:443", creds, metadata)
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
		wantErr   bool
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
		{input: "", wantErr: true},
		{input: "dns:///", wantErr: true},
		{input: "google-c2p:///", wantErr: true},
		{input: ":443", wantErr: true},
		{input: "https://storage.googleapis.com:443", wantErr: true},
		{input: "dns://8.8.8.8/storage.googleapis.com:443", wantErr: true},
		{input: "evil.com@storage.googleapis.com:443", wantErr: true},
		{input: "dns:///evil.com@storage.googleapis.com:443", wantErr: true},
		{input: "google-c2p:///evil.com@storage.googleapis.com?force-xds", wantErr: true},
		{input: "evil.com/storage.googleapis.com:443", wantErr: true},
		{input: "storage.googleapis.com:443?foo=bar", wantErr: true},
		{input: "google-c2p:///storage.googleapis.com?force-xds#frag", wantErr: true},
		{input: "foo:bar:baz.googleapis.com", wantErr: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			gotHost, gotQuery, err := parseTarget(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseTarget(%q) err = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
			if err != nil {
				return
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
	t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
	t.Setenv(internal.UniverseDomainEnvVar, "")

	t.Run("non-default universe domain warning", func(t *testing.T) {
		opts := &Options{
			UniverseDomain: "example.com",
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

		endpoint := "storage.example.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathInterconnectUnsupportedUDReason
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})

	t.Run("non-default credentials universe domain warning", func(t *testing.T) {
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
			TokenProvider:          &staticTP{tok: &auth.Token{Value: "fakeToken"}},
			UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
		})

		endpoint := "storage-direct.googleapis.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathInterconnectUnsupportedUDReason
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})

	t.Run("credentials universe domain error warning", func(t *testing.T) {
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
			UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "", errors.New("mds error")
			}),
		})

		endpoint := "storage-direct.googleapis.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathInterconnectUnsupportedUDReason
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

		endpoint := "storage-direct.googleapis.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, nil)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathInterconnectInvalidCredsReason
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

		endpoint := "storage-direct.googleapis.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathInterconnectInvalidCredsReason
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

		endpoint := "storage-direct.googleapis.com:443"
		reason, _ := canUseDirectPath(t.Context(), endpoint, opts, creds)
		logDirectPathMisconfig(opts, endpoint, reason)
		wantMsg := directPathNotEnabledReason
		if !strings.Contains(logOutput.String(), wantMsg) {
			t.Errorf("got log %q, want containing %q", logOutput.String(), wantMsg)
		}
	})
}

func defaultComputeTokenProvider() auth.TokenProvider {
	return &staticTP{
		tok: token(map[string]interface{}{
			"auth.google.tokenSource":    "compute-metadata",
			"auth.google.serviceAccount": "default",
		}),
	}
}

func nonComputeTokenProvider() auth.TokenProvider {
	return &staticTP{tok: &auth.Token{Value: "fakeToken"}}
}

func stubGCEState(t *testing.T, gce bool, mdsUD string, mdsErr error) {
	t.Helper()
	origOnComputeEngine := onComputeEngine
	origMDS := metadataUniverseDomain
	onComputeEngine = func() bool { return gce }
	metadataUniverseDomain = func(context.Context) (string, error) {
		return mdsUD, mdsErr
	}
	t.Cleanup(func() {
		onComputeEngine = origOnComputeEngine
		metadataUniverseDomain = origMDS
	})
}

type fakeALTSAuthInfo struct{}

func (fakeALTSAuthInfo) AuthType() string { return "alts" }

type countingTP struct {
	base  auth.TokenProvider
	calls int
}

func (c *countingTP) Token(ctx context.Context) (*auth.Token, error) {
	c.calls++
	return c.base.Token(ctx)
}

func TestHasUniverseDomainHost(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		universeDomain string
		want           bool
	}{
		{name: "host_with_port", endpoint: "storage.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "host_without_port", endpoint: "storage.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "exact_universe_domain_with_port", endpoint: "googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "exact_universe_domain", endpoint: "googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "sandbox_subdomain", endpoint: "storage-preprod.sandbox.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "dns_scheme_with_port", endpoint: "dns:///storage.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "dns_scheme_without_port", endpoint: "dns:///storage.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "google_c2p_scheme", endpoint: "google-c2p:///storage.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "google_c2p_scheme_with_query", endpoint: "google-c2p:///storage-direct.googleapis.com?force-xds", universeDomain: "googleapis.com", want: true},
		{name: "uppercase_host", endpoint: "STORAGE.GOOGLEAPIS.COM:443", universeDomain: "googleapis.com", want: true},
		{name: "tpc_endpoint", endpoint: "storage.example.com:443", universeDomain: "googleapis.com", want: false},
		{name: "tpc_dns_endpoint", endpoint: "dns:///storage.example.com:443", universeDomain: "googleapis.com", want: false},
		{name: "tpc_google_c2p_endpoint", endpoint: "google-c2p:///storage-direct.example.com?force-xds", universeDomain: "googleapis.com", want: false},
		{name: "lookalike_suffix_domain", endpoint: "storage.googleapis.com.evil.com:443", universeDomain: "googleapis.com", want: false},
		{name: "missing_dot_boundary", endpoint: "evilgoogleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "leading_dot", endpoint: ".googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "empty_label", endpoint: "foo..googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "userinfo_smuggling", endpoint: "evil.com@storage.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "dns_userinfo_smuggling", endpoint: "dns:///evil.com@storage.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "google_c2p_userinfo_smuggling", endpoint: "google-c2p:///evil.com@storage.googleapis.com?force-xds", universeDomain: "googleapis.com", want: false},
		{name: "path_smuggling", endpoint: "evil.com/storage.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "trailing_path_smuggling", endpoint: "storage.googleapis.com:443/evil", universeDomain: "googleapis.com", want: false},
		{name: "dns_custom_authority", endpoint: "dns://8.8.8.8/storage.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "google_c2p_custom_authority", endpoint: "google-c2p://8.8.8.8/storage.googleapis.com", universeDomain: "googleapis.com", want: false},
		{name: "https_scheme", endpoint: "https://storage.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "query_string_without_c2p", endpoint: "storage.googleapis.com:443?foo=bar", universeDomain: "googleapis.com", want: false},
		{name: "dns_query_string", endpoint: "dns:///storage.googleapis.com:443?foo=bar", universeDomain: "googleapis.com", want: false},
		{name: "google_c2p_fragment", endpoint: "google-c2p:///storage.googleapis.com?force-xds#frag", universeDomain: "googleapis.com", want: false},
		{name: "malformed_host_port", endpoint: "foo:bar:baz.googleapis.com", universeDomain: "googleapis.com", want: false},
		{name: "bare_host_no_dot", endpoint: "localhost", universeDomain: "googleapis.com", want: false},
		{name: "localhost_port", endpoint: "localhost:8080", universeDomain: "googleapis.com", want: false},
		{name: "ipv4_loopback", endpoint: "127.0.0.1:8080", universeDomain: "googleapis.com", want: false},
		{name: "ipv6_loopback", endpoint: "[::1]:8080", universeDomain: "googleapis.com", want: false},
		{name: "empty_endpoint", endpoint: "", universeDomain: "googleapis.com", want: false},
		{name: "empty_dns_target", endpoint: "dns:///", universeDomain: "googleapis.com", want: false},
		{name: "empty_c2p_target", endpoint: "google-c2p:///", universeDomain: "googleapis.com", want: false},
		{name: "empty_universe_domain", endpoint: "storage.googleapis.com:443", universeDomain: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasUniverseDomainHost(tc.endpoint, tc.universeDomain); got != tc.want {
				t.Errorf("hasUniverseDomainHost(%q, %q) = %v, want %v", tc.endpoint, tc.universeDomain, got, tc.want)
			}
		})
	}
}

func TestCanUseDirectPath(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		opts       *Options
		creds      *auth.Credentials
		disableEnv string
		udEnv      string
		gce        bool
		mdsUD      string
		mdsErr     error
		want       bool
		wantReason string
	}{
		{
			name:     "gdu_gce_adc",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
			}),
			gce:   true,
			mdsUD: "googleapis.com",
			want:  true,
		},
		{
			name:     "gdu_gce_adc_with_trailing_newline",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com\n"),
			}),
			gce:   true,
			mdsUD: "googleapis.com\n",
			want:  true,
		},
		{
			name:       "direct_path_disabled",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathNotEnabledReason,
		},
		{
			name:       "disabled_by_env",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			disableEnv: "true",
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
		},
		{
			name:     "unsupported_scheme_no_warning",
			endpoint: "https://storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:    auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:      true,
			mdsUD:    "googleapis.com",
			want:     false,
		},
		{
			name:     "tpc_gce_adc_without_client_universe_domain",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
			}),
			gce:        true,
			mdsUD:      "example.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "tpc_gce_adc_with_trailing_newline",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("example.com\n"),
			}),
			gce:        true,
			mdsUD:      "example.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "creds_universe_domain_error_fails_closed",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider: defaultComputeTokenProvider(),
				UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
					return "", errors.New("mds unavailable")
				}),
			}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:       "creds_without_ud_provider_blocked_by_tpc_mds",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:        true,
			mdsUD:      "example.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:       "creds_without_ud_provider_mds_error_fails_closed",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:        true,
			mdsErr:     errors.New("mds error"),
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "creds_without_ud_provider_allowed_by_gdu_mds",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:    auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:      true,
			mdsUD:    "googleapis.com",
			want:     true,
		},
		{
			name:     "gdu_wrapped_ud_provider_blocked_by_tpc_mds",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
			}),
			gce:        true,
			mdsUD:      "example.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "tpc_endpoint_without_client_universe_domain",
			endpoint: "storage.example.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
			}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "tpc_client_universe_domain_with_gdu_endpoint",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				UniverseDomain:  "example.com",
				InternalOptions: &InternalOptions{EnableDirectPath: true},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
			}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "tpc_env_var_with_gdu_endpoint",
			endpoint: "storage.googleapis.com:443",
			opts:     &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
			}),
			udEnv:      "example.com",
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:       "not_on_gce",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			gce:        false,
			want:       false,
			wantReason: directPathNotOnGCEReason,
		},
		{
			name:       "nil_creds",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			gce:        true,
			want:       false,
			wantReason: directPathIncompatibleCredReason,
		},
		{
			name:       "creds_with_nil_token_provider",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      &auth.Credentials{},
			gce:        true,
			want:       false,
			wantReason: directPathIncompatibleCredReason,
		},
		{
			name:       "non_compute_token_provider_disallowed",
			endpoint:   "storage.googleapis.com:443",
			opts:       &Options{InternalOptions: &InternalOptions{EnableDirectPath: true}},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: nonComputeTokenProvider()}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathIncompatibleCredReason,
		},
		{
			name:     "allow_non_default_sa_tpc_creds_rejected",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                true,
					EnableNonDefaultSAForDirectPath: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          nonComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
			}),
			gce:        true,
			mdsUD:      "googleapis.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "allow_non_default_sa_gdu_creds_on_tpc_vm_rejected",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                true,
					EnableNonDefaultSAForDirectPath: true,
				},
			},
			creds:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: nonComputeTokenProvider()}),
			gce:        true,
			mdsUD:      "example.com",
			want:       false,
			wantReason: directPathUnsupportedUniverseReason,
		},
		{
			name:     "allow_non_default_sa_gdu_creds_on_gdu_vm_allowed",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                true,
					EnableNonDefaultSAForDirectPath: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: nonComputeTokenProvider()}),
			gce:   true,
			mdsUD: "googleapis.com",
			want:  true,
		},
		{
			name:     "interconnect_tpc_creds_rejected",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          nonComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
			}),
			gce:        false,
			want:       false,
			wantReason: directPathNotOnGCEReason,
		},
		{
			name:     "interconnect_creds_ud_error_fails_closed",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider: nonComputeTokenProvider(),
				UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
					return "", errors.New("ud error")
				}),
			}),
			gce:        false,
			want:       false,
			wantReason: directPathNotOnGCEReason,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, tc.disableEnv)
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			t.Setenv(internal.UniverseDomainEnvVar, tc.udEnv)
			stubGCEState(t, tc.gce, tc.mdsUD, tc.mdsErr)

			gotReason, got := canUseDirectPath(t.Context(), tc.endpoint, tc.opts, tc.creds)
			if got != tc.want || gotReason != tc.wantReason {
				t.Errorf("canUseDirectPath(%q) = (%q, %v), want (%q, %v)", tc.endpoint, gotReason, got, tc.wantReason, tc.want)
			}
		})
	}
}

func TestDirectPathCredentialsBundle_ALTSPerRPCCredsUniverseDomain(t *testing.T) {
	tests := []struct {
		name      string
		clientUD  string
		envUD     string
		credsFile string
		wantErr   bool
	}{
		{
			name:      "matching_non_default_universe_domain_succeeds",
			clientUD:  "example.com",
			credsFile: "../internal/testdata/sa_universe_domain.json",
			wantErr:   false,
		},
		{
			name:      "mismatched_client_universe_domain_rejected",
			clientUD:  "example.com",
			credsFile: "../internal/testdata/sa.json",
			wantErr:   true,
		},
		{
			name:      "mismatched_env_universe_domain_rejected",
			envUD:     "example.com",
			credsFile: "../internal/testdata/sa.json",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(internal.UniverseDomainEnvVar, tc.envUD)
			creds := auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider: defaultComputeTokenProvider(),
			})
			perRPCCreds := &grpcCredentialsProvider{
				creds:                creds,
				endpoint:             "storage.googleapis.com:443",
				clientUniverseDomain: tc.clientUD,
			}
			opts := &Options{
				UniverseDomain: tc.clientUD,
				DetectOpts: &credentials.DetectOptions{
					CredentialsFile:  tc.credsFile,
					Audience:         "https://storage.example.com/",
					UseSelfSignedJWT: true,
				},
				InternalOptions: &InternalOptions{
					EnableDirectPath:     true,
					EnableDirectPathXds:  true,
					AllowHardBoundTokens: []string{"ALTS"},
				},
			}
			bundle, err := directPathCredentialsBundle(t.Context(), false, opts, "storage.googleapis.com:443", creds, perRPCCreds)
			if err != nil {
				t.Fatalf("directPathCredentialsBundle() unexpected error: %v", err)
			}
			altsCtx := grpccreds.NewContextWithRequestInfo(t.Context(), grpccreds.RequestInfo{
				AuthInfo: fakeALTSAuthInfo{},
			})
			md, err := bundle.PerRPCCredentials().GetRequestMetadata(altsCtx, "https://storage.example.com/")
			if (err != nil) != tc.wantErr {
				t.Fatalf("ALTSPerRPCCreds.GetRequestMetadata() err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && md["authorization"] == "" {
				t.Errorf("ALTSPerRPCCreds.GetRequestMetadata() missing authorization header")
			}
		})
	}
}

func TestDial_DirectPathUniverseDomainGating(t *testing.T) {
	tests := []struct {
		name        string
		opts        *Options
		udEnv       string
		mdsUD       string
		mdsErr      error
		wantTarget  string
		wantNumOpts int
	}{
		{
			name: "gdu_gce_xds_rewrites_to_google_c2p",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsUD:       "googleapis.com",
			wantTarget:  "google-c2p:///storage.googleapis.com",
			wantNumOpts: 1,
		},
		{
			name: "gdu_gce_xds_dns_scheme_rewrites_to_google_c2p",
			opts: &Options{
				Endpoint:         "dns:///storage.googleapis.com:443",
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
				}),
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			mdsUD:       "googleapis.com",
			wantTarget:  "google-c2p:///storage.googleapis.com",
			wantNumOpts: 1,
		},
		{
			name: "gdu_gce_grpclb_rewrites_to_dns",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
				},
			},
			mdsUD:       "googleapis.com",
			wantTarget:  "dns:///storage.googleapis.com:443",
			wantNumOpts: 3,
		},
		{
			name: "tpc_universe_domain_option_blocks_directpath",
			opts: &Options{
				UniverseDomain:   "example.com",
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsUD:       "example.com",
			wantTarget:  "storage.example.com:443",
			wantNumOpts: 2,
		},
		{
			name: "tpc_env_var_blocks_directpath",
			opts: &Options{
				Endpoint:         "storage.googleapis.com:443",
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
				}),
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			udEnv:       "example.com",
			mdsUD:       "googleapis.com",
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
		{
			name: "tpc_endpoint_without_universe_domain_blocks_directpath",
			opts: &Options{
				Endpoint:         "storage.example.com:443",
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsUD:       "googleapis.com",
			wantTarget:  "storage.example.com:443",
			wantNumOpts: 2,
		},
		{
			name: "tpc_creds_without_client_universe_domain_blocks_xds_directpath",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsUD:       "example.com",
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
		{
			name: "tpc_creds_without_client_universe_domain_blocks_grpclb_directpath",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          defaultComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
				},
			},
			mdsUD:       "example.com",
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
		{
			name: "raw_token_provider_on_tpc_vm_blocks_directpath",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: defaultComputeTokenProvider(),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsUD:       "example.com",
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
		{
			name: "raw_token_provider_with_mds_error_blocks_directpath",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: defaultComputeTokenProvider(),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			},
			mdsErr:      errors.New("mds error"),
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
		{
			name: "interconnect_with_tpc_creds_blocks_directpath",
			opts: &Options{
				DisableTelemetry: true,
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider:          nonComputeTokenProvider(),
					UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
				}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate:             "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:                    true,
					EnableDirectPathXds:                 true,
					EnableDirectPathXdsOverInterconnect: true,
				},
			},
			mdsUD:       "googleapis.com",
			wantTarget:  "storage.googleapis.com:443",
			wantNumOpts: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, "")
			t.Setenv(enableDirectPathXdsEnvVar, "")
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			t.Setenv(internal.UniverseDomainEnvVar, tc.udEnv)
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("EXPERIMENTAL_GOOGLE_API_USE_S2A", "false")
			stubGCEState(t, true, tc.mdsUD, tc.mdsErr)

			origDialContext := grpcDialContext
			origTimeoutDialer := timeoutDialerOption
			timeoutDialerOption = nil
			var gotTarget string
			var gotNumOpts int
			grpcDialContext = func(_ context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
				gotTarget = target
				gotNumOpts = len(opts)
				return nil, nil
			}
			t.Cleanup(func() {
				grpcDialContext = origDialContext
				timeoutDialerOption = origTimeoutDialer
			})

			if _, err := dial(t.Context(), true, tc.opts); err != nil {
				t.Fatalf("dial() unexpected error: %v", err)
			}
			if gotTarget != tc.wantTarget {
				t.Errorf("dial() target = %q, want %q", gotTarget, tc.wantTarget)
			}
			if gotNumOpts != tc.wantNumOpts {
				t.Errorf("dial() len(opts) = %d, want %d", gotNumOpts, tc.wantNumOpts)
			}
		})
	}
}

func TestFetchMetadataUniverseDomain(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
		wantErr    bool
	}{
		{name: "gdu_response", statusCode: http.StatusOK, body: "googleapis.com", want: "googleapis.com"},
		{name: "tpc_response_trimmed", statusCode: http.StatusOK, body: "example.com\n", want: "example.com"},
		{name: "empty_response_defaults_to_gdu", statusCode: http.StatusOK, body: "", want: "googleapis.com"},
		{name: "not_found_defaults_to_gdu", statusCode: http.StatusNotFound, body: "not found", want: "googleapis.com"},
		{name: "non_404_error_fails_closed", statusCode: http.StatusForbidden, body: "forbidden", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/computeMetadata/v1/universe/universe-domain" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(srv.URL, "http://"))

			got, err := fetchMetadataUniverseDomain(t.Context())
			if (err != nil) != tc.wantErr {
				t.Fatalf("fetchMetadataUniverseDomain() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got != tc.want {
				t.Errorf("fetchMetadataUniverseDomain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchMetadataUniverseDomain_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	origTimeout := metadataUniverseDomainTimeout
	metadataUniverseDomainTimeout = 50 * time.Millisecond
	t.Cleanup(func() { metadataUniverseDomainTimeout = origTimeout })

	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(srv.URL, "http://"))

	got, err := fetchMetadataUniverseDomain(t.Context())
	if err == nil {
		t.Fatalf("fetchMetadataUniverseDomain() = (%q, nil), want timeout error", got)
	}
}

func TestLogDirectPathMisconfig_NonDefaultUniverseDomain(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		opts     *Options
		creds    *auth.Credentials
		mdsUD    string
		mdsErr   error
	}{
		{
			name:     "non_gdu_universe_domain_option",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				UniverseDomain: "example.com",
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
		},
		{
			name:     "non_gdu_endpoint_host",
			endpoint: "storage.example.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
		},
		{
			name:     "non_gdu_credentials_universe_domain",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("example.com"),
			}),
		},
		{
			name:     "credentials_universe_domain_error",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider: defaultComputeTokenProvider(),
				UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
					return "", errors.New("mds error")
				}),
			}),
		},
		{
			name:     "non_gdu_mds_without_provider",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			mdsUD: "example.com",
		},
		{
			name:     "mds_error_without_provider",
			endpoint: "storage.googleapis.com:443",
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			},
			creds:  auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			mdsErr: errors.New("mds error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, "")
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			t.Setenv(internal.UniverseDomainEnvVar, "")
			stubGCEState(t, true, tc.mdsUD, tc.mdsErr)

			var logOutput bytes.Buffer
			tc.opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

			reason, _ := canUseDirectPath(t.Context(), tc.endpoint, tc.opts, tc.creds)
			logDirectPathMisconfig(tc.opts, tc.endpoint, reason)

			if !strings.Contains(logOutput.String(), directPathUnsupportedUniverseReason) {
				t.Errorf("logDirectPathMisconfig(%q) logged %q, want substring %q", tc.endpoint, logOutput.String(), directPathUnsupportedUniverseReason)
			}
		})
	}
}

func TestLogDirectPathMisconfig_ValidGDU(t *testing.T) {
	tests := []struct {
		name  string
		creds *auth.Credentials
		mdsUD string
	}{
		{
			name: "with_universe_domain_provider",
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          defaultComputeTokenProvider(),
				UniverseDomainProvider: internal.StaticCredentialsProperty("googleapis.com\n"),
			}),
			mdsUD: "googleapis.com",
		},
		{
			name:  "without_provider_gdu_mds",
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: defaultComputeTokenProvider()}),
			mdsUD: "googleapis.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, "")
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			t.Setenv(internal.UniverseDomainEnvVar, "")
			stubGCEState(t, true, tc.mdsUD, nil)

			var logOutput bytes.Buffer
			opts := &Options{
				Logger: slog.New(slog.NewTextHandler(&logOutput, nil)),
				InternalOptions: &InternalOptions{
					EnableDirectPath:    true,
					EnableDirectPathXds: true,
				},
			}
			endpoint := "storage.googleapis.com:443"
			reason, _ := canUseDirectPath(t.Context(), endpoint, opts, tc.creds)
			logDirectPathMisconfig(opts, endpoint, reason)

			if logOutput.Len() != 0 {
				t.Errorf("logDirectPathMisconfig() logged %q, want empty output", logOutput.String())
			}
		})
	}
}

func TestDial_DirectPathDoesNotDuplicateLookups(t *testing.T) {
	tests := []struct {
		name        string
		mdsUD       string
		wantWarning string
	}{
		{name: "healthy_gdu", mdsUD: "googleapis.com"},
		{name: "rejected_non_gdu_mds", mdsUD: "example.com", wantWarning: directPathUnsupportedUniverseReason},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(disableDirectPathEnvVar, "")
			t.Setenv(enableDirectPathXdsEnvVar, "")
			t.Setenv(enableDirectPathXdsOverInterconnectEnvVar, "")
			t.Setenv(internal.UniverseDomainEnvVar, "")
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("EXPERIMENTAL_GOOGLE_API_USE_S2A", "false")

			origOnComputeEngine := onComputeEngine
			origMDS := metadataUniverseDomain
			origDialContext := grpcDialContext
			onComputeEngine = func() bool { return true }
			mdsCalls := 0
			metadataUniverseDomain = func(context.Context) (string, error) {
				mdsCalls++
				return tc.mdsUD, nil
			}
			grpcDialContext = func(context.Context, string, ...grpc.DialOption) (*grpc.ClientConn, error) {
				return nil, nil
			}
			logRateLimiter = rate.Sometimes{Interval: time.Second}
			t.Cleanup(func() {
				onComputeEngine = origOnComputeEngine
				metadataUniverseDomain = origMDS
				grpcDialContext = origDialContext
				logRateLimiter = rate.Sometimes{Interval: time.Second}
			})

			var logOutput bytes.Buffer
			tp := &countingTP{base: defaultComputeTokenProvider()}
			opts := &Options{
				DisableTelemetry: true,
				Logger:           slog.New(slog.NewTextHandler(&logOutput, nil)),
				Credentials:      auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: tp}),
				InternalOptions: &InternalOptions{
					DefaultEndpointTemplate: "storage.UNIVERSE_DOMAIN:443",
					EnableDirectPath:        true,
					EnableDirectPathXds:     true,
				},
			}

			if _, err := dial(t.Context(), true, opts); err != nil {
				t.Fatalf("dial() unexpected error: %v", err)
			}
			if tp.calls != 1 {
				t.Errorf("dial() token lookups = %d, want 1", tp.calls)
			}
			if mdsCalls != 1 {
				t.Errorf("dial() metadata universe domain lookups = %d, want 1", mdsCalls)
			}
			if tc.wantWarning == "" {
				if logOutput.Len() != 0 {
					t.Errorf("dial() logged %q, want empty output", logOutput.String())
				}
			} else if !strings.Contains(logOutput.String(), tc.wantWarning) {
				t.Errorf("dial() logged %q, want substring %q", logOutput.String(), tc.wantWarning)
			}
		})
	}
}
