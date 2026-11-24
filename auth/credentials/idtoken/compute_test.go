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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const metadataHostEnv = "GCE_METADATA_HOST"

func TestComputeCredentials(t *testing.T) {
	wantTok, _ := createRS256JWT(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Metadata-Flavor"), "Google"; got != want {
			t.Errorf("Metadata-Flavor header: got %q, want %q", got, want)
		}
		if !strings.Contains(r.URL.Path, identitySuffix) {
			t.Errorf("got %q, want contains %q", r.URL.Path, identitySuffix)
		}
		if got, want := r.URL.Query().Get("audience"), "aud"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("format"), "full"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("licenses"), "TRUE"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		w.Write([]byte(wantTok))
	}))
	defer ts.Close()
	t.Setenv(metadataHostEnv, strings.TrimPrefix(ts.URL, "http://"))
	tp, err := computeCredentials(&Options{
		Audience:           "aud",
		ComputeTokenFormat: ComputeTokenFormatFullWithLicense,
	})
	if err != nil {
		t.Fatalf("computeCredentials() = %v", err)
	}
	tok, err := tp.Token(context.Background())
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if wantExp := time.Unix(expiry, 0); !tok.Expiry.Equal(wantExp) {
		t.Errorf("tok.Expiry = %v, want %v", tok.Expiry, wantExp)
	}
}

func TestComputeCredentials_Standard(t *testing.T) {
	wantTok, _ := createRS256JWT(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Metadata-Flavor"), "Google"; got != want {
			t.Errorf("Metadata-Flavor header: got %q, want %q", got, want)
		}
		if !strings.Contains(r.URL.Path, identitySuffix) {
			t.Errorf("got %q, want contains %q", r.URL.Path, identitySuffix)
		}
		if got, want := r.URL.Query().Get("audience"), "aud"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("format"), ""; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("licenses"), ""; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		w.Write([]byte(wantTok))
	}))
	defer ts.Close()
	t.Setenv(metadataHostEnv, strings.TrimPrefix(ts.URL, "http://"))
	tp, err := computeCredentials(&Options{
		Audience:           "aud",
		ComputeTokenFormat: ComputeTokenFormatStandard,
	})
	if err != nil {
		t.Fatalf("computeCredentials() = %v", err)
	}
	tok, err := tp.Token(context.Background())
	if err != nil {
		t.Fatalf("tp.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Errorf("got %q, want %q", tok.Value, wantTok)
	}
	if wantExp := time.Unix(expiry, 0); !tok.Expiry.Equal(wantExp) {
		t.Errorf("tok.Expiry = %v, want %v", tok.Expiry, wantExp)
	}
}

func TestComputeCredentials_MalformedJWT(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-a-jwt"))
	}))
	defer ts.Close()
	t.Setenv(metadataHostEnv, strings.TrimPrefix(ts.URL, "http://"))
	tp, err := computeCredentials(&Options{Audience: "aud"})
	if err != nil {
		t.Fatalf("computeCredentials() = %v", err)
	}
	if _, err := tp.Token(context.Background()); err == nil {
		t.Fatal("tp.Token() = nil, want error for malformed JWT")
	}
}

func TestComputeCredentials_CacheUsesExpClaim(t *testing.T) {
	makeJWTWithExp := func(exp int64) string {
		h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
		b, err := json.Marshal(Payload{Audience: "aud", Expires: exp})
		if err != nil {
			t.Fatal(err)
		}
		p := base64.RawURLEncoding.EncodeToString(b)
		return fmt.Sprintf("%s.%s.c2ln", h, p)
	}

	expiredTok := makeJWTWithExp(time.Now().Add(-10 * time.Second).Unix())
	freshExp := time.Now().Add(30 * time.Minute).Unix()
	freshTok := makeJWTWithExp(freshExp)

	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Write([]byte(expiredTok))
			return
		}
		w.Write([]byte(freshTok))
	}))
	defer ts.Close()
	t.Setenv(metadataHostEnv, strings.TrimPrefix(ts.URL, "http://"))

	tp, err := computeCredentials(&Options{Audience: "aud"})
	if err != nil {
		t.Fatalf("computeCredentials() = %v", err)
	}
	ctx := context.Background()

	// First call receives a token whose exp has already passed.
	tok1, err := tp.Token(ctx)
	if err != nil {
		t.Fatalf("first tp.Token() = %v", err)
	}
	if tok1.Value != expiredTok {
		t.Fatalf("first tok.Value = %q, want %q", tok1.Value, expiredTok)
	}

	// Second call must refresh instead of treating the expired token as valid for 1h.
	tok2, err := tp.Token(ctx)
	if err != nil {
		t.Fatalf("second tp.Token() = %v", err)
	}
	if tok2.Value != freshTok {
		t.Fatalf("second tok.Value = %q, want %q", tok2.Value, freshTok)
	}
	if want := time.Unix(freshExp, 0); !tok2.Expiry.Equal(want) {
		t.Errorf("second tok.Expiry = %v, want %v", tok2.Expiry, want)
	}

	// Third call should be served from cache without hitting the metadata server again.
	tok3, err := tp.Token(ctx)
	if err != nil {
		t.Fatalf("third tp.Token() = %v", err)
	}
	if tok3.Value != freshTok {
		t.Fatalf("third tok.Value = %q, want %q", tok3.Value, freshTok)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("metadata server calls = %d, want 2", got)
	}
}

func TestComputeCredentials_Invalid(t *testing.T) {
	if _, err := computeCredentials(&Options{
		Audience:     "aud",
		CustomClaims: map[string]interface{}{"foo": "bar"},
	}); err == nil {
		t.Fatal("computeCredentials() = nil, expected non-nil error", err)
	}
}
