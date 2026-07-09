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

// multiPlexingLimit caps the number of vRPCs in flight on a single session
// stream. The current protocol does not support multiplexing, so the value
// must stay at 1; raising it requires a negotiated server-side change.
const multiPlexingLimit = 1

// Default heartbeat tunables. The interval is replaced once the server sends a
// SessionParametersResponse; the initial deadline is generous enough to span
// the first handshake.
const (
	defaultHeartbeatInterval = 10 * time.Second
	initialHeartbeatGrace    = 30 * time.Minute
)

// Sentinel session-level error codes. They are wrapped in a gRPC Unavailable
// status (so existing retry plumbing sees codes.Unavailable) while errors.Is
// lets callers distinguish the underlying cause.
var (
	// ErrSessionNotActive is returned by Invoke pre-Send when the session
	// is not yet active or has begun shutting down. Errors carrying this
	// cause are delivered tagged StateUncommitted (never left the client)
	// so RetryingVRpc retries them regardless of Idempotent.
	ErrSessionNotActive = errors.New("bigtable: session not active")
	// ErrUnavailableHeartBeatMissed indicates the session was torn down because
	// the server stopped sending heartbeats within the negotiated window.
	// Delivered via cancelActiveRPCs and tagged StateTransportFailure.
	ErrUnavailableHeartBeatMissed = errors.New("bigtable: session unavailable: server heartbeat missed")
	// ErrUnavailableGoAway indicates the server sent GOAWAY. The session
	// transitions to Closing (pool stops handing it out) but any in-flight
	// vRPC keeps running — if the server sends the response before dropping
	// the stream, the RPC completes cleanly (Java parity). Only if the
	// stream actually terminates without a response does the RPC get failed
	// via handleClose → cancelActiveRPCs (tagged StateTransportFailure).
	ErrUnavailableGoAway = errors.New("bigtable: session unavailable: server sent GOAWAY")
	// ErrUnavailableSessionError indicates the server reported a fatal
	// session-level error (an ErrorResponse with rpc_id == 0). Delivered
	// via cancelActiveRPCs and tagged StateTransportFailure.
	ErrUnavailableSessionError = errors.New("bigtable: session unavailable: server reported session error")
)

// Stream is the bidirectional gRPC stream a Session multiplexes over.
type Stream interface {
	Send(*spb.SessionRequest) error
	Recv() (*spb.SessionResponse, error)
	Header() (metadata.MD, error)
	// Context is the stream's context. After Header() returns, peer info
	// (remote TCP addr) is available via peer.FromContext — sessionz uses
	// it to link a slow vRPC row to the specific conn in tcpz.
	Context() context.Context
}

// SessionHooks contains optional callbacks invoked at points in a session's
// lifecycle. Any field may be nil; the session calls only the non-nil hooks.
// Hooks must not block; dispatch long work to a goroutine. This follows the
// net/http/httptrace.ClientTrace pattern.
//
// Lifecycle ordering guarantees:
//
//	OnStart   → fires once when Session.Start is invoked.
//	OnActive  → fires once at Starting → Ready transition (after PeerInfo
//	             is populated).
//	OnClosing → fires once when the session is FIRST known to be dying —
//	             on the first successful transition out of Ready (any
//	             non-Ready terminal-bound state). Java parity:
//	             SessionImpl.onSessionClosing. Consumers use it to
//	             remove the session from pool routing structures BEFORE
//	             the actual close (potentially up to
//	             waitServerCloseGrace seconds) completes.
//	OnClose   → fires once at the end of teardown, after the stream has
//	             actually closed. Always fires AFTER OnClosing (the
//	             session guarantees the ordering via closingOnce +
//	             closeOnce).
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

// vrpcResult is the single value delivered to Invoke through resultChan.
// Exactly one of resp, errResp, err is set:
//
//	resp    — server success frame; ClusterInfo lives on it.
//	errResp — server error frame; ClusterInfo lives on it, Status carries
//	          the gRPC code and any RetryInfo.
//	err     — transport-side failure (cancel, close, heartbeat miss);
//	          already tagged StateTransportFailure at the source. No server
//	          frame arrived, so no ClusterInfo.
type vrpcResult struct {
	resp    *spb.VirtualRpcResponse
	errResp *spb.ErrorResponse
	err     error
}

// ClusterInfo returns whichever server frame's ClusterInformation is set,
// or nil if the result carries a transport-side error.
func (r vrpcResult) ClusterInfo() *spb.ClusterInformation {
	if r.resp != nil {
		return r.resp.ClusterInfo
	}
	if r.errResp != nil {
		return r.errResp.ClusterInfo
	}
	return nil
}

// vrpcImpl tracks the state of an in-flight virtual RPC. Populated once
// by Session.Invoke at construction, never mutated after — activeRPC.CAS
// is the publication point.
type vrpcImpl struct {
	id         int64
	method     string
	resultChan chan vrpcResult
}

// Session manages the lifecycle of a Bigtable Session and routes vRPCs
// over its bidirectional Stream.
//
// All fields formerly guarded by an internal mu are now atomics — the hot
// path (Invoke, State(), the picker) no longer takes a per-session mutex,
// removing four lock/unlock pairs and the cross-goroutine cache-line
// ping-pong the picker used to trigger by calling State() per session
// under lock. sendMu still guards concurrent Send() writers since
// grpc.ClientStream.Send is not safe for concurrent use.
//
// Observability state (counters, event ring, latency histogram, tracer,
// logger, metric-attribution plumbing) lives in the embedded sessionDebug
// — see session_debug.go. Embedded so bare field access (s.okRpcs,
// s.recordEvent, s.tracer, …) still resolves.
type Session struct {
	// nextRPCID is mutated exclusively via atomic ops.
	nextRPCID atomic.Int64

	sendMu sync.Mutex

	logName     string
	stream      Stream
	hooks       SessionHooks
	sessionType SessionType

	// state is the session's lifecycle position (State constants). Read
	// with State(); mutate through transitionTo.
	state atomic.Int32

	// closingOnce serializes hooks.OnClosing so it fires exactly once
	// across the four transition sites that can drive a session out of
	// Ready (Close, ForceClose, handleGoAway, handleClose). Java parity
	// with onSessionClosing.
	closingOnce sync.Once
	// closeOnce serializes hooks.OnClose and tracer.recordClose so they
	// fire exactly once even if multiple paths race to close the session.
	closeOnce sync.Once

	// activeRPC holds the single in-flight vRPC (multiPlexingLimit=1).
	// Invoke sets it via CompareAndSwap(nil, rpc) — the CAS is the
	// pool-serialization invariant made explicit: if it fails, someone
	// bypassed the pool's per-session checkout gate.
	activeRPC atomic.Pointer[vrpcImpl]

	// poolHandle is the back-ref to this session's SessionHandle, stored
	// once by SessionPoolImpl.OnActive right after the handle is minted.
	// OnClose reads it to hand the handle to sl.OnSessionClosed without a
	// per-pool bookkeeping map. Nil for sessions never promoted from
	// starting → active or for tests that bypass the pool.
	poolHandle atomic.Pointer[SessionHandle]

	// heartbeatIntervalNano is the server-negotiated keep-alive interval
	// (ns). handleSessionParameters mutates it from readLoop while the
	// hot path reads it via resetHeartbeatDeadline.
	heartbeatIntervalNano atomic.Int64
	// nextHeartbeatDeadlineNano is the wall-clock deadline (UnixNano)
	// the heartbeat watchdog compares against. Every outbound frame + every
	// inbound frame extends it via resetHeartbeatDeadline.
	nextHeartbeatDeadlineNano atomic.Int64

	// quiescent is closed when the in-flight vRPC (if any) has drained
	// after the session entered StateClosing, or when ForceClose runs.
	quiescent     chan struct{}
	quiescentOnce sync.Once

	// peerInfo is populated by peerInfoExtracter from the stream header,
	// synchronously in handleOpenSession before hooks.onActive fires.
	// atomic.Pointer so the picker / AfeID / snapshot can read it lock-free.
	peerInfo atomic.Pointer[spb.PeerInfo]

	// refreshConfig is stored once when the server sends
	// SessionRefreshConfig. atomic.Pointer keeps the getter allocation-
	// and lock-free.
	refreshConfig atomic.Pointer[spb.SessionRefreshConfig]

	// sessionDebug carries the observability state. See session_debug.go.
	sessionDebug
}

// SessionOption configures a Session at construction time.
type SessionOption func(*Session)

// NewSession constructs a Session bound to the given stream. Pass a zero-value
// SessionHooks if you don't need lifecycle callbacks.
func NewSession(logName string, stream Stream, hooks SessionHooks, sessionType SessionType, opts ...SessionOption) *Session {
	s := &Session{
		logName:     logName,
		stream:      stream,
		hooks:       hooks,
		quiescent:   make(chan struct{}),
		sessionType: sessionType,
	}
	s.sessionDebug.init(sessionType)
	s.state.Store(int32(StateNew))
	s.heartbeatIntervalNano.Store(int64(defaultHeartbeatInterval))
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(initialHeartbeatGrace).UnixNano())
	for _, o := range opts {
		o(s)
	}
	return s
}

// LogName returns the session's diagnostic identifier. Set once at
// construction; safe to read lock-free.
func (s *Session) LogName() string {
	return s.logName
}

// State returns the current state.
func (s *Session) State() State {
	return State(s.state.Load())
}

// PeerInfo returns the peer info, or nil if it has not been parsed yet.
func (s *Session) PeerInfo() *spb.PeerInfo {
	return s.peerInfo.Load()
}

// afeID identifies the AFE (Application Front End) a session is pinned to,
// derived from PeerInfo.ApplicationFrontendId. The zero value is the
// sentinel for "unknown" — used before PeerInfo is populated or when the
// server did not send the bigtable-peer-info header. Java-parity: mirrors
// the AutoValue AfeId in SessionList.java, which wraps the same signed
// 64-bit long.
type afeID int64

// AfeID returns the AFE identifier for this session, or 0 if PeerInfo is
// nil (header absent or session pre-Active). Stable for the session's
// lifetime — PeerInfo is populated once, synchronously with the transition
// to StateReady (see handleOpenSession).
func (s *Session) AfeID() afeID {
	if p := s.peerInfo.Load(); p != nil {
		return afeID(p.GetApplicationFrontendId())
	}
	return 0
}

// RefreshConfig returns the server-provided refresh configuration, or nil
// if the server has not sent one.
func (s *Session) RefreshConfig() *spb.SessionRefreshConfig {
	return s.refreshConfig.Load()
}

// transitionTo sets the session state to `to` iff ok(currentState) returns
// true. Returns the previous state and whether the transition was applied.
// Retries on CAS failure so a losing racer with a still-valid current state
// still transitions; the predicate is re-evaluated after each spurious loss.
func (s *Session) transitionTo(to State, ok func(State) bool) (prev State, applied bool) {
	for {
		prev = State(s.state.Load())
		if !ok(prev) {
			return prev, false
		}
		if s.state.CompareAndSwap(int32(prev), int32(to)) {
			s.lastStateChangeNano.Store(time.Now().UnixNano())
			return prev, true
		}
	}
}

// isState returns a predicate matching any of `allowed`.
func isState(allowed ...State) func(State) bool {
	return func(s State) bool {
		for _, a := range allowed {
			if s == a {
				return true
			}
		}
		return false
	}
}

// notState returns a predicate matching any state NOT in `forbidden`.
func notState(forbidden ...State) func(State) bool {
	return func(s State) bool {
		for _, f := range forbidden {
			if s == f {
				return false
			}
		}
		return true
	}
}

// signalQuiescent closes the quiescent channel exactly once.
func (s *Session) signalQuiescent() {
	s.quiescentOnce.Do(func() { close(s.quiescent) })
}

// sessionErr couples a gRPC Unavailable status with a sentinel cause so that
// both status.Code(err) and errors.Is(err, sentinel) work.
type sessionErr struct {
	st    *status.Status
	cause error
}

func (e *sessionErr) Error() string              { return e.st.Err().Error() }
func (e *sessionErr) Unwrap() error              { return e.cause }
func (e *sessionErr) GRPCStatus() *status.Status { return e.st }

// unavailable builds a sessionErr from a sentinel cause and human-readable
// detail. The returned error carries codes.Unavailable.
func unavailable(cause error, format string, args ...interface{}) error {
	return &sessionErr{
		st:    status.Newf(codes.Unavailable, format, args...),
		cause: cause,
	}
}
