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

package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"cloud.google.com/go/auth"
)

type redirectTestTokenProvider string

func (p redirectTestTokenProvider) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Value: string(p), Type: "Bearer"}, nil
}

// redirectHarness starts two servers that both listen on the loopback
// interface. Server A is addressed as "localhost" and server B as
// "127.0.0.1": net/http decides whether a redirect leaves the original
// domain by comparing hostnames, so a hop between them is cross-domain,
// whereas two httptest servers addressed by their URLs are both 127.0.0.1
// and count as the same host.
//
// Server A redirects /same to itself, /cross to server B, and /chain to
// server B, which sends it back to server A. Every other path records the
// Authorization header and the "key" query parameter it received.
func redirectHarness(t *testing.T) (addrA string, received func(path string) (header, key string)) {
	t.Helper()

	var mu sync.Mutex
	type seen struct{ header, key string }
	got := map[string]seen{}
	record := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		got[r.URL.Path] = seen{r.Header.Get("Authorization"), r.URL.Query().Get("key")}
	}

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bounce" {
			http.Redirect(w, r, addrA+"/back", http.StatusFound)
			return
		}
		record(w, r)
	}))
	t.Cleanup(serverB.Close)
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, addrA+"/stayed", http.StatusFound)
		case "/cross":
			http.Redirect(w, r, serverB.URL+"/landed", http.StatusFound)
		case "/chain":
			http.Redirect(w, r, serverB.URL+"/bounce", http.StatusFound)
		default:
			record(w, r)
		}
	}))
	t.Cleanup(serverA.Close)
	u, err := url.Parse(serverA.URL)
	if err != nil {
		t.Fatal(err)
	}
	addrA = "http://localhost:" + u.Port()

	return addrA, func(path string) (string, string) {
		mu.Lock()
		defer mu.Unlock()
		s := got[path]
		return s.header, s.key
	}
}

var redirectCases = []struct {
	path, final string
	kept        bool // whether the credential should reach the final request
}{
	// A redirect within the domain keeps the credential, as net/http does
	// for the Authorization header.
	{"/same", "/stayed", true},
	// A redirect to another domain gets no credential.
	{"/cross", "/landed", false},
	// A chain that left the domain gets no credential even where it comes back.
	{"/chain", "/back", false},
}

func TestAuthTransport_Redirect(t *testing.T) {
	const token = "secret"
	addrA, received := redirectHarness(t)

	// Control: a header the caller set is dropped by net/http when the
	// redirect leaves the domain. If it is not, the hop is not cross-domain
	// on this machine and the cases below would measure nothing.
	req, err := http.NewRequest(http.MethodGet, addrA+"/cross", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if h, _ := received("/landed"); h != "" {
		t.Fatalf("control: net/http sent Authorization %q to another domain; the test cannot isolate the transport", h)
	}

	client := &http.Client{Transport: &authTransport{
		creds:                        auth.NewCredentials(&auth.CredentialsOptions{TokenProvider: redirectTestTokenProvider(token)}),
		base:                         http.DefaultTransport,
		skipUniverseDomainValidation: true,
	}}
	for _, tc := range redirectCases {
		resp, err := client.Get(addrA + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		resp.Body.Close()
		want := ""
		if tc.kept {
			want = "Bearer " + token
		}
		if h, _ := received(tc.final); h != want {
			t.Errorf("GET %s: the redirect target received Authorization %q, want %q", tc.path, h, want)
		}
	}
}

func TestAPIKeyTransport_Redirect(t *testing.T) {
	const key = "secret"
	addrA, received := redirectHarness(t)

	client := &http.Client{Transport: &apiKeyTransport{Key: key, Transport: http.DefaultTransport}}
	for _, tc := range redirectCases {
		resp, err := client.Get(addrA + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		resp.Body.Close()
		want := ""
		if tc.kept {
			want = key
		}
		if _, k := received(tc.final); k != want {
			t.Errorf("GET %s: the redirect target received key %q, want %q", tc.path, k, want)
		}
	}
}

func TestRedirectedCrossDomain(t *testing.T) {
	// chain builds the request an http.Client would hand to a RoundTripper
	// after following redirects through the given URLs, in order.
	chain := func(urls ...string) *http.Request {
		var req *http.Request
		for _, s := range urls {
			u, err := url.Parse(s)
			if err != nil {
				t.Fatal(err)
			}
			next := &http.Request{URL: u, Header: http.Header{}}
			if req != nil {
				next.Response = &http.Response{Request: req}
			}
			req = next
		}
		return req
	}
	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"not a redirect", chain("https://example.com/a"), false},
		{"same host", chain("https://example.com/a", "https://example.com/b"), false},
		{"subdomain", chain("https://example.com/a", "https://api.example.com/b"), false},
		{"parent domain", chain("https://api.example.com/a", "https://example.com/b"), true},
		{"other host", chain("https://example.com/a", "https://evil.example/b"), true},
		{"suffix that is not a subdomain", chain("https://example.com/a", "https://notexample.com/b"), true},
		{"left and came back", chain("https://example.com/a", "https://evil.example/b", "https://example.com/c"), true},
	}
	for _, tt := range tests {
		if got := redirectedCrossDomain(tt.req); got != tt.want {
			t.Errorf("%s: redirectedCrossDomain = %v, want %v", tt.name, got, tt.want)
		}
	}
}
