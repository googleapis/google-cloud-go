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

package header

import (
	"fmt"
	"runtime"
	"strings"
	"unicode"

	"cloud.google.com/go/auth/internal"
)

// tokenType represents different types for a token.
type tokenType string

// credType represents different types for a credential.
type credType string

const (
	// GOOGLE_API_CLIENT_HEADER is the header key "x-goog-api-client"
	GOOGLE_API_CLIENT_HEADER = "x-goog-api-client"
	// CredTypeImp is the x-goog-api-client header credential type for
	// impersonated service accounts.
	CredTypeImp credType = "imp" // SA_IMPERSONATE
	// CredTypeMDS is the x-goog-api-client header credential type for compute
	// metadata server service accounts.
	CredTypeMDS credType = "mds" // SA_MDS
	// CredTypeSA is the x-goog-api-client header credential type for service
	// accounts using assertion flows.
	CredTypeSA credType = "sa" // SA_ASSERTION
	// CredTypeUser is the x-goog-api-client header credential type for user
	// credentials.
	CredTypeUser credType = "u"
	// TokenTypeAccess is the x-goog-api-client header token type for access
	// tokens.
	TokenTypeAccess tokenType = "at"
	// TokenTypeID is the x-goog-api-client header token type for ID tokens.
	TokenTypeID tokenType = "it"
	// TokenTypeNone is a sentinel value for the x-goog-api-client header token
	// type, indicating that no value should be sent.
	TokenTypeNone tokenType = "NONE"

	// versionUnknown is only used when the runtime version cannot be determined.
	versionUnknown = "UNKNOWN"
)

var (
	// version is a package internal global variable for testing purposes.
	version = runtime.Version
)

// GoVersion returns a Go runtime version derived from the runtime environment
// that is modified to be suitable for reporting in a header, meaning it has no
// whitespace. If it is unable to determine the Go runtime version, it returns
// versionUnknown.
func GoVersion() string {
	const develPrefix = "devel +"

	s := version()
	if strings.HasPrefix(s, develPrefix) {
		s = s[len(develPrefix):]
		if p := strings.IndexFunc(s, unicode.IsSpace); p >= 0 {
			s = s[:p]
		}
		return s
	} else if p := strings.IndexFunc(s, unicode.IsSpace); p >= 0 {
		s = s[:p]
	}

	notSemverRune := func(r rune) bool {
		return !strings.ContainsRune("0123456789.", r)
	}

	if strings.HasPrefix(s, "go1") {
		s = s[2:]
		var prerelease string
		if p := strings.IndexFunc(s, notSemverRune); p >= 0 {
			s, prerelease = s[:p], s[p:]
		}
		if strings.HasSuffix(s, ".") {
			s += "0"
		} else if strings.Count(s, ".") < 2 {
			s += ".0"
		}
		if prerelease != "" {
			// Some release candidates already have a dash in them.
			if !strings.HasPrefix(prerelease, "-") {
				prerelease = "-" + prerelease
			}
			s += prerelease
		}
		return s
	}
	return versionUnknown
}

// GetGoogHeaderToken returns a x-goog-api-client header value for a credentials
// token request. Examples:
//
// * Impersonated credentials access token: "gl-go/1.23.0 auth/0.9.1 auth-request-type/at cred-type/imp"
// * Impersonated credentials ID token: "gl-go/1.23.0 auth/0.9.1 auth-request-type/it cred-type/imp"
// * MDS access token: "gl-go/1.23.0 auth/0.9.1 auth-request-type/at cred-type/mds"
// * Service account credentials access token: "gl-go/1.23.0 auth/0.9.1 auth-request-type/at cred-type/sa"
// * Service account credentials ID token: "gl-go/1.23.0 auth/0.9.1 auth-request-type/it cred-type/sa"
// * User access token: "gl-go/1.23.0 auth/0.9.1 cred-type/u"
func GetGoogHeaderToken(ct credType, at tokenType) string {
	if ct == CredTypeUser {
		// Omit auth-request-type for user credentials.
		return fmt.Sprintf("gl-go/%s auth/%s cred-type/%s",
			GoVersion(),
			internal.Version,
			ct)
	}
	return fmt.Sprintf("gl-go/%s auth/%s auth-request-type/%s cred-type/%s",
		GoVersion(),
		internal.Version,
		at,
		ct)
}
