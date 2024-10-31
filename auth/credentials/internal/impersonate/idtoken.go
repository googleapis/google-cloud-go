// Copyright 2024 Google LLC
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
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

var (
	universeDomainPlaceholder            = "UNIVERSE_DOMAIN"
	iamCredentialsUniverseDomainEndpoint = "https://iamcredentials.UNIVERSE_DOMAIN"
)

// IDTokenOptions provides configuration for [IDTokenOptions.Token].
type IDTokenOptions struct {
	Client              *http.Client
	UniverseDomain      auth.CredentialsPropertyProvider
	ServiceAccountEmail string
	GenerateIDTokenRequest
}

// GenerateIDTokenRequest holds the request to the IAM generateIdToken RPC.
type GenerateIDTokenRequest struct {
	Audience     string   `json:"audience"`
	IncludeEmail bool     `json:"includeEmail"`
	Delegates    []string `json:"delegates,omitempty"`
}

// GenerateIDTokenResponse holds the response from the IAM generateIdToken RPC.
type GenerateIDTokenResponse struct {
	Token string `json:"token"`
}

// Token call IAM generateIdToken with the configuration provided in [IDTokenOptions].
func (o IDTokenOptions) Token(ctx context.Context) (*auth.Token, error) {
	universeDomain, err := o.UniverseDomain.GetProperty(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := strings.Replace(iamCredentialsUniverseDomainEndpoint, universeDomainPlaceholder, universeDomain, 1)
	url := fmt.Sprintf("%s/v1/%s:generateIdToken", endpoint, internal.FormatIAMServiceAccountResource(o.ServiceAccountEmail))

	bodyBytes, err := json.Marshal(o.GenerateIDTokenRequest)
	if err != nil {
		return nil, fmt.Errorf("credentials: unable to marshal request: %w", err)
	}
	body, err := internal.DoJSONRequest(ctx, o.Client, url, "POST", bodyBytes, "credentials")
	if err != nil {
		return nil, err
	}
	var tokenResp GenerateIDTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("credentials: unable to parse response: %w", err)
	}
	return &auth.Token{
		Value: tokenResp.Token,
		// Generated ID tokens are good for one hour.
		Expiry: time.Now().Add(1 * time.Hour),
	}, nil
}
