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

package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect"
	"cloud.google.com/go/auth/internal"
	"github.com/google/go-cmp/cmp"
)

func TestAddAuthorizationMiddleware(t *testing.T) {
	tp := staticTP("fakeToken")
	tests := []struct {
		name    string
		client  *http.Client
		tp      auth.TokenProvider
		wantErr bool
		want    string
	}{
		{
			name:    "missing both required fields",
			wantErr: true,
		},
		{
			name:    "missing client field",
			tp:      tp,
			wantErr: true,
		},
		{
			name:    "missing TokenProvider field",
			client:  internal.CloneDefaultClient(),
			wantErr: true,
		},
		{
			name:   "works",
			client: internal.CloneDefaultClient(),
			tp:     tp,
			want:   "fakeToken",
		},
		{
			name:   "works, no transport",
			client: &http.Client{},
			tp:     tp,
			want:   "fakeToken",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AddAuthorizationMiddleware(tt.client, tt.tp)
			if tt.wantErr && err == nil {
				t.Fatalf("AddAuthorizationMiddleware() = nil, want error")
			}
			if tt.wantErr {
				return
			}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got := r.Header.Get("Authorization")
				if !strings.Contains(got, tt.want) {
					t.Errorf("got %q, want contain %q", got, tt.want)
				}

			}))
			defer ts.Close()
			tt.client.Get(ts.URL)
		})
	}
}

func TestNewClient_FailsValidation(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
	}{
		{
			name: "missing options",
		},
		{
			name: "has creds with disable options, tp",
			opts: &Options{
				DisableAuthentication: true,
				TokenProvider:         staticTP("fakeToken"),
			},
		},
		{
			name: "has creds with disable options, cred file",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &detect.Options{
					CredentialsFile: "abc.123",
				},
			},
		},
		{
			name: "has creds with disable options, cred json",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &detect.Options{
					CredentialsJSON: []byte(`{"foo":"bar"}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.opts)
			if err == nil {
				t.Fatal("NewClient() = _, nil, want error")
			}
		})
	}
}

func TestOptions_ResolveDetectOptions(t *testing.T) {
	tests := []struct {
		name string
		in   *Options
		want *detect.Options
	}{
		{
			name: "base",
			in: &Options{
				DetectOpts: &detect.Options{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"scope"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "self-signed, with scope",
			in: &Options{
				InternalOptions: &InternalOptions{
					EnableJWTWithScope: true,
				},
				DetectOpts: &detect.Options{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:           []string{"scope"},
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "self-signed, with aud",
			in: &Options{
				DetectOpts: &detect.Options{
					Audience:        "aud",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:         "aud",
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "use default scopes",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"default"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "don't use default scopes, scope provided",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					Scopes:          []string{"non-default"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"non-default"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "don't use default scopes, aud provided",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					Audience:        "non-default",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:         "non-default",
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "use default aud",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:        "default",
				CredentialsFile: "/path/to/a/file",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.resolveDetectOptions()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewClient_DetectedServiceAccount(t *testing.T) {
	testQuota := "testquota"
	wantHeader := "bar"
	t.Setenv("GOOGLE_CLOUD_QUOTA_PROJECT", testQuota)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "" {
			t.Errorf(`got "", want an auth token`)
		}
		if got := r.Header.Get("Foo"); got != wantHeader {
			t.Errorf("got %q, want %q", got, wantHeader)
		}
		if got := r.Header.Get(quotaProjectHeaderKey); got != testQuota {
			t.Errorf("got %q, want %q", got, testQuota)
		}
	}))
	defer ts.Close()
	client, err := NewClient(&Options{
		Headers: http.Header{"Foo": []string{wantHeader}},
		InternalOptions: &InternalOptions{
			DefaultEndpoint: ts.URL,
		},
		DetectOpts: &detect.Options{
			Audience:         ts.URL,
			CredentialsFile:  "../internal/testdata/sa.json",
			UseSelfSignedJWT: true,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(req); err != nil {
		t.Fatalf("client.Get() = %v", err)
	}
}

func TestNewClient_APIKey(t *testing.T) {
	testQuota := "testquota"
	apiKey := "thereisnospoon"
	wantHeader := "bar"
	t.Setenv("GOOGLE_CLOUD_QUOTA_PROJECT", testQuota)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("key")
		if got != apiKey {
			t.Errorf("got %q, want %q", got, apiKey)
		}
		if got := r.Header.Get("Foo"); got != wantHeader {
			t.Errorf("got %q, want %q", got, wantHeader)
		}
		if got := r.Header.Get(quotaProjectHeaderKey); got != testQuota {
			t.Errorf("got %q, want %q", got, testQuota)
		}
	}))
	defer ts.Close()
	client, err := NewClient(&Options{
		APIKey:  apiKey,
		Headers: http.Header{"Foo": []string{wantHeader}},
	})
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	if _, err := client.Get(ts.URL); err != nil {
		t.Fatalf("client.Get() = %v", err)
	}
}

type staticTP string

func (tp staticTP) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{
		Value: string(tp),
	}, nil
}
