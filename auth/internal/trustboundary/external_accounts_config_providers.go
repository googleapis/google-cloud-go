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
	"fmt"
	"regexp"

	"cloud.google.com/go/auth/internal"
)

const (
	workloadAllowedLocationsEndpoint  = "https://iamcredentials.%s/v1/projects/%s/locations/global/workloadIdentityPools/%s/allowedLocations"
	workforceAllowedLocationsEndpoint = "https://iamcredentials.%s/v1/locations/global/workforcePools/%s/allowedLocations"
)

var (
	workforceAudiencePattern = regexp.MustCompile(`//iam.googleapis.com/locations/global/workforcePools/([^/]+)`)
	workloadAudiencePattern  = regexp.MustCompile(`//iam.googleapis.com/projects/([^/]+)/locations/global/workloadIdentityPools/([^/]+)`)
	universeDomainPattern    = regexp.MustCompile(`//iam.([^/]+)/`)
)

// NewExternalAccountTrustBoundaryConfigProvider creates a new ConfigProvider for external accounts.
func NewExternalAccountTrustBoundaryConfigProvider(audience, universeDomain string) (ConfigProvider, error) {
	if universeDomain == "" {
		matches := universeDomainPattern.FindStringSubmatch(audience)
		if len(matches) > 1 {
			universeDomain = matches[1]
		} else {
			universeDomain = internal.DefaultUniverseDomain
		}
	}

	if matches := workloadAudiencePattern.FindStringSubmatch(audience); len(matches) > 0 {
		return &workloadIdentityPoolConfigProvider{
			projectNumber:  matches[1],
			poolID:         matches[2],
			universeDomain: universeDomain,
		}, nil
	}
	if matches := workforceAudiencePattern.FindStringSubmatch(audience); len(matches) > 0 {
		return &workforcePoolConfigProvider{
			poolID:         matches[1],
			universeDomain: universeDomain,
		}, nil
	}
	return nil, fmt.Errorf("trustboundary: unknown audience format: %s", audience)
}

type workforcePoolConfigProvider struct {
	poolID         string
	universeDomain string
}

func (p *workforcePoolConfigProvider) GetTrustBoundaryEndpoint(ctx context.Context) (string, error) {
	return fmt.Sprintf(workforceAllowedLocationsEndpoint, p.universeDomain, p.poolID), nil
}

func (p *workforcePoolConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	return p.universeDomain, nil
}

type workloadIdentityPoolConfigProvider struct {
	projectNumber  string
	poolID         string
	universeDomain string
}

func (p *workloadIdentityPoolConfigProvider) GetTrustBoundaryEndpoint(ctx context.Context) (string, error) {
	return fmt.Sprintf(workloadAllowedLocationsEndpoint, p.universeDomain, p.projectNumber, p.poolID), nil
}

func (p *workloadIdentityPoolConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	return p.universeDomain, nil
}