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

package idtoken

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

type generateIAMIDTokenRequest struct {
	Audience     string `json:"audience"`
	IncludeEmail bool   `json:"includeEmail"`
}

type generateIAMIDTokenResponse struct {
	Token string `json:"token"`
}

// iamIDTokenProvider performs an authenticated RPC with the IAM service to
// obtain an ID token. The provided client must be fully authenticated and
// authorized with the iam.serviceAccountTokenCreator role.
//
// This TokenProvider is primarily intended for use in non-GDU universes, which
// do not have access to the oauth2.googleapis.com/token endpoint, and thus must
// use IAM generateIdToken instead.
type iamIDTokenProvider struct {
	client *http.Client
	// universeDomain is used for endpoint construction.
	universeDomain string
	// signerEmail is the service account client email used to form the IAM generateIdToken endpoint.
	signerEmail string
	audience    string
}

func (i iamIDTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	tokenReq := generateIAMIDTokenRequest{
		Audience:     i.audience,
		IncludeEmail: true,
	}
	endpoint := strings.Replace(iamCredentialsUniverseDomainEndpoint, universeDomainPlaceholder, i.universeDomain, 1)
	url := fmt.Sprintf("%s/v1/%s:generateIdToken", endpoint, internal.FormatIAMServiceAccountName(i.signerEmail))

	bodyBytes, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("idtoken: unable to marshal request: %w", err)
	}
	body, err := internal.DoJSONRequest(ctx, i.client, url, "POST", bodyBytes, "idtoken")
	if err != nil {
		return nil, err
	}
	var tokenResp generateIAMIDTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("idtoken: unable to parse response: %w", err)
	}
	return &auth.Token{
		Value: tokenResp.Token,
		// Generated ID tokens are good for one hour.
		Expiry: time.Now().Add(1 * time.Hour),
	}, nil
}
