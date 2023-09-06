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

package downscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

var identityBindingEndpoint = "https://sts.googleapis.com/v1/token"

// Options for configuring [NewTokenProvider].
type Options struct {
	// BaseProvider is the [cloud.google.com/go/auth.TokenProvider] used to
	// create the downscoped provider. The downscoped provider therefore has
	// some subset of the accesses of the original BaseProvider. Required.
	BaseProvider auth.TokenProvider
	// Rules defines the accesses held by the new downscoped provider. One or
	// more AccessBoundaryRules are required to define permissions for the new
	// downscoped provider. Each one defines an access (or set of accesses) that
	// the new provider has to a given resource. There can be a maximum of 10
	// AccessBoundaryRules. Required.
	Rules []AccessBoundaryRule
	// Client configures the underlying client used to make network requests
	// when fetching tokens. Optional.
	Client *http.Client
}

func (c Options) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return internal.CloneDefaultClient()
}

// An AccessBoundaryRule Sets the permissions (and optionally conditions) that
// the new token has on given resource.
type AccessBoundaryRule struct {
	// AvailableResource is the full resource name of the Cloud Storage bucket
	// that the rule applies to. Use the format
	// //storage.googleapis.com/projects/_/buckets/bucket-name.
	AvailableResource string `json:"availableResource"`
	// AvailablePermissions is a list that defines the upper bound on the available permissions
	// for the resource. Each value is the identifier for an IAM predefined role or custom role,
	// with the prefix inRole:. For example: inRole:roles/storage.objectViewer.
	// Only the permissions in these roles will be available.
	AvailablePermissions []string `json:"availablePermissions"`
	// An Condition restricts the availability of permissions
	// to specific Cloud Storage objects. Optional.
	//
	// A Condition can be used to make permissions available for specific objects,
	// rather than all objects in a Cloud Storage bucket.
	Condition *AvailabilityCondition `json:"availabilityCondition,omitempty"`
}

// An AvailabilityCondition restricts access to a given Resource.
type AvailabilityCondition struct {
	// An Expression specifies the Cloud Storage objects where
	// permissions are available. For further documentation, see
	// https://cloud.google.com/iam/docs/conditions-overview. Required.
	Expression string `json:"expression"`
	// Title is short string that identifies the purpose of the condition. Optional.
	Title string `json:"title,omitempty"`
	// Description details about the purpose of the condition. Optional.
	Description string `json:"description,omitempty"`
}

// NewTokenProvider returns a [cloud.google.com/go/auth.TokenProvider] that is
// more restrictive than [Options.BaseProvider] provided.
func NewTokenProvider(opts *Options) (auth.TokenProvider, error) {
	if opts == nil {
		return nil, fmt.Errorf("downscope: providing opts is required")
	}
	if opts.BaseProvider == nil {
		return nil, fmt.Errorf("downscope: BaseProvider cannot be nil")
	}
	if len(opts.Rules) == 0 {
		return nil, fmt.Errorf("downscope: length of AccessBoundaryRules must be at least 1")
	}
	if len(opts.Rules) > 10 {
		return nil, fmt.Errorf("downscope: length of AccessBoundaryRules may not be greater than 10")
	}
	for _, val := range opts.Rules {
		if val.AvailableResource == "" {
			return nil, fmt.Errorf("downscope: all rules must have a nonempty AvailableResource")
		}
		if len(val.AvailablePermissions) == 0 {
			return nil, fmt.Errorf("downscope: all rules must provide at least one permission")
		}
	}
	return &downscopedTokenProvider{Options: opts, Client: opts.client()}, nil
}

// downscopedTokenProvider is used to retrieve a downscoped tokens.
type downscopedTokenProvider struct {
	Options *Options
	Client  *http.Client
}

type downscopedOptions struct {
	Boundary accessBoundary `json:"accessBoundary"`
}

type accessBoundary struct {
	AccessBoundaryRules []AccessBoundaryRule `json:"accessBoundaryRules"`
}

type downscopedTokenResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
}

func (dts *downscopedTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	downscopedOptions := downscopedOptions{
		Boundary: accessBoundary{
			AccessBoundaryRules: dts.Options.Rules,
		},
	}

	tok, err := dts.Options.BaseProvider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("downscope: unable to obtain root token: %w", err)
	}
	b, err := json.Marshal(downscopedOptions)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Add("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	form.Add("subject_token_type", "urn:ietf:params:oauth:token-type:access_token")
	form.Add("requested_token_type", "urn:ietf:params:oauth:token-type:access_token")
	form.Add("subject_token", tok.Value)
	form.Add("options", string(b))

	resp, err := dts.Client.PostForm(identityBindingEndpoint, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := internal.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downscope: unable to exchange token, %v: %s", resp.StatusCode, respBody)
	}

	var tresp downscopedTokenResponse
	err = json.Unmarshal(respBody, &tresp)
	if err != nil {
		return nil, err
	}

	// An exchanged token that is derived from a service account (2LO) has an
	// expired_in value a token derived from a users token (3LO) does not.
	// The following code uses the time remaining on rootToken for a user as the
	// value for the derived token's lifetime.
	var expiryTime time.Time
	if tresp.ExpiresIn > 0 {
		expiryTime = time.Now().Add(time.Duration(tresp.ExpiresIn) * time.Second)
	} else {
		expiryTime = tok.Expiry
	}
	return &auth.Token{
		Value:  tresp.AccessToken,
		Type:   tresp.TokenType,
		Expiry: expiryTime,
	}, nil
}
