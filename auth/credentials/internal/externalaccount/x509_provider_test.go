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

package externalaccount

import (
	"context"
	"testing"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/credsfile"
)

func TestCreateX509Credential(t *testing.T) {
	var tests = []struct {
		name              string
		certificateConfig credsfile.CertificateConfig
		wantErr           bool
	}{
		{
			name: "Basic Creation",
			certificateConfig: credsfile.CertificateConfig{
				UseDefaultCertificateConfig: true,
			},
		},
		{
			name: "Specific location",
			certificateConfig: credsfile.CertificateConfig{
				CertificateConfigLocation: "test",
			},
		},
		{
			name: "Default and location provided",
			certificateConfig: credsfile.CertificateConfig{
				UseDefaultCertificateConfig: true,
				CertificateConfigLocation:   "test",
			},
			wantErr: true,
		},
		{
			name:              "Neither default or location provided",
			certificateConfig: credsfile.CertificateConfig{},
			wantErr:           true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newSubjectTokenProvider(&Options{
				Client: internal.DefaultClient(),
				CredentialSource: &credsfile.CredentialSource{
					Certificate: &tt.certificateConfig,
				},
			})
			if tt.wantErr == true {
				if err == nil {
					t.Fatalf("got nil, want an error")
				}
			} else if err != nil {
				t.Errorf("got error: %v, expected no error", err)
			}
		})
	}
}

func TestRetrieveSubjectToken_X509(t *testing.T) {
	opts := cloneTestOpts()
	opts.CredentialSource = &credsfile.CredentialSource{
		Certificate: &credsfile.CertificateConfig{
			UseDefaultCertificateConfig: true,
		},
	}

	base, err := newSubjectTokenProvider(opts)
	if err != nil {
		t.Fatalf("newSubjectTokenProvider(): %v", err)
	}

	got, err := base.subjectToken(context.Background())
	if err != nil {
		t.Fatalf("subjectToken(): %v", err)
	}

	if want := ""; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := base.providerType(), x509ProviderType; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClient_Success(t *testing.T) {
	client, err := createX509Client("testdata/certificate_config_workload.json")
	if err != nil {
		t.Fatalf("createX509Client(): %v", err)
	}

	if client == nil {
		t.Error("client returned was nil")
	}
}

func TestGetClient_error(t *testing.T) {
	if _, err := createX509Client("testdata/bad_file.json"); err == nil {
		t.Errorf("got nil, want an error")
	}
}
