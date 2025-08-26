// Copyright 2025 Google LLC
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

package headers

import (
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"github.com/google/go-cmp/cmp"
)

func TestSetAuth(t *testing.T) {
	tests := []struct {
		name            string
		baseToken       *auth.Token
		tbd             *internal.TrustBoundaryData
		wantAuthHeader  string
		wantTBHeader    bool
		wantTBHeaderVal string
		wantGRPCMeta    map[string]string
	}{
		{
			name:           "auth only",
			baseToken:      &auth.Token{Value: "token_val", Type: "Bearer"},
			wantAuthHeader: "Bearer token_val",
			wantTBHeader:   false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with empty tbd",
			baseToken: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
			},
			tbd:            &internal.TrustBoundaryData{},
			wantAuthHeader: "Bearer token_val",
			wantTBHeader:   false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with no-op tbd",
			baseToken: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
			},
			tbd:             internal.NewNoOpTrustBoundaryData(),
			wantAuthHeader:  "Bearer token_val",
			wantTBHeader:    true,
			wantTBHeaderVal: "",
			wantGRPCMeta:    map[string]string{"authorization": "Bearer token_val", "x-allowed-locations": ""},
		},
		{
			name: "auth with tbd",
			baseToken: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
			},
			tbd:             internal.NewTrustBoundaryData(nil, "some_value"),
			wantAuthHeader:  "Bearer token_val",
			wantTBHeader:    true,
			wantTBHeaderVal: "some_value",
			wantGRPCMeta:    map[string]string{"authorization": "Bearer token_val", "x-allowed-locations": "some_value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &auth.Token{
				Value: tt.baseToken.Value,
				Type:  tt.baseToken.Type,
			}
			if tt.tbd != nil {
				token.Metadata = map[string]interface{}{internal.TrustBoundaryDataKey: *tt.tbd}
			}
			// Test HTTP
			req := httptest.NewRequest("GET", "/", nil)
			SetAuthHeader(token, req)
			if got := req.Header.Get("Authorization"); got != tt.wantAuthHeader {
				t.Errorf("Authorization header: got %q, want %q", got, tt.wantAuthHeader)
			}
			if got, ok := req.Header["X-Allowed-Locations"]; ok != tt.wantTBHeader || (ok && got[0] != tt.wantTBHeaderVal) {
				t.Errorf("X-Allowed-Locations header: got %q, want %q (present: %v)", got, tt.wantTBHeaderVal, tt.wantTBHeader)
			}

			// Test gRPC
			gotMeta := make(map[string]string)
			SetAuthMetadata(token, gotMeta)
			if diff := cmp.Diff(tt.wantGRPCMeta, gotMeta); diff != "" {
				t.Errorf("gRPC metadata mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
