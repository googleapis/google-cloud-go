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
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestExtractServerLatency(t *testing.T) {
	invalidFormat := "invalid format"
	invalidFormatMD := metadata.MD{
		serverTimingMDKey: []string{invalidFormat},
	}
	invalidFormatErr := fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", invalidFormat)

	tests := []struct {
		desc        string
		headerMD    metadata.MD
		trailerMD   metadata.MD
		wantLatency float64
		wantError   error
	}{
		{
			desc:        "No server latency in header or trailer",
			headerMD:    metadata.MD{},
			trailerMD:   metadata.MD{},
			wantLatency: 0,
			wantError:   fmt.Errorf("strconv.ParseFloat: parsing \"\": invalid syntax"),
		},
		{
			desc: "Server latency in header",
			headerMD: metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD:   metadata.MD{},
			wantLatency: 1234,
			wantError:   nil,
		},
		{
			desc:     "Server latency in trailer",
			headerMD: metadata.MD{},
			trailerMD: metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=5678"},
			},
			wantLatency: 5678,
			wantError:   nil,
		},
		{
			desc: "Server latency in both header and trailer",
			headerMD: metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD: metadata.MD{
				serverTimingMDKey: []string{"gfet4t7; dur=5678"},
			},
			wantLatency: 1234,
			wantError:   nil,
		},
		{
			desc:        "Invalid server latency format in header",
			headerMD:    invalidFormatMD,
			trailerMD:   metadata.MD{},
			wantLatency: 0,
			wantError:   invalidFormatErr,
		},
		{
			desc:        "Invalid server latency format in trailer",
			headerMD:    metadata.MD{},
			trailerMD:   invalidFormatMD,
			wantLatency: 0,
			wantError:   invalidFormatErr,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotLatency, gotErr := extractServerLatency(test.headerMD, test.trailerMD)
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
			if gotLatency != test.wantLatency {
				t.Errorf("latency got: %v, want: %v", gotLatency, test.wantLatency)
			}
		})
	}
}

func TestExtractLocation(t *testing.T) {
	invalidFormatErr := "cannot parse invalid wire-format data"
	tests := []struct {
		desc        string
		headerMD    metadata.MD
		trailerMD   metadata.MD
		wantCluster string
		wantZone    string
		wantError   error
	}{
		{
			desc:        "No location metadata in header or trailer",
			headerMD:    metadata.MD{},
			trailerMD:   metadata.MD{},
			wantCluster: defaultCluster,
			wantZone:    defaultZone,
			wantError:   fmt.Errorf("failed to get location metadata"),
		},
		{
			desc:        "Location metadata in header",
			headerMD:    *testHeaderMD,
			trailerMD:   metadata.MD{},
			wantCluster: clusterID1,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc:        "Location metadata in trailer",
			headerMD:    metadata.MD{},
			trailerMD:   *testTrailerMD,
			wantCluster: clusterID2,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc:        "Location metadata in both header and trailer",
			headerMD:    *testHeaderMD,
			trailerMD:   *testTrailerMD,
			wantCluster: clusterID1,
			wantZone:    zoneID1,
			wantError:   nil,
		},
		{
			desc: "Invalid location metadata format in header",
			headerMD: metadata.MD{
				locationMDKey: []string{"invalid format"},
			},
			trailerMD:   metadata.MD{},
			wantCluster: defaultCluster,
			wantZone:    defaultZone,
			wantError:   errors.New(invalidFormatErr),
		},
		{
			desc:     "Invalid location metadata format in trailer",
			headerMD: metadata.MD{},
			trailerMD: metadata.MD{
				locationMDKey: []string{"invalid format"},
			},
			wantCluster: defaultCluster,
			wantZone:    defaultZone,
			wantError:   errors.New(invalidFormatErr),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotCluster, gotZone, gotErr := extractLocation(test.headerMD, test.trailerMD)
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

func TestExtractPeerInfo(t *testing.T) {
	expectedPeerInfo := &btpb.PeerInfo{
		TransportType: btpb.PeerInfo_TRANSPORT_TYPE_CLOUD_PATH,
	}

	protoBytes, err := proto.Marshal(expectedPeerInfo)
	if err != nil {
		t.Fatalf("failed to marshal peer info for test setup: %v", err)
	}

	// Encode it to Raw URL-safe Base64
	validBase64 := base64.RawURLEncoding.EncodeToString(protoBytes)

	// 2. Create mock invalid data
	invalidBase64 := "invalid-base64-data-$$$"

	invalidProtoBytes := []byte("this is definitely not a valid protobuf message")
	invalidProtoBase64 := base64.RawURLEncoding.EncodeToString(invalidProtoBytes)

	tests := []struct {
		desc      string
		headerMD  metadata.MD
		trailerMD metadata.MD
		want      *btpb.PeerInfo
		wantErr   bool
	}{
		{
			desc:      "No peer info in header or trailer",
			headerMD:  metadata.MD{},
			trailerMD: metadata.MD{},
			want:      nil,
			wantErr:   false,
		},
		{
			desc: "Peer info in header",
			headerMD: metadata.MD{
				peerInfoMDKey: []string{validBase64},
			},
			trailerMD: metadata.MD{},
			want:      expectedPeerInfo,
			wantErr:   false,
		},
		{
			desc:     "Peer info in trailer",
			headerMD: metadata.MD{},
			trailerMD: metadata.MD{
				peerInfoMDKey: []string{validBase64},
			},
			want:    expectedPeerInfo,
			wantErr: false,
		},
		{
			desc: "Peer info in both header and trailer (header takes precedence)",
			headerMD: metadata.MD{
				peerInfoMDKey: []string{validBase64},
			},
			trailerMD: metadata.MD{
				peerInfoMDKey: []string{"some-other-data"},
			},
			want:    expectedPeerInfo,
			wantErr: false,
		},
		{
			desc: "Invalid base64 in header",
			headerMD: metadata.MD{
				peerInfoMDKey: []string{invalidBase64},
			},
			trailerMD: metadata.MD{},
			want:      nil,
			wantErr:   true,
		},
		{
			desc: "Invalid protobuf in header",
			headerMD: metadata.MD{
				peerInfoMDKey: []string{invalidProtoBase64},
			},
			trailerMD: metadata.MD{},
			want:      nil,
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := extractPeerInfo(test.headerMD, test.trailerMD)

			// Check error expectations
			if (err != nil) != test.wantErr {
				t.Errorf("extractPeerInfo() error = %v, wantErr %v", err, test.wantErr)
				return
			}

			// Check if the returned protobuf matches our expectation
			// proto.Equal is used to safely compare protobuf messages in Go
			if !proto.Equal(got, test.want) {
				t.Errorf("extractPeerInfo() got = %v, want %v", got, test.want)
			}
		})
	}
}

func TestToOtelMetricAttrsAttemptLatencies2(t *testing.T) {
	tracer := &builtinMetricsTracer{
		method:      metricMethodPrefix + "ReadRows",
		tableName:   "test-table",
		isStreaming: true,
		clientAttributes: []attribute.KeyValue{
			attribute.String(monitoredResLabelKeyProject, "test-project"),
		},
		currOp: opTracer{
			status: "UNAVAILABLE", // Operation status
			currAttempt: attemptTracer{
				status:           "OK", // Attempt status (should override operation status)
				clusterID:        "test-cluster",
				zoneID:           "test-zone",
				transportType:    "TRANSPORT_TYPE_CLOUD_PATH",
				transportRegion:  "us-central1",
				transportZone:    "us-central1-b",
				transportSubZone: "sub-1",
			},
		},
	}

	attrSet, err := tracer.toOtelMetricAttrs(metricNameAttemptLatencies2)
	if err != nil {
		t.Fatalf("unexpected error generating attributes: %v", err)
	}

	expectedAttrs := map[attribute.Key]attribute.Value{
		metricLabelKeyMethod:             attribute.StringValue("Bigtable.ReadRows"),
		monitoredResLabelKeyTable:        attribute.StringValue("test-table"),
		monitoredResLabelKeyCluster:      attribute.StringValue("test-cluster"),
		monitoredResLabelKeyZone:         attribute.StringValue("test-zone"),
		monitoredResLabelKeyProject:      attribute.StringValue("test-project"),
		metricLabelKeyStatus:             attribute.StringValue("OK"),
		metricLabelKeyStreamingOperation: attribute.BoolValue(true),
		metricTransportType:              attribute.StringValue("TRANSPORT_TYPE_CLOUD_PATH"),
		metricTransportRegion:            attribute.StringValue("us-central1"),
		metricTransportZone:              attribute.StringValue("us-central1-b"),
		metricTransportSubZone:           attribute.StringValue("sub-1"),
	}

	for key, expectedVal := range expectedAttrs {
		val, ok := attrSet.Value(key)
		if !ok {
			t.Errorf("missing expected attribute key: %v", key)
			continue
		}
		if val != expectedVal {
			t.Errorf("attribute [%v] = %v, want %v", key, val.Emit(), expectedVal.Emit())
		}
	}

	if attrSet.Len() != len(expectedAttrs) {
		t.Errorf("got %d attributes, want %d", attrSet.Len(), len(expectedAttrs))
	}
}
