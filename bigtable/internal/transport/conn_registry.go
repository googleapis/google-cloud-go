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
	"sync"
	"time"
)

// graveyardCap bounds how many recently-dead conn records the registry
// keeps for the tcpz age-distribution histogram. Sized so a chatty client
// that recycles conns aggressively still has a representative sample.
const graveyardCap = 512

// ConnRegistry tracks every *net.TCPConn returned by a custom gRPC dialer
// so tcpz can render live TCP_INFO for each. The registry never wraps the
// returned conn — gRPC receives the raw *net.TCPConn unchanged, so nothing
// in the RPC hot path traverses ConnRegistry code. Registry state is
// touched only on dial (rare) and Snapshot (only when someone renders
// tcpz). Dead entries are pruned lazily during Snapshot when getsockopt
// reports the fd is gone; their (dial, death) times survive in a bounded
// ring so lifetime distributions include departed conns.
type ConnRegistry struct {
	mu    sync.RWMutex
	seq   uint64 // monotonic id so keys are unique across identical addr pairs
	conns map[uint64]*trackedConn

	// graveyard is a ring of the most-recent graveyardCap deaths. graveIdx
	// points at the next slot to write; graveFull says whether we've wrapped.
	// Kept under the same mu as conns — writes only happen inside Snapshot,
	// which already re-acquires the write lock for pruning.
	graveyard []DeadConnInfo
	graveIdx  int
	graveFull bool
}

// trackedConn holds one dial's outputs plus a strong ref to the conn so we
// can call SyscallConn on it during Snapshot. Strong-ref-with-lazy-prune
// (vs weak refs or finalizers) trades a small window of stale entries for
// zero interference with gRPC's lifecycle expectations.
type trackedConn struct {
	remoteAddr string
	localAddr  string
	dialedAt   time.Time
	conn       *net.TCPConn
}

// NewConnRegistry constructs an empty registry.
func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[uint64]*trackedConn)}
}

// Dial is the entry point wired into grpc.WithContextDialer. Delegates to
// net.Dialer.DialContext ("tcp" over addr), and on success records the
// *net.TCPConn in the registry before returning the raw conn to gRPC —
// no wrapping, so gRPC's type assertions (*net.TCPConn, SyscallConn, TLS
// deadline handling) all see exactly what they would without tcpz.
func (r *ConnRegistry) Dial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		r.add(tc)
	}
	return conn, nil
}

func (r *ConnRegistry) add(tc *net.TCPConn) {
	r.mu.Lock()
	r.seq++
	r.conns[r.seq] = &trackedConn{
		remoteAddr: tc.RemoteAddr().String(),
		localAddr:  tc.LocalAddr().String(),
		dialedAt:   time.Now(),
		conn:       tc,
	}
	r.mu.Unlock()
}

// TCPInfoSnapshot is the platform-agnostic view of one registered conn as
// of Snapshot time. On Linux the numeric fields come from struct tcp_info
// via getsockopt(TCP_INFO); on other platforms Err is populated and the
// numeric fields are zero. RemoteAddr / LocalAddr / DialedAt are captured
// at dial time and are always present.
//
// Fields are grouped by what question they answer. "Why is TotalRetrans
// high?" is answered by the Congestion + Loss-classification blocks:
// CAState/Backoff say what the kernel thinks the loss regime is; DSACK,
// Reordering, and DeliveredCE distinguish real drops from spurious /
// out-of-order / ECN-signaled events; BytesRetrans + BytesSent give the
// actual retrans ratio.
type TCPInfoSnapshot struct {
	// Identity — captured at dial time.
	RemoteAddr string
	LocalAddr  string
	DialedAt   time.Time

	// TCP + congestion state.
	State   string // TCP FSM state (ESTABLISHED, CLOSE_WAIT, …)
	CAState string // congestion-control state (Open/Disorder/CWR/Recovery/Loss)
	Backoff uint32 // RTO exponential-backoff count; >0 = we've timed out and are waiting longer

	// Round-trip time.
	RTT    time.Duration
	RTTVar time.Duration
	MinRTT time.Duration

	// Window + segment sizing.
	MSS         uint32 // send MSS
	PMTU        uint32 // path MTU (bytes). <1500 = tunneling/VPN in path; silent throughput killer if a middlebox black-holes PMTUD ICMP.
	SndCwnd     uint32 // send congestion window (MSS units)
	SndSsthresh uint32 // slow-start threshold; drop = we've reduced cwnd from loss
	SndWnd      uint32 // current send window (bytes)
	RcvWnd      uint32 // current receive window (bytes)

	// Loss + retransmit counters.
	Retransmits  uint32 // current burst of retransmits
	Retrans      uint32 // currently-outstanding retransmits
	TotalRetrans uint32 // cumulative retransmits over conn lifetime
	Lost         uint32 // segments the kernel considers lost right now
	Sacked       uint32 // segments selectively-ACK'd
	Unacked      uint32 // segments in flight
	Reordering   uint32 // reordering-tolerance estimate (bigger = kernel is being patient)
	ReordSeen    uint32 // times reordering has been observed
	DsackDups    uint32 // duplicate SACKs — spurious retransmits (receiver actually got the byte)
	DeliveredCE  uint32 // packets delivered with ECN Congestion-Experienced marks
	RcvOooPack   uint32 // packets received out-of-order

	// RTO / probe timing.
	RTO                time.Duration // current retransmit timeout
	ATO                time.Duration // delayed-ACK timeout
	Probes             uint32        // zero-window probes attempted
	TotalRTO           uint32        // cumulative RTOs (Linux 4.20+)
	TotalRTORecoveries uint32        // cumulative RTO-driven recovery events
	TotalRTOTime       time.Duration // cumulative time spent in RTO

	// Volume / rate.
	SegsOut       uint32
	SegsIn        uint32
	DataSegsOut   uint32
	DataSegsIn    uint32
	BytesSent     uint64        // total data bytes sent
	BytesAcked    uint64        // total data bytes acknowledged
	BytesRetrans  uint64        // total data bytes retransmitted (retrans ratio = /BytesSent)
	BytesReceived uint64        // total data bytes received
	Delivered     uint32        // total packets delivered
	DeliveryRate  uint64        // recent delivery rate, bytes/sec (BBR estimate)
	PacingRate    uint64        // target pacing rate, bytes/sec
	NotsentBytes  uint32        // bytes buffered but not yet on the wire — app-limited signal
	BusyTime      time.Duration // cumulative time socket was busy sending
	RwndLimited   time.Duration // cumulative time limited by receiver window
	SndbufLimited time.Duration // cumulative time limited by send buffer
	Rehash        uint32        // times the flow was rehashed (indicates path changes)

	// LastDataRecv / LastDataSent express "how long since the socket last
	// carried data" — helps distinguish "idle since forever" from "just
	// hung." Both derived from tcp_info's last_data_{recv,sent}.
	LastDataRecv time.Duration
	LastDataSent time.Duration

	// RetransRatioPct = BytesRetrans / BytesSent * 100, precomputed for
	// the common "% of bytes retransmitted" view. Zero when BytesSent is
	// zero (no data has flowed yet).
	RetransRatioPct float64

	// Err is set when this platform can't read TCP_INFO or the read
	// failed on a live fd. Dead fds (EBADF/ENOTCONN) are pruned rather
	// than surfaced, so a populated Err always means "conn exists but
	// info wasn't readable" — e.g. non-Linux OS.
	Err string
}

// DeadConnInfo is what the registry remembers about a pruned conn: the
// endpoints plus dial and death times. Lifetime = DiedAt.Sub(DialedAt) is
// the value tcpz plots for the "how long did conns live?" histogram.
// DiedAt is when Snapshot noticed the death (the getsockopt returned
// EBADF/ENOTCONN/ErrClosed) — approximate but within one Snapshot cadence
// of actual close.
type DeadConnInfo struct {
	RemoteAddr string
	LocalAddr  string
	DialedAt   time.Time
	DiedAt     time.Time
}

// Snapshot reads TCP_INFO for every registered conn and returns the
// results, oldest dial first. Dead entries (readTCPInfo returned an
// isDeadConn error) are removed from the registry before returning so a
// gRPC-closed conn doesn't linger indefinitely; their (dial, death) pair
// is copied into the graveyard ring for post-mortem age analysis. All
// syscalls happen outside the registry lock so a slow syscall can't block
// dials or other snapshots.
func (r *ConnRegistry) Snapshot() []TCPInfoSnapshot {
	r.mu.RLock()
	keys := make([]uint64, 0, len(r.conns))
	byKey := make(map[uint64]*trackedConn, len(r.conns))
	for k, tc := range r.conns {
		keys = append(keys, k)
		byKey[k] = tc
	}
	r.mu.RUnlock()

	sortKeysAscending(keys)

	out := make([]TCPInfoSnapshot, 0, len(keys))
	var dead []uint64
	for _, k := range keys {
		tc := byKey[k]
		snap, err := readTCPInfo(tc.conn)
		if err != nil {
			if isDeadConn(err) {
				dead = append(dead, k)
				continue
			}
			snap = TCPInfoSnapshot{Err: err.Error()}
		}
		snap.RemoteAddr = tc.remoteAddr
		snap.LocalAddr = tc.localAddr
		snap.DialedAt = tc.dialedAt
		out = append(out, snap)
	}
	if len(dead) > 0 {
		now := time.Now()
		r.mu.Lock()
		for _, k := range dead {
			tc, ok := r.conns[k]
			if !ok {
				continue
			}
			r.recordDeathLocked(DeadConnInfo{
				RemoteAddr: tc.remoteAddr,
				LocalAddr:  tc.localAddr,
				DialedAt:   tc.dialedAt,
				DiedAt:     now,
			})
			delete(r.conns, k)
		}
		r.mu.Unlock()
	}
	return out
}

// recordDeathLocked appends to the graveyard ring. Caller MUST hold
// r.mu.Lock().
func (r *ConnRegistry) recordDeathLocked(d DeadConnInfo) {
	if r.graveyard == nil {
		r.graveyard = make([]DeadConnInfo, graveyardCap)
	}
	r.graveyard[r.graveIdx] = d
	r.graveIdx++
	if r.graveIdx >= graveyardCap {
		r.graveIdx = 0
		r.graveFull = true
	}
}

// DeadConns returns a copy of the graveyard, oldest death first. Bounded
// at graveyardCap; older deaths are silently dropped as new ones arrive.
// Empty slice when nothing has died yet.
func (r *ConnRegistry) DeadConns() []DeadConnInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.graveyard == nil {
		return nil
	}
	var n int
	if r.graveFull {
		n = graveyardCap
	} else {
		n = r.graveIdx
	}
	out := make([]DeadConnInfo, 0, n)
	if r.graveFull {
		out = append(out, r.graveyard[r.graveIdx:]...)
		out = append(out, r.graveyard[:r.graveIdx]...)
	} else {
		out = append(out, r.graveyard[:r.graveIdx]...)
	}
	return out
}

// Len reports the registered conn count (including any not-yet-pruned
// dead entries). Cheap; useful for a "conns=N" summary row.
func (r *ConnRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.conns)
}

// sortKeysAscending is a tiny helper — uint64 sort without pulling
// sort.Slice's reflection overhead into a debug path that runs frequently.
func sortKeysAscending(ks []uint64) {
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j-1] > ks[j]; j-- {
			ks[j-1], ks[j] = ks[j], ks[j-1]
		}
	}
}

// unsupportedTCPInfoErr is the error tcp_info_*.go implementations return
// when the platform can't expose TCP_INFO. Exposed as a constant so tests
// can assert on it without importing platform-specific code.
type unsupportedTCPInfoErr struct{}

func (unsupportedTCPInfoErr) Error() string { return "tcp_info not supported on this platform" }

// ErrTCPInfoUnsupported is the sentinel returned by readTCPInfo on non-Linux.
var ErrTCPInfoUnsupported = unsupportedTCPInfoErr{}

// annotateReadErr is a small helper used by platform-specific readers when
// they want to prefix a syscall error with context.
func annotateReadErr(op string, err error) error {
	return fmt.Errorf("%s: %w", op, err)
}
