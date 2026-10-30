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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/api/option"
)

func TestSourceAPIHandling_REST(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	wantVersion := "1.2.3"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion := r.Header.Get("x-goog-api-version")
		if gotVersion != wantVersion {
			t.Fatalf("server didn't get expected api version header, got %q want %q", gotVersion, wantVersion)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	c, err := NewOperationsRESTClient(ctx,
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("NewOperationsRESTClient: %v", err)
	}
	c.SetSourceAPIVersion(wantVersion)
	c.GetOperation(ctx, new(longrunningpb.GetOperationRequest{}))

}
