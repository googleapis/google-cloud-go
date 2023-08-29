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
	"net/http"
	"net/url"
	"testing"

	"cloud.google.com/go/auth"
	"github.com/google/go-cmp/cmp"
)

var (
	clientID           = "rbrgnognrhongo3bi4gb9ghg9g"
	clientSecret       = "notsosecret"
	audience           = []string{"32555940559.apps.googleusercontent.com"}
	grantType          = []string{stsGrantType}
	requestedTokenType = []string{stsTokenType}
	subjectTokenType   = []string{"urn:ietf:params:oauth:token-type:jwt"}
	subjectToken       = []string{"eyJhbGciOiJSUzI1NiIsImtpZCI6IjJjNmZhNmY1OTUwYTdjZTQ2NWZjZjI0N2FhMGIwOTQ4MjhhYzk1MmMiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiIzMjU1NTk0MDU1OS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSIsImF1ZCI6IjMyNTU1OTQwNTU5LmFwcHMuZ29vZ2xldXNlcmNvbnRlbnQuY29tIiwic3ViIjoiMTEzMzE4NTQxMDA5MDU3Mzc4MzI4IiwiaGQiOiJnb29nbGUuY29tIiwiZW1haWwiOiJpdGh1cmllbEBnb29nbGUuY29tIiwiZW1haWxfdmVyaWZpZWQiOnRydWUsImF0X2hhc2giOiI5OVJVYVFrRHJsVDFZOUV0SzdiYXJnIiwiaWF0IjoxNjAxNTgxMzQ5LCJleHAiOjE2MDE1ODQ5NDl9.SZ-4DyDcogDh_CDUKHqPCiT8AKLg4zLMpPhGQzmcmHQ6cJiV0WRVMf5Lq911qsvuekgxfQpIdKNXlD6yk3FqvC2rjBbuEztMF-OD_2B8CEIYFlMLGuTQimJlUQksLKM-3B2ITRDCxnyEdaZik0OVssiy1CBTsllS5MgTFqic7w8w0Cd6diqNkfPFZRWyRYsrRDRlHHbH5_TUnv2wnLVHBHlNvU4wU2yyjDIoqOvTRp8jtXdq7K31CDhXd47-hXsVFQn2ZgzuUEAkH2Q6NIXACcVyZOrjBcZiOQI9IRWz-g03LzbzPSecO7I8dDrhqUSqMrdNUz_f8Kr8JFhuVMfVug"}
	scope              = []string{"https://www.googleapis.com/auth/devstorage.full_control"}
	ContentType        = []string{"application/x-www-form-urlencoded"}
)

func TestClientAuthentication_InjectHeaderAuthentication(t *testing.T) {
	valuesH := url.Values{
		"audience":             audience,
		"grant_type":           grantType,
		"requested_token_type": requestedTokenType,
		"subject_token_type":   subjectTokenType,
		"subject_token":        subjectToken,
		"scope":                scope,
	}
	headerH := http.Header{
		"Content-Type": ContentType,
	}

	headerAuthentication := clientAuthentication{
		AuthStyle:    auth.StyleInHeader,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	headerAuthentication.InjectAuthentication(valuesH, headerH)

	if got, want := valuesH["audience"], audience; !cmp.Equal(got, want) {
		t.Errorf("audience = %q, want %q", got, want)
	}
	if got, want := valuesH["grant_type"], grantType; !cmp.Equal(got, want) {
		t.Errorf("grant_type = %q, want %q", got, want)
	}
	if got, want := valuesH["requested_token_type"], requestedTokenType; !cmp.Equal(got, want) {
		t.Errorf("requested_token_type = %q, want %q", got, want)
	}
	if got, want := valuesH["subject_token_type"], subjectTokenType; !cmp.Equal(got, want) {
		t.Errorf("subject_token_type = %q, want %q", got, want)
	}
	if got, want := valuesH["subject_token"], subjectToken; !cmp.Equal(got, want) {
		t.Errorf("subject_token = %q, want %q", got, want)
	}
	if got, want := valuesH["scope"], scope; !cmp.Equal(got, want) {
		t.Errorf("scope = %q, want %q", got, want)
	}
	if got, want := headerH["Authorization"], []string{"Basic cmJyZ25vZ25yaG9uZ28zYmk0Z2I5Z2hnOWc6bm90c29zZWNyZXQ="}; !cmp.Equal(got, want) {
		t.Errorf("Authorization in header = %q, want %q", got, want)
	}
}

func TestClientAuthentication_ParamsAuthentication(t *testing.T) {
	valuesP := url.Values{
		"audience":             audience,
		"grant_type":           grantType,
		"requested_token_type": requestedTokenType,
		"subject_token_type":   subjectTokenType,
		"subject_token":        subjectToken,
		"scope":                scope,
	}
	headerP := http.Header{
		"Content-Type": ContentType,
	}
	paramsAuthentication := clientAuthentication{
		AuthStyle:    auth.StyleInParams,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	paramsAuthentication.InjectAuthentication(valuesP, headerP)

	if got, want := valuesP["audience"], audience; !cmp.Equal(got, want) {
		t.Errorf("audience = %q, want %q", got, want)
	}
	if got, want := valuesP["grant_type"], grantType; !cmp.Equal(got, want) {
		t.Errorf("grant_type = %q, want %q", got, want)
	}
	if got, want := valuesP["requested_token_type"], requestedTokenType; !cmp.Equal(got, want) {
		t.Errorf("requested_token_type = %q, want %q", got, want)
	}
	if got, want := valuesP["subject_token_type"], subjectTokenType; !cmp.Equal(got, want) {
		t.Errorf("subject_token_type = %q, want %q", got, want)
	}
	if got, want := valuesP["subject_token"], subjectToken; !cmp.Equal(got, want) {
		t.Errorf("subject_token = %q, want %q", got, want)
	}
	if got, want := valuesP["scope"], scope; !cmp.Equal(got, want) {
		t.Errorf("scope = %q, want %q", got, want)
	}
	if got, want := valuesP["client_id"], []string{clientID}; !cmp.Equal(got, want) {
		t.Errorf("client_id = %q, want %q", got, want)
	}
	if got, want := valuesP["client_secret"], []string{clientSecret}; !cmp.Equal(got, want) {
		t.Errorf("client_secret = %q, want %q", got, want)
	}
}
