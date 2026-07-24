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
	"io"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// stubStream answers Header/Context but never sends or receives.
type stubStream struct {
	ctx context.Context
}

func newStubStream() *stubStream { return &stubStream{ctx: context.Background()} }

func (s *stubStream) Send(*spb.SessionRequest) error { return io.EOF }
func (s *stubStream) Recv() (*spb.SessionResponse, error) {
	return nil, io.EOF
}
func (s *stubStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (s *stubStream) Context() context.Context     { return s.ctx }

func newTestSession(t *testing.T, opts ...SessionOption) *Session {
	t.Helper()
	return NewSession("test-session", newStubStream(), SessionHooks{}, SessionTypeTable, opts...)
}

func TestNewSession_Defaults(t *testing.T) {
	s := newTestSession(t)

	if got := s.State(); got != StateNew {
		t.Errorf("initial State: got %s, want StateNew", got)
	}
	if s.LogName() != "test-session" {
		t.Errorf("LogName: got %q, want %q", s.LogName(), "test-session")
	}
	if s.sessionType != SessionTypeTable {
		t.Errorf("sessionType: got %v, want SessionTypeTable", s.sessionType)
	}
	if s.PeerInfo() != nil {
		t.Errorf("PeerInfo default: got %v, want nil", s.PeerInfo())
	}
	if s.RefreshConfig() != nil {
		t.Errorf("RefreshConfig default: got %v, want nil", s.RefreshConfig())
	}
	select {
	case <-s.quiescent:
		t.Errorf("quiescent chan closed at construction — should stay open until signalQuiescent")
	default:
	}
	if got := s.heartbeatIntervalNano.Load(); got != int64(defaultHeartbeatInterval) {
		t.Errorf("heartbeatIntervalNano: got %d, want %d", got, int64(defaultHeartbeatInterval))
	}
	if got := s.lastStateChangeNano.Load(); got == 0 {
		t.Errorf("lastStateChangeNano: expected non-zero stamp from NewSession")
	}
}

func TestSession_SignalQuiescent_ClosesChannel(t *testing.T) {
	s := newTestSession(t)
	s.signalQuiescent()
	select {
	case <-s.quiescent:
	case <-time.After(100 * time.Millisecond):
		t.Errorf("quiescent chan not closed after signalQuiescent")
	}
}

// TestSession_SignalQuiescent_IdempotentUnderRace guards the sync.Once — a
// racy close(ch) would panic.
func TestSession_SignalQuiescent_IdempotentUnderRace(t *testing.T) {
	s := newTestSession(t)
	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			s.signalQuiescent()
		}()
	}
	wg.Wait()
	s.signalQuiescent()
	select {
	case <-s.quiescent:
	default:
		t.Errorf("expected quiescent channel to be closed")
	}
}

func TestSession_AfeID_ZeroBeforePeerInfo(t *testing.T) {
	s := newTestSession(t)
	if got := s.AfeID(); got != 0 {
		t.Errorf("AfeID before PeerInfo: got %d, want 0", got)
	}
}

func TestSession_AfeID_FromPeerInfo(t *testing.T) {
	s := newTestSession(t)
	s.peerInfo.Store(&spb.PeerInfo{ApplicationFrontendId: 0xABCDEF})
	if got := s.AfeID(); got != AfeID(0xABCDEF) {
		t.Errorf("AfeID: got %#x, want %#x", int64(got), 0xABCDEF)
	}
}

func TestSession_RefreshConfig_StoreLoad(t *testing.T) {
	s := newTestSession(t)
	rc := &spb.SessionRefreshConfig{}
	s.refreshConfig.Store(rc)
	if got := s.RefreshConfig(); got != rc {
		t.Errorf("RefreshConfig: got %p, want %p", got, rc)
	}
}

func TestVrpcResult_ClusterInfo(t *testing.T) {
	ci := &spb.ClusterInformation{ClusterId: "c1"}
	cases := []struct {
		name string
		r    vrpcResult
		want *spb.ClusterInformation
	}{
		{"from resp", vrpcResult{resp: &spb.VirtualRpcResponse{ClusterInfo: ci}}, ci},
		{"from errResp", vrpcResult{errResp: &spb.ErrorResponse{ClusterInfo: ci}}, ci},
		{"transport err — nil", vrpcResult{err: errors.New("boom")}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.ClusterInfo(); got != tc.want {
				t.Errorf("ClusterInfo(): got %p, want %p", got, tc.want)
			}
		})
	}
}

func TestUnavailable_WrapsSentinel(t *testing.T) {
	err := unavailable(ErrUnavailableGoAway, "server sent GOAWAY on session %s", "s-1")
	if err == nil {
		t.Fatalf("unavailable returned nil")
	}
	if code := status.Code(err); code != codes.Unavailable {
		t.Errorf("gRPC code: got %s, want Unavailable", code)
	}
	if !errors.Is(err, ErrUnavailableGoAway) {
		t.Errorf("errors.Is(err, ErrUnavailableGoAway) = false, want true")
	}
	if got := err.Error(); !contains(got, "server sent GOAWAY on session s-1") {
		t.Errorf("Error() message missing formatted detail: got %q", got)
	}
	if unwrapped := errors.Unwrap(err); unwrapped != ErrUnavailableGoAway {
		t.Errorf("Unwrap: got %v, want %v", unwrapped, ErrUnavailableGoAway)
	}
}

func TestSessionHooks_NilSafe(t *testing.T) {
	var h SessionHooks
	h.onStart(context.Background())
	h.onActive(nil)
	h.onClosing(nil)
	h.onClose(nil, nil)
}

func TestSessionHooks_FireOncePerCall(t *testing.T) {
	var started, active, closing, closed int
	h := SessionHooks{
		OnStart:   func(context.Context) { started++ },
		OnActive:  func(*Session) { active++ },
		OnClosing: func(*Session) { closing++ },
		OnClose:   func(*Session, error) { closed++ },
	}
	h.onStart(context.Background())
	h.onActive(nil)
	h.onClosing(nil)
	h.onClose(nil, errors.New("x"))
	if started != 1 || active != 1 || closing != 1 || closed != 1 {
		t.Errorf("hook counts: started=%d active=%d closing=%d closed=%d, want 1 each",
			started, active, closing, closed)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
