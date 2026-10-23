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

// TestAuthTransport_CrossHostRedirectDoesNotLeakToken verifies that the
// authTransport does not forward bearer tokens to a different host when
// following a redirect. An attacker-controlled redirect (e.g. via an open
// redirect on the target API) must not receive the caller's credentials.
func TestAuthTransport_CrossHostRedirectDoesNotLeakToken(t *testing.T) {
	const token = "super-secret-token"

	// victim receives requests and records whether the auth header was present.
	var victimSawToken bool
	victim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			victimSawToken = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer victim.Close()

	// redirector redirects all requests to the victim (different host).
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, victim.URL+"/", http.StatusFound)
	}))
	defer redirector.Close()

	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: staticTP(token),
	})
	client := &http.Client{
		Transport: &authTransport{
			creds: creds,
			base:  http.DefaultTransport,
		},
	}

	resp, err := client.Get(redirector.URL + "/resource")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if victimSawToken {
		t.Error("bearer token was leaked to cross-host redirect destination")
	}
}

// TestAuthTransport_SameHostRedirectIsAuthorized verifies that same-host
// redirects continue to carry the bearer token.
func TestAuthTransport_SameHostRedirectIsAuthorized(t *testing.T) {
	const token = "super-secret-token"

	var finalSawToken bool
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+token {
			finalSawToken = true
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
	})

	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: staticTP(token),
	})
	client := &http.Client{
		Transport: &authTransport{
			creds: creds,
			base:  http.DefaultTransport,
		},
	}

	resp, err := client.Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if !finalSawToken {
		t.Error("bearer token was not forwarded to same-host redirect destination")
	}
}
