// Copyright 2026 Google LLC
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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"regexp"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/retry"
	"cloud.google.com/go/auth/internal/transport/cert"
	"cloud.google.com/go/compute/metadata"
	"github.com/googleapis/gax-go/v2/internallog"
)

// ProviderKey is the key to fetch the DataProvider from Token Metadata.
const ProviderKey = "regionalaccessboundary.ProviderKey"

const (
	// serviceAccountAllowedLocationsEndpoint is the URL for fetching allowed locations for a given service account email.
	serviceAccountAllowedLocationsEndpoint = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s/allowedLocations"
	// serviceAccountAllowedLocationsMTLSEndpoint is the mTLS URL for fetching allowed locations for a given service account email.
	serviceAccountAllowedLocationsMTLSEndpoint = "https://iamcredentials.mtls.googleapis.com/v1/projects/-/serviceAccounts/%s/allowedLocations"

	// cacheTTL is the duration cached RAB data remains valid before hard expiry.
	cacheTTL = 6 * time.Hour
	// cacheSoftExpiry is the threshold before hard expiry where a background refresh is triggered.
	cacheSoftExpiry = 1 * time.Hour
	// baseCooldownDuration is the initial delay after a failed background fetch.
	baseCooldownDuration = 15 * time.Minute
)

var (
	// retryOptions configures the retry behavior for Regional Access Boundary lookups.
	retryOptions = &retry.Options{
		Initial:     1 * time.Second,
		Max:         60 * time.Second,
		Multiplier:  2.0,
		MaxAttempts: 6,
	}

	// ErrSkipRegionalAccessBoundary indicates that the Regional Access Boundary lookup
	// should be permanently skipped for this instance or credential.
	ErrSkipRegionalAccessBoundary = errors.New("regionalaccessboundary: skip lookup")

	emailRegexp = regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`)
)

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

	retryer := retry.NewWithOptions(retryOptions)
	startTime := time.Now()
	var response *http.Response
	for {
		response, err = client.Do(req)

		var statusCode int
		if response != nil {
			statusCode = response.StatusCode
		}
		pause, shouldRetry := retryer.Retry(statusCode, err)

		// Enforce a maximum 1 minute retry window for specific server errors.
		if shouldRetry && (statusCode == 500 || statusCode == 502 || statusCode == 503 || statusCode == 504) {
			if time.Since(startTime)+pause > 1*time.Minute {
				shouldRetry = false
			}
		}

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
	skipLookup       bool          // permanently skips RAB lookup if the identity is unsupported
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

	// Clone the token and its metadata to avoid mutating shared/cached state.
	newToken := *token
	newToken.Metadata = maps.Clone(token.Metadata)
	if newToken.Metadata == nil {
		newToken.Metadata = make(map[string]interface{})
	}
	newToken.Metadata[ProviderKey] = p

	return &newToken, nil
}

// GetHeaderValue immediately returns a valid header if it's cached, or kicks off a background fetch
// if it is unpopulated or expired.
func (p *DataProvider) GetHeaderValue(ctx context.Context, reqURL string, accessToken *auth.Token) string {
	p.mu.RLock()
	skip := p.skipLookup
	data := p.data
	dataExpiry := p.dataExpiry
	p.mu.RUnlock()

	if skip {
		return ""
	}

	if !strings.Contains(reqURL, "://") {
		reqURL = "https://" + reqURL
	}
	if u, err := url.Parse(reqURL); err == nil {
		host := u.Host
		if host == "" && strings.HasPrefix(u.Path, "/") {
			host = strings.TrimPrefix(u.Path, "/")
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Skip lookup for regional endpoints.
		if host == "rep.googleapis.com" || strings.HasSuffix(host, ".rep.googleapis.com") ||
			host == "rep.sandbox.googleapis.com" || strings.HasSuffix(host, ".rep.sandbox.googleapis.com") {
			return ""
		}
		// Skip lookup for IAM and STS endpoints as they do not require RAB headers.
		if host == "iam.googleapis.com" || host == "iamcredentials.googleapis.com" ||
			host == "sts.googleapis.com" {
			return ""
		}
	}

	// Skip lookup for non-default universe domains.
	uniDomain, err := p.configProvider.GetUniverseDomain(ctx)
	if err != nil {
		p.logger.WarnContext(ctx, "regionalaccessboundary: error getting universe domain", "error", err)
		return ""
	}
	if uniDomain != "" && uniDomain != internal.DefaultUniverseDomain {
		return ""
	}

	now := time.Now()
	if data != nil && now.Before(dataExpiry) {
		val, _ := data.RegionalAccessBoundaryHeader()

		// Soft Expiry: if the cached data is within the soft expiration window,
		// initiate a non-blocking background refresh to proactively fetch new data
		// while continuing to serve the current valid cache block.
		if now.After(dataExpiry.Add(-cacheSoftExpiry)) {
			p.mu.Lock()
			if !p.isFetching && now.After(p.cooldownExpiry) {
				p.isFetching = true
				go p.fetchAsync(context.Background(), accessToken)
			}
			p.mu.Unlock()
		}

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

// fetchAsync performs the background lookup for Regional Access Boundary data.
// It updates the provider's state based on the result (success or failure).
func (p *DataProvider) fetchAsync(ctx context.Context, accessToken *auth.Token) {
	var cloned bool
	var fetchClient *http.Client
	defer func() {
		p.mu.Lock()
		p.isFetching = false
		p.mu.Unlock()
		if cloned && fetchClient != nil {
			fetchClient.CloseIdleConnections()
		}
	}()

	url, err := p.configProvider.GetRegionalAccessBoundaryEndpoint(ctx)
	if errors.Is(err, ErrSkipRegionalAccessBoundary) {
		// If the compute environment or identity does not support Regional Access Boundary
		// lookups, permanently disable subsequent attempts to avoid redundant retries.
		p.mu.Lock()
		p.skipLookup = true
		p.mu.Unlock()
		return
	}
	if err != nil {
		p.logger.WarnContext(ctx, "regionalaccessboundary: error getting the lookup endpoint", "error", err)
		p.handleFetchFailure(ctx)
		return
	}

	fetchClient = p.client
	if strings.Contains(url, ".mtls.") {
		if provider, err := cert.DefaultProvider(); err == nil && provider != nil {
			clonedClient := maybeCloneClientWithMTLS(p.client, provider)
			if clonedClient != p.client {
				fetchClient = clonedClient
				cloned = true
			}
		}
	}
	newData, fetchErr := fetchRegionalAccessBoundaryData(ctx, fetchClient, url, accessToken, p.logger)

	if fetchErr != nil {
		p.logger.WarnContext(ctx, "regionalaccessboundary: async fetch failed", "error", fetchErr)
		p.handleFetchFailure(ctx)
		return
	}

	p.handleFetchSuccess(newData)
}

// handleFetchSuccess updates the cache with new data and clears any existing cooldown.
func (p *DataProvider) handleFetchSuccess(newData *internal.RegionalAccessBoundaryData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = newData
	p.dataExpiry = time.Now().Add(cacheTTL)
	p.cooldownExpiry = time.Time{}
	p.cooldownDuration = baseCooldownDuration
}

// handleFetchFailure triggers the cooldown period using exponential backoff.
func (p *DataProvider) handleFetchFailure(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Add random bounded jitter (between half of the base and the full base) to prevent thundering herds
	jitter := p.cooldownDuration/2 + time.Duration(rand.Int63n(int64(p.cooldownDuration/2)))
	p.cooldownExpiry = time.Now().Add(jitter)

	// Exponential backoff for the NEXT attempt, up to cacheTTL max (6 hours)
	nextCooldown := p.cooldownDuration * 2
	if nextCooldown > cacheTTL {
		nextCooldown = cacheTTL
	}
	p.cooldownDuration = nextCooldown
}

func resolveLocalMTLSEndpoint(base, mtls string) (string, error) {
	mode := strings.ToLower(os.Getenv("GOOGLE_API_USE_MTLS_ENDPOINT"))
	if mode == "" {
		mode = strings.ToLower(os.Getenv("GOOGLE_API_USE_MTLS")) // Deprecated.
	}
	if mode == "always" {
		return mtls, nil
	}
	if mode == "never" {
		return base, nil
	}

	// Honor explicit client certificate suppression matching transport.isClientCertificateEnabled.
	// We ignore parsing errors to default to false, matching the behavior in cba.go.
	if val, ok := os.LookupEnv("GOOGLE_API_USE_CLIENT_CERTIFICATE"); ok {
		b, _ := strconv.ParseBool(val)
		if !b {
			return base, nil
		}
	}

	// Check if a client certificate is available. If an error occurs (e.g., malformed
	// certificate configuration file), propagate the error up to trigger cooldown.
	provider, err := cert.DefaultProvider()
	if err != nil {
		return "", err
	}
	if provider != nil {
		return mtls, nil
	}
	return base, nil
}

type cloneableTransport interface {
	Clone() *http.Transport
}

// maybeCloneClientWithMTLS returns a clone of client configured to use the client
// certificate provider if the client's transport is cloneable. If the transport
// cannot be cloned, it returns the original client unmodified.
func maybeCloneClientWithMTLS(client *http.Client, provider cert.Provider) *http.Client {
	if client == nil {
		return nil
	}
	c := *client
	var trans *http.Transport
	if client.Transport == nil {
		if defaultTrans, ok := http.DefaultTransport.(cloneableTransport); ok {
			trans = defaultTrans.Clone()
		} else {
			// Fallback to a clean transport configured with standard timeouts
			// and ForceAttemptHTTP2 enabled.
			trans = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			}
		}
	} else if cloneable, ok := client.Transport.(cloneableTransport); ok {
		trans = cloneable.Clone()
		if trans == nil {
			return client
		}
	} else {
		// If transport is not cloneable (e.g. custom wrapper), we return the client unmodified.
		return client
	}

	if trans.TLSClientConfig == nil {
		trans.TLSClientConfig = &tls.Config{}
	} else {
		trans.TLSClientConfig = trans.TLSClientConfig.Clone()
	}

	// Only set GetClientCertificate if the user has not already configured
	// client certificates on this transport.
	if trans.TLSClientConfig.GetClientCertificate == nil && len(trans.TLSClientConfig.Certificates) == 0 {
		trans.TLSClientConfig.GetClientCertificate = provider
	}

	c.Transport = trans
	return &c
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
	base := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, sac.ServiceAccountEmail)
	mtls := fmt.Sprintf(serviceAccountAllowedLocationsMTLSEndpoint, sac.ServiceAccountEmail)
	return resolveLocalMTLSEndpoint(base, mtls)
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
// It lazily fetches and caches the necessary service account email metadata.
type GCEConfigProvider struct {
	// universeDomainProvider provides the universe domain and underlying metadata client.
	universeDomainProvider *internal.ComputeUniverseDomainProvider
	logger                 *slog.Logger

	// Caching for service account email
	saMu       sync.Mutex
	saEmail    string
	skipLookup bool
}

// NewGCEConfigProvider creates a new GCEConfigProvider
// which uses the provided gceUDP to interact with the GCE metadata server.
func NewGCEConfigProvider(gceUDP *internal.ComputeUniverseDomainProvider, logger *slog.Logger) *GCEConfigProvider {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	// The validity of gceUDP and its internal MetadataClient will be checked
	// within the GetRegionalAccessBoundaryEndpoint and GetUniverseDomain methods.
	return &GCEConfigProvider{
		universeDomainProvider: gceUDP,
		logger:                 internallog.New(logger),
	}
}

func (g *GCEConfigProvider) fetchSA(ctx context.Context) (string, error) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		return "", errors.New("regionalaccessboundary: GCEConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
	}
	mdClient := g.universeDomainProvider.MetadataClient
	saEmail, err := mdClient.EmailWithContext(ctx, "default")
	if err != nil {
		var ndErr metadata.NotDefinedError
		if errors.As(err, &ndErr) {
			return "", ErrSkipRegionalAccessBoundary
		}
		return "", fmt.Errorf("regionalaccessboundary: GCE config: failed to get service account email: %w", err)
	}
	return saEmail, nil
}

// GetRegionalAccessBoundaryEndpoint constructs the Regional Access Boundary lookup URL for a GCE environment.
// It uses cached service account email after the first call.
func (g *GCEConfigProvider) GetRegionalAccessBoundaryEndpoint(ctx context.Context) (string, error) {
	// Check if we already have a cached service account email.
	g.saMu.Lock()
	if g.skipLookup {
		g.saMu.Unlock()
		return "", ErrSkipRegionalAccessBoundary
	}
	if g.saEmail != "" {
		email := g.saEmail
		g.saMu.Unlock()
		base := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, email)
		mtls := fmt.Sprintf(serviceAccountAllowedLocationsMTLSEndpoint, email)
		return resolveLocalMTLSEndpoint(base, mtls)
	}
	g.saMu.Unlock()

	// Fetch the email from the metadata server. We do not hold the lock
	// during this I/O operation to avoid blocking other goroutines.
	email, err := g.fetchSA(ctx)
	if err != nil {
		if errors.Is(err, ErrSkipRegionalAccessBoundary) {
			g.saMu.Lock()
			g.skipLookup = true
			g.saMu.Unlock()
		}
		return "", err
	}

	if !emailRegexp.MatchString(email) {
		// If the metadata server response does not look like a standard email (e.g.,
		// a GKE Workload Identity pool ID or principal), permanently skip RAB lookups.
		g.saMu.Lock()
		alreadySkipped := g.skipLookup
		g.skipLookup = true
		g.saMu.Unlock()
		if !alreadySkipped && g.logger != nil {
			g.logger.InfoContext(ctx, "regionalaccessboundary: RAB lookup is skipped for this instance", "response", email)
		}
		return "", ErrSkipRegionalAccessBoundary
	}

	// Cache the successful result.
	g.saMu.Lock()
	g.saEmail = email
	g.saMu.Unlock()

	base := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, email)
	mtls := fmt.Sprintf(serviceAccountAllowedLocationsMTLSEndpoint, email)
	return resolveLocalMTLSEndpoint(base, mtls)
}

// GetUniverseDomain retrieves the universe domain from the GCE metadata server.
func (g *GCEConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	if g.universeDomainProvider == nil || g.universeDomainProvider.MetadataClient == nil {
		return "", errors.New("regionalaccessboundary: GCEConfigProvider not properly initialized (missing ComputeUniverseDomainProvider or MetadataClient)")
	}
	ud, err := g.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		return "", fmt.Errorf("regionalaccessboundary: GCE config: failed to get universe domain: %w", err)
	}
	if ud == "" {
		return internal.DefaultUniverseDomain, nil
	}
	return ud, nil
}
