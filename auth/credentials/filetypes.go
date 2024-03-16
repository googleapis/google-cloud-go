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
	"errors"
	"fmt"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials/internal/externalaccount"
	"cloud.google.com/go/auth/credentials/internal/externalaccountuser"
	"cloud.google.com/go/auth/credentials/internal/gdch"
	"cloud.google.com/go/auth/credentials/internal/impersonate"
	internalauth "cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/internaldetect"
)

func fileCredentials(b []byte, opts *DetectOptions) (*auth.Credentials, error) {
	fileType, err := internaldetect.ParseFileType(b)
	if err != nil {
		return nil, err
	}

	var projectID, quotaProjectID, universeDomain string
	var tp auth.TokenProvider
	switch fileType {
	case internaldetect.ServiceAccountKey:
		f, err := internaldetect.ParseServiceAccount(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleServiceAccount(f, opts)
		if err != nil {
			return nil, err
		}
		projectID = f.ProjectID
		universeDomain = f.UniverseDomain
	case internaldetect.UserCredentialsKey:
		f, err := internaldetect.ParseUserCredentials(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleUserCredential(f, opts)
		if err != nil {
			return nil, err
		}
		quotaProjectID = f.QuotaProjectID
	case internaldetect.ExternalAccountKey:
		f, err := internaldetect.ParseExternalAccount(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleExternalAccount(f, opts)
		if err != nil {
			return nil, err
		}
		quotaProjectID = f.QuotaProjectID
		universeDomain = f.UniverseDomain
	case internaldetect.ExternalAccountAuthorizedUserKey:
		f, err := internaldetect.ParseExternalAccountAuthorizedUser(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleExternalAccountAuthorizedUser(f, opts)
		if err != nil {
			return nil, err
		}
		quotaProjectID = f.QuotaProjectID
	case internaldetect.ImpersonatedServiceAccountKey:
		f, err := internaldetect.ParseImpersonatedServiceAccount(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleImpersonatedServiceAccount(f, opts)
		if err != nil {
			return nil, err
		}
		universeDomain = f.UniverseDomain
	case internaldetect.GDCHServiceAccountKey:
		f, err := internaldetect.ParseGDCHServiceAccount(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleGDCHServiceAccount(f, opts)
		if err != nil {
			return nil, err
		}
		projectID = f.Project
	default:
		return nil, fmt.Errorf("detect: unsupported filetype %q", fileType)
	}
	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider: auth.NewCachedTokenProvider(tp, &auth.CachedTokenProviderOptions{
			ExpireEarly: opts.EarlyTokenRefresh,
		}),
		JSON:                   b,
		ProjectIDProvider:      internalauth.StaticCredentialsProperty(projectID),
		QuotaProjectIDProvider: internalauth.StaticCredentialsProperty(quotaProjectID),
		UniverseDomainProvider: internalauth.StaticCredentialsProperty(universeDomain),
	}), nil
}

func handleServiceAccount(f *internaldetect.ServiceAccountFile, opts *DetectOptions) (auth.TokenProvider, error) {
	if opts.UseSelfSignedJWT {
		return configureSelfSignedJWT(f, opts)
	}
	opts2LO := &auth.Options2LO{
		Email:        f.ClientEmail,
		PrivateKey:   []byte(f.PrivateKey),
		PrivateKeyID: f.PrivateKeyID,
		Scopes:       opts.scopes(),
		TokenURL:     f.TokenURL,
		Subject:      opts.Subject,
	}
	if opts2LO.TokenURL == "" {
		opts2LO.TokenURL = jwtTokenURL
	}
	return auth.New2LOTokenProvider(opts2LO)
}

func handleUserCredential(f *internaldetect.UserCredentialsFile, opts *DetectOptions) (auth.TokenProvider, error) {
	opts3LO := &auth.Options3LO{
		ClientID:         f.ClientID,
		ClientSecret:     f.ClientSecret,
		Scopes:           opts.scopes(),
		AuthURL:          googleAuthURL,
		TokenURL:         opts.tokenURL(),
		AuthStyle:        auth.StyleInParams,
		EarlyTokenExpiry: opts.EarlyTokenRefresh,
		RefreshToken:     f.RefreshToken,
	}
	return auth.New3LOTokenProvider(opts3LO)
}

func handleExternalAccount(f *internaldetect.ExternalAccountFile, opts *DetectOptions) (auth.TokenProvider, error) {
	externalOpts := &externalaccount.Options{
		Audience:                       f.Audience,
		SubjectTokenType:               f.SubjectTokenType,
		TokenURL:                       f.TokenURL,
		TokenInfoURL:                   f.TokenInfoURL,
		ServiceAccountImpersonationURL: f.ServiceAccountImpersonationURL,
		ServiceAccountImpersonationLifetimeSeconds: f.ServiceAccountImpersonation.TokenLifetimeSeconds,
		ClientSecret:             f.ClientSecret,
		ClientID:                 f.ClientID,
		CredentialSource:         f.CredentialSource,
		QuotaProjectID:           f.QuotaProjectID,
		Scopes:                   opts.scopes(),
		WorkforcePoolUserProject: f.WorkforcePoolUserProject,
		Client:                   opts.client(),
	}
	return externalaccount.NewTokenProvider(externalOpts)
}

func handleExternalAccountAuthorizedUser(f *internaldetect.ExternalAccountAuthorizedUserFile, opts *DetectOptions) (auth.TokenProvider, error) {
	externalOpts := &externalaccountuser.Options{
		Audience:     f.Audience,
		RefreshToken: f.RefreshToken,
		TokenURL:     f.TokenURL,
		TokenInfoURL: f.TokenInfoURL,
		ClientID:     f.ClientID,
		ClientSecret: f.ClientSecret,
		Scopes:       opts.scopes(),
		Client:       opts.client(),
	}
	return externalaccountuser.NewTokenProvider(externalOpts)
}

func handleImpersonatedServiceAccount(f *internaldetect.ImpersonatedServiceAccountFile, opts *DetectOptions) (auth.TokenProvider, error) {
	if f.ServiceAccountImpersonationURL == "" || f.CredSource == nil {
		return nil, errors.New("missing 'source_credentials' field or 'service_account_impersonation_url' in credentials")
	}

	tp, err := fileCredentials(f.CredSource, opts)
	if err != nil {
		return nil, err
	}
	return impersonate.NewTokenProvider(&impersonate.Options{
		URL:       f.ServiceAccountImpersonationURL,
		Scopes:    opts.scopes(),
		Tp:        tp,
		Delegates: f.Delegates,
		Client:    opts.client(),
	})
}

func handleGDCHServiceAccount(f *internaldetect.GDCHServiceAccountFile, opts *DetectOptions) (auth.TokenProvider, error) {
	return gdch.NewTokenProvider(f, &gdch.Options{
		STSAudience: opts.STSAudience,
		Client:      opts.client(),
	})
}
