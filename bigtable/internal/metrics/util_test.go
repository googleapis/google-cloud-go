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

package internal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// equalErrs compares two errors by string containment. Local helper
// so util_test.go doesn't need to import from the bigtable package
// (which is upstream of the metrics package).
func equalErrs(gotErr error, wantErr error) bool {
	if gotErr == nil && wantErr == nil {
		return true
	}
	if gotErr == nil || wantErr == nil {
		return false
	}
	return strings.Contains(gotErr.Error(), wantErr.Error())
}

// Test fixtures — duplicated from the bigtable-package metrics_test.go
// since util_test.go lives in a separate package now.
var (
	clusterID1 = "cluster-id-1"
	clusterID2 = "cluster-id-2"
	zoneID1    = "zone-id-1"

	testHeadersUtil, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID1,
		ZoneId:    &zoneID1,
	})
	testTrailersUtil, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID2,
		ZoneId:    &zoneID1,
	})

	testHeaderMD = &metadata.MD{
		LocationMDKey:     []string{string(testHeadersUtil)},
		ServerTimingMDKey: []string{"gfet4t7; dur=1234"},
	}
	testTrailerMD = &metadata.MD{
		LocationMDKey:     []string{string(testTrailersUtil)},
		ServerTimingMDKey: []string{"gfet4t7; dur=5678"},
	}
)

func TestExtractServerLatency(t *testing.T) {
	invalidFormat := "invalid format"
	invalidFormatMD := metadata.MD{
		ServerTimingMDKey: []string{invalidFormat},
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
				ServerTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD:   metadata.MD{},
			wantLatency: 1234,
			wantError:   nil,
		},
		{
			desc:     "Server latency in trailer",
			headerMD: metadata.MD{},
			trailerMD: metadata.MD{
				ServerTimingMDKey: []string{"gfet4t7; dur=5678"},
			},
			wantLatency: 5678,
			wantError:   nil,
		},
		{
			desc: "Server latency in both header and trailer",
			headerMD: metadata.MD{
				ServerTimingMDKey: []string{"gfet4t7; dur=1234"},
			},
			trailerMD: metadata.MD{
				ServerTimingMDKey: []string{"gfet4t7; dur=5678"},
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
			gotLatency, gotErr := ExtractServerLatency(test.headerMD, test.trailerMD)
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
				LocationMDKey: []string{"invalid format"},
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
				LocationMDKey: []string{"invalid format"},
			},
			wantCluster: defaultCluster,
			wantZone:    defaultZone,
			wantError:   errors.New(invalidFormatErr),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotCluster, gotZone, gotErr := ExtractLocation(test.headerMD, test.trailerMD)
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
		t.Fatalf("marshal peer info: %v", err)
	}
	// Server sends URL-safe base64; unpadded (RawURL) is the wire shape,
	// but URLEncoding (padded) must also decode — see ExtractPeerInfo.
	validRawURL := base64.RawURLEncoding.EncodeToString(protoBytes)
	validURLPadded := base64.URLEncoding.EncodeToString(protoBytes)
	invalidBase64 := "invalid-base64-data-$$$"
	invalidProtoBase64 := base64.RawURLEncoding.EncodeToString([]byte("not-a-protobuf"))

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
			desc:      "Peer info in header (raw url, unpadded)",
			headerMD:  metadata.MD{PeerInfoMDKey: []string{validRawURL}},
			trailerMD: metadata.MD{},
			want:      expectedPeerInfo,
			wantErr:   false,
		},
		{
			desc:      "Peer info in header (url, padded)",
			headerMD:  metadata.MD{PeerInfoMDKey: []string{validURLPadded}},
			trailerMD: metadata.MD{},
			want:      expectedPeerInfo,
			wantErr:   false,
		},
		{
			desc:      "Peer info in trailer",
			headerMD:  metadata.MD{},
			trailerMD: metadata.MD{PeerInfoMDKey: []string{validRawURL}},
			want:      expectedPeerInfo,
			wantErr:   false,
		},
		{
			desc:      "Header wins over trailer",
			headerMD:  metadata.MD{PeerInfoMDKey: []string{validRawURL}},
			trailerMD: metadata.MD{PeerInfoMDKey: []string{"garbage"}},
			want:      expectedPeerInfo,
			wantErr:   false,
		},
		{
			desc:      "Invalid base64",
			headerMD:  metadata.MD{PeerInfoMDKey: []string{invalidBase64}},
			trailerMD: metadata.MD{},
			want:      nil,
			wantErr:   true,
		},
		{
			desc:      "Invalid protobuf",
			headerMD:  metadata.MD{PeerInfoMDKey: []string{invalidProtoBase64}},
			trailerMD: metadata.MD{},
			want:      nil,
			wantErr:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, err := ExtractPeerInfo(test.headerMD, test.trailerMD)
			if (err != nil) != test.wantErr {
				t.Fatalf("ExtractPeerInfo() err = %v, wantErr %v", err, test.wantErr)
			}
			if !proto.Equal(got, test.want) {
				t.Errorf("ExtractPeerInfo() got = %v, want %v", got, test.want)
			}
		})
	}
}

func TestToOtelMetricAttrsAttemptLatencies2(t *testing.T) {
	tracer := &Tracer{
		method:      metricMethodPrefix + "ReadRows",
		tableName:   "test-table",
		isStreaming: true,
		clientAttributes: []attribute.KeyValue{
			attribute.String(MonitoredResLabelKeyProject, "test-project"),
		},
		currOp: OpTracer{
			status: "UNAVAILABLE",
			currAttempt: AttemptTracer{
				status:           "OK",
				clusterID:        "test-cluster",
				zoneID:           "test-zone",
				transportType:    "cloudpath",
				transportRegion:  "us-central1",
				transportZone:    "us-central1-b",
				transportSubZone: "sub-1",
			},
		},
	}

	attrSet, err := tracer.toOtelMetricAttrs(MetricNameAttemptLatencies2)
	if err != nil {
		t.Fatalf("toOtelMetricAttrs: %v", err)
	}
	want := map[attribute.Key]attribute.Value{
		MetricLabelKeyMethod:             attribute.StringValue("Bigtable.ReadRows"),
		MonitoredResLabelKeyTable:        attribute.StringValue("test-table"),
		MonitoredResLabelKeyCluster:      attribute.StringValue("test-cluster"),
		MonitoredResLabelKeyZone:         attribute.StringValue("test-zone"),
		MonitoredResLabelKeyProject:      attribute.StringValue("test-project"),
		MetricLabelKeyStatus:             attribute.StringValue("OK"),
		MetricLabelKeyStreamingOperation: attribute.BoolValue(true),
		MetricTransportType:              attribute.StringValue("cloudpath"),
		MetricTransportRegion:            attribute.StringValue("us-central1"),
		MetricTransportZone:              attribute.StringValue("us-central1-b"),
		MetricTransportSubZone:           attribute.StringValue("sub-1"),
	}
	for key, expected := range want {
		val, ok := attrSet.Value(key)
		if !ok {
			t.Errorf("missing attribute %v", key)
			continue
		}
		if val != expected {
			t.Errorf("attr[%v] = %v, want %v", key, val.Emit(), expected.Emit())
		}
	}
	if attrSet.Len() != len(want) {
		t.Errorf("attr count = %d, want %d", attrSet.Len(), len(want))
	}
}
