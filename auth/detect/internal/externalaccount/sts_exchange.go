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

package externalaccount

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

type exchangeOptions struct {
	client         *http.Client
	endpoint       string
	request        *stsTokenExchangeRequest
	authentication clientAuthentication
	headers        http.Header
	extraOpts      map[string]interface{}
}

// exchangeToken performs an oauth2 token exchange with the provided endpoint.
// The first 4 fields are all mandatory.  headers can be used to pass additional
// headers beyond the bare minimum required by the token exchange.  options can
// be used to pass additional JSON-structured options to the remote server.
func exchangeToken(ctx context.Context, opts *exchangeOptions) (*stsTokenExchangeResponse, error) {
	data := url.Values{}
	data.Set("audience", opts.request.Audience)
	data.Set("grant_type", stsGrantType)
	data.Set("requested_token_type", stsTokenType)
	data.Set("subject_token_type", opts.request.SubjectTokenType)
	data.Set("subject_token", opts.request.SubjectToken)
	data.Set("scope", strings.Join(opts.request.Scope, " "))
	if opts.extraOpts != nil {
		opts, err := json.Marshal(opts.extraOpts)
		if err != nil {
			return nil, fmt.Errorf("detect: failed to marshal additional options: %w", err)
		}
		data.Set("options", string(opts))
	}
	opts.authentication.InjectAuthentication(data, opts.headers)
	encodedData := data.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", opts.endpoint, strings.NewReader(encodedData))
	if err != nil {
		return nil, fmt.Errorf("detect: failed to properly build http request: %w", err)

	}
	for key, list := range opts.headers {
		for _, val := range list {
			req.Header.Add(key, val)
		}
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(encodedData)))

	resp, err := opts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("detect: invalid response from Secure Token Server: %w", err)
	}
	defer resp.Body.Close()

	body, err := internal.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if c := resp.StatusCode; c < http.StatusOK || c > http.StatusMultipleChoices {
		return nil, fmt.Errorf("detect: status code %d: %s", c, body)
	}
	var stsResp stsTokenExchangeResponse
	if err := json.Unmarshal(body, &stsResp); err != nil {
		return nil, fmt.Errorf("detect: failed to unmarshal response body from Secure Token Server: %w", err)
	}

	return &stsResp, nil
}

// stsTokenExchangeRequest contains fields necessary to make an oauth2 token
// exchange.
type stsTokenExchangeRequest struct {
	ActingParty struct {
		ActorToken     string
		ActorTokenType string
	}
	GrantType          string
	Resource           string
	Audience           string
	Scope              []string
	RequestedTokenType string
	SubjectToken       string
	SubjectTokenType   string
}

// stsTokenExchangeResponse is used to decode the remote server response during
// an oauth2 token exchange.
type stsTokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	Scope           string `json:"scope"`
}

// clientAuthentication represents an OAuth client ID and secret and the
// mechanism for passing these credentials as stated in rfc6749#2.3.1.
type clientAuthentication struct {
	AuthStyle    auth.Style
	ClientID     string
	ClientSecret string
}

// InjectAuthentication is used to add authentication to a Secure Token Service
// exchange request.  It modifies either the passed url.Values or http.Header
// depending on the desired authentication format.
func (c *clientAuthentication) InjectAuthentication(values url.Values, headers http.Header) {
	if c.ClientID == "" || c.ClientSecret == "" || values == nil || headers == nil {
		return
	}
	switch c.AuthStyle {
	case auth.StyleInHeader:
		plainHeader := c.ClientID + ":" + c.ClientSecret
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(plainHeader)))
	default:
		values.Set("client_id", c.ClientID)
		values.Set("client_secret", c.ClientSecret)
	}
}
