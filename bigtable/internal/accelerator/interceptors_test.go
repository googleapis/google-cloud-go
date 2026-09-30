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
	"context"
	"os"
	"path/filepath"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newServerWithSecret starts an Server wired to accept exactly
// `secret` on every RPC. It returns the server and the write end of the stdin
// pipe — callers must defer both server.Stop() and stdinW.Close() (in that
// order). Keeping stdinW open prevents monitorStdin from triggering a premature
// shutdown while tests are running.
func newServerWithSecret(t *testing.T, udsPath, secret string) (*Server, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	go func() { w.WriteString(secret + "\n") }()
	channel := &Channel{}
	server := NewServer(udsPath, channel, WithStdinReader(r))
	if err := server.Start(); err != nil {
		r.Close()
		w.Close()
		t.Fatalf("failed to start server: %v", err)
	}
	return server, w
}

func TestServer_AuthInterceptor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-auth-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	const secret = "correct-secret-token"
	server, stdinW := newServerWithSecret(t, udsPath, secret)
	defer stdinW.Close()
	defer server.Stop()

	conn, err := grpc.NewClient("unix://"+udsPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// PingAndWarm is a real Bigtable RPC (so the proxy resolves its proto types)
	// that the channel does not proxy, so it returns Unimplemented. That makes
	// Unimplemented a reliable stand-in for "auth passed": the request reached
	// the channel instead of being rejected by the auth interceptor.
	req := &btpb.PingAndWarmRequest{}
	resp := &btpb.PingAndWarmResponse{}

	// A rejected handshake surfaces as Internal (not Unauthenticated), so it is
	// not mistaken for a Bigtable credential failure by the calling client.
	t.Run("MissingToken", func(t *testing.T) {
		err := conn.Invoke(context.Background(), "/google.bigtable.v2.Bigtable/PingAndWarm", req, resp)
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("expected Internal, got %v", got)
		}
	})

	t.Run("WrongToken", func(t *testing.T) {
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-accelerator-token", "wrong-token"))
		err := conn.Invoke(ctx, "/google.bigtable.v2.Bigtable/PingAndWarm", req, resp)
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("expected Internal, got %v", got)
		}
	})

	t.Run("CorrectToken", func(t *testing.T) {
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-accelerator-token", secret))
		err := conn.Invoke(ctx, "/google.bigtable.v2.Bigtable/PingAndWarm", req, resp)
		// Auth passes and falls through to the zero-value channel, which returns
		// Unimplemented for PingAndWarm — proving the auth layer did not
		// short-circuit with Internal.
		if got := status.Code(err); got != codes.Unimplemented {
			t.Errorf("expected auth to pass (Unimplemented from channel), got %v", got)
		}
	})
}

func TestServer_EndToEnd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "accelerator-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	// Zero-value channel is fine for these tests — Invoke returns
	// Unimplemented for non-MutateRow methods without touching the channel's
	// internals, and the stream interceptor's NewStream call likewise hits
	// the channel's default-Unimplemented case for non-ReadRows methods.
	channel := &Channel{}

	server := NewServer(udsPath, channel, WithStdinReader(nil))
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start accelerator server: %v", err)
	}
	defer server.Stop()

	conn, err := grpc.NewClient("unix://"+udsPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect to UDS server: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Unary RPC that Channel.Invoke does not implement → falls
	// through to the channel's default Unimplemented case. PingAndWarm is a
	// connection-management RPC the channel does not proxy.
	t.Run("UnimplementedUnaryMethod", func(t *testing.T) {
		req := &btpb.PingAndWarmRequest{}
		resp := &btpb.PingAndWarmResponse{}
		err := conn.Invoke(ctx, "/google.bigtable.v2.Bigtable/PingAndWarm", req, resp)
		if err == nil {
			t.Fatal("expected error for unimplemented method, got nil")
		}
		if got := status.Code(err); got != codes.Unimplemented {
			t.Errorf("expected Unimplemented, got %v", got)
		}
	})

	// Server-streaming RPC that Channel.NewStream does not
	// implement → channel returns Unimplemented from NewStream, the stream
	// interceptor propagates it.
	t.Run("UnimplementedStreamingMethod", func(t *testing.T) {
		client := btpb.NewBigtableClient(conn)
		stream, err := client.SampleRowKeys(ctx, &btpb.SampleRowKeysRequest{})
		if err != nil {
			if got := status.Code(err); got != codes.Unimplemented {
				t.Fatalf("expected Unimplemented on NewStream, got %v", got)
			}
			return
		}
		_, err = stream.Recv()
		if got := status.Code(err); got != codes.Unimplemented {
			t.Errorf("expected Unimplemented on Recv, got %v", got)
		}
	})
}
