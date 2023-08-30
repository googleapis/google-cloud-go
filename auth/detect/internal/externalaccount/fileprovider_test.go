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
	"testing"

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/internaldetect"
)

func TestRetrieveFileSubjectToken(t *testing.T) {
	var tests = []struct {
		name string
		cs   internaldetect.CredentialSource
		want string
	}{
		{
			name: "UntypedFileSource",
			cs: internaldetect.CredentialSource{
				File: textBaseCredPath,
			},
			want: "street123",
		},
		{
			name: "TextFileSource",
			cs: internaldetect.CredentialSource{
				File:   textBaseCredPath,
				Format: internaldetect.Format{Type: fileTypeText},
			},
			want: "street123",
		},
		{
			name: "JSONFileSource",
			cs: internaldetect.CredentialSource{
				File:   jsonBaseCredPath,
				Format: internaldetect.Format{Type: fileTypeJSON, SubjectTokenFieldName: "SubjToken"},
			},
			want: "321road",
		},
	}

	for _, test := range tests {
		test := test
		opts := testFileOpts()
		opts.CredentialSource = test.cs

		t.Run(test.name, func(t *testing.T) {
			base, err := newSubjectTokenProvider(opts)
			if err != nil {
				t.Fatalf("parse() failed %v", err)
			}

			out, err := base.subjectToken(context.Background())
			if err != nil {
				t.Errorf("Method subjectToken() errored.")
			} else if test.want != out {
				t.Errorf("got %v, want %v", out, test.want)
			}

		})
	}
}

func testFileOpts() *Options {
	return &Options{
		Audience:                       "32555940559.apps.googleusercontent.com",
		SubjectTokenType:               "urn:ietf:params:oauth:token-type:jwt",
		TokenURL:                       "http://localhost:8080/v1/token",
		TokenInfoURL:                   "http://localhost:8080/v1/tokeninfo",
		ServiceAccountImpersonationURL: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/service-gcs-admin@$PROJECT_ID.iam.gserviceaccount.com:generateAccessToken",
		ClientSecret:                   "notsosecret",
		ClientID:                       "rbrgnognrhongo3bi4gb9ghg9g",
		Client:                         internal.CloneDefaultClient(),
	}
}
