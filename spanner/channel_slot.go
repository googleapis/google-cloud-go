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
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// errChannelSlotClosed is returned when a slot is closed before or during a
// CloudPath dial. It uses codes.Canceled to match gRPC's error for a closed
// ClientConn, ensuring callers do not retry it.
var errChannelSlotClosed = spannerErrorf(codes.Canceled, "spanner: channel is closed")

// channelSlot is the connection of one channel pool entry. It pairs the
// entry's direct connection with a CloudPath connection that it dials on
// first use after the pool falls back. An attempt finishes on the side it was
// sent on.
type channelSlot struct {
	direct     atomic.Pointer[slotConn] // nil for an entry created after fallback until dialed for recovery
	dialDirect func(context.Context) (gtransport.ConnPool, error)
	fallback   *directPathFallback // nil when fallback is off

	mu      sync.Mutex
	cloud   atomic.Pointer[slotConn]
	dialing *cloudDial
	closed  bool

	// Recovery probing state.
	inFallback             atomic.Bool
	fallbackGen            uint64
	consecutiveProbes      int
	probeStreakStart       time.Time
	lastProbeFailureReason string
	probeCancel            context.CancelFunc
	probeWG                sync.WaitGroup

	closeConnsOnce sync.Once
}

type slotConn struct{ gtransport.ConnPool }

// cloudDial tracks an in-flight CloudPath dial for a channelSlot so that
// concurrent RPCs share a single dial instead of opening duplicate connections.
type cloudDial struct {
	cancel context.CancelFunc
	done   chan struct{}
	// conn and err are set before done is closed.
	conn gtransport.ConnPool
	err  error
}

func newChannelSlot(direct gtransport.ConnPool, fallback *directPathFallback) *channelSlot {
	s := &channelSlot{fallback: fallback}
	if direct != nil {
		s.direct.Store(&slotConn{direct})
	}
	fallback.registerSlot(s)
	return s
}

// newCloudPathSlot returns a slot with only a CloudPath connection, for
// entries created after fallback. The slot closes cloud when closed.
func newCloudPathSlot(cloud gtransport.ConnPool, fallback *directPathFallback, dialDirect func(context.Context) (gtransport.ConnPool, error), fallbackGen uint64) *channelSlot {
	s := &channelSlot{
		fallback:   fallback,
		dialDirect: dialDirect,
	}
	if cloud != nil {
		s.cloud.Store(&slotConn{cloud})
	}
	if fallback != nil && fallback.recovery.enabled {
		// This slot has no DirectPath connection yet, so it starts in fallback
		// and probes DirectPath even if another slot recovers before registration.
		s.enterFallback(fallbackGen)
		fallback.registerSlot(s)
	}
	return s
}

// dialChannelSlot dials a slot's connection. Once the pool has fallen back, it
// dials CloudPath directly with cloudDialTimeout unless the client has no
// separate CloudPath.
func dialChannelSlot(ctx context.Context, dial func(context.Context) (gtransport.ConnPool, error), fallback *directPathFallback, cloudDialTimeout time.Duration) (*channelSlot, error) {
	if fallback != nil {
		if w := fallback.current(); w.fallback {
			timeoutErr := spannerErrorf(codes.DeadlineExceeded, "spanner: CloudPath dial did not complete within %v", cloudDialTimeout)
			cloud, err := fallback.dialWithBoundCtx(ctx, cloudDialTimeout, fallback.dialCloud, timeoutErr)
			if err == nil {
				return newCloudPathSlot(cloud, fallback, dial, w.generation), nil
			}
			if !errors.Is(err, errNoCloudPath) {
				return nil, ToSpannerError(err)
			}
			// Disable probing pool-wide before creating the slot so newChannelSlot
			// never spawns a probe loop for a caller-supplied pool.
			fallback.probingDisabled.Store(true)
		}
	}
	direct, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	return newChannelSlot(direct, fallback), nil
}

// effectiveFallback returns the current fallback window and whether the slot
// should route over CloudPath. Loading inFallback before window ensures a
// caller that observes inFallback=false also observes the post-recovery window.
func (s *channelSlot) effectiveFallback() (*fallbackWindow, bool) {
	if s.fallback == nil {
		return nil, false
	}
	inFallback := s.inFallback.Load()
	w := s.fallback.current()
	if !s.fallback.recovery.enabled || w.fallback {
		return w, w.fallback
	}
	return w, inFallback
}

func (s *channelSlot) isInFallback() bool {
	_, inFallback := s.effectiveFallback()
	return inFallback
}

// route returns the connection for a new attempt, the fallback window the
// attempt is sent under, and whether the attempt uses CloudPath fallback. The
// window is nil when fallback is off.
func (s *channelSlot) route(ctx context.Context) (gtransport.ConnPool, *fallbackWindow, bool, error) {
	w, inFallback := s.effectiveFallback()
	if w == nil {
		if d := s.direct.Load(); d != nil {
			return d.ConnPool, nil, false, nil
		}
		return nil, nil, false, nil
	}
	if !inFallback {
		if d := s.direct.Load(); d != nil {
			return d.ConnPool, w, false, nil
		}
	}
	c, err := s.cloudConn(ctx)
	return c, w, true, err
}

func (s *channelSlot) enterFallback(gen uint64) {
	if s.fallback == nil || !s.fallback.recovery.enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || gen <= s.fallbackGen {
		return
	}
	s.fallbackGen = gen
	s.consecutiveProbes = 0
	s.probeStreakStart = time.Time{}
	s.lastProbeFailureReason = ""
	s.inFallback.Store(true)
	if s.fallback.probingDisabled.Load() || s.fallback.ctx.Err() != nil || (s.direct.Load() == nil && s.dialDirect == nil) {
		return
	}
	if s.probeCancel == nil {
		probeCtx, cancel := context.WithCancel(s.fallback.ctx)
		s.probeCancel = cancel
		s.probeWG.Add(1)
		go s.probeLoop(probeCtx, cancel)
	}
}

func (s *channelSlot) probeLoop(ctx context.Context, cancel context.CancelFunc) {
	defer s.probeWG.Done()
	defer cancel()

	wait := s.fallback.recovery.probeInterval
	if j := s.fallback.recovery.probeJitter; j != nil {
		wait += max(0, j(wait))
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.probeCancel = nil
			s.mu.Unlock()
			return
		case <-timer.C:
			s.probeOnce(ctx)
			s.mu.Lock()
			if ctx.Err() != nil || s.closed || s.fallback.probingDisabled.Load() || !s.inFallback.Load() {
				s.probeCancel = nil
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
			timer.Reset(s.fallback.recovery.probeInterval)
		}
	}
}

func (s *channelSlot) probeOnce(ctx context.Context) {
	s.mu.Lock()
	if s.closed || s.fallback.probingDisabled.Load() || !s.inFallback.Load() {
		s.mu.Unlock()
		return
	}
	probeGen := s.fallbackGen
	s.mu.Unlock()

	probeStart := time.Now()
	var reason string
	if sid, _ := s.fallback.probeSession.Load().(string); sid == "" {
		reason = codes.FailedPrecondition.String()
	} else if directConn, err := s.directConnForProbe(ctx); err != nil {
		reason = status.Code(err).String()
	} else {
		probeCtx := ctx
		if len(s.fallback.recovery.probeMD) > 0 {
			probeCtx = metadata.NewOutgoingContext(probeCtx, s.fallback.recovery.probeMD)
		}
		probeCtx = metadata.AppendToOutgoingContext(probeCtx, "x-goog-request-params", "name="+url.QueryEscape(sid))
		probeCtx, cancel := context.WithTimeout(probeCtx, s.fallback.recovery.probeRPCTimeout)
		var p peer.Peer
		_, err := spannerpb.NewSpannerClient(directConn).GetSession(probeCtx, &spannerpb.GetSessionRequest{Name: sid}, grpc.Peer(&p))
		cancel()
		if err != nil {
			reason = status.Code(err).String()
		} else if !isDirectPathPeerFn(&p) {
			reason = directPathRecoveryProbeNotDirectPathReason
		}
	}
	if ctx.Err() != nil {
		return
	}

	s.mu.Lock()
	if ctx.Err() != nil || s.closed || s.fallback.probingDisabled.Load() || !s.inFallback.Load() || s.fallbackGen != probeGen {
		s.mu.Unlock()
		return
	}
	if reason != "" {
		s.consecutiveProbes = 0
		s.probeStreakStart = time.Time{}
		shouldLog := reason != s.lastProbeFailureReason
		if shouldLog {
			s.lastProbeFailureReason = reason
		}
		s.mu.Unlock()
		if shouldLog {
			logf(s.fallback.logger, "spanner: DirectPath recovery probe failed (%s); staying on CloudPath", reason)
		}
		return
	}
	s.lastProbeFailureReason = ""
	s.consecutiveProbes++
	if s.consecutiveProbes == 1 {
		s.probeStreakStart = probeStart
	}
	if s.consecutiveProbes < s.fallback.recovery.minRecoveryProbes || probeStart.Sub(s.probeStreakStart) < s.fallback.recovery.minRecoveryDuration {
		s.mu.Unlock()
		return
	}

	// Hold s.mu across tryRecoverSlot (closeMu is a leaf lock) so a closing
	// slot cannot reset the shared fallback window or become a dangling canary.
	allowed, reset := s.fallback.tryRecoverSlot(s, s.fallbackGen)
	if !allowed {
		s.mu.Unlock()
		return
	}
	s.consecutiveProbes = 0
	s.probeStreakStart = time.Time{}
	s.inFallback.Store(false)
	s.mu.Unlock()

	s.fallback.finishSlotRecovery(reset)
}

func (s *channelSlot) directConnForProbe(ctx context.Context) (gtransport.ConnPool, error) {
	if c := s.direct.Load(); c != nil {
		return c.ConnPool, nil
	}
	if s.dialDirect == nil {
		return nil, status.Error(codes.FailedPrecondition, "spanner: slot has no DirectPath dialer")
	}
	timeoutErr := status.Error(codes.DeadlineExceeded, "spanner: DirectPath probe dial timed out")
	wrapped, err := s.fallback.dialWithBoundCtx(ctx, s.fallback.recovery.probeRPCTimeout, s.dialDirect, timeoutErr)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	closed, existing := s.closed, s.direct.Load()
	if !closed && existing == nil {
		s.direct.Store(&slotConn{wrapped})
		s.mu.Unlock()
		return wrapped, nil
	}
	s.mu.Unlock()
	wrapped.Close()
	if closed {
		return nil, errChannelSlotClosed
	}
	return existing.ConnPool, nil
}

// cloudConn returns the CloudPath connection, dialing it on first use.
// Concurrent callers share one dial and stop waiting when their own ctx ends;
// a failed dial is not cached.
func (s *channelSlot) cloudConn(ctx context.Context) (gtransport.ConnPool, error) {
	if c := s.cloud.Load(); c != nil {
		return c.ConnPool, nil
	}
	s.mu.Lock()
	if c := s.cloud.Load(); c != nil {
		s.mu.Unlock()
		return c.ConnPool, nil
	}
	if s.closed {
		s.mu.Unlock()
		return nil, errChannelSlotClosed
	}
	d := s.dialing
	if d == nil {
		dialCtx, cancel := context.WithCancel(s.fallback.ctx)
		d = &cloudDial{cancel: cancel, done: make(chan struct{})}
		s.dialing = d
		go s.runCloudDial(dialCtx, d)
	}
	s.mu.Unlock()
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
		c = scopedConn{ConnPool: c, cancel: d.cancel}
	case errors.Is(err, errNoCloudPath):
		// Reuse s.direct without closing it twice when the slot closes, and stop
		// probing pool-wide since there is no separate CloudPath to recover from.
		d.cancel()
		s.fallback.probingDisabled.Store(true)
		var direct gtransport.ConnPool
		if sc := s.direct.Load(); sc != nil {
			direct = sc.ConnPool
		}
		c, err = borrowedPool{direct}, nil
	default:
		d.cancel()
	}
	s.mu.Lock()
	s.dialing = nil
	closed := s.closed
	if err == nil && !closed {
		s.cloud.Store(&slotConn{c})
	}
	s.mu.Unlock()
	switch {
	case closed:
		if err == nil {
			c.Close()
		}
		d.err = errChannelSlotClosed
	case err != nil:
		d.err = spannerErrorf(codes.Unavailable, "spanner: failed to dial CloudPath fallback: %v", err)
	default:
		d.conn = c
	}
	close(d.done)
}

// Conn returns the slot's current connection without dialing. After a switch
// that is the direct connection until the CloudPath one is dialed.
func (s *channelSlot) Conn() *grpc.ClientConn {
	if s.isInFallback() {
		if c := s.cloud.Load(); c != nil {
			return c.Conn()
		}
	}
	if d := s.direct.Load(); d != nil {
		return d.Conn()
	}
	return nil
}

func (s *channelSlot) Num() int { return 1 }

// Close stops probing, unregisters the slot, closes both sides, and cancels a
// CloudPath dial in progress.
func (s *channelSlot) Close() error {
	s.mu.Lock()
	s.closed = true
	probeCancel := s.probeCancel
	s.probeCancel = nil
	d := s.dialing
	s.mu.Unlock()
	if probeCancel != nil {
		probeCancel()
	}
	if d != nil {
		d.cancel()
	}
	s.probeWG.Wait()
	if s.fallback != nil {
		s.fallback.unregisterSlot(s)
	}

	var err error
	s.closeConnsOnce.Do(func() {
		if direct := s.direct.Load(); direct != nil {
			err = direct.Close()
		}
		if cloud := s.cloud.Load(); cloud != nil {
			err = errors.Join(err, cloud.Close())
		}
	})
	return err
}

func (s *channelSlot) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	conn, w, usedFallback, err := s.route(ctx)
	if err != nil {
		// The attempt never left the client, so it has no outcome to record.
		return err
	}
	err = conn.Invoke(ctx, method, args, reply, opts...)
	if w != nil {
		s.fallback.recordSlotAttempt(ctx, w, usedFallback, err)
	}
	return err
}

// NewStream records the stream's outcome when gRPC finishes the call, so it
// counts even if the caller never reads the final status.
func (s *channelSlot) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, w, usedFallback, err := s.route(ctx)
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
		// A stream that delivered data before the reader stopped it succeeded.
		if status.Code(err) == codes.Canceled && stream.received.Load() && errors.Is(context.Cause(ctx), errStreamStoppedByReader) {
			err = nil
		}
		s.fallback.recordSlotAttempt(ctx, w, usedFallback, err)
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

// scopedConn wraps a ConnPool with the cancel function of the context it was
// dialed with. That dial context must stay uncanceled until the connection
// closes (because some credentials capture it for token refreshes), at which
// point Close calls cancel.
type scopedConn struct {
	gtransport.ConnPool
	cancel context.CancelFunc
}

func (c scopedConn) Close() error {
	err := c.ConnPool.Close()
	c.cancel()
	return err
}

// borrowedPool is a ConnPool that does not close the pool it wraps.
type borrowedPool struct{ gtransport.ConnPool }

func (borrowedPool) Close() error { return nil }
