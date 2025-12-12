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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/compute/metadata"
)

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
			requestCount := 0

			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestCount++
					// Use second response if it's a retry
					if tt.secondResponse != nil && requestCount > 1 {
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

			if tt.wantRequestCount > 0 && requestCount != tt.wantRequestCount {
				t.Errorf("fetchRegionalAccessBoundaryData() requestCount = %d, want %d", requestCount, tt.wantRequestCount)
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

func TestIsRegionalAccessBoundaryEnabled(t *testing.T) {
	newEnvVar := "GOOGLE_AUTH_REGIONAL_ACCESS_BOUNDARY_ENABLE_EXPERIMENT"
	oldEnvVar := "GOOGLE_AUTH_TRUST_BOUNDARY_ENABLED"

	tests := []struct {
		name              string
		newEnvVal         string
		oldEnvVal         string
		setNewEnv         bool
		setOldEnv         bool
		want              bool
	}{
		{
			name:      "new env unset, old env unset",
			setNewEnv: false,
			setOldEnv: false,
			want:      false,
		},
		{
			name:      "new env empty, old env unset",
			newEnvVal: "",
			setNewEnv: true,
			setOldEnv: false,
			want:      false,
		},
		{
			name:      "new env true, old env unset",
			newEnvVal: "true",
			setNewEnv: true,
			setOldEnv: false,
			want:      true,
		},
		{
			name:      "new env 1, old env unset",
			newEnvVal: "1",
			setNewEnv: true,
			setOldEnv: false,
			want:      true,
		},
		{
			name:      "new env false, old env unset",
			newEnvVal: "false",
			setNewEnv: true,
			setOldEnv: false,
			want:      false,
		},
		{
			name:      "new env 0, old env unset",
			newEnvVal: "0",
			setNewEnv: true,
			setOldEnv: false,
			want:      false,
		},
		{
			name:      "new env invalid, old env unset",
			newEnvVal: "invalid",
			setNewEnv: true,
			setOldEnv: false,
			want:      false,
		},
		{
			name:      "new env unset, old env true",
			setNewEnv: false,
			oldEnvVal: "true",
			setOldEnv: true,
			want:      true,
		},
		{
			name:      "new env unset, old env invalid",
			setNewEnv: false,
			oldEnvVal: "invalid",
			setOldEnv: true,
			want:      false,
		},
		{
			name:      "new env true, old env false (new takes precedence)",
			newEnvVal: "true",
			setNewEnv: true,
			oldEnvVal: "false",
			setOldEnv: true,
			want:      true,
		},
		{
			name:      "new env false, old env true (new takes precedence)",
			newEnvVal: "false",
			setNewEnv: true,
			oldEnvVal: "true",
			setOldEnv: true,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setNewEnv {
				t.Setenv(newEnvVar, tt.newEnvVal)
			} else {
				os.Unsetenv(newEnvVar)
			}
			if tt.setOldEnv {
				t.Setenv(oldEnvVar, tt.oldEnvVal)
			} else {
				os.Unsetenv(oldEnvVar)
			}

			got, err := isRegionalAccessBoundaryEnabled()
			if err != nil {
				t.Fatalf("isRegionalAccessBoundaryEnabled() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("isRegionalAccessBoundaryEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceAccountConfig(t *testing.T) {
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
				wantURL: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, "example.com", "test-sa@example.iam.gserviceaccount.com"),
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
				wantURL: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, internal.DefaultUniverseDomain, "test-sa@example.iam.gserviceaccount.com"),
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
	defaultTestEmail := "test-sa@example.iam.gserviceaccount.com"
	defaultTestUD := "example.com"
	defaultExpectedEndpoint := fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, defaultTestUD, defaultTestEmail)

	originalGCEHost := os.Getenv("GCE_METADATA_HOST")
	defer os.Setenv("GCE_METADATA_HOST", originalGCEHost)

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
			wantErrEndpoint: "regionalaccessboundary: GCE config: failed to get universe domain",
			wantErrUD: "regionalaccessboundary: GCE config: failed to get universe domain",
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
			wantErrEndpoint: "regionalaccessboundary: GCE config: failed to get service account email: metadata: GCE metadata \"instance/service-accounts/default/email\" not defined",
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
			expectedEndpoint: fmt.Sprintf(serviceAccountAllowedLocationsEndpoint, internal.DefaultUniverseDomain, defaultTestEmail),
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
				os.Setenv("GCE_METADATA_HOST", parsedURL.Host)
				mdClient := metadata.NewClient(server.Client())
				udp := &internal.ComputeUniverseDomainProvider{
					MetadataClient: mdClient,
				}
				provider = NewGCEConfigProvider(udp)
			} else {
				os.Unsetenv("GCE_METADATA_HOST")
				provider = NewGCEConfigProvider(tt.gceUDP)
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
	originalGCEHost := os.Getenv("GCE_METADATA_HOST")
	defer os.Setenv("GCE_METADATA_HOST", originalGCEHost)

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
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
	os.Setenv("GCE_METADATA_HOST", parsedURL.Host)
	mdClient := metadata.NewClient(server.Client())
	udp := &internal.ComputeUniverseDomainProvider{MetadataClient: mdClient}
	provider := NewGCEConfigProvider(udp)

	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("call-%d", i+1), func(t *testing.T) {
			provider.GetRegionalAccessBoundaryEndpoint(context.Background())
			provider.GetUniverseDomain(context.Background())
			// The actual number of requests to the metadata server is 2 (one for email, one for UD)
			if requestCount > 2 {
				t.Errorf("expected metadata server to be called at most 2 times, but was called %d times", requestCount)
			}
		})
	}
}

type mockConfigProvider struct {
	endpointCallCount   int
	universeCallCount   int
	endpointToReturn    string
	endpointErrToReturn error
	universeToReturn    string
	universeErrToReturn error
}

func (m *mockConfigProvider) GetRegionalAccessBoundaryEndpoint(ctx context.Context) (string, error) {
	m.endpointCallCount++
	return m.endpointToReturn, m.endpointErrToReturn
}

func (m *mockConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	m.universeCallCount++
	return m.universeToReturn, m.universeErrToReturn
}

func (m *mockConfigProvider) Reset() {
	m.endpointCallCount = 0
	m.universeCallCount = 0
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

	type serverResponse struct {
		status int
		body   string
	}

	tests := []struct {
		name                  string
		mockConfig            *mockConfigProvider
		serverResponse        *serverResponse // for fetchRegionalAccessBoundaryData
		baseProvider          *mockTokenProvider
		wantDataOnToken       *internal.RegionalAccessBoundaryData
		wantErr               string
		wantUniverseCallCount int
		wantEndpointCallCount int
		// secondRun allows testing caching behavior by running the same hook again
		// with a different server/mock configuration.
		secondRun *struct {
			serverResponse        *serverResponse
			wantDataOnToken       *internal.RegionalAccessBoundaryData
			wantErr               string
			wantUniverseCallCount int
			wantEndpointCallCount int
		}
	}{
		{
			name: "Non-default universe domain returns nil",
			mockConfig: &mockConfigProvider{
				universeToReturn: "example.com",
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			wantDataOnToken:       nil,
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 0,
		},
		{
			name: "Default universe, no cache, successful fetch",
			mockConfig: &mockConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-east1"], "encodedLocations": "0xABC"}`,
			},
			wantDataOnToken:       internal.NewRegionalAccessBoundaryData([]string{"us-east1"}, "0xABC"),
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Default universe, fetch fails, no cache, returns error",
			mockConfig: &mockConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			serverResponse: &serverResponse{
				status: http.StatusInternalServerError,
				body:   "server error",
			},
			wantDataOnToken:       &internal.RegionalAccessBoundaryData{},
			wantErr:               "and no cache available",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Error from GetUniverseDomain",
			mockConfig: &mockConfigProvider{
				universeErrToReturn: errors.New("universe domain error"),
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			wantDataOnToken:       &internal.RegionalAccessBoundaryData{},
			wantErr:               "error getting universe domain",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 0,
		},
		{
			name: "Error from GetRegionalAccessBoundaryEndpoint",
			mockConfig: &mockConfigProvider{
				universeToReturn:    internal.DefaultUniverseDomain,
				endpointErrToReturn: errors.New("endpoint error"),
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			wantDataOnToken:       &internal.RegionalAccessBoundaryData{},
			wantErr:               "error getting the lookup endpoint",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Cache fallback on second call",
			mockConfig: &mockConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			baseProvider: &mockTokenProvider{
				TokenToReturn: &auth.Token{Value: "base-token"},
			},
			serverResponse: &serverResponse{ // First call is successful
				status: http.StatusOK,
				body:   `{"locations": ["us-east1"], "encodedLocations": "0xABC"}`,
			},
			wantDataOnToken:       internal.NewRegionalAccessBoundaryData([]string{"us-east1"}, "0xABC"),
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
			secondRun: &struct {
				serverResponse        *serverResponse
				wantDataOnToken       *internal.RegionalAccessBoundaryData
				wantErr               string
				wantUniverseCallCount int
				wantEndpointCallCount int
			}{
				serverResponse: &serverResponse{ // Second call fails
					status: http.StatusInternalServerError,
					body:   "server error",
				},
				wantDataOnToken: internal.NewRegionalAccessBoundaryData([]string{"us-east1"}, "0xABC"), // Should get cached data
				wantErr:         "",                                                           // No error due to fallback
				// It tries to fetch again, but falls back to cache.
				wantUniverseCallCount: 1,
				wantEndpointCallCount: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockConfig.Reset()
			var server *httptest.Server
			client := http.DefaultClient

			if tt.serverResponse != nil { // Indicates a fetch is expected
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.serverResponse.status)
					fmt.Fprintln(w, tt.serverResponse.body)
				}))
				defer server.Close()
				tt.mockConfig.endpointToReturn = server.URL
				client = server.Client() // Use the test server's client
			}

			provider, err := NewProvider(client, tt.mockConfig, nil, tt.baseProvider)
			if err != nil {
				t.Fatalf("NewProvider() failed: %v", err)
			}

			// First run
			token, err := provider.Token(ctx)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("provider.Token() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("provider.Token() error = %q, want %q", err.Error(), tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("provider.Token() unexpected error: %v", err)
			} else {
				if token.Value != tt.baseProvider.TokenToReturn.Value {
					t.Errorf("provider.Token() value = %q, want %q", token.Value, tt.baseProvider.TokenToReturn.Value)
				}
				var gotData internal.RegionalAccessBoundaryData
				data, ok := token.Metadata[internal.RegionalAccessBoundaryDataKey]
				if tt.wantDataOnToken == nil {
					if ok {
						t.Errorf("provider.Token() data on token = %+v, want nil", data)
					}
				} else {
					if ok {
						gotData, _ = data.(internal.RegionalAccessBoundaryData)
					}
					if !reflect.DeepEqual(gotData, *tt.wantDataOnToken) {
						t.Errorf("provider.Token() data on token = %+v, want %+v", gotData, *tt.wantDataOnToken)
					}
				}
			}

			if tt.mockConfig.universeCallCount != tt.wantUniverseCallCount {
				t.Errorf("GetUniverseDomain call count = %d, want %d", tt.mockConfig.universeCallCount, tt.wantUniverseCallCount)
			}
			if tt.mockConfig.endpointCallCount != tt.wantEndpointCallCount {
				t.Errorf("GetRegionalAccessBoundaryEndpoint call count = %d, want %d", tt.mockConfig.endpointCallCount, tt.wantEndpointCallCount)
			}

			// Second run, if configured
			if tt.secondRun != nil {
				// Reset server if needed
				if tt.secondRun.serverResponse != nil {
					server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(tt.secondRun.serverResponse.status)
						fmt.Fprintln(w, tt.secondRun.serverResponse.body)
					})
				}

				// Reset mock call counts for the second run
				tt.mockConfig.Reset()

				secondToken, err := provider.Token(ctx)

				if tt.secondRun.wantErr != "" {
					if err == nil {
						t.Fatalf("provider.Token() second run error = nil, want %q", tt.secondRun.wantErr)
					}
					if !strings.Contains(err.Error(), tt.secondRun.wantErr) {
						t.Errorf("provider.Token() second run error = %q, want %q", err.Error(), tt.secondRun.wantErr)
					}
				} else if err != nil {
					t.Fatalf("provider.Token() second run unexpected error: %v", err)
				} else {
					var gotData internal.RegionalAccessBoundaryData
					if data, ok := secondToken.Metadata[internal.RegionalAccessBoundaryDataKey]; ok {
						gotData, _ = data.(internal.RegionalAccessBoundaryData)
					}
					if !reflect.DeepEqual(gotData, *tt.secondRun.wantDataOnToken) {
						t.Errorf("provider.Token() second run data on token = %+v, want %+v", gotData, *tt.secondRun.wantDataOnToken)
					}
				}

				if tt.mockConfig.universeCallCount != tt.secondRun.wantUniverseCallCount {
					t.Errorf("second run GetUniverseDomain call count = %d, want %d", tt.mockConfig.universeCallCount, tt.secondRun.wantUniverseCallCount)
				}
				if tt.mockConfig.endpointCallCount != tt.secondRun.wantEndpointCallCount {
					t.Errorf("second run GetRegionalAccessBoundaryEndpoint call count = %d, want %d", tt.mockConfig.endpointCallCount, tt.secondRun.wantEndpointCallCount)
				}
			}
		})
	}
}
