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

package impersonate_test

import (
	"log"

	"cloud.google.com/go/auth/httptransport"
	"cloud.google.com/go/auth/impersonate"
)

func ExampleNewCredentialTokenProvider_serviceAccount() {
	// Base credentials sourced from ADC or provided client options
	tp, err := impersonate.NewCredentialTokenProvider(&impersonate.CredentialOptions{
		TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		// Optionally supply delegates
		Delegates: []string{"bar@project-id.iam.gserviceaccount.com"},
	})
	if err != nil {
		log.Fatal(err)
	}

	// TODO(codyoss): link to option once it exists.

	// Use this TokenProvider with a client library
	_ = tp
}

func ExampleNewCredentialTokenProvider_adminUser() {
	// Base credentials sourced from ADC or provided client options
	tp, err := impersonate.NewCredentialTokenProvider(&impersonate.CredentialOptions{
		TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		// Optionally supply delegates
		Delegates: []string{"bar@project-id.iam.gserviceaccount.com"},
		// Specify user to impersonate
		Subject: "admin@example.com",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Use this TokenProvider with a client library like
	// "google.golang.org/api/admin/directory/v1"
	_ = tp
}

func ExampleNewIDTokenProvider() {
	// Base credentials sourced from ADC or provided client options.
	tp, err := impersonate.NewIDTokenProvider(&impersonate.IDTokenOptions{
		Audience:        "http://example.com/",
		TargetPrincipal: "foo@project-id.iam.gserviceaccount.com",
		IncludeEmail:    true,
		// Optionally supply delegates.
		Delegates: []string{"bar@project-id.iam.gserviceaccount.com"},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create an authenticated client
	client, err := httptransport.NewClient(&httptransport.Options{
		TokenProvider: tp,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Use your client that is authenticated with impersonated credentials to
	// make requests.
	client.Get("http://example.com/")
}
