
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

package trustboundary

import (
	"context"
	"reflect"
	"testing"

	
)

func TestNewExternalAccountTrustBoundaryConfigProvider(t *testing.T) {
	tests := []struct {
		name           string
		audience       string
		universeDomain string
		wantProvider   ConfigProvider
		wantErr        string
	}{
		{
			name:           "workload identity pool with explicit universe domain",
			audience:       "//iam.googleapis.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "example.com",
			wantProvider: &workloadIdentityPoolConfigProvider{
				projectNumber:  "12345",
				poolID:         "my-pool",
				universeDomain: "example.com",
			},
		},
		{
			name:           "workload identity pool with universe domain from audience",
			audience:       "//iam.googleapis.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "",
			wantProvider: &workloadIdentityPoolConfigProvider{
				projectNumber:  "12345",
				poolID:         "my-pool",
				universeDomain: "googleapis.com",
			},
		},
		{
			name:           "workload identity pool with non-matching universe in audience",
			audience:       "//iam.custom.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
			universeDomain: "",
			wantErr:        "trustboundary: unknown audience format: //iam.custom.com/projects/12345/locations/global/workloadIdentityPools/my-pool",
		},
		{
			name:           "workforce pool with explicit universe domain",
			audience:       "//iam.googleapis.com/locations/global/workforcePools/my-pool",
			universeDomain: "example.com",
			wantProvider: &workforcePoolConfigProvider{
				poolID:         "my-pool",
				universeDomain: "example.com",
			},
		},
		{
			name:           "workforce pool with universe domain from audience",
			audience:       "//iam.googleapis.com/locations/global/workforcePools/my-pool",
			universeDomain: "",
			wantProvider: &workforcePoolConfigProvider{
				poolID:         "my-pool",
				universeDomain: "googleapis.com",
			},
		},
		{
			name:           "workforce pool with non-matching universe in audience",
			audience:       "//iam.custom.com/locations/global/workforcePools/my-pool",
			universeDomain: "",
			wantErr:        "trustboundary: unknown audience format: //iam.custom.com/locations/global/workforcePools/my-pool",
		},
		{
			name:           "audience does not contain universe, fallback to default, but fails match",
			audience:       "projects/123/workloadIdentityPools/my-pool",
			universeDomain: "",
			wantErr:        "trustboundary: unknown audience format: projects/123/workloadIdentityPools/my-pool",
		},
		{
			name:           "unknown audience format",
			audience:       "invalid-audience-format",
			universeDomain: "",
			wantErr:        "trustboundary: unknown audience format: invalid-audience-format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewExternalAccountTrustBoundaryConfigProvider(tt.audience, tt.universeDomain)

			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("NewExternalAccountTrustBoundaryConfigProvider() error = %v, wantErr %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewExternalAccountTrustBoundaryConfigProvider() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(provider, tt.wantProvider) {
				t.Errorf("NewExternalAccountTrustBoundaryConfigProvider() provider = %v, want %v", provider, tt.wantProvider)
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
