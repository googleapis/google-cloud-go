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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	testMTLSEndpoint           = "https://test.mtls.googleapis.com/"
	testEndpointTemplate       = "https://test.UNIVERSE_DOMAIN/"
	testRegularEndpoint        = "https://test.googleapis.com/"
	testOverrideEndpoint       = "https://test.override.example.com/"
	testUniverseDomain         = "example.com"
	testUniverseDomainEndpoint = "https://test.example.com/"
)

var (
	validConfigResp = func() (string, error) {
		validConfig := mtlsConfig{
			S2A: &s2aAddresses{
				PlaintextAddress: testS2AAddr,
				MTLSAddress:      "",
			},
		}
		configStr, err := json.Marshal(validConfig)
		if err != nil {
			return "", err
		}
		return string(configStr), nil
	}

	errorConfigResp = func() (string, error) {
		return "", fmt.Errorf("error getting config")
	}

	invalidConfigResp = func() (string, error) {
		return "{}", nil
	}

	invalidJSONResp = func() (string, error) {
		return "test", nil
	}
	fakeClientCertSource = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) { return nil, nil }
)

func TestOptions_UniverseDomain(t *testing.T) {
	testCases := []struct {
		name                string
		opts                *Options
		wantUniverseDomain  string
		wantDefaultEndpoint string
		wantIsGDU           bool
		wantMergedEndpoint  string
	}{
		{
			name:                "empty",
			opts:                &Options{},
			wantUniverseDomain:  "googleapis.com",
			wantDefaultEndpoint: "",
			wantIsGDU:           true,
			wantMergedEndpoint:  "",
		},
		{
			name: "defaults",
			opts: &Options{
				DefaultEndpointTemplate: "https://test.UNIVERSE_DOMAIN/",
			},
			wantUniverseDomain:  "googleapis.com",
			wantDefaultEndpoint: "https://test.googleapis.com/",
			wantIsGDU:           true,
			wantMergedEndpoint:  "",
		},
		{
			name: "non-GDU",
			opts: &Options{
				DefaultEndpointTemplate: "https://test.UNIVERSE_DOMAIN/",
				UniverseDomain:          "example.com",
			},
			wantUniverseDomain:  "example.com",
			wantDefaultEndpoint: "https://test.example.com/",
			wantIsGDU:           false,
			wantMergedEndpoint:  "",
		},
		{
			name: "merged endpoint",
			opts: &Options{
				DefaultEndpointTemplate: "https://test.UNIVERSE_DOMAIN/bar/baz",
				Endpoint:                "myhost:8000",
			},
			wantUniverseDomain:  "googleapis.com",
			wantDefaultEndpoint: "https://test.googleapis.com/bar/baz",
			wantIsGDU:           true,
			wantMergedEndpoint:  "https://myhost:8000/bar/baz",
		},
	}
	for _, tc := range testCases {
		if got := tc.opts.getUniverseDomain(); got != tc.wantUniverseDomain {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.wantUniverseDomain)
		}
		if got := tc.opts.isUniverseDomainGDU(); got != tc.wantIsGDU {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.wantIsGDU)
		}
		if got := tc.opts.defaultEndpoint(); got != tc.wantDefaultEndpoint {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.wantDefaultEndpoint)
		}
		if tc.opts.Endpoint != "" {
			got, err := tc.opts.mergedEndpoint()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got != tc.wantMergedEndpoint {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.wantMergedEndpoint)
			}
		}
	}
}

func TestGetEndpoint(t *testing.T) {
	testCases := []struct {
		endpoint                string
		defaultEndpointTemplate string
		want                    string
		wantErr                 bool
	}{
		{
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			want:                    "https://foo.googleapis.com/bar/baz",
		},
		{
			endpoint:                "myhost:3999",
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			want:                    "https://myhost:3999/bar/baz",
		},
		{
			endpoint:                "https://host/path/to/bar",
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			want:                    "https://host/path/to/bar",
		},
		{
			endpoint:                "host:123",
			defaultEndpointTemplate: "",
			want:                    "host:123",
		},
		{
			endpoint:                "host:123",
			defaultEndpointTemplate: "default:443",
			want:                    "host:123",
		},
		{
			endpoint:                "host:123",
			defaultEndpointTemplate: "default:443/bar/baz",
			want:                    "host:123/bar/baz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got, err := getEndpoint(&Options{
				Endpoint:                tc.endpoint,
				DefaultEndpointTemplate: tc.defaultEndpointTemplate,
			}, nil)
			if tc.wantErr && err == nil {
				t.Fatalf("want err, got nil err")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil err, got %v", err)
			}
			if tc.want != got {
				t.Errorf("getEndpoint(%q, %q): got %v; want %v", tc.endpoint, tc.defaultEndpointTemplate, got, tc.want)
			}
		})
	}
}

func TestGetEndpointWithClientCertSource(t *testing.T) {
	testCases := []struct {
		endpoint                string
		defaultEndpointTemplate string
		defaultMTLSEndpoint     string
		want                    string
		wantErr                 bool
	}{
		{
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			defaultMTLSEndpoint:     "https://foo.mtls.googleapis.com/bar/baz",
			want:                    "https://foo.mtls.googleapis.com/bar/baz",
		},
		{
			defaultEndpointTemplate: "https://staging-foo.sandbox.UNIVERSE_DOMAIN/bar/baz",
			defaultMTLSEndpoint:     "https://staging-foo.mtls.sandbox.googleapis.com/bar/baz",
			want:                    "https://staging-foo.mtls.sandbox.googleapis.com/bar/baz",
		},
		{
			endpoint:                "myhost:3999",
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			want:                    "https://myhost:3999/bar/baz",
		},
		{
			endpoint:                "https://host/path/to/bar",
			defaultEndpointTemplate: "https://foo.UNIVERSE_DOMAIN/bar/baz",
			want:                    "https://host/path/to/bar",
		},
		{
			endpoint:                "host:port",
			defaultEndpointTemplate: "",
			want:                    "host:port",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got, err := getEndpoint(&Options{
				Endpoint:                tc.endpoint,
				DefaultEndpointTemplate: tc.defaultEndpointTemplate,
				DefaultMTLSEndpoint:     tc.defaultMTLSEndpoint,
			}, fakeClientCertSource)
			if tc.wantErr && err == nil {
				t.Fatalf("want err, got nil err")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil err, got %v", err)
			}
			if tc.want != got {
				t.Fatalf("getEndpoint(%q, %q): got %v; want %v", tc.endpoint, tc.defaultEndpointTemplate, got, tc.want)
			}
		})
	}
}

func TestGetGRPCTransportConfigAndEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		opts          *Options
		s2ARespFn     func() (string, error)
		mtlsEnabledFn func() bool
		want          string
	}{
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
		},
		{
			name: "has client cert",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
		},
		{
			name: "no client cert, endpoint is not MTLS enabled",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return false },
			want:          testRegularEndpoint,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address empty",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2ARespFn:     invalidConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testRegularEndpoint,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty, override endpoint",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
				Endpoint:                testOverrideEndpoint,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testOverrideEndpoint,
		},
	}
	defer setupTest(t)()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpGetMetadataMTLSConfig = tc.s2ARespFn
			mtlsEndpointEnabledForS2A = tc.mtlsEnabledFn
			if tc.opts.ClientCertProvider != nil {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
			} else {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			}
			_, endpoint, _ := GetGRPCTransportCredsAndEndpoint(tc.opts)
			if tc.want != endpoint {
				t.Fatalf("%s: want endpoint: [%s], got [%s]", tc.name, tc.want, endpoint)
			}
			// Let the cached MTLS config expire at the end of each test case.
			time.Sleep(2 * time.Millisecond)
		})
	}
}

func TestGetHTTPTransportConfig_S2a(t *testing.T) {
	testCases := []struct {
		name          string
		opts          *Options
		s2aFn         func() (string, error)
		mtlsEnabledFn func() bool
		want          string
		isDialFnNil   bool
	}{
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
		},
		{
			name: "has client cert",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
			isDialFnNil:   true,
		},
		{
			name: "no client cert, endpoint is not MTLS enabled",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return false },
			want:          testRegularEndpoint,
			isDialFnNil:   true,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address empty",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2aFn:         invalidConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testRegularEndpoint,
			isDialFnNil:   true,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty, override endpoint",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
				Endpoint:                testOverrideEndpoint,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testOverrideEndpoint,
		},
		{
			name: "no client cert, S2A address not empty, but DefaultMTLSEndpoint is not set",
			opts: &Options{
				DefaultMTLSEndpoint:     "",
				DefaultEndpointTemplate: testEndpointTemplate,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testRegularEndpoint,
			isDialFnNil:   true,
		},
		{
			name: "no client cert, S2A address not empty, override endpoint is set",
			opts: &Options{
				DefaultMTLSEndpoint: "",
				Endpoint:            testOverrideEndpoint,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testOverrideEndpoint,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty, custom HTTP client",
			opts: &Options{
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				DefaultEndpointTemplate: testEndpointTemplate,
				Client:                  http.DefaultClient,
			},
			s2aFn:         validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testRegularEndpoint,
			isDialFnNil:   true,
		},
	}
	defer setupTest(t)()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpGetMetadataMTLSConfig = tc.s2aFn
			mtlsEndpointEnabledForS2A = tc.mtlsEnabledFn
			if tc.opts.ClientCertProvider != nil {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
			} else {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			}
			_, dialFunc, err := GetHTTPTransportConfig(tc.opts)
			if err != nil {
				t.Fatalf("%s: err: %v", tc.name, err)
			}
			if want, got := tc.isDialFnNil, dialFunc == nil; want != got {
				t.Errorf("%s: expecting returned dialFunc is nil: [%v], got [%v]", tc.name, tc.isDialFnNil, got)
			}
			// Let MTLS config expire at end of each test case.
			time.Sleep(2 * time.Millisecond)
		})
	}
}

func setupTest(t *testing.T) func() {
	oldDefaultMTLSEnabled := mtlsEndpointEnabledForS2A
	oldHTTPGet := httpGetMetadataMTLSConfig
	oldExpiry := configExpiry

	configExpiry = time.Millisecond
	t.Setenv(googleAPIUseS2AEnv, "true")

	return func() {
		httpGetMetadataMTLSConfig = oldHTTPGet
		mtlsEndpointEnabledForS2A = oldDefaultMTLSEnabled
		configExpiry = oldExpiry
	}
}

func TestGetTransportConfig_UniverseDomain(t *testing.T) {
	testCases := []struct {
		name         string
		opts         *Options
		wantEndpoint string
		wantErr      error
	}{
		{
			name: "google default universe (GDU), no client cert, template is regular endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testRegularEndpoint,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
			},
			wantEndpoint: testRegularEndpoint,
		},
		{
			name: "google default universe (GDU), no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
			},
			wantEndpoint: testRegularEndpoint,
		},
		{
			name: "google default universe (GDU), client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testMTLSEndpoint,
		},
		{
			name: "UniverseDomain, no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
			},
			wantEndpoint: testUniverseDomainEndpoint,
		},
		{
			name: "UniverseDomain, client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testUniverseDomainEndpoint,
			wantErr:      errUniverseNotSupportedMTLS,
		},
	}

	for _, tc := range testCases {
		if tc.opts.ClientCertProvider != nil {
			os.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
		} else {
			os.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		}
		config, err := getTransportConfig(tc.opts)
		if err != nil {
			if err != tc.wantErr {
				t.Fatalf("%s: err: %v", tc.name, err)
			}
		} else {
			if tc.wantEndpoint != config.endpoint {
				t.Errorf("%s: want endpoint: [%s], got [%s]", tc.name, tc.wantEndpoint, config.endpoint)
			}
		}
	}
}

func TestGetGRPCTransportCredsAndEndpoint_UniverseDomain(t *testing.T) {
	testCases := []struct {
		name         string
		opts         *Options
		wantEndpoint string
		wantErr      error
	}{
		{
			name: "google default universe (GDU), no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
			},
			wantEndpoint: testRegularEndpoint,
		},
		{
			name: "google default universe (GDU), no client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
		{
			name: "google default universe (GDU), client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testMTLSEndpoint,
		},
		{
			name: "google default universe (GDU), client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				ClientCertProvider:      fakeClientCertSource,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
		{
			name: "UniverseDomain, no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
			},
			wantEndpoint: testUniverseDomainEndpoint,
		},
		{
			name: "UniverseDomain, no client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
		{
			name: "UniverseDomain, client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantErr: errUniverseNotSupportedMTLS,
		},
		{
			name: "UniverseDomain, client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
	}

	for _, tc := range testCases {
		if tc.opts.ClientCertProvider != nil {
			os.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
		} else {
			os.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		}
		_, endpoint, err := GetGRPCTransportCredsAndEndpoint(tc.opts)
		if err != nil {
			if err != tc.wantErr {
				t.Fatalf("%s: err: %v", tc.name, err)
			}
		} else {
			if tc.wantEndpoint != endpoint {
				t.Errorf("%s: want endpoint: [%s], got [%s]", tc.name, tc.wantEndpoint, endpoint)
			}
		}
	}
}
