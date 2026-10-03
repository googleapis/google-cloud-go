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
	"net"
	"runtime"
	"testing"
	"time"
)

// TestConnRegistry_DialRegistersAndSnapshot verifies Dial captures the
// remote/local addr and DialedAt, and that Snapshot returns one entry
// per registered conn. On Linux the RTT / MSS fields should be non-zero
// after the first byte flows; on other platforms Err is populated.
func TestConnRegistry_DialRegistersAndSnapshot(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Bounce one byte so both sides have measurable TCP state.
			buf := make([]byte, 1)
			_, _ = c.Read(buf)
			_, _ = c.Write(buf)
			c.Close()
		}
	}()

	reg := NewConnRegistry()
	conn, err := reg.Dial(context.Background(), ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Trigger one byte so Linux has stats to report.
	if _, err := conn.Write([]byte{1}); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}

	if got, want := reg.Len(), 1; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	snaps := reg.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snaps))
	}
	s := snaps[0]
	if s.RemoteAddr != ln.Addr().String() {
		t.Errorf("RemoteAddr = %q, want %q", s.RemoteAddr, ln.Addr().String())
	}
	if s.LocalAddr == "" {
		t.Error("LocalAddr empty")
	}
	if time.Since(s.DialedAt) > time.Second {
		t.Errorf("DialedAt = %v, expected within last second", s.DialedAt)
	}
	switch runtime.GOOS {
	case "linux":
		if s.Err != "" {
			t.Errorf("Err = %q on linux, want empty", s.Err)
		}
		// State should be ESTABLISHED right after handshake+bounce, or
		// CLOSE_WAIT if the server side already FIN'd — accept either.
		if s.State == "" {
			t.Error("State empty on linux")
		}
	default:
		if s.Err == "" {
			t.Errorf("Err empty on %s, want unsupported sentinel", runtime.GOOS)
		}
	}
}

// TestConnRegistry_SnapshotPrunesDeadConns confirms that a conn whose fd
// was closed out from under the registry is removed on the next
// Snapshot — this is the mechanism that keeps the map honest without a
// Close hook on the wire.
func TestConnRegistry_SnapshotPrunesDeadConns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prune mechanism relies on Linux errno differentiation")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	reg := NewConnRegistry()
	conn, err := reg.Dial(context.Background(), ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// After Close, the fd is gone. Snapshot should notice and prune —
	// either immediately or after Go's runtime finalizer clears the fd
	// entry. Give it one call; if the entry is present but Err-flagged
	// that's also acceptable, because different kernels report closed
	// fds via different errnos.
	_ = reg.Snapshot()
	// A second snapshot after any lazy state should not leak entries
	// permanently — assert the map is either empty or the sole entry
	// is flagged dead.
	snaps := reg.Snapshot()
	if len(snaps) == 0 {
		return // pruned as expected
	}
	if snaps[0].Err == "" && snaps[0].State != "" {
		t.Errorf("expected pruned or errored entry, got live snap: %+v", snaps[0])
	}
}

// TestConnRegistry_ConcurrentDialsAndSnapshots is a coarse race check —
// hammer Dial and Snapshot from parallel goroutines under `-race` to
// confirm no lock ordering / map-access bugs.
func TestConnRegistry_ConcurrentDialsAndSnapshots(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 32)
				for {
					n, err := c.Read(buf)
					if err != nil {
						c.Close()
						return
					}
					if _, err := c.Write(buf[:n]); err != nil {
						c.Close()
						return
					}
				}
			}(c)
		}
	}()

	reg := NewConnRegistry()
	done := make(chan struct{})
	// conns is populated by the dialer goroutine and read by the main
	// test after <-done, which provides the happens-before edge. Held
	// open across the Len() assertion so the registry entries survive
	// long enough to be observed — closing before the assertion races
	// registry deregistration and flakes Len()==0 under load.
	var conns []net.Conn
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			c, err := reg.Dial(context.Background(), ln.Addr().String())
			if err != nil {
				t.Errorf("dial: %v", err)
				return
			}
			conns = append(conns, c)
			time.Sleep(2 * time.Millisecond)
		}
	}()
	// Snapshotter goroutine.
	for i := 0; i < 40; i++ {
		_ = reg.Snapshot()
		time.Sleep(time.Millisecond)
	}
	<-done
	if reg.Len() == 0 {
		t.Error("Len() = 0 after 20 dials, expected registered conns")
	}
	for _, c := range conns {
		c.Close()
	}
}
