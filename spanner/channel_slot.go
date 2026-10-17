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

package spanner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errChannelSlotClosed = spannerErrorf(codes.Unavailable, "spanner: channel is closed")

// channelSlot is one logical gRPC channel of a Spanner channel pool. Both the
// static pool and the dynamic channel pool are arrays of slots, and session
// handles and transactions bind to a slot.
//
// A slot owns a direct connection, which uses DirectPath when DirectPath is
// enabled. With DirectPath fallback, it also owns a CloudPath connection that
// it dials on the first attempt routed to it, and every new attempt follows
// the pool-wide fallback window: a transaction bound to the slot keeps the
// slot, and its next RPC after a switch goes to the slot's CloudPath side.
// An attempt finishes on the side it was dispatched to.
type channelSlot struct {
	// id is the channel id reported in the x-goog-spanner-request-id header.
	id     uint64
	direct gtransport.ConnPool
	// fallback is nil when DirectPath fallback is off; the slot then only
	// forwards to direct.
	fallback *directPathFallback
	// dialCloud dials the CloudPath side. It fails with errNoCloudPath when
	// the client uses a caller-supplied pool, which has no separate CloudPath.
	dialCloud func(context.Context) (gtransport.ConnPool, error)

	cloudMu sync.Mutex
	cloud   atomic.Pointer[slotConn]
	// dialing is the CloudPath dial in progress, if any.
	dialing *cloudDial
	closed  bool
}

type slotConn struct{ gtransport.ConnPool }

// cloudDial is one CloudPath dial of a slot. It runs on its own goroutine
// under a context that only the slot and its pool cancel, so every attempt
// waiting for it can give up at its own deadline without cancelling the dial
// for the others.
type cloudDial struct {
	cancel context.CancelFunc
	done   chan struct{}
	// conn and err are set before done is closed.
	conn gtransport.ConnPool
	err  error
}

func newChannelSlot(id uint64, direct gtransport.ConnPool, fallback *directPathFallback, dialCloud func(context.Context) (gtransport.ConnPool, error)) *channelSlot {
	return &channelSlot{id: id, direct: direct, fallback: fallback, dialCloud: dialCloud}
}

// newCloudPathSlot returns a slot for a pool that has already fallen back to
// CloudPath: it has only the CloudPath connection, which it closes once. A
// future recovery to DirectPath would have to dial DirectPath for it first.
func newCloudPathSlot(id uint64, cloud gtransport.ConnPool, fallback *directPathFallback) *channelSlot {
	s := &channelSlot{id: id, direct: cloud, fallback: fallback}
	s.cloud.Store(&slotConn{unownedPool{cloud}})
	return s
}

// route returns the connection for a new attempt and the fallback window it is
// dispatched under. The window is nil when fallback is off. Waiting for the
// CloudPath dial ends with ctx.
func (s *channelSlot) route(ctx context.Context) (gtransport.ConnPool, *fallbackWindow, error) {
	if s.fallback == nil {
		return s.direct, nil, nil
	}
	w := s.fallback.current()
	if a, ok := ctx.Value(slotAttemptKey{}).(*slotAttempt); ok {
		a.window.Store(w)
	}
	if !w.fallback {
		return s.direct, w, nil
	}
	c, err := s.cloudConn(ctx)
	return c, w, err
}

// cloudConn returns the CloudPath connection, dialing it on first use. All
// attempts that need it while it is being dialed share one dial, and each
// stops waiting when its own ctx ends. A failed dial is not cached: the next
// attempt dials again.
func (s *channelSlot) cloudConn(ctx context.Context) (gtransport.ConnPool, error) {
	if c := s.cloud.Load(); c != nil {
		return c.ConnPool, nil
	}
	s.cloudMu.Lock()
	if c := s.cloud.Load(); c != nil {
		s.cloudMu.Unlock()
		return c.ConnPool, nil
	}
	if s.closed {
		s.cloudMu.Unlock()
		return nil, errChannelSlotClosed
	}
	d := s.dialing
	if d == nil {
		dialCtx, cancel := context.WithCancel(s.fallback.ctx)
		d = &cloudDial{cancel: cancel, done: make(chan struct{})}
		s.dialing = d
		go s.runCloudDial(dialCtx, d)
	}
	s.cloudMu.Unlock()
	select {
	case <-d.done:
		return d.conn, d.err
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func (s *channelSlot) runCloudDial(ctx context.Context, d *cloudDial) {
	c, err := s.dialCloud(ctx)
	switch {
	case err == nil:
		c = cloudPathConn{ConnPool: c, cancel: d.cancel}
	case errors.Is(err, errNoCloudPath):
		// The client uses a caller-supplied pool, so the slot keeps using its
		// direct side and owns nothing more.
		d.cancel()
		c, err = unownedPool{s.direct}, nil
	default:
		d.cancel()
	}
	s.cloudMu.Lock()
	s.dialing = nil
	closed := s.closed
	if err == nil && !closed {
		s.cloud.Store(&slotConn{c})
	}
	s.cloudMu.Unlock()
	switch {
	case err != nil:
		d.err = spannerErrorf(codes.Unavailable, "spanner: failed to dial CloudPath fallback for channel %d: %v", s.id, err)
	case closed:
		// The slot closed while the dial ran; nothing will use the connection.
		c.Close()
		d.err = errChannelSlotClosed
	default:
		d.conn = c
	}
	close(d.done)
}

// Conn returns the connection new attempts use. While the CloudPath side of a
// slot that switched is still being dialed, it returns the direct connection.
func (s *channelSlot) Conn() *grpc.ClientConn {
	if s.fallback != nil && s.fallback.current().fallback {
		if c := s.cloud.Load(); c != nil {
			return c.Conn()
		}
	}
	return s.direct.Conn()
}

// Num returns 1: a slot is one logical channel.
func (s *channelSlot) Num() int { return 1 }

// Close closes both sides of the slot and cancels a CloudPath dial in
// progress.
func (s *channelSlot) Close() error {
	s.cloudMu.Lock()
	if s.closed {
		s.cloudMu.Unlock()
		return nil
	}
	s.closed = true
	cloud := s.cloud.Load()
	d := s.dialing
	s.cloudMu.Unlock()
	if d != nil {
		d.cancel()
	}
	var errs []error
	if cloud != nil {
		errs = append(errs, cloud.Close())
	}
	errs = append(errs, s.direct.Close())
	return errors.Join(errs...)
}

// Invoke sends a unary RPC on the side the current fallback window selects.
func (s *channelSlot) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	conn, w, err := s.route(ctx)
	if err != nil {
		// The attempt never left the client; there is no path outcome.
		return err
	}
	err = conn.Invoke(ctx, method, args, reply, opts...)
	if w != nil {
		s.fallback.recordAttempt(ctx, w, err)
	}
	return err
}

// NewStream opens a stream on the side the current fallback window selects.
// The stream stays on that side until it finishes. Its outcome is recorded
// when gRPC finishes the call, whether or not the caller reads the final
// status: generated client code returns a failed SendMsg without calling
// RecvMsg. A stream that its reader stopped after it delivered data, such as
// ReadRow after its row or a query the caller stopped after the rows it
// wanted, did its work and counts as a success.
func (s *channelSlot) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, w, err := s.route(ctx)
	if err != nil {
		// The attempt never left the client; there is no path outcome.
		return nil, err
	}
	if w == nil {
		return conn.NewStream(ctx, desc, method, opts...)
	}
	stream := &slotStream{}
	// gRPC calls OnFinish at most once per stream in the common case, but
	// some stream-creation failures fire it twice or not at all, so the
	// outcome is recorded once here.
	var recorded atomic.Bool
	finish := func(err error) {
		if !recorded.CompareAndSwap(false, true) {
			return
		}
		if status.Code(err) == codes.Canceled && stream.received.Load() && errors.Is(context.Cause(ctx), errStreamStoppedByReader) {
			err = nil
		}
		s.fallback.recordAttempt(ctx, w, err)
	}
	// Prepend onto a fresh slice so that concurrent calls never write into
	// spare capacity of the caller's options.
	opts = append([]grpc.CallOption{grpc.OnFinish(finish)}, opts...)
	cs, err := conn.NewStream(ctx, desc, method, opts...)
	if err != nil {
		finish(err)
		return nil, err
	}
	stream.ClientStream = cs
	return stream, nil
}

// slotStream is a stream of a channel slot with DirectPath fallback. It
// notes whether the stream delivered a message.
type slotStream struct {
	grpc.ClientStream
	received atomic.Bool
}

func (s *slotStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err == nil && !s.received.Load() {
		s.received.Store(true)
	}
	return err
}

// borrowedConn is a single-connection ConnPool view of a connection that
// another pool owns. Close is a no-op; the owner closes the connection.
type borrowedConn struct{ *grpc.ClientConn }

func (c borrowedConn) Conn() *grpc.ClientConn { return c.ClientConn }
func (c borrowedConn) Num() int               { return 1 }
func (c borrowedConn) Close() error           { return nil }

// cloudPathConn is a CloudPath connection and the cancel function of the
// context it was dialed with. Some credentials keep the dial's context to
// refresh tokens, so the context lives as long as the connection: Close
// cancels it.
type cloudPathConn struct {
	gtransport.ConnPool
	cancel context.CancelFunc
}

func (c cloudPathConn) Close() error {
	err := c.ConnPool.Close()
	c.cancel()
	return err
}

// unownedPool uses a pool that something else closes. Close is a no-op.
type unownedPool struct{ gtransport.ConnPool }

func (unownedPool) Close() error { return nil }

// slotAttempt tells an operation which fallback window the last attempt it
// sent through a channel slot was dispatched under, so that the operation's
// outcome is attributed to the path that attempt took. Retries of one
// operation can take different paths; the last attempt produced the outcome.
type slotAttempt struct {
	window atomic.Pointer[fallbackWindow]
}

type slotAttemptKey struct{}

// withSlotAttempt returns a context that records the attempts sent with it.
func withSlotAttempt(ctx context.Context) (context.Context, *slotAttempt) {
	a := &slotAttempt{}
	return context.WithValue(ctx, slotAttemptKey{}, a), a
}
