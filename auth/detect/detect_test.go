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

package detect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect/internal/gdch"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/internaldetect"
	"cloud.google.com/go/auth/internal/jwt"
)

type tokResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func TestDefaultCredentials_GdchServiceAccountKey(t *testing.T) {
	aud := "http://sampele-aud.com/"
	b, err := os.ReadFile("../internal/testdata/gdch.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseGDCHServiceAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected request method: %v", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		parts := strings.Split(r.FormValue("subject_token"), ".")
		var header jwt.Header
		var claims jwt.Claims
		b, err = base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &header); err != nil {
			t.Fatal(err)
		}
		b, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &claims); err != nil {
			t.Fatal(err)
		}

		if got := r.FormValue("audience"); got != aud {
			t.Errorf("got audience %v, want %v", got, gdch.GrantType)
		}
		if want := jwt.HeaderAlgRSA256; header.Algorithm != want {
			t.Errorf("got alg %q, want %q", header.Algorithm, want)
		}
		if want := jwt.HeaderType; header.Type != want {
			t.Errorf("got typ %q, want %q", header.Type, want)
		}
		if want := "abcdef1234567890"; header.KeyID != want {
			t.Errorf("got kid %q, want %q", header.KeyID, want)
		}

		if want := "system:serviceaccount:fake_project:sa_name"; claims.Iss != want {
			t.Errorf("got iss %q, want %q", claims.Iss, want)
		}
		if want := "system:serviceaccount:fake_project:sa_name"; claims.Sub != want {
			t.Errorf("got sub %q, want %q", claims.Sub, want)
		}
		if want := fmt.Sprintf("http://%s", r.Host); claims.Aud != want {
			t.Errorf("got aud %q, want %q", claims.Aud, want)
		}
		resp := &tokResp{
			AccessToken: "a_fake_token",
			TokenType:   internal.TokenTypeBearer,
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))
	f.TokenURL = ts.URL
	f.CertPath = "../internal/testdata/cert.pem"
	b, err = json.Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DefaultCredentials(&Options{CredentialsJSON: b}); err == nil {
		t.Fatal("STSAudience should be required")
	}
	creds, err := DefaultCredentials(&Options{
		CredentialsJSON: b,
		STSAudience:     aud,
	})
	if err != nil {
		t.Fatal(err)
	}

	if want := "fake_project"; creds.ProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got AccessToken %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got TokenType %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_ImpersonatedServiceAccountKey(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/imp.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseImpersonatedServiceAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &struct {
			AccessToken string `json:"accessToken"`
			ExpireTime  string `json:"expireTime"`
		}{
			AccessToken: "a_fake_token",
			ExpireTime:  "2006-01-02T15:04:05Z",
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))
	f.ServiceAccountImpersonationURL = ts.URL
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON:  b,
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
		UseSelfSignedJWT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_UserCredentialsKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := &tokResp{
			AccessToken: "a_fake_token",
			TokenType:   internal.TokenTypeBearer,
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))

	creds, err := DefaultCredentials(&Options{
		CredentialsFile: "../internal/testdata/user.json",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		TokenURL:        ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "fake_project2"; creds.QuotaProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_UserCredentialsKey_UniverseDomain(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := &tokResp{
			AccessToken: "a_fake_token",
			TokenType:   internal.TokenTypeBearer,
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))

	creds, err := DefaultCredentials(&Options{
		CredentialsFile: "../internal/testdata/user_universe_domain.json",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		TokenURL:        ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "fake_project2"; creds.QuotaProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_ServiceAccountKey(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/sa.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseServiceAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &tokResp{
			AccessToken: "a_fake_token",
			TokenType:   internal.TokenTypeBearer,
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))
	f.TokenURL = ts.URL
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON: b,
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "fake_project"; creds.ProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_ServiceAccountKeySelfSigned(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/sa.json")
	if err != nil {
		t.Fatal(err)
	}
	oldNow := now
	now = func() time.Time { return time.Date(2000, 2, 1, 12, 30, 0, 0, time.UTC) }
	defer func() { now = oldNow }()
	wantTok := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6ImFiY2RlZjEyMzQ1Njc4OTAifQ.eyJpc3MiOiJnb3BoZXJAZmFrZV9wcm9qZWN0LmlhbS5nc2VydmljZWFjY291bnQuY29tIiwic2NvcGUiOiJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9hdXRoL2Nsb3VkLXBsYXRmb3JtIiwiZXhwIjo5NDk0MTE4MDAsImlhdCI6OTQ5NDA4MjAwLCJhdWQiOiIiLCJzdWIiOiJnb3BoZXJAZmFrZV9wcm9qZWN0LmlhbS5nc2VydmljZWFjY291bnQuY29tIn0.n9Hggd-1Vw4WTQiWkh7q9r5eDsz-khU5vwkZl2VmgdUF3ZxDq1ARzchCNtTifeorzbp9C0i0vCr855G7FZkVCJXPVMcnxbwfMSafUYmVsmutbQiV9eTWfWM0_Ljiwa9GEbv1bN06Lz4LrelPKEaxsDbY6tU8LJUiome_gSMLfLk"

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON:  b,
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
		UseSelfSignedJWT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "fake_project"; creds.ProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Fatalf("got %q, want %q", tok.Value, wantTok)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_ServiceAccountKeySelfSigned_UniverseDomain(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/sa_universe_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	oldNow := now
	now = func() time.Time { return time.Date(2000, 2, 1, 12, 30, 0, 0, time.UTC) }
	defer func() { now = oldNow }()
	wantTok := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6ImFiY2RlZjEyMzQ1Njc4OTAifQ.eyJpc3MiOiJnb3BoZXJAZmFrZV9wcm9qZWN0LmlhbS5nc2VydmljZWFjY291bnQuY29tIiwic2NvcGUiOiJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9hdXRoL2Nsb3VkLXBsYXRmb3JtIiwiZXhwIjo5NDk0MTE4MDAsImlhdCI6OTQ5NDA4MjAwLCJhdWQiOiIiLCJzdWIiOiJnb3BoZXJAZmFrZV9wcm9qZWN0LmlhbS5nc2VydmljZWFjY291bnQuY29tIn0.n9Hggd-1Vw4WTQiWkh7q9r5eDsz-khU5vwkZl2VmgdUF3ZxDq1ARzchCNtTifeorzbp9C0i0vCr855G7FZkVCJXPVMcnxbwfMSafUYmVsmutbQiV9eTWfWM0_Ljiwa9GEbv1bN06Lz4LrelPKEaxsDbY6tU8LJUiome_gSMLfLk"

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON:  b,
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
		UseSelfSignedJWT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "fake_project"; creds.ProjectID() != want {
		t.Fatalf("got %q, want %q", creds.ProjectID(), want)
	}
	if want := "example.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if tok.Value != wantTok {
		t.Fatalf("got %q, want %q", tok.Value, wantTok)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_ClientCredentials(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/clientcreds_installed.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseClientCredentials(b)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := &tokResp{
			AccessToken: "a_fake_token",
			TokenType:   internal.TokenTypeBearer,
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Fatal(err)
		}
	}))
	f.Installed.TokenURI = ts.URL
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON: b,
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		TokenURL:        ts.URL,
		AuthHandlerOptions: &auth.AuthorizationHandlerOptions{
			Handler: func(authCodeURL string) (code string, state string, err error) {
				return "code", "state", nil
			},
			State: "state",
			PKCEOpts: &auth.PKCEOptions{
				Challenge:       "codeChallenge",
				ChallengeMethod: "plain",
				Verifier:        "codeChallenge",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

// Better coverage of all external account features tested in the sub-package.
func TestDefaultCredentials_ExternalAccountKey(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/exaccount_url.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseExternalAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path == "/token" {
			resp := &struct {
				Token string `json:"id_token"`
			}{
				Token: "a_fake_token_base",
			}
			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				t.Error(err)
			}
		} else if r.URL.Path == "/sts" {
			r.ParseForm()
			if got, want := r.Form.Get("subject_token"), "a_fake_token_base"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}

			resp := &struct {
				AccessToken string `json:"access_token"`
				ExpiresIn   int    `json:"expires_in"`
			}{
				AccessToken: "a_fake_token_sts",
				ExpiresIn:   60,
			}
			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				t.Error(err)
			}
		} else if r.URL.Path == "/impersonate" {
			if want := "a_fake_token_sts"; !strings.Contains(r.Header.Get("Authorization"), want) {
				t.Errorf("missing sts token: got %q, want %q", r.Header.Get("Authorization"), want)
			}

			resp := &struct {
				AccessToken string `json:"accessToken"`
				ExpireTime  string `json:"expireTime"`
			}{
				AccessToken: "a_fake_token",
				ExpireTime:  "2006-01-02T15:04:05Z",
			}
			if err := json.NewEncoder(w).Encode(&resp); err != nil {
				t.Error(err)
			}
		} else {
			t.Errorf("unexpected call to %q", r.URL.Path)
		}
	}))
	f.ServiceAccountImpersonationURL = ts.URL + "/impersonate"
	f.CredentialSource.URL = ts.URL + "/token"
	f.TokenURL = ts.URL + "/sts"
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON:  b,
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
		UseSelfSignedJWT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "googleapis.com"; creds.UniverseDomain() != want {
		t.Fatalf("got %q, want %q", creds.UniverseDomain(), want)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}
func TestDefaultCredentials_ExternalAccountAuthorizedUserKey(t *testing.T) {
	b, err := os.ReadFile("../internal/testdata/exaccount_user.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := internaldetect.ParseExternalAccountAuthorizedUser(b)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if got, want := r.URL.Path, "/sts"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		r.ParseForm()
		if got, want := r.Form.Get("refresh_token"), "refreshing"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got, want := r.Form.Get("grant_type"), "refresh_token"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}

		resp := &struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}{
			AccessToken: "a_fake_token",
			ExpiresIn:   60,
		}
		if err := json.NewEncoder(w).Encode(&resp); err != nil {
			t.Error(err)
		}
	}))
	f.TokenURL = ts.URL + "/sts"
	b, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	creds, err := DefaultCredentials(&Options{
		CredentialsJSON:  b,
		Scopes:           []string{"https://www.googleapis.com/auth/cloud-platform"},
		UseSelfSignedJWT: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := creds.Token(context.Background())
	if err != nil {
		t.Fatalf("creds.Token() = %v", err)
	}
	if want := "a_fake_token"; tok.Value != want {
		t.Fatalf("got %q, want %q", tok.Value, want)
	}
	if want := internal.TokenTypeBearer; tok.Type != want {
		t.Fatalf("got %q, want %q", tok.Type, want)
	}
}

func TestDefaultCredentials_Fails(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "nothingToSeeHere")
	t.Setenv("HOME", "nothingToSeeHere")
	t.Setenv("APPDATA", "nothingToSeeHere")
	allowOnGCECheck = false
	defer func() { allowOnGCECheck = true }()
	if _, err := DefaultCredentials(&Options{
		Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
	}); !strings.Contains(err.Error(), adcSetupURL) {
		t.Fatalf("got %v, wanted to contain %v", err, adcSetupURL)
	}
}

func TestDefaultCredentials_BadFiletype(t *testing.T) {
	if _, err := DefaultCredentials(&Options{
		CredentialsJSON: []byte(`{"type":"42"}`),
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
	}); err == nil {
		t.Fatal("got nil, want non-nil err")
	}
}

func TestDefaultCredentials_Validate(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
	}{
		{
			name: "missing options",
		},
		{
			name: "scope and audience provided",
			opts: &Options{
				Scopes:   []string{"scope"},
				Audience: "aud",
			},
		},
		{
			name: "file and json provided",
			opts: &Options{
				Scopes:          []string{"scope"},
				CredentialsFile: "path",
				CredentialsJSON: []byte(`{"some":"json"}`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DefaultCredentials(tt.opts); err == nil {
				t.Error("got nil, want an error")
			}
		})
	}
}
