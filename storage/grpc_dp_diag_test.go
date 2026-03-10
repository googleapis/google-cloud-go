package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
)

func TestDirectPathDiagnostic(t *testing.T) {
	// Start a mock metadata server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/computeMetadata/v1/instance/service-accounts/"+defaultServiceAccount+"/email" {
			w.Write([]byte("default-compute@developer.gserviceaccount.com"))
		} else if r.URL.Path == "/computeMetadata/v1/"+defaultServiceAccountToken {
			w.Write([]byte(`{"access_token": "mock-token", "expires_in": 3600, "token_type": "Bearer"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	os.Setenv("GCE_METADATA_HOST", ts.URL[7:]) // remove "http://"
	defer os.Unsetenv("GCE_METADATA_HOST")

	tests := []struct {
		name string
		opts []option.ClientOption
		env  map[string]string
		want string
	}{
		{
			name: "disabled via env var",
			opts: []option.ClientOption{internaloption.EnableDirectPath(true)},
			env:  map[string]string{directPathDisableEnvVar: "true"},
			want: reasonEnvVarDisabled,
		},
		{
			name: "option disabled",
			opts: []option.ClientOption{internaloption.EnableDirectPath(false)},
			want: reasonOptionDisabled,
		},
		{
			name: "unsupported endpoint",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("https://storage.googleapis.com"),
			},
			want: reasonUnsupportedEndpoint,
		},
		{
			name: "xds not enabled",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
			},
			want: reasonXDSNotEnabled,
		},
		{
			name: "custom grpc conn",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithGRPCConn(&grpc.ClientConn{}),
			},
			want: reasonCustomGRPCConn,
		},
		{
			name: "custom http client",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithHTTPClient(&http.Client{}),
			},
			want: reasonCustomHTTPClient,
		},
		{
			name: "no auth",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithoutAuthentication(),
			},
			want: reasonNoAuth,
		},
		{
			name: "with api key",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
				option.WithAPIKey("fake-api-key"),
			},
			want: reasonAPIKey,
		},
		{
			name: "success undetermined",
			opts: []option.ClientOption{
				internaloption.EnableDirectPath(true),
				option.WithEndpoint("dns:///storage.googleapis.com"),
				internaloption.EnableDirectPathXds(),
			},
			want: reasonUndetermined,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.env) > 0 {
				for k, v := range tc.env {
					t.Setenv(k, v)
				}
			}

			got := directPathDiagnostic(context.Background(), tc.opts...)
			if got != tc.want {
				t.Errorf("directPathDiagnostic() = %v; want %v", got, tc.want)
			}
		})
	}
}
