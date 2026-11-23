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

package storage

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"cloud.google.com/go/auth"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/api/transport"
	"google.golang.org/grpc"
)

func TestStorageMonitoredResource(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		desc               string
		detectedAttributes []attribute.KeyValue
		wantAttributes     attribute.Set
	}{
		{
			desc: "default values set when GCP attributes are not detected",
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("global"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
		{
			desc: "use detected values when GCE attributes are detected",
			detectedAttributes: []attribute.KeyValue{
				{Key: "cloud.region",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud.platform",
					Value: attribute.StringValue("gce")},
				{Key: "host.id",
					Value: attribute.StringValue("gce-instance-id")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("gce"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("gce-instance-id"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
		{
			desc: "use detected values when FAAS attributes are detected",
			detectedAttributes: []attribute.KeyValue{
				{Key: "cloud.region",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud.platform",
					Value: attribute.StringValue("cloud-run")},
				{Key: "faas.id",
					Value: attribute.StringValue("run-instance-id")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("cloud-run"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("run-instance-id"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			smr, err := newStorageMonitoredResource(ctx, "project-id", "grpc", resource.WithAttributes(test.detectedAttributes...))
			if err != nil {
				t.Errorf("newStorageMonitoredResource: %v", err)
			}
			resultSet := smr.resource.Set()
			for _, want := range test.wantAttributes.ToSlice() {
				got, exists := resultSet.Value(want.Key)
				if !exists {
					t.Errorf("resultSet[%v] not set", want.Key)
					continue
				}
				if got != want.Value {
					t.Errorf("want[%v] = %v, got: %v", want.Key, want.Value.AsString(), got.AsString())
					continue
				}
			}
		})
	}
}

func TestNewGRPCMetricContext(t *testing.T) {
	ctx := context.Background()
	mr := metric.NewManualReader()
	attrs := []attribute.KeyValue{
		{Key: "cloud.region",
			Value: attribute.StringValue("us-central1")},
		{Key: "cloud.platform",
			Value: attribute.StringValue("gcp")},
		{Key: "host.id",
			Value: attribute.StringValue("gce-instance-id")},
	}
	cfg := metricsConfig{
		project:         "project-id",
		manualReader:    mr,
		disableExporter: true, // disable since this is a unit test
		resourceOpts:    []resource.Option{resource.WithAttributes(attrs...)},
	}
	mc, err := newGRPCMetricContext(ctx, cfg)
	if err != nil {
		t.Errorf("newGRPCMetricContext: %v", err)
	}
	defer mc.close()
	rm := metricdata.ResourceMetrics{}
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Errorf("ManualReader.Collect: %v", err)
	}
	monitoredResourceWant := map[string]string{
		"gcp.resource_type": "storage.googleapis.com/Client",
		"api":               "grpc",
		"cloud_platform":    "gcp",
		"host_id":           "gce-instance-id",
		"location":          "us-central1",
		"project_id":        "project-id",
		"instance_id":       "ignore",
	}
	for _, attr := range rm.Resource.Attributes() {
		want := monitoredResourceWant[string(attr.Key)]
		if want == "ignore" {
			continue
		}
		got := attr.Value.AsString()
		if want != got {
			t.Errorf("got: %v want: %v", got, want)
		}
	}
}

func TestNewExporterLogSuppressor(t *testing.T) {
	ctx := context.Background()
	s := &exporterLogSuppressor{Exporter: &failingExporter{}}
	if err := s.Export(ctx, nil); err == nil {
		t.Errorf("exporterLogSuppressor: did not emit an error when one was expected")
	}
	if err := s.Export(ctx, nil); err != nil {
		t.Errorf("exporterLogSuppressor: emitted an error when it should have suppressed")
	}
}

type failingExporter struct {
	metric.Exporter
}

func (f *failingExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	return fmt.Errorf("PermissionDenied")
}

func TestNewGRPCMetricContextWithCustomProvider(t *testing.T) {
	// Create a manual reader for collecting metrics.
	mr := metric.NewManualReader()

	// Create a custom meter provider with the manual reader.
	customProvider := metric.NewMeterProvider(
		metric.WithReader(mr),
	)
	ctx := context.Background()

	cfg := metricsConfig{
		project:         "project-id",
		meterProvider:   customProvider, // use custom provider
		disableExporter: true,           // disable since this is a unit test
	}

	mc, err := newGRPCMetricContext(ctx, cfg)
	if err != nil {
		t.Errorf("newGRPCMetricContext: %v", err)
	}
	defer mc.close()

	// Verify the provider in metricsContext is our custom provider.
	if mc.provider != customProvider {
		t.Errorf("metricsContext.provider = %v, want %v", mc.provider, customProvider)
	}

	// Now we can collect metrics from the manual reader that's registered with our custom provider.
	rm := metricdata.ResourceMetrics{}
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Errorf("ManualReader.Collect: %v", err)
	}
}

func TestNewGRPCMetricContextWithCustomProviderExporter(t *testing.T) {
	// 1. Create a custom meter provider.
	mr := metric.NewManualReader()
	customProvider := metric.NewMeterProvider(
		metric.WithReader(mr),
	)
	// 2. Create a custom `failingExporter`.
	failingExp := &failingExporter{}
	customExporter := metric.Exporter(failingExp)

	// 3. Create the metrics context with both the custom provider and the custom exporter.
	// The custom provider should take precedence and prevent the failing exporter from being used.
	ctx := context.Background()
	cfg := metricsConfig{
		project:        "project-id",
		meterProvider:  customProvider,
		customExporter: &customExporter,
	}

	mc, err := newGRPCMetricContext(ctx, cfg)
	if err != nil {
		t.Errorf("newGRPCMetricContext: %v", err)
	}
	// 4. Verification: The `failingExporter` should not have been used.
	// We expect the close to succeed, which means the failing exporter was not registered.
	// If the failing exporter had been used, mc.close() would likely panic or return an error.
	mc.close()
}

func TestNewGRPCMetricContext_CustomProviderSkipsDefaultExporter(t *testing.T) {
	customProvider := metric.NewMeterProvider(metric.WithReader(metric.NewManualReader()))
	mc, err := newGRPCMetricContext(context.Background(), metricsConfig{
		// Empty project and no resourceOpts would fail if newStorageMonitoredResource
		// or smr.exporter were invoked.
		meterProvider: customProvider,
	})
	if err != nil {
		t.Fatalf("newGRPCMetricContext: %v", err)
	}
	mc.close()
}

func TestResolveMetricsUniverseDomain(t *testing.T) {
	tests := []struct {
		name    string
		envUD   string
		envMTLS string
		opts    []option.ClientOption
		want    string
	}{
		{
			name: "default",
			want: "googleapis.com",
		},
		{
			name:  "env_var",
			envUD: "env.example.com",
			want:  "env.example.com",
		},
		{
			name:  "option_overrides_env_and_endpoint",
			envUD: "env.example.com",
			opts: []option.ClientOption{
				option.WithUniverseDomain("custom.example.com"),
				option.WithEndpoint("https://storage.other.example.com/storage/v1/"),
			},
			want: "custom.example.com",
		},
		{
			name:    "option_with_mtls_always",
			envMTLS: "always",
			opts:    []option.ClientOption{option.WithUniverseDomain("custom.example.com")},
			want:    "custom.example.com",
		},
		{
			name:    "default_with_mtls_always",
			envMTLS: "always",
			want:    "googleapis.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUD)
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", tc.envMTLS)
			if got := resolveMetricsUniverseDomain(tc.opts); got != tc.want {
				t.Errorf("resolveMetricsUniverseDomain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMonitoringClientOptions(t *testing.T) {
	t.Run("disabled_on_http_emulator", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "localhost:9000")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
		if _, ok := monitoringClientOptions(nil, nil, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with STORAGE_EMULATOR_HOST")
		}
	})

	t.Run("disabled_on_grpc_emulator", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "localhost:9001")
		if _, ok := monitoringClientOptions(nil, nil, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with STORAGE_EMULATOR_HOST_GRPC")
		}
	})

	t.Run("disabled_without_authentication", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithoutAuthentication()}, nil, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with WithoutAuthentication")
		}
	})

	t.Run("disabled_on_custom_transport_even_with_ambient_creds", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
		ambientCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "ambient"}}}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithHTTPClient(http.DefaultClient)}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with custom HTTP client")
		}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithGRPCConn(&grpc.ClientConn{})}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with custom gRPC conn")
		}
	})

	t.Run("forwards_universe_domain_and_cloned_auth_creds", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
		t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
		t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "")

		parentCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "tok"}}}
		clientOpts := []option.ClientOption{
			option.WithUniverseDomain("custom.example.com"),
			option.WithEndpoint("https://storage.custom.example.com/storage/v1/"),
		}

		mOpts, ok := monitoringClientOptions(clientOpts, parentCreds, nil)
		if !ok {
			t.Fatal("monitoringClientOptions() ok = false, want true")
		}

		// Verify Monitoring resolves to monitoring.<universe>:443, not monitoring.googleapis.com
		// and not the Storage endpoint.
		res, err := internaloption.NewUnsafeResolver(append(mOpts,
			internaloption.WithDefaultEndpointTemplate("monitoring.UNIVERSE_DOMAIN:443"),
			internaloption.WithDefaultUniverseDomain("googleapis.com"),
		)...)
		if err != nil {
			t.Fatalf("NewUnsafeResolver: %v", err)
		}
		ep, err := res.ResolvedGRPCEndpoint()
		if err != nil {
			t.Fatalf("ResolvedGRPCEndpoint: %v", err)
		}
		if ep != "monitoring.custom.example.com:443" {
			t.Errorf("resolved monitoring endpoint = %q, want %q", ep, "monitoring.custom.example.com:443")
		}

		// Mutate parentCreds in place (as http_client.go does when wrapping) and ensure
		// the forwarded Monitoring credentials remain untouched.
		parentCreds.TokenProvider = nil
		gotCreds, err := internaloption.AuthCreds(context.Background(), mOpts)
		if err != nil {
			t.Fatalf("internaloption.AuthCreds: %v", err)
		}
		if gotCreds == nil || gotCreds.TokenProvider == nil {
			t.Fatal("forwarded auth credentials were nil or mutated by parent in-place update")
		}
	})

	t.Run("forwards_legacy_google_creds", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		t.Setenv("STORAGE_EMULATOR_HOST_GRPC", "")
		t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
		t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "")

		gCreds := &google.Credentials{
			ProjectID:   "test-proj",
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "legacy-tok"}),
		}
		mOpts, ok := monitoringClientOptions([]option.ClientOption{option.WithUniverseDomain("custom.example.com")}, nil, gCreds)
		if !ok {
			t.Fatal("monitoringClientOptions() ok = false, want true")
		}
		res, err := internaloption.NewUnsafeResolver(append(mOpts,
			internaloption.WithDefaultEndpointTemplate("monitoring.UNIVERSE_DOMAIN:443"),
			internaloption.WithDefaultUniverseDomain("googleapis.com"),
		)...)
		if err != nil {
			t.Fatalf("NewUnsafeResolver: %v", err)
		}
		if ep, err := res.ResolvedGRPCEndpoint(); err != nil || ep != "monitoring.custom.example.com:443" {
			t.Errorf("resolved monitoring endpoint = (%q, %v), want (%q, nil)", ep, err, "monitoring.custom.example.com:443")
		}
		gotCreds, err := transport.Creds(context.Background(), mOpts...)
		if err != nil {
			t.Fatalf("transport.Creds: %v", err)
		}
		if gotCreds != gCreds {
			t.Errorf("resolved credentials = %v, want caller's legacy credentials", gotCreds)
		}
	})
}

func TestInitMetrics_EmptyProjectIDReturnsError(t *testing.T) {
	// Ensure initMetrics fails fast instead of passing an empty project ID to
	// mexporter.New (which would fall back to google.FindDefaultCredentials).
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCE_METADATA_HOST", "127.0.0.1:1")
	if _, _, err := initMetrics(context.Background(), "", &storageConfig{}); err == nil {
		t.Fatal("initMetrics() with empty projectID = nil, want error")
	}
}
