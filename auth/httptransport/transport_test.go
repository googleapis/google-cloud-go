// Copyright 2024 Google LLC
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
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

func TestAuthTransport_GetClientUniverseDomain(t *testing.T) {
	nonDefault := "example.com"
	nonDefault2 := "other-example.com"
	tests := []struct {
		name                 string
		clientUniverseDomain string
		envUniverseDomain    string
		want                 string
	}{
		{
			name:                 "default",
			clientUniverseDomain: "",
			want:                 internal.DefaultUniverseDomain,
		},
		{
			name:                 "client option",
			clientUniverseDomain: nonDefault,
			want:                 nonDefault,
		},
		{
			name:                 "env var",
			clientUniverseDomain: "",
			envUniverseDomain:    nonDefault2,
			want:                 nonDefault2,
		},
		{
			name:                 "client option and env var",
			clientUniverseDomain: nonDefault,
			envUniverseDomain:    nonDefault2,
			want:                 nonDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envUniverseDomain != "" {
				t.Setenv(internal.UniverseDomainEnvVar, tt.envUniverseDomain)
			}
			at := &authTransport{clientUniverseDomain: tt.clientUniverseDomain}
			got := at.getClientUniverseDomain()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransports_RedirectCredentials(t *testing.T) {
	const token = "secret-token"
	const apiKey = "secret-key"
	var got []bool // whether each hop carried a credential
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization") != "" || r.URL.Query().Get("key") != "")
		if next := r.URL.Query().Get("next"); next != "" {
			http.Redirect(w, r, next, http.StatusFound)
		}
	})
	s1 := httptest.NewServer(h)
	defer s1.Close()
	s2 := httptest.NewServer(h)
	defer s2.Close()
	origin := s1.URL
	otherPort := s2.URL
	otherHost := strings.Replace(s2.URL, "127.0.0.1", "localhost", 1)
	chain := func(urls ...string) string {
		u := urls[len(urls)-1] + "/"
		for i := len(urls) - 2; i >= 0; i-- {
			u = urls[i] + "/?next=" + url.QueryEscape(u)
		}
		return u
	}

	transports := map[string]http.RoundTripper{
		"authTransport": &authTransport{
			creds: auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: staticTP(token)}),
			base:  http.DefaultTransport,
		},
		"apiKeyTransport": &apiKeyTransport{Key: apiKey, Transport: http.DefaultTransport},
	}
	tests := []struct {
		name string
		urls []string
		want []bool
	}{
		{"same host", []string{origin, origin}, []bool{true, true}},
		{"same hostname, other port", []string{origin, otherPort}, []bool{true, false}},
		{"other host", []string{origin, otherHost}, []bool{true, false}},
		{"other host redirects to itself", []string{origin, otherHost, otherHost}, []bool{true, false, false}},
		{"back to original host", []string{origin, otherHost, origin}, []bool{true, false, false}},
	}
	for name, tr := range transports {
		for _, tt := range tests {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				got = nil
				resp, err := (&http.Client{Transport: tr}).Get(chain(tt.urls...))
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if !slices.Equal(got, tt.want) {
					t.Errorf("credential sent per hop = %v, want %v", got, tt.want)
				}
			})
		}
	}
}

func TestAPIKeyTransport_DoesNotModifyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	req, err := http.NewRequest("GET", srv.URL+"/?a=b", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&apiKeyTransport{Key: "secret-key", Transport: http.DefaultTransport}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := req.URL.RawQuery; got != "a=b" {
		t.Errorf("RoundTrip changed the caller's URL query to %q", got)
	}
}

func TestRedirectedOffOrigin(t *testing.T) {
	tests := []struct {
		chain []string
		want  bool
	}{
		{[]string{"https://a.com/x"}, false},
		{[]string{"https://a.com/x", "https://a.com/y"}, false},
		{[]string{"https://A.com/x", "https://a.com:443/y"}, false},
		{[]string{"https://a.com/x", "http://a.com/y"}, true},
		{[]string{"https://a.com/x", "https://a.com:8443/y"}, true},
		{[]string{"https://a.com/x", "https://sub.a.com/y"}, true},
		{[]string{"https://a.com/x", "https://b.com/y", "https://b.com/z"}, true},
		{[]string{"https://a.com/x", "https://b.com/y", "https://a.com/z"}, true},
	}
	for _, tt := range tests {
		var req *http.Request
		for _, u := range tt.chain {
			next := httptest.NewRequest("GET", u, nil)
			if req != nil {
				next.Response = &http.Response{Request: req}
			}
			req = next
		}
		if got := redirectedOffOrigin(req); got != tt.want {
			t.Errorf("redirectedOffOrigin(%v) = %v, want %v", tt.chain, got, tt.want)
		}
	}

	req := httptest.NewRequest("GET", "https://a.com/y", nil)
	req.Response = &http.Response{}
	if !redirectedOffOrigin(req) {
		t.Error("redirectedOffOrigin = false for a redirect with no previous request, want true")
	}
}
