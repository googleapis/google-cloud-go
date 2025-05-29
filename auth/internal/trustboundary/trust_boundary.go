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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/auth/internal"
)

// CredentialType represents the type of credential for which trust boundary data is being fetched.
type CredentialType int

const (
	// ServiceAccount indicates a service account credential.
	ServiceAccount CredentialType = iota
	// WorkforceIdentityPool indicates a workforce identity pool credential.
	WorkforceIdentityPool
	// WorkloadIdentityPool indicates a workload identity pool credential.
	WorkloadIdentityPool
)

const (
	// NoOpEncodedLocations is a special value indicating that no trust boundary is enforced.
	NoOpEncodedLocations = "0x0"
	// serviceAccountAllowedLocationsEndpoint is the URL for fetching allowed locations for a given service account email.
	serviceAccountAllowedLocationsEndpoint = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s/allowedLocations"
)

// DataProvider provides an interface for fetching trust boundary data.
// It's responsible for obtaining trust boundary information, including caching and specific logic for different credential types.
type DataProvider interface {
	// GetTrustBoundaryData retrieves the trust boundary data (type Data).
	// The accessToken is the bearer token used to authenticate the lookup request to the Trust Boundary API.
	// The context provided should be used for any network requests.
	GetTrustBoundaryData(ctx context.Context, accessToken string) (*Data, error)
}

// AllowedLocationsResponse is the structure of the response from the Trust Boundary API.
type AllowedLocationsResponse struct {
	// Locations is the list of allowed locations.
	Locations []string `json:"locations"`
	// EncodedLocations is the encoded representation of the allowed locations.
	EncodedLocations string `json:"encodedLocations"`
}

// Data represents the trust boundary data.
type Data struct {
	// Locations is the list of locations that the token is allowed to be used in.
	locations []string
	// EncodedLocations represents the locations in an encoded format.
	encodedLocations string
}

// Locations returns a read-only copy of the allowed locations.
func (t *Data) Locations() []string {
	if t == nil {
		return nil
	}
	locs := make([]string, len(t.locations))
	copy(locs, t.locations)
	return locs
}

// EncodedLocations returns the encoded representation of the allowed locations.
func (t *Data) EncodedLocations() string {
	if t == nil {
		return ""
	}
	return t.encodedLocations
}

// IsNoOpOrEmpty reports whether the trust boundary is a no-op or empty.
// A no-op trust boundary is one where no restrictions are enforced.
// An empty trust boundary is one where no locations are specified.
func (t *Data) IsNoOpOrEmpty() bool {
	if t == nil {
		return true
	}
	return t.encodedLocations == NoOpEncodedLocations || t.encodedLocations == ""
}

// NewNoOpTrustBoundaryData returns a new TrustBoundaryData with no restrictions.
func NewNoOpTrustBoundaryData() *Data {
	return &Data{
		encodedLocations: NoOpEncodedLocations,
	}
}

// NewTrustBoundaryData returns a new TrustBoundaryData with the specified locations and encoded locations.
func NewTrustBoundaryData(locations []string, encodedLocations string) *Data {
	locationsCopy := make([]string, len(locations))
	copy(locationsCopy, locations)
	return &Data{
		locations:        locationsCopy,
		encodedLocations: encodedLocations,
	}
}

// fetchTrustBoundaryData fetches the trust boundary data from the API.
func fetchTrustBoundaryData(ctx context.Context, client *http.Client, url string, accessToken string) (*Data, error) {
	if client == nil {
		return nil, errors.New("trustboundary: HTTP client is required")
	}

	if url == "" {
		return nil, errors.New("trustboundary: URL cannot be empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: failed to create trust boundary request: %w", err)
	}

	if accessToken == "" {
		return nil, errors.New("trustboundary: access token required for lookup API authentication")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: failed to fetch trust boundary: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: failed to read trust boundary response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trustboundary: trust boundary request failed with status: %s, body: %s", response.Status, string(body))
	}

	apiResponse := AllowedLocationsResponse{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("trustboundary: failed to unmarshal trust boundary response: %w", err)
	}

	return NewTrustBoundaryData(apiResponse.Locations, apiResponse.EncodedLocations), nil
}

// LookupServiceAccountTrustBoundary fetches trust boundary data for a service account.
// It validates input, checks for non-GDU universes, and optimizes by returning cached no-op data.
// It attempts to fetch new data and falls back to provided cached data if the fetch fails, returning nil error on successful fallback.
// The accessToken is used to authenticate the lookup request.
func LookupServiceAccountTrustBoundary(ctx context.Context, client *http.Client, serviceAccountEmail string, cachedData *Data, universeDomain string, accessToken string) (*Data, error) {
	// Validate client.
	if client == nil {
		return nil, errors.New("trustboundary: HTTP client cannot be nil")
	}

	// Validate service account email.
	if serviceAccountEmail == "" {
		return nil, errors.New("trustboundary: service account email cannot be empty")
	}

	// Check domain, skip trust boundary flow for non-gdu universe domain.
	if universeDomain != "" && universeDomain != internal.DefaultUniverseDomain {
		return NewNoOpTrustBoundaryData(), nil
	}

	// If the cached trust boundary data indicates a no-op (no restrictions),
	// skip the lookup to optimize performance and reduce load on the lookup API endpoint.
	if cachedData != nil && cachedData.EncodedLocations() == NoOpEncodedLocations {
		return cachedData, nil
	}

	url := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, serviceAccountEmail)
	trustBoundaryData, err := fetchTrustBoundaryData(ctx, client, url, accessToken)

	// If fetchTrustBoundaryData returned an error, attempt to fall back to cached data if available.
	if err != nil {
		if cachedData != nil {
			return cachedData, nil
		}
		return nil, fmt.Errorf("trustboundary: failed to fetch trust boundary data and no cache available: %w", err)
	}

	return trustBoundaryData, nil
}
