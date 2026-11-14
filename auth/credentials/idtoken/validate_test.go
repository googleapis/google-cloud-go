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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/auth/internal/jwt"
)

const (
	keyID              = "1234"
	testAudience       = "test-audience"
	expiry       int64 = 233431200
)

var (
	beforeExp = func() time.Time { return time.Unix(expiry-1, 0) }
	afterExp  = func() time.Time { return time.Unix(expiry+1, 0) }
)

func TestValidateRS256(t *testing.T) {
	idToken, pk := createRS256JWT(t)
	tests := []struct {
		name         string
		keyID        string
		certsURL     string
		n            *big.Int
		e            int
		nowFunc      func() time.Time
		wantErr      bool
		wantCertsURL string
	}{
		{
			name:         "works",
			keyID:        keyID,
			n:            pk.N,
			e:            pk.E,
			nowFunc:      beforeExp,
			wantErr:      false,
			wantCertsURL: googleSACertsURL,
		},
		{
			name:         "works with custom certs url",
			keyID:        keyID,
			certsURL:     "https://www.googleapis.com/service_accounts/v1/jwk/chat@system.gserviceaccount.com",
			n:            pk.N,
			e:            pk.E,
			nowFunc:      beforeExp,
			wantErr:      false,
			wantCertsURL: "https://www.googleapis.com/service_accounts/v1/jwk/chat@system.gserviceaccount.com",
		},
		{
			name:         "no matching key",
			keyID:        "5678",
			n:            pk.N,
			e:            pk.E,
			nowFunc:      beforeExp,
			wantErr:      true,
			wantCertsURL: googleSACertsURL,
		},
		{
			name:         "sig does not match",
			keyID:        keyID,
			n:            new(big.Int).SetBytes([]byte("42")),
			e:            42,
			nowFunc:      beforeExp,
			wantErr:      true,
			wantCertsURL: googleSACertsURL,
		},
		{
			name:         "token expired",
			keyID:        keyID,
			n:            pk.N,
			e:            pk.E,
			nowFunc:      afterExp,
			wantErr:      true,
			wantCertsURL: googleSACertsURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					if req.URL.String() != tt.wantCertsURL {
						t.Fatalf("Invalid request uri, want %v got %v", tt.wantCertsURL, req.URL.String())
					}
					cr := certResponse{
						Keys: []jwk{
							{
								Kid: tt.keyID,
								N:   base64.RawURLEncoding.EncodeToString(tt.n.Bytes()),
								E:   base64.RawURLEncoding.EncodeToString(new(big.Int).SetInt64(int64(tt.e)).Bytes()),
							},
						},
					}
					b, err := json.Marshal(&cr)
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
			oldNow := now
			defer func() { now = oldNow }()
			now = tt.nowFunc

			v, err := NewValidator(&ValidatorOptions{
				Client:        client,
				RS256CertsURL: tt.certsURL,
			})
			if err != nil {
				t.Fatalf("NewValidator(...) = %q, want nil", err)
			}
			payload, err := v.Validate(context.Background(), idToken, testAudience)
			if tt.wantErr && err != nil {
				// Got the error we wanted.
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(ctx, %s, %s): got err %q, want nil", idToken, testAudience, err)
			}
			if tt.wantErr && err == nil {
				t.Fatalf("Validate(ctx, %s, %s): got nil err, want err", idToken, testAudience)
			}
			if payload == nil {
				t.Fatalf("Got nil payload, err: %v", err)
			}
			if payload.Audience != testAudience {
				t.Fatalf("Validate(ctx, %s, %s): got %v, want %v", idToken, testAudience, payload.Audience, testAudience)
			}
			if len(payload.Claims) == 0 {
				t.Fatalf("Validate(ctx, %s, %s): missing Claims map. payload.Claims = %+v", idToken, testAudience, payload.Claims)
			}
			if got, ok := payload.Claims["aud"]; !ok {
				t.Fatalf("Validate(ctx, %s, %s): missing aud claim. payload.Claims = %+v", idToken, testAudience, payload.Claims)
			} else {
				got, ok := got.(string)
				if !ok {
					t.Fatalf("Validate(ctx, %s, %s): aud wasn't a string. payload.Claims = %+v", idToken, testAudience, payload.Claims)
				}
				if got != testAudience {
					t.Fatalf("Validate(ctx, %s, %s): Payload[aud] want %v got %v", idToken, testAudience, testAudience, got)
				}
			}
		})
	}
}

func TestValidateES256(t *testing.T) {
	idToken, pk := createES256JWT(t)
	tests := []struct {
		name         string
		keyID        string
		certsURL     string
		x            *big.Int
		y            *big.Int
		nowFunc      func() time.Time
		wantErr      bool
		wantCertsURL string
	}{
		{
			name:         "works",
			keyID:        keyID,
			x:            pk.X,
			y:            pk.Y,
			nowFunc:      beforeExp,
			wantErr:      false,
			wantCertsURL: googleIAPCertsURL,
		},
		{
			name:         "works with custom certs url",
			keyID:        keyID,
			certsURL:     "http://example.com",
			x:            pk.X,
			y:            pk.Y,
			nowFunc:      beforeExp,
			wantErr:      false,
			wantCertsURL: "http://example.com",
		},
		{
			name:         "no matching key",
			keyID:        "5678",
			x:            pk.X,
			y:            pk.Y,
			nowFunc:      beforeExp,
			wantErr:      true,
			wantCertsURL: googleIAPCertsURL,
		},
		{
			name:         "sig does not match",
			keyID:        keyID,
			x:            new(big.Int),
			y:            new(big.Int),
			nowFunc:      beforeExp,
			wantErr:      true,
			wantCertsURL: googleIAPCertsURL,
		},
		{
			name:         "token expired",
			keyID:        keyID,
			x:            pk.X,
			y:            pk.Y,
			nowFunc:      afterExp,
			wantErr:      true,
			wantCertsURL: googleIAPCertsURL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: RoundTripFn(func(req *http.Request) *http.Response {
					if req.URL.String() != tt.wantCertsURL {
						t.Fatalf("Invalid request uri, want %v got %v", tt.wantCertsURL, req.URL.String())
					}
					cr := certResponse{
						Keys: []jwk{
							{
								Kid: tt.keyID,
								X:   base64.RawURLEncoding.EncodeToString(tt.x.Bytes()),
								Y:   base64.RawURLEncoding.EncodeToString(tt.y.Bytes()),
							},
						},
					}
					b, err := json.Marshal(&cr)
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
			oldNow := now
			defer func() { now = oldNow }()
			now = tt.nowFunc

			v, err := NewValidator(&ValidatorOptions{
				Client:        client,
				ES256CertsURL: tt.certsURL,
			})
			if err != nil {
				t.Fatalf("NewValidator(...) = %q, want nil", err)
			}
			payload, err := v.Validate(context.Background(), idToken, testAudience)
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(ctx, %s, %s) = %q, want nil", idToken, testAudience, err)
			}
			if !tt.wantErr && payload.Audience != testAudience {
				t.Fatalf("got %v, want %v", payload.Audience, testAudience)
			}
		})
	}
}

func TestParsePayload(t *testing.T) {
	idToken, _ := createRS256JWT(t)
	tests := []struct {
		name                string
		token               string
		wantPayloadAudience string
		wantErr             bool
	}{{
		name:                "valid token",
		token:               idToken,
		wantPayloadAudience: testAudience,
	}, {
		name:    "unparseable token",
		token:   "aaa.bbb.ccc",
		wantErr: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := ParsePayload(tt.token)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("ParsePayload(%q) got error %v, wantErr = %v", tt.token, err, tt.wantErr)
			}
			if tt.wantPayloadAudience != "" {
				if payload == nil || payload.Audience != tt.wantPayloadAudience {
					t.Errorf("ParsePayload(%q) got payload %+v, want payload with audience = %q", tt.token, payload, tt.wantPayloadAudience)
				}
			}
		})
	}
}

func createES256JWT(t *testing.T) (string, ecdsa.PublicKey) {
	t.Helper()
	header, claims := commonToken(t, "ES256")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	signedContent := header + "." + claims
	hashed := sha256.Sum256([]byte(signedContent))
	hash := hashed[:]
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash)
	if err != nil {
		t.Fatalf("unable to sign content: %v", err)
	}
	// R and S must each be left-padded to es256KeySize bytes (RFC 7518,
	// section 3.4); leaving S unpadded produced a short signature roughly
	// once in every 256 runs.
	sig := make([]byte, 2*es256KeySize)
	r.FillBytes(sig[:es256KeySize])
	s.FillBytes(sig[es256KeySize:])
	signature := base64.RawURLEncoding.EncodeToString(sig)
	return fmt.Sprintf("%s.%s.%s", header, claims, signature), privateKey.PublicKey
}

func createRS256JWT(t *testing.T) (string, rsa.PublicKey) {
	t.Helper()
	header, claims := commonToken(t, jwt.HeaderAlgRSA256)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	signedContent := header + "." + claims
	hashed := sha256.Sum256([]byte(signedContent))
	hash := hashed[:]
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash)
	if err != nil {
		t.Fatalf("unable to sign content: %v", err)
	}
	signature := base64.RawURLEncoding.EncodeToString(sig)
	return fmt.Sprintf("%s.%s.%s", header, claims, signature), privateKey.PublicKey
}

// returns header and claims
func commonToken(t *testing.T, alg string) (string, string) {
	t.Helper()
	header := jwt.Header{
		KeyID:     keyID,
		Algorithm: alg,
		Type:      jwt.HeaderType,
	}
	payload := Payload{
		Issuer:   "example.com",
		Audience: testAudience,
		Expires:  expiry,
	}

	hb, err := json.Marshal(&header)
	if err != nil {
		t.Fatalf("unable to marshall header: %v", err)
	}
	pb, err := json.Marshal(&payload)
	if err != nil {
		t.Fatalf("unable to marshall payload: %v", err)
	}
	eb := base64.RawURLEncoding.EncodeToString(hb)
	ep := base64.RawURLEncoding.EncodeToString(pb)
	return eb, ep
}

type RoundTripFn func(req *http.Request) *http.Response

func (f RoundTripFn) RoundTrip(req *http.Request) (*http.Response, error) { return f(req), nil }

// TestValidateMalformedTokens checks that attacker-controlled tokens are
// rejected with an error rather than causing a panic.
func TestValidateMalformedTokens(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	_, pk := createES256JWT(t)
	es256Header, claims := commonToken(t, "ES256")

	client := &http.Client{
		Transport: RoundTripFn(func(req *http.Request) *http.Response {
			cr := certResponse{Keys: []jwk{{
				Kid: keyID,
				X:   enc(pk.X.Bytes()),
				Y:   enc(pk.Y.Bytes()),
			}}}
			b, err := json.Marshal(&cr)
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

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "null payload",
			token: enc([]byte(`{"alg":"RS256"}`)) + "." + enc([]byte("null")) + "." + enc([]byte("x")),
		},
		{
			name:  "null header",
			token: enc([]byte("null")) + "." + claims + "." + enc([]byte("x")),
		},
		{
			name:  "ES256 empty signature",
			token: es256Header + "." + claims + ".",
		},
		{
			name:  "ES256 short signature",
			token: es256Header + "." + claims + "." + enc(make([]byte, 31)),
		},
		{
			name:  "ES256 long signature",
			token: es256Header + "." + claims + "." + enc(make([]byte, 65)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldNow := now
			defer func() { now = oldNow }()
			now = beforeExp

			v, err := NewValidator(&ValidatorOptions{Client: client})
			if err != nil {
				t.Fatalf("NewValidator(...) = %q, want nil", err)
			}
			if _, err := v.Validate(context.Background(), tt.token, ""); err == nil {
				t.Fatalf("Validate(ctx, %q, \"\") = nil error, want error", tt.token)
			}
		})
	}
}

// FuzzParsePayload checks that parsing arbitrary token input never panics.
func FuzzParsePayload(f *testing.F) {
	enc := base64.RawURLEncoding.EncodeToString
	f.Add("aaa.bbb.ccc")
	f.Add(enc([]byte(`{"alg":"RS256"}`)) + "." + enc([]byte("null")) + "." + enc([]byte("x")))
	f.Add(enc([]byte("null")) + "." + enc([]byte(`{"aud":"a"}`)) + "." + enc([]byte("x")))
	f.Fuzz(func(t *testing.T, token string) {
		ParsePayload(token)
	})
}
