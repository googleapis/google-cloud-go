package udsthin

import (
	"context"
	"sync/atomic"

	"google.golang.org/grpc"
	pb "poc/gothin/udsthin/gcscorepb"
)

// This file isolates all POC benchmark, resource-accounting, and security-test
// hooks so that client.go, reader.go, writer.go, list.go, and session.go
// contain only the thin-client SDK implementation.

// RPCs counts UDS gRPC crossings (unary calls, stream opens, and client-stream
// data frames) via client interceptors without touching the SDK methods.
var RPCs atomic.Int64

func countUnary(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	switch method {
	case "/gcscore.v1.GcsCore/Ping", "/gcscore.v1.GcsCore/Stats", "/gcscore.v1.GcsCore/UpdateToken", "/gcscore.v1.GcsCore/CloseSession":
	default:
		RPCs.Add(1)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

func countStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	RPCs.Add(1)
	s, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil || !desc.ClientStreams {
		return s, err
	}
	return &countedClientStream{ClientStream: s}, nil
}

type countedClientStream struct {
	grpc.ClientStream
	sentHeader bool
}

func (s *countedClientStream) SendMsg(m any) error {
	if !s.sentHeader {
		s.sentHeader = true
	} else {
		RPCs.Add(1)
	}
	return s.ClientStream.SendMsg(m)
}

func (s *countedClientStream) CloseSend() error {
	RPCs.Add(1)
	return s.ClientStream.CloseSend()
}

// Sidecar returns the underlying process manager (used by benchmarks to report
// spawn latency, PID, and to shut down the sidecar after a test run).
func (c *Client) Sidecar() *Sidecar { return c.side }

// SidecarStats queries the sidecar's Stats RPC for user+sys CPU seconds,
// current RSS, peak RSS (VmHWM), and OS thread count so benchmarks can charge
// both processes to the workload. reset=true clears the sidecar's VmHWM.
func (c *Client) SidecarStats(ctx context.Context, reset bool) (cpuSecs, rssMiB, peakMiB float64, threads int, err error) {
	s, err := c.stub().Stats(ctx, &pb.StatsRequest{ResetPeakRss: reset})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return s.CpuUserSecs + s.CpuSysSecs,
		float64(s.RssBytes) / (1 << 20),
		float64(s.PeakRssBytes) / (1 << 20),
		int(s.Threads),
		nil
}

// SessionID exposes the active session token ID for security verification
// tests (e.g. attempting to replay a session ID from another PID).
func (c *Client) SessionID() string { return c.session }

// ForceSession overrides the client's session ID to simulate a cross-process
// session-hijack attempt in cmd/bench2_uds hijack.
func (c *Client) ForceSession(id string) { c.session = id }
