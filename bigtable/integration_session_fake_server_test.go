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
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// fakeBigtableServer is an in-process Bigtable server used to exercise the
// vRPC session path at the wire level. It implements just enough of the
// BigtableServer surface to handle OpenTable handshakes, PingAndWarm,
// GetClientConfiguration (returning SessionLoad=1.0 so the Diverter routes
// traffic through the session path), and VirtualRpc requests for ReadRow /
// MutateRow.
//
// Every VirtualRpcRequest is captured (full proto, under mutex) so tests can
// assert on Deadline, Metadata.AttemptNumber, Metadata.AttemptStart, and the
// payload shape. A configurable hook (firstAttemptErr) makes the first vRPC
// per stream fail with a chosen status — enabling retry tests without per-test
// server boilerplate.
type fakeBigtableServer struct {
	btpb.UnimplementedBigtableServer

	mu sync.Mutex
	// captured is the ordered log of every VirtualRpcRequest the server
	// has seen, paired with the stream index that served it. One struct
	// per vRPC keeps request and stream aligned by construction, so a
	// test snapshotting requests and later reading stream indexes cannot
	// race an in-flight append.
	captured           []capturedVRpc
	openSessionCnt     int
	closeSessionCnt    int
	getClientConfigCnt int

	// attemptErrs is a queue: each incoming VirtualRpcRequest pops one
	// entry and (if non-nil) returns the encoded SessionResponse_Error to
	// the client. Empty queue = every request succeeds normally.
	attemptErrs []fakeAttemptErr

	// responseDelay is applied before every reply frame (success or error).
	// Used to force deadline / cancellation to fire mid-flight.
	responseDelay time.Duration

	// readRowResponse holds the TableResponse to send for ReadRow vRPCs.
	// Overridable via setReadRowResponse — the encoded bytes cache is
	// re-marshaled on set so a "row: nil" reply is representable.
	readRowResponseBytes []byte
	// mutateRowResponse is the proto-encoded TableResponse payload returned
	// for MutateRow virtual RPCs.
	mutateRowResponseBytes []byte

	// peerInfoHeaderBase is used as a template for the bigtable-peer-info
	// stream header. If per-session AFE IDs are configured via
	// setPeerInfoRotation, the fake stamps an increasing ApplicationFrontendId
	// onto a clone of this template per session; otherwise every session
	// receives the same base header.
	peerInfoHeaderBase *btpb.PeerInfo
	// peerInfoAfeRotation, when non-empty, provides ApplicationFrontendId
	// values to rotate through on successive OpenTable streams. Guarded by mu.
	peerInfoAfeRotation []int64
	// nextPeerInfoIdx is the index into peerInfoAfeRotation for the next
	// stream. atomic to keep the OpenTable read cheap.
	nextPeerInfoIdx atomic.Int64

	// poolMinCount/poolMaxCount stamp the SessionPoolConfiguration returned
	// from GetClientConfiguration. The client's ClientConfigurationManager
	// treats server-supplied values as authoritative and overwrites the
	// per-pool min/max — so tests must set these to match the
	// ClientConfig.SessionPoolMin/Max the harness passes, otherwise the
	// server defaults (5/400) fill the pool with far more sessions than
	// the test expects. Defaults align with the harness (1/2).
	poolMinCount int32
	poolMaxCount int32

	// sessionParamsKeepAlive, when > 0, causes the fake to send a
	// SessionParameters frame carrying this KeepAlive immediately after
	// the OpenSession handshake. Updates the client's atomic heartbeat
	// deadline to now + 3×KeepAlive. Note: the heartBeatLoop's Timer is
	// armed once at session Start to initialHeartbeatGrace (30m) and is
	// not reshuffled when the atomic shortens — the watchdog remains
	// parked for its original 30-min sleep. Tests use this together with
	// stallVRpcCount to prove that decoupling empirically.
	sessionParamsKeepAlive time.Duration

	// stallVRpcCount, when > 0, causes the next N VirtualRpcRequests to
	// block until the stream context is cancelled (no reply, no
	// heartbeat). Decremented under mu as each stall hits.
	stallVRpcCount int
}

// fakeAttemptErr is one queued reply for a VirtualRpcRequest. RetryInfo is
// optional; when non-nil, it is packed into the ErrorResponse envelope so
// the client's retry classifier sees an explicit server go-ahead.
type fakeAttemptErr struct {
	Status    *rpcstatus.Status
	RetryInfo *errdetails.RetryInfo
}

// capturedVRpc pairs one recorded VirtualRpcRequest with the stream index
// that served it. See fakeBigtableServer.captured for the alignment rule.
type capturedVRpc struct {
	req       *btpb.VirtualRpcRequest
	streamIdx int64
}

// getClientConfigCount returns the number of GetClientConfiguration RPCs the
// server has answered. Used by the harness to wait for the initial poll to
// land (which is what flips the Diverter to SessionLoad=1.0).
func (s *fakeBigtableServer) getClientConfigCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getClientConfigCnt
}

func newFakeBigtableServer(t *testing.T) *fakeBigtableServer {
	t.Helper()
	srv := &fakeBigtableServer{
		// Match newSessionTestHarness's ClientConfig defaults. Tests that
		// override SessionPoolMin/Max on the client MUST also call
		// setSessionPoolSizing on the fake, otherwise the config manager's
		// UpdateConfig overwrites the client values with these defaults.
		poolMinCount: 1,
		poolMaxCount: 2,
	}

	// Default ReadRow response: row "test-row" with one cell containing
	// "test-value" under family "fam1", qualifier "col1".
	rrResp := &btpb.TableResponse{
		Payload: &btpb.TableResponse_ReadRow{
			ReadRow: &btpb.SessionReadRowResponse{
				Row: &btpb.Row{
					Key: []byte("test-row"),
					Families: []*btpb.Family{{
						Name: "fam1",
						Columns: []*btpb.Column{{
							Qualifier: []byte("col1"),
							Cells: []*btpb.Cell{{
								Value:           []byte("test-value"),
								TimestampMicros: 1000,
							}},
						}},
					}},
				},
			},
		},
	}
	b, err := proto.Marshal(rrResp)
	if err != nil {
		t.Fatalf("proto.Marshal ReadRow response: %v", err)
	}
	srv.readRowResponseBytes = b

	mrResp := &btpb.TableResponse{
		Payload: &btpb.TableResponse_MutateRow{
			MutateRow: &btpb.SessionMutateRowResponse{},
		},
	}
	b, err = proto.Marshal(mrResp)
	if err != nil {
		t.Fatalf("proto.Marshal MutateRow response: %v", err)
	}
	srv.mutateRowResponseBytes = b
	return srv
}

// snapshotVRpcs returns a copy of every VirtualRpcRequest the server has
// seen, paired with the stream index that served it. A single-slice
// snapshot means a caller iterating both fields cannot read a
// misaligned pair even if a fresh vRPC lands mid-inspection.
func (s *fakeBigtableServer) snapshotVRpcs() []capturedVRpc {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]capturedVRpc, len(s.captured))
	copy(out, s.captured)
	return out
}

// setFirstAttemptErr arms the server so the next VirtualRpcRequest is met
// with the given status (no server-supplied RetryInfo). The arming is
// cleared after one use. Prefer queueAttemptErrs when the test needs
// multiple errors or an attached RetryInfo.
func (s *fakeBigtableServer) setFirstAttemptErr(st *rpcstatus.Status) {
	s.queueAttemptErrs(fakeAttemptErr{Status: st})
}

// queueAttemptErrs pushes N errors onto the reply queue. The next N
// VirtualRpcRequests will each pop one entry (in order) and receive it as
// a SessionResponse_Error, optionally carrying the supplied RetryInfo.
// After the queue drains, subsequent requests succeed normally.
func (s *fakeBigtableServer) queueAttemptErrs(errs ...fakeAttemptErr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attemptErrs = append(s.attemptErrs, errs...)
}

// queuedAttemptErrCount returns how many queued replies remain unused. A
// zero return after a call means the server consumed every armed error.
func (s *fakeBigtableServer) queuedAttemptErrCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.attemptErrs)
}

// setResponseDelay makes the fake sleep this long before sending each
// reply frame. Used to force ctx deadline / cancel to fire mid-flight.
func (s *fakeBigtableServer) setResponseDelay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responseDelay = d
}

// setReadRowResponse overrides the TableResponse the fake returns for
// ReadRow vRPCs. Encodes to bytes once; subsequent requests use the
// cached slice under mu.
func (s *fakeBigtableServer) setReadRowResponse(t *testing.T, resp *btpb.TableResponse) {
	t.Helper()
	b, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("proto.Marshal ReadRow override: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readRowResponseBytes = b
}

// setPeerInfoRotation configures the fake to stamp
// PeerInfo.ApplicationFrontendId with the values in `ids`, rotating one
// per OpenTable stream (i.e. one per session). Call before session pool
// starts opening streams. Non-thread-safe once traffic is flowing.
func (s *fakeBigtableServer) setPeerInfoRotation(ids []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerInfoAfeRotation = append([]int64(nil), ids...)
	if s.peerInfoHeaderBase == nil {
		s.peerInfoHeaderBase = &btpb.PeerInfo{
			TransportType: btpb.PeerInfo_TRANSPORT_TYPE_SESSION_DIRECT_ACCESS,
		}
	}
}

// setSessionPoolSizing sets the min/max the fake advertises in
// GetClientConfiguration. Call this BEFORE the harness's
// waitForSessionRouting completes so the first config poll delivers the
// intended sizing to the pool.
func (s *fakeBigtableServer) setSessionPoolSizing(min, max int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poolMinCount = min
	s.poolMaxCount = max
}

// setSessionParamsKeepAlive tells the fake to emit a SessionParameters
// frame with the given KeepAlive interval on every stream (right after
// OpenSession). Call before any stream opens.
func (s *fakeBigtableServer) setSessionParamsKeepAlive(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionParamsKeepAlive = d
}

// queueVRpcStalls arms the next `n` VirtualRpcRequests to block until
// the stream ctx is cancelled — no reply, no heartbeat. Used to force
// the client into the "no server frames" state that the missed-
// heartbeat watchdog exists to detect.
func (s *fakeBigtableServer) queueVRpcStalls(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stallVRpcCount += n
}

// closeSessionCount returns how many CloseSession frames the server has
// received across all streams.
func (s *fakeBigtableServer) closeSessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeSessionCnt
}

// openSessionCount returns how many OpenSession handshakes the server has
// completed. Bumped once per bidi stream.
func (s *fakeBigtableServer) openSessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openSessionCnt
}

// peerInfoHeaderFor returns the base64-encoded PeerInfo header value to
// stamp onto the OpenTable stream. Returns "" when no peer info is
// configured (older behaviour — sessions get AfeID=0).
func (s *fakeBigtableServer) peerInfoHeaderFor(streamIdx int64) string {
	s.mu.Lock()
	base := s.peerInfoHeaderBase
	rot := s.peerInfoAfeRotation
	s.mu.Unlock()
	if base == nil {
		return ""
	}
	pi := proto.Clone(base).(*btpb.PeerInfo)
	if len(rot) > 0 {
		pi.ApplicationFrontendId = rot[int(streamIdx)%len(rot)]
	}
	raw, err := proto.Marshal(pi)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func (s *fakeBigtableServer) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	return &btpb.PingAndWarmResponse{}, nil
}

func (s *fakeBigtableServer) GetClientConfiguration(ctx context.Context, req *btpb.GetClientConfigurationRequest) (*btpb.ClientConfiguration, error) {
	s.mu.Lock()
	s.getClientConfigCnt++
	minC, maxC := s.poolMinCount, s.poolMaxCount
	s.mu.Unlock()
	// SessionLoad: 1.0 pins the Diverter to the session path so every
	// ReadRow/Apply on the TableShim routes through SessionTable. Note: the
	// AddSessionLoadListener listener is invoked at registration time with
	// the manager's *default* config (SessionLoad=0), and is only flipped to
	// 1.0 once this RPC's response is parsed by the configManager. Tests
	// must wait for that to happen before issuing data calls — see
	// waitForSessionRouting in the harness.
	//
	// Explicit SessionPool sizing is included because ClientConfigurationManager
	// treats server values as authoritative and overwrites the per-pool
	// min/max the client asked for. Leaving SessionPool unset here means the
	// manager's built-in default (Min=5, Max=400) fills the pool with far
	// more sessions than tests expect. See setSessionPoolSizing.
	return &btpb.ClientConfiguration{
		SessionConfiguration: &btpb.SessionClientConfiguration{
			SessionLoad: 1.0,
			SessionPoolConfiguration: &btpb.SessionClientConfiguration_SessionPoolConfiguration{
				MinSessionCount: minC,
				MaxSessionCount: maxC,
			},
		},
	}, nil
}

// OpenTable handles the bidi stream: completes the OpenSession handshake,
// then for every VirtualRpcRequest decodes the inner TableRequest, captures
// the full proto, and replies with the appropriate TableResponse (or an
// error popped from the queue). Also honors an optional response delay so
// tests can drive deadline / cancel semantics deterministically.
func (s *fakeBigtableServer) OpenTable(stream btpb.Bigtable_OpenTableServer) error {
	// Handshake: first message must be OpenSession.
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.GetOpenSession() == nil {
		return fmt.Errorf("fakeBigtableServer: expected OpenSession as first frame, got %T", first.GetPayload())
	}
	// Stamp per-stream state before the reply so handleOpenSession sees a
	// consistent OpenSession count when it lands.
	streamIdx := s.nextPeerInfoIdx.Add(1) - 1
	s.mu.Lock()
	s.openSessionCnt++
	s.mu.Unlock()

	// If configured, send the bigtable-peer-info header BEFORE the first
	// response so the client's handleOpenSession parses it synchronously.
	if hdr := s.peerInfoHeaderFor(streamIdx); hdr != "" {
		if err := stream.SendHeader(metadata.Pairs("bigtable-peer-info", hdr)); err != nil {
			return err
		}
	}

	if err := stream.Send(&btpb.SessionResponse{
		Payload: &btpb.SessionResponse_OpenSession{
			OpenSession: &btpb.OpenSessionResponse{},
		},
	}); err != nil {
		return err
	}

	// Optional: negotiate a short KeepAlive so the client's atomic
	// heartbeat deadline shortens to now + 3×KeepAlive. Handled by
	// handleSessionParameters (session_lifecycle.go:318-328).
	s.mu.Lock()
	keepAlive := s.sessionParamsKeepAlive
	s.mu.Unlock()
	if keepAlive > 0 {
		if err := stream.Send(&btpb.SessionResponse{
			Payload: &btpb.SessionResponse_SessionParameters{
				SessionParameters: &btpb.SessionParametersResponse{
					KeepAlive: durationpb.New(keepAlive),
				},
			},
		}); err != nil {
			return err
		}
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// CloseSession ends the stream cleanly.
		if req.GetCloseSession() != nil {
			s.mu.Lock()
			s.closeSessionCnt++
			s.mu.Unlock()
			return nil
		}
		vrpc := req.GetVirtualRpc()
		if vrpc == nil {
			// Ignore other oneof variants (heartbeats etc) — not used here.
			continue
		}

		// Capture the full vRPC request and pop one queued error (if any)
		// under a single lock so ordering is deterministic under load.
		s.mu.Lock()
		s.captured = append(s.captured, capturedVRpc{req: vrpc, streamIdx: streamIdx})
		var armed *fakeAttemptErr
		if len(s.attemptErrs) > 0 {
			armed = &s.attemptErrs[0]
			s.attemptErrs = s.attemptErrs[1:]
		}
		stall := s.stallVRpcCount > 0
		if stall {
			s.stallVRpcCount--
		}
		delay := s.responseDelay
		s.mu.Unlock()

		// Stall path: never reply, never emit heartbeats. Block until
		// the client cancels the stream (via ctx expiry or ForceClose).
		if stall {
			<-stream.Context().Done()
			return stream.Context().Err()
		}

		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-stream.Context().Done():
				return stream.Context().Err()
			}
		}

		if armed != nil {
			errResp := &btpb.SessionResponse{
				Payload: &btpb.SessionResponse_Error{
					Error: &btpb.ErrorResponse{
						RpcId:     vrpc.RpcId,
						Status:    armed.Status,
						RetryInfo: armed.RetryInfo,
					},
				},
			}
			if err := stream.Send(errResp); err != nil {
				return err
			}
			continue
		}

		// Decode the inner TableRequest to pick the right response shape.
		var tableReq btpb.TableRequest
		if err := proto.Unmarshal(vrpc.Payload, &tableReq); err != nil {
			return fmt.Errorf("fakeBigtableServer: unmarshal TableRequest: %v", err)
		}

		s.mu.Lock()
		readBytes := s.readRowResponseBytes
		writeBytes := s.mutateRowResponseBytes
		s.mu.Unlock()

		var payload []byte
		switch tableReq.Payload.(type) {
		case *btpb.TableRequest_ReadRow:
			payload = readBytes
		case *btpb.TableRequest_MutateRow:
			payload = writeBytes
		default:
			return fmt.Errorf("fakeBigtableServer: unsupported TableRequest payload %T", tableReq.Payload)
		}

		resp := &btpb.SessionResponse{
			Payload: &btpb.SessionResponse_VirtualRpc{
				VirtualRpc: &btpb.VirtualRpcResponse{
					RpcId: vrpc.RpcId,
					ClusterInfo: &btpb.ClusterInformation{
						ClusterId: "fake-c1",
						ZoneId:    "fake-z1",
					},
					Stats: &btpb.SessionRequestStats{
						BackendLatency: durationpb.New(7 * time.Millisecond),
					},
					Payload: payload,
				},
			},
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}
