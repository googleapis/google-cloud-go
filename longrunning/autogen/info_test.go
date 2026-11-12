// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package longrunning

import (
	"slices"
	"testing"

	"google.golang.org/api/option"
)

func TestSetSourceAPIVersion(t *testing.T) {
	wantVersion := "1.2.3"

	var client *OperationsClient
	var err error
	client, err = NewOperationsClient(t.Context(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewOperationsClient: %v", err)
	}
	verifyVersionKVPresent(t, client, false, wantVersion)
	client.SetSourceAPIVersion(wantVersion)
	verifyVersionKVPresent(t, client, true, wantVersion)

	client, err = NewOperationsRESTClient(t.Context(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewOperationsRESTClient: %v", err)
	}
	verifyVersionKVPresent(t, client, false, wantVersion)
	client.SetSourceAPIVersion(wantVersion)
	verifyVersionKVPresent(t, client, true, wantVersion)
}

func verifyVersionKVPresent(t *testing.T, c *OperationsClient, wantPresent bool, versionID string) {
	t.Helper()
	ic := c.internalClient
	var headers []string
	if grpcClient, ok := ic.(*operationsGRPCClient); ok {
		headers = grpcClient.xGoogHeaders
	}
	if restClient, ok := ic.(*operationsRESTClient); ok {
		headers = restClient.xGoogHeaders
	}

	gotPresent := false
	idx := slices.Index(headers, "x-goog-api-version")
	// ensure the key is present
	if idx >= 0 && idx%2 == 0 && idx < len(headers)-1 {
		// ensure the next value is the desired version string
		if headers[idx+1] == versionID {
			gotPresent = true
		}
	}

	if gotPresent != wantPresent {
		t.Errorf("api version header/key present mismatched, got present %t want present %t", gotPresent, wantPresent)
	}
}
