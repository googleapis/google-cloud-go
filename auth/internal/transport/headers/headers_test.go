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
		token           *auth.Token
		wantAuthHeader  string
		wantTBHeader    bool
		wantTBHeaderVal string
		wantGRPCMeta    map[string]string
	}{
		{
			name:           "auth only",
			token:          &auth.Token{Value: "token_val", Type: "Bearer"},
			wantAuthHeader: "Bearer token_val",
			wantTBHeader:   false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with empty tbd",
			token: &auth.Token{
				Value:             "token_val",
				Type:              "Bearer",
				TrustBoundaryData: internal.TrustBoundaryData{},
			},
			wantAuthHeader: "Bearer token_val",
			wantTBHeader:   false,
			wantGRPCMeta:   map[string]string{"authorization": "Bearer token_val"},
		},
		{
			name: "auth with no-op tbd",
			token: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
				TrustBoundaryData: internal.TrustBoundaryData{
					EncodedLocations: "0x0",
				},
			},
			wantAuthHeader:  "Bearer token_val",
			wantTBHeader:    true,
			wantTBHeaderVal: "",
			wantGRPCMeta:    map[string]string{"authorization": "Bearer token_val", "x-allowed-locations": ""},
		},
		{
			name: "auth with tbd",
			token: &auth.Token{
				Value: "token_val",
				Type:  "Bearer",
				TrustBoundaryData: internal.TrustBoundaryData{
					EncodedLocations: "some_value",
				},
			},
			wantAuthHeader:  "Bearer token_val",
			wantTBHeader:    true,
			wantTBHeaderVal: "some_value",
			wantGRPCMeta:    map[string]string{"authorization": "Bearer token_val", "x-allowed-locations": "some_value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test HTTP
			req := httptest.NewRequest("GET", "/", nil)
			SetAuthHeader(tt.token, req)
			if got := req.Header.Get("Authorization"); got != tt.wantAuthHeader {
				t.Errorf("Authorization header: got %q, want %q", got, tt.wantAuthHeader)
			}
			if got, ok := req.Header["X-Allowed-Locations"]; ok != tt.wantTBHeader || (ok && got[0] != tt.wantTBHeaderVal) {
				t.Errorf("X-Allowed-Locations header: got %q, want %q (present: %v)", got, tt.wantTBHeaderVal, tt.wantTBHeader)
			}

			// Test gRPC
			gotMeta := make(map[string]string)
			SetAuthMetadata(tt.token, gotMeta)
			if diff := cmp.Diff(tt.wantGRPCMeta, gotMeta); diff != "" {
				t.Errorf("gRPC metadata mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
