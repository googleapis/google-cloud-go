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
)

// staticChannelPool is the fixed-size channel pool: one channelSlot per
// connection of a dialed pool, and no scaling. Slot ids are 1..Num().
type staticChannelPool struct {
	// direct holds the slots' direct connections. The slots borrow them, and
	// the pool closes direct once.
	direct gtransport.ConnPool
	slots  []*channelSlot
	next   atomic.Uint64

	// fallback is the DirectPath fallback state of all slots, or nil.
	fallback *directPathFallback

	closeOnce sync.Once
	closeErr  error
}

// newStaticChannelPool builds the pool over the connections of direct. With a
// non-nil fallback, every slot falls back to a CloudPath connection that
// dialCloud dials on first use.
func newStaticChannelPool(direct gtransport.ConnPool, fallback *directPathFallback, dialCloud func(context.Context) (gtransport.ConnPool, error)) (*staticChannelPool, error) {
	n := direct.Num()
	if n <= 0 {
		return nil, spannerErrorf(codes.InvalidArgument, "spanner: the gRPC connection pool has no connections")
	}
	p := &staticChannelPool{direct: direct, slots: make([]*channelSlot, n), fallback: fallback}
	// Conn round-robins over the dialed pool, so n calls visit every
	// connection once. The session manager relied on the same enumeration
	// before the pool was built from slots.
	for i := range p.slots {
		p.slots[i] = newChannelSlot(uint64(i+1), borrowedConn{direct.Conn()}, fallback, dialCloud)
	}
	return p, nil
}

// nextSlot returns the slots in round-robin order.
func (p *staticChannelPool) nextSlot() *channelSlot {
	return p.slots[(p.next.Add(1)-1)%uint64(len(p.slots))]
}

// Conn returns the connection the first slot uses for new attempts.
func (p *staticChannelPool) Conn() *grpc.ClientConn { return p.slots[0].Conn() }

// Num returns the number of slots.
func (p *staticChannelPool) Num() int { return len(p.slots) }

// Invoke sends a unary RPC on the next slot.
func (p *staticChannelPool) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	return p.nextSlot().Invoke(ctx, method, args, reply, opts...)
}

// NewStream opens a stream on the next slot.
func (p *staticChannelPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return p.nextSlot().NewStream(ctx, desc, method, opts...)
}

// Close stops DirectPath fallback and closes every slot and the dialed
// connections.
func (p *staticChannelPool) Close() error {
	p.closeOnce.Do(func() {
		if p.fallback != nil {
			p.fallback.close()
		}
		var errs []error
		for _, s := range p.slots {
			errs = append(errs, s.Close())
		}
		errs = append(errs, p.direct.Close())
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
