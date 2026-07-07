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

package internal

import (
	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// FineGrainLatencyBounds matches java-bigtable's
// AGGREGATION_WITH_MILLIS_HISTOGRAM: fine sub-ms + coarse tail. Shared
// by transport_latencies and attempt_latencies2.
var FineGrainLatencyBounds = []float64{
	// Linear 0 → 3ms by 0.1ms (31 boundaries): fine-grained sub-ms.
	0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9,
	1.0, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9,
	2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 3.0,
	// Coarse 4ms → 80ms.
	4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0, 20.0, 25.0, 30.0, 40.0, 50.0, 65.0, 80.0,
	// Coarse 100ms → 900ms.
	100.0, 130.0, 160.0, 200.0, 250.0, 300.0, 400.0, 500.0, 650.0, 800.0, 900.0,
	// Coarse 1s → 50s.
	1000.0, 2000.0, 3000.0, 4000.0, 5000.0, 6000.0, 10000.0, 20000.0, 50000.0,
	// Long tail: 100s → 5000s (~83 min).
	100000.0, 200000.0, 500000.0, 1000000.0, 2000000.0, 5000000.0,
}

// TransportTypeName maps the PeerInfo transport type enum to the short
// label used in metric attributes and debug UIs (e.g. "cloudpath",
// "session_directpath"). Prefer this over .String(), which yields the
// verbose "TRANSPORT_TYPE_…" proto enum names. Exported so the outer
// bigtable package can share the same mapping (attempt_latencies2's
// transport_type attribute) without duplicating the switch.
func TransportTypeName(tt spb.PeerInfo_TransportType) string {
	switch tt {
	case spb.PeerInfo_TRANSPORT_TYPE_EXTERNAL:
		return "external"
	case spb.PeerInfo_TRANSPORT_TYPE_CLOUD_PATH:
		return "cloudpath"
	case spb.PeerInfo_TRANSPORT_TYPE_DIRECT_ACCESS:
		return "directpath"
	case spb.PeerInfo_TRANSPORT_TYPE_SESSION_EXTERNAL:
		return "session_external"
	case spb.PeerInfo_TRANSPORT_TYPE_SESSION_CLOUD_PATH:
		return "session_cloudpath"
	case spb.PeerInfo_TRANSPORT_TYPE_SESSION_DIRECT_ACCESS:
		return "session_directpath"
	case spb.PeerInfo_TRANSPORT_TYPE_SESSION_UNKNOWN:
		return "session_unknown"
	default:
		return "unknown"
	}
}
