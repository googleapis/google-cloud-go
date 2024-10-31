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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/credsfile"
)

func TestNewCredentials_ServiceAccount(t *testing.T) {
	ctx := context.Background()
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
	tok, err := creds.Token(ctx)
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if got, _ := creds.UniverseDomain(ctx); got != internal.DefaultUniverseDomain {
		t.Errorf("got %q, want %q", got, internal.DefaultUniverseDomain)
	}
}

func TestNewCredentials_ServiceAccount_UniverseDomain(t *testing.T) {
	wantAudience := "aud"
	wantClientEmail := "gopher@fake_project.iam.gserviceaccount.com"
	wantUniverseDomain := "example.com"
	wantTok := "id-token"
	client := &http.Client{
		Transport: RoundTripFn(func(req *http.Request) *http.Response {
			defer req.Body.Close()
			b, err := io.ReadAll(req.Body)
			if err != nil {
				t.Error(err)
			}
			var r generateIAMIDTokenRequest
			if err := json.Unmarshal(b, &r); err != nil {
				t.Error(err)
			}
			if r.Audience != wantAudience {
				t.Errorf("got %q, want %q", r.Audience, wantAudience)
			}
			if !r.IncludeEmail {
				t.Errorf("got %t, want %t", r.IncludeEmail, false)
			}
			if !strings.Contains(req.URL.Path, wantClientEmail) {
				t.Errorf("got %q, want %q", req.URL.Path, wantClientEmail)
			}
			if !strings.Contains(req.URL.Hostname(), wantUniverseDomain) {
				t.Errorf("got %q, want %q", req.URL.Hostname(), wantUniverseDomain)
			}
			if !strings.Contains(req.URL.Path, "generateIdToken") {
				t.Fatal("path must contain 'generateIdToken'")
			}

			resp := generateIAMIDTokenResponse{
				Token: wantTok,
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

	ctx := context.Background()
	creds, err := NewCredentials(&Options{
		Audience:        wantAudience,
		CredentialsFile: "../../internal/testdata/sa_universe_domain.json",
		Client:          client,
		UniverseDomain:  wantUniverseDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := creds.Token(ctx)
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if got, _ := creds.UniverseDomain(ctx); got != wantUniverseDomain {
		t.Errorf("got %q, want %q", got, wantUniverseDomain)
	}
}

func TestNewCredentials_ServiceAccount_UniverseDomain_NoClient(t *testing.T) {
	wantUniverseDomain := "example.com"
	ctx := context.Background()
	creds, err := NewCredentials(&Options{
		Audience:        "aud",
		CredentialsFile: "../../internal/testdata/sa_universe_domain.json",
		UniverseDomain:  wantUniverseDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	// To test client creation and usage without a mock client, we must expect a failed token request.
	_, err = creds.Token(ctx)
	if err == nil {
		t.Fatal("token call to example.com did not fail")
	}
	// Assert that the failed token request targeted the universe domain.
	if !strings.Contains(err.Error(), wantUniverseDomain) {
		t.Errorf("got %q, want %q", err.Error(), wantUniverseDomain)
	}
	if got, _ := creds.UniverseDomain(ctx); got != wantUniverseDomain {
		t.Errorf("got %q, want %q", got, wantUniverseDomain)
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

func TestNewCredentials_ImpersonatedServiceAccount(t *testing.T) {
	wantTok, _ := createRS256JWT(t)
	client := internal.DefaultClient()
	client.Transport = mockTransport{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(fmt.Sprintf(`{"token": %q}`, wantTok)))
		}),
	}
	creds, err := NewCredentials(&Options{
		Audience:        "aud",
		CredentialsFile: "../../internal/testdata/imp.json",
		CustomClaims: map[string]interface{}{
			"foo": "bar",
		},
		Client: client,
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
