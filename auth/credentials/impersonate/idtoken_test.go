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
)

func TestNewIDTokenCredentials(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name               string
		config             IDTokenOptions
		wantErr            bool
		wantUniverseDomain string
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
	}

	for _, tt := range tests {
		name := tt.name
		t.Run(name, func(t *testing.T) {
			idTok := "id-token"
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					defer req.Body.Close()
					b, err := io.ReadAll(req.Body)
					if err != nil {
						t.Error(err)
					}
					var r generateIDTokenRequest
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

					resp := generateIDTokenResponse{
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
			if tt.config.Credentials == nil {
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
			if tt.config.Credentials == nil {
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
