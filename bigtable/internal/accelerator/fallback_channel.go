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
	"errors"
	"io"
	"sync/atomic"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// channelConn is implemented by both *Channel (session-only) and
// *FallbackChannel (session + classic). Server.channel uses this interface so
// either can be passed to NewServer without a code change at the call site.
type channelConn interface {
	grpc.ClientConnInterface
	Close() error
}

var _ channelConn = (*Channel)(nil)
var _ channelConn = (*FallbackChannel)(nil)

// FallbackChannel wraps a session Channel with a bigtable.Client classic
// fallback. On codes.Unimplemented from the session backend (indicating the
// instance does not support session serving) it transparently retries the RPC
// via the classic path. A sticky atomic bool prevents re-trying session after
// the first backend Unimplemented so subsequent RPCs skip session entirely.
//
// The sticky flag is channel-scoped, not table-scoped: Unimplemented from the
// session backend means the instance lacks session serving for all tables, so
// routing everything classic after the first trip is correct.
//
// UNIMPLEMENTED that originates inside the daemon (unsupported request shapes
// from validateSingleRowReadRequest, or methods the daemon does not serve) is
// NOT caught here — it propagates to the caller as-is, letting the Python
// client's per-method breaker handle it. Only backend-originated
// Unimplemented (from RecvMsg, after a valid session dispatch) trips the
// sticky flag and redirects to classic.
type FallbackChannel struct {
	session *Channel
	classic *classicFallback
	tripped atomic.Bool
}

// NewFallbackChannel constructs a FallbackChannel. It dials a new
// bigtable.Client (classic path) in addition to the caller-supplied session
// Channel. project, instance, appProfile, and opts should match those used
// to construct the session Channel so both paths dial the same Bigtable scope
// with the same credentials.
func NewFallbackChannel(ctx context.Context, session *Channel, project, instance, appProfile string, opts ...option.ClientOption) (*FallbackChannel, error) {
	cf, err := newClassicFallback(ctx, project, instance, appProfile, opts...)
	if err != nil {
		return nil, err
	}
	return &FallbackChannel{session: session, classic: cf}, nil
}

// Invoke implements grpc.ClientConnInterface for unary RPCs (MutateRow).
// Session is tried first unless the channel is already tripped; on
// codes.Unimplemented the sticky flag is set and classic is used instead.
func (fc *FallbackChannel) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	if method != v2pb.Bigtable_MutateRow_FullMethodName {
		return status.Errorf(codes.Unimplemented, "accelerator fallback: unary method %s not supported", method)
	}
	req, ok := args.(*v2pb.MutateRowRequest)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator fallback: unexpected request type %T for MutateRow", args)
	}

	if !fc.tripped.Load() {
		err := fc.session.Invoke(ctx, method, args, reply, opts...)
		if err == nil {
			return nil
		}
		if status.Code(err) != codes.Unimplemented {
			return err
		}
		fc.tripped.Store(true)
	}

	resource, err := adapters.DefaultMutateRowRequestAdapter.ExtractResource(req)
	if err != nil {
		return err
	}
	return fc.classic.MutateRow(ctx, resource, req)
}

// NewStream implements grpc.ClientConnInterface for streaming RPCs (ReadRows).
// Returns a fallbackReadRowsStream that tries session first and falls back to
// classic on codes.Unimplemented from the session backend.
func (fc *FallbackChannel) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if method != v2pb.Bigtable_ReadRows_FullMethodName {
		return nil, status.Errorf(codes.Unimplemented, "accelerator fallback: streaming method %s not supported", method)
	}

	if fc.tripped.Load() {
		return &fallbackReadRowsStream{ctx: ctx, fc: fc}, nil
	}
	// Channel.NewStream for ReadRows always succeeds — it returns a
	// readRowsClientStream immediately without contacting the backend.
	// Unimplemented only surfaces later from RecvMsg when the backend rejects
	// the session dispatch. Propagate any unexpected error as-is.
	inner, err := fc.session.NewStream(ctx, desc, method, opts...)
	if err != nil {
		return nil, err
	}
	return &fallbackReadRowsStream{ctx: ctx, fc: fc, inner: inner}, nil
}

// Close shuts down both the session Channel and the classic fallback.
func (fc *FallbackChannel) Close() error {
	return errors.Join(fc.session.Close(), fc.classic.Close())
}

// fallbackReadRowsStream implements grpc.ClientStream for the ReadRows RPC.
// It tries the session stream (inner) first and transparently retries via the
// classic bigtable.Client when the session backend returns Unimplemented.
//
// Concurrency contract: same as readRowsClientStream — drive sequentially from
// a single goroutine. SendMsg must be called exactly once before RecvMsg.
//
// Two kinds of UNIMPLEMENTED are distinguished by where they appear:
//
//   - From inner.SendMsg: the request shape is not supported by session (e.g. a
//     range scan or reversed read). This error propagates to the caller as-is;
//     the sticky breaker is NOT set. The caller (Python client) handles it with
//     its per-method native fallback.
//
//   - From inner.RecvMsg: the session backend returned Unimplemented, meaning
//     the instance does not have session serving enabled. The sticky breaker is
//     set and the same request is immediately retried via classic.
type fallbackReadRowsStream struct {
	ctx context.Context
	fc  *FallbackChannel

	// inner is nil when fc was already tripped at NewStream time or when
	// NewStream itself returned Unimplemented; otherwise it is the session stream.
	inner grpc.ClientStream

	// req is buffered from SendMsg for use in the classic fallback path.
	req  *v2pb.ReadRowsRequest
	sent bool

	// Terminal state.
	done        bool
	terminalErr error
}

func (s *fallbackReadRowsStream) Header() (gmetadata.MD, error) {
	if s.inner != nil {
		return s.inner.Header()
	}
	return nil, nil
}
func (s *fallbackReadRowsStream) Trailer() gmetadata.MD {
	if s.inner != nil {
		return s.inner.Trailer()
	}
	return nil
}
func (s *fallbackReadRowsStream) CloseSend() error {
	if s.inner != nil {
		return s.inner.CloseSend()
	}
	return nil
}
func (s *fallbackReadRowsStream) Context() context.Context { return s.ctx }

// SendMsg buffers the request and forwards it to the session stream (if any).
// An Unimplemented from the session stream means the request shape is not
// supported — that error propagates directly; the sticky breaker is not set.
func (s *fallbackReadRowsStream) SendMsg(m any) error {
	if s.sent {
		return status.Error(codes.Internal, "accelerator fallback: ReadRows SendMsg called more than once")
	}
	req, ok := m.(*v2pb.ReadRowsRequest)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator fallback: unexpected request type %T for ReadRows", m)
	}
	s.req = req
	s.sent = true

	if s.inner == nil {
		// Channel already tripped — validate shape before going classic.
		// Without this call, a multi-row or range-scan request would reach
		// classicFallback.ReadRow and return codes.Internal rather than
		// codes.Unimplemented, breaking the caller's per-method native fallback.
		return validateSingleRowReadRequest(req)
	}
	// Forward to session. Any error — including Unimplemented for unsupported
	// shapes — propagates to the caller. Do NOT catch Unimplemented here: a
	// shape-based Unimplemented is per-request, not an instance-level condition,
	// and the caller's native fallback handles it.
	return s.inner.SendMsg(req)
}

// RecvMsg drives the session stream on the first call; if the session backend
// returns Unimplemented it trips the sticky flag and retries via classic.
func (s *fallbackReadRowsStream) RecvMsg(m any) error {
	if s.done {
		return s.terminalErr
	}
	if !s.sent {
		return status.Error(codes.Internal, "accelerator fallback: ReadRows RecvMsg called before SendMsg")
	}
	resp, ok := m.(*v2pb.ReadRowsResponse)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator fallback: unexpected reply type %T for ReadRows", m)
	}

	if s.inner != nil {
		err := s.inner.RecvMsg(resp)
		switch {
		case err == nil:
			// Successful session response. This includes the missing-row case:
			// readRowsClientStream always returns nil on its first RecvMsg call
			// (even when the row was not found), setting resp to an empty
			// ReadRowsResponse with no chunks. io.EOF is only returned on a
			// subsequent call, which is guarded by s.done above and never reached.
			s.done = true
			s.terminalErr = io.EOF
			return nil
		case status.Code(err) == codes.Unimplemented:
			// Backend-originated Unimplemented: instance lacks session serving.
			// Trip the sticky breaker and fall through to classic below.
			s.fc.tripped.Store(true)
			s.inner = nil
		default:
			return s.terminate(err)
		}
	}

	// Classic path: bigtable.Client.ReadRow with full client retry logic.
	resource, err := adapters.DefaultReadRowRequestAdapter.ExtractResource(s.req)
	if err != nil {
		return s.terminate(err)
	}
	classicResp, err := s.fc.classic.ReadRow(s.ctx, resource, s.req)
	if err != nil {
		return s.terminate(err)
	}
	proto.Reset(resp)
	if classicResp != nil {
		proto.Merge(resp, classicResp)
	}
	s.done = true
	s.terminalErr = io.EOF
	return nil
}

func (s *fallbackReadRowsStream) terminate(err error) error {
	s.done = true
	s.terminalErr = err
	return err
}
