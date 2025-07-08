// Copyright 2025 Google LLC
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

// Package optionadapt helps converts types used in [cloud.google.com/go/auth]
// and [google.golang.org/api/option].
package optionadapt

import (
	"context"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/grpctransport"
	"cloud.google.com/go/auth/oauth2adapt"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
)

// Assign to var for unit test replacement
var dialContextNewAuth = grpctransport.Dial

type testReceiver struct {
	APIKey    string
	Endpoint  string
	Scopes    []string
	UserAgent string
}

// DialPool is an adapter to call new auth library Dial.
func DialPool(ctx context.Context, secure bool, poolSize int, opts []option.ClientOption) (grpctransport.GRPCClientConnPool, error) {

	ds, err := internaloption.ParseClientOptions(opts)
	// honor options if set
	var creds *auth.Credentials
	if ds.InternalCredentials != nil {
		creds = oauth2adapt.AuthCredentialsFromOauth2Credentials(ds.InternalCredentials)
	} else if ds.Credentials != nil {
		creds = oauth2adapt.AuthCredentialsFromOauth2Credentials(ds.Credentials)
	} else if ds.AuthCredentials != nil {
		creds = ds.AuthCredentials
	} else if ds.TokenSource != nil {
		credOpts := &auth.CredentialsOptions{
			TokenProvider: oauth2adapt.TokenProviderFromTokenSource(ds.TokenSource),
		}
		if ds.QuotaProject != "" {
			credOpts.QuotaProjectIDProvider = auth.CredentialsPropertyFunc(func(ctx context.Context) (string, error) {
				return ds.QuotaProject, nil
			})
		}
		creds = auth.NewCredentials(credOpts)
	}

	var skipValidation bool
	// If our clients explicitly setup the credential skip validation as it is
	// assumed correct
	if ds.SkipValidation || ds.InternalCredentials != nil {
		skipValidation = true
	}

	var aud string
	if len(ds.Audiences) > 0 {
		aud = ds.Audiences[0]
	}
	metadata := map[string]string{}
	if ds.QuotaProject != "" {
		metadata["X-goog-user-project"] = ds.QuotaProject
	}
	if ds.RequestReason != "" {
		metadata["X-goog-request-reason"] = ds.RequestReason
	}

	// Defaults for older clients that don't set this value yet
	defaultEndpointTemplate := ds.DefaultEndpointTemplate
	if defaultEndpointTemplate == "" {
		defaultEndpointTemplate = ds.DefaultEndpoint
	}

	pool, err := dialContextNewAuth(ctx, secure, &grpctransport.Options{
		DisableTelemetry:      ds.TelemetryDisabled,
		DisableAuthentication: ds.NoAuth,
		Endpoint:              ds.Endpoint,
		Metadata:              metadata,
		GRPCDialOpts:          prepareDialOptsNewAuth(ds),
		PoolSize:              poolSize,
		Credentials:           creds,
		ClientCertProvider:    ds.ClientCertSource,
		APIKey:                ds.APIKey,
		DetectOpts: &credentials.DetectOptions{
			Scopes:          ds.Scopes,
			Audience:        aud,
			CredentialsFile: ds.CredentialsFile,
			CredentialsJSON: ds.CredentialsJSON,
			Logger:          ds.Logger,
		},
		InternalOptions: &grpctransport.InternalOptions{
			EnableNonDefaultSAForDirectPath: ds.AllowNonDefaultServiceAccount,
			EnableDirectPath:                ds.EnableDirectPath,
			EnableDirectPathXds:             ds.EnableDirectPathXds,
			EnableJWTWithScope:              ds.EnableJwtWithScope,
			AllowHardBoundTokens:            ds.AllowHardBoundTokens,
			DefaultAudience:                 ds.DefaultAudience,
			DefaultEndpointTemplate:         defaultEndpointTemplate,
			DefaultMTLSEndpoint:             ds.DefaultMTLSEndpoint,
			DefaultScopes:                   ds.DefaultScopes,
			SkipValidation:                  skipValidation,
		},
		UniverseDomain: ds.UniverseDomain,
		Logger:         ds.Logger,
	})
	return pool, err
}

func prepareDialOptsNewAuth(ds *internaloption.ParsedOptions) []grpc.DialOption {
	var opts []grpc.DialOption
	if ds.UserAgent != "" {
		opts = append(opts, grpc.WithUserAgent(ds.UserAgent))
	}

	return append(opts, ds.GRPCDialOpts...)
}
