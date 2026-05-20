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

	"cloud.google.com/go/auth/credentials/internal/impersonate"
)

func TestNewIDTokenCredentials(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name               string
		config             IDTokenOptions
		wantErr            bool
		wantUniverseDomain string
		setup              func(*testing.T)
		skipTokenCall      bool
	}{
		{
			name: "missing aud",
			config: IDTokenOptions{
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			},
			wantErr: true,
		},
		{
			name: "missing targetPrincipal",
			config: IDTokenOptions{
				Audience: "http://example.com/",
			},
			wantErr: true,
		},
		{
			name: "works",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			},
			wantUniverseDomain: "googleapis.com",
		},
		{
			name: "universe domain from options",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				UniverseDomain:  "example.com",
			},
			wantUniverseDomain: "googleapis.com", // From creds, not IDTokenOptions.UniverseDomain
		},
		{
			name: "universe domain from options and credentials",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				UniverseDomain:  "NOT.example.com",
				Credentials:     staticCredentials("example.com"),
			},
			wantUniverseDomain: "example.com", // From creds, not IDTokenOptions.UniverseDomain
		},
		{
			name: "universe domain from credentials",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				Credentials:     staticCredentials("example.com"),
			},
			wantUniverseDomain: "example.com",
		},
		{
			name: "universe domain from env var (detected)",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			},
			setup: func(t *testing.T) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "../../internal/testdata/sa.json")
				t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "env-universe.com")
			},
			wantUniverseDomain: "env-universe.com",
			skipTokenCall:      true,
		},
		{
			name: "universe domain from options (detected override)",
			config: IDTokenOptions{
				Audience:        "http://example.com/",
				TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
				UniverseDomain:  "options-universe.com",
			},
			setup: func(t *testing.T) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "../../internal/testdata/sa.json")
				t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "env-universe.com")
			},
			wantUniverseDomain: "options-universe.com",
			skipTokenCall:      true,
		},
	}

	for _, tt := range tests {
		name := tt.name
		t.Run(name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			idTok := "id-token"
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
					if r.Audience != tt.config.Audience {
						t.Errorf("got %q, want %q", r.Audience, tt.config.Audience)
					}
					if !strings.Contains(req.URL.Path, tt.config.TargetPrincipal) {
						t.Errorf("got %q, want %q", req.URL.Path, tt.config.TargetPrincipal)
					}
					if !strings.Contains(req.URL.Hostname(), tt.wantUniverseDomain) {
						t.Errorf("got %q, want %q", req.URL.Hostname(), tt.wantUniverseDomain)
					}
					if !strings.Contains(req.URL.Path, "generateIdToken") {
						t.Error("path must contain 'generateIdToken'")
					}

					resp := impersonate.GenerateIDTokenResponse{
						Token: idTok,
					}
					b, err = json.Marshal(&resp)
					if err != nil {
						t.Fatalf("unable to marshal response: %v", err)
					}
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader(b)),
						Header:     make(http.Header),
					}
				}),
			}
			if tt.config.Credentials == nil && !tt.skipTokenCall {
				tt.config.Client = client
			}
			creds, err := NewIDTokenCredentials(&tt.config)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("err: %v", err)
				}
				return
			}
			// Static config.Credentials is invalid for Token request, skip.
			if tt.config.Credentials == nil && !tt.skipTokenCall {
				tok, err := creds.Token(ctx)
				if err != nil {
					t.Error(err)
				}
				if tok.Value != idTok {
					t.Errorf("got %q, want %q", tok.Value, idTok)
				}
			}
			if got, _ := creds.UniverseDomain(ctx); got != tt.wantUniverseDomain {
				t.Errorf("got %q, want %q", got, tt.wantUniverseDomain)
			}
		})
	}
}
