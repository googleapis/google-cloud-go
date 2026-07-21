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
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// InvokeResult carries the full set of outputs from a single Invoke call.
//
// Fields:
//   - Response: decoded vRPC payload (typed per VRpcDescriptor.Decode); nil on error.
//   - ClusterInfo: server-reported routing/cluster identity; may be set on
//     both success and error paths if the server included it.
//   - Stats: server-reported per-request statistics (notably BackendLatency);
//     nil if the server did not populate Stats on the success frame.
//   - SentAt: local monotonic timestamp captured immediately before the vRPC
//     frame was handed to the bidi Send. Used downstream to derive
//     client-side blocking latency (sentAt - attemptStart).
//
// ErrorResponse.RetryInfo from the server is plumbed via the returned error
// using gRPC status details — callers can extract it with
// status.FromError(err).Details() and type-asserting to *errdetails.RetryInfo.
type InvokeResult struct {
	Response    interface{}
	ClusterInfo *spb.ClusterInformation
	Stats       *spb.SessionRequestStats
	SentAt      time.Time
	// PeerInfo is the serving session's parsed peer info (from the
	// bigtable-peer-info header the server sent on session open). Bound to
	// the session, not this specific vRPC — every InvokeResult on the same
	// session carries the same pointer. Nil when the pool bypassed a
	// session (checkout failure). Feeds the outer metrics tracer's
	// transport_type / transport_region / transport_zone / transport_subzone
	// attributes on attempt_latencies2.
	PeerInfo *spb.PeerInfo
	// RpcIDOnSession is the per-session monotonic id of this call
	// (1, 2, 3, …). Distinguishes warm-up vRPCs (small id) from
	// established-session vRPCs.
	RpcIDOnSession int64
	// TransportLatency is the time between the vRPC frame being handed
	// to the bidi Send and the response (or server-side error) arriving
	// on the stream. Approximates network RTT + server queue + Backend;
	// (TransportLatency - BackendLatency) surfaces "everything except
	// server processing". Zero when Invoke returned before a Recv event
	// (context cancellation or pre-Send failure).
	TransportLatency time.Duration
}
