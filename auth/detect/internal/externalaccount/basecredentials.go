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
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect/internal/impersonate"
	"cloud.google.com/go/auth/internal/internaldetect"
)

const (
	// Subject token file types.
	fileTypeText = "text"
	fileTypeJSON = "json"

	stsGrantType = "urn:ietf:params:oauth:grant-type:token-exchange"
	stsTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

var (
	// now aliases time.Now for testing
	now = func() time.Time {
		return time.Now().UTC()
	}
	validWorkforceAudiencePattern *regexp.Regexp = regexp.MustCompile(`//iam\.googleapis\.com/locations/[^/]+/workforcePools/`)
)

// Options stores the configuration for fetching tokens with external credentials.
type Options struct {
	// Audience is the Secure Token Service (STS) audience which contains the resource name for the workload
	// identity pool or the workforce pool and the provider identifier in that pool.
	Audience string
	// SubjectTokenType is the STS token type based on the Oauth2.0 token exchange spec
	// e.g. `urn:ietf:params:oauth:token-type:jwt`.
	SubjectTokenType string
	// TokenURL is the STS token exchange endpoint.
	TokenURL string
	// TokenInfoURL is the token_info endpoint used to retrieve the account related information (
	// user attributes like account identifier, eg. email, username, uid, etc). This is
	// needed for gCloud session account identification.
	TokenInfoURL string
	// ServiceAccountImpersonationURL is the URL for the service account impersonation request. This is only
	// required for workload identity pools when APIs to be accessed have not integrated with UberMint.
	ServiceAccountImpersonationURL string
	// ServiceAccountImpersonationLifetimeSeconds is the number of seconds the service account impersonation
	// token will be valid for.
	ServiceAccountImpersonationLifetimeSeconds int
	// ClientSecret is currently only required if token_info endpoint also
	// needs to be called with the generated GCP access token. When provided, STS will be
	// called with additional basic authentication using client_id as username and client_secret as password.
	ClientSecret string
	// ClientID is only required in conjunction with ClientSecret, as described above.
	ClientID string
	//  internaldetect.CredentialSource contains the necessary information to retrieve the token itself, as well
	// as some environmental information.
	CredentialSource internaldetect.CredentialSource
	// QuotaProjectID is injected by gCloud. If the value is non-empty, the Auth libraries
	// will set the x-goog-user-project which overrides the project associated with the credentials.
	QuotaProjectID string
	// Scopes contains the desired scopes for the returned access token.
	Scopes []string
	// The optional workforce pool user project number when the credential
	// corresponds to a workforce pool and not a workload identity pool.
	// The underlying principal must still have serviceusage.services.use IAM
	// permission to use the project for billing/quota.
	WorkforcePoolUserProject string
	// Client for token request.
	Client *http.Client
}

func NewTokenProvider(opts *Options) (auth.TokenProvider, error) {
	if opts.WorkforcePoolUserProject != "" {
		valid := validateWorkforceAudience(opts.Audience)
		if !valid {
			return nil, fmt.Errorf("detect: workforce_pool_user_project should not be set for non-workforce pool credentials")
		}
	}

	tp := tokenProvider{
		client: opts.Client,
		opts:   opts,
	}
	if opts.ServiceAccountImpersonationURL == "" {
		return auth.NewCachedTokenProvider(tp, nil), nil
	}
	scopes := opts.Scopes
	tp.opts.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
	imp, err := impersonate.NewTokenProvider(&impersonate.Options{
		Client:               opts.Client,
		URL:                  opts.ServiceAccountImpersonationURL,
		Scopes:               scopes,
		Tp:                   auth.NewCachedTokenProvider(tp, nil),
		TokenLifetimeSeconds: opts.ServiceAccountImpersonationLifetimeSeconds,
	})
	if err != nil {
		return nil, err
	}
	return auth.NewCachedTokenProvider(imp, nil), nil
}

func validateWorkforceAudience(input string) bool {
	return validWorkforceAudiencePattern.MatchString(input)
}

// baseProvider determines the type of  internaldetect.CredentialSource needed.
func (o *Options) baseProvider() (subjectTokenProvider, error) {
	if len(o.CredentialSource.EnvironmentID) > 3 && o.CredentialSource.EnvironmentID[:3] == "aws" {
		if awsVersion, err := strconv.Atoi(o.CredentialSource.EnvironmentID[3:]); err == nil {
			if awsVersion != 1 {
				return nil, fmt.Errorf("detect: aws version '%d' is not supported in the current build", awsVersion)
			}

			awsCreds := &awsCredentialProvider{
				EnvironmentID:               o.CredentialSource.EnvironmentID,
				RegionURL:                   o.CredentialSource.RegionURL,
				RegionalCredVerificationURL: o.CredentialSource.RegionalCredVerificationURL,
				CredVerificationURL:         o.CredentialSource.URL,
				TargetResource:              o.Audience,
				Client:                      o.Client,
			}
			if o.CredentialSource.IMDSv2SessionTokenURL != "" {
				awsCreds.IMDSv2SessionTokenURL = o.CredentialSource.IMDSv2SessionTokenURL
			}

			return awsCreds, nil
		}
	} else if o.CredentialSource.File != "" {
		return fileCredentialProvider{File: o.CredentialSource.File, Format: o.CredentialSource.Format}, nil
	} else if o.CredentialSource.URL != "" {
		return urlCredentialProvider{URL: o.CredentialSource.URL, Headers: o.CredentialSource.Headers, Format: o.CredentialSource.Format, Client: o.Client}, nil
	} else if o.CredentialSource.Executable != nil {
		return CreateExecutableCredential(o.Client, o.CredentialSource.Executable, o)
	}
	return nil, errors.New("detect: unable to parse credential source")
}

type subjectTokenProvider interface {
	subjectToken(ctx context.Context) (string, error)
}

// tokenProvider is the provider that handles external credentials. It is used to retrieve Tokens.
type tokenProvider struct {
	client *http.Client
	opts   *Options
}

func (ts tokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	conf := ts.opts

	credSource, err := conf.baseProvider()
	if err != nil {
		return nil, err
	}
	subjectToken, err := credSource.subjectToken(ctx)

	if err != nil {
		return nil, err
	}
	stsRequest := stsTokenExchangeRequest{
		GrantType:          stsGrantType,
		Audience:           conf.Audience,
		Scope:              conf.Scopes,
		RequestedTokenType: stsTokenType,
		SubjectToken:       subjectToken,
		SubjectTokenType:   conf.SubjectTokenType,
	}
	header := make(http.Header)
	header.Add("Content-Type", "application/x-www-form-urlencoded")
	clientAuth := clientAuthentication{
		AuthStyle:    auth.StyleInHeader,
		ClientID:     conf.ClientID,
		ClientSecret: conf.ClientSecret,
	}
	var options map[string]interface{}
	// Do not pass workforce_pool_user_project when client authentication is used.
	// The client ID is sufficient for determining the user project.
	if conf.WorkforcePoolUserProject != "" && conf.ClientID == "" {
		options = map[string]interface{}{
			"userProject": conf.WorkforcePoolUserProject,
		}
	}
	stsResp, err := exchangeToken(ctx, ts.client, conf.TokenURL, &stsRequest, clientAuth, header, options)
	if err != nil {
		return nil, err
	}

	accessToken := &auth.Token{
		Value: stsResp.AccessToken,
		Type:  stsResp.TokenType,
	}
	if stsResp.ExpiresIn < 0 {
		return nil, fmt.Errorf("detect: got invalid expiry from security token service")
	} else if stsResp.ExpiresIn >= 0 {
		accessToken.Expiry = now().Add(time.Duration(stsResp.ExpiresIn) * time.Second)
	}
	return accessToken, nil
}
