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
	"context"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/peer"
)

// fakeAltsAuthInfo satisfies both credentials.AuthInfo (via AuthType) and
// alts.AuthInfo (via the embedded nil interface, which propagates the alts
// method set to this type at compile time). The type-assertion path in
// recordProbePeer only cares about the method set — no method is actually
// invoked — so the embedded nil is safe here.
type fakeAltsAuthInfo struct{ alts.AuthInfo }

func (fakeAltsAuthInfo) AuthType() string { return "alts" }

// TestGetClientConfigDirectAccessChecker_ImplementsInterface is a compile-time
// guard so *getClientConfigDirectAccessChecker keeps satisfying the
// DirectAccessChecker interface as either side evolves.
func TestGetClientConfigDirectAccessChecker_ImplementsInterface(t *testing.T) {
	var _ DirectAccessChecker = (*getClientConfigDirectAccessChecker)(nil)
}

// TestGetClientConfigDirectAccessChecker_DialerReturnsConfigured verifies
// Dialer() hands back the exact func passed at construction — the pool
// factory relies on this to reuse the dialer for post-probe redials.
func TestGetClientConfigDirectAccessChecker_DialerReturnsConfigured(t *testing.T) {
	want := func() (*BigtableConn, error) { return nil, nil }
	c := newGetClientConfigDirectAccessChecker(want, "projects/p/instances/i", "profile", nil, nil, nil)
	got := c.Dialer()
	if fmt.Sprintf("%p", got) != fmt.Sprintf("%p", want) {
		t.Errorf("Dialer() returned a different function than configured")
	}
}

// TestGetClientConfigDirectAccessChecker_DialFailedShortCircuits verifies
// CheckCompatibility returns (nil, false) when the dialer fails, without
// panicking on the nil BigtableConn or trying to close it.
func TestGetClientConfigDirectAccessChecker_DialFailedShortCircuits(t *testing.T) {
	dialer := func() (*BigtableConn, error) { return nil, fmt.Errorf("dial failed") }
	c := newGetClientConfigDirectAccessChecker(dialer, "projects/p/instances/i", "profile", nil, nil, nil)

	conn, compatible := c.CheckCompatibility(context.Background())
	if compatible {
		t.Errorf("compatible = true on dial failure, want false")
	}
	if conn != nil {
		t.Errorf("conn = %v on dial failure, want nil", conn)
	}
}

// TestRecordProbePeer_ALTSAndIPv4 covers the observation side effects that
// CheckCompatibility relies on: a peer with alts.AuthInfo + IPv4 TCPAddr
// must set isALTSConn=true and remoteAddrType=ipv4 so ipProtocol() reports
// "ipv4" in the success-metric label.
func TestRecordProbePeer_ALTSAndIPv4(t *testing.T) {
	bc := &BigtableConn{}
	p := &peer.Peer{
		Addr:     &net.TCPAddr{IP: net.ParseIP("10.0.0.1")},
		AuthInfo: fakeAltsAuthInfo{},
	}
	recordProbePeer(bc, p)

	if !bc.isALTSConn.Load() {
		t.Error("isALTSConn = false, want true for ALTS AuthInfo")
	}
	if got := bc.ipProtocol(); got != "ipv4" {
		t.Errorf("ipProtocol() = %q, want %q", got, "ipv4")
	}
}

// TestRecordProbePeer_ALTSAndIPv6 mirrors the IPv4 test on IPv6 so the
// remoteAddrType branch is exercised.
func TestRecordProbePeer_ALTSAndIPv6(t *testing.T) {
	bc := &BigtableConn{}
	p := &peer.Peer{
		Addr:     &net.TCPAddr{IP: net.ParseIP("2001:db8::1")},
		AuthInfo: fakeAltsAuthInfo{},
	}
	recordProbePeer(bc, p)

	if !bc.isALTSConn.Load() {
		t.Error("isALTSConn = false, want true for ALTS AuthInfo")
	}
	if got := bc.ipProtocol(); got != "ipv6" {
		t.Errorf("ipProtocol() = %q, want %q", got, "ipv6")
	}
}

// TestRecordProbePeer_NonALTS ensures a non-ALTS AuthInfo (TLS,
// InsecureCredentials, etc.) leaves isALTSConn false — otherwise the
// compatibility check would report DirectPath eligibility for plain TLS
// channels.
func TestRecordProbePeer_NonALTS(t *testing.T) {
	bc := &BigtableConn{}
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.1")},
		// AuthInfo left nil — models an insecure or non-ALTS handshake.
	}
	recordProbePeer(bc, p)

	if bc.isALTSConn.Load() {
		t.Error("isALTSConn = true for non-ALTS AuthInfo, want false")
	}
}

// TestRecordProbePeer_NilInputSafe ensures a nil *peer.Peer is a no-op —
// callers may pass nil when the RPC failed before the peer info was
// populated. Verifies both side-effect fields stay at their pre-call
// values.
func TestRecordProbePeer_NilInputSafe(t *testing.T) {
	bc := &BigtableConn{}
	bc.remoteAddrType.Store(int32(unknown))
	recordProbePeer(bc, nil)

	if bc.isALTSConn.Load() {
		t.Error("isALTSConn = true after nil peer, want false")
	}
	if got := bc.remoteAddrType.Load(); got != int32(unknown) {
		t.Errorf("remoteAddrType = %d, want %d (unknown)", got, int32(unknown))
	}
}
