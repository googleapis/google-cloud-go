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

package regionalaccessboundary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/retry"
	"github.com/googleapis/gax-go/v2/internallog"
)

// ProviderKey is the key to fetch the DataProvider from Token Metadata.
const ProviderKey = "regionalaccessboundary.ProviderKey"

const (
	// serviceAccountAllowedLocationsEndpoint is the URL for fetching allowed locations for a given service account email.
	serviceAccountAllowedLocationsEndpoint = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s/allowedLocations"
)

// isEnabled wraps isRegionalAccessBoundaryEnabled with sync.OnceValues to ensure it's
// called only once.
var isEnabled = sync.OnceValues(isRegionalAccessBoundaryEnabled)

// IsEnabled returns if the Regional Access Boundary feature is enabled and an error if
// the configuration is invalid. The underlying check is performed only once.
func IsEnabled() (bool, error) {
	return isEnabled()
}

// isRegionalAccessBoundaryEnabled checks if the Regional Access Boundary feature is enabled via
// GOOGLE_AUTH_TRUST_BOUNDARY_ENABLED environment variable.
//
// If the environment variable is not set, it is considered false.
//
// The environment variable is interpreted as a boolean with the following
// (case-insensitive) rules:
//   - "true", "1" are considered true.
//   - "false", "0" are considered false.
//
// Any other values will return an error.
func isRegionalAccessBoundaryEnabled() (bool, error) {
	newEnvVar := "GOOGLE_AUTH_REGIONAL_ACCESS_BOUNDARY_ENABLE_EXPERIMENT"
	oldEnvVar := "GOOGLE_AUTH_TRUST_BOUNDARY_ENABLED"

	// Check new environment variable first.
	if val, ok := os.LookupEnv(newEnvVar); ok {
		val = strings.ToLower(val)
		if val == "true" || val == "1" {
			return true, nil
		}
		return false, nil // Ignore other values, default to false
	}

	// Fallback to old environment variable.
	if val, ok := os.LookupEnv(oldEnvVar); ok {
		val = strings.ToLower(val)
		if val == "true" || val == "1" {
			return true, nil
		}
		return false, nil // Ignore other values, default to false
	}
	return false, nil
}

// ConfigProvider provides specific configuration for Regional Access Boundary lookups.
type ConfigProvider interface {
	// GetRegionalAccessBoundaryEndpoint returns the endpoint URL for the Regional Access Boundary lookup.
	GetRegionalAccessBoundaryEndpoint(ctx context.Context) (url string, err error)
	// GetUniverseDomain returns the universe domain associated with the credential.
	// It may return an error if the universe domain cannot be determined.
	GetUniverseDomain(ctx context.Context) (string, error)
}

// AllowedLocationsResponse is the structure of the response from the Regional Access Boundary API.
type AllowedLocationsResponse struct {
	// Locations is the list of allowed locations.
	Locations []string `json:"locations"`
	// EncodedLocations is the encoded representation of the allowed locations.
	EncodedLocations string `json:"encodedLocations"`
}

// fetchRegionalAccessBoundaryData fetches the Regional Access Boundary data from the API.
func fetchRegionalAccessBoundaryData(ctx context.Context, client *http.Client, url string, token *auth.Token, logger *slog.Logger) (*internal.RegionalAccessBoundaryData, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if client == nil {
		return nil, errors.New("regionalaccessboundary: HTTP client is required")
	}

	if url == "" {
		return nil, errors.New("regionalaccessboundary: URL cannot be empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("regionalaccessboundary: failed to create Regional Access Boundary request: %w", err)
	}

	if token == nil || token.Value == "" {
		return nil, errors.New("regionalaccessboundary: access token required for lookup API authentication")
	}
	typ := token.Type
	if typ == "" {
		typ = internal.TokenTypeBearer
	}
	req.Header.Set("Authorization", typ+" "+token.Value)
	logger.DebugContext(ctx, "Regional Access Boundary request", "request", internallog.HTTPRequest(req, nil))

	retryer := retry.New()
	var response *http.Response
	for {
		response, err = client.Do(req)

		var statusCode int
		if response != nil {
			statusCode = response.StatusCode
		}
		pause, shouldRetry := retryer.Retry(statusCode, err)

		if !shouldRetry {
			break
		}

		if response != nil {
			// Drain and close the body to reuse the connection
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}

		if err := retry.Sleep(ctx, pause); err != nil {
			return nil, err
		}
	}

	if err != nil {
		return nil, fmt.Errorf("regionalaccessboundary: failed to fetch Regional Access Boundary: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("regionalaccessboundary: failed to read Regional Access Boundary response: %w", err)
	}

	logger.DebugContext(ctx, "Regional Access Boundary response", "response", internallog.HTTPResponse(response, body))

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("regionalaccessboundary: Regional Access Boundary request failed with status: %s, body: %s", response.Status, string(body))
	}

	apiResponse := AllowedLocationsResponse{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("regionalaccessboundary: failed to unmarshal Regional Access Boundary response: %w", err)
	}

	if apiResponse.EncodedLocations == "" {
		return nil, errors.New("regionalaccessboundary: invalid API response: encodedLocations is empty")
	}

	return internal.NewRegionalAccessBoundaryData(apiResponse.Locations, apiResponse.EncodedLocations), nil
}

// DataProvider fetches and caches Regional Access Boundary Data.
// It implements the auth.TokenProvider interface and uses a ConfigProvider
// to get type-specific details for the lookup.
type DataProvider struct {
	client         *http.Client
	configProvider ConfigProvider
	logger         *slog.Logger
	base           auth.TokenProvider

	mu               sync.RWMutex
	data             *internal.RegionalAccessBoundaryData
	dataExpiry       time.Time
	isFetching       bool
	cooldownExpiry   time.Time
	cooldownDuration time.Duration // tracks the current cooldown duration for exponential backoff
}

// NewProvider wraps the provided base [auth.TokenProvider] and returns a new
// provider that fetches and caches the Regional Access Boundary data. It uses
// the provided HTTP client and configProvider.
func NewProvider(client *http.Client, configProvider ConfigProvider, logger *slog.Logger, base auth.TokenProvider) (*DataProvider, error) {
	if client == nil {
		return nil, errors.New("regionalaccessboundary: HTTP client cannot be nil for DataProvider")
	}
	if configProvider == nil {
		return nil, errors.New("regionalaccessboundary: ConfigProvider cannot be nil for DataProvider")
	}
	p := &DataProvider{
		client:           client,
		configProvider:   configProvider,
		logger:           internallog.New(logger),
		base:             base,
		cooldownDuration: 15 * time.Minute,
	}
	return p, nil
}

// Token retrieves a token from the base provider and injects the DataProvider
// instance into its metadata.
func (p *DataProvider) Token(ctx context.Context) (*auth.Token, error) {
	token, err := p.base.Token(ctx)
	if err != nil {
		return nil, err
	}

	if token.Metadata == nil {
		token.Metadata = make(map[string]interface{})
	}

	token.Metadata[ProviderKey] = p
	return token, nil
}

// It immediately returns a valid header if it's cached, or kicks off a background fetch
// if it is unpopulated or expired.
func (p *DataProvider) GetHeaderValue(ctx context.Context, reqURL string, accessToken *auth.Token) string {
	// Skip lookup for regional endpoints.
	if strings.Contains(reqURL, "rep.googleapis.com") || strings.Contains(reqURL, "rep.sandbox.googleapis.com") {
		return ""
	}

	// Skip lookup for non-default universe domains.
	uniDomain, err := p.configProvider.GetUniverseDomain(ctx)
	if err == nil && uniDomain != "" && uniDomain != internal.DefaultUniverseDomain {
		return ""
	}

	// Return the cached data if present and not expired.
	p.mu.RLock()
	data := p.data
	dataExpiry := p.dataExpiry
	p.mu.RUnlock()

	now := time.Now()
	if data != nil && now.Before(dataExpiry) {
		val, _ := data.RegionalAccessBoundaryHeader()
		return val
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Skip lookup if in cooldown or another process is already fetching.
	if p.isFetching || time.Now().Before(p.cooldownExpiry) {
		return ""
	}

	// Start async RAB lookup and return empty header.
	p.isFetching = true
	go p.fetchAsync(context.Background(), accessToken)
	return ""
}

func (p *DataProvider) fetchAsync(ctx context.Context, accessToken *auth.Token) {
	defer func() {
		p.mu.Lock()
		p.isFetching = false
		p.mu.Unlock()
	}()

	url, err := p.configProvider.GetRegionalAccessBoundaryEndpoint(ctx)
	if err != nil {
		p.logger.ErrorContext(ctx, "regionalaccessboundary: error getting the lookup endpoint", "error", err)
		return
	}

	newData, fetchErr := fetchRegionalAccessBoundaryData(ctx, p.client, url, accessToken, p.logger)

	if fetchErr != nil {
		p.logger.ErrorContext(ctx, "regionalaccessboundary: async fetch failed", "error", fetchErr)
		p.handleFetchFailure(ctx)
		return
	}

	p.handleFetchSuccess(newData)
}

func (p *DataProvider) handleFetchSuccess(newData *internal.RegionalAccessBoundaryData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = newData
	p.dataExpiry = time.Now().Add(6 * time.Hour) // 6 hour TTL
	p.cooldownExpiry = time.Time{}               // reset cooldown
	p.cooldownDuration = 15 * time.Minute        // reset cooldown multiplier
}

func (p *DataProvider) handleFetchFailure(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cooldownExpiry = time.Now().Add(p.cooldownDuration)

	// Exponential backoff for cooldown, up to 6 hours max
	nextCooldown := p.cooldownDuration * 2
	maxCooldown := 6 * time.Hour
	if nextCooldown > maxCooldown {
		nextCooldown = maxCooldown
	}
	p.cooldownDuration = nextCooldown
}

// serviceAccountConfig holds configuration for SA Regional Access Boundary lookups.
// It implements the ConfigProvider interface.
type serviceAccountConfig struct {
	ServiceAccountEmail string
	UniverseDomain      string
}

// NewServiceAccountConfigProvider creates a new config for service accounts.
func NewServiceAccountConfigProvider(saEmail, universeDomain string) ConfigProvider {
	return &serviceAccountConfig{
		ServiceAccountEmail: saEmail,
		UniverseDomain:      universeDomain,
	}
}

// GetRegionalAccessBoundaryEndpoint returns the formatted URL for fetching allowed locations
// for the configured service account.
func (sac *serviceAccountConfig) GetRegionalAccessBoundaryEndpoint(ctx context.Context) (url string, err error) {
	if sac.ServiceAccountEmail == "" {
		return "", errors.New("regionalaccessboundary: service account email cannot be empty for config")
	}
	return fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, sac.ServiceAccountEmail), nil
}

// GetUniverseDomain returns the configured universe domain, defaulting to
// [internal.DefaultUniverseDomain] if not explicitly set.
func (sac *serviceAccountConfig) GetUniverseDomain(ctx context.Context) (string, error) {
	if sac.UniverseDomain == "" {
		return internal.DefaultUniverseDomain, nil
	}
	return sac.UniverseDomain, nil
}

// GCEConfigProvider implements ConfigProvider for GCE environments.
// It lazily fetches and caches the necessary metadata (service account email, universe domain)
type GCEConfigProvider struct {
	// universeDomainProvider provides the universe domain and underlying metadata client.
	universeDomainProvider *internal.ComputeUniverseDomainProvider

	// Caching for service account email
	saOnce     sync.Once
	saEmail    string
	saEmailErr error

	// Caching for universe domain
	udOnce sync.Once
	ud     string
	udErr  error
}

// NewGCEConfigProvider creates a new GCEConfigProvider
// which uses the provided gceUDP to interact with the GCE metadata server.
func NewGCEConfigProvider(gceUDP *internal.ComputeUniverseDomainProvider) *GCEConfigProvider {
	// The validity of gceUDP and its internal MetadataClient will be checked
	// within the GetRegionalAccessBoundaryEndpoint and GetUniverseDomain methods.
	return &GCEConfigProvider{
		universeDomainProvider: gceUDP,
	}
}

func (g *GCEConfigProvider) fetchSA(ctx context.Context) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		g.saEmailErr = errors.New("regionalaccessboundary: GCEConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
		return
	}
	mdClient := g.universeDomainProvider.MetadataClient
	saEmail, err := mdClient.EmailWithContext(ctx, "default")
	if err != nil {
		g.saEmailErr = fmt.Errorf("regionalaccessboundary: GCE config: failed to get service account email: %w", err)
		return
	}
	g.saEmail = saEmail
}

func (g *GCEConfigProvider) fetchUD(ctx context.Context) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		g.udErr = errors.New("regionalaccessboundary: GCEConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
		return
	}
	ud, err := g.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		g.udErr = fmt.Errorf("regionalaccessboundary: GCE config: failed to get universe domain: %w", err)
		return
	}
	if ud == "" {
		ud = internal.DefaultUniverseDomain
	}
	g.ud = ud
}

// GetRegionalAccessBoundaryEndpoint constructs the Regional Access Boundary lookup URL for a GCE environment.
// It uses cached metadata (service account email, universe domain) after the first call.
func (g *GCEConfigProvider) GetRegionalAccessBoundaryEndpoint(ctx context.Context) (string, error) {
	g.saOnce.Do(func() { g.fetchSA(ctx) })
	if g.saEmailErr != nil {
		return "", g.saEmailErr
	}
	g.udOnce.Do(func() { g.fetchUD(ctx) })
	if g.udErr != nil {
		return "", g.udErr
	}
	return fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, g.saEmail), nil
}

// GetUniverseDomain retrieves the universe domain from the GCE metadata server.
// It uses a cached value after the first call.
func (g *GCEConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	g.udOnce.Do(func() { g.fetchUD(ctx) })
	if g.udErr != nil {
		return "", g.udErr
	}
	return g.ud, nil
}
