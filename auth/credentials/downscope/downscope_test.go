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

package downscope

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
)

var (
	standardReqBody  = "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange&options=%7B%22accessBoundary%22%3A%7B%22accessBoundaryRules%22%3A%5B%7B%22availableResource%22%3A%22test1%22%2C%22availablePermissions%22%3A%5B%22Perm1%22%2C%22Perm2%22%5D%7D%5D%7D%7D&requested_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token&subject_token=token_base&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token"
	standardRespBody = `{"access_token":"fake_token","expires_in":42,"token_type":"Bearer"}`
)

func staticCredentials(tok string) *auth.Credentials {
	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: staticTokenProvider(tok),
	})
}

type staticTokenProvider string

func (s staticTokenProvider) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: string(s)}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testRules() []AccessBoundaryRule {
	return []AccessBoundaryRule{
		{
			AvailableResource:    "test1",
			AvailablePermissions: []string{"Perm1", "Perm2"},
		},
	}
}

func TestNewCredentials(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Errorf("request method = %q, want %q", r.Method, http.MethodPost)
			}
			if got, want := r.URL.String(), "https://sts.googleapis.com/v1/token"; got != want {
				t.Errorf("request URL = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("io.ReadAll(r.Body) unexpected error: %v", err)
			}
			if got, want := string(body), standardReqBody; got != want {
				t.Errorf("exchange payload = %q, want %q", got, want)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(standardRespBody)),
			}, nil
		}),
	}
	creds, err := NewCredentials(&Options{
		Credentials: staticCredentials("token_base"),
		Rules:       testRules(),
		Client:      client,
	})
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error: %v", err)
	}

	tok, err := creds.Token(t.Context())
	if err != nil {
		t.Fatalf("creds.Token() unexpected error: %v", err)
	}
	if got, want := tok.Value, "fake_token"; got != want {
		t.Errorf("tok.Value = %q, want %q", got, want)
	}
	ud, err := creds.UniverseDomain(t.Context())
	if err != nil {
		t.Fatalf("creds.UniverseDomain() unexpected error: %v", err)
	}
	if got, want := ud, "googleapis.com"; got != want {
		t.Errorf("creds.UniverseDomain() = %q, want %q", got, want)
	}
}

func TestNewCredentials_Validations(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
	}{
		{
			name: "no opts",
			opts: nil,
		},
		{
			name: "no provider",
			opts: &Options{},
		},
		{
			name: "no rules",
			opts: &Options{
				Credentials: staticCredentials("token_base"),
			},
		},
		{
			name: "too many rules",
			opts: &Options{
				Credentials: staticCredentials("token_base"),
				Rules:       []AccessBoundaryRule{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
			},
		},
		{
			name: "no resource",
			opts: &Options{
				Credentials: staticCredentials("token_base"),
				Rules:       []AccessBoundaryRule{{}},
			},
		},
		{
			name: "no perm",
			opts: &Options{
				Credentials: staticCredentials("token_base"),
				Rules: []AccessBoundaryRule{{
					AvailableResource: "resource",
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCredentials(test.opts); err == nil {
				t.Fatal("want non-nil err")
			}
		})
	}
}

func TestNewCredentials_UniverseDomain(t *testing.T) {
	type ctxKey struct{}
	tests := []struct {
		name               string
		sourceUDP          auth.CredentialsPropertyProvider
		optsUniverseDomain string
		ctxVal             string
		wantUniverseDomain string
		wantEndpoint       string
	}{
		{
			name:               "default_universe_domain",
			wantUniverseDomain: "googleapis.com",
			wantEndpoint:       "https://sts.googleapis.com/v1/token",
		},
		{
			name: "inherited_from_source_credentials",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "example.com", nil
			}),
			wantUniverseDomain: "example.com",
			wantEndpoint:       "https://sts.example.com/v1/token",
		},
		{
			name: "matching_explicit_universe_domain",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "example.com", nil
			}),
			optsUniverseDomain: "example.com",
			wantUniverseDomain: "example.com",
			wantEndpoint:       "https://sts.example.com/v1/token",
		},
		{
			name:               "matching_explicit_default_universe_domain",
			optsUniverseDomain: "googleapis.com",
			wantUniverseDomain: "googleapis.com",
			wantEndpoint:       "https://sts.googleapis.com/v1/token",
		},
		{
			name: "dynamic_source_universe_domain_propagates_context",
			sourceUDP: auth.CredentialsPropertyFunc(func(ctx context.Context) (string, error) {
				v, _ := ctx.Value(ctxKey{}).(string)
				return v, nil
			}),
			ctxVal:             "dynamic.example.com",
			wantUniverseDomain: "dynamic.example.com",
			wantEndpoint:       "https://sts.dynamic.example.com/v1/token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			if tc.ctxVal != "" {
				ctx = context.WithValue(ctx, ctxKey{}, tc.ctxVal)
			}
			var gotURL string
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					gotURL = r.URL.String()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"application/json"}},
						Body:       io.NopCloser(strings.NewReader(standardRespBody)),
					}, nil
				}),
			}
			sourceCreds := auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          staticTokenProvider("token_base"),
				UniverseDomainProvider: tc.sourceUDP,
			})
			opts := &Options{
				Credentials:    sourceCreds,
				Rules:          testRules(),
				Client:         client,
				UniverseDomain: tc.optsUniverseDomain,
			}
			creds, err := NewCredentials(opts)
			if err != nil {
				t.Fatalf("NewCredentials() unexpected error: %v", err)
			}

			gotUD, err := creds.UniverseDomain(ctx)
			if err != nil {
				t.Fatalf("creds.UniverseDomain() unexpected error: %v", err)
			}
			if gotUD != tc.wantUniverseDomain {
				t.Errorf("creds.UniverseDomain() = %q, want %q", gotUD, tc.wantUniverseDomain)
			}

			tok, err := creds.Token(ctx)
			if err != nil {
				t.Fatalf("creds.Token() unexpected error: %v", err)
			}
			if got, want := tok.Value, "fake_token"; got != want {
				t.Errorf("tok.Value = %q, want %q", got, want)
			}
			if gotURL != tc.wantEndpoint {
				t.Errorf("STS request URL = %q, want %q", gotURL, tc.wantEndpoint)
			}
		})
	}
}

type spyTokenProvider struct {
	called bool
}

func (s *spyTokenProvider) Token(context.Context) (*auth.Token, error) {
	s.called = true
	return &auth.Token{Value: "root_secret_token"}, nil
}

func TestNewCredentials_UniverseDomainErrors(t *testing.T) {
	errMetadata := errors.New("metadata server unavailable")
	tests := []struct {
		name               string
		sourceUDP          auth.CredentialsPropertyProvider
		optsUniverseDomain string
		wantErrIs          error
	}{
		{
			name: "mismatch_tpc_source_and_gdu_option",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "tpc.example.com", nil
			}),
			optsUniverseDomain: "googleapis.com",
		},
		{
			name: "mismatch_tpc_source_and_other_tpc_option",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "tpc-a.example.com", nil
			}),
			optsUniverseDomain: "tpc-b.example.com",
		},
		{
			name:               "mismatch_gdu_source_and_tpc_option",
			optsUniverseDomain: "tpc.example.com",
		},
		{
			name: "source_universe_domain_error",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "", errMetadata
			}),
			wantErrIs: errMetadata,
		},
		{
			name: "source_universe_domain_error_with_explicit_option",
			sourceUDP: auth.CredentialsPropertyFunc(func(context.Context) (string, error) {
				return "", errMetadata
			}),
			optsUniverseDomain: "tpc.example.com",
			wantErrIs:          errMetadata,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyTokenProvider{}
			httpCalled := false
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					httpCalled = true
					return nil, errors.New("unexpected HTTP call")
				}),
			}
			sourceCreds := auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          spy,
				UniverseDomainProvider: tc.sourceUDP,
			})
			creds, err := NewCredentials(&Options{
				Credentials:    sourceCreds,
				Rules:          testRules(),
				Client:         client,
				UniverseDomain: tc.optsUniverseDomain,
			})
			if err != nil {
				t.Fatalf("NewCredentials() unexpected error: %v", err)
			}

			_, err = creds.Token(t.Context())
			if err == nil {
				t.Fatal("creds.Token() = nil, want non-nil error")
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Errorf("creds.Token() error = %v, want errors.Is(%v)", err, tc.wantErrIs)
			}
			if spy.called {
				t.Error("source Credentials.Token() was called, want no root token fetch on universe domain failure")
			}
			if httpCalled {
				t.Error("HTTP request was sent to STS, want no network call on universe domain failure")
			}
		})
	}
}
