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

package jwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func TestSignAndVerifyDecode(t *testing.T) {
	header := &Header{
		Algorithm: "RS256",
		Type:      "JWT",
	}
	payload := &Claims{
		Iss: "http://google.com/",
		Aud: "",
		Exp: 3610,
		Iat: 10,
		AdditionalClaims: map[string]interface{}{
			"foo": "bar",
		},
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	token, err := EncodeJWS(header, payload, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyJWS(token, &privateKey.PublicKey); err != nil {
		t.Fatal(err)
	}

	claims, err := DecodeJWS(token)
	if err != nil {
		t.Fatal(err)
	}

	if claims.Iss != payload.Iss {
		t.Errorf("got %q, want %q", claims.Iss, payload.Iss)
	}
	if claims.Aud != payload.Aud {
		t.Errorf("got %q, want %q", claims.Aud, payload.Aud)
	}
	if claims.Exp != payload.Exp {
		t.Errorf("got %d, want %d", claims.Exp, payload.Exp)
	}
	if claims.Iat != payload.Iat {
		t.Errorf("got %d, want %d", claims.Iat, payload.Iat)
	}
	if claims.AdditionalClaims["foo"] != payload.AdditionalClaims["foo"] {
		t.Errorf("got %q, want %q", claims.AdditionalClaims["foo"], payload.AdditionalClaims["foo"])
	}
}

func TestSignAndVerifyDecode_ES256(t *testing.T) {
	header := &Header{
		Algorithm: HeaderAlgES256,
		Type:      HeaderType,
	}
	payload := &Claims{
		Iss: "http://google.com/",
		Aud: "audience",
		Exp: 3610,
		Iat: 10,
		AdditionalClaims: map[string]interface{}{
			"foo": "bar",
		},
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	token, err := EncodeJWS(header, payload, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyJWS(token, &privateKey.PublicKey); err != nil {
		t.Fatal(err)
	}

	claims, err := DecodeJWS(token)
	if err != nil {
		t.Fatal(err)
	}

	if claims.Iss != payload.Iss {
		t.Errorf("got %q, want %q", claims.Iss, payload.Iss)
	}
	if claims.Aud != payload.Aud {
		t.Errorf("got %q, want %q", claims.Aud, payload.Aud)
	}
	if claims.Exp != payload.Exp {
		t.Errorf("got %d, want %d", claims.Exp, payload.Exp)
	}
	if claims.Iat != payload.Iat {
		t.Errorf("got %d, want %d", claims.Iat, payload.Iat)
	}
	if claims.AdditionalClaims["foo"] != payload.AdditionalClaims["foo"] {
		t.Errorf("got %q, want %q", claims.AdditionalClaims["foo"], payload.AdditionalClaims["foo"])
	}
}

func TestVerifyFailsOnMalformedClaim(t *testing.T) {
	err := VerifyJWS("abc.def", nil)
	if err == nil {
		t.Error("got no errors; want improperly formed JWT not to be verified")
	}
}

func TestDecodeJWS_Invalid(t *testing.T) {
	for _, tok := range []string{
		"fake_token",
		"eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJhdWQifQ",
		"eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJhdWQifQ.c2ln.extra",
		"eyJhbGciOiJSUzI1NiJ9.!!!invalid-base64!!!.sig",
		"eyJhbGciOiJSUzI1NiJ9.bm90LWpzb24.sig",
	} {
		if _, err := DecodeJWS(tok); err == nil {
			t.Errorf("DecodeJWS(%q) = nil, want error", tok)
		}
	}
}

func TestClaimsExpiry(t *testing.T) {
	if got := (&Claims{}).Expiry(); !got.IsZero() {
		t.Errorf("Expiry() with Exp=0 = %v, want zero time", got)
	}
	if got := (&Claims{Exp: -1}).Expiry(); !got.IsZero() {
		t.Errorf("Expiry() with Exp=-1 = %v, want zero time", got)
	}
	if got, want := (&Claims{Exp: 1700000000}).Expiry(), time.Unix(1700000000, 0); !got.Equal(want) {
		t.Errorf("Expiry() = %v, want %v", got, want)
	}
}
