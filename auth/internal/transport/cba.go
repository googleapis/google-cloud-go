// Copyright 2023 Google LLC
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

package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/auth/internal/transport/cert"
	"cloud.google.com/go/compute/metadata"
	"github.com/google/s2a-go"
	"github.com/google/s2a-go/fallback"
	"google.golang.org/grpc/credentials"
)

const (
	mTLSModeAlways = "always"
	mTLSModeNever  = "never"
	mTLSModeAuto   = "auto"

	configEndpointSuffix = "googleAutoMtlsConfiguration"

	// Experimental: if true, the code will try MTLS with S2A as the default for transport security. Default value is false.
	googleAPIUseS2AEnv     = "EXPERIMENTAL_GOOGLE_API_USE_S2A"
	googleAPIUseCertSource = "GOOGLE_API_USE_CLIENT_CERTIFICATE"
	googleAPIUseMTLS       = "GOOGLE_API_USE_MTLS_ENDPOINT"
	googleAPIUseMTLSOld    = "GOOGLE_API_USE_MTLS"
)

var (
	// The period an MTLS config can be reused before needing refresh.
	configExpiry = time.Hour

	// mtlsEndpointEnabledForS2A checks if the endpoint is indeed MTLS-enabled, so that we can use S2A for MTLS connection.
	mtlsEndpointEnabledForS2A = func() bool {
		// TODO(xmenxk): determine this via discovery config.
		return true
	}

	// mdsMTLSAutoConfigSource is an instance of reuseMTLSConfigSource, with metadataMTLSAutoConfig as its config source.
	mtlsOnce                sync.Once
	mdsMTLSAutoConfigSource mtlsConfigSource
)

// Options is a struct that is duplicated information from the individual
// transport packages in order to avoid cyclic deps.
type Options struct {
	Endpoint            string
	DefaultEndpoint     string
	DefaultMTLSEndpoint string
	ClientCertProvider  cert.Provider
	Client              *http.Client
}

// GetGRPCTransportConfigAndEndpoint returns an instance of credentials.TransportCredentials, and the
// corresponding endpoint to use for GRPC client.
func GetGRPCTransportConfigAndEndpoint(opts *Options) (credentials.TransportCredentials, string, error) {
	config, err := getTransportConfig(opts)
	if err != nil {
		return nil, "", err
	}

	defaultTransportCreds := credentials.NewTLS(&tls.Config{
		GetClientCertificate: config.clientCertSource,
	})
	if config.s2aAddress == "" {
		return defaultTransportCreds, config.endpoint, nil
	}

	var fallbackOpts *s2a.FallbackOptions
	// In case of S2A failure, fall back to the endpoint that would've been used without S2A.
	if fallbackHandshake, err := fallback.DefaultFallbackClientHandshakeFunc(config.endpoint); err == nil {
		fallbackOpts = &s2a.FallbackOptions{
			FallbackClientHandshakeFunc: fallbackHandshake,
		}
	}

	s2aTransportCreds, err := s2a.NewClientCreds(&s2a.ClientOptions{
		S2AAddress:   config.s2aAddress,
		FallbackOpts: fallbackOpts,
	})
	if err != nil {
		// Use default if we cannot initialize S2A client transport credentials.
		return defaultTransportCreds, config.endpoint, nil
	}
	return s2aTransportCreds, config.s2aMTLSEndpoint, nil
}

// GetHTTPTransportConfig returns a client certificate source, a function for dialing MTLS with S2A,
// and the endpoint to use for HTTP client.
func GetHTTPTransportConfig(opts *Options) (cert.Provider, func(context.Context, string, string) (net.Conn, error), error) {
	config, err := getTransportConfig(opts)
	if err != nil {
		return nil, nil, err
	}

	if config.s2aAddress == "" {
		return config.clientCertSource, nil, nil
	}

	var fallbackOpts *s2a.FallbackOptions
	// In case of S2A failure, fall back to the endpoint that would've been used without S2A.
	if fallbackURL, err := url.Parse(config.endpoint); err == nil {
		if fallbackDialer, fallbackServerAddr, err := fallback.DefaultFallbackDialerAndAddress(fallbackURL.Hostname()); err == nil {
			fallbackOpts = &s2a.FallbackOptions{
				FallbackDialer: &s2a.FallbackDialer{
					Dialer:     fallbackDialer,
					ServerAddr: fallbackServerAddr,
				},
			}
		}
	}

	dialTLSContextFunc := s2a.NewS2ADialTLSContextFunc(&s2a.ClientOptions{
		S2AAddress:   config.s2aAddress,
		FallbackOpts: fallbackOpts,
	})
	return nil, dialTLSContextFunc, nil
}

func getTransportConfig(opts *Options) (*transportConfig, error) {
	clientCertSource, err := getClientCertificateSource(opts)
	if err != nil {
		return &transportConfig{}, err
	}
	endpoint, err := getEndpoint(opts, clientCertSource)
	if err != nil {
		return &transportConfig{}, err
	}
	defaultTransportConfig := transportConfig{
		clientCertSource: clientCertSource,
		endpoint:         endpoint,
	}

	if !shouldUseS2A(clientCertSource, opts) {
		return &defaultTransportConfig, nil
	}

	s2aMTLSEndpoint := opts.DefaultMTLSEndpoint
	// If there is endpoint override, honor it.
	if opts.Endpoint != "" {
		s2aMTLSEndpoint = endpoint
	}
	s2aAddress := GetS2AAddress()
	if s2aAddress == "" {
		return &defaultTransportConfig, nil
	}
	return &transportConfig{
		clientCertSource: clientCertSource,
		endpoint:         endpoint,
		s2aAddress:       s2aAddress,
		s2aMTLSEndpoint:  s2aMTLSEndpoint,
	}, nil
}

// getClientCertificateSource returns a default client certificate source, if
// not provided by the user.
//
// A nil default source can be returned if the source does not exist. Any exceptions
// encountered while initializing the default source will be reported as client
// error (ex. corrupt metadata file).
//
// Important Note: For now, the environment variable GOOGLE_API_USE_CLIENT_CERTIFICATE
// must be set to "true" to allow certificate to be used (including user provided
// certificates). For details, see AIP-4114.
func getClientCertificateSource(opts *Options) (cert.Provider, error) {
	if !isClientCertificateEnabled() {
		return nil, nil
	} else if opts.ClientCertProvider != nil {
		return opts.ClientCertProvider, nil
	} else {
		return cert.DefaultSource()
	}
}

func isClientCertificateEnabled() bool {
	useClientCert := os.Getenv(googleAPIUseCertSource)
	// TODO(andyrzhao): Update default to return "true" after DCA feature is fully released.
	return strings.ToLower(useClientCert) == "true"
}

type transportConfig struct {
	// The client certificate source.
	clientCertSource cert.Provider
	// The corresponding endpoint to use based on client certificate source.
	endpoint string
	// The S2A address if it can be used, otherwise an empty string.
	s2aAddress string
	// The MTLS endpoint to use with S2A.
	s2aMTLSEndpoint string
}

// getEndpoint returns the endpoint for the service, taking into account the
// user-provided endpoint override "settings.Endpoint".
//
// If no endpoint override is specified, we will either return the default endpoint or
// the default mTLS endpoint if a client certificate is available.
//
// You can override the default endpoint choice (mtls vs. regular) by setting the
// GOOGLE_API_USE_MTLS_ENDPOINT environment variable.
//
// If the endpoint override is an address (host:port) rather than full base
// URL (ex. https://...), then the user-provided address will be merged into
// the default endpoint. For example, WithEndpoint("myhost:8000") and
// WithDefaultEndpoint("https://foo.com/bar/baz") will return "https://myhost:8080/bar/baz"
func getEndpoint(opts *Options, clientCertSource cert.Provider) (string, error) {
	if opts.Endpoint == "" {
		mtlsMode := getMTLSMode()
		if mtlsMode == mTLSModeAlways || (clientCertSource != nil && mtlsMode == mTLSModeAuto) {
			return opts.DefaultMTLSEndpoint, nil
		}
		return opts.DefaultEndpoint, nil
	}
	if strings.Contains(opts.Endpoint, "://") {
		// User passed in a full URL path, use it verbatim.
		return opts.Endpoint, nil
	}
	if opts.DefaultEndpoint == "" {
		// If DefaultEndpoint is not configured, use the user provided endpoint verbatim.
		// This allows a naked "host[:port]" URL to be used with GRPC Direct Path.
		return opts.Endpoint, nil
	}

	// Assume user-provided endpoint is host[:port], merge it with the default endpoint.
	return mergeEndpoints(opts.DefaultEndpoint, opts.Endpoint)
}

func getMTLSMode() string {
	mode := os.Getenv(googleAPIUseMTLS)
	if mode == "" {
		mode = os.Getenv(googleAPIUseMTLSOld) // Deprecated.
	}
	if mode == "" {
		return mTLSModeAuto
	}
	return strings.ToLower(mode)
}

func mergeEndpoints(baseURL, newHost string) (string, error) {
	u, err := url.Parse(fixScheme(baseURL))
	if err != nil {
		return "", err
	}
	return strings.Replace(baseURL, u.Host, newHost, 1), nil
}

func fixScheme(baseURL string) string {
	if !strings.Contains(baseURL, "://") {
		return "https://" + baseURL
	}
	return baseURL
}

// GetS2AAddress returns the S2A address to be reached via plaintext connection.
func GetS2AAddress() string {
	c, err := getMetadataMTLSAutoConfig().Config()
	if err != nil {
		return ""
	}
	if !c.Valid() {
		return ""
	}
	return c.S2A.PlaintextAddress
}

type mtlsConfigSource interface {
	Config() (*mtlsConfig, error)
}

// mtlsConfig contains the configuration for establishing MTLS connections with Google APIs.
type mtlsConfig struct {
	S2A    *s2aAddresses `json:"s2a"`
	Expiry time.Time
}

func (c *mtlsConfig) Valid() bool {
	return c != nil && c.S2A != nil && !c.expired()
}
func (c *mtlsConfig) expired() bool {
	return c.Expiry.Before(time.Now())
}

// s2aAddresses contains the plaintext and/or MTLS S2A addresses.
type s2aAddresses struct {
	// PlaintextAddress is the plaintext address to reach S2A
	PlaintextAddress string `json:"plaintext_address"`
	// MTLSAddress is the MTLS address to reach S2A
	MTLSAddress string `json:"mtls_address"`
}

// getMetadataMTLSAutoConfig returns mdsMTLSAutoConfigSource, which is backed by config from MDS with auto-refresh.
func getMetadataMTLSAutoConfig() mtlsConfigSource {
	mtlsOnce.Do(func() {
		mdsMTLSAutoConfigSource = &reuseMTLSConfigSource{
			src: &metadataMTLSAutoConfig{},
		}
	})
	return mdsMTLSAutoConfigSource
}

// reuseMTLSConfigSource caches a valid version of mtlsConfig, and uses `src` to refresh upon config expiry.
// It implements the mtlsConfigSource interface, so calling Config() on it returns an mtlsConfig.
type reuseMTLSConfigSource struct {
	src    mtlsConfigSource // src.Config() is called when config is expired
	mu     sync.Mutex       // mutex guards config
	config *mtlsConfig      // cached config
}

func (cs *reuseMTLSConfigSource) Config() (*mtlsConfig, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.config.Valid() {
		return cs.config, nil
	}
	c, err := cs.src.Config()
	if err != nil {
		return nil, err
	}
	cs.config = c
	return c, nil
}

// metadataMTLSAutoConfig is an implementation of the interface mtlsConfigSource
// It has the logic to query MDS and return an mtlsConfig
type metadataMTLSAutoConfig struct{}

var httpGetMetadataMTLSConfig = func() (string, error) {
	return metadata.Get(configEndpointSuffix)
}

func (cs *metadataMTLSAutoConfig) Config() (*mtlsConfig, error) {
	resp, err := httpGetMetadataMTLSConfig()
	if err != nil {
		log.Printf("querying MTLS config from MDS endpoint failed: %v", err)
		return defaultMTLSConfig(), nil
	}
	var config mtlsConfig
	err = json.Unmarshal([]byte(resp), &config)
	if err != nil {
		log.Printf("unmarshalling MTLS config from MDS endpoint failed: %v", err)
		return defaultMTLSConfig(), nil
	}

	if config.S2A == nil {
		log.Printf("returned MTLS config from MDS endpoint is invalid: %v", config)
		return defaultMTLSConfig(), nil
	}

	// set new expiry
	config.Expiry = time.Now().Add(configExpiry)
	return &config, nil
}

func defaultMTLSConfig() *mtlsConfig {
	return &mtlsConfig{
		S2A: &s2aAddresses{
			PlaintextAddress: "",
			MTLSAddress:      "",
		},
		Expiry: time.Now().Add(configExpiry),
	}
}

func shouldUseS2A(clientCertSource cert.Provider, opts *Options) bool {
	// If client cert is found, use that over S2A.
	if clientCertSource != nil {
		return false
	}
	// If EXPERIMENTAL_GOOGLE_API_USE_S2A is not set to true, skip S2A.
	if strings.ToLower(os.Getenv(googleAPIUseS2AEnv)) != "true" {
		return false
	}
	// If DefaultMTLSEndpoint is not set and no endpoint override, skip S2A.
	if opts.DefaultMTLSEndpoint == "" && opts.Endpoint == "" {
		return false
	}
	// If MTLS is not enabled for this endpoint, skip S2A.
	if !mtlsEndpointEnabledForS2A() {
		return false
	}
	// If custom HTTP client is provided, skip S2A.
	if opts.Client != nil {
		return false
	}
	return true
}
