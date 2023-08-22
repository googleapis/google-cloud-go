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

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/jwt"
)

const (
	// Parameter keys for AuthCodeURL method to support PKCE.
	codeChallengeKey       = "code_challenge"
	codeChallengeMethodKey = "code_challenge_method"

	// Parameter key for Exchange method to support PKCE.
	codeVerifierKey = "code_verifier"

	defaultExpiryDelta = 10 * time.Second
)

var (
	defaultGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	defaultHeader    = &jwt.Header{Algorithm: jwt.HeaderAlgRSA256, Type: jwt.HeaderType}

	// for testing
	timeNow = time.Now
)

// TokenProvider specifies an interface for anything that can return a token.
type TokenProvider interface {
	// Token returns a Token or an error.
	// The Token returned must be safe to use
	// concurrently.
	// The returned Token must not be modified.
	// The context provided must be sent along to any requests that are made in
	// the implementing code.
	Token(context.Context) (*Token, error)
}

// Token holds the credential token used to authorized requests. All fields are
// considered read-only.
type Token struct {
	// Value is the token used to authorize requests. It is usually an access
	// token but may be other types of tokens such as ID tokens in some flows.
	Value string
	// Type is the type of token Value is. If uninitialized, it should be
	// assumed to be a "Bearer" token.
	Type string
	// Expiry is the time the token is set to expire.
	Expiry time.Time
	// Metadata  may include, but is not limited to, the body of the token
	// response returned by the server.
	Metadata map[string]interface{} // TODO(codyoss): maybe make a method to flatten metadata to avoid []string for url.Values
}

// IsValid reports that a [Token] is non-nil, has a [Token.Value], and has not
// expired. A token is considered expired if [Token.Expiry] has passed or will
// pass in the next 10 seconds.
func (t *Token) IsValid() bool {
	return t.isValidWithEarlyExpiry(defaultExpiryDelta)
}

func (t *Token) isValidWithEarlyExpiry(earlyExpiry time.Duration) bool {
	if t == nil || t.Value == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return !t.Expiry.Round(0).Add(-earlyExpiry).Before(timeNow())
}

// CachedTokenProviderOptions provided options for configuring a
// CachedTokenProvider.
type CachedTokenProviderOptions struct {
	// DisableAutoRefresh makes the TokenProvider always return the same token,
	// even if it is expired.
	DisableAutoRefresh bool
	// ExpireEarly configures the amount of time before a token expires, that it
	// should be refreshed.
	ExpireEarly time.Duration
}

func (ctpo *CachedTokenProviderOptions) autoRefresh() bool {
	if ctpo == nil {
		return true
	}
	return !ctpo.DisableAutoRefresh
}

func (ctpo *CachedTokenProviderOptions) expireEarly() time.Duration {
	if ctpo == nil {
		return defaultExpiryDelta
	}
	return ctpo.ExpireEarly
}

// NewCachedTokenProvider wraps a [TokenProvider] to cache the tokens returned
// by the underlying provider.
func NewCachedTokenProvider(tp TokenProvider, opts *CachedTokenProviderOptions) TokenProvider {
	if ctp, ok := tp.(*cachedTokenProvider); ok {
		return ctp
	}
	return &cachedTokenProvider{
		tp:          tp,
		autoRefresh: opts.autoRefresh(),
		expireEarly: opts.expireEarly(),
	}
}

type cachedTokenProvider struct {
	tp          TokenProvider
	autoRefresh bool
	expireEarly time.Duration

	mu          sync.Mutex
	cachedToken *Token
}

func (c *cachedTokenProvider) Token(ctx context.Context) (*Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedToken.IsValid() || !c.autoRefresh {
		return c.cachedToken, nil
	}
	t, err := c.tp.Token(ctx)
	if err != nil {
		return nil, err
	}
	c.cachedToken = t
	return t, nil
}

// Error is a error associated with retrieving a [Token]. It can hold useful
// additional details for debugging.
type Error struct {
	// Response is the HTTP response associated with error. The body will always
	// be already closed and consumed.
	Response *http.Response
	// Body is the HTTP response body.
	Body []byte
	// Err is the underlying wrapped error.
	Err error

	// code returned in the token response
	code string
	// description returned in the token response
	description string
	// uri returned in the token response
	uri string
}

func (r *Error) Error() string {
	if r.code != "" {
		s := fmt.Sprintf("auth: %q", r.code)
		if r.description != "" {
			s += fmt.Sprintf(" %q", r.description)
		}
		if r.uri != "" {
			s += fmt.Sprintf(" %q", r.uri)
		}
		return s
	}
	return fmt.Sprintf("auth: cannot fetch token: %v\nResponse: %s", r.Response.StatusCode, r.Body)
}

// Temporary returns true if the error is considered temporary and may be able
// to be retried.
func (e *Error) Temporary() bool {
	if e.Response == nil {
		return false
	}
	sc := e.Response.StatusCode
	return sc == 500 || sc == 503 || sc == 408 || sc == 429
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Style describes how the token endpoint wants receive the ClientID and
// ClientSecret.
type Style int

const (
	// StyleUnknown means the value has not been initiated. Sending this in
	// a request will cause the token exchange to fail.
	StyleUnknown Style = 0
	// StyleInParams sends client info in the body of a POST request.
	StyleInParams Style = 1
	// StyleInHeader sends client info using Basic Authorization header.
	StyleInHeader Style = 2
)

// ConfigJWT2LO is the configuration settings for doing a 2-legged JWT OAuth2 flow.
type ConfigJWT2LO struct {
	// Email is the OAuth2 client ID. This value is set as the "iss" in the
	// JWT.
	Email string
	// PrivateKey contains the contents of an RSA private key or the
	// contents of a PEM file that contains a private key. It is used to sign
	// the JWT created.
	PrivateKey []byte
	// PrivateKeyID is the ID of the key used to sign the JWT. It is used as the
	// "kid" in the JWT header.
	PrivateKeyID string
	// Subject is the used for to impersonate a user. It is used as the "sub" in
	// the JWT.m Optional.
	Subject string
	// Scopes specifies requested permissions for the token. Optional.
	Scopes []string
	// TokenURL is th URL the JWT is sent to.
	TokenURL string
	// Expires specifies the lifetime of the token.
	Expires time.Duration
	// Audience specifies the "aud" in the JWT. Optional.
	Audience string
	// PrivateClaims allows specifying any custom claims for the JWT. Optional.
	PrivateClaims map[string]interface{}

	// Client is the client to be used to make the underlying token requests.
	// Optional.
	Client *http.Client
	// UseIDToken requests that the token returned be an ID token if one is
	// returned from the server. Optional.
	UseIDToken bool
}

func (c *ConfigJWT2LO) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return internal.CloneDefaultClient()
}

// TokenProvider returns a [TokenProvider] based on the provided fields set on
// [ConfigJWT2LO].
func (c *ConfigJWT2LO) TokenProvider() TokenProvider {
	return tokenProvider2LO{c: c, Client: c.client()}
}

type tokenProvider2LO struct {
	c      *ConfigJWT2LO
	Client *http.Client
}

func (tp tokenProvider2LO) Token(ctx context.Context) (*Token, error) {
	pk, err := internal.ParseKey(tp.c.PrivateKey)
	if err != nil {
		return nil, err
	}
	claimSet := &jwt.Claims{
		Iss:              tp.c.Email,
		Scope:            strings.Join(tp.c.Scopes, " "),
		Aud:              tp.c.TokenURL,
		AdditionalClaims: tp.c.PrivateClaims,
	}
	if subject := tp.c.Subject; subject != "" {
		claimSet.Sub = subject
	}
	if t := tp.c.Expires; t > 0 {
		claimSet.Exp = time.Now().Add(t).Unix()
	}
	if aud := tp.c.Audience; aud != "" {
		claimSet.Aud = aud
	}
	h := *defaultHeader
	h.KeyID = tp.c.PrivateKeyID
	payload, err := jwt.EncodeJWS(&h, claimSet, pk)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("grant_type", defaultGrantType)
	v.Set("assertion", payload)
	resp, err := tp.Client.PostForm(tp.c.TokenURL, v)
	if err != nil {
		return nil, fmt.Errorf("auth: cannot fetch token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("auth: cannot fetch token: %w", err)
	}
	if c := resp.StatusCode; c < 200 || c > 299 {
		return nil, &Error{
			Response: resp,
			Body:     body,
		}
	}
	// tokenRes is the JSON response body.
	var tokenRes struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		IDToken     string `json:"id_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("auth: cannot fetch token: %w", err)
	}
	token := &Token{
		Value: tokenRes.AccessToken,
		Type:  tokenRes.TokenType,
	}
	token.Metadata = make(map[string]interface{})
	json.Unmarshal(body, &token.Metadata) // no error checks for optional fields

	if secs := tokenRes.ExpiresIn; secs > 0 {
		token.Expiry = time.Now().Add(time.Duration(secs) * time.Second)
	}
	if v := tokenRes.IDToken; v != "" {
		// decode returned id token to get expiry
		claimSet, err := jwt.DecodeJWS(v)
		if err != nil {
			return nil, fmt.Errorf("auth: error decoding JWT token: %w", err)
		}
		token.Expiry = time.Unix(claimSet.Exp, 0)
	}
	if tp.c.UseIDToken {
		if tokenRes.IDToken == "" {
			return nil, fmt.Errorf("auth: response doesn't have JWT token")
		}
		token.Value = tokenRes.IDToken
	}
	return token, nil
}
