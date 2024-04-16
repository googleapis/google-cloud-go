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

package oauth2adapt

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"cloud.google.com/go/auth"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestTokenProviderFromTokenSource(t *testing.T) {
	tests := []struct {
		name  string
		token string
		err   error
	}{
		{
			name:  "working token",
			token: "fakeToken",
			err:   nil,
		},
		{
			name: "coverts err",
			err: &oauth2.RetrieveError{
				Body:      []byte("some bytes"),
				ErrorCode: "412",
				Response: &http.Response{
					StatusCode: http.StatusTeapot,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := TokenProviderFromTokenSource(tokenSource{
				token: tt.token,
				err:   tt.err,
			})
			tok, err := tp.Token(context.Background())
			if tt.err != nil {
				aErr := &auth.Error{}
				if !errors.As(err, &aErr) {
					t.Fatalf("error not of correct type: %T", err)
				}
				err := tt.err.(*oauth2.RetrieveError)
				if !cmp.Equal(aErr.Body, err.Body) {
					t.Errorf("got %s, want %s", aErr.Body, err.Body)
				}
				if !cmp.Equal(aErr.Err, err) {
					t.Errorf("got %s, want %s", aErr.Err, err)
				}
				if !cmp.Equal(aErr.Response, err.Response) {
					t.Errorf("got %s, want %s", aErr.Err, err)
				}
				return
			}
			if tok.Value != tt.token {
				t.Errorf("got %q, want %q", tok.Value, tt.token)
			}
		})
	}
}

func TestTokenSourceFromTokenProvider(t *testing.T) {
	tests := []struct {
		name  string
		token string
		err   error
	}{
		{
			name:  "working token",
			token: "fakeToken",
			err:   nil,
		},
		{
			name: "coverts err",
			err: &auth.Error{
				Body: []byte("some bytes"),
				Response: &http.Response{
					StatusCode: http.StatusTeapot,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := TokenSourceFromTokenProvider(tokenProvider{
				token: tt.token,
				err:   tt.err,
			})
			tok, err := ts.Token()
			if tt.err != nil {
				// Should be able to be an auth.Error
				aErr := &auth.Error{}
				if !errors.As(err, &aErr) {
					t.Fatalf("error not of correct type: %T", err)
				}
				err := tt.err.(*auth.Error)
				if !cmp.Equal(aErr.Body, err.Body) {
					t.Errorf("got %s, want %s", aErr.Body, err.Body)
				}
				if !cmp.Equal(aErr.Response, err.Response) {
					t.Errorf("got %s, want %s", aErr.Err, err)
				}

				// Should be able to be an oauth2.RetrieveError
				rErr := &oauth2.RetrieveError{}
				if !errors.As(err, &rErr) {
					t.Fatalf("error not of correct type: %T", err)
				}
				if !cmp.Equal(rErr.Body, err.Body) {
					t.Errorf("got %s, want %s", aErr.Body, err.Body)
				}
				if !cmp.Equal(rErr.Response, err.Response) {
					t.Errorf("got %s, want %s", aErr.Err, err)
				}
				return
			}
			if tok.AccessToken != tt.token {
				t.Errorf("got %q, want %q", tok.AccessToken, tt.token)
			}
		})
	}
}

func TestAuthCredentialsFromOauth2Credentials(t *testing.T) {
	ctx := context.Background()
	inputCreds := &google.Credentials{
		ProjectID:   "test_project",
		TokenSource: tokenSource{token: "token"},
		JSON:        []byte("json"),
		UniverseDomainProvider: func() (string, error) {
			return "domain", nil
		},
	}
	outCreds := AuthCredentialsFromOauth2Credentials(inputCreds)

	gotProject, err := outCreds.ProjectID(ctx)
	if err != nil {
		t.Fatalf("outCreds.ProjectID() = %v", err)
	}
	if want := inputCreds.ProjectID; gotProject != want {
		t.Fatalf("got %q, want %q", gotProject, want)
	}

	gotToken, err := outCreds.Token(ctx)
	if err != nil {
		t.Fatalf("outCreds.Token() = %v", err)
	}
	wantTok, err := inputCreds.TokenSource.Token()
	if err != nil {
		t.Fatalf("inputCreds.TokenSource.Token() = %v", err)
	}
	if gotToken.Value != wantTok.AccessToken {
		t.Fatalf("got %q, want %q", gotToken.Value, wantTok.AccessToken)
	}

	gotJSON := outCreds.JSON()
	if want := inputCreds.JSON; !cmp.Equal(gotJSON, want) {
		t.Fatalf("got %s, want %s", gotJSON, want)
	}

	gotUD, err := outCreds.UniverseDomain(ctx)
	if err != nil {
		t.Fatalf("outCreds.UniverseDomain() = %v", err)
	}
	wantUD, err := inputCreds.GetUniverseDomain()
	if err != nil {
		t.Fatalf("inputCreds.GetUniverseDomain() = %v", err)
	}
	if gotUD != wantUD {
		t.Fatalf("got %q, want %q", wantUD, wantUD)
	}
}

func TestOauth2CredentialsFromAuthCredentials(t *testing.T) {
	ctx := context.Background()
	inputCreds := auth.NewCredentials(&auth.CredentialsOptions{
		ProjectIDProvider: auth.CredentialsPropertyFunc(func(ctx context.Context) (string, error) {
			return "project", nil
		}),
		TokenProvider: tokenProvider{token: "token"},
		JSON:          []byte("json"),
		UniverseDomainProvider: auth.CredentialsPropertyFunc(func(ctx context.Context) (string, error) {
			return "domain", nil
		}),
	})
	outCreds := Oauth2CredentialsFromAuthCredentials(inputCreds)

	wantProject, err := inputCreds.ProjectID(ctx)
	if err != nil {
		t.Fatalf("inputCreds.ProjectID() = %v", err)
	}
	if outCreds.ProjectID != wantProject {
		t.Fatalf("got %q, want %q", outCreds.ProjectID, wantProject)
	}

	gotToken, err := inputCreds.Token(ctx)
	if err != nil {
		t.Fatalf("inputCreds.Token() = %v", err)
	}
	wantTok, err := outCreds.TokenSource.Token()
	if err != nil {
		t.Fatalf("outCreds.TokenSource.Token() = %v", err)
	}
	if gotToken.Value != wantTok.AccessToken {
		t.Fatalf("got %q, want %q", gotToken.Value, wantTok.AccessToken)
	}

	wantJSON := inputCreds.JSON()
	if !cmp.Equal(outCreds.JSON, wantJSON) {
		t.Fatalf("got %s, want %s", outCreds.JSON, wantJSON)
	}

	wantUD, err := inputCreds.UniverseDomain(ctx)
	if err != nil {
		t.Fatalf("outCreds.UniverseDomain() = %v", err)
	}
	gotUD, err := outCreds.GetUniverseDomain()
	if err != nil {
		t.Fatalf("inputCreds.GetUniverseDomain() = %v", err)
	}
	if gotUD != wantUD {
		t.Fatalf("got %q, want %q", wantUD, wantUD)
	}
}

type tokenSource struct {
	token string
	err   error
}

func (ts tokenSource) Token() (*oauth2.Token, error) {
	if ts.err != nil {
		return nil, ts.err
	}
	return &oauth2.Token{
		AccessToken: ts.token,
	}, nil
}

type tokenProvider struct {
	token string
	err   error
}

func (tp tokenProvider) Token(context.Context) (*auth.Token, error) {
	if tp.err != nil {
		return nil, tp.err
	}
	return &auth.Token{
		Value: tp.token,
	}, nil
}
