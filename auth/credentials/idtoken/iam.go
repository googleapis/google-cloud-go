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
	"net/http"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials/internal/impersonate"
)

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
	universeDomain auth.CredentialsPropertyProvider
	// signerEmail is the service account client email used to form the IAM generateIdToken endpoint.
	signerEmail string
	audience    string
}

func (i iamIDTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	opts := impersonate.IDTokenOptions{
		Client:              i.client,
		UniverseDomain:      i.universeDomain,
		ServiceAccountEmail: i.signerEmail,
		GenerateIDTokenRequest: impersonate.GenerateIDTokenRequest{
			Audience:     i.audience,
			IncludeEmail: true,
		},
	}
	return opts.Token(ctx)
}
