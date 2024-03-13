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

package impersonate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/httptransport"
	"cloud.google.com/go/auth/internal"
)

var (
	iamCredentialsEndpoint = "https://iamcredentials.googleapis.com"
	oauth2Endpoint         = "https://oauth2.googleapis.com"
)

// NewCredentialTokenProvider returns an impersonated
// [cloud.google.com/go/auth/TokenProvider] configured with the provided options
// and using credentials loaded from Application Default Credentials as the base
// credentials if not provided with the opts.
func NewCredentialTokenProvider(opts *CredentialOptions) (auth.TokenProvider, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	var isStaticToken bool
	// Default to the longest acceptable value of one hour as the token will
	// be refreshed automatically if not set.
	lifetime := 1 * time.Hour
	if opts.Lifetime != 0 {
		lifetime = opts.Lifetime
		// Don't auto-refresh token if a lifetime is configured.
		isStaticToken = true
	}

	var client *http.Client
	if opts.Client == nil && opts.TokenProvider == nil {
		var err error
		client, err = httptransport.NewClient(&httptransport.Options{
			InternalOptions: &httptransport.InternalOptions{
				DefaultAudience: defaultAud,
				DefaultScopes:   []string{defaultScope},
			},
		})
		if err != nil {
			return nil, err
		}
	} else if opts.TokenProvider != nil {
		client = internal.CloneDefaultClient()
		if err := httptransport.AddAuthorizationMiddleware(client, opts.TokenProvider); err != nil {
			return nil, err
		}
	} else {
		client = opts.Client
	}

	// If a subject is specified a different auth-flow is initiated to
	// impersonate as the provided subject (user).
	if opts.Subject != "" {
		return user(opts, client, lifetime, isStaticToken)
	}

	its := impersonatedTokenProvider{
		client:          client,
		targetPrincipal: opts.TargetPrincipal,
		lifetime:        fmt.Sprintf("%.fs", lifetime.Seconds()),
	}
	for _, v := range opts.Delegates {
		its.delegates = append(its.delegates, formatIAMServiceAccountName(v))
	}
	its.scopes = make([]string, len(opts.Scopes))
	copy(its.scopes, opts.Scopes)

	var tpo *auth.CachedTokenProviderOptions
	if isStaticToken {
		tpo = &auth.CachedTokenProviderOptions{
			DisableAutoRefresh: true,
		}
	}
	return auth.NewCachedTokenProvider(its, tpo), nil
}

// CredentialOptions for generating an impersonated credential token.
type CredentialOptions struct {
	// TargetPrincipal is the email address of the service account to
	// impersonate. Required.
	TargetPrincipal string
	// Scopes that the impersonated credential should have. Required.
	Scopes []string
	// Delegates are the service account email addresses in a delegation chain.
	// Each service account must be granted roles/iam.serviceAccountTokenCreator
	// on the next service account in the chain. Optional.
	Delegates []string
	// Lifetime is the amount of time until the impersonated token expires. If
	// unset the token's lifetime will be one hour and be automatically
	// refreshed. If set the token may have a max lifetime of one hour and will
	// not be refreshed. Service accounts that have been added to an org policy
	// with constraints/iam.allowServiceAccountCredentialLifetimeExtension may
	// request a token lifetime of up to 12 hours. Optional.
	Lifetime time.Duration
	// Subject is the sub field of a JWT. This field should only be set if you
	// wish to impersonate as a user. This feature is useful when using domain
	// wide delegation. Optional.
	Subject string

	// TokenProvider is the provider of the credentials used to fetch the ID
	// token. If not provided, and a Client is also not provided, credentials
	// will try to be detected from the environment. Optional.
	TokenProvider auth.TokenProvider
	// Client configures the underlying client used to make network requests
	// when fetching tokens. If provided the client should provide it's own
	// credentials at call time. Optional.
	Client *http.Client
}

func (o *CredentialOptions) validate() error {
	if o == nil {
		return errors.New("impersonate: options must be provided")
	}
	if o.TargetPrincipal == "" {
		return errors.New("impersonate: target service account must be provided")
	}
	if len(o.Scopes) == 0 {
		return errors.New("impersonate: scopes must be provided")
	}
	if o.Lifetime.Hours() > 12 {
		return errors.New("impersonate: max lifetime is 12 hours")
	}
	return nil
}

func formatIAMServiceAccountName(name string) string {
	return fmt.Sprintf("projects/-/serviceAccounts/%s", name)
}

type generateAccessTokenRequest struct {
	Delegates []string `json:"delegates,omitempty"`
	Lifetime  string   `json:"lifetime,omitempty"`
	Scope     []string `json:"scope,omitempty"`
}

type generateAccessTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireTime  string `json:"expireTime"`
}

type impersonatedTokenProvider struct {
	client *http.Client

	targetPrincipal string
	lifetime        string
	scopes          []string
	delegates       []string
}

// Token returns an impersonated Token.
func (i impersonatedTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	reqBody := generateAccessTokenRequest{
		Delegates: i.delegates,
		Lifetime:  i.lifetime,
		Scope:     i.scopes,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to marshal request: %w", err)
	}
	url := fmt.Sprintf("%s/v1/%s:generateAccessToken", iamCredentialsEndpoint, formatIAMServiceAccountName(i.targetPrincipal))
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to generate access token: %w", err)
	}
	defer resp.Body.Close()
	body, err := internal.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to read body: %w", err)
	}
	if c := resp.StatusCode; c < 200 || c > 299 {
		return nil, fmt.Errorf("impersonate: status code %d: %s", c, body)
	}

	var accessTokenResp generateAccessTokenResponse
	if err := json.Unmarshal(body, &accessTokenResp); err != nil {
		return nil, fmt.Errorf("impersonate: unable to parse response: %w", err)
	}
	expiry, err := time.Parse(time.RFC3339, accessTokenResp.ExpireTime)
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to parse expiry: %w", err)
	}
	return &auth.Token{
		Value:  accessTokenResp.AccessToken,
		Expiry: expiry,
	}, nil
}
