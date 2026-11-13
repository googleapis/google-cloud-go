// Package udsthin is a pure-Go (CGO_ENABLED=0) thin client for the GCS Rust
// core running as a local gRPC server on a Unix Domain Socket. It exposes the
// same method shapes as cloud.google.com/go/storage:
//
//	client.Bucket(b).Object(o).NewReader(ctx) / NewRangeReader(ctx, off, len)
//	client.Bucket(b).Object(o).If(storage.Conditions{...}).NewWriter(ctx)
//	client.Bucket(b).Objects(ctx, &storage.Query{...}) -> Next()/PageInfo()
//	client.Bucket(b).Object(o).Open(ctx) -> ReadAt(ctx, p, off) / Close(ctx)
//
// File layout mirrors how a production thin client is organized:
//   - client.go        Client, Options, BucketHandle, ObjectHandle, error mapping
//   - reader.go        Reader (NewReader / NewRangeReader) and Opened (ReadAt)
//   - writer.go        Writer (buffered single-shot + streaming resumable/appendable)
//   - list.go          ObjectIterator (BucketHandle.Objects)
//   - session.go       Per-caller OAuth2 token-lending session & refresh loop
//   - sidecar.go       Per-user UDS discovery, dial-then-spawn reuse, lifecycle
//   - embed_*.go       Embedded sidecar binary extraction + memfd fallback
//   - rawcodec.go      Zero-copy protobuf receive codec aliasing gRPC buffers
//   - shm_*.go         Shared-memory (memfd + SCM_RIGHTS + mmap) read data plane
//   - observability.go Benchmark/test-only interceptors and sidecar Stats hooks
package udsthin

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	pb "poc/gothin/udsthin/gcscorepb"
)

// Transport selects the wire protocol the Rust core uses towards GCS.
type Transport = pb.Transport

const (
	JSON = pb.Transport_JSON
	GRPC = pb.Transport_GRPC
)

// Options configures a thin Client and its connection to the local Rust sidecar.
type Options struct {
	// Socket path ('@name' = Linux abstract). Empty = per-user default.
	Socket string
	// Path to the gcs-core-server binary. Empty = $GCS_CORE_SERVER or PATH lookup.
	ServerBinary string
	// Do not spawn; expect a server to be listening already.
	NoSpawn bool
	// Transport the core uses towards GCS for reads/list.
	Transport Transport
	// Transport for writes (0 = same as Transport). gRPC writes need appendable
	// objects, which regional buckets reject (UNIMPLEMENTED); use JSON there.
	WriteTransport Transport
	// Bytes per ReadChunk message requested from the server (0 = server default).
	ChunkSize uint32
	// Number of UDS connections to multiplex over.
	Conns int
	// Idle seconds before the sidecar exits on its own.
	IdleSecs int
	// Also expose a TCP listener (microbenchmarks only).
	TCP string
	// Per-caller credentials: if set, the client lends short-lived access
	// tokens from this source to a sidecar session (x-session metadata) and
	// refreshes them before expiry. If nil, the sidecar uses its own ADC.
	TokenSource oauth2.TokenSource
	// Prefer the binary embedded in this package over PATH/env lookup.
	UseEmbedded bool
	// Decode read chunks without copying (custom codec aliasing receive buffers).
	ZeroCopyDecode bool
}

// Client is a thin GCS client backed by a local Rust gRPC-over-UDS sidecar.
type Client struct {
	side    *Sidecar
	conns   []*grpc.ClientConn
	next    atomic.Uint32
	opts    Options
	session string
	stopRef chan struct{}
}

func dialUDS(sock string) func(context.Context, string) (net.Conn, error) {
	addr := sock
	if strings.HasPrefix(sock, "@") {
		addr = "\x00" + sock[1:]
	}
	return func(ctx context.Context, _ string) (net.Conn, error) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "unix", addr)
		if err != nil {
			return nil, err
		}
		if uc, ok := conn.(*net.UnixConn); ok {
			_ = uc.SetReadBuffer(8 << 20)
			_ = uc.SetWriteBuffer(8 << 20)
		}
		return conn, nil
	}
}

// NewClient connects to (or spawns) the local Rust sidecar and initializes a
// pool of UDS gRPC connections.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	if opts.Conns <= 0 {
		opts.Conns = 4
	}
	side, err := Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	c := &Client{side: side, opts: opts, stopRef: make(chan struct{})}
	for i := 0; i < opts.Conns; i++ {
		cc, err := grpc.NewClient("passthrough:///unix",
			grpc.WithContextDialer(dialUDS(side.Socket)),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(64<<20),
				grpc.MaxCallSendMsgSize(64<<20),
			),
			grpc.WithInitialWindowSize(64<<20),
			grpc.WithInitialConnWindowSize(256<<20),
			grpc.WithReadBufferSize(4<<20),
			grpc.WithWriteBufferSize(4<<20),
			grpc.WithChainUnaryInterceptor(c.unarySession, countUnary),
			grpc.WithChainStreamInterceptor(c.streamSession, countStream),
		)
		if err != nil {
			return nil, err
		}
		c.conns = append(c.conns, cc)
	}
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, cc := range c.conns {
		if _, err := pb.NewGcsCoreClient(cc).Ping(rctx, &pb.PingRequest{}); err != nil {
			return nil, fmt.Errorf("sidecar not reachable on %s: %w", side.Socket, err)
		}
	}
	if opts.TokenSource != nil {
		if err := c.startSession(ctx); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Close closes the client's UDS connections and tears down its credential
// session. The sidecar process stays alive until its idle timeout so other
// processes on the machine can reuse it.
func (c *Client) Close() error {
	close(c.stopRef)
	if c.session != "" {
		_, _ = c.stub().CloseSession(context.Background(), &pb.SessionRef{SessionId: c.session})
	}
	for _, cc := range c.conns {
		_ = cc.Close()
	}
	return nil
}

func (c *Client) writeTransport() Transport {
	if c.opts.WriteTransport != 0 {
		return c.opts.WriteTransport
	}
	return c.opts.Transport
}

func (c *Client) conn() *grpc.ClientConn {
	i := c.next.Add(1) % uint32(len(c.conns))
	return c.conns[i]
}

func (c *Client) stub() pb.GcsCoreClient { return pb.NewGcsCoreClient(c.conn()) }

// BucketHandle references a GCS bucket.
type BucketHandle struct {
	c    *Client
	name string
}

// Bucket returns a BucketHandle for the named bucket.
func (c *Client) Bucket(name string) *BucketHandle { return &BucketHandle{c: c, name: name} }

// ObjectHandle references a GCS object inside a bucket.
type ObjectHandle struct {
	b      *BucketHandle
	object string
	gen    int64
	conds  *storage.Conditions
}

// Object returns an ObjectHandle for the named object.
func (b *BucketHandle) Object(name string) *ObjectHandle { return &ObjectHandle{b: b, object: name} }

// Generation scopes the handle to a specific object generation.
func (o *ObjectHandle) Generation(gen int64) *ObjectHandle { o2 := *o; o2.gen = gen; return &o2 }

// If attaches preconditions (e.g. DoesNotExist, GenerationMatch) to the handle.
func (o *ObjectHandle) If(conds storage.Conditions) *ObjectHandle {
	o2 := *o
	o2.conds = &conds
	return &o2
}

func (o *ObjectHandle) ifGenMatch() *int64 {
	if o.conds == nil {
		return nil
	}
	if o.conds.DoesNotExist {
		var z int64
		return &z
	}
	if o.conds.GenerationMatch != 0 {
		g := o.conds.GenerationMatch
		return &g
	}
	return nil
}

// mapErr reconstructs the cloud.google.com/go/storage error contract from the
// sidecar's gRPC status code.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.NotFound:
		return storage.ErrObjectNotExist
	case codes.FailedPrecondition:
		return &googleapi.Error{Code: 412, Message: st.Message()}
	case codes.PermissionDenied:
		return &googleapi.Error{Code: 403, Message: st.Message()}
	case codes.Unauthenticated:
		return &googleapi.Error{Code: 401, Message: st.Message()}
	case codes.ResourceExhausted:
		return &googleapi.Error{Code: 429, Message: st.Message()}
	case codes.Unavailable:
		return &googleapi.Error{Code: 503, Message: st.Message()}
	}
	return err
}
