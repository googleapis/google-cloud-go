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

// errChannelSlotClosed has the code of an RPC on a closed gRPC connection,
// which callers do not retry.
var errChannelSlotClosed = spannerErrorf(codes.Canceled, "spanner: channel is closed")

// channelSlot is the connection of one channel pool entry. It pairs the
// entry's direct connection with a CloudPath connection that it dials on
// first use after the pool falls back. An attempt finishes on the side it was
// sent on.
type channelSlot struct {
	channelID uint64
	direct    gtransport.ConnPool // nil for an entry created after the pool fell back
	fallback  *directPathFallback // nil when fallback is off

	cloudMu sync.Mutex
	cloud   atomic.Pointer[slotConn]
	dialing *cloudDial
	closed  bool
}

type slotConn struct{ gtransport.ConnPool }

// cloudDial is one CloudPath dial of a slot, shared by the attempts that wait
// for it.
type cloudDial struct {
	cancel context.CancelFunc
	done   chan struct{}
	// conn and err are set before done is closed.
	conn gtransport.ConnPool
	err  error
}

func newChannelSlot(channelID uint64, direct gtransport.ConnPool, fallback *directPathFallback) *channelSlot {
	return &channelSlot{channelID: channelID, direct: direct, fallback: fallback}
}

// newCloudPathSlot returns a slot for an entry created after the pool fell
// back. The switch is one way, so the slot only needs cloud, which it owns.
func newCloudPathSlot(channelID uint64, cloud gtransport.ConnPool, fallback *directPathFallback) *channelSlot {
	s := &channelSlot{channelID: channelID, fallback: fallback}
	s.cloud.Store(&slotConn{cloud})
	return s
}

// route returns the connection for a new attempt and the fallback window the
// attempt is sent under. The window is nil when fallback is off.
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

// Concurrent callers of cloudConn share one dial, and each stops waiting when
// its own ctx ends. A failed dial is not cached.
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
	c, err := s.fallback.dialCloud(ctx)
	switch {
	case err == nil:
		c = cloudPathConn{ConnPool: c, cancel: d.cancel}
	case errors.Is(err, errNoCloudPath):
		// The slot keeps using its direct side and owns nothing more.
		d.cancel()
		c, err = borrowedPool{s.direct}, nil
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
	case closed:
		if err == nil {
			c.Close()
		}
		d.err = errChannelSlotClosed
	case err != nil:
		d.err = spannerErrorf(codes.Unavailable, "spanner: failed to dial CloudPath fallback for channel %d: %v", s.channelID, err)
	default:
		d.conn = c
	}
	close(d.done)
}

// Conn returns the slot's current connection without dialing. After a switch
// that is the direct connection until the CloudPath one is dialed.
func (s *channelSlot) Conn() *grpc.ClientConn {
	if s.fallback != nil && s.fallback.current().fallback {
		if c := s.cloud.Load(); c != nil {
			return c.Conn()
		}
	}
	return s.direct.Conn()
}

func (s *channelSlot) Num() int { return 1 }

// Close closes both sides and cancels a CloudPath dial in progress.
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
	var err error
	if s.direct != nil {
		err = s.direct.Close()
	}
	if cloud != nil {
		err = errors.Join(err, cloud.Close())
	}
	return err
}

func (s *channelSlot) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	conn, w, err := s.route(ctx)
	if err != nil {
		// The attempt never left the client, so it has no outcome to record.
		return err
	}
	err = conn.Invoke(ctx, method, args, reply, opts...)
	if w != nil {
		s.fallback.recordAttempt(ctx, w, err)
	}
	return err
}

// NewStream records the stream's outcome when gRPC finishes the call, so it
// counts even if the caller never reads the final status.
func (s *channelSlot) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, w, err := s.route(ctx)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return conn.NewStream(ctx, desc, method, opts...)
	}
	stream := &slotStream{}
	// Some stream-creation failures make gRPC call OnFinish twice or not at
	// all, so the outcome is recorded once here.
	var recorded atomic.Bool
	finish := func(err error) {
		if !recorded.CompareAndSwap(false, true) {
			return
		}
		// A stream that delivered data before its reader stopped it did its
		// work.
		if status.Code(err) == codes.Canceled && stream.received.Load() && errors.Is(context.Cause(ctx), errStreamStoppedByReader) {
			err = nil
		}
		s.fallback.recordAttempt(ctx, w, err)
	}
	// Prepend onto a fresh slice: appending could write into the caller's.
	opts = append([]grpc.CallOption{grpc.OnFinish(finish)}, opts...)
	cs, err := conn.NewStream(ctx, desc, method, opts...)
	if err != nil {
		finish(err)
		return nil, err
	}
	stream.ClientStream = cs
	return stream, nil
}

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

// cloudPathConn is a CloudPath connection that keeps the context it was
// dialed with until it closes. Some credentials refresh tokens with that
// context.
type cloudPathConn struct {
	gtransport.ConnPool
	cancel context.CancelFunc
}

func (c cloudPathConn) Close() error {
	err := c.ConnPool.Close()
	c.cancel()
	return err
}

// slotAttempt holds the fallback window of the last attempt that an operation
// sent through a channel slot. Retries of one operation can take different
// paths; the last attempt produced the operation's outcome.
type slotAttempt struct {
	window atomic.Pointer[fallbackWindow]
}

type slotAttemptKey struct{}

func withSlotAttempt(ctx context.Context) (context.Context, *slotAttempt) {
	a := &slotAttempt{}
	return context.WithValue(ctx, slotAttemptKey{}, a), a
}
