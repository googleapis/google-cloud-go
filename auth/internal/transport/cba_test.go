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
	"testing"
	"time"
)

const (
	testMTLSEndpoint     = "test.mtls.endpoint"
	testRegularEndpoint  = "test.endpoint"
	testOverrideEndpoint = "test.override.endpoint"
	testS2AAddr          = "testS2AAddress:port"
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

func TestGetEndpoint(t *testing.T) {
	testCases := []struct {
		endpoint        string
		defaultEndpoint string
		want            string
		wantErr         bool
	}{
		{
			defaultEndpoint: "https://foo.googleapis.com/bar/baz",
			want:            "https://foo.googleapis.com/bar/baz",
		},
		{
			endpoint:        "myhost:3999",
			defaultEndpoint: "https://foo.googleapis.com/bar/baz",
			want:            "https://myhost:3999/bar/baz",
		},
		{
			endpoint:        "https://host/path/to/bar",
			defaultEndpoint: "https://foo.googleapis.com/bar/baz",
			want:            "https://host/path/to/bar",
		},
		{
			endpoint:        "host:123",
			defaultEndpoint: "",
			want:            "host:123",
		},
		{
			endpoint:        "host:123",
			defaultEndpoint: "default:443",
			want:            "host:123",
		},
		{
			endpoint:        "host:123",
			defaultEndpoint: "default:443/bar/baz",
			want:            "host:123/bar/baz",
		},
	}

	for _, tc := range testCases {
		got, err := getEndpoint(&Options{
			Endpoint:        tc.endpoint,
			DefaultEndpoint: tc.defaultEndpoint,
		}, nil)
		if tc.wantErr && err == nil {
			t.Errorf("want err, got nil err")
			continue
		}
		if !tc.wantErr && err != nil {
			t.Errorf("want nil err, got %v", err)
			continue
		}
		if tc.want != got {
			t.Errorf("getEndpoint(%q, %q): got %v; want %v", tc.endpoint, tc.defaultEndpoint, got, tc.want)
		}
	}
}

func TestGetEndpointWithClientCertSource(t *testing.T) {

	testCases := []struct {
		endpoint            string
		defaultEndpoint     string
		defaultMTLSEndpoint string
		want                string
		wantErr             bool
	}{
		{
			defaultEndpoint:     "https://foo.googleapis.com/bar/baz",
			defaultMTLSEndpoint: "https://foo.mtls.googleapis.com/bar/baz",
			want:                "https://foo.mtls.googleapis.com/bar/baz",
		},
		{
			defaultEndpoint:     "https://staging-foo.sandbox.googleapis.com/bar/baz",
			defaultMTLSEndpoint: "https://staging-foo.mtls.sandbox.googleapis.com/bar/baz",
			want:                "https://staging-foo.mtls.sandbox.googleapis.com/bar/baz",
		},
		{
			endpoint:        "myhost:3999",
			defaultEndpoint: "https://foo.googleapis.com/bar/baz",
			want:            "https://myhost:3999/bar/baz",
		},
		{
			endpoint:        "https://host/path/to/bar",
			defaultEndpoint: "https://foo.googleapis.com/bar/baz",
			want:            "https://host/path/to/bar",
		},
		{
			endpoint:        "host:port",
			defaultEndpoint: "",
			want:            "host:port",
		},
	}

	for _, tc := range testCases {
		got, err := getEndpoint(&Options{
			Endpoint:            tc.endpoint,
			DefaultEndpoint:     tc.defaultEndpoint,
			DefaultMTLSEndpoint: tc.defaultMTLSEndpoint,
		}, fakeClientCertSource)
		if tc.wantErr && err == nil {
			t.Errorf("want err, got nil err")
			continue
		}
		if !tc.wantErr && err != nil {
			t.Errorf("want nil err, got %v", err)
			continue
		}
		if tc.want != got {
			t.Errorf("getEndpoint(%q, %q): got %v; want %v", tc.endpoint, tc.defaultEndpoint, got, tc.want)
		}
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
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
		},
		{
			name: "has client cert",
			opts: &Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
				ClientCertProvider:  fakeClientCertSource,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testMTLSEndpoint,
		},
		{
			name: "no client cert, endpoint is not MTLS enabled",
			opts: &Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return false },
			want:          testRegularEndpoint,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address empty",
			opts: &Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			s2ARespFn:     invalidConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testRegularEndpoint,
		},
		{
			name: "no client cert, endpoint is MTLS enabled, S2A address not empty, override endpoint",
			opts: &Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
				Endpoint:            testOverrideEndpoint,
			},
			s2ARespFn:     validConfigResp,
			mtlsEnabledFn: func() bool { return true },
			want:          testOverrideEndpoint,
		},
	}
	defer setupTest(t)()

	for _, tc := range testCases {
		httpGetMetadataMTLSConfig = tc.s2ARespFn
		mtlsEndpointEnabledForS2A = tc.mtlsEnabledFn
		if tc.opts.ClientCertProvider != nil {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
		} else {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		}
		_, endpoint, _ := GetGRPCTransportConfigAndEndpoint(tc.opts)
		if tc.want != endpoint {
			t.Errorf("%s: want endpoint: [%s], got [%s]", tc.name, tc.want, endpoint)
		}
		// Let the cached MTLS config expire at the end of each test case.
		time.Sleep(2 * time.Millisecond)
	}
}

func TestGetHTTPTransportConfigAndEndpoint(t *testing.T) {
	testCases := []struct {
		Desc         string
		Opts         *Options
		S2ARespFunc  func() (string, error)
		MTLSEnabled  func() bool
		WantEndpoint string
		DialFuncNil  bool
	}{
		{
			"no client cert, endpoint is MTLS enabled, S2A address not empty",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			validConfigResp,
			func() bool { return true },
			testMTLSEndpoint,
			false,
		},
		{
			"has client cert",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
				ClientCertProvider:  fakeClientCertSource,
			},
			validConfigResp,
			func() bool { return true },
			testMTLSEndpoint,
			true,
		},
		{
			"no client cert, endpoint is not MTLS enabled",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			validConfigResp,
			func() bool { return false },
			testRegularEndpoint,
			true,
		},
		{
			"no client cert, endpoint is MTLS enabled, S2A address empty",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			invalidConfigResp,
			func() bool { return true },
			testRegularEndpoint,
			true,
		},
		{
			"no client cert, endpoint is MTLS enabled, S2A address not empty, override endpoint",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
				Endpoint:            testOverrideEndpoint,
			},
			validConfigResp,
			func() bool { return true },
			testOverrideEndpoint,
			false,
		},
		{
			"no client cert, S2A address not empty, but DefaultMTLSEndpoint is not set",
			&Options{
				DefaultMTLSEndpoint: "",
				DefaultEndpoint:     testRegularEndpoint,
			},
			validConfigResp,
			func() bool { return true },
			testRegularEndpoint,
			true,
		},
		{
			"no client cert, S2A address not empty, override endpoint is set",
			&Options{
				DefaultMTLSEndpoint: "",
				Endpoint:            testOverrideEndpoint,
			},
			validConfigResp,
			func() bool { return true },
			testOverrideEndpoint,
			false,
		},
		{
			"no client cert, endpoint is MTLS enabled, S2A address not empty, custom HTTP client",
			&Options{
				DefaultMTLSEndpoint: testMTLSEndpoint,
				DefaultEndpoint:     testRegularEndpoint,
			},
			validConfigResp,
			func() bool { return true },
			testRegularEndpoint,
			true,
		},
	}
	defer setupTest(t)()

	for _, tc := range testCases {
		httpGetMetadataMTLSConfig = tc.S2ARespFunc
		mtlsEndpointEnabledForS2A = tc.MTLSEnabled
		if tc.Opts.ClientCertProvider != nil {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "true")
		} else {
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		}
		_, dialFunc, _ := GetHTTPTransportConfig(tc.Opts)
		if want, got := tc.DialFuncNil, dialFunc == nil; want != got {
			t.Errorf("%s: expecting returned dialFunc is nil: [%v], got [%v]", tc.Desc, tc.DialFuncNil, got)
		}
		// Let MTLS config expire at end of each test case.
		time.Sleep(2 * time.Millisecond)
	}
}

func TestGetS2AAddress(t *testing.T) {
	testCases := []struct {
		Desc     string
		RespFunc func() (string, error)
		Want     string
	}{
		{
			Desc:     "test valid config",
			RespFunc: validConfigResp,
			Want:     testS2AAddr,
		},
		{
			Desc:     "test error when getting config",
			RespFunc: errorConfigResp,
			Want:     "",
		},
		{
			Desc:     "test invalid config",
			RespFunc: invalidConfigResp,
			Want:     "",
		},
		{
			Desc:     "test invalid JSON response",
			RespFunc: invalidJSONResp,
			Want:     "",
		},
	}

	oldHTTPGet := httpGetMetadataMTLSConfig
	oldExpiry := configExpiry
	configExpiry = time.Millisecond
	defer func() {
		httpGetMetadataMTLSConfig = oldHTTPGet
		configExpiry = oldExpiry
	}()
	for _, tc := range testCases {
		httpGetMetadataMTLSConfig = tc.RespFunc
		if want, got := tc.Want, GetS2AAddress(); got != want {
			t.Errorf("%s: want address [%s], got address [%s]", tc.Desc, want, got)
		}
		// Let the MTLS config expire at the end of each test case.
		time.Sleep(2 * time.Millisecond)
	}
}

func TestMTLSConfigExpiry(t *testing.T) {
	oldHTTPGet := httpGetMetadataMTLSConfig
	oldExpiry := configExpiry
	configExpiry = 1 * time.Second
	defer func() {
		httpGetMetadataMTLSConfig = oldHTTPGet
		configExpiry = oldExpiry
	}()
	httpGetMetadataMTLSConfig = validConfigResp
	if got, want := GetS2AAddress(), testS2AAddr; got != want {
		t.Errorf("expected address: [%s], got [%s]", want, got)
	}
	httpGetMetadataMTLSConfig = invalidConfigResp
	if got, want := GetS2AAddress(), testS2AAddr; got != want {
		t.Errorf("cached config should still be valid, expected address: [%s], got [%s]", want, got)
	}
	time.Sleep(1 * time.Second)
	if got, want := GetS2AAddress(), ""; got != want {
		t.Errorf("config should be refreshed, expected address: [%s], got [%s]", want, got)
	}
	// Let the MTLS config expire before running other tests.
	time.Sleep(1 * time.Second)
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
