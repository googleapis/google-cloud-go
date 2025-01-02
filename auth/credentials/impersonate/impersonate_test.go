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

package impersonate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"github.com/google/go-cmp/cmp"
)

func TestNewCredentials_serviceAccount(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name               string
		config             CredentialsOptions
		wantErr            error
		wantUniverseDomain string
	}{
		{
			name:    "missing targetPrincipal",
			wantErr: errMissingTargetPrincipal,
		},
		{
			name: "missing scopes",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			},
			wantErr: errMissingScopes,
		},
		{
			name: "lifetime over max",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Scopes:          []string{"scope"},
				Lifetime:        13 * time.Hour,
			},
			wantErr: errLifetimeOverMax,
		},
		{
			name: "works",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Scopes:          []string{"scope"},
			},
			wantErr:            nil,
			wantUniverseDomain: "googleapis.com",
		},
		{
			name: "universe domain from options",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Scopes:          []string{"scope"},
				UniverseDomain:  "example.com",
			},
			wantErr:            nil,
			wantUniverseDomain: "googleapis.com", // From creds, not CredentialsOptions.UniverseDomain
		},
		{
			name: "universe domain from options and credentials",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Scopes:          []string{"scope"},
				UniverseDomain:  "NOT.example.com",
				Credentials:     staticCredentials("example.com"),
			},
			wantErr:            nil,
			wantUniverseDomain: "example.com", // From creds, not CredentialsOptions.UniverseDomain
		},
		{
			name: "universe domain from credentials",
			config: CredentialsOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Scopes:          []string{"scope"},
				Credentials:     staticCredentials("example.com"),
			},
			wantErr:            nil,
			wantUniverseDomain: "example.com",
		},
	}

	for _, tt := range tests {
		name := tt.name
		t.Run(name, func(t *testing.T) {
			saTok := "sa-token"
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					if !strings.Contains(req.URL.Path, "generateAccessToken") {
						t.Fatal("path must contain 'generateAccessToken'")
					}
					defer req.Body.Close()
					b, err := io.ReadAll(req.Body)
					if err != nil {
						t.Error(err)
					}
					var r generateAccessTokenRequest
					if err := json.Unmarshal(b, &r); err != nil {
						t.Error(err)
					}
					if !cmp.Equal(r.Scope, tt.config.Scopes) {
						t.Errorf("got %v, want %v", r.Scope, tt.config.Scopes)
					}
					if !strings.Contains(req.URL.Path, tt.config.TargetPrincipal) {
						t.Errorf("got %q, want %q", req.URL.Path, tt.config.TargetPrincipal)
					}
					if !strings.Contains(req.URL.Hostname(), tt.wantUniverseDomain) {
						t.Errorf("got %q, want %q", req.URL.Hostname(), tt.wantUniverseDomain)
					}

					resp := generateAccessTokenResponse{
						AccessToken: saTok,
						ExpireTime:  time.Now().Format(time.RFC3339),
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
			if tt.config.Credentials == nil {
				tt.config.Client = client
			}
			creds, err := NewCredentials(&tt.config)
			if err != nil {
				if err != tt.wantErr {
					t.Fatalf("err: %v", err)
				}
			} else if tt.config.Credentials != nil {
				// config.Credentials is invalid for Token request, just assert universe domain.
				if got, _ := creds.UniverseDomain(ctx); got != tt.wantUniverseDomain {
					t.Errorf("got %q, want %q", got, tt.wantUniverseDomain)
				}
			} else {
				tok, err := creds.Token(ctx)
				if err != nil {
					t.Error(err)
				}
				if tok.Value != saTok {
					t.Errorf("got %q, want %q", tok.Value, saTok)
				}
				if got, _ := creds.UniverseDomain(ctx); got != tt.wantUniverseDomain {
					t.Errorf("got %q, want %q", got, tt.wantUniverseDomain)
				}
			}
		})
	}
}

type RoundTripFn func(req *http.Request) *http.Response

func (f RoundTripFn) RoundTrip(req *http.Request) (*http.Response, error) { return f(req), nil }

func staticCredentials(universeDomain string) *auth.Credentials {
	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider:          staticTokenProvider("base credentials Token should never be called"),
		UniverseDomainProvider: internal.StaticCredentialsProperty(universeDomain),
	})
}

type staticTokenProvider string

func (s staticTokenProvider) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: string(s)}, nil
}
