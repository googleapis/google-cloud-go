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
	"sync"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/metadata"
)

const sessionConcurrencyLimit = 1 // Limit of 1 indicates no multiplexing.

// State represents the state of a Session.
type State int

const (
	// StateNew indicates the session is newly created and not yet active.
	StateNew State = iota
	// StateStarting indicates the session is dialing and handshaking.
	StateStarting
	// StateActive indicates the session is active and ready for RPCs.
	StateActive
	// StateClosing indicates the session is in the process of closing.
	StateClosing
	// StateWaitServerClose indicates the session has requested close and is waiting for server EOF.
	StateWaitServerClose
	// StateClosed indicates the session is closed.
	StateClosed
)

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
	case StateWaitServerClose:
		return "WaitServerClose"
	case StateClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// Stream represents a bidirectional stream for Session requests and responses.
type Stream interface {
	Send(*spb.SessionRequest) error
	Recv() (*spb.SessionResponse, error)
	Header() (metadata.MD, error)
}

// Listener receives notifications of Session lifecycle events.
type Listener interface {
	OnStart(ctx context.Context)
	OnActive(s *Session)
	OnClose(s *Session, err error)
}

type vrpcResult struct {
	resp        *spb.VirtualRpcResponse
	clusterInfo *spb.ClusterInformation
	err         error
}

// VRPCImpl represents an active virtual RPC.
type VRPCImpl struct {
	id         int64
	method     string
	resultChan chan vrpcResult
}

// Session manages the lifecycle of a Bigtable Session.
type Session struct {
	mu                    sync.Mutex
	sendMu                sync.Mutex
	vrpcSem               *semaphore.Weighted // Serializes vRPC execution to ensure only one runs at a time per session.
	state                 State
	lastStateChange       time.Time
	logName               string
	stream                Stream
	listener              Listener
	activeRPCs            map[int64]*VRPCImpl
	nextRPCID             int64
	heartbeatInterval     time.Duration
	nextHeartbeatDeadline time.Time
	peerInfo              *spb.PeerInfo
	hasOkRpcs             bool
	hasErrorRpcs          bool

	handshakeDone chan struct{}
	handshakeErr  error
	tracer        *sessionTracer
	sessionType   SessionType
}

// NewSession creates a new Session constructor.
func NewSession(logName string, stream Stream, listener Listener, sessionType SessionType) *Session {
	return &Session{
		state:                 StateNew,
		lastStateChange:       time.Now(),
		logName:               logName,
		stream:                stream,
		listener:              listener,
		activeRPCs:            make(map[int64]*VRPCImpl),
		heartbeatInterval:     10 * time.Second,
		nextHeartbeatDeadline: time.Now().Add(30 * time.Minute),
		handshakeDone:         make(chan struct{}),
		tracer:                newSessionTracer(sessionType),
		sessionType:           sessionType,
		vrpcSem:               semaphore.NewWeighted(sessionConcurrencyLimit),
	}
}

// LogName returns the session log identifier.
func (s *Session) LogName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logName
}

// State returns the current operational State of the Session.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// StateString returns the current session state string.
func (s *Session) StateString() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.String()
}

// PeerInfo returns the  peer information of the session connection.
func (s *Session) PeerInfo() *spb.PeerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peerInfo
}

// HasOkRpcs returns true if the session processed successful RPCs.
func (s *Session) HasOkRpcs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasOkRpcs
}

// HasErrorRpcs returns true if the session processed failed RPCs.
func (s *Session) HasErrorRpcs() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasErrorRpcs
}
