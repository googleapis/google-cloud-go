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

package storage

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

type recordedLog struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// recordingLogHandler captures structured slog records in memory and notifies
// listeners on each Handle call so async watcher tests synchronize without
// fixed sleeps.
type recordingLogHandler struct {
	mu      sync.Mutex
	level   slog.Level
	records []recordedLog
	notify  chan struct{}
}

func newRecordingLogHandler(level slog.Level) *recordingLogHandler {
	return &recordingLogHandler{
		level:  level,
		notify: make(chan struct{}, 16),
	}
}

func (h *recordingLogHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

func (h *recordingLogHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, recordedLog{
		level: r.Level,
		msg:   r.Message,
		attrs: attrs,
	})
	h.mu.Unlock()
	select {
	case h.notify <- struct{}{}:
	default:
	}
	return nil
}

func (h *recordingLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingLogHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingLogHandler) snapshot() []recordedLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedLog, len(h.records))
	copy(out, h.records)
	return out
}

// waitForLogCount blocks until h has captured at least want records or timeout
// elapses.
func waitForLogCount(t *testing.T, h *recordingLogHandler, want int, timeout time.Duration) []recordedLog {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		recs := h.snapshot()
		if len(recs) >= want {
			return recs
		}
		select {
		case <-h.notify:
		case <-timer.C:
			t.Fatalf("timed out after %v waiting for %d log record(s); got %d: %+v", timeout, want, len(recs), recs)
		}
	}
}

// attrsWithoutElapsed returns a shallow copy of attrs excluding elapsed_ms so
// callers can compare all deterministic fields with cmp.Diff.
func attrsWithoutElapsed(attrs map[string]any) map[string]any {
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		if k != "elapsed_ms" {
			out[k] = v
		}
	}
	return out
}

// fakeConnectivityWatcher implements connectivityWatcher with deterministic
// state transitions and per-wait state notifications for unit tests.
type fakeConnectivityWatcher struct {
	target string

	mu           sync.Mutex
	state        connectivity.State
	changed      chan struct{}
	connectCalls int
	onConnect    func()
	waitStates   chan connectivity.State
}

func newFakeConnectivityWatcher(target string, initial connectivity.State) *fakeConnectivityWatcher {
	return &fakeConnectivityWatcher{
		target:     target,
		state:      initial,
		changed:    make(chan struct{}),
		waitStates: make(chan connectivity.State, 16),
	}
}

func (f *fakeConnectivityWatcher) Target() string { return f.target }

func (f *fakeConnectivityWatcher) GetState() connectivity.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakeConnectivityWatcher) Connect() {
	f.mu.Lock()
	f.connectCalls++
	cb := f.onConnect
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (f *fakeConnectivityWatcher) setState(s connectivity.State) {
	f.mu.Lock()
	f.state = s
	close(f.changed)
	f.changed = make(chan struct{})
	f.mu.Unlock()
}

func (f *fakeConnectivityWatcher) WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool {
	f.mu.Lock()
	select {
	case f.waitStates <- sourceState:
	default:
	}
	if f.state != sourceState {
		f.mu.Unlock()
		return true
	}
	ch := f.changed
	f.mu.Unlock()

	select {
	case <-ctx.Done():
		return false
	case <-ch:
		return true
	}
}

func (f *fakeConnectivityWatcher) connects() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connectCalls
}

// waitForObservedState blocks until fake enters WaitForStateChange for a state
// and returns it so multi-step transition tests synchronize cleanly.
func waitForObservedState(t *testing.T, fake *fakeConnectivityWatcher, timeout time.Duration) connectivity.State {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case s := <-fake.waitStates:
		return s
	case <-timer.C:
		t.Fatalf("timed out after %v waiting for WaitForStateChange call", timeout)
		return connectivity.Idle
	}
}

// startTestGRPCServer starts an in-process gRPC server on an ephemeral loopback
// port and registers Stop on test cleanup.
func startTestGRPCServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	srv := grpc.NewServer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
		<-done
	})
	return lis.Addr().String()
}

// startHangingTCPListener accepts connections without completing an HTTP/2
// handshake so a real gRPC channel remains in CONNECTING until closed.
func startHangingTCPListener(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	closed := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, acceptErr := lis.Accept()
			if acceptErr != nil {
				return
			}
			mu.Lock()
			if closed {
				mu.Unlock()
				c.Close()
				return
			}
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		closed = true
		toClose := conns
		conns = nil
		mu.Unlock()
		lis.Close()
		for _, c := range toClose {
			c.Close()
		}
		<-done
	})
	return lis.Addr().String()
}

// localGRPCClientOptions returns client options targeting a local insecure gRPC
// server with telemetry captured by h.
func localGRPCClientOptions(t *testing.T, addr string, h *recordingLogHandler, extra ...option.ClientOption) []option.ClientOption {
	t.Helper()
	unsetEnvForTest(t, "STORAGE_EMULATOR_HOST_GRPC")
	opts := []option.ClientOption{
		option.WithEndpoint(addr),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
		option.WithLogger(slog.New(h)),
		WithDisabledClientMetrics(),
	}
	return append(opts, extra...)
}

func TestDetectTransportType(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "empty", target: "", want: transportCloudPath},
		{name: "cloudpath_default", target: "storage.googleapis.com:443", want: transportCloudPath},
		{name: "dns_scheme", target: "dns:///storage.googleapis.com:443", want: transportCloudPath},
		{name: "direct_host_without_c2p", target: "storage-direct.googleapis.com:443", want: transportCloudPath},
		{name: "emulator_host", target: "localhost:8080", want: transportCloudPath},
		{name: "directpath_default", target: "google-c2p:///storage.googleapis.com", want: transportDirectPath},
		{name: "directpath_uppercase_scheme", target: "GOOGLE-C2P:///storage.googleapis.com", want: transportDirectPath},
		{name: "direct_host_no_force_xds", target: "google-c2p:///storage-direct.googleapis.com", want: transportDirectPath},
		{name: "interconnect_prod", target: "google-c2p:///storage-direct.googleapis.com?force-xds", want: transportDirectPathInterconnect},
		{name: "interconnect_uppercase_scheme", target: "GOOGLE-C2P:///storage-direct.googleapis.com?force-xds", want: transportDirectPathInterconnect},
		{name: "interconnect_preprod", target: "google-c2p:///storage-preprod-direct.sandbox.googleapis.com?force-xds", want: transportDirectPathInterconnect},
		{name: "force_xds_with_value_true", target: "google-c2p:///storage-direct.googleapis.com?force-xds=true", want: transportDirectPathInterconnect},
		{name: "force_xds_with_value_zero", target: "google-c2p:///storage-direct.googleapis.com?force-xds=0", want: transportDirectPathInterconnect},
		{name: "force_xds_multiple_params", target: "google-c2p:///storage-direct.googleapis.com?a=1&force-xds&b=2", want: transportDirectPathInterconnect},
		{name: "force_xds_before_fragment", target: "google-c2p:///storage-direct.googleapis.com?force-xds#frag", want: transportDirectPathInterconnect},
		{name: "force_xds_percent_encoded_key", target: "google-c2p:///storage-direct.googleapis.com?force%2Dxds", want: transportDirectPathInterconnect},
		{name: "force_xds_as_value_ignored", target: "google-c2p:///storage.googleapis.com?foo=force-xds", want: transportDirectPath},
		{name: "force_xds_key_substring_ignored", target: "google-c2p:///storage.googleapis.com?disable-force-xds", want: transportDirectPath},
		{name: "force_xds_in_fragment_ignored", target: "google-c2p:///storage.googleapis.com#frag?force-xds", want: transportDirectPath},
		{name: "two_slash_c2p_not_directpath", target: "google-c2p://storage-direct.googleapis.com?force-xds", want: transportCloudPath},
		{name: "c2p_experimental_not_directpath", target: "google-c2p-experimental:///storage-direct.googleapis.com?force-xds", want: transportCloudPath},
		{name: "malformed_url_escape", target: "google-c2p:///storage.googleapis.com%zz", want: transportCloudPath},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectTransportType(tc.target); got != tc.want {
				t.Errorf("detectTransportType(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestChannelReadyNote(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		want      string
	}{
		{
			name:      "cloudpath_no_note",
			requested: transportCloudPath,
			want:      "",
		},
		{
			name:      "directpath_note",
			requested: transportDirectPath,
			want:      ", but gRPC does not expose the LB policy, so transport_type assumes the requested transport",
		},
		{
			name:      "interconnect_note",
			requested: transportDirectPathInterconnect,
			want:      ", but gRPC does not expose the LB policy, so transport_type assumes the requested transport",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := channelReadyNote(tc.requested); got != tc.want {
				t.Errorf("channelReadyNote(%q) = %q, want %q", tc.requested, got, tc.want)
			}
		})
	}
}

func TestWatchChannelReady_ReadyLogs(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		wantRequested string
		wantReadyMsg  string
	}{
		{
			name:          "cloudpath_default",
			target:        "storage.googleapis.com:443",
			wantRequested: transportCloudPath,
			wantReadyMsg:  "gRPC channel [0] is ready.",
		},
		{
			name:          "directpath",
			target:        "google-c2p:///storage.googleapis.com",
			wantRequested: transportDirectPath,
			wantReadyMsg:  "gRPC channel [0] is ready, but gRPC does not expose the LB policy, so transport_type assumes the requested transport.",
		},
		{
			name:          "interconnect",
			target:        "google-c2p:///storage-direct.googleapis.com?force-xds",
			wantRequested: transportDirectPathInterconnect,
			wantReadyMsg:  "gRPC channel [0] is ready, but gRPC does not expose the LB policy, so transport_type assumes the requested transport.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRecordingLogHandler(slog.LevelInfo)
			logger := slog.New(h)
			fake := newFakeConnectivityWatcher(tc.target, connectivity.Ready)

			logChannelConfiguration(logger, tc.target)
			stop := watchChannelReady(t.Context(), logger, fake, time.Now(), time.Second)
			t.Cleanup(stop)

			recs := waitForLogCount(t, h, 2, time.Second)
			// Verify stop is idempotent and waits cleanly.
			stop()
			stop()

			wantConfig := map[string]any{
				"requested_transport_type": tc.wantRequested,
				"endpoint":                 tc.target,
				"authority":                unknownTelemetryValue,
				"grpc_version":             grpc.Version,
			}
			if recs[0].level != slog.LevelInfo || recs[0].msg != "Configured gRPC transport." {
				t.Errorf("logChannelConfiguration(%q) = (%v, %q), want (INFO, %q)", tc.target, recs[0].level, recs[0].msg, "Configured gRPC transport.")
			}
			if diff := cmp.Diff(wantConfig, recs[0].attrs); diff != "" {
				t.Errorf("logChannelConfiguration(%q) attrs diff (-want +got):\n%s", tc.target, diff)
			}

			readyRec := recs[1]
			if readyRec.level != slog.LevelInfo || readyRec.msg != tc.wantReadyMsg {
				t.Errorf("watchChannelReady(%q) = (%v, %q), want (INFO, %q)", tc.target, readyRec.level, readyRec.msg, tc.wantReadyMsg)
			}
			elapsedMS, ok := readyRec.attrs["elapsed_ms"].(int64)
			if !ok || elapsedMS < 0 {
				t.Errorf("watchChannelReady(%q) elapsed_ms = %v (%T), want non-negative int64", tc.target, readyRec.attrs["elapsed_ms"], readyRec.attrs["elapsed_ms"])
			}
			wantReady := map[string]any{
				"transport_type":           tc.wantRequested,
				"requested_transport_type": tc.wantRequested,
				"lb_policy":                unknownTelemetryValue,
			}
			if diff := cmp.Diff(wantReady, attrsWithoutElapsed(readyRec.attrs)); diff != "" {
				t.Errorf("watchChannelReady(%q) attrs diff (-want +got):\n%s", tc.target, diff)
			}
		})
	}
}

func TestLogChannelReady_NegativeElapsedClamped(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	logChannelReady(slog.New(h), transportCloudPath, -5*time.Millisecond, "")

	recs := h.snapshot()
	if len(recs) != 1 {
		t.Fatalf("logChannelReady(-5ms) emitted %d record(s), want 1", len(recs))
	}
	if got := recs[0].attrs["elapsed_ms"]; got != int64(0) {
		t.Errorf("logChannelReady(-5ms) elapsed_ms = %v, want 0", got)
	}
}

func TestWatchChannelReady_IdleTriggersConnect(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	target := "google-c2p:///storage-direct.googleapis.com?force-xds"
	fake := newFakeConnectivityWatcher(target, connectivity.Idle)
	// Simulate an idle channel that transitions to READY only once Connect() is
	// called.
	fake.onConnect = func() {
		fake.setState(connectivity.Connecting)
		fake.setState(connectivity.Ready)
	}

	stop := watchChannelReady(t.Context(), slog.New(h), fake, time.Now(), time.Second)
	t.Cleanup(stop)

	recs := waitForLogCount(t, h, 1, time.Second)
	stop()

	if fake.connects() < 1 {
		t.Errorf("watchChannelReady(%q, Idle) Connect calls = %d, want >= 1", target, fake.connects())
	}
	if got := recs[0].attrs["transport_type"]; got != transportDirectPathInterconnect {
		t.Errorf("watchChannelReady(%q, Idle) transport_type = %v, want %q", target, got, transportDirectPathInterconnect)
	}
}

func TestWatchChannelReady_TransientFailureRecovers(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	target := "google-c2p:///storage-direct.googleapis.com?force-xds"
	fake := newFakeConnectivityWatcher(target, connectivity.Connecting)

	stop := watchChannelReady(t.Context(), slog.New(h), fake, time.Now(), time.Second)
	t.Cleanup(stop)

	// Synchronize each state transition with WaitForStateChange so the watcher
	// is guaranteed to observe TransientFailure rather than skipping straight
	// to Ready.
	if got := waitForObservedState(t, fake, time.Second); got != connectivity.Connecting {
		t.Fatalf("first WaitForStateChange sourceState = %v, want %v", got, connectivity.Connecting)
	}
	fake.setState(connectivity.TransientFailure)
	if got := waitForObservedState(t, fake, time.Second); got != connectivity.TransientFailure {
		t.Fatalf("second WaitForStateChange sourceState = %v, want %v", got, connectivity.TransientFailure)
	}
	fake.setState(connectivity.Connecting)
	if got := waitForObservedState(t, fake, time.Second); got != connectivity.Connecting {
		t.Fatalf("third WaitForStateChange sourceState = %v, want %v", got, connectivity.Connecting)
	}
	fake.setState(connectivity.Ready)

	recs := waitForLogCount(t, h, 1, time.Second)
	stop()

	if len(recs) != 1 {
		t.Fatalf("watchChannelReady(%q) emitted %d record(s), want 1: %+v", target, len(recs), recs)
	}
	if got := recs[0].attrs["transport_type"]; got != transportDirectPathInterconnect {
		t.Errorf("watchChannelReady(%q) transport_type = %v, want %q", target, got, transportDirectPathInterconnect)
	}
}

func TestWatchChannelReady_Shutdown(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	target := "google-c2p:///storage-direct.googleapis.com?force-xds"
	fake := newFakeConnectivityWatcher(target, connectivity.Shutdown)

	stop := watchChannelReady(t.Context(), slog.New(h), fake, time.Now(), time.Second)
	t.Cleanup(stop)

	recs := waitForLogCount(t, h, 1, time.Second)
	stop()

	rec := recs[0]
	if rec.level != slog.LevelInfo || rec.msg != "gRPC channel [0] did not become ready." {
		t.Errorf("watchChannelReady(%q, Shutdown) = (%v, %q), want (INFO, %q)", target, rec.level, rec.msg, "gRPC channel [0] did not become ready.")
	}
	wantAttrs := map[string]any{
		"requested_transport_type": transportDirectPathInterconnect,
		"lb_policy":                unknownTelemetryValue,
		"status":                   "channel shut down",
	}
	if diff := cmp.Diff(wantAttrs, attrsWithoutElapsed(rec.attrs)); diff != "" {
		t.Errorf("watchChannelReady(%q, Shutdown) attrs diff (-want +got):\n%s", target, diff)
	}
}

func TestWatchChannelReady_Timeout(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	target := "google-c2p:///storage-direct.googleapis.com?force-xds"
	fake := newFakeConnectivityWatcher(target, connectivity.Connecting)

	stop := watchChannelReady(t.Context(), slog.New(h), fake, time.Now(), 20*time.Millisecond)
	t.Cleanup(stop)

	recs := waitForLogCount(t, h, 1, time.Second)
	stop()

	rec := recs[0]
	if rec.level != slog.LevelInfo || rec.msg != "gRPC channel [0] did not become ready." {
		t.Errorf("watchChannelReady(%q, timeout=20ms) = (%v, %q), want (INFO, %q)", target, rec.level, rec.msg, "gRPC channel [0] did not become ready.")
	}
	wantAttrs := map[string]any{
		"requested_transport_type": transportDirectPathInterconnect,
		"lb_policy":                unknownTelemetryValue,
		"status":                   context.DeadlineExceeded.Error(),
	}
	if diff := cmp.Diff(wantAttrs, attrsWithoutElapsed(rec.attrs)); diff != "" {
		t.Errorf("watchChannelReady(%q, timeout=20ms) attrs diff (-want +got):\n%s", target, diff)
	}
}

func TestWatchChannelReady_AlreadyExpiredDoesNotConnect(t *testing.T) {
	h := newRecordingLogHandler(slog.LevelInfo)
	target := "google-c2p:///storage-direct.googleapis.com?force-xds"
	fake := newFakeConnectivityWatcher(target, connectivity.Idle)

	stop := watchChannelReady(t.Context(), slog.New(h), fake, time.Now(), 0)
	t.Cleanup(stop)

	recs := waitForLogCount(t, h, 1, time.Second)
	stop()

	if got := fake.connects(); got != 0 {
		t.Errorf("watchChannelReady(%q, timeout=0) Connect calls = %d, want 0", target, got)
	}
	if got := recs[0].attrs["status"]; got != context.DeadlineExceeded.Error() {
		t.Errorf("watchChannelReady(%q, timeout=0) status = %v, want %q", target, got, context.DeadlineExceeded.Error())
	}
}

func TestChannelTelemetry_LocalEndpointReady(t *testing.T) {
	addr := startTestGRPCServer(t)
	h := newRecordingLogHandler(slog.LevelInfo)

	client, err := NewGRPCClient(t.Context(), localGRPCClientOptions(t, addr, h)...)
	if err != nil {
		t.Fatalf("NewGRPCClient(%q) failed: %v", addr, err)
	}
	defer client.Close()

	recs := waitForLogCount(t, h, 2, 5*time.Second)
	wantStartup := map[string]any{
		"requested_transport_type": transportCloudPath,
		"endpoint":                 addr,
		"authority":                unknownTelemetryValue,
		"grpc_version":             grpc.Version,
	}
	if recs[0].level != slog.LevelInfo || recs[0].msg != "Configured gRPC transport." {
		t.Errorf("NewGRPCClient(%q) startup log = (%v, %q), want (INFO, %q)", addr, recs[0].level, recs[0].msg, "Configured gRPC transport.")
	}
	if diff := cmp.Diff(wantStartup, recs[0].attrs); diff != "" {
		t.Errorf("NewGRPCClient(%q) startup attrs diff (-want +got):\n%s", addr, diff)
	}

	wantReady := map[string]any{
		"transport_type":           transportCloudPath,
		"requested_transport_type": transportCloudPath,
		"lb_policy":                unknownTelemetryValue,
	}
	if recs[1].level != slog.LevelInfo || recs[1].msg != "gRPC channel [0] is ready." {
		t.Errorf("NewGRPCClient(%q) ready log = (%v, %q), want (INFO, %q)", addr, recs[1].level, recs[1].msg, "gRPC channel [0] is ready.")
	}
	if diff := cmp.Diff(wantReady, attrsWithoutElapsed(recs[1].attrs)); diff != "" {
		t.Errorf("NewGRPCClient(%q) ready attrs diff (-want +got):\n%s", addr, diff)
	}
	if elapsedMS, ok := recs[1].attrs["elapsed_ms"].(int64); !ok || elapsedMS < 0 {
		t.Errorf("NewGRPCClient(%q) ready elapsed_ms = %v (%T), want non-negative int64", addr, recs[1].attrs["elapsed_ms"], recs[1].attrs["elapsed_ms"])
	}
}

func TestChannelTelemetry_EmulatorReady(t *testing.T) {
	addr := startTestGRPCServer(t)
	t.Setenv("STORAGE_EMULATOR_HOST_GRPC", addr)
	h := newRecordingLogHandler(slog.LevelInfo)

	client, err := NewGRPCClient(t.Context(),
		option.WithLogger(slog.New(h)),
		WithDisabledClientMetrics(),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient(emulator=%q) failed: %v", addr, err)
	}
	defer client.Close()

	recs := waitForLogCount(t, h, 2, 5*time.Second)
	wantStartup := map[string]any{
		"requested_transport_type": transportCloudPath,
		"endpoint":                 addr,
		"authority":                unknownTelemetryValue,
		"grpc_version":             grpc.Version,
	}
	if diff := cmp.Diff(wantStartup, recs[0].attrs); diff != "" {
		t.Errorf("NewGRPCClient(emulator=%q) startup attrs diff (-want +got):\n%s", addr, diff)
	}
	wantReady := map[string]any{
		"transport_type":           transportCloudPath,
		"requested_transport_type": transportCloudPath,
		"lb_policy":                unknownTelemetryValue,
	}
	if diff := cmp.Diff(wantReady, attrsWithoutElapsed(recs[1].attrs)); diff != "" {
		t.Errorf("NewGRPCClient(emulator=%q) ready attrs diff (-want +got):\n%s", addr, diff)
	}
}

func TestChannelTelemetry_CloseCancelsWatcher(t *testing.T) {
	addr := startHangingTCPListener(t)
	h := newRecordingLogHandler(slog.LevelInfo)

	client, err := NewGRPCClient(t.Context(), localGRPCClientOptions(t, addr, h)...)
	if err != nil {
		t.Fatalf("NewGRPCClient(%q) failed: %v", addr, err)
	}

	// Close immediately while channel [0] is still trying to handshake. Close
	// must return promptly rather than waiting for the 30s readiness deadline.
	closeStart := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close() failed: %v", err)
	}
	if elapsed := time.Since(closeStart); elapsed > 5*time.Second {
		t.Errorf("client.Close() took %v, want prompt cancellation well under 30s", elapsed)
	}

	recs := h.snapshot()
	if len(recs) != 2 {
		t.Fatalf("NewGRPCClient(%q) + Close() emitted %d log record(s), want 2: %+v", addr, len(recs), recs)
	}
	if recs[1].level != slog.LevelInfo || recs[1].msg != "gRPC channel [0] did not become ready." {
		t.Errorf("Close() watcher log = (%v, %q), want (INFO, %q)", recs[1].level, recs[1].msg, "gRPC channel [0] did not become ready.")
	}
	wantAttrs := map[string]any{
		"requested_transport_type": transportCloudPath,
		"lb_policy":                unknownTelemetryValue,
		"status":                   context.Canceled.Error(),
	}
	if diff := cmp.Diff(wantAttrs, attrsWithoutElapsed(recs[1].attrs)); diff != "" {
		t.Errorf("Close() watcher attrs diff (-want +got):\n%s", diff)
	}
}

func TestChannelTelemetry_DecoupledFromInitContext(t *testing.T) {
	addr := startTestGRPCServer(t)
	h := newRecordingLogHandler(slog.LevelInfo)

	// Block the transport dial until after initCtx is canceled so the channel
	// is guaranteed to still be CONNECTING when cancelInit runs.
	dialStarted := make(chan struct{})
	releaseDial := make(chan struct{})
	var dialOnce sync.Once
	dialer := func(ctx context.Context, target string) (net.Conn, error) {
		dialOnce.Do(func() { close(dialStarted) })
		select {
		case <-releaseDial:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", target)
	}

	initCtx, cancelInit := context.WithCancel(t.Context())
	client, err := NewGRPCClient(initCtx, localGRPCClientOptions(t, addr, h, option.WithGRPCDialOption(grpc.WithContextDialer(dialer)))...)
	if err != nil {
		t.Fatalf("NewGRPCClient(%q) failed: %v", addr, err)
	}
	t.Cleanup(func() {
		select {
		case <-releaseDial:
		default:
			close(releaseDial)
		}
		client.Close()
	})

	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked gRPC dial to start")
	}
	cancelInit()
	close(releaseDial)

	recs := waitForLogCount(t, h, 2, 5*time.Second)
	if recs[1].msg != "gRPC channel [0] is ready." {
		t.Errorf("NewGRPCClient(%q) after cancelInit logged %q, want %q", addr, recs[1].msg, "gRPC channel [0] is ready.")
	}
}

func TestChannelTelemetry_SkipsCustomGRPCConn(t *testing.T) {
	unsetEnvForTest(t, "STORAGE_EMULATOR_HOST_GRPC")
	addr := startTestGRPCServer(t)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q) failed: %v", addr, err)
	}
	defer conn.Close()

	h := newRecordingLogHandler(slog.LevelInfo)
	client, err := NewGRPCClient(t.Context(),
		option.WithGRPCConn(conn),
		option.WithLogger(slog.New(h)),
		WithDisabledClientMetrics(),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient(WithGRPCConn) failed: %v", err)
	}
	// Verify telemetry did not call Connect() on the caller-owned connection
	// before client.Close() shuts it down.
	if got := conn.GetState(); got != connectivity.Idle {
		t.Errorf("custom conn.GetState() before Close = %v, want %v (telemetry must not call Connect)", got, connectivity.Idle)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close() failed: %v", err)
	}
	if recs := h.snapshot(); len(recs) != 0 {
		t.Errorf("NewGRPCClient(WithGRPCConn) emitted %d log record(s), want 0: %+v", len(recs), recs)
	}
}

func TestChannelTelemetry_DisabledLoggerNoGoroutine(t *testing.T) {
	unsetEnvForTest(t, "GOOGLE_SDK_GO_LOGGING_LEVEL")
	addr := startTestGRPCServer(t)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q) failed: %v", addr, err)
	}
	defer conn.Close()

	stop := startClientChannelTelemetry(t.Context(), nil, conn, time.Now())
	if stop != nil {
		stop()
		t.Errorf("startClientChannelTelemetry(disabled logger) returned non-nil stop func, want nil")
	}
	if got := conn.GetState(); got != connectivity.Idle {
		t.Errorf("conn.GetState() with disabled logger = %v, want %v", got, connectivity.Idle)
	}
}

func TestChannelTelemetry_WarnLevelSkipsInfoTelemetry(t *testing.T) {
	addr := startTestGRPCServer(t)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q) failed: %v", addr, err)
	}
	defer conn.Close()

	h := newRecordingLogHandler(slog.LevelWarn)
	stop := startClientChannelTelemetry(
		t.Context(),
		[]option.ClientOption{option.WithLogger(slog.New(h))},
		conn,
		time.Now(),
	)
	if stop != nil {
		stop()
		t.Errorf("startClientChannelTelemetry(LevelWarn) returned non-nil stop func, want nil")
	}
	if got := conn.GetState(); got != connectivity.Idle {
		t.Errorf("conn.GetState() with LevelWarn = %v, want %v", got, connectivity.Idle)
	}
}

func TestStartClientChannelTelemetry_EnvLoggingLevel(t *testing.T) {
	// Verify GOOGLE_SDK_GO_LOGGING_LEVEL=info activates telemetry without
	// option.WithLogger, while warn keeps it skipped.
	addr := startTestGRPCServer(t)

	t.Run("info_enables", func(t *testing.T) {
		t.Setenv("GOOGLE_SDK_GO_LOGGING_LEVEL", "info")
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("grpc.NewClient(%q) failed: %v", addr, err)
		}
		defer conn.Close()

		stop := startClientChannelTelemetry(t.Context(), nil, conn, time.Now())
		if stop == nil {
			t.Fatal("startClientChannelTelemetry(GOOGLE_SDK_GO_LOGGING_LEVEL=info) = nil, want non-nil")
		}
		stop()
	})

	t.Run("warn_skips", func(t *testing.T) {
		t.Setenv("GOOGLE_SDK_GO_LOGGING_LEVEL", "warn")
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("grpc.NewClient(%q) failed: %v", addr, err)
		}
		defer conn.Close()

		stop := startClientChannelTelemetry(t.Context(), nil, conn, time.Now())
		if stop != nil {
			stop()
			t.Errorf("startClientChannelTelemetry(GOOGLE_SDK_GO_LOGGING_LEVEL=warn) returned non-nil stop func, want nil")
		}
		if got := conn.GetState(); got != connectivity.Idle {
			t.Errorf("conn.GetState() with GOOGLE_SDK_GO_LOGGING_LEVEL=warn = %v, want %v", got, connectivity.Idle)
		}
	})
}

func TestChannelTelemetry_NotRateLimited(t *testing.T) {
	addr := startTestGRPCServer(t)
	h := newRecordingLogHandler(slog.LevelInfo)
	opts := localGRPCClientOptions(t, addr, h)

	// Construct two clients back to back well within 1s to verify telemetry is
	// not gated behind auth/grpctransport's 1-second rate limiter.
	c1, err := NewGRPCClient(t.Context(), opts...)
	if err != nil {
		t.Fatalf("first NewGRPCClient(%q) failed: %v", addr, err)
	}
	defer c1.Close()

	c2, err := NewGRPCClient(t.Context(), opts...)
	if err != nil {
		t.Fatalf("second NewGRPCClient(%q) failed: %v", addr, err)
	}
	defer c2.Close()

	recs := waitForLogCount(t, h, 4, 5*time.Second)
	var configured, ready int
	for _, r := range recs {
		switch r.msg {
		case "Configured gRPC transport.":
			configured++
		case "gRPC channel [0] is ready.":
			ready++
		}
	}
	if configured != 2 || ready != 2 {
		t.Errorf("two NewGRPCClient(%q) calls logged (configured=%d, ready=%d), want (2, 2)", addr, configured, ready)
	}
}

func TestChannelTelemetry_NewClientErrorNoWatcher(t *testing.T) {
	unsetEnvForTest(t, "STORAGE_EMULATOR_HOST_GRPC")
	h := newRecordingLogHandler(slog.LevelInfo)

	// Passing an invalid credential file forces gapic.NewClient to fail before
	// telemetry starts.
	_, err := NewGRPCClient(t.Context(),
		option.WithCredentialsFile("/nonexistent/path/to/creds.json"),
		option.WithLogger(slog.New(h)),
		WithDisabledClientMetrics(),
	)
	if err == nil {
		t.Fatal("NewGRPCClient(nonexistent credentials) succeeded, want error")
	}
	if recs := h.snapshot(); len(recs) != 0 {
		t.Errorf("failed NewGRPCClient emitted %d log record(s), want 0: %+v", len(recs), recs)
	}
}

func TestChannelTelemetry_PooledClientReady(t *testing.T) {
	addr := startTestGRPCServer(t)
	h := newRecordingLogHandler(slog.LevelInfo)

	client, err := NewGRPCClient(t.Context(), localGRPCClientOptions(t, addr, h, option.WithGRPCConnectionPool(2))...)
	if err != nil {
		t.Fatalf("NewGRPCClient(%q, pool=2) failed: %v", addr, err)
	}
	defer client.Close()

	recs := waitForLogCount(t, h, 2, 5*time.Second)
	if recs[0].msg != "Configured gRPC transport." || recs[1].msg != "gRPC channel [0] is ready." {
		t.Errorf("NewGRPCClient(%q, pool=2) messages = (%q, %q), want (%q, %q)", addr, recs[0].msg, recs[1].msg, "Configured gRPC transport.", "gRPC channel [0] is ready.")
	}
}

func TestNewGRPCStorageClient_SkipsDiagnosticForC2PTarget(t *testing.T) {
	unsetEnvForTest(t, "STORAGE_EMULATOR_HOST_GRPC")
	addr := startTestGRPCServer(t)

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "c2p_target_skips_diagnostic",
			target: "google-c2p:///storage.googleapis.com",
			want:   "",
		},
		{
			name:   "non_c2p_target_runs_diagnostic",
			target: addr,
			want:   reasonCustomGRPCConn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := grpc.NewClient(tc.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("grpc.NewClient(%q) failed: %v", tc.target, err)
			}
			defer conn.Close()

			c, err := newGRPCStorageClient(t.Context(),
				withClientOptions(
					option.WithGRPCConn(conn),
					WithDisabledClientMetrics(),
				),
			)
			if err != nil {
				t.Fatalf("newGRPCStorageClient(%q) failed: %v", tc.target, err)
			}
			defer c.Close()

			if c.dpDiag != tc.want {
				t.Errorf("newGRPCStorageClient(%q).dpDiag = %q, want %q", tc.target, c.dpDiag, tc.want)
			}
		})
	}
}
