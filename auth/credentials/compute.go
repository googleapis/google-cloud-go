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

package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/compute/metadata"
)

const defaultUniverseDomain = "googleapis.com"

var (
	computeTokenMetadata = map[string]interface{}{
		"auth.google.tokenSource":    "compute-metadata",
		"auth.google.serviceAccount": "default",
	}
	computeTokenURI = "instance/service-accounts/default/token"
)

// computeTokenProvider creates a [cloud.google.com/go/auth.TokenProvider] that
// uses the metadata service to retrieve tokens.
func computeTokenProvider(earlyExpiry time.Duration, scope ...string) auth.TokenProvider {
	return auth.NewCachedTokenProvider(computeProvider{scopes: scope}, &auth.CachedTokenProviderOptions{
		ExpireEarly: earlyExpiry,
	})
}

// computeProvider fetches tokens from the google cloud metadata service.
type computeProvider struct {
	scopes []string
}

type metadataTokenResp struct {
	AccessToken  string `json:"access_token"`
	ExpiresInSec int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (cs computeProvider) Token(ctx context.Context) (*auth.Token, error) {
	tokenURI, err := url.Parse(computeTokenURI)
	if err != nil {
		return nil, err
	}
	if len(cs.scopes) > 0 {
		v := url.Values{}
		v.Set("scopes", strings.Join(cs.scopes, ","))
		tokenURI.RawQuery = v.Encode()
	}
	tokenJSON, err := metadata.Get(tokenURI.String())
	if err != nil {
		return nil, err
	}
	var res metadataTokenResp
	if err := json.NewDecoder(strings.NewReader(tokenJSON)).Decode(&res); err != nil {
		return nil, fmt.Errorf("detect: invalid token JSON from metadata: %w", err)
	}
	if res.ExpiresInSec == 0 || res.AccessToken == "" {
		return nil, errors.New("detect: incomplete token received from metadata")
	}
	return &auth.Token{
		Value:    res.AccessToken,
		Type:     res.TokenType,
		Expiry:   time.Now().Add(time.Duration(res.ExpiresInSec) * time.Second),
		Metadata: computeTokenMetadata,
	}, nil

}

// computeUniverseDomainProvider fetches the credentials universe domain from
// the google cloud metadata service.
type computeUniverseDomainProvider struct {
	universeDomainOnce sync.Once
	universeDomain     string
	universeDomainErr  error
}

func (c *computeUniverseDomainProvider) GetProperty(ctx context.Context) (string, error) {
	c.universeDomainOnce.Do(func() {
		c.universeDomain, c.universeDomainErr = getMetadataUniverseDomain(ctx)
	})
	if c.universeDomainErr != nil {
		return "", c.universeDomainErr
	}
	return c.universeDomain, nil
}

// httpGetMetadataUniverseDomain is a package var for unit test substitution.
var httpGetMetadataUniverseDomain = func(ctx context.Context) (string, error) {
	client := metadata.NewClient(&http.Client{Timeout: time.Second})
	// TODO(chrisdsmith): set ctx on request
	return client.Get("universe/universe_domain")
}

func getMetadataUniverseDomain(ctx context.Context) (string, error) {
	universeDomain, err := httpGetMetadataUniverseDomain(ctx)
	if err == nil {
		return universeDomain, nil
	}
	if _, ok := err.(metadata.NotDefinedError); ok {
		// http.StatusNotFound (404)
		return defaultUniverseDomain, nil
	} else {
		return "", err
	}
}
