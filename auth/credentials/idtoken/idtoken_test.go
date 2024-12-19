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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestNewCredentials_ServiceAccount_NoClient(t *testing.T) {
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
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
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
					w.Write([]byte(fmt.Sprintf(`{"token": %q}`, wantTok)))
				}),
			}

			opts := &Options{
				Audience: "aud",
				CustomClaims: map[string]interface{}{
					"foo": "bar",
				},
				Client: client,
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
		})
	}
}
