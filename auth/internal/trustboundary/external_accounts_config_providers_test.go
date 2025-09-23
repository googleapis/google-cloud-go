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

package trustboundary

import (
	"context"
	"strings"
	"testing"
)

func TestNewExternalAccountConfigProvider(t *testing.T) {
	tests := []struct {
		name           string
		audience       string
		universeDomain string
		wantProvider   ConfigProvider
		wantErr        string
	}{
		{
			name:           "workload identity pool with matching explicit universe domain",
			audience:       "//iam.googleapis.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "googleapis.com",
			wantProvider: &workloadIdentityPoolConfigProvider{
				projectNumber:  "12345",
				poolID:         "my-pool",
				universeDomain: "googleapis.com",
			},
		},
		{
			name:           "workload identity pool with universe domain from audience",
			audience:       "//iam.custom.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "",
			wantProvider: &workloadIdentityPoolConfigProvider{
				projectNumber:  "12345",
				poolID:         "my-pool",
				universeDomain: "custom.com",
			},
		},
		{
			name:           "workload identity pool with non-matching universe domain",
			audience:       "//iam.custom.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "example.com",
			wantErr:        "provided universe domain (\"example.com\") does not match domain in audience",
		},
		{
			name:           "workforce pool with matching explicit universe domain",
			audience:       "//iam.googleapis.com/locations/global/workforcePools/my-pool",
			universeDomain: "googleapis.com",
			wantProvider: &workforcePoolConfigProvider{
				poolID:         "my-pool",
				universeDomain: "googleapis.com",
			},
		},
		{
			name:           "workforce pool with universe domain from audience",
			audience:       "//iam.custom.com/locations/global/workforcePools/my-pool",
			universeDomain: "",
			wantProvider: &workforcePoolConfigProvider{
				poolID:         "my-pool",
				universeDomain: "custom.com",
			},
		},
		{
			name:           "workforce pool with non-matching universe domain",
			audience:       "//iam.custom.com/locations/global/workforcePools/my-pool",
			universeDomain: "example.com",
			wantErr:        "provided universe domain (\"example.com\") does not match domain in audience",
		},
		{
			name:           "unknown audience format",
			audience:       "invalid-audience-format",
			universeDomain: "",
			wantErr:        "unknown audience format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewExternalAccountConfigProvider(tt.audience, tt.universeDomain)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("NewExternalAccountConfigProvider() error = %v, wantErr %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewExternalAccountConfigProvider() unexpected error: %v", err)
			}
			switch want := tt.wantProvider.(type) {
			case *workloadIdentityPoolConfigProvider:
				got, ok := provider.(*workloadIdentityPoolConfigProvider)
				if !ok {
					t.Fatalf("NewExternalAccountConfigProvider() got provider type %T, want %T", provider, want)
				}
				if *got != *want {
					t.Errorf("NewExternalAccountConfigProvider() got = %v, want %v", got, want)
				}
			case *workforcePoolConfigProvider:
				got, ok := provider.(*workforcePoolConfigProvider)
				if !ok {
					t.Fatalf("NewExternalAccountConfigProvider() got provider type %T, want %T", provider, want)
				}
				if *got != *want {
					t.Errorf("NewExternalAccountConfigProvider() got = %v, want %v", got, want)
				}
			default:
				t.Fatalf("unexpected provider type in test setup: %T", want)
			}
		})
	}
}

func TestWorkloadIdentityPoolConfigProvider(t *testing.T) {
	ctx := context.Background()
	p := &workloadIdentityPoolConfigProvider{
		projectNumber:  "12345",
		poolID:         "my-pool",
		universeDomain: "example.com",
	}

	t.Run("GetUniverseDomain", func(t *testing.T) {
		ud, err := p.GetUniverseDomain(ctx)
		if err != nil {
			t.Fatalf("GetUniverseDomain() unexpected error: %v", err)
		}
		if ud != "example.com" {
			t.Errorf("GetUniverseDomain() = %q, want %q", ud, "example.com")
		}
	})

	t.Run("GetTrustBoundaryEndpoint", func(t *testing.T) {
		want := "https://iamcredentials.example.com/v1/projects/12345/locations/global/workloadIdentityPools/my-pool/allowedLocations"
		endpoint, err := p.GetTrustBoundaryEndpoint(ctx)
		if err != nil {
			t.Fatalf("GetTrustBoundaryEndpoint() unexpected error: %v", err)
		}
		if endpoint != want {
			t.Errorf("GetTrustBoundaryEndpoint() = %q, want %q", endpoint, want)
		}
	})
}

func TestWorkforcePoolConfigProvider(t *testing.T) {
	ctx := context.Background()
	p := &workforcePoolConfigProvider{
		poolID:         "my-pool",
		universeDomain: "example.com",
	}

	t.Run("GetUniverseDomain", func(t *testing.T) {
		ud, err := p.GetUniverseDomain(ctx)
		if err != nil {
			t.Fatalf("GetUniverseDomain() unexpected error: %v", err)
		}
		if ud != "example.com" {
			t.Errorf("GetUniverseDomain() = %q, want %q", ud, "example.com")
		}
	})

	t.Run("GetTrustBoundaryEndpoint", func(t *testing.T) {
		want := "https://iamcredentials.example.com/v1/locations/global/workforcePools/my-pool/allowedLocations"
		endpoint, err := p.GetTrustBoundaryEndpoint(ctx)
		if err != nil {
			t.Fatalf("GetTrustBoundaryEndpoint() unexpected error: %v", err)
		}
		if endpoint != want {
			t.Errorf("GetTrustBoundaryEndpoint() = %q, want %q", endpoint, want)
		}
	})
}
