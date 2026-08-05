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
// It hosts the gRPC server-side scaffolding — a UDS listener and proxy
// interceptors that forward every RPC through a Channel — alongside the
// Channel itself, which backs those RPCs with internal/session. The
// interceptor implementations live in interceptors.go alongside
// bigtableServerStub.
package accelerator

import (
	"io"
	"log"
	"net"
	"os"
	"sync"
	"syscall"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
)

// Server manages the lifecycle of the local gRPC UDS server.
type Server struct {
	udsPath      string
	grpcServer   *grpc.Server
	service      *bigtableServerStub
	listener     net.Listener
	shutdownChan chan struct{}
	stopOnce     sync.Once
	stdinReader  io.Reader // stdin source; nil disables the stdin watchdog. Defaults to os.Stdin; override with WithStdinReader.
	channel      *Channel
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

// NewServer creates a new Server instance.
func NewServer(udsPath string, channel *Channel, opts ...ServerOption) *Server {
	s := &Server{
		udsPath:      udsPath,
		shutdownChan: make(chan struct{}),
		stdinReader:  os.Stdin,
		channel:      channel,
		service:      newBigtableServerStub(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start boots the gRPC server on the Unix Domain Socket asynchronously.
func (s *Server) Start() error {
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

	// Proxy interceptors route each RPC through the Channel.
	s.grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(proxyUnaryInterceptor(s.channel)),
		grpc.StreamInterceptor(proxyStreamInterceptor(s.channel)),
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

	return nil
}

// Stop gracefully stops the server, cleans up the UDS socket, and notifies
// monitors.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		close(s.shutdownChan)

		if s.grpcServer != nil {
			s.grpcServer.GracefulStop()
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

// ShutdownChan returns a read-only channel that is closed when the server
// shuts down. Main entrypoints block on this to know when to exit.
func (s *Server) ShutdownChan() <-chan struct{} {
	return s.shutdownChan
}
