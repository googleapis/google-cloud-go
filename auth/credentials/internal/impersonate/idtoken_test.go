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

package impersonate

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/jwt"
)

func TestIDTokenIAMOptions_Expiry(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const wantExp = 1893456000
	tokStr, err := jwt.EncodeJWS(
		&jwt.Header{Algorithm: jwt.HeaderAlgRSA256, Type: jwt.HeaderType},
		&jwt.Claims{Iss: "example.com", Aud: "aud", Iat: wantExp - 3600, Exp: wantExp},
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(GenerateIDTokenResponse{Token: tokStr}); err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	client := internal.DefaultClient()
	client.Transport = rewriteURLTransport{base: ts.URL, rt: client.Transport}

	opts := IDTokenIAMOptions{
		Client:              client,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		UniverseDomain:      internal.StaticCredentialsProperty("googleapis.com"),
		ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		GenerateIDTokenRequest: GenerateIDTokenRequest{
			Audience: "aud",
		},
	}
	tok, err := opts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if tok.Value != tokStr {
		t.Errorf("tok.Value = %q, want %q", tok.Value, tokStr)
	}
	if want := time.Unix(wantExp, 0); !tok.Expiry.Equal(want) {
		t.Errorf("tok.Expiry = %v, want %v", tok.Expiry, want)
	}
}

func TestIDTokenIAMOptions_MissingExp(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"aud":"aud"}`))
	tokStr := header + "." + claims + ".c2ln"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(GenerateIDTokenResponse{Token: tokStr}); err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	client := internal.DefaultClient()
	client.Transport = rewriteURLTransport{base: ts.URL, rt: client.Transport}

	opts := IDTokenIAMOptions{
		Client:              client,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		UniverseDomain:      internal.StaticCredentialsProperty("googleapis.com"),
		ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		GenerateIDTokenRequest: GenerateIDTokenRequest{
			Audience: "aud",
		},
	}
	tok, err := opts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if !tok.Expiry.IsZero() {
		t.Errorf("tok.Expiry = %v, want zero time", tok.Expiry)
	}
}

func TestIDTokenIAMOptions_MalformedJWT(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(GenerateIDTokenResponse{Token: "not-a-jwt"}); err != nil {
			t.Fatal(err)
		}
	}))
	defer ts.Close()

	client := internal.DefaultClient()
	client.Transport = rewriteURLTransport{base: ts.URL, rt: client.Transport}

	opts := IDTokenIAMOptions{
		Client:              client,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		UniverseDomain:      internal.StaticCredentialsProperty("googleapis.com"),
		ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		GenerateIDTokenRequest: GenerateIDTokenRequest{
			Audience: "aud",
		},
	}
	if _, err := opts.Token(context.Background()); err == nil {
		t.Fatal("Token() = nil, want error for malformed JWT")
	}
}

type rewriteURLTransport struct {
	base string
	rt   http.RoundTripper
}

func (t rewriteURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	target := strings.TrimRight(t.base, "/") + req.URL.Path
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, target, clone.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = clone.Header
	return t.rt.RoundTrip(newReq)
}
