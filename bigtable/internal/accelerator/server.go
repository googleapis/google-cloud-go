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

// Package accelerator implements the in-process Bigtable accelerator daemon.
// It hosts the gRPC server-side scaffolding — a UDS listener, lifecycle
// watchdogs (stdin EOF and parent-PID reparent), and proxy interceptors that
// forward every RPC through a Channel — alongside the Channel itself, which
// backs those RPCs with internal/session. The interceptor implementations
// live in interceptors.go alongside bigtableServerStub.
package accelerator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
)

// defaultHandshakeTimeout bounds how long Start waits for the parent process to
// write the auth secret to stdin. Without it, a parent that never writes would
// leave the daemon blocked in readSecret forever — never binding the socket and
// never exiting.
const defaultHandshakeTimeout = 5 * time.Second

// Server manages the lifecycle of the local gRPC UDS server.
type Server struct {
	udsPath      string
	grpcServer   *grpc.Server
	service      *bigtableServerStub
	listener     net.Listener
	shutdownChan chan struct{}
	stopOnce     sync.Once
	stdinReader  io.Reader     // stdin source; nil disables the stdin watchdog. Defaults to os.Stdin; override with WithStdinReader.
	stdinBuf     *bufio.Reader // wraps stdinReader once in Start(); shared by readSecret + monitorStdin
	authSecret   string
	channel      *Channel

	handshakeTimeout time.Duration // bounds readSecret; defaults to defaultHandshakeTimeout, override with WithHandshakeTimeout.
}

// ServerOption configures optional Server behavior. Options are applied in
// order by NewServer; later options override earlier ones for the same field.
type ServerOption func(*Server)

// WithStdinReader overrides the reader the server treats as stdin. The shipped
// daemon uses the os.Stdin default; tests inject a pipe, or pass nil to disable
// the stdin watchdog. Providing it as a construction-time option keeps the
// underlying field immutable after NewServer returns.
func WithStdinReader(r io.Reader) ServerOption {
	return func(s *Server) { s.stdinReader = r }
}

// WithHandshakeTimeout overrides how long Start waits for the parent to write
// the auth secret to stdin before failing. Primarily for tests that want a short
// deadline; the shipped daemon uses defaultHandshakeTimeout.
func WithHandshakeTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.handshakeTimeout = d }
}

// NewServer creates a new Server instance.
func NewServer(udsPath string, channel *Channel, opts ...ServerOption) *Server {
	s := &Server{
		udsPath:          udsPath,
		shutdownChan:     make(chan struct{}),
		stdinReader:      os.Stdin,
		channel:          channel,
		service:          newBigtableServerStub(),
		handshakeTimeout: defaultHandshakeTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start boots the gRPC server on the Unix Domain Socket asynchronously.
func (s *Server) Start() error {
	// Read the auth secret from stdin BEFORE binding — the parent process writes
	// the secret before the daemon starts serving, so it is always present on the
	// pipe.
	if s.stdinReader != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.handshakeTimeout)
		defer cancel()
		if err := s.readSecret(ctx); err != nil {
			return err
		}
	}

	// Clear any stale socket left by a previous instance so the bind below
	// succeeds. Safe under the daemon's 1:1 pairing with its parent: no live
	// sibling shares this path.
	_ = os.Remove(s.udsPath)

	// Bind under a private umask so the socket is never world-connectable, even
	// for the instant between creation and the Chmod below. net.Listen -> bind(2)
	// creates the socket file subject to the process umask; the common default
	// (022) would leave it group/other accessible during that window, letting any
	// local process connect before we lock it down. umask is process-global, so
	// we narrow it only around the bind and restore it immediately after.
	oldMask := syscall.Umask(0077)
	l, err := net.Listen("unix", s.udsPath)
	syscall.Umask(oldMask)
	if err != nil {
		log.Printf("failed to start accelerator: %v", err)
		return err
	}
	s.listener = l

	// Tighten to 0600, dropping the owner-execute bit the 0077 umask leaves at
	// 0700. The daemon is paired 1:1 with its parent process, so no other user
	// should be able to issue RPCs against it. Because the umask above already
	// guaranteed the socket was never group/other accessible, this Chmod is
	// race-free hardening rather than the sole line of defense.
	if err := os.Chmod(s.udsPath, 0600); err != nil {
		_ = l.Close()
		_ = os.Remove(s.udsPath)
		log.Printf("failed to chmod accelerator socket: %v", err)
		return err
	}

	// Chain the proxy interceptors, which route each RPC through the Channel.
	// The auth interceptors are prepended only when a secret was read from stdin
	// — the sole secret-less path is test mode (WithStdinReader(nil)), where auth
	// is intentionally disabled. Gating the wiring here (rather than relying on a
	// no-op inside the interceptor) keeps "auth is off" explicit and off the RPC
	// hot path.
	unaryInterceptors := []grpc.UnaryServerInterceptor{proxyUnaryInterceptor(s.channel)}
	streamInterceptors := []grpc.StreamServerInterceptor{proxyStreamInterceptor(s.channel)}
	if s.authSecret != "" {
		unaryInterceptors = append([]grpc.UnaryServerInterceptor{authUnaryInterceptor(s.authSecret)}, unaryInterceptors...)
		streamInterceptors = append([]grpc.StreamServerInterceptor{authStreamInterceptor(s.authSecret)}, streamInterceptors...)
	}
	s.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// Register the empty bigtableServerStub. gRPC requires a typed
	// server implementation to be registered before it will accept RPCs for
	// the service, but the interceptors above short-circuit every method on
	// the stub before it is reached — the stub exists solely to satisfy
	// gRPC's registration contract.
	btpb.RegisterBigtableServer(s.grpcServer, s.service)

	go func() {
		// Serve returns ErrServerStopped on a clean GracefulStop/Stop; anything
		// else means the server fell over after a successful Listen (e.g. the
		// listener was yanked). Log it so the daemon doesn't die silently while
		// Start() has already reported success.
		if err := s.grpcServer.Serve(s.listener); err != nil && err != grpc.ErrServerStopped {
			log.Printf("accelerator: gRPC server exited with error: %v", err)
		}
		// Whatever made Serve return, the server is no longer accepting RPCs.
		// Drive a full Stop() so the listener, channel, and socket are torn down
		// and ShutdownChan is closed — otherwise a self-inflicted Serve failure
		// would leave a half-dead daemon behind. Stop() is idempotent, so the
		// clean-shutdown path (Stop() already running) is a no-op.
		s.Stop()
	}()

	go s.monitorStdin()
	go s.monitorParentPid()

	return nil
}

// Stop gracefully stops the server, cleans up the UDS socket, and notifies
// monitors.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		close(s.shutdownChan)

		if s.grpcServer != nil {
			// GracefulStop blocks until all in-flight RPCs finish. A wedged or
			// slow-draining RPC would otherwise hang shutdown forever, which is
			// especially harmful on the watchdog paths (stdin EOF, parent-PID
			// reparent) whose whole purpose is to guarantee the daemon exits.
			// Bound the graceful drain and fall back to a hard Stop().
			stopped := make(chan struct{})
			go func() {
				s.grpcServer.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				s.grpcServer.Stop()
			}
		}

		if s.listener != nil {
			_ = s.listener.Close()
		}

		// Close the channel after gRPC has drained — backends owned by the
		// channel's resource manager are torn down here, not while in-flight
		// RPCs may still be using them.
		if s.channel != nil {
			_ = s.channel.Close()
		}

		_ = os.Remove(s.udsPath)
	})
}

// readSecret reads the auth secret written by the parent process to stdin
// before the daemon binds. It wraps stdinReader once in a bufio.Reader
// (stored as s.stdinBuf) so monitorStdin can continue consuming the same
// buffered stream without losing bytes.
//
// An empty or whitespace-only secret is rejected so the daemon fails closed:
// serving with an empty authSecret would disable the auth interceptor (see
// Server.Start), so we refuse to start rather than accept unauthenticated
// callers. This guarantees the shipped binary — which always feeds os.Stdin —
// either has a real secret or never binds the socket.
//
// The read is bounded by ctx: ReadString blocks until a newline or EOF, so a
// parent that opens the pipe but never writes would otherwise wedge Start
// forever. On timeout we return ctx.Err(); the read goroutine stays parked on
// stdin until the process exits, which is fine because Start returns an error
// and the daemon never serves.
func (s *Server) readSecret(ctx context.Context) error {
	s.stdinBuf = bufio.NewReader(s.stdinReader)

	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := s.stdinBuf.ReadString('\n')
		ch <- readResult{line, err}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("accelerator: timed out reading auth secret from stdin: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			return fmt.Errorf("accelerator: failed to read auth secret from stdin: %w", res.err)
		}
		secret := strings.TrimSpace(res.line)
		if secret == "" {
			return fmt.Errorf("accelerator: received empty auth secret from stdin; refusing to serve unauthenticated")
		}
		s.authSecret = secret
		return nil
	}
}

// monitorStdin monitors standard input for an EOF signal. Tests that don't
// want this watchdog pass WithStdinReader(nil).
func (s *Server) monitorStdin() {
	if s.stdinReader == nil {
		return
	}
	// Use the buffered reader created by readSecret so bytes already consumed
	// into the buffer are not lost (in practice stdin only carries the secret
	// line followed by EOF, but using the same reader is correct).
	var r io.Reader
	if s.stdinBuf != nil {
		r = s.stdinBuf
	} else {
		r = s.stdinReader
	}
	buf := make([]byte, 1)
	for {
		select {
		case <-s.shutdownChan:
			return
		default:
			_, err := r.Read(buf)
			if err != nil {
				s.Stop()
				return
			}
		}
	}
}

// monitorParentPid stops the daemon if its parent PID changes, which happens
// when the original parent dies and the process is reparented (to init or a
// subreaper). If the daemon is already parented to init at startup (e.g. under
// a process supervisor or in a container), there is no original parent to
// outlive, so the watchdog is disabled.
func (s *Server) monitorParentPid() {
	initialPpid := os.Getppid()
	if initialPpid == 1 {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownChan:
			return
		case <-ticker.C:
			if os.Getppid() != initialPpid {
				s.Stop()
				return
			}
		}
	}
}

// ShutdownChan returns a read-only channel that is closed when the server
// shuts down. Main entrypoints block on this to know when to exit.
func (s *Server) ShutdownChan() <-chan struct{} {
	return s.shutdownChan
}
