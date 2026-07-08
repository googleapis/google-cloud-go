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

import spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"

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
