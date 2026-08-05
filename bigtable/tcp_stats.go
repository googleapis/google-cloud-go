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

package bigtable

import (
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// TCPStats is the opt-in collector for per-connection TCP statistics
// (RTT, retransmits, cwnd, etc.) from every gRPC dial a bigtable client
// makes. Create one, pass its ClientOption() into NewClient / NewAdminClient,
// then hand the same TCPStats to tcpz.Handler to expose a live debug page.
//
// The collector never wraps the returned net.Conn — it just records a
// reference for later TCP_INFO reads. The RPC hot path traverses zero
// TCPStats code. Cost is limited to one map insert per gRPC dial and one
// getsockopt(TCP_INFO) per conn per debug-page render.
//
// Not compatible with DirectPath. When the client runs over DirectPath
// (xDS-managed connections) the standard dialer is bypassed, so nothing
// is registered and Snapshot returns empty.
//
// Linux only. On other platforms Snapshot returns entries with Err
// populated ("tcp_info not supported on this platform"); dialing and
// registration still work but the numeric fields stay zero.
type TCPStats struct {
	reg *btransport.ConnRegistry
}

// NewTCPStats constructs an empty collector. Call ClientOption() to wire
// it into a Client, and pass the same *TCPStats to tcpz.Handler.
func NewTCPStats() *TCPStats {
	return &TCPStats{reg: btransport.NewConnRegistry()}
}

// ClientOption returns an option.ClientOption that installs the TCP-stats
// dialer. Safe to pass to NewClient, NewClientWithConfig, NewAdminClient,
// etc. — the underlying grpc.WithContextDialer is additive to whatever
// gRPC options bigtable already applies.
func (t *TCPStats) ClientOption() option.ClientOption {
	return option.WithGRPCDialOption(grpc.WithContextDialer(t.reg.Dial))
}

// Snapshot returns the current per-connection TCP_INFO for every conn the
// dialer captured, oldest first. Stale entries (fd closed by gRPC) are
// pruned during Snapshot so this list stays honest.
func (t *TCPStats) Snapshot() []btransport.TCPInfoSnapshot {
	return t.reg.Snapshot()
}

// Len returns the number of currently-registered conns (including any
// closed-but-not-yet-pruned ones). Useful for a "conns=N" summary.
func (t *TCPStats) Len() int { return t.reg.Len() }

// DeadConns returns the recently-departed conns the registry still
// remembers, oldest death first. Used by tcpz to plot conn-lifetime
// distributions that include already-closed conns. Bounded — very old
// deaths eventually fall off the ring.
func (t *TCPStats) DeadConns() []btransport.DeadConnInfo { return t.reg.DeadConns() }
