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
	"testing"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/transport/cert"
)

const (
	testEndpointTemplate            = "https://test.UNIVERSE_DOMAIN/"
	testMTLSEndpoint                = "https://test.mtls.googleapis.com/"
	testMTLSEndpointTemplate        = "https://test.mtls.UNIVERSE_DOMAIN/"
	testDefaultUniverseEndpoint     = "https://test.googleapis.com/"
	testDefaultUniverseMTLSEndpoint = "https://test.mtls.googleapis.com/"
	testOverrideEndpoint            = "https://test.override.example.com/"
	testUniverseDomain              = "example.com"
	testUniverseDomainEndpoint      = "https://test.example.com/"
	testUniverseDomainMTLSEndpoint  = "https://test.mtls.example.com/"
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

	validConfigRespMTLSS2A = func() (string, error) {
		validConfig := mtlsConfig{
			S2A: &s2aAddresses{
				PlaintextAddress: "",
				MTLSAddress:      testMTLSS2AAddr,
			},
		}
		configStr, err := json.Marshal(validConfig)
		if err != nil {
			return "", err
		}
		return string(configStr), nil
	}

	validConfigRespDualS2A = func() (string, error) {
		validConfig := mtlsConfig{
			S2A: &s2aAddresses{
				PlaintextAddress: testS2AAddr,
				MTLSAddress:      testMTLSS2AAddr,
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
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opts.getUniverseDomain(); got != tc.wantUniverseDomain {
				t.Errorf("got %v, want %v", got, tc.wantUniverseDomain)
			}
			if got := tc.opts.isUniverseDomainGDU(); got != tc.wantIsGDU {
				t.Errorf("got %v, want %v", got, tc.wantIsGDU)
			}
			if got := tc.opts.defaultEndpoint(); got != tc.wantDefaultEndpoint {
				t.Errorf("got %v, want %v", got, tc.wantDefaultEndpoint)
			}
			if tc.opts.Endpoint != "" {
				got, err := tc.opts.mergedEndpoint()
				if err != nil {
					t.Fatalf("%v", err)
				}
				if got != tc.wantMergedEndpoint {
					t.Errorf("got %v, want %v", got, tc.wantMergedEndpoint)
				}
			}
		})
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

func TestGetGRPCTransportConfigAndEndpoint_S2A(t *testing.T) {
	testCases := []struct {
		name      string
		opts      *Options
		s2ARespFn func() (string, error)
		want      string
	}{
		{
			name: "has client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				ClientCertProvider:      fakeClientCertSource,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "has client cert, MTLSEndpointTemplate",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "no client cert, S2A address not empty",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "no client cert, S2A address not empty, MTLSEndpointTemplate",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "no client cert, S2A address not empty, EnableDirectPath == true",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				EnableDirectPath:        true,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseEndpoint,
		},
		{
			name: "no client cert, S2A address not empty, EnableDirectPathXds == true",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				EnableDirectPathXds:     true,
			},
			s2ARespFn: validConfigResp,
			want:      testDefaultUniverseEndpoint,
		},
		{
			name: "no client cert, S2A address empty",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn: invalidConfigResp,
			want:      testDefaultUniverseEndpoint,
		},
		{
			name: "no client cert, S2A address not empty, override endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				Endpoint:                testOverrideEndpoint,
			},
			s2ARespFn: validConfigResp,
			want:      testOverrideEndpoint,
		},
		{
			"no client cert, S2A address not empty, DefaultMTLSEndpoint not set",
			&Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     "",
			},
			validConfigResp,
			testDefaultUniverseEndpoint,
		},
		{
			"no client cert, MTLS S2A address not empty, no MTLS MDS cert",
			&Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			validConfigRespMTLSS2A,
			testDefaultUniverseEndpoint,
		},
		{
			"no client cert, dual S2A addresses, no MTLS MDS cert",
			&Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
			},
			validConfigRespDualS2A,
			testDefaultUniverseMTLSEndpoint,
		},
		{
			"no client cert, dual S2A addresses, no MTLS MDS cert, MTLSEndpointTemplate",
			&Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			validConfigRespDualS2A,
			testDefaultUniverseMTLSEndpoint,
		},
	}
	defer setupTest(t)()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpGetMetadataMTLSConfig = tc.s2ARespFn
			mtlsConfiguration, _ = queryConfig()
			if tc.opts.ClientCertProvider != nil {
				t.Setenv(googleAPIUseCertSource, "true")
			} else {
				t.Setenv(googleAPIUseCertSource, "false")
			}
			_, endpoint, _ := GetGRPCTransportCredsAndEndpoint(tc.opts)
			if tc.want != endpoint {
				t.Fatalf("want endpoint: %s, got %s", tc.want, endpoint)
			}
		})
	}
}

func TestGetHTTPTransportConfig_S2A(t *testing.T) {
	testCases := []struct {
		name        string
		opts        *Options
		s2ARespFn   func() (string, error)
		want        string
		isDialFnNil bool
	}{
		{
			name: "has client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			s2ARespFn:   validConfigResp,
			want:        testMTLSEndpointTemplate,
			isDialFnNil: true,
		},
		{
			name: "no client cert, S2A address not empty",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn: validConfigResp,
			want:      testMTLSEndpointTemplate,
		},
		{
			name: "no client cert, S2A address empty",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn:   invalidConfigResp,
			want:        testDefaultUniverseEndpoint,
			isDialFnNil: true,
		},
		{
			name: "no client cert, S2A address not empty, override endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				Endpoint:                testOverrideEndpoint,
			},
			s2ARespFn:   validConfigResp,
			want:        testOverrideEndpoint,
			isDialFnNil: true,
		},
		{
			name: "no client cert, S2A address not empty, but DefaultMTLSEndpoint is not set",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     "",
			},
			s2ARespFn:   validConfigResp,
			want:        testDefaultUniverseEndpoint,
			isDialFnNil: true,
		},
		{
			name: "no client cert, S2A address not empty, custom HTTP client",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				Client:                  http.DefaultClient,
			},
			s2ARespFn:   validConfigResp,
			want:        testDefaultUniverseEndpoint,
			isDialFnNil: true,
		},
		{
			name: "no client cert, MTLS S2A address not empty, no MTLS MDS cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn:   validConfigRespMTLSS2A,
			want:        testDefaultUniverseEndpoint,
			isDialFnNil: true,
		},
		{
			name: "no client cert, dual S2A addresses, no MTLS MDS cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			s2ARespFn:   validConfigRespDualS2A,
			want:        testMTLSEndpointTemplate,
			isDialFnNil: false,
		},
	}
	defer setupTest(t)()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpGetMetadataMTLSConfig = tc.s2ARespFn
			mtlsConfiguration, _ = queryConfig()
			if tc.opts.ClientCertProvider != nil {
				t.Setenv(googleAPIUseCertSource, "true")
			} else {
				t.Setenv(googleAPIUseCertSource, "false")
			}
			_, dialFunc, err := GetHTTPTransportConfig(tc.opts)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if want, got := tc.isDialFnNil, dialFunc == nil; want != got {
				t.Errorf("expecting returned dialFunc is nil: [%v], got [%v]", tc.isDialFnNil, got)
			}
		})
	}
}

func TestLoadMTLSMDSTransportCreds(t *testing.T) {
	testCases := []struct {
		name     string
		rootFile string
		keyFile  string
		wantErr  bool
	}{
		{
			name:     "missing root file",
			rootFile: "",
			keyFile:  "./testdata/mtls_mds_key.pem",
			wantErr:  true,
		},
		{
			name:     "missing key file",
			rootFile: "./testdata/mtls_mds_root.pem",
			keyFile:  "",
			wantErr:  true,
		},
		{
			name:     "missing both root and key files",
			rootFile: "",
			keyFile:  "",
			wantErr:  true,
		},
		{
			name:     "load credentials success",
			rootFile: "./testdata/mtls_mds_root.pem",
			keyFile:  "./testdata/mtls_mds_key.pem",
			wantErr:  false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadMTLSMDSTransportCreds(tc.rootFile, tc.keyFile)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("loadMTLSMDSTransportCreds(%q, %q) got error: %v, want error: %v", tc.rootFile, tc.keyFile, gotErr, tc.wantErr)
			}
		})
	}
}

func setupTest(t *testing.T) func() {
	oldHTTPGet := httpGetMetadataMTLSConfig
	t.Setenv(googleAPIUseS2AEnv, "true")

	return func() {
		httpGetMetadataMTLSConfig = oldHTTPGet
	}
}

func TestGetTransportConfig_UniverseDomain(t *testing.T) {
	testCases := []struct {
		name         string
		opts         *Options
		wantEndpoint string
	}{
		{
			name: "google default universe (GDU), no client cert, template is regular endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testDefaultUniverseEndpoint,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			wantEndpoint: testDefaultUniverseEndpoint,
		},
		{
			name: "google default universe (GDU), no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			wantEndpoint: testDefaultUniverseEndpoint,
		},
		{
			name: "google default universe (GDU), client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpoint,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "google default universe (GDU), client cert, MTLSEndpointTemplate",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "UniverseDomain, no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
			},
			wantEndpoint: testUniverseDomainEndpoint,
		},
		{
			name: "UniverseDomain, client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testUniverseDomainMTLSEndpoint,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opts.ClientCertProvider != nil {
				t.Setenv(googleAPIUseCertSource, "true")
			} else {
				t.Setenv(googleAPIUseCertSource, "false")
			}
			config, err := getTransportConfig(tc.opts)
			if err != nil {
				t.Fatalf("err: %v", err)
			} else {
				if tc.wantEndpoint != config.endpoint {
					t.Errorf("want endpoint: %s, got %s", tc.wantEndpoint, config.endpoint)
				}
			}
		})
	}
}

func TestGetGRPCTransportCredsAndEndpoint_UniverseDomain(t *testing.T) {
	testCases := []struct {
		name         string
		opts         *Options
		wantEndpoint string
	}{
		{
			name: "google default universe (GDU), no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
			},
			wantEndpoint: testDefaultUniverseEndpoint,
		},
		{
			name: "google default universe (GDU), no client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
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
			wantEndpoint: testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "google default universe (GDU), client cert, MTLSEndpointTemplate",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testDefaultUniverseMTLSEndpoint,
		},
		{
			name: "google default universe (GDU), client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				ClientCertProvider:      fakeClientCertSource,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
		{
			name: "UniverseDomain, no client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
			},
			wantEndpoint: testUniverseDomainEndpoint,
		},
		{
			name: "UniverseDomain, no client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
		{
			name: "UniverseDomain, client cert",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
			},
			wantEndpoint: testUniverseDomainMTLSEndpoint,
		},
		{
			name: "UniverseDomain, client cert, endpoint",
			opts: &Options{
				DefaultEndpointTemplate: testEndpointTemplate,
				DefaultMTLSEndpoint:     testMTLSEndpointTemplate,
				UniverseDomain:          testUniverseDomain,
				ClientCertProvider:      fakeClientCertSource,
				Endpoint:                testOverrideEndpoint,
			},
			wantEndpoint: testOverrideEndpoint,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opts.ClientCertProvider != nil {
				t.Setenv(googleAPIUseCertSource, "true")
			} else {
				t.Setenv(googleAPIUseCertSource, "false")
			}
			_, endpoint, err := GetGRPCTransportCredsAndEndpoint(tc.opts)
			if err != nil {
				t.Fatalf("err: %v", err)
			} else {
				if tc.wantEndpoint != endpoint {
					t.Errorf("want endpoint: %s, got %s", tc.wantEndpoint, endpoint)
				}
			}
		})
	}
}

func TestGetClientCertificateProvider(t *testing.T) {
	testCases := []struct {
		name             string
		opts             *Options
		useCertEnvVar    string
		wantCertProvider cert.Provider
		wantErr          error
	}{
		{
			name: "UseCertEnvVar false, Domain is GDU",
			opts: &Options{
				UniverseDomain:     internal.DefaultUniverseDomain,
				ClientCertProvider: fakeClientCertSource,
				Endpoint:           testDefaultUniverseEndpoint,
			},
			useCertEnvVar:    "false",
			wantCertProvider: nil,
		},
		{
			name: "UseCertEnvVar unset, Domain is not GDU",
			opts: &Options{
				UniverseDomain:     testUniverseDomain,
				ClientCertProvider: fakeClientCertSource,
				Endpoint:           testOverrideEndpoint,
			},
			useCertEnvVar:    "unset",
			wantCertProvider: nil,
		},
		{
			name: "UseCertEnvVar unset, Domain is GDU",
			opts: &Options{
				UniverseDomain:     internal.DefaultUniverseDomain,
				ClientCertProvider: fakeClientCertSource,
				Endpoint:           testDefaultUniverseEndpoint,
			},
			useCertEnvVar:    "unset",
			wantCertProvider: fakeClientCertSource,
		},
		{
			name: "UseCertEnvVar true, Domain is not GDU",
			opts: &Options{
				UniverseDomain:     testUniverseDomain,
				ClientCertProvider: fakeClientCertSource,
				Endpoint:           testOverrideEndpoint,
			},
			useCertEnvVar:    "true",
			wantCertProvider: fakeClientCertSource,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useCertEnvVar != "unset" {
				t.Setenv(googleAPIUseCertSource, tc.useCertEnvVar)
			}
			certProvider, err := GetClientCertificateProvider(tc.opts)
			if err != nil {
				if err != tc.wantErr {
					t.Fatalf("err: %v", err)
				}
			} else {
				want := fmt.Sprintf("%v", tc.wantCertProvider)
				got := fmt.Sprintf("%v", certProvider)
				if want != got {
					t.Errorf("want cert provider: %v, got %v", want, got)
				}
			}
		})
	}
}
