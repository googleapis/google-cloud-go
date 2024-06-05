/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"google.golang.org/grpc/metadata"
)

var (
	clusterID1 = "cluster-id-1"
	clusterID2 = "cluster-id-2"
	zoneID1    = "zone-id-1"

	testHeaders, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID1,
		ZoneId:    &zoneID1,
	})
	testTrailers, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID2,
		ZoneId:    &zoneID1,
	})

	testHeaderMD = &metadata.MD{
		locationMDKey:     []string{string(testHeaders)},
		serverTimingMDKey: []string{"gfet4t7; dur=1234"},
	}
	testTrailerMD = &metadata.MD{
		locationMDKey:     []string{string(testTrailers)},
		serverTimingMDKey: []string{"gfet4t7; dur=5678"},
	}
)

func equalErrs(gotErr error, wantErr error) bool {
	if gotErr == nil && wantErr == nil {
		return true
	}
	if gotErr == nil || wantErr == nil {
		return false
	}
	return strings.Contains(gotErr.Error(), wantErr.Error())
}

type unknownMetricsProvider struct{}

func (unknownMetricsProvider) isMetricsProvider() {}

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	appProfile := "test-app-profile"
	clientUID := "test-uid"

	origGenerateClientUID := generateClientUID
	generateClientUID = func() (string, error) {
		return clientUID, nil
	}
	defer func() {
		generateClientUID = origGenerateClientUID
	}()

	wantClientAttributes := []attribute.KeyValue{
		attribute.String(monitoredResLabelKeyProject, project),
		attribute.String(monitoredResLabelKeyInstance, instance),
		attribute.String(metricLabelKeyAppProfile, appProfile),
		attribute.String(metricLabelKeyClientUID, clientUID),
		attribute.String(metricLabelKeyClientName, clientName),
	}

	mpOpts, err := WithBuiltIn(ctx, project)
	if err != nil {
		t.Fatalf("WithBuiltIn failed: %v", err)
	}
	customMeterProvider := sdkmetric.NewMeterProvider(mpOpts...)
	customOpenTelemetryMetricsProvider := CustomOpenTelemetryMetricsProvider{MeterProvider: customMeterProvider}

	tests := []struct {
		desc               string
		metricsProvider    MetricsProvider
		wantError          error
		wantBuiltinEnabled bool
		setEmulator        bool
	}{
		{
			desc:               "should create a new tracer factory with default meter provider",
			metricsProvider:    nil,
			wantBuiltinEnabled: true,
		},
		{
			desc:               "should create a new tracer factory with custom meter provider",
			metricsProvider:    customOpenTelemetryMetricsProvider,
			wantBuiltinEnabled: true,
		},
		{
			desc:            "should create a new tracer factory with noop meter provider",
			metricsProvider: NoopMetricsProvider{},
		},
		{
			desc:            "should not create instruments when BIGTABLE_EMULATOR_HOST is set",
			metricsProvider: customOpenTelemetryMetricsProvider,
			setEmulator:     true,
		},
		{
			desc:            "should return an error for unknown metrics provider type",
			metricsProvider: unknownMetricsProvider{},
			wantError:       errors.New("Unknown MetricsProvider type"),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.setEmulator {
				// Set environment variable
				t.Setenv("BIGTABLE_EMULATOR_HOST", "localhost:8086")
			}
			tracerFactory, gotErr := newBuiltinMetricsTracerFactory(ctx, project, instance, appProfile, test.metricsProvider)
			if !equalErrs(gotErr, test.wantError) {
				t.Fatalf("err: got: %v, want: %v", gotErr, test.wantError)
			}
			if tracerFactory.builtinEnabled != test.wantBuiltinEnabled {
				t.Errorf("builtinEnabled: got: %v, want: %v", tracerFactory.builtinEnabled, test.wantBuiltinEnabled)
			}

			if diff := testutil.Diff(tracerFactory.clientAttributes, wantClientAttributes,
				cmpopts.IgnoreUnexported(attribute.KeyValue{}, attribute.Value{})); diff != "" {
				t.Errorf("clientAttributes: got=-, want=+ \n%v", diff)
			}

			// Check instruments
			gotNonNilInstruments := tracerFactory.operationLatencies != nil &&
				tracerFactory.serverLatencies != nil &&
				tracerFactory.attemptLatencies != nil &&
				tracerFactory.retryCount != nil
			if test.wantBuiltinEnabled != gotNonNilInstruments {
				t.Errorf("NonNilInstruments: got: %v, want: %v", gotNonNilInstruments, test.wantBuiltinEnabled)
			}
		})
	}
}

func TestToOtelMetricAttrs(t *testing.T) {
	mt := builtinMetricsTracer{
		tableName:    "my-table",
		appProfileID: "my-app-profile",
		method:       "ReadRows",
		isStreaming:  true,
		status:       codes.OK.String(),
		headerMD:     testHeaderMD,
		trailerMD:    &metadata.MD{},
		attemptCount: 1,
	}
	tests := []struct {
		desc       string
		mt         builtinMetricsTracer
		metricName string
		wantAttrs  []attribute.KeyValue
		wantError  error
	}{
		{
			desc:       "Known metric",
			mt:         mt,
			metricName: metricNameOperationLatencies,
			wantAttrs: []attribute.KeyValue{
				attribute.String(monitoredResLabelKeyTable, "my-table"),
				attribute.String(metricLabelKeyMethod, "ReadRows"),
				attribute.Bool(metricLabelKeyStreamingOperation, true),
				attribute.String(metricLabelKeyOperationStatus, codes.OK.String()),
				attribute.String(monitoredResLabelKeyCluster, clusterID1),
				attribute.String(monitoredResLabelKeyZone, zoneID1),
			},
			wantError: nil,
		},
		{
			desc:       "Unknown metric",
			mt:         mt,
			metricName: "unknown_metric",
			wantAttrs:  nil,
			wantError:  fmt.Errorf("Unable to create attributes list for unknown metric: unknown_metric"),
		},
	}

	lessKeyValue := func(a, b attribute.KeyValue) bool { return a.Key < b.Key }
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotAttrs, gotErr := test.mt.toOtelMetricAttrs(test.metricName)
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
			if diff := testutil.Diff(gotAttrs, test.wantAttrs,
				cmpopts.IgnoreUnexported(attribute.KeyValue{}, attribute.Value{}),
				cmpopts.SortSlices(lessKeyValue)); diff != "" {
				t.Errorf("got=-, want=+ \n%v", diff)
			}
		})
	}
}

func TestGetServerLatency(t *testing.T) {
	invalidFormat := "invalid format"
	invalidFormatMD := &metadata.MD{
		serverTimingMDKey: []string{invalidFormat},
	}
	invalidFormatErr := fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", invalidFormat)

	tests := []struct {
		desc        string
		headerMD    *metadata.MD
		trailerMD   *metadata.MD
		wantLatency float64
		wantError   error
	}{
		{
			desc:        "No server latency in header or trailer",
			headerMD:    &metadata.MD{},
			trailerMD:   &metadata.MD{},
			wantLatency: 0,
			wantError:   fmt.Errorf("strconv.ParseFloat: parsing \"\": invalid syntax"),
		},
		{
			desc: "Server latency in header",
			headerMD: &metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD:   &metadata.MD{},
			wantLatency: 1234,
			wantError:   nil,
		},
		{
			desc:     "Server latency in trailer",
			headerMD: &metadata.MD{},
			trailerMD: &metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=5678"},
			},
			wantLatency: 5678,
			wantError:   nil,
		},
		{
			desc: "Server latency in both header and trailer",
			headerMD: &metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD: &metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=5678"},
			},
			wantLatency: 1234,
			wantError:   nil,
		},
		{
			desc:        "Invalid server latency format in header",
			headerMD:    invalidFormatMD,
			trailerMD:   &metadata.MD{},
			wantLatency: 0,
			wantError:   invalidFormatErr,
		},
		{
			desc:        "Invalid server latency format in trailer",
			headerMD:    &metadata.MD{},
			trailerMD:   invalidFormatMD,
			wantLatency: 0,
			wantError:   invalidFormatErr,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			mt := builtinMetricsTracer{
				headerMD:  test.headerMD,
				trailerMD: test.trailerMD,
			}

			gotLatency, gotErr := mt.getServerLatency()
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
			if gotLatency != test.wantLatency {
				t.Errorf("latency got: %v, want: %v", gotLatency, test.wantLatency)
			}
		})
	}
}

func TestGetLocation(t *testing.T) {
	invalidFormatErr := "cannot parse invalid wire-format data"
	tests := []struct {
		desc        string
		headerMD    *metadata.MD
		trailerMD   *metadata.MD
		wantCluster string
		wantZone    string
		wantError   error
	}{
		{
			desc:        "No location metadata in header or trailer",
			headerMD:    &metadata.MD{},
			trailerMD:   &metadata.MD{},
			wantCluster: "",
			wantZone:    "",
			wantError:   fmt.Errorf("Failed to get location metadata"),
		},
		{
			desc:        "Location metadata in header",
			headerMD:    testHeaderMD,
			trailerMD:   &metadata.MD{},
			wantCluster: clusterID1,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc:        "Location metadata in trailer",
			headerMD:    &metadata.MD{},
			trailerMD:   testTrailerMD,
			wantCluster: clusterID2,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc:        "Location metadata in both header and trailer",
			headerMD:    testHeaderMD,
			trailerMD:   testTrailerMD,
			wantCluster: clusterID1,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc: "Invalid location metadata format in header",
			headerMD: &metadata.MD{
				locationMDKey: []string{"invalid format"},
			},
			trailerMD:   &metadata.MD{},
			wantCluster: "",
			wantZone:    "",
			wantError:   fmt.Errorf(invalidFormatErr),
		},
		{
			desc:     "Invalid location metadata format in trailer",
			headerMD: &metadata.MD{},
			trailerMD: &metadata.MD{
				locationMDKey: []string{"invalid format"},
			},
			wantCluster: "",
			wantZone:    "",
			wantError:   fmt.Errorf(invalidFormatErr),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			mt := builtinMetricsTracer{
				headerMD:  test.headerMD,
				trailerMD: test.trailerMD,
			}

			gotCluster, gotZone, gotErr := mt.getLocation()
			if gotCluster != test.wantCluster {
				t.Errorf("cluster got: %v, want: %v", gotCluster, test.wantCluster)
			}
			if gotZone != test.wantZone {
				t.Errorf("zone got: %v, want: %v", gotZone, test.wantZone)
			}
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
		})
	}
}
