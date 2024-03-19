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

	"github.com/google/go-cmp/cmp"
)

func TestNewCredentials_serviceAccount(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name            string
		targetPrincipal string
		scopes          []string
		lifetime        time.Duration
		wantErr         bool
	}{
		{
			name:    "missing targetPrincipal",
			wantErr: true,
		},
		{
			name:            "missing scopes",
			targetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			wantErr:         true,
		},
		{
			name:            "lifetime over max",
			targetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			scopes:          []string{"scope"},
			lifetime:        13 * time.Hour,
			wantErr:         true,
		},
		{
			name:            "works",
			targetPrincipal: "foo@project-id.iam.gserviceaccount.com",
			scopes:          []string{"scope"},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		name := tt.name
		t.Run(name, func(t *testing.T) {
			saTok := "sa-token"
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					if strings.Contains(req.URL.Path, "generateAccessToken") {
						defer req.Body.Close()
						b, err := io.ReadAll(req.Body)
						if err != nil {
							t.Error(err)
						}
						var r generateAccessTokenRequest
						if err := json.Unmarshal(b, &r); err != nil {
							t.Error(err)
						}
						if !cmp.Equal(r.Scope, tt.scopes) {
							t.Errorf("got %v, want %v", r.Scope, tt.scopes)
						}
						if !strings.Contains(req.URL.Path, tt.targetPrincipal) {
							t.Errorf("got %q, want %q", req.URL.Path, tt.targetPrincipal)
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
					}
					return nil
				}),
			}
			ts, err := NewCredentials(&CredentialsOptions{
				TargetPrincipal: tt.targetPrincipal,
				Scopes:          tt.scopes,
				Lifetime:        tt.lifetime,
				Client:          client,
			})
			if tt.wantErr && err != nil {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tok, err := ts.Token(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tok.Value != saTok {
				t.Fatalf("got %q, want %q", tok.Value, saTok)
			}
		})
	}
}

type RoundTripFn func(req *http.Request) *http.Response

func (f RoundTripFn) RoundTrip(req *http.Request) (*http.Response, error) { return f(req), nil }
