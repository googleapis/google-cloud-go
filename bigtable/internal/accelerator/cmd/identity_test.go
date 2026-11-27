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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
)

func TestIdentityFromCredsJSON(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        string
		wantEmail string
		wantType  string
	}{
		{
			name:      "service_account_key",
			in:        `{"type":"service_account","client_email":"sa@proj.iam.gserviceaccount.com","private_key":"-----BEGIN..."}`,
			wantEmail: "sa@proj.iam.gserviceaccount.com",
			wantType:  "service_account",
		},
		{
			name:      "impersonated_service_account",
			in:        `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/target@proj.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"service_account","client_email":"source@proj.iam.gserviceaccount.com"}}`,
			wantEmail: "target@proj.iam.gserviceaccount.com",
			wantType:  "impersonated_service_account",
		},
		{
			name:      "external_account_with_impersonation",
			in:        `{"type":"external_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/wif@proj.iam.gserviceaccount.com:generateAccessToken"}`,
			wantEmail: "wif@proj.iam.gserviceaccount.com",
			wantType:  "external_account",
		},
		{
			name:      "external_account_without_impersonation",
			in:        `{"type":"external_account","audience":"//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/p/providers/pr"}`,
			wantEmail: "",
			wantType:  "external_account",
		},
		{
			name:      "authorized_user",
			in:        `{"type":"authorized_user","client_id":"abc.apps.googleusercontent.com","refresh_token":"x"}`,
			wantEmail: "",
			wantType:  "authorized_user",
		},
		{
			name:      "external_account_authorized_user",
			in:        `{"type":"external_account_authorized_user","audience":"//iam.googleapis.com/locations/global/workforcePools/p/providers/pr","refresh_token":"x"}`,
			wantEmail: "",
			wantType:  "external_account_authorized_user",
		},
		{
			name:      "gdch_service_account",
			in:        `{"type":"gdch_service_account","project":"proj","name":"sa"}`,
			wantEmail: "",
			wantType:  "gdch_service_account",
		},
		{
			name:      "empty",
			in:        "",
			wantEmail: "",
			wantType:  "",
		},
		{
			name:      "garbage",
			in:        "not json",
			wantEmail: "",
			wantType:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotEmail, gotType := identityFromCredsJSON([]byte(tc.in))
			if gotEmail != tc.wantEmail || gotType != tc.wantType {
				t.Errorf("identityFromCredsJSON(%q) = (%q, %q), want (%q, %q)", tc.in, gotEmail, gotType, tc.wantEmail, tc.wantType)
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
	principal, credType, err := identityFromCreds(t.Context(), creds)
	if err != nil {
		t.Fatalf("identityFromCreds: %v", err)
	}
	if err := writeIdentity(udsPath, principal, credType, scopes); err != nil {
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
	if doc.Type != "service_account" {
		t.Errorf("type = %q, want %q", doc.Type, "service_account")
	}
	if len(doc.Scopes) != 1 || doc.Scopes[0] != scopes[0] {
		t.Errorf("scopes = %v, want %v", doc.Scopes, scopes)
	}
	// The document must never contain the private key material.
	if strings.Contains(string(b), "PRIVATE KEY") {
		t.Errorf("identity doc leaked key material: %s", b)
	}
}

func TestIdentityFromCreds_NoGCEFallthroughForJSONOrNilCreds(t *testing.T) {
	const gceEmail = "vm-default-sa@gce.iam.gserviceaccount.com"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Metadata-Flavor", "Google")
		if strings.HasSuffix(r.URL.Path, "/service-accounts/default/email") {
			_, _ = w.Write([]byte(gceEmail))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(ts.URL, "http://"))

	for _, tc := range []struct {
		name          string
		creds         *auth.Credentials
		wantPrincipal string
		wantType      string
	}{
		{
			name:          "nil_credentials",
			creds:         nil,
			wantPrincipal: "",
			wantType:      "",
		},
		{
			name: "authorized_user_json",
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				JSON: []byte(`{"type":"authorized_user","client_id":"abc","refresh_token":"tok"}`),
			}),
			wantPrincipal: "",
			wantType:      "authorized_user",
		},
		{
			name: "direct_wif_external_account_json",
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				JSON: []byte(`{"type":"external_account","audience":"//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/p/providers/pr"}`),
			}),
			wantPrincipal: "",
			wantType:      "external_account",
		},
		{
			name: "service_account_json",
			creds: auth.NewCredentials(&auth.CredentialsOptions{
				JSON: []byte(`{"type":"service_account","client_email":"sa@proj.iam.gserviceaccount.com"}`),
			}),
			wantPrincipal: "sa@proj.iam.gserviceaccount.com",
			wantType:      "service_account",
		},
		{
			name:          "gce_metadata_credentials",
			creds:         &auth.Credentials{},
			wantPrincipal: gceEmail,
			wantType:      gceMetadataCredType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPrincipal, gotType, err := identityFromCreds(t.Context(), tc.creds)
			if err != nil {
				t.Fatalf("identityFromCreds: %v", err)
			}
			if gotPrincipal != tc.wantPrincipal || gotType != tc.wantType {
				t.Errorf("identityFromCreds() = (%q, %q), want (%q, %q)", gotPrincipal, gotType, tc.wantPrincipal, tc.wantType)
			}
		})
	}
}

func TestIdentityFromCreds_MetadataError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Metadata-Flavor", "Google")
		if strings.HasSuffix(r.URL.Path, "/service-accounts/default/email") {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(ts.URL, "http://"))

	gotPrincipal, gotType, err := identityFromCreds(t.Context(), &auth.Credentials{})
	if err == nil {
		t.Fatal("identityFromCreds() error = nil, want non-nil")
	}
	if gotPrincipal != "" || gotType != "" {
		t.Errorf("identityFromCreds() = (%q, %q), want (\"\", \"\")", gotPrincipal, gotType)
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
