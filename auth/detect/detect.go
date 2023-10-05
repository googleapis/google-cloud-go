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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/internaldetect"
	"cloud.google.com/go/compute/metadata"
)

const (
	// jwtTokenURL is Google's OAuth 2.0 token URL to use with the JWT(2LO) flow.
	jwtTokenURL = "https://oauth2.googleapis.com/token"

	// Google's OAuth 2.0 default endpoints.
	googleAuthURL  = "https://accounts.google.com/o/oauth2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"

	// Help on default credentials
	adcSetupURL = "https://cloud.google.com/docs/authentication/external/set-up-adc"
)

var (
	// for testing
	allowOnGCECheck = true
)

// Credentials holds Google credentials, including
// [Application Default Credentials](https://developers.google.com/accounts/docs/application-default-credentials).
type Credentials struct {
	json           []byte
	projectID      string
	quotaProjectID string

	auth.TokenProvider
}

func newCredentials(tokenProvider auth.TokenProvider, json []byte, projectID string, quotaProjectID string) *Credentials {
	return &Credentials{
		json:           json,
		projectID:      internal.GetProjectID(json, projectID),
		quotaProjectID: internal.GetQuotaProject(json, quotaProjectID),
		TokenProvider:  tokenProvider,
	}
}

// JSON returns the bytes associated with the the file used to source
// credentials if one was used.
func (c *Credentials) JSON() []byte {
	return c.json
}

// ProjectID returns the associated project ID from the underlying file or
// environment.
func (c *Credentials) ProjectID() string {
	return c.projectID
}

// QuotaProjectID returns the associated quota project ID from the underlying
// file or environment.
func (c *Credentials) QuotaProjectID() string {
	return c.quotaProjectID
}

// OnGCE reports whether this process is running in Google Cloud.
func OnGCE() bool {
	// TODO(codyoss): once all libs use this auth lib move metadata check here
	return allowOnGCECheck && metadata.OnGCE()
}

// DefaultCredentials searches for "Application Default Credentials" and returns
// a credential based on the [Options] provided.
//
// It looks for credentials in the following places, preferring the first
// location found:
//
//   - A JSON file whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS
//     environment variable. For workload identity federation, refer to
//     https://cloud.google.com/iam/docs/how-to#using-workload-identity-federation
//     on how to generate the JSON configuration file for on-prem/non-Google
//     cloud platforms.
//   - A JSON file in a location known to the gcloud command-line tool. On
//     Windows, this is %APPDATA%/gcloud/application_default_credentials.json. On
//     other systems, $HOME/.config/gcloud/application_default_credentials.json.
//   - On Google Compute Engine, Google App Engine standard second generation
//     runtimes, and Google App Engine flexible environment, it fetches
//     credentials from the metadata server.
func DefaultCredentials(opts *Options) (*Credentials, error) {
	// TODO(codyoss): add some validation logic here.
	if opts.CredentialsJSON != nil {
		return readCredentialsFileJSON(opts.CredentialsJSON, opts)
	}
	if filename := internaldetect.GetFileNameFromEnv(opts.CredentialsFile); filename != "" {
		if creds, err := readCredentialsFile(filename, opts); err == nil {
			return creds, err
		}
	}

	fileName := internaldetect.GetWellKnownFileName()
	if b, err := os.ReadFile(fileName); err == nil {
		return readCredentialsFileJSON(b, opts)
	}

	if OnGCE() {
		id, _ := metadata.ProjectID()
		return newCredentials(computeTokenProvider(opts.EarlyTokenRefresh, opts.Scopes...), nil, id, ""), nil
	}

	return nil, fmt.Errorf("detect: could not find default credentials. See %v for more information", adcSetupURL)
}

// Options provides configuration for [DefaultCredentials].
type Options struct {
	// Scopes that credentials tokens should have. Example:
	// https://www.googleapis.com/auth/cloud-platform
	Scopes []string
	// Audience that credentials tokens should have. Only applicable for 2LO
	// flows with service accounts. If specified, scopes should not be provided.
	Audience string
	// Subject is the user email used for [domain wide delegation](https://developers.google.com/identity/protocols/oauth2/service-account#delegatingauthority).
	// Optional.
	Subject string
	// EarlyTokenRefresh configures how early before a token expires that it
	// should be refreshed.
	EarlyTokenRefresh time.Duration
	// AuthHandlerOptions configures an authorization handler and other options
	// for 3LO flows. It is required, and only used, for client credential
	// flows.
	AuthHandlerOptions *auth.AuthorizationHandlerOptions
	// TokenURL allows to set the token endpoint for user credential flows. If
	// unset the default value is: https://oauth2.googleapis.com/token.
	// Optional.
	TokenURL string
	// STSAudience is the audience sent to when retrieving an STS token.
	// Currently this only used for GDCH auth flow, for which it is required.
	STSAudience string
	// CredentialsFile overrides detection logic and sources a credential file
	// from the provided filepath. Optional.
	CredentialsFile string
	// CredentialsJSON overrides detection logic and uses the JSON bytes as the
	// source for the credential. Optional.
	CredentialsJSON []byte
	// UseSelfSignedJWT directs service account based credentials to create a
	// self-signed JWT with the private key found in the file, skipping any
	// network requests that would normally be made. Optional.
	UseSelfSignedJWT bool
	// Client configures the underlying client used to make network requests
	// when fetching tokens. Optional.
	Client *http.Client
}

func (o *Options) tokenURL() string {
	if o.TokenURL != "" {
		return o.TokenURL
	}
	return googleTokenURL
}

func (o *Options) scopes() []string {
	scopes := make([]string, len(o.Scopes))
	copy(scopes, o.Scopes)
	return scopes
}

func (o *Options) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return internal.CloneDefaultClient()
}

func readCredentialsFile(filename string, opts *Options) (*Credentials, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return readCredentialsFileJSON(b, opts)
}

func readCredentialsFileJSON(b []byte, opts *Options) (*Credentials, error) {
	// attempt to parse jsonData as a Google Developers Console client_credentials.json.
	config := clientCredConfigFromJSON(b, opts)
	if config != nil {
		tp, err := auth.New3LOTokenProvider("", config)
		if err != nil {
			return nil, err
		}
		return newCredentials(tp, b, "", ""), nil
	}
	return fileCredentials(b, opts)
}

func clientCredConfigFromJSON(b []byte, opts *Options) *auth.Options3LO {
	var creds internaldetect.ClientCredentialsFile
	var c *internaldetect.Config3LO
	if err := json.Unmarshal(b, &creds); err != nil {
		return nil
	}
	switch {
	case creds.Web != nil:
		c = creds.Web
	case creds.Installed != nil:
		c = creds.Installed
	default:
		return nil
	}
	if len(c.RedirectURIs) < 1 {
		return nil
	}
	var handleOpts *auth.AuthorizationHandlerOptions
	if opts.AuthHandlerOptions != nil {
		// TODO(codyoss): these have to be here for this flow, validate that
		handleOpts = &auth.AuthorizationHandlerOptions{
			Handler:  opts.AuthHandlerOptions.Handler,
			State:    opts.AuthHandlerOptions.State,
			PKCEOpts: opts.AuthHandlerOptions.PKCEOpts,
		}
	}
	return &auth.Options3LO{
		ClientID:         c.ClientID,
		ClientSecret:     c.ClientSecret,
		RedirectURL:      c.RedirectURIs[0],
		Scopes:           opts.scopes(),
		AuthURL:          c.AuthURI,
		TokenURL:         c.TokenURI,
		Client:           opts.client(),
		EarlyTokenExpiry: opts.EarlyTokenRefresh,
		AuthHandlerOpts:  handleOpts,
		// TODO(codyoss): refactor this out. We need to add in auto-detection
		// for this use case.
		AuthStyle: auth.StyleInParams,
	}
}
