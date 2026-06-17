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
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/retry"
	"cloud.google.com/go/auth/internal/transport/cert"
	"cloud.google.com/go/compute/metadata"
)

func init() {
	// Override retry options for testing to avoid long waits.
	retryOptions = &retry.Options{
		Initial:     1 * time.Millisecond,
		Max:         5 * time.Millisecond,
		Multiplier:  2.0,
		MaxAttempts: 6,
	}
}

func TestFetchRegionalAccessBoundaryData(t *testing.T) {
	type serverResponse struct {
		status int
		body   string
	}

	tests := []struct {
		name             string
		serverResponse   *serverResponse
		secondResponse   *serverResponse // For retry test
		token            *auth.Token
		urlOverride      *string // To test empty URL
		useNilClient     bool
		ctx              context.Context
		wantData         *internal.RegionalAccessBoundaryData
		wantErr          string
		wantReqHeaders   map[string]string
		wantRequestCount int
	}{
		{
			name: "Success - OK with locations",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-central1"], "encodedLocations": "0xABC"}`,
			},
			token:    &auth.Token{Value: "test-token"},
			ctx:      context.Background(),
			wantData: internal.NewRegionalAccessBoundaryData([]string{"us-central1"}, "0xABC"),
			wantReqHeaders: map[string]string{
				"Authorization": "Bearer test-token",
			},
			wantRequestCount: 1,
		},
		{
			name: "Success after one retry on 503 error",
			serverResponse: &serverResponse{
				status: http.StatusServiceUnavailable,
				body:   "server unavailable",
			},
			secondResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-central1"], "encodedLocations": "0xABC"}`,
			},
			token:            &auth.Token{Value: "test-token"},
			ctx:              context.Background(),
			wantData:         internal.NewRegionalAccessBoundaryData([]string{"us-central1"}, "0xABC"),
			wantRequestCount: 2,
		},
		{
			name: "Error - Non-200 Status",
			serverResponse: &serverResponse{
				status: http.StatusInternalServerError,
				body:   "server error",
			},
			token:   &auth.Token{Value: "test-token"},
			ctx:     context.Background(),
			wantErr: "Regional Access Boundary request failed with status: 500 Internal Server Error, body: server error",
		},
		{
			name: "Error - Malformed JSON",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"encodedLocations": "0x123", malformed`,
			},
			token:   &auth.Token{Value: "test-token"},
			ctx:     context.Background(),
			wantErr: "failed to unmarshal Regional Access Boundary response",
		},
		{
			name: "Error - Missing encodedLocations",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-east1"]}`,
			},
			token:   &auth.Token{Value: "test-token"},
			ctx:     context.Background(),
			wantErr: "invalid API response: encodedLocations is empty",
		},
		{
			name: "Error - Empty encodedLocations string",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": [], "encodedLocations": ""}`,
			},
			token:   &auth.Token{Value: "test-token"},
			ctx:     context.Background(),
			wantErr: "invalid API response: encodedLocations is empty",
		},
		{
			name:         "Error - Nil HTTP client",
			useNilClient: true,
			token:        &auth.Token{Value: "test-token"},
			ctx:          context.Background(),
			wantErr:      "HTTP client is required",
		},
		{
			name:        "Error - Empty URL",
			urlOverride: new(string),
			token:       &auth.Token{Value: "test-token"},
			ctx:         context.Background(),
			wantErr:     "URL cannot be empty",
		},
		{
			name: "Error - Empty Access Token",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"encodedLocations": "0x0"}`,
			},
			token:   &auth.Token{Value: ""},
			ctx:     context.Background(),
			wantErr: "access token required for lookup API authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			var url string
			var requestCount atomic.Int32

			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestCount.Add(1)
					// Use second response if it's a retry
					if tt.secondResponse != nil && requestCount.Load() > 1 {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tt.secondResponse.status)
						fmt.Fprint(w, tt.secondResponse.body)
						return
					}
					// Default response
					if tt.wantReqHeaders != nil {
						for key, val := range tt.wantReqHeaders {
							if got := r.Header.Get(key); got != val {
								t.Errorf("Header %s = %q, want %q", key, got, val)
							}
						}
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.serverResponse.status)
					fmt.Fprint(w, tt.serverResponse.body)
				}))
				defer server.Close()
				url = server.URL
			}

			if tt.urlOverride != nil {
				url = *tt.urlOverride
			}

			var client *http.Client
			if tt.useNilClient {
				client = nil
			} else {
				client = http.DefaultClient
			}

			data, err := fetchRegionalAccessBoundaryData(tt.ctx, client, url, tt.token, nil)

			if tt.wantRequestCount > 0 && requestCount.Load() != int32(tt.wantRequestCount) {
				t.Errorf("fetchRegionalAccessBoundaryData() requestCount = %d, want %d", requestCount.Load(), tt.wantRequestCount)
			}

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("fetchRegionalAccessBoundaryData() error = nil, want substring %q", tt.wantErr)
				}
				// Strip the common prefix before checking the specific error message part.
				gotError := strings.TrimPrefix(err.Error(), "regionalaccessboundary: ")
				if !strings.HasPrefix(gotError, tt.wantErr) {
					t.Errorf("fetchRegionalAccessBoundaryData() error = %q, want error: %q", gotError, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("fetchRegionalAccessBoundaryData() unexpected error: %v", err)
				}
				if !reflect.DeepEqual(data, tt.wantData) {
					t.Errorf("fetchRegionalAccessBoundaryData() data = %+v, want %+v", data, tt.wantData)
				}
			}
		})
	}
}

func TestServiceAccountConfig(t *testing.T) {
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	saEmail := "test-sa@example.iam.gserviceaccount.com"
	ud := "example.com"

	cfg := NewServiceAccountConfigProvider(saEmail, ud).(*serviceAccountConfig)

	if cfg.ServiceAccountEmail != saEmail {
		t.Errorf("NewServiceAccountConfigProvider().ServiceAccountEmail = %q, want %q", cfg.ServiceAccountEmail, saEmail)
	}
	if cfg.UniverseDomain != ud {
		t.Errorf("NewServiceAccountConfigProvider().UniverseDomain = %q, want %q", cfg.UniverseDomain, ud)
	}

	t.Run("GetRegionalAccessBoundaryEndpoint", func(t *testing.T) {
		tests := []struct {
			name    string
			saEmail string
			ud      string
			wantURL string
			wantErr string
		}{
			{
				name:    "Valid SA Email",
				ud:      "example.com",
				saEmail: "test-sa@example.iam.gserviceaccount.com",
				wantURL: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, "test-sa@example.iam.gserviceaccount.com"),
			},
			{
				name:    "Empty SA Email",
				saEmail: "",
				wantErr: "regionalaccessboundary: service account email cannot be empty for config",
			},
			{
				name:    "Empty UD defaults to googleapis.com",
				ud:      "",
				saEmail: "test-sa@example.iam.gserviceaccount.com",
				wantURL: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, "test-sa@example.iam.gserviceaccount.com"),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := NewServiceAccountConfigProvider(tt.saEmail, tt.ud)
				url, err := cfg.GetRegionalAccessBoundaryEndpoint(context.Background())
				if (err != nil && err.Error() != tt.wantErr) || (err == nil && tt.wantErr != "") {
					t.Errorf("GetRegionalAccessBoundaryEndpoint() error = %v, wantErr %q", err, tt.wantErr)
					return
				}
				if url != tt.wantURL {
					t.Errorf("GetRegionalAccessBoundaryEndpoint() url = %q, wantURL %q", url, tt.wantURL)
				}
			})
		}
	})

	t.Run("GetUniverseDomain", func(t *testing.T) {
		tests := []struct {
			name    string
			inputUD string
			wantUD  string
		}{
			{
				name:    "Valid UD",
				inputUD: "example.com",
				wantUD:  "example.com",
			},
			{
				name:    "Empty UD defaults to googleapis.com",
				inputUD: "",
				wantUD:  internal.DefaultUniverseDomain,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := NewServiceAccountConfigProvider("test-sa@example.com", tt.inputUD)
				gotUD, err := cfg.GetUniverseDomain(context.Background())
				if err != nil {
					t.Fatalf("GetUniverseDomain() unexpected error: %v", err)
				}
				if gotUD != tt.wantUD {
					t.Errorf("GetUniverseDomain() = %q, want %q", gotUD, tt.wantUD)
				}
			})
		}
	})
}

func TestGCEConfigProvider(t *testing.T) {
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	defaultTestEmail := "test-sa@example.iam.gserviceaccount.com"
	defaultTestUD := "example.com"
	defaultExpectedEndpoint := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, defaultTestEmail)

	tests := []struct {
		name                    string
		setupServer             func(t *testing.T) http.HandlerFunc
		gceUDP                  *internal.ComputeUniverseDomainProvider
		expectedEndpoint        string
		expectedUD              string
		wantErrEndpoint         string
		wantErrUD               string
		skipServerConfiguration bool
	}{
		{
			name: "Success",
			setupServer: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/computeMetadata/v1/instance/service-accounts/default/email":
						w.Write([]byte(defaultTestEmail))
					case "/computeMetadata/v1/universe/universe-domain":
						w.Write([]byte(defaultTestUD))
					default:
						http.NotFound(w, r)
					}
				}
			},
			expectedEndpoint: defaultExpectedEndpoint,
			expectedUD:       defaultTestUD,
		},
		{
			name: "Error fetching email",
			setupServer: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/computeMetadata/v1/instance/service-accounts/default/email" {
						http.Error(w, "email error", http.StatusInternalServerError)
						return
					}
					if r.URL.Path == "/computeMetadata/v1/universe/universe-domain" {
						w.Write([]byte(defaultTestUD))
						return
					}
					http.NotFound(w, r)
				}
			},
			wantErrEndpoint: "regionalaccessboundary: GCE config: failed to get service account email",
			expectedUD:      defaultTestUD,
		},
		{
			name: "Error fetching universe domain",
			setupServer: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/computeMetadata/v1/instance/service-accounts/default/email" {
						w.Write([]byte(defaultTestEmail))
						return
					}
					if r.URL.Path == "/computeMetadata/v1/universe/universe-domain" {
						http.Error(w, "ud error", http.StatusInternalServerError)
						return
					}
					http.NotFound(w, r)
				}
			},
			expectedEndpoint: defaultExpectedEndpoint,
			wantErrUD:        "regionalaccessboundary: GCE config: failed to get universe domain",
		},
		{
			name:                    "Nil ComputeUniverseDomainProvider",
			gceUDP:                  nil,
			wantErrEndpoint:         "regionalaccessboundary: GCEConfigProvider not properly initialized",
			wantErrUD:               "regionalaccessboundary: GCEConfigProvider not properly initialized",
			skipServerConfiguration: true,
		},
		{
			name: "ComputeUniverseDomainProvider with nil MetadataClient",
			gceUDP: &internal.ComputeUniverseDomainProvider{
				MetadataClient: nil,
			},
			wantErrEndpoint:         "regionalaccessboundary: GCEConfigProvider not properly initialized",
			wantErrUD:               "regionalaccessboundary: GCEConfigProvider not properly initialized",
			skipServerConfiguration: true,
		},
		{
			name: "Metadata server returns 404 for email",
			setupServer: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/computeMetadata/v1/instance/service-accounts/default/email" {
						http.NotFound(w, r)
						return
					}
					if r.URL.Path == "/computeMetadata/v1/universe/universe-domain" {
						w.Write([]byte(defaultTestUD))
						return
					}
					http.NotFound(w, r)
				}
			},
			wantErrEndpoint: "regionalaccessboundary: skip lookup",
			expectedUD:      defaultTestUD,
		},
		{
			name: "Metadata server returns empty universe domain (defaults to googleapis.com)",
			setupServer: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/computeMetadata/v1/instance/service-accounts/default/email":
						w.Write([]byte(defaultTestEmail))
					case "/computeMetadata/v1/universe/universe-domain":
						w.Write([]byte("")) // Empty UD
					default:
						http.NotFound(w, r)
					}
				}
			},
			expectedEndpoint: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, defaultTestEmail),
			expectedUD:       internal.DefaultUniverseDomain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var provider ConfigProvider

			if !tt.skipServerConfiguration {
				server := httptest.NewServer(tt.setupServer(t))
				defer server.Close()

				parsedURL, err := url.Parse(server.URL)
				if err != nil {
					t.Fatalf("Failed to parse server URL: %v", err)
				}
				t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
				mdClient := metadata.NewClient(server.Client())
				udp := &internal.ComputeUniverseDomainProvider{
					MetadataClient: mdClient,
				}
				provider = NewGCEConfigProvider(udp, nil)
			} else {
				t.Setenv("GCE_METADATA_HOST", "")
				provider = NewGCEConfigProvider(tt.gceUDP, nil)
			}

			endpoint, err := provider.GetRegionalAccessBoundaryEndpoint(ctx)
			if tt.wantErrEndpoint != "" {
				if err == nil {
					t.Errorf("GetRegionalAccessBoundaryEndpoint() error = nil, want  %q", tt.wantErrEndpoint)
				} else if !strings.Contains(err.Error(), tt.wantErrEndpoint) {
					t.Errorf("GetRegionalAccessBoundaryEndpoint() error = %q, want  %q", err.Error(), tt.wantErrEndpoint)
				}
			} else if err != nil {
				t.Errorf("GetRegionalAccessBoundaryEndpoint() unexpected error: %v", err)
			} else if endpoint != tt.expectedEndpoint {
				t.Errorf("GetRegionalAccessBoundaryEndpoint() = %q, want %q", endpoint, tt.expectedEndpoint)
			}

			ud, err := provider.GetUniverseDomain(ctx)
			if tt.wantErrUD != "" {
				if err == nil {
					t.Errorf("GetUniverseDomain() error = nil, wantErr substring %q", tt.wantErrUD)
				} else if !strings.Contains(err.Error(), tt.wantErrUD) {
					t.Errorf("GetUniverseDomain() error = %q, wantErr substring %q", err.Error(), tt.wantErrUD)
				}
			} else if err != nil {
				t.Errorf("GetUniverseDomain() unexpected error: %v", err)
			} else if ud != tt.expectedUD {
				t.Errorf("GetUniverseDomain() = %q, want %q", ud, tt.expectedUD)
			}
		})
	}
}

func TestGCEConfigProvider_CachesResults(t *testing.T) {

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			w.Write([]byte("test-sa@example.com"))
		case "/computeMetadata/v1/universe/universe-domain":
			w.Write([]byte("example.com"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}
	provider := NewGCEConfigProvider(udp, nil)

	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("call-%d", i+1), func(t *testing.T) {
			provider.GetRegionalAccessBoundaryEndpoint(context.Background())
			provider.GetUniverseDomain(context.Background())
			// The actual number of requests to the metadata server is 2 (one for email, one for UD)
			if m := requestCount.Load(); m > 2 {
				t.Errorf("expected metadata server to be called at most 2 times, but was called %d times", m)
			}
		})
	}
}

func TestGCEConfigProvider_TransientFailure(t *testing.T) {
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	var failMDS atomic.Bool
	failMDS.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failMDS.Load() {
			http.Error(w, "metadata server down", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			w.Write([]byte("test-sa@example.com"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}
	provider := NewGCEConfigProvider(udp, nil)

	ctx := context.Background()

	// First call should fail (MDS down)
	_, err := provider.GetRegionalAccessBoundaryEndpoint(ctx)
	if err == nil {
		t.Fatal("GetRegionalAccessBoundaryEndpoint() expected error on first call, got nil")
	}

	// Enable success for the next call
	failMDS.Store(false)

	// Second call should succeed (MDS up)
	endpoint, err := provider.GetRegionalAccessBoundaryEndpoint(ctx)
	if err != nil {
		t.Fatalf("GetRegionalAccessBoundaryEndpoint() unexpected error on second call: %v", err)
	}

	want := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, "test-sa@example.com")
	if endpoint != want {
		t.Errorf("GetRegionalAccessBoundaryEndpoint() = %q, want %q", endpoint, want)
	}
}

type mockConfigProvider struct {
	mu                  sync.Mutex
	endpointCallCount   int
	universeCallCount   int
	endpointToReturn    string
	endpointErrToReturn error
	universeToReturn    string
	universeErrToReturn error
	endpointCallCh      chan struct{}
}

func (m *mockConfigProvider) GetRegionalAccessBoundaryEndpoint(ctx context.Context) (string, error) {
	m.mu.Lock()
	m.endpointCallCount++
	m.mu.Unlock()
	if m.endpointCallCh != nil {
		select {
		case m.endpointCallCh <- struct{}{}:
		default:
		}
	}
	return m.endpointToReturn, m.endpointErrToReturn
}

func (m *mockConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	m.mu.Lock()
	m.universeCallCount++
	m.mu.Unlock()
	return m.universeToReturn, m.universeErrToReturn
}

func (m *mockConfigProvider) Reset() {
	m.mu.Lock()
	m.endpointCallCount = 0
	m.universeCallCount = 0
	m.mu.Unlock()
}

func (m *mockConfigProvider) GetEndpointCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endpointCallCount
}

type mockTokenProvider struct {
	TokenToReturn *auth.Token
	ErrorToReturn error
}

func (m *mockTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	return m.TokenToReturn, m.ErrorToReturn
}

func TestDataProvider_Token(t *testing.T) {
	ctx := context.Background()
	baseToken := &auth.Token{
		Value: "base-token",
		Metadata: map[string]interface{}{
			"pre-existing-key": "pre-existing-value",
		},
	}
	baseProvider := &mockTokenProvider{TokenToReturn: baseToken}

	provider, err := NewProvider(http.DefaultClient, &mockConfigProvider{}, nil, baseProvider)
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	token, err := provider.Token(ctx)
	if err != nil {
		t.Fatalf("provider.Token() unexpected error: %v", err)
	}

	if token.Value != baseToken.Value {
		t.Errorf("provider.Token() value = %q, want %q", token.Value, baseToken.Value)
	}

	if p, ok := token.Metadata[ProviderKey]; !ok || p != provider {
		t.Errorf("provider.Token() metadata missing ProviderKey or incorrect provider reference")
	}

	// Assert that the original token metadata was not mutated.
	if _, ok := baseToken.Metadata[ProviderKey]; ok {
		t.Errorf("provider.Token() mutated the base token's metadata")
	}

	// Assert that existing metadata entries are preserved.
	if v, ok := token.Metadata["pre-existing-key"]; !ok || v != "pre-existing-value" {
		t.Errorf("provider.Token() did not preserve existing metadata entries")
	}

	// Assert that the returned metadata map is not the same map instance as baseToken.Metadata.
	token.Metadata["test_mutation"] = true
	if _, ok := baseToken.Metadata["test_mutation"]; ok {
		t.Errorf("provider.Token() returned a metadata map that shares state with the base token")
	}
}

func TestDataProvider_GetHeaderValue(t *testing.T) {
	ctx := context.Background()

	t.Run("Regional endpoint skip", func(t *testing.T) {
		tests := []struct {
			name     string
			reqURL   string
			wantCall bool
		}{
			{"Standard Regional URL", "https://us-central1.rep.googleapis.com/v1/...", false},
			{"Regional Host Only", "us-central1.rep.googleapis.com", false},
			{"Regional with Port", "us-central1.rep.googleapis.com:443", false},
			{"Injected Query Param", "https://pubsub.googleapis.com/v1/...?tracking=rep.googleapis.com", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				token := &auth.Token{Value: "base"}
				mockConfig := &mockConfigProvider{
					universeToReturn: internal.DefaultUniverseDomain,
					endpointCallCh:   make(chan struct{}, 1),
				}
				provider, _ := NewProvider(http.DefaultClient, mockConfig, nil, &mockTokenProvider{TokenToReturn: token})

				_ = provider.GetHeaderValue(ctx, tt.reqURL, token)

				if tt.wantCall {
					select {
					case <-mockConfig.endpointCallCh:
						// Success
					case <-time.After(1 * time.Second):
						t.Errorf("GetHeaderValue(%q) did not initiate fetch (want fetch)", tt.reqURL)
					}
				} else {
					select {
					case <-mockConfig.endpointCallCh:
						t.Errorf("GetHeaderValue(%q) initiated fetch unexpectedly", tt.reqURL)
					case <-time.After(50 * time.Millisecond):
						// Success
					}
				}
			})
		}
	})

	t.Run("Resolver scheme skip", func(t *testing.T) {
		tests := []struct {
			name     string
			reqURL   string
			wantCall bool
		}{
			{"DNS Resolver Scheme IAM", "dns:///iam.googleapis.com", false},
			{"DNS Resolver Scheme STS", "dns:///sts.googleapis.com", false},
			{"DNS Resolver Scheme Regional", "dns:///us-central1.rep.googleapis.com", false},
			{"DNS Resolver Scheme Global", "dns:///pubsub.googleapis.com", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				token := &auth.Token{Value: "base"}
				mockConfig := &mockConfigProvider{
					universeToReturn: internal.DefaultUniverseDomain,
					endpointCallCh:   make(chan struct{}, 1),
				}
				provider, _ := NewProvider(http.DefaultClient, mockConfig, nil, &mockTokenProvider{TokenToReturn: token})

				_ = provider.GetHeaderValue(ctx, tt.reqURL, token)

				if tt.wantCall {
					select {
					case <-mockConfig.endpointCallCh:
						// Success
					case <-time.After(1 * time.Second):
						t.Errorf("GetHeaderValue(%q) did not initiate fetch (want fetch)", tt.reqURL)
					}
				} else {
					select {
					case <-mockConfig.endpointCallCh:
						t.Errorf("GetHeaderValue(%q) initiated fetch unexpectedly", tt.reqURL)
					case <-time.After(50 * time.Millisecond):
						// Success
					}
				}
			})
		}
	})

	t.Run("Async fetch success", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer wg.Done()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"locations": ["us-east1"], "encodedLocations": "0xABC"}`)
		}))
		defer server.Close()

		mockConfig := &mockConfigProvider{
			universeToReturn: internal.DefaultUniverseDomain,
			endpointToReturn: server.URL,
		}

		token := &auth.Token{Value: "base"}
		provider, _ := NewProvider(server.Client(), mockConfig, nil, &mockTokenProvider{TokenToReturn: token})

		val := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
		if val != "" {
			t.Errorf("First call should return empty while fetching async")
		}

		wg.Wait() // Wait for server to receive request

		// Wait for background fetch to complete and populate cache.
		var val2 string
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			val2 = provider.GetHeaderValue(ctx, "https://example.com/v1", token)
			if val2 != "" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if val2 == "" {
			t.Errorf("Second call did not return cached header within timeout")
		} else if val2 != "0xABC" {
			t.Errorf("Second call returned %q, expected %q", val2, "0xABC")
		}
	})

	t.Run("Endpoint_fetch_failure_sets_cooldown", func(t *testing.T) {
		configProvider := &mockConfigProvider{
			endpointErrToReturn: errors.New("endpoint error"),
		}
		provider, err := NewProvider(http.DefaultClient, configProvider, nil, nil)
		if err != nil {
			t.Fatalf("NewProvider() unexpected error: %v", err)
		}

		// First call should trigger async fetch (which fails).
		val := provider.GetHeaderValue(context.Background(), "https://example.com", nil)
		if val != "" {
			t.Errorf("First call expected empty string, got %q", val)
		}

		// Wait for the async fetch to complete and set cooldown.
		// Since there is no direct channel to signal completion, we poll the cooldown status.
		deadline := time.Now().Add(1 * time.Second)
		cooldownSet := false
		for time.Now().Before(deadline) {
			provider.mu.Lock()
			if !provider.cooldownExpiry.IsZero() {
				cooldownSet = true
				provider.mu.Unlock()
				break
			}
			provider.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}

		if !cooldownSet {
			t.Errorf("Cooldown expiry was not set after endpoint failure")
		}
	})
}

func TestGCEConfigProvider_NonGSAIdentity_BypassesLookup(t *testing.T) {
	ctx := context.Background()
	nonGSAEmail := "project.svc.id.goog"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			w.Write([]byte(nonGSAEmail))
		case "/computeMetadata/v1/universe/universe-domain":
			w.Write([]byte("googleapis.com"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	gceConfig := NewGCEConfigProvider(udp, logger)

	// Fetch endpoint first time - should log and return error
	endpoint, err := gceConfig.GetRegionalAccessBoundaryEndpoint(ctx)
	if !errors.Is(err, ErrSkipRegionalAccessBoundary) {
		t.Fatalf("expected ErrSkipRegionalAccessBoundary, got %v", err)
	}
	if endpoint != "" {
		t.Errorf("expected empty endpoint, got %q", endpoint)
	}

	// Verify that the INFO log message was logged
	logStr := logBuf.String()
	if !strings.Contains(logStr, "RAB lookup is skipped for this instance") {
		t.Errorf("expected log buffer to contain 'RAB lookup is skipped for this instance', got:\n%s", logStr)
	}

	// Reset log buffer to check one-time logging
	logBuf.Reset()

	// Fetch endpoint second time - should silently return SkipRegionalAccessBoundary without logging again
	endpoint2, err2 := gceConfig.GetRegionalAccessBoundaryEndpoint(ctx)
	if !errors.Is(err2, ErrSkipRegionalAccessBoundary) {
		t.Fatalf("second call: expected ErrSkipRegionalAccessBoundary, got %v", err2)
	}
	if endpoint2 != "" {
		t.Errorf("second call: expected empty endpoint, got %q", endpoint2)
	}
	if logBuf.Len() > 0 {
		t.Errorf("expected no log output on second call, got:\n%s", logBuf.String())
	}
}

func TestDataProvider_GetHeaderValue_BypassesNonGSA(t *testing.T) {
	ctx := context.Background()
	nonGSAEmail := "project.svc.id.goog[ns/sa]"

	var mdsRequestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mdsRequestCount.Add(1)
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			w.Write([]byte(nonGSAEmail))
		case "/computeMetadata/v1/universe/universe-domain":
			w.Write([]byte("googleapis.com"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}

	gceConfig := NewGCEConfigProvider(udp, nil)
	token := &auth.Token{Value: "base-token"}
	provider, err := NewProvider(server.Client(), gceConfig, nil, &mockTokenProvider{TokenToReturn: token})
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	// First call triggers background fetchAsync which fetches metadata email and detects invalid format.
	val := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
	if val != "" {
		t.Errorf("First call expected empty header, got %q", val)
	}

	// Wait for the async fetch to run, identify the invalid identity, and update provider.skipLookup to true.
	deadline := time.Now().Add(1 * time.Second)
	skipLookupSet := false
	for time.Now().Before(deadline) {
		provider.mu.RLock()
		if provider.skipLookup {
			skipLookupSet = true
			provider.mu.RUnlock()
			break
		}
		provider.mu.RUnlock()
		time.Sleep(10 * time.Millisecond)
	}

	if !skipLookupSet {
		t.Fatal("skipLookup was not set on DataProvider after detecting invalid identity")
	}

	// Reset request count to check that no more calls are made to MDS.
	mdsRequestCount.Store(0)

	// Second and subsequent calls should immediately return empty string, bypassing all fetches.
	for i := 0; i < 5; i++ {
		val2 := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
		if val2 != "" {
			t.Errorf("Call %d: expected empty header, got %q", i+2, val2)
		}
	}

	if m := mdsRequestCount.Load(); m > 0 {
		t.Errorf("expected zero MDS metadata calls during bypass, got %d", m)
	}
}

func TestDataProvider_GetHeaderValue_BypassesMissingSA(t *testing.T) {
	ctx := context.Background()

	var mdsRequestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mdsRequestCount.Add(1)
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			http.NotFound(w, r) // returns metadata.NotDefinedError
		case "/computeMetadata/v1/universe/universe-domain":
			w.Write([]byte("googleapis.com"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	t.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}

	gceConfig := NewGCEConfigProvider(udp, nil)
	token := &auth.Token{Value: "base-token"}
	provider, err := NewProvider(server.Client(), gceConfig, nil, &mockTokenProvider{TokenToReturn: token})
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	// First call triggers background fetchAsync which fetches metadata email, gets 404 (NotDefinedError),
	// and returns ErrSkipRegionalAccessBoundary.
	val := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
	if val != "" {
		t.Errorf("First call expected empty header, got %q", val)
	}

	// Wait for the async fetch to run and update provider.skipLookup to true.
	deadline := time.Now().Add(1 * time.Second)
	skipLookupSet := false
	for time.Now().Before(deadline) {
		provider.mu.RLock()
		if provider.skipLookup {
			skipLookupSet = true
			provider.mu.RUnlock()
			break
		}
		provider.mu.RUnlock()
		time.Sleep(10 * time.Millisecond)
	}

	if !skipLookupSet {
		t.Fatal("skipLookup was not set on DataProvider after detecting missing GCE SA")
	}

	// Reset request count to check that no more calls are made to MDS.
	mdsRequestCount.Store(0)

	// Second and subsequent calls should immediately return empty string, bypassing all fetches.
	for i := 0; i < 5; i++ {
		val2 := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
		if val2 != "" {
			t.Errorf("Call %d: expected empty header, got %q", i+2, val2)
		}
	}

	if m := mdsRequestCount.Load(); m > 0 {
		t.Errorf("expected zero MDS metadata calls during bypass, got %d", m)
	}
}

func TestResolveLocalMTLSEndpoint(t *testing.T) {
	base := "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/foo/allowedLocations"
	mtls := "https://iamcredentials.mtls.googleapis.com/v1/projects/-/serviceAccounts/foo/allowedLocations"

	defaultProvider, _ := cert.DefaultProvider()
	hasClientCert := defaultProvider != nil

	tests := []struct {
		name               string
		envUseMTLSEndpoint string
		envUseMTLS         string
		envUseClientCert   string
		want               string
	}{
		{
			name:               "always env var forces mtls",
			envUseMTLSEndpoint: "always",
			want:               mtls,
		},
		{
			name:               "never env var forces base",
			envUseMTLSEndpoint: "never",
			want:               base,
		},
		{
			name:       "always legacy env var forces mtls",
			envUseMTLS: "always",
			want:       mtls,
		},
		{
			name:             "client cert disabled env var forces base",
			envUseClientCert: "false",
			want:             base,
		},
		{
			name:             "client cert enabled env var checks client cert availability",
			envUseClientCert: "true",
			want: func() string {
				if hasClientCert {
					return mtls
				}
				return base
			}(),
		},
		{
			name:             "client cert invalid env var forces base",
			envUseClientCert: "invalid",
			want:             base,
		},
		{
			name:               "auto env var checks client cert availability",
			envUseMTLSEndpoint: "auto",
			want: func() string {
				if hasClientCert {
					return mtls
				}
				return base
			}(),
		},
		{
			name:               "MTLSEndpoint env var takes precedence over legacy MTLS env var",
			envUseMTLSEndpoint: "never",
			envUseMTLS:         "always",
			want:               base,
		},
		{
			name: "default when no env vars checks client cert availability",
			want: func() string {
				if hasClientCert {
					return mtls
				}
				return base
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envUseMTLSEndpoint != "" {
				t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", tt.envUseMTLSEndpoint)
			}
			if tt.envUseMTLS != "" {
				t.Setenv("GOOGLE_API_USE_MTLS", tt.envUseMTLS)
			}
			if tt.envUseClientCert != "" {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", tt.envUseClientCert)
			}

			got, err := resolveLocalMTLSEndpoint(base, mtls)
			if err != nil {
				t.Fatalf("resolveLocalMTLSEndpoint() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveLocalMTLSEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDataProvider_GetHeaderValue_BypassesNonDefaultUniverse(t *testing.T) {
	ctx := context.Background()
	mockConfig := &mockConfigProvider{
		universeToReturn: "custom-universe.com",
		endpointCallCh:   make(chan struct{}, 1),
	}
	token := &auth.Token{Value: "base-token"}
	provider, err := NewProvider(http.DefaultClient, mockConfig, nil, &mockTokenProvider{TokenToReturn: token})
	if err != nil {
		t.Fatalf("NewProvider() failed: %v", err)
	}

	val := provider.GetHeaderValue(ctx, "https://example.com/v1", token)
	if val != "" {
		t.Errorf("expected empty header for non-default universe, got %q", val)
	}

	// Verify that the endpoint builder was never called to trigger background fetch
	select {
	case <-mockConfig.endpointCallCh:
		t.Error("GetHeaderValue triggered Allowed Locations fetch on non-default universe domain, want skip")
	case <-time.After(100 * time.Millisecond):
		// Success: no fetch was initiated
	}
}

func TestMaybeCloneClientWithMTLS(t *testing.T) {
	fakeProvider := func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return nil, nil
	}

	t.Run("nil transport", func(t *testing.T) {
		client := &http.Client{}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		if cloned == nil {
			t.Fatal("expected non-nil client")
		}
		if cloned == client {
			t.Error("expected a new client pointer, got the same")
		}
		trans, ok := cloned.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("expected cloned transport to be *http.Transport, got %T", cloned.Transport)
		}
		if trans.TLSClientConfig == nil {
			t.Fatal("expected non-nil TLSClientConfig")
		}
		if reflect.ValueOf(trans.TLSClientConfig.GetClientCertificate).Pointer() != reflect.ValueOf(fakeProvider).Pointer() {
			t.Error("provider callback did not match")
		}
	})

	t.Run("existing http.Transport", func(t *testing.T) {
		originalTrans := &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{
				ServerName: "original",
			},
		}
		client := &http.Client{Transport: originalTrans}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		if cloned == nil {
			t.Fatal("expected non-nil client")
		}
		if cloned == client {
			t.Error("expected a new client pointer, got the same")
		}
		trans, ok := cloned.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("expected cloned transport to be *http.Transport, got %T", cloned.Transport)
		}
		if trans == originalTrans {
			t.Error("expected a cloned transport pointer, got the same")
		}
		if trans.TLSClientConfig.ServerName != "original" {
			t.Errorf("expected ServerName to be preserved as 'original', got %q", trans.TLSClientConfig.ServerName)
		}
		if reflect.ValueOf(trans.TLSClientConfig.GetClientCertificate).Pointer() != reflect.ValueOf(fakeProvider).Pointer() {
			t.Error("provider callback did not match")
		}
	})

	t.Run("pre-configured client certs are preserved", func(t *testing.T) {
		existingProvider := func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &tls.Certificate{}, nil
		}
		originalTrans := &http.Transport{
			TLSClientConfig: &tls.Config{
				GetClientCertificate: existingProvider,
			},
		}
		client := &http.Client{Transport: originalTrans}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		trans := cloned.Transport.(*http.Transport)
		if reflect.ValueOf(trans.TLSClientConfig.GetClientCertificate).Pointer() != reflect.ValueOf(existingProvider).Pointer() {
			t.Error("expected existing client cert provider to be preserved, but it was overwritten")
		}
	})

	t.Run("pre-configured Certificates are preserved", func(t *testing.T) {
		originalTrans := &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{{}},
			},
		}
		client := &http.Client{Transport: originalTrans}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		trans := cloned.Transport.(*http.Transport)
		if trans.TLSClientConfig.GetClientCertificate != nil {
			t.Error("expected GetClientCertificate to remain nil when Certificates are pre-configured")
		}
	})

	t.Run("custom RoundTripper is returned as-is", func(t *testing.T) {
		customTrans := &fakeRoundTripper{}
		client := &http.Client{Transport: customTrans}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		if cloned != client {
			t.Error("expected client to be returned unmodified for custom RoundTripper")
		}
	})

	t.Run("nil transport with non-cloneable default transport", func(t *testing.T) {
		oldDefault := http.DefaultTransport
		defer func() { http.DefaultTransport = oldDefault }()
		http.DefaultTransport = &fakeRoundTripper{}

		client := &http.Client{}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		trans, ok := cloned.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("expected cloned transport to be *http.Transport, got %T", cloned.Transport)
		}
		if trans.TLSClientConfig == nil {
			t.Fatal("expected non-nil TLSClientConfig")
		}
		if reflect.ValueOf(trans.TLSClientConfig.GetClientCertificate).Pointer() != reflect.ValueOf(fakeProvider).Pointer() {
			t.Error("provider callback did not match")
		}
	})

	t.Run("nil client input", func(t *testing.T) {
		cloned := maybeCloneClientWithMTLS(nil, fakeProvider)
		if cloned != nil {
			t.Errorf("expected nil client, got %v", cloned)
		}
	})

	t.Run("clone returns nil", func(t *testing.T) {
		client := &http.Client{Transport: &fakeCloneableTransport{clone: nil}}
		cloned := maybeCloneClientWithMTLS(client, fakeProvider)
		if cloned != client {
			t.Error("expected client to be returned unmodified when clone is nil")
		}
	})
}

type fakeRoundTripper struct{}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, nil
}

type fakeCloneableTransport struct {
	clone *http.Transport
}

func (f *fakeCloneableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, nil
}

func (f *fakeCloneableTransport) Clone() *http.Transport {
	return f.clone
}
