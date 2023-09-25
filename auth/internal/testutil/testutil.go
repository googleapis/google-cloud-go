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

package testutil

import (
	"fmt"
	"net/http"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

// IntegrationTestCheck is a helper to check if an integration test should be
// run
func IntegrationTestCheck(t *testing.T) {
	t.Helper()
	t.Skip("TODO(codyoss): remove this once we add all secrets")
	if testing.Short() {
		t.Skip("skipping integration test")
	}
}

// TODO(codyoss): remove all code below when httptransport package is added.

// AddAuthorizationMiddleware adds a middleware to the provided client's
// transport that sets the Authorization header with the value produced by the
// provided [cloud.google.com/go/auth.TokenProvider]. An error is returned only
// if client or tp is nil.
func AddAuthorizationMiddleware(client *http.Client, tp auth.TokenProvider) error {
	if client == nil || tp == nil {
		return fmt.Errorf("httptransport: client and tp must not be nil")
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	client.Transport = &authTransport{
		provider: auth.NewCachedTokenProvider(tp, nil),
		base:     base,
	}
	return nil
}

type authTransport struct {
	provider auth.TokenProvider
	base     http.RoundTripper
}

// RoundTrip authorizes and authenticates the request with an
// access token from Transport's Source. Per the RoundTripper contract we must
// not modify the initial request, so we clone it, and we must close the body
// on any errors that happens during our token logic.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqBodyClosed := false
	if req.Body != nil {
		defer func() {
			if !reqBodyClosed {
				req.Body.Close()
			}
		}()
	}
	token, err := t.provider.Token(req.Context())
	if err != nil {
		return nil, err
	}
	req2 := req.Clone(req.Context())
	SetAuthHeader(token, req2)
	reqBodyClosed = true
	return t.base.RoundTrip(req2)
}

// SetAuthHeader uses the provided token to set the Authorization header on a
// request. If the token.Type is empty, the type is assumed to be Bearer.
func SetAuthHeader(token *auth.Token, req *http.Request) {
	typ := token.Type
	if typ == "" {
		typ = internal.TokenTypeBearer
	}
	req.Header.Set("Authorization", typ+" "+token.Value)
}
