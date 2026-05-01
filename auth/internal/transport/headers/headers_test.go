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
	"context"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal/regionalaccessboundary"
	"github.com/google/go-cmp/cmp"
)

type mockProvider struct {
	val    string
	gotCtx context.Context
}

func (m *mockProvider) GetHeaderValue(ctx context.Context, reqURL string, token *auth.Token) string {
	m.gotCtx = ctx
	return m.val
}

func TestSetAuth(t *testing.T) {
	tests := []struct {
		name             string
		baseToken        *auth.Token
		provider         *mockProvider
		wantAuthHeader   string
		wantRABHeader    bool
		wantRABHeaderVal string
		wantGRPCMeta     map[string]string
	}{
		{
			name:           "auth only",
			baseToken:      &auth.Token{Value: "token_val", Type: "Bearer"},
			wantAuthHeader: "Bearer token_val",
			wantRABHeader:  false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with empty rab",
			baseToken: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
			},
			provider:       &mockProvider{val: ""},
			wantAuthHeader: "Bearer token_val",
			wantRABHeader:  false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with rab",
			baseToken: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
			},
			provider:         &mockProvider{val: "some_value"},
			wantAuthHeader:   "Bearer token_val",
			wantRABHeader:    true,
			wantRABHeaderVal: "some_value",
			wantGRPCMeta:     map[string]string{"authorization": "Bearer token_val", "x-allowed-locations": "some_value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &auth.Token{
				Value: tt.baseToken.Value,
				Type:  tt.baseToken.Type,
			}
			if tt.provider != nil {
				token.Metadata = map[string]interface{}{regionalaccessboundary.ProviderKey: tt.provider}
			}
			// Test HTTP
			req := httptest.NewRequest("GET", "https://example.com/v1", nil)
			SetAuthHeader(token, req)
			if got := req.Header.Get("Authorization"); got != tt.wantAuthHeader {
				t.Errorf("Authorization header: got %q, want %q", got, tt.wantAuthHeader)
			}
			if got, ok := req.Header["X-Allowed-Locations"]; ok != tt.wantRABHeader || (ok && got[0] != tt.wantRABHeaderVal) {
				t.Errorf("X-Allowed-Locations header: got %q, want %q (present: %v)", got, tt.wantRABHeaderVal, tt.wantRABHeader)
			}

			// Test gRPC
			gotMeta := make(map[string]string)
			type testKeyType struct{}
			var testKey testKeyType
			testCtx := context.WithValue(context.Background(), testKey, "testVal")
			SetAuthMetadata(testCtx, token, "https://example.com/v1", gotMeta)
			if diff := cmp.Diff(tt.wantGRPCMeta, gotMeta); diff != "" {
				t.Errorf("gRPC metadata mismatch (-want +got):\n%s", diff)
			}
			if tt.provider != nil {
				if v := tt.provider.gotCtx.Value(testKey); v != "testVal" {
					t.Errorf("SetAuthMetadata did not pass context to provider, got value %v", v)
				}
			}
		})
	}
}
