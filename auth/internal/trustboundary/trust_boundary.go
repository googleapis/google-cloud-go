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
	"log/slog"
	"net/http"

	"cloud.google.com/go/auth/internal"
	"github.com/googleapis/gax-go/v2/internallog"
)

const (
	// NoOpEncodedLocations is a special value indicating that no trust boundary is enforced.
	NoOpEncodedLocations = "0x0"
	// serviceAccountAllowedLocationsEndpoint is the URL for fetching allowed locations for a given service account email.
	serviceAccountAllowedLocationsEndpoint = "https://iamcredentials.%s/v1/projects/-/serviceAccounts/%s/allowedLocations"
)

// DataProvider provides an interface for fetching trust boundary data.
// It's responsible for obtaining trust boundary information, including caching and specific logic for different credential types.
type DataProvider interface {
	// GetTrustBoundaryData retrieves the trust boundary data (type Data).
	// The accessToken is the bearer token used to authenticate the lookup request to the Trust Boundary API.
	// The context provided should be used for any network requests.
	GetTrustBoundaryData(ctx context.Context, accessToken string) (*Data, error)
}

// ConfigProvider provides specific configuration for trust boundary lookups.
type ConfigProvider interface {
	// GetTrustBoundaryEndpoint returns the endpoint URL for the trust boundary lookup.
	GetTrustBoundaryEndpoint(ctx context.Context) (url string, err error)
	// GetUniverseDomain returns the universe domain associated with the credential.
	// It may return an error if the universe domain cannot be determined.
	GetUniverseDomain(ctx context.Context) (string, error)
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
	Locations []string
	// EncodedLocations represents the locations in an encoded format.
	EncodedLocations string
}

// NewNoOpTrustBoundaryData returns a new TrustBoundaryData with no restrictions.
func NewNoOpTrustBoundaryData() *Data {
	return &Data{
		EncodedLocations: NoOpEncodedLocations,
	}
}

// NewTrustBoundaryData returns a new TrustBoundaryData with the specified locations and encoded locations.
func NewTrustBoundaryData(locations []string, encodedLocations string) *Data {
	locationsCopy := make([]string, len(locations))
	copy(locationsCopy, locations)
	return &Data{
		Locations:        locationsCopy,
		EncodedLocations: encodedLocations,
	}
}

// fetchTrustBoundaryData fetches the trust boundary data from the API.
func fetchTrustBoundaryData(ctx context.Context, client *http.Client, url string, accessToken string, logger *slog.Logger) (*Data, error) {
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

	logger.DebugContext(ctx, "trust boundary request", "request", internallog.HTTPRequest(req, nil))

	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: failed to fetch trust boundary: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: failed to read trust boundary response: %w", err)
	}

	logger.DebugContext(ctx, "trust boundary response", "response", internallog.HTTPResponse(response, body))

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trustboundary: trust boundary request failed with status: %s, body: %s", response.Status, string(body))
	}

	apiResponse := AllowedLocationsResponse{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("trustboundary: failed to unmarshal trust boundary response: %w", err)
	}

	if apiResponse.EncodedLocations == "" {
		return nil, errors.New("trustboundary: invalid API response: encodedLocations is empty")
	}

	return NewTrustBoundaryData(apiResponse.Locations, apiResponse.EncodedLocations), nil
}

// ServiceAccountTrustBoundaryConfig holds configuration for SA trust boundary lookups.
// It implements the TrustBoundaryConfigProvider interface.
type ServiceAccountTrustBoundaryConfig struct {
	ServiceAccountEmail string
	UniverseDomain      string
}

// NewServiceAccountTrustBoundaryConfig creates a new config for service accounts.
func NewServiceAccountTrustBoundaryConfig(saEmail, universeDomain string) *ServiceAccountTrustBoundaryConfig {
	return &ServiceAccountTrustBoundaryConfig{
		ServiceAccountEmail: saEmail,
		UniverseDomain:      universeDomain,
	}
}

// GetTrustBoundaryEndpoint returns the formatted URL for fetching allowed locations
// for the configured service account and universe domain.
func (sac *ServiceAccountTrustBoundaryConfig) GetTrustBoundaryEndpoint(ctx context.Context) (url string, err error) {
	if sac.ServiceAccountEmail == "" {
		return "", errors.New("trustboundary: service account email cannot be empty for config")
	}
	ud := sac.UniverseDomain
	if ud == "" {
		ud = internal.DefaultUniverseDomain
	}
	return fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, ud, sac.ServiceAccountEmail), nil
}

// GetUniverseDomain returns the configured universe domain, defaulting to
// [internal.DefaultUniverseDomain] if not explicitly set.
func (sac *ServiceAccountTrustBoundaryConfig) GetUniverseDomain(ctx context.Context) (string, error) {
	if sac.UniverseDomain == "" {
		return internal.DefaultUniverseDomain, nil
	}
	return sac.UniverseDomain, nil
}

// dataProvider fetches and caches trust boundary Data.
// It implements the DataProvider interface and uses a ConfigProvider
// to get type-specific details for the lookup.
type dataProvider struct {
	client         *http.Client
	configProvider ConfigProvider
	data           *Data
	logger         *slog.Logger
}

// NewTrustBoundaryDataProvider creates a new DataProvider.
// It will use the provided HTTP client to make requests and the configProvider
// to determine the correct endpoint and universe domain for trust boundary lookups.
func NewTrustBoundaryDataProvider(client *http.Client, configProvider ConfigProvider, logger *slog.Logger) (DataProvider, error) {
	if client == nil {
		return nil, errors.New("trustboundary: HTTP client cannot be nil for TrustBoundaryDataProvider")
	}
	if configProvider == nil {
		return nil, errors.New("trustboundary: TrustBoundaryConfigProvider cannot be nil for TrustBoundaryDataProvider")
	}
	return &dataProvider{
		client:         client,
		configProvider: configProvider,
		logger:         internallog.New(logger),
	}, nil
}

// GetTrustBoundaryData retrieves the trust boundary data.
// It first checks the universe domain: if it's non-default, a NoOp is returned.
// Otherwise, it checks a local cache. If the data is not cached as NoOp,
// it fetches new data from the endpoint provided by its ConfigProvider,
// using the given accessToken for authentication. Results are cached.
// If fetching fails, it returns previously cached data if available, otherwise the fetch error.
func (p *dataProvider) GetTrustBoundaryData(ctx context.Context, accessToken string) (*Data, error) {
	// Check the universe domain.
	uniDomain, err := p.configProvider.GetUniverseDomain(ctx)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: error getting universe domain: %w", err)
	}
	if uniDomain != "" && uniDomain != internal.DefaultUniverseDomain {
		if p.data == nil || p.data.EncodedLocations != NoOpEncodedLocations {
			p.data = NewNoOpTrustBoundaryData()
		}
		return p.data, nil
	}

	// Check cache for a no-op result from a previous API call.
	cachedData := p.data
	if cachedData != nil && cachedData.EncodedLocations == NoOpEncodedLocations {
		return cachedData, nil
	}

	// Get the endpoint
	url, err := p.configProvider.GetTrustBoundaryEndpoint(ctx)
	if err != nil {
		return nil, fmt.Errorf("trustboundary: error getting the lookup endpoint: %w", err)
	}

	// Proceed to fetch new data.
	newData, fetchErr := fetchTrustBoundaryData(ctx, p.client, url, accessToken, p.logger)

	if fetchErr != nil {
		// Fetch failed. Fallback to cachedData if available.
		if cachedData != nil {
			return cachedData, nil // Successful fallback
		}
		// No cache to fallback to.
		return nil, fmt.Errorf("trustboundary: failed to fetch trust boundary data for endpoint %s and no cache available: %w", url, fetchErr)
	}

	// Fetch successful. Update cache.
	p.data = newData
	return newData, nil
}

// GCETrustBoundaryConfigProvider implements TrustBoundaryConfigProvider for GCE environments.
// It lazily fetches the necessary metadata (service account email, universe domain)
// from the GCE metadata server on each call to its methods.
type GCETrustBoundaryConfigProvider struct {
	// universeDomainProvider provides the universe domain and underlying metadata client.
	universeDomainProvider *internal.ComputeUniverseDomainProvider
}

// NewGCETrustBoundaryConfigProvider creates a new GCETrustBoundaryConfigProvider
// which uses the provided gceUDP to interact with the GCE metadata server.
func NewGCETrustBoundaryConfigProvider(gceUDP *internal.ComputeUniverseDomainProvider) ConfigProvider {
	// The validity of gceUDP and its internal MetadataClient will be checked
	// within the GetTrustBoundaryEndpoint and GetUniverseDomain methods.
	return &GCETrustBoundaryConfigProvider{
		universeDomainProvider: gceUDP,
	}
}

// GetTrustBoundaryEndpoint constructs the trust boundary lookup URL for a GCE environment.
// It fetches the default service account email and universe domain from the metadata server.
func (g *GCETrustBoundaryConfigProvider) GetTrustBoundaryEndpoint(ctx context.Context) (string, error) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		return "", errors.New("trustboundary: GCETrustBoundaryConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
	}
	mdClient := g.universeDomainProvider.MetadataClient
	saEmail, err := mdClient.EmailWithContext(ctx, "default")
	if err != nil {
		return "", fmt.Errorf("trustboundary: GCE config: failed to get service account email: %w", err)
	}
	ud, err := g.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		return "", fmt.Errorf("trustboundary: GCE config: failed to get universe domain: %w", err)
	}
	if ud == "" {
		ud = internal.DefaultUniverseDomain
	}
	return fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, ud, saEmail), nil
}

// GetUniverseDomain retrieves the universe domain from the GCE metadata server.
// If the metadata server returns an empty universe domain, it defaults to [internal.DefaultUniverseDomain
func (g *GCETrustBoundaryConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		return "", errors.New("trustboundary: GCETrustBoundaryConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
	}
	ud, err := g.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		return "", fmt.Errorf("trustboundary: GCE config: failed to get universe domain: %w", err)
	}
	if ud == "" {
		return internal.DefaultUniverseDomain, nil
	}
	return ud, nil
}
