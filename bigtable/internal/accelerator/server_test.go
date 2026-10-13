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

package accelerator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServer_Permissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-perm-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	// Zero-value Channel is sufficient here: this test only
	// exercises Start/Stop; no RPCs are issued, so the channel's
	// session.Client is never dereferenced. Close handles a nil sc.
	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(nil))
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	info, err := os.Stat(udsPath)
	if err != nil {
		t.Fatalf("failed to stat socket file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("expected socket permissions 0600, got %o", mode)
	}
}

func TestServer_LifecycleStdinEOF(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-lifecycle-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Write the handshake secret before Start() blocks on ReadString.
	go func() {
		w.WriteString("test-handshake-secret\n")
	}()

	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(r)) // inject the pipe instead of os.Stdin
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close write pipe: %v", err)
	}

	select {
	case <-server.shutdownChan:
		// success
	case <-time.After(3 * time.Second):
		t.Error("server failed to shut down within timeout after stdin EOF")
	}
}

func TestServer_ReadSecret(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-secret-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	const want = "my-test-secret-token"
	go func() { w.WriteString(want + "\n") }()

	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(r))
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	if got := server.authSecret; got != want {
		t.Errorf("authSecret = %q, want %q", got, want)
	}
}

func TestServer_ReadSecretEOFBeforeNewline(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-badsecret-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Close write end immediately — daemon gets EOF before reading a newline.
	w.Close()

	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(r))
	if err := server.Start(); err == nil {
		server.Stop()
		t.Error("expected Start() to fail when stdin closes before secret newline, but got nil")
	}
}

func TestServer_ReadSecretEmpty(t *testing.T) {
	// A blank secret line must fail the daemon closed rather than serving with
	// auth disabled.
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"EmptyLine", "\n"},
		{"WhitespaceOnly", "   \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "accelerator-emptysecret-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)
			udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("failed to create pipe: %v", err)
			}
			defer r.Close()
			go func() {
				w.WriteString(tc.payload)
				w.Close()
			}()

			channel := &Channel{}
			server := NewServer(udsPath, channel, WithStdinReader(r))
			if err := server.Start(); err == nil {
				server.Stop()
				t.Error("expected Start() to fail on empty auth secret, but got nil")
			}
		})
	}
}

func TestServer_ReadSecretTimeout(t *testing.T) {
	// Parent opens the pipe but never writes the secret. Start must fail on the
	// handshake deadline rather than blocking forever in readSecret.
	tmpDir, err := os.MkdirTemp("", "accelerator-secrettimeout-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close() // keep the write end open so the read blocks (no EOF).

	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(r), WithHandshakeTimeout(50*time.Millisecond))

	done := make(chan error, 1)
	go func() { done <- server.Start() }()

	select {
	case err := <-done:
		if err == nil {
			server.Stop()
			t.Fatal("expected Start() to fail on handshake timeout, but got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return within 2s; handshake timeout not enforced")
	}
}

func TestServer_StartFailure(t *testing.T) {
	udsPath := "/nonexistent-dir-123456/bt_proxy.sock"
	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(nil))
	if err := server.Start(); err == nil {
		t.Error("expected error when starting server with nonexistent UDS path, but got nil")
	}
}
