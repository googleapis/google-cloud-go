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

package idtoken

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/credentials/internal/impersonate"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/credsfile"
)

func TestNewCredentials_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Options
		wantErr error
	}{
		{
			name:    "missing opts",
			wantErr: errMissingOpts,
		},
		{
			name:    "missing audience",
			opts:    &Options{},
			wantErr: errMissingAudience,
		},
		{
			name: "both credentials",
			opts: &Options{
				Audience:        "aud",
				CredentialsFile: "creds.json",
				CredentialsJSON: []byte{0, 1},
			},
			wantErr: errBothFileAndJSON,
		},
	}
	for _, tt := range tests {
		name := tt.name
		t.Run(name, func(t *testing.T) {
			err := tt.opts.validate()
			if err == nil {
				t.Fatalf("error expected: %s", tt.wantErr)
			}
			if err != tt.wantErr {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCredentials_ServiceAccount(t *testing.T) {
	ctx := context.Background()
	wantTok, _ := createRS256JWT(t)
	b, err := os.ReadFile("../../internal/testdata/sa.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := credsfile.ParseServiceAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"id_token": "%s"}`, wantTok)))
	}))
	defer ts.Close()
	f.TokenURL = ts.URL
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := NewCredentials(&Options{
		Audience:        "aud",
		CredentialsJSON: b,
		CustomClaims: map[string]interface{}{
			"foo": "bar",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := creds.Token(ctx)
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if got, _ := creds.UniverseDomain(ctx); got != internal.DefaultUniverseDomain {
		t.Errorf("got %q, want %q", got, internal.DefaultUniverseDomain)
	}
}

func TestNewCredentials_ServiceAccount_UniverseDomain(t *testing.T) {
	wantAudience := "aud"
	wantClientEmail := "gopher@fake_project.iam.gserviceaccount.com"
	wantUniverseDomain := "example.com"
	wantTok := "id-token"
	client := &http.Client{
		Transport: RoundTripFn(func(req *http.Request) *http.Response {
			defer req.Body.Close()
			b, err := io.ReadAll(req.Body)
			if err != nil {
				t.Error(err)
			}
			var r impersonate.GenerateIDTokenRequest
			if err := json.Unmarshal(b, &r); err != nil {
				t.Error(err)
			}
			if r.Audience != wantAudience {
				t.Errorf("got %q, want %q", r.Audience, wantAudience)
			}
			if r.IncludeEmail {
				t.Errorf("got %t, want %t", r.IncludeEmail, false)
			}
			if !strings.Contains(req.URL.Path, wantClientEmail) {
				t.Errorf("got %q, want %q", req.URL.Path, wantClientEmail)
			}
			if !strings.Contains(req.URL.Hostname(), wantUniverseDomain) {
				t.Errorf("got %q, want %q", req.URL.Hostname(), wantUniverseDomain)
			}
			if !strings.Contains(req.URL.Path, "generateIdToken") {
				t.Fatal("path must contain 'generateIdToken'")
			}

			resp := impersonate.GenerateIDTokenResponse{
				Token: wantTok,
			}
			b, err = json.Marshal(&resp)
			if err != nil {
				t.Fatalf("unable to marshal response: %v", err)
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(b)),
				Header:     http.Header{},
			}
		}),
	}

	ctx := context.Background()
	creds, err := NewCredentials(&Options{
		Audience:        wantAudience,
		CredentialsFile: "../../internal/testdata/sa_universe_domain.json",
		Client:          client,
		UniverseDomain:  wantUniverseDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := creds.Token(ctx)
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if got, _ := creds.UniverseDomain(ctx); got != wantUniverseDomain {
		t.Errorf("got %q, want %q", got, wantUniverseDomain)
	}
}

func TestNewCredentials_ServiceAccount_UniverseDomain_NoClient(t *testing.T) {
	wantUniverseDomain := "example.com"
	ctx := context.Background()
	creds, err := NewCredentials(&Options{
		Audience:        "aud",
		CredentialsFile: "../../internal/testdata/sa_universe_domain.json",
		UniverseDomain:  wantUniverseDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	// To test client creation and usage without a mock client, we must expect a failed token request.
	_, err = creds.Token(ctx)
	if err == nil {
		t.Fatal("token call to example.com did not fail")
	}
	// Assert that the failed token request targeted the universe domain.
	if !strings.Contains(err.Error(), wantUniverseDomain) {
		t.Errorf("got %q, want %q", err.Error(), wantUniverseDomain)
	}
	if got, _ := creds.UniverseDomain(ctx); got != wantUniverseDomain {
		t.Errorf("got %q, want %q", got, wantUniverseDomain)
	}
}

type mockTransport struct {
	handler http.HandlerFunc
}

func (m mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rw := httptest.NewRecorder()
	m.handler(rw, r)
	return rw.Result(), nil
}

func TestNewCredentials_ImpersonatedAndExternal(t *testing.T) {
	tests := []struct {
		name string
		adc  string
		file string
	}{
		{
			name: "ADC external account",
			adc:  "../../internal/testdata/exaccount_url.json",
		},
		{
			name: "CredentialsFile impersonated service account",
			file: "../../internal/testdata/imp.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantTok, _ := createRS256JWT(t)
			client := internal.DefaultClient()
			client.Transport = mockTransport{
				handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "generateAccessToken") {
						t.Errorf("unexpected call to generateAccessToken")
					}
					w.Write([]byte(fmt.Sprintf(`{"token": %q}`, wantTok)))
				}),
			}

			opts := &Options{
				Audience: "aud",
				CustomClaims: map[string]interface{}{
					"foo": "bar",
				},
				Client: client,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			if tt.file != "" {
				opts.CredentialsFile = tt.file
			} else if tt.adc != "" {
				t.Setenv(credsfile.GoogleAppCredsEnvVar, tt.adc)
			} else {
				t.Fatal("test fixture must have adc or file")
			}

			creds, err := NewCredentials(opts)
			if err != nil {
				t.Fatal(err)
			}
			tok, err := creds.Token(context.Background())
			if err != nil {
				t.Fatalf("tp.Token() = %v", err)
			}
			if tok.Value != wantTok {
				t.Errorf("got %q, want %q", tok.Value, wantTok)
			}
			// Assertions for JSON and UniverseDomain propagation
			if len(creds.JSON()) == 0 {
				t.Error("expected non-empty JSON from credentials")
			}
			ud, err := creds.UniverseDomain(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if ud != internal.DefaultUniverseDomain {
				t.Errorf("got %q, want %q", ud, internal.DefaultUniverseDomain)
			}
		})
	}
}

// TestNewCredentials_ImpersonatedAndExternal_NoClient is a regression test for
// https://github.com/googleapis/google-cloud-go/issues/19939. When the caller
// does not provide a client, the generateIdToken request must be authenticated
// with the base credentials instead of being sent through an unauthenticated
// default client.
func TestNewCredentials_ImpersonatedAndExternal_NoClient(t *testing.T) {
	wantTok, _ := createRS256JWT(t)
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/subject":
			w.Write([]byte(`{"id_token": "subject-token"}`))
		case r.URL.Path == "/sts":
			w.Write([]byte(`{"access_token": "sts-token", "issued_token_type": "urn:ietf:params:oauth:token-type:access_token", "token_type": "Bearer", "expires_in": 3600}`))
		case strings.Contains(r.URL.Path, "generateAccessToken"):
			t.Errorf("unexpected call to generateAccessToken")
		case strings.Contains(r.URL.Path, "generateIdToken"):
			gotAuth = r.Header.Get("Authorization")
			fmt.Fprintf(w, `{"token": %q}`, wantTok)
		default:
			t.Errorf("unexpected request to %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	// Route all requests, including the generateIdToken call to the
	// https://iamcredentials.googleapis.com endpoint, to the local test
	// server. Both internal.DefaultClient and the authenticated client built
	// by the impersonate package clone http.DefaultTransport, which preserves
	// the dial overrides.
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", ts.Listener.Addr().String())
	}
	origTransport := http.DefaultTransport
	http.DefaultTransport = &http.Transport{
		DialContext:    dial,
		DialTLSContext: dial,
	}
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	externalAccountJSON := fmt.Sprintf(`{
		"type": "external_account",
		"audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@fake_project.iam.gserviceaccount.com:generateAccessToken",
		"token_url": "%s/sts",
		"credential_source": {
			"url": "%s/subject",
			"format": {
				"type": "json",
				"subject_token_field_name": "id_token"
			}
		}
	}`, ts.URL, ts.URL)

	tests := []struct {
		name string
		json []byte
		// wantAuthPrefix is a prefix because the impersonated service account
		// case authenticates with a self-signed JWT that is not stable across
		// runs.
		wantAuthPrefix string
	}{
		{
			name:           "external account",
			json:           []byte(externalAccountJSON),
			wantAuthPrefix: "Bearer sts-token",
		},
		{
			name:           "impersonated service account",
			json:           readTestFile(t, "../../internal/testdata/imp.json"),
			wantAuthPrefix: "Bearer ey",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAuth = ""
			creds, err := NewCredentials(&Options{
				Audience:        "aud",
				CredentialsJSON: tt.json,
				Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatal(err)
			}
			tok, err := creds.Token(context.Background())
			if err != nil {
				t.Fatalf("creds.Token() = %v", err)
			}
			if tok.Value != wantTok {
				t.Errorf("got %q, want %q", tok.Value, wantTok)
			}
			if !strings.HasPrefix(gotAuth, tt.wantAuthPrefix) {
				t.Errorf("generateIdToken Authorization header = %q, want prefix %q", gotAuth, tt.wantAuthPrefix)
			}
		})
	}
}

func TestNewCredentials_TypeValidation(t *testing.T) {
	tests := []struct {
		name       string
		credType   credentials.CredType
		json       []byte // Use raw JSON to test NewCredentialsFromJSON
		file       string // For NewCredentialsFromFile
		wantErr    bool
		wantErrMsg string // Expected substring in the error message
	}{
		{
			name:     "ServiceAccount_FromFile_Success",
			credType: credentials.ServiceAccount,
			file:     "../../internal/testdata/sa.json",
			wantErr:  false,
		},
		{
			name:       "ServiceAccount_FromFile_Mismatch",
			credType:   credentials.ServiceAccount,
			file:       "../../internal/testdata/user.json",
			wantErr:    true,
			wantErrMsg: `credentials: expected type "service_account", found "authorized_user"`,
		},
		{
			name:       "UserCredentials_FromJSON_Unsupported",
			credType:   credentials.AuthorizedUser,
			json:       readTestFile(t, "../../internal/testdata/user.json"),
			wantErr:    true,
			wantErrMsg: "idtoken: unsupported credentials type: authorized_user",
		},
		{
			name:       "UserCredentials_FromJSON_Mismatch",
			credType:   credentials.AuthorizedUser,
			json:       readTestFile(t, "../../internal/testdata/sa.json"),
			wantErr:    true,
			wantErrMsg: `credentials: expected type "authorized_user", found "service_account"`,
		},
		{
			name:       "Error_NonExistentFile",
			credType:   credentials.ServiceAccount,
			file:       "nonexistent.json",
			wantErr:    true,
			wantErrMsg: "no such file or directory",
		},
		{
			name:       "Error_MalformedJSON",
			credType:   credentials.ServiceAccount,
			json:       []byte(`{"type": "service_account",}`), // Invalid JSON with trailing comma
			wantErr:    true,
			wantErrMsg: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			opts := &Options{Audience: "aud"}
			if tt.file != "" {
				_, err = NewCredentialsFromFile(tt.credType, tt.file, opts)
			} else {
				_, err = NewCredentialsFromJSON(tt.credType, tt.json, opts)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("test %q: got error %v, want error %v", tt.name, err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("test %q: got error message %q, want error message containing %q", tt.name, err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func readTestFile(t *testing.T, filename string) []byte {
	t.Helper()
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v", filename, err)
	}
	return b
}

func TestNewCredentials_UserCredentials_ADC_Unsupported(t *testing.T) {
	t.Setenv(credsfile.GoogleAppCredsEnvVar, "../../internal/testdata/user.json")
	opts := &Options{
		Audience: "aud",
	}
	_, err := NewCredentials(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	wantErrMsg := "idtoken: unsupported credentials type: authorized_user"
	if !strings.Contains(err.Error(), wantErrMsg) {
		t.Errorf("got error message %q, want error message containing %q", err.Error(), wantErrMsg)
	}
}
