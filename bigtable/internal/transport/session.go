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
	"log"
	"sync"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"golang.org/x/sync/semaphore"
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
	// ErrSessionNotActive is returned by Invoke when the session is not yet
	// active or has begun shutting down.
	ErrSessionNotActive = errors.New("bigtable: session not active")
	// ErrUnavailableHeartBeatMissed indicates the session was torn down because
	// the server stopped sending heartbeats within the negotiated window.
	ErrUnavailableHeartBeatMissed = errors.New("bigtable: session unavailable: server heartbeat missed")
	// ErrUnavailableGoAway indicates the server sent a GOAWAY and cancelled
	// vRPCs that had not yet been admitted.
	ErrUnavailableGoAway = errors.New("bigtable: session unavailable: server sent GOAWAY")
	// ErrUnavailableSessionError indicates the server reported a fatal
	// session-level error (an ErrorResponse with rpc_id == 0).
	ErrUnavailableSessionError = errors.New("bigtable: session unavailable: server reported session error")
)

// State represents the lifecycle state of a Session. Sessions move strictly
// forward through the values; once StateClosed is reached the session is
// terminal.
type State int

const (
	// StateNew indicates the session is newly created and not yet active.
	StateNew State = iota
	// StateStarting indicates the session is dialing and handshaking.
	StateStarting
	// StateActive indicates the session is active and ready for RPCs.
	StateActive
	// StateClosing indicates the session is draining and shutting down. It
	// covers both the pre-CloseSession drain and the post-CloseSession wait
	// for the server's EOF.
	StateClosing
	// StateClosed indicates the session is closed.
	StateClosed
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateNew:
		return "New"
	case StateStarting:
		return "Starting"
	case StateActive:
		return "Active"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// Stream is the bidirectional gRPC stream a Session multiplexes over.
type Stream interface {
	Send(*spb.SessionRequest) error
	Recv() (*spb.SessionResponse, error)
	Header() (metadata.MD, error)
}

// SessionHooks contains optional callbacks invoked at points in a session's
// lifecycle. Any field may be nil; the session calls only the non-nil hooks.
// Hooks must not block; dispatch long work to a goroutine. This follows the
// net/http/httptrace.ClientTrace pattern.
type SessionHooks struct {
	OnStart  func(ctx context.Context)
	OnActive func(s *Session)
	OnClose  func(s *Session, err error)
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

func (h SessionHooks) onClose(s *Session, err error) {
	if h.OnClose != nil {
		h.OnClose(s, err)
	}
}

// vrpcResult is the single value delivered to Invoke through resultChan.
type vrpcResult struct {
	resp        *spb.VirtualRpcResponse
	clusterInfo *spb.ClusterInformation
	err         error
}

// vrpcImpl tracks the state of an in-flight virtual RPC.
type vrpcImpl struct {
	id         int64
	method     string
	resultChan chan vrpcResult
}

// Session manages the lifecycle of a Bigtable Session and routes vRPCs over
// its bidirectional Stream.
type Session struct {
	// nextRPCID is mutated exclusively via atomic ops; using atomic.Int64
	// guarantees 8-byte alignment on 32-bit platforms without struct layout
	// constraints.
	nextRPCID atomic.Int64

	mu     sync.Mutex
	sendMu sync.Mutex

	logger      *log.Logger
	logName     string
	stream      Stream
	hooks       SessionHooks
	sessionType SessionType
	// TODO: add sessionTracer field for session-scoped OTel metrics
	// (open/close/duration/uptime, per-operation latency). Lands in a
	// follow-up PR; lifecycle and vRPC sites will then re-add the
	// recordOpen/recordClose/recordOperation/setPeerInfo callbacks.
	vrpcSem *semaphore.Weighted

	state           State
	lastStateChange time.Time
	// closeOnce serializes hooks.OnClose so it fires exactly once even if
	// multiple paths race to close the session.
	closeOnce sync.Once

	activeRPCs map[int64]*vrpcImpl

	heartbeatInterval     time.Duration
	nextHeartbeatDeadline time.Time

	// quiescent is closed when no vRPCs remain in activeRPCs after the
	// session enters StateClosing, or when ForceClose runs. Close() waits on
	// it to drain in-flight RPCs without polling.
	quiescent     chan struct{}
	quiescentOnce sync.Once

	peerInfo      *spb.PeerInfo
	refreshConfig *spb.SessionRefreshConfig

	hasOkRpcs    bool
	hasErrorRpcs bool
}

// SessionOption configures a Session at construction time.
type SessionOption func(*Session)

// WithSessionLogger attaches a logger for diagnostic output. Without it, the
// session logs nothing.
func WithSessionLogger(logger *log.Logger) SessionOption {
	return func(s *Session) { s.logger = logger }
}

// NewSession constructs a Session bound to the given stream. Pass a zero-value
// SessionHooks if you don't need lifecycle callbacks.
func NewSession(logName string, stream Stream, hooks SessionHooks, sessionType SessionType, opts ...SessionOption) *Session {
	s := &Session{
		state:                 StateNew,
		lastStateChange:       time.Now(),
		logName:               logName,
		stream:                stream,
		hooks:                 hooks,
		activeRPCs:            make(map[int64]*vrpcImpl),
		heartbeatInterval:     defaultHeartbeatInterval,
		nextHeartbeatDeadline: time.Now().Add(initialHeartbeatGrace),
		quiescent:             make(chan struct{}),
		sessionType:           sessionType,
		vrpcSem:               semaphore.NewWeighted(multiPlexingLimit),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// LogName returns the session's diagnostic identifier.
func (s *Session) LogName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logName
}

// State returns the current state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// PeerInfo returns the peer info, or nil if it has not been parsed yet.
func (s *Session) PeerInfo() *spb.PeerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peerInfo
}

// RefreshConfig returns the server-provided refresh configuration, or nil if
// the server has not sent one.
func (s *Session) RefreshConfig() *spb.SessionRefreshConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshConfig
}

// HasOkRpcs reports whether the session served at least one successful vRPC.
func (s *Session) HasOkRpcs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasOkRpcs
}

// HasErrorRpcs reports whether the session served at least one failed vRPC.
func (s *Session) HasErrorRpcs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasErrorRpcs
}

// debugf logs at debug level if a logger has been attached.
func (s *Session) debugf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	btopt.Debugf(s.logger, "bigtable_session %s: "+format, append([]interface{}{s.logName}, args...)...)
}

// transitionTo sets the session state to `to` iff ok(currentState) returns
// true. Returns the previous state and whether the transition was applied.
// All callers using this helper share the same lock/timestamp/transition
// pattern, so adding/removing transitions does not duplicate that bookkeeping.
func (s *Session) transitionTo(to State, ok func(State) bool) (prev State, applied bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev = s.state
	if !ok(prev) {
		return prev, false
	}
	s.state = to
	s.lastStateChange = time.Now()
	return prev, true
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
