package trustboundary

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

	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/compute/metadata"
)

func TestNewNoOpTrustBoundaryData(t *testing.T) {
	data := NewNoOpTrustBoundaryData()

	if data == nil {
		t.Fatal("NewNoOpTrustBoundaryData() returned nil")
	}

	if got := data.EncodedLocations(); got != NoOpEncodedLocations {
		t.Errorf("NewNoOpTrustBoundaryData().EncodedLocations() = %q, want %q", got, NoOpEncodedLocations)
	}

	// Expect an empty slice, not nil.
	expectedLocations := []string{}
	if got := data.Locations(); !reflect.DeepEqual(got, expectedLocations) {
		t.Errorf("NewNoOpTrustBoundaryData().Locations() = %v, want %v", got, expectedLocations)
	}

	if !data.IsNoOpOrEmpty() {
		t.Errorf("NewNoOpTrustBoundaryData().IsNoOpOrEmpty() = false, want true")
	}
}

func TestNewTrustBoundaryData(t *testing.T) {
	tests := []struct {
		name               string
		locations          []string
		encodedLocations   string
		wantLocations      []string
		wantEncoded        string
		wantIsNoOpOrEmpty  bool
		modifyReturnedLocs bool // to test if the returned slice is a copy
	}{
		{
			name:               "Standard data",
			locations:          []string{"us-central1", "europe-west1"},
			encodedLocations:   "0xABC123",
			wantLocations:      []string{"us-central1", "europe-west1"},
			wantEncoded:        "0xABC123",
			wantIsNoOpOrEmpty:  false,
			modifyReturnedLocs: true,
		},
		{
			name:               "Empty locations, not no-op encoded",
			locations:          []string{},
			encodedLocations:   "0xDEF456",
			wantLocations:      []string{},
			wantEncoded:        "0xDEF456",
			wantIsNoOpOrEmpty:  false,
			modifyReturnedLocs: false,
		},
		{
			name:               "Nil locations, not no-op encoded",
			locations:          nil,
			encodedLocations:   "0xGHI789",
			wantLocations:      []string{}, // Expect empty slice, not nil
			wantEncoded:        "0xGHI789",
			wantIsNoOpOrEmpty:  false,
			modifyReturnedLocs: false,
		},
		{
			name:               "No-op encoded locations",
			locations:          []string{"us-east1"},
			encodedLocations:   NoOpEncodedLocations,
			wantLocations:      []string{"us-east1"},
			wantEncoded:        NoOpEncodedLocations,
			wantIsNoOpOrEmpty:  true,
			modifyReturnedLocs: true,
		},
		{
			name:               "Empty string encoded locations",
			locations:          []string{},
			encodedLocations:   "",
			wantLocations:      []string{},
			wantEncoded:        "",
			wantIsNoOpOrEmpty:  true,
			modifyReturnedLocs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := NewTrustBoundaryData(tt.locations, tt.encodedLocations)

			if got := data.EncodedLocations(); got != tt.wantEncoded {
				t.Errorf("NewTrustBoundaryData().EncodedLocations() = %q, want %q", got, tt.wantEncoded)
			}

			gotLocations := data.Locations()
			if !reflect.DeepEqual(gotLocations, tt.wantLocations) {
				t.Errorf("NewTrustBoundaryData().Locations() = %v, want %v", gotLocations, tt.wantLocations)
			}

			// Test that Locations() returns a copy
			if tt.modifyReturnedLocs && len(gotLocations) > 0 {
				gotLocations[0] = "modified-location"
				if reflect.DeepEqual(data.Locations(), gotLocations) {
					t.Errorf("Modifying returned Locations() slice affected internal slice. Original: %v, Internal after mod: %v", tt.wantLocations, data.Locations())
				}
			}

			if got := data.IsNoOpOrEmpty(); got != tt.wantIsNoOpOrEmpty {
				t.Errorf("NewTrustBoundaryData().IsNoOpOrEmpty() = %v, want %v", got, tt.wantIsNoOpOrEmpty)
			}
		})
	}
}

func TestData_Methods_NilReceiver(t *testing.T) {
	var data *Data = nil

	if got := data.Locations(); got != nil {
		t.Errorf("nil.Locations() = %v, want nil", got)
	}

	if got := data.EncodedLocations(); got != "" {
		t.Errorf("nil.EncodedLocations() = %q, want \"\"", got)
	}

	if !data.IsNoOpOrEmpty() {
		t.Errorf("nil.IsNoOpOrEmpty() = false, want true")
	}
}

func TestFetchTrustBoundaryData(t *testing.T) {
	type serverResponse struct {
		status int
		body   string
	}

	tests := []struct {
		name           string
		serverResponse *serverResponse
		accessToken    string
		urlOverride    *string // To test empty URL
		useNilClient   bool
		ctx            context.Context
		wantData       *Data
		wantErr        string
		wantReqHeaders map[string]string
	}{
		{
			name: "Success - OK with locations",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-central1"], "encodedLocations": "0xABC"}`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantData:    NewTrustBoundaryData([]string{"us-central1"}, "0xABC"),
			wantReqHeaders: map[string]string{
				"Authorization": "Bearer test-token",
			},
		},
		{
			name: "Success - OK No-Op response",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"encodedLocations": "0x0"}`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantData:    NewTrustBoundaryData(nil, "0x0"),
		},
		{
			name: "Success - OK No-Op response with empty locations array",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": [], "encodedLocations": "0x0"}`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantData:    NewTrustBoundaryData([]string{}, "0x0"),
		},
		{
			name: "Error - Non-200 Status",
			serverResponse: &serverResponse{
				status: http.StatusInternalServerError,
				body:   "server error",
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantErr:     "trust boundary request failed with status: 500 Internal Server Error, body: server error",
		},
		{
			name: "Error - Malformed JSON",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"encodedLocations": "0x123", malformed`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantErr:     "failed to unmarshal trust boundary response",
		},
		{
			name: "Error - Missing encodedLocations",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-east1"]}`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantErr:     "invalid API response: encodedLocations is empty",
		},
		{
			name: "Error - Empty encodedLocations string",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": [], "encodedLocations": ""}`,
			},
			accessToken: "test-token",
			ctx:         context.Background(),
			wantErr:     "invalid API response: encodedLocations is empty",
		},
		{
			name:         "Error - Nil HTTP client",
			useNilClient: true,
			accessToken:  "test-token",
			ctx:          context.Background(),
			wantErr:      "HTTP client is required",
		},
		{
			name:        "Error - Empty URL",
			urlOverride: new(string),
			accessToken: "test-token",
			ctx:         context.Background(),
			wantErr:     "URL cannot be empty",
		},
		{
			name: "Error - Empty Access Token",
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"encodedLocations": "0x0"}`,
			},
			accessToken: "",
			ctx:         context.Background(),
			wantErr:     "access token required for lookup API authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			var url string

			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.wantReqHeaders != nil {
						for key, val := range tt.wantReqHeaders {
							if got := r.Header.Get(key); got != val {
								t.Errorf("Header %s = %q, want %q", key, got, val)
							}
						}
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.serverResponse.status)
					fmt.Fprintln(w, tt.serverResponse.body)
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

			data, err := fetchTrustBoundaryData(tt.ctx, client, url, tt.accessToken)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("fetchTrustBoundaryData() error = nil, want substring %q", tt.wantErr)
				}
				// Strip the common prefix before checking the specific error message part.
				gotError := strings.TrimPrefix(err.Error(), "trustboundary: ")
				if !strings.HasPrefix(gotError, tt.wantErr) {
					t.Errorf("fetchTrustBoundaryData() error = %q, want error: %q", gotError, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("fetchTrustBoundaryData() unexpected error: %v", err)
				}
				if !reflect.DeepEqual(data, tt.wantData) {
					t.Errorf("fetchTrustBoundaryData() data = %+v, want %+v", data, tt.wantData)
				}
			}
		})
	}
}

func TestServiceAccountTrustBoundaryConfig(t *testing.T) {
	saEmail := "test-sa@example.iam.gserviceaccount.com"
	ud := "example.com"

	cfg := NewServiceAccountTrustBoundaryConfig(saEmail, ud)

	if cfg.ServiceAccountEmail != saEmail {
		t.Errorf("NewServiceAccountTrustBoundaryConfig().ServiceAccountEmail = %q, want %q", cfg.ServiceAccountEmail, saEmail)
	}
	if cfg.UniverseDomain != ud {
		t.Errorf("NewServiceAccountTrustBoundaryConfig().UniverseDomain = %q, want %q", cfg.UniverseDomain, ud)
	}

	t.Run("GetTrustBoundaryEndpoint", func(t *testing.T) {
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
				wantErr: "trustboundary: service account email cannot be empty for config",
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
				cfg := NewServiceAccountTrustBoundaryConfig(tt.saEmail, tt.ud)
				url, err := cfg.GetTrustBoundaryEndpoint(context.Background())
				if (err != nil && err.Error() != tt.wantErr) || (err == nil && tt.wantErr != "") {
					t.Errorf("GetTrustBoundaryEndpoint() error = %v, wantErr %q", err, tt.wantErr)
					return
				}
				if url != tt.wantURL {
					t.Errorf("GetTrustBoundaryEndpoint() url = %q, wantURL %q", url, tt.wantURL)
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
				cfg := NewServiceAccountTrustBoundaryConfig("test-sa@example.com", tt.inputUD)
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

func TestGCETrustBoundaryConfigProvider(t *testing.T) {
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
			wantErrEndpoint: "trustboundary: GCE config: failed to get service account email",
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
			wantErrEndpoint: "trustboundary: GCE config: failed to get universe domain",
			wantErrUD:       "trustboundary: GCE config: failed to get universe domain",
		},
		{
			name:                    "Nil ComputeUniverseDomainProvider",
			gceUDP:                  nil,
			wantErrEndpoint:         "trustboundary: GCETrustBoundaryConfigProvider not properly initialized",
			wantErrUD:               "trustboundary: GCETrustBoundaryConfigProvider not properly initialized",
			skipServerConfiguration: true,
		},
		{
			name: "ComputeUniverseDomainProvider with nil MetadataClient",
			gceUDP: &internal.ComputeUniverseDomainProvider{
				MetadataClient: nil,
			},
			wantErrEndpoint:         "trustboundary: GCETrustBoundaryConfigProvider not properly initialized",
			wantErrUD:               "trustboundary: GCETrustBoundaryConfigProvider not properly initialized",
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
			wantErrEndpoint: "trustboundary: GCE config: failed to get service account email: metadata: GCE metadata \"instance/service-accounts/default/email\" not defined",
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
			var provider TrustBoundaryConfigProvider

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
				provider = NewGCETrustBoundaryConfigProvider(udp)
			} else {
				os.Unsetenv("GCE_METADATA_HOST")
				provider = NewGCETrustBoundaryConfigProvider(tt.gceUDP)
			}

			endpoint, err := provider.GetTrustBoundaryEndpoint(ctx)
			if tt.wantErrEndpoint != "" {
				if err == nil {
					t.Errorf("GetTrustBoundaryEndpoint() error = nil, want  %q", tt.wantErrEndpoint)
				} else if !strings.Contains(err.Error(), tt.wantErrEndpoint) {
					t.Errorf("GetTrustBoundaryEndpoint() error = %q, want  %q", err.Error(), tt.wantErrEndpoint)
				}
			} else if err != nil {
				t.Errorf("GetTrustBoundaryEndpoint() unexpected error: %v", err)
			} else if endpoint != tt.expectedEndpoint {
				t.Errorf("GetTrustBoundaryEndpoint() = %q, want %q", endpoint, tt.expectedEndpoint)
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

type mockTrustBoundaryConfigProvider struct {
	endpointCallCount   int
	universeCallCount   int
	endpointToReturn    string
	endpointErrToReturn error
	universeToReturn    string
	universeErrToReturn error
}

func (m *mockTrustBoundaryConfigProvider) GetTrustBoundaryEndpoint(ctx context.Context) (string, error) {
	m.endpointCallCount++
	return m.endpointToReturn, m.endpointErrToReturn
}

func (m *mockTrustBoundaryConfigProvider) GetUniverseDomain(ctx context.Context) (string, error) {
	m.universeCallCount++
	return m.universeToReturn, m.universeErrToReturn
}

func (m *mockTrustBoundaryConfigProvider) Reset() {
	m.endpointCallCount = 0
	m.universeCallCount = 0
}

func TestNewTrustBoundaryDataProvider(t *testing.T) {
	mockConfigProvider := &mockTrustBoundaryConfigProvider{}
	tests := []struct {
		name           string
		client         *http.Client
		configProvider TrustBoundaryConfigProvider
		wantErr        string
	}{
		{
			name:           "Valid provider",
			client:         http.DefaultClient,
			configProvider: mockConfigProvider,
		},
		{
			name:           "Nil client",
			client:         nil,
			configProvider: mockConfigProvider,
			wantErr:        "trustboundary: HTTP client cannot be nil for TrustBoundaryDataProvider",
		},
		{
			name:           "Nil config provider",
			client:         http.DefaultClient,
			configProvider: nil,
			wantErr:        "trustboundary: TrustBoundaryConfigProvider cannot be nil for TrustBoundaryDataProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTrustBoundaryDataProvider(tt.client, tt.configProvider)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("NewTrustBoundaryDataProvider() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("NewTrustBoundaryDataProvider() error = %q, want %q", err.Error(), tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("NewTrustBoundaryDataProvider() unexpected error: %v", err)
			}
		})
	}
}

func TestTrustBoundaryDataProvider_GetTrustBoundaryData(t *testing.T) {
	ctx := context.Background()
	defaultAccessToken := "test-access-token"

	type serverResponse struct {
		status int
		body   string
	}

	tests := []struct {
		name                  string
		mockConfig            *mockTrustBoundaryConfigProvider
		initialCachedData     *Data
		serverResponse        *serverResponse // for fetchTrustBoundaryData
		wantData              *Data
		wantErr               string
		wantUniverseCallCount int
		wantEndpointCallCount int
	}{
		{
			name: "Non-default universe domain returns NoOp and caches it",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: "example.com",
			},
			wantData:              NewNoOpTrustBoundaryData(),
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 0,
		},
		{
			name: "Default universe, returns NoOp from cache",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			initialCachedData:     NewNoOpTrustBoundaryData(),
			wantData:              NewNoOpTrustBoundaryData(),
			wantUniverseCallCount: 1, // Universe is checked
			wantEndpointCallCount: 0, // Endpoint fetch is skipped due to cached NoOp
		},
		{
			name: "Default universe, no cache, successful fetch and caches result",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-east1"], "encodedLocations": "0xABC"}`,
			},
			wantData:              NewTrustBoundaryData([]string{"us-east1"}, "0xABC"),
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Default universe, fetch fails, no initial cache, returns error",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			serverResponse: &serverResponse{
				status: http.StatusInternalServerError,
				body:   "server error",
			},
			wantErr:               "failed to fetch trust boundary data",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Default universe, fetch fails, with existing cache, returns cache",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: internal.DefaultUniverseDomain,
			},
			initialCachedData: NewTrustBoundaryData([]string{"cached-loc"}, "0xCACHE"),
			serverResponse: &serverResponse{
				status: http.StatusInternalServerError,
				body:   "server error",
			},
			wantData:              NewTrustBoundaryData([]string{"cached-loc"}, "0xCACHE"), // Expect cached data
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Error from GetUniverseDomain",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeErrToReturn: errors.New("universe domain error"),
			},
			wantErr:               "error getting universe domain: universe domain error",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 0,
		},
		{
			name: "Error from GetTrustBoundaryEndpoint",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn:    internal.DefaultUniverseDomain,
				endpointErrToReturn: errors.New("endpoint error"),
			},
			wantErr:               "error getting the lookup endpoint: endpoint error",
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
		},
		{
			name: "Empty universe domain from provider defaults to DefaultUniverseDomain, successful fetch",
			mockConfig: &mockTrustBoundaryConfigProvider{
				universeToReturn: "", // Empty, should default
			},
			serverResponse: &serverResponse{
				status: http.StatusOK,
				body:   `{"locations": ["us-default"], "encodedLocations": "0xDEF"}`,
			},
			wantData:              NewTrustBoundaryData([]string{"us-default"}, "0xDEF"),
			wantUniverseCallCount: 1,
			wantEndpointCallCount: 1,
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

			provider, err := NewTrustBoundaryDataProvider(client, tt.mockConfig)
			if err != nil {
				t.Fatalf("NewTrustBoundaryDataProvider() failed: %v", err)
			}
			internalProvider := provider.(*TrustBoundaryDataProvider)
			if tt.initialCachedData != nil {
				internalProvider.data = tt.initialCachedData
			}

			gotData, err := provider.GetTrustBoundaryData(ctx, defaultAccessToken)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("GetTrustBoundaryData() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("GetTrustBoundaryData() error = %q, want %q", err.Error(), tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("GetTrustBoundaryData() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(gotData, tt.wantData) {
				t.Errorf("GetTrustBoundaryData() data = %+v, want %+v", gotData, tt.wantData)
			}

			if tt.mockConfig.universeCallCount != tt.wantUniverseCallCount {
				t.Errorf("GetUniverseDomain call count = %d, want %d", tt.mockConfig.universeCallCount, tt.wantUniverseCallCount)
			}
			if tt.mockConfig.endpointCallCount != tt.wantEndpointCallCount {
				t.Errorf("GetTrustBoundaryEndpoint call count = %d, want %d", tt.mockConfig.endpointCallCount, tt.wantEndpointCallCount)
			}

			if !reflect.DeepEqual(internalProvider.data, tt.wantData) {
				t.Errorf("Final cache state mismatch. Got %+v, want %+v", internalProvider.data, tt.wantData)
			}
		})
	}
}
