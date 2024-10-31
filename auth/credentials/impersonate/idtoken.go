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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	"cloud.google.com/go/auth/internal"
)

// IDTokenOptions for generating an impersonated ID token.
type IDTokenOptions struct {
	// Audience is the `aud` field for the token, such as an API endpoint the
	// token will grant access to. Required.
	Audience string
	// TargetPrincipal is the email address of the service account to
	// impersonate. Required.
	TargetPrincipal string
	// IncludeEmail includes the target service account's email in the token.
	// The resulting token will include both an `email` and `email_verified`
	// claim. Optional.
	IncludeEmail bool
	// Delegates are the ordered service account email addresses in a delegation
	// chain. Each service account must be granted
	// roles/iam.serviceAccountTokenCreator on the next service account in the
	// chain. Optional.
	Delegates []string

	// Credentials used to fetch the ID token. If not provided, and a Client is
	// also not provided, base credentials will try to be detected from the
	// environment. Optional.
	Credentials *auth.Credentials
	// Client configures the underlying client used to make network requests
	// when fetching tokens. If provided the client should provide it's own
	// base credentials at call time. Optional.
	Client *http.Client
	// UniverseDomain is the default service domain for a given Cloud universe.
	// The default value is "googleapis.com". This is the universe domain
	// configured for the client, which will be compared to the universe domain
	// that is separately configured for the credentials. Optional.
	UniverseDomain string
}

func (o *IDTokenOptions) validate() error {
	if o == nil {
		return errors.New("impersonate: options must be provided")
	}
	if o.Audience == "" {
		return errors.New("impersonate: audience must be provided")
	}
	if o.TargetPrincipal == "" {
		return errors.New("impersonate: target service account must be provided")
	}
	return nil
}

var (
	defaultScope = "https://www.googleapis.com/auth/cloud-platform"
)

// NewIDTokenCredentials creates an impersonated
// [cloud.google.com/go/auth/Credentials] that returns ID tokens configured
// with the provided config and using credentials loaded from Application
// Default Credentials as the base credentials if not provided with the opts.
// The tokens produced are valid for one hour and are automatically refreshed.
func NewIDTokenCredentials(opts *IDTokenOptions) (*auth.Credentials, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	client := opts.Client
	creds := opts.Credentials
	if client == nil {
		var err error
		if creds == nil {
			creds, err = credentials.DetectDefault(&credentials.DetectOptions{
				Scopes:           []string{defaultScope},
				UseSelfSignedJWT: true,
			})
			if err != nil {
				return nil, err
			}
		}
		client, err = httptransport.NewClient(&httptransport.Options{
			Credentials:    creds,
			UniverseDomain: opts.UniverseDomain,
		})
		if err != nil {
			return nil, err
		}
	}

	universeDomainProvider := resolveUniverseDomainProvider(creds)
	itp := impersonatedIDTokenProvider{
		client: client,
		// Pass the credentials universe domain provider to configure the endpoint.
		universeDomainProvider: universeDomainProvider,
		targetPrincipal:        opts.TargetPrincipal,
		audience:               opts.Audience,
		includeEmail:           opts.IncludeEmail,
	}
	for _, v := range opts.Delegates {
		itp.delegates = append(itp.delegates, internal.FormatIAMServiceAccountName(v))
	}

	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider:          auth.NewCachedTokenProvider(itp, nil),
		UniverseDomainProvider: universeDomainProvider,
	}), nil
}

type generateIDTokenRequest struct {
	Audience     string   `json:"audience"`
	IncludeEmail bool     `json:"includeEmail"`
	Delegates    []string `json:"delegates,omitempty"`
}

type generateIDTokenResponse struct {
	Token string `json:"token"`
}

type impersonatedIDTokenProvider struct {
	client                 *http.Client
	universeDomainProvider auth.CredentialsPropertyProvider

	targetPrincipal string
	audience        string
	includeEmail    bool
	delegates       []string
}

func (i impersonatedIDTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	genIDTokenReq := generateIDTokenRequest{
		Audience:     i.audience,
		IncludeEmail: i.includeEmail,
		Delegates:    i.delegates,
	}
	universeDomain, err := i.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := strings.Replace(iamCredentialsUniverseDomainEndpoint, universeDomainPlaceholder, universeDomain, 1)
	url := fmt.Sprintf("%s/v1/%s:generateIdToken", endpoint, internal.FormatIAMServiceAccountName(i.targetPrincipal))

	bodyBytes, err := json.Marshal(genIDTokenReq)
	if err != nil {
		return nil, fmt.Errorf("impersonate: unable to marshal request: %w", err)
	}
	body, err := internal.DoJSONRequest(ctx, i.client, url, "POST", bodyBytes, "impersonate")
	if err != nil {
		return nil, err
	}
	var generateIDTokenResp generateIDTokenResponse
	if err := json.Unmarshal(body, &generateIDTokenResp); err != nil {
		return nil, fmt.Errorf("impersonate: unable to parse response: %w", err)
	}
	return &auth.Token{
		Value: generateIDTokenResp.Token,
		// Generated ID tokens are good for one hour.
		Expiry: time.Now().Add(1 * time.Hour),
	}, nil
}
