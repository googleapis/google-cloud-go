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

package detect

import (
	"errors"
	"fmt"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect/internal/externalaccount"
	"cloud.google.com/go/auth/detect/internal/gdch"
	"cloud.google.com/go/auth/detect/internal/impersonate"
	"cloud.google.com/go/auth/internal/internaldetect"
)

func fileCredentials(b []byte, opts *Options) (*Credentials, error) {
	fileType, err := internaldetect.ParseFileType(b)
	if err != nil {
		return nil, err
	}

	var projectID, quotaProjectID string
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
	case internaldetect.ImpersonatedServiceAccountKey:
		f, err := internaldetect.ParseImpersonatedServiceAccount(b)
		if err != nil {
			return nil, err
		}
		tp, err = handleImpersonatedServiceAccount(f, opts)
		if err != nil {
			return nil, err
		}
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
	return newCredentials(auth.NewCachedTokenProvider(tp, &auth.CachedTokenProviderOptions{
		ExpireEarly: opts.EarlyTokenRefresh,
	}), b, projectID, quotaProjectID), nil
}

func handleServiceAccount(f *internaldetect.ServiceAccountFile, opts *Options) (auth.TokenProvider, error) {
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

func handleUserCredential(f *internaldetect.UserCredentialsFile, opts *Options) (auth.TokenProvider, error) {
	opts3LO := &auth.Options3LO{
		ClientID:         f.ClientID,
		ClientSecret:     f.ClientSecret,
		Scopes:           opts.scopes(),
		AuthURL:          googleAuthURL,
		TokenURL:         opts.tokenURL(),
		AuthStyle:        auth.StyleInParams,
		EarlyTokenExpiry: opts.EarlyTokenRefresh,
	}
	return auth.New3LOTokenProvider(f.RefreshToken, opts3LO)
}

func handleExternalAccount(f *internaldetect.ExternalAccountFile, opts *Options) (auth.TokenProvider, error) {
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

func handleImpersonatedServiceAccount(f *internaldetect.ImpersonatedServiceAccountFile, opts *Options) (auth.TokenProvider, error) {
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

func handleGDCHServiceAccount(f *internaldetect.GDCHServiceAccountFile, opts *Options) (auth.TokenProvider, error) {
	return gdch.NewTokenProvider(f, &gdch.Options{
		STSAudience: opts.STSAudience,
		Client:      opts.client(),
	})
}
