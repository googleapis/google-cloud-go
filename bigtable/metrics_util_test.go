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
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/metadata"
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
			wantError:   errors.New("strconv.ParseFloat: parsing \"\": invalid syntax"),
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
			wantError:   errors.New("failed to get location metadata"),
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
