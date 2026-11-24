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

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
)

func TestEmailFromCredsJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "service account key",
			in:   `{"type":"service_account","client_email":"sa@proj.iam.gserviceaccount.com","private_key":"-----BEGIN..."}`,
			want: "sa@proj.iam.gserviceaccount.com",
		},
		{
			name: "impersonated",
			in:   `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/target@proj.iam.gserviceaccount.com:generateAccessToken"}`,
			want: "target@proj.iam.gserviceaccount.com",
		},
		{
			name: "workload identity federation",
			in:   `{"type":"external_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/wif@proj.iam.gserviceaccount.com:generateAccessToken"}`,
			want: "wif@proj.iam.gserviceaccount.com",
		},
		{
			name: "authorized user has no local email",
			in:   `{"type":"authorized_user","client_id":"abc.apps.googleusercontent.com","refresh_token":"x"}`,
			want: "",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "garbage",
			in:   "not json",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailFromCredsJSON([]byte(tc.in)); got != tc.want {
				t.Errorf("emailFromCredsJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEmailFromImpersonationURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/x@y.iam.gserviceaccount.com:generateAccessToken",
			want: "x@y.iam.gserviceaccount.com",
		},
		{in: "https://example.com/no/marker", want: ""},
		{in: "", want: ""},
	} {
		if got := emailFromImpersonationURL(tc.in); got != tc.want {
			t.Errorf("emailFromImpersonationURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteIdentity_WritesDocNextToSocket(t *testing.T) {
	dir := t.TempDir()
	udsPath := filepath.Join(dir, "sock")

	// Point ADC at a service-account key so resolution is local and
	// deterministic (no metadata server, no network).
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	pemEscaped, err := json.Marshal(string(pemBytes))
	if err != nil {
		t.Fatalf("json.Marshal PEM: %v", err)
	}

	keyPath := filepath.Join(dir, "key.json")
	const email = "unit@proj.iam.gserviceaccount.com"
	key := `{"type":"service_account","project_id":"proj","client_email":"` + email + `",` +
		`"private_key_id":"kid","token_uri":"https://oauth2.googleapis.com/token",` +
		`"private_key":` + string(pemEscaped) + `}`
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", keyPath)

	scopes := []string{"https://www.googleapis.com/auth/bigtable.data"}
	// Resolve ADC once (as main.go does) and derive the principal from those same
	// creds, rather than a second DetectDefault call inside resolution.
	creds, err := resolveCredentials(t.Context(), credentials.DetectDefault, scopes)
	if err != nil {
		t.Fatalf("resolveCredentials: %v", err)
	}
	principal, err := principalFromCreds(t.Context(), creds)
	if err != nil {
		t.Fatalf("principalFromCreds: %v", err)
	}
	if err := writeIdentity(udsPath, principal, scopes); err != nil {
		t.Fatalf("writeIdentity: %v", err)
	}

	path := filepath.Join(dir, identityFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("identity file not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("identity file mode = %o, want 600", perm)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc identityDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal identity: %v", err)
	}
	if doc.Principal != email {
		t.Errorf("principal = %q, want %q", doc.Principal, email)
	}
	if len(doc.Scopes) != 1 || doc.Scopes[0] != scopes[0] {
		t.Errorf("scopes = %v, want %v", doc.Scopes, scopes)
	}
	// The document must never contain the private key material.
	if strings.Contains(string(b), "PRIVATE KEY") {
		t.Errorf("identity doc leaked key material: %s", b)
	}
}

type staticTokenProvider struct{}

func (staticTokenProvider) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: "tok"}, nil
}

func TestResolveCredentials_FailsClosedOnUniverseDomain(t *testing.T) {
	staticTP := staticTokenProvider{}
	scopes := []string{"https://www.googleapis.com/auth/bigtable.data"}

	tests := []struct {
		name    string
		detect  func(*credentials.DetectOptions) (*auth.Credentials, error)
		wantErr bool
	}{
		{
			name: "default GDU succeeds and sets UseSelfSignedJWT",
			detect: func(opts *credentials.DetectOptions) (*auth.Credentials, error) {
				if !opts.UseSelfSignedJWT {
					t.Errorf("DetectOptions.UseSelfSignedJWT = false, want true")
				}
				return auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: staticTP,
				}), nil
			},
		},
		{
			name: "explicit GDU succeeds",
			detect: func(*credentials.DetectOptions) (*auth.Credentials, error) {
				return auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: staticTP,
					UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
						return "googleapis.com", nil
					}),
				}), nil
			},
		},
		{
			name: "TPC universe domain rejected",
			detect: func(*credentials.DetectOptions) (*auth.Credentials, error) {
				return auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: staticTP,
					UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
						return "tpc.example.test", nil
					}),
				}), nil
			},
			wantErr: true,
		},
		{
			name: "metadata lookup error fails closed",
			detect: func(*credentials.DetectOptions) (*auth.Credentials, error) {
				return auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: staticTP,
					UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
						return "", errors.New("mds timeout")
					}),
				}), nil
			},
			wantErr: true,
		},
		{
			name: "detect error fails closed",
			detect: func(*credentials.DetectOptions) (*auth.Credentials, error) {
				return nil, errors.New("no ADC")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveCredentials(t.Context(), tc.detect, scopes)
			if (err != nil) != tc.wantErr {
				t.Errorf("resolveCredentials() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
