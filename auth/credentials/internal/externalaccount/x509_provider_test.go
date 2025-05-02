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
	"encoding/base64"
	"encoding/json"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/credsfile"
)

const (
	testDataDir = "testdata"
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

func loadCerAsEncodedString(path string) (string, error) {
	leafCertData, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	leafCert, err := parseCertificate(leafCertData)
	if err != nil {
		return "", err
	}
	leafCertEncoded := base64.StdEncoding.EncodeToString(leafCert.Raw)
	return leafCertEncoded, nil
}

func TestRetrieveSubjectToken_X509(t *testing.T) {
	encodedLeaf, err := loadCerAsEncodedString(path.Join(testDataDir, "x509_leaf_certificate.pem"))
	if err != nil {
		t.Fatalf("Failed to load the test leaf certificate: %v", err)
	}
	encodedIntermediate, err := loadCerAsEncodedString(path.Join(testDataDir, "x509_intermediate_certificate.pem"))
	if err != nil {
		t.Fatalf("Failed to load the test intermediate certificate: %v", err)
	}
	tests := []struct {
		name           string
		trustChainPath string
		configFilePath string
		wantErr        bool
		wantErrMsg     string
		trustChain     []string
	}{
		{
			name:           "no_trust_chain",
			configFilePath: path.Join(testDataDir, "x509_certificate_config.json"),
			wantErr:        false,
			trustChain:     []string{encodedLeaf},
		},
		{
			name:           "trust_chain_with_leaf",
			trustChainPath: path.Join(testDataDir, "trust_chain_with_leaf.pem"),
			configFilePath: path.Join(testDataDir, "x509_certificate_config.json"),
			wantErr:        false,
			trustChain:     []string{encodedLeaf, encodedIntermediate},
		},
		{
			name:           "trust_chain_without_leaf",
			trustChainPath: path.Join(testDataDir, "trust_chain_without_leaf.pem"),
			configFilePath: path.Join(testDataDir, "x509_certificate_config.json"),
			wantErr:        false,
			trustChain:     []string{encodedLeaf, encodedIntermediate},
		},
		{
			name:           "trust_chain_wrong_order",
			trustChainPath: path.Join(testDataDir, "trust_chain_wrong_order.pem"),
			configFilePath: path.Join(testDataDir, "x509_certificate_config.json"),
			wantErr:        true,
			wantErrMsg:     "the leaf certificate must be at the top of the trust chain file",

			trustChain: []string{encodedLeaf, encodedIntermediate},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &x509Provider{
				TrustChainPath: tt.trustChainPath,
				ConfigFilePath: tt.configFilePath,
			}

			got, err := provider.subjectToken(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, but got none")
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error got: %v, want: %v", err, tt.wantErrMsg)
				}
				return
			}

			var gotTrustChain []string
			if err := json.Unmarshal([]byte(got), &gotTrustChain); err != nil {
				t.Fatalf("failed to unmarshal got: %v", err)
			}

			if !reflect.DeepEqual(gotTrustChain, tt.trustChain) {
				t.Errorf("got %v, want %v", gotTrustChain, tt.trustChain)
			}
			if got, want := provider.providerType(), x509ProviderType; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}

}

func TestClient_Success(t *testing.T) {
	client, err := createX509Client(path.Join(testDataDir, "certificate_config_workload.json"))
	if err != nil {
		t.Fatalf("createX509Client(): %v", err)
	}

	if client == nil {
		t.Error("client returned was nil")
	}
}

func TestGetClient_error(t *testing.T) {
	if _, err := createX509Client(path.Join(testDataDir, "bad_file.json")); err == nil {
		t.Errorf("got nil, want an error")
	}
}
