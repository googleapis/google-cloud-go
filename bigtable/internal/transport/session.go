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
	"errors"
	"sync"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Raising multiPlexingLimit requires a negotiated server-side change.
const multiPlexingLimit = 1

const (
	// defaultHeartbeatInterval is the fallback cadence when no server-provided
	// SessionParametersResponse has landed yet. Once negotiated, this is
	// overwritten by the server-supplied interval.
	defaultHeartbeatInterval = 100 * time.Millisecond
	// initialHeartbeatGrace is the deadline used from OpenSession until
	// SessionParametersResponse arrives with the real cadence; kept in
	// lock-step with defaultHeartbeatInterval so a session that never
	// receives SessionParameters trips within one interval.
	initialHeartbeatGrace = 100 * time.Millisecond
)

// Session-level errors, wrapped in codes.Unavailable so retry plumbing works
// via status.Code while errors.Is distinguishes the cause.
var (
	ErrSessionNotActive           = errors.New("bigtable: session not active")
	ErrUnavailableHeartBeatMissed = errors.New("bigtable: session unavailable: server heartbeat missed")
	ErrUnavailableGoAway          = errors.New("bigtable: session unavailable: server sent GOAWAY")
	ErrUnavailableSessionError    = errors.New("bigtable: session unavailable: server reported session error")
)

// Stream is the bidirectional gRPC stream a Session multiplexes over.
type Stream interface {
	Send(*spb.SessionRequest) error
	Recv() (*spb.SessionResponse, error)
	Header() (metadata.MD, error)
	Context() context.Context
}

// SessionHooks holds optional lifecycle callbacks. Nil fields are skipped.
// Hooks must not block.
type SessionHooks struct {
	OnStart   func(ctx context.Context)
	OnActive  func(s *Session)
	OnClosing func(s *Session)
	OnClose   func(s *Session, err error)
}

func (h SessionHooks) onStart(ctx context.Context) {
	if h.OnStart != nil {
		h.OnStart(ctx)
	}
}

func (h SessionHooks) onActive(s *Session) {
	if h.OnActive != nil {
		h.OnActive(s)
	}
}

func (h SessionHooks) onClosing(s *Session) {
	if h.OnClosing != nil {
		h.OnClosing(s)
	}
}

func (h SessionHooks) onClose(s *Session, err error) {
	if h.OnClose != nil {
		h.OnClose(s, err)
	}
}

// vrpcResult is the value delivered to Invoke on resultChan. Exactly one of
// resp, errResp, err is set.
type vrpcResult struct {
	resp    *spb.VirtualRpcResponse
	errResp *spb.ErrorResponse
	err     error
}

// ClusterInfo returns whichever server frame's ClusterInformation is set, or
// nil on a transport-side err.
func (r vrpcResult) ClusterInfo() *spb.ClusterInformation {
	if r.resp != nil {
		return r.resp.ClusterInfo
	}
	if r.errResp != nil {
		return r.errResp.ClusterInfo
	}
	return nil
}

// vrpcImpl tracks an in-flight virtual RPC. Publication point is the
// activeRPC assignment under slotMu.
type vrpcImpl struct {
	id         int64
	method     string
	resultChan chan vrpcResult
}

// Session manages the lifecycle of a Bigtable Session and routes vRPCs over
// its bidirectional Stream.
type Session struct {
	nextRPCID atomic.Int64

	// sendMu serializes concurrent Send calls — grpc.ClientStream.Send is
	// not safe for concurrent use.
	sendMu sync.Mutex

	logName     string
	stream      Stream
	hooks       SessionHooks
	sessionType SessionType

	// state is the lifecycle position; read via State(), mutate via
	// transitionTo. lastStateChangeNano is stamped inside transitionTo on
	// each successful swap; it lives on the embedded sessionDebug (below)
	// so the observability field set stays co-located with the rest of the
	// debug/metric plumbing.

	state atomic.Int32

	// closingOnce/closeOnce fire hooks.OnClosing/OnClose exactly once each
	// even when multiple teardown paths race.
	closingOnce sync.Once
	closeOnce   sync.Once

	// slotMu serializes the (activeRPC, currentCancel) pair for the
	// one-in-flight slot. Innermost lock; held only across pointer
	// assignments. Accessors land with Invoke in a follow-up PR.
	slotMu        sync.Mutex
	activeRPC     *vrpcImpl
	currentCancel *vrpcResult

	// heartbeat*Nano: interval is server-negotiated (SessionParameters);
	// deadline is extended by every inbound/outbound frame.
	heartbeatIntervalNano     atomic.Int64
	nextHeartbeatDeadlineNano atomic.Int64

	// quiescent closes when the in-flight vRPC drains after StateClosing,
	// or when ForceClose runs.
	quiescent     chan struct{}
	quiescentOnce sync.Once

	// peerInfo is set once, synchronously in handleOpenSession before
	// hooks.onActive fires — reads stay lock-free.
	peerInfo atomic.Pointer[spb.PeerInfo]
	// refreshConfig is set once when the server sends SessionRefreshConfig.
	refreshConfig atomic.Pointer[spb.SessionRefreshConfig]

	// sessionDebug bundles observability: per-session counters, event
	// ring, latency histogram, tracer/logger, close-reason attribution.
	// Embedded so bare field access (s.tracer, s.okRpcs, s.recordEvent,
	// ...) continues to compile once vRPC / lifecycle land.
	sessionDebug
}

// SessionOption configures a Session at construction time.
type SessionOption func(*Session)

// NewSession constructs a Session bound to stream. Zero-value SessionHooks is
// valid.
func NewSession(logName string, stream Stream, hooks SessionHooks, sessionType SessionType, opts ...SessionOption) *Session {
	s := &Session{
		logName:     logName,
		stream:      stream,
		hooks:       hooks,
		quiescent:   make(chan struct{}),
		sessionType: sessionType,
	}
	s.state.Store(int32(StateNew))
	s.heartbeatIntervalNano.Store(int64(defaultHeartbeatInterval))
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(initialHeartbeatGrace).UnixNano())
	s.sessionDebug.init(sessionType)
	for _, o := range opts {
		o(s)
	}
	return s
}

// LogName returns the diagnostic identifier.
func (s *Session) LogName() string { return s.logName }

// State returns the current state.
func (s *Session) State() State { return State(s.state.Load()) }

// PeerInfo returns the peer info, or nil pre-Ready.
func (s *Session) PeerInfo() *spb.PeerInfo { return s.peerInfo.Load() }

// AfeID returns the AFE identifier, or 0 pre-Ready. Stable for the session's
// lifetime — PeerInfo is populated once at StateReady. AfeID type lives in
// afe_snapshot.go (same package).
func (s *Session) AfeID() AfeID {
	if p := s.peerInfo.Load(); p != nil {
		return AfeID(p.GetApplicationFrontendId())
	}
	return 0
}

// RefreshConfig returns the server-provided refresh configuration, or nil.
func (s *Session) RefreshConfig() *spb.SessionRefreshConfig { return s.refreshConfig.Load() }

// signalQuiescent closes the quiescent channel exactly once.
func (s *Session) signalQuiescent() {
	s.quiescentOnce.Do(func() { close(s.quiescent) })
}

// sessionErr couples a gRPC Unavailable status with a sentinel cause so both
// status.Code and errors.Is work.
type sessionErr struct {
	st    *status.Status
	cause error
}

func (e *sessionErr) Error() string              { return e.st.Err().Error() }
func (e *sessionErr) Unwrap() error              { return e.cause }
func (e *sessionErr) GRPCStatus() *status.Status { return e.st }

// unavailable builds a sessionErr carrying codes.Unavailable.
func unavailable(cause error, format string, args ...interface{}) error {
	return &sessionErr{
		st:    status.Newf(codes.Unavailable, format, args...),
		cause: cause,
	}
}
