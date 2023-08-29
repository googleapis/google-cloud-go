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
	"encoding/base64"
	"net/http"
	"net/url"

	"cloud.google.com/go/auth"
)

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
	// AuthStyleInHeader corresponds to basic authentication as defined in
	// rfc7617#2
	case auth.StyleInHeader:
		plainHeader := c.ClientID + ":" + c.ClientSecret
		headers.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(plainHeader)))
	default:
		values.Set("client_id", c.ClientID)
		values.Set("client_secret", c.ClientSecret)
	}
}
