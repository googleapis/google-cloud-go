// Copyright 2025 Google LLC
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

package internal

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
)

const connThreshold = 10

var _ grpc.ClientConnInterface = &LeastLoadedChannelPool{}

// LeastLoadedChannelPool implements ConnPool and routes requests to the connection
// with the least number of active requests.
//
// To benefit from automatic load tracking, use the Invoke and NewStream methods
// directly on the leastLoadedConnPool instance.
type LeastLoadedChannelPool struct {
	conns []*grpc.ClientConn
	load  []int64 // Tracks active requests per connection

	// Mutex is only used for selecting the least loaded connection.
	// The load array itself is manipulated using atomic operations.
	mu   sync.Mutex
	dial func() (*grpc.ClientConn, error)
}

// NewLeastLoadedChannelPool creates a pool of connPoolSize and takes the dial func()
func NewLeastLoadedChannelPool(connPoolSize int, dial func() (*grpc.ClientConn, error)) (*LeastLoadedChannelPool, error) {
	pool := &LeastLoadedChannelPool{
		dial: dial,
	}
	for i := 0; i < connPoolSize; i++ {
		conn, err := dial()
		if err != nil {
			defer pool.Close()
			return nil, err
		}
		pool.conns = append(pool.conns, conn)
		pool.load = append(pool.load, 0)

	}
	return pool, nil

}

// Num returns the number of connections in the pool.
func (p *LeastLoadedChannelPool) Num() int {
	return len(p.conns)
}

// Conn returns the connection currently `estimated“ to have the least load.
// Note: Using the returned *grpc.ClientConn directly will NOT automatically
// update the load counters in the pool. Use the pool's Invoke/NewStream
// methods for automatic load tracking.
func (p *LeastLoadedChannelPool) Conn() *grpc.ClientConn {
	index := p.selectLeastLoaded()
	if index < 0 || index >= len(p.conns) {
		// Should not happen with proper initialization
		return nil
	}
	return p.conns[index]
}

// Close closes all connections in the pool.
func (p *LeastLoadedChannelPool) Close() error {
	var errs multiError
	for _, conn := range p.conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Invoke selects the least loaded connection and calls Invoke on it.
// This method provides automatic load tracking.
func (p *LeastLoadedChannelPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	index := p.selectLeastLoaded()
	if index < 0 || index >= len(p.conns) {
		return fmt.Errorf("grpc: no connections available in the pool")
	}
	conn := p.conns[index]

	atomic.AddInt64(&p.load[index], 1)
	defer atomic.AddInt64(&p.load[index], -1)

	return conn.Invoke(ctx, method, args, reply, opts...)
}

// NewStream selects the least loaded connection and calls NewStream on it.
// This method provides automatic load tracking via a wrapped stream.
func (p *LeastLoadedChannelPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	index := p.selectLeastLoaded()
	if index < 0 || index >= len(p.conns) {
		return nil, fmt.Errorf("grpc: no connections available in the pool")
	}
	conn := p.conns[index]

	atomic.AddInt64(&p.load[index], 1)

	stream, err := conn.NewStream(ctx, desc, method, opts...)

	if err != nil {
		atomic.AddInt64(&p.load[index], -1) // Decrement if stream creation failed
		return nil, err
	}

	// Wrap the stream to decrement load when the stream finishes.
	return &cachingStream{
		ClientStream: stream,
		pool:         p,
		connIndex:    index,
		once:         sync.Once{},
	}, nil
}

// selectLeastLoaded() returns the index of the connection via random of two
func (p *LeastLoadedChannelPool) selectLeastLoadedRandomofTwo() int {
	numConns := len(p.conns)
	if numConns == 0 {
		return -1
	}
	if numConns == 1 {
		return 0
	}

	// Pick two distinct random indices
	idx1 := rand.Intn(numConns)
	idx2 := rand.Intn(numConns)
	// Simple way to ensure they are different for small numConns.
	// For very large numConns, the chance of collision is low,
	// but a loop is safer.
	for idx2 == idx1 {
		idx2 = rand.Intn(numConns)
	}

	load1 := atomic.LoadInt64(&p.load[idx1])
	load2 := atomic.LoadInt64(&p.load[idx2])

	if load1 <= load2 {
		return idx1
	}
	return idx2
}

func (p *LeastLoadedChannelPool) selectLeastLoaded() int {
	// if the conn num < connThreshold, iterates over conn map
	if p.Num() > connThreshold {
		return p.selectLeastLoadedIterative()
	}
	// otherwise, pick random two and select the one
	// No need for mutex
	return p.selectLeastLoadedRandomofTwo()
}

// selectLeastLoaded returns the index of the connection with the minimum load.
func (p *LeastLoadedChannelPool) selectLeastLoadedIterative() int {
	if len(p.conns) == 0 {
		return -1
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	minIndex := 0
	minLoad := atomic.LoadInt64(&p.load[0])

	for i := 1; i < len(p.conns); i++ {
		currentLoad := atomic.LoadInt64(&p.load[i])
		if currentLoad < minLoad {
			minLoad = currentLoad
			minIndex = i
		}
	}
	return minIndex
}

// cachingStream wraps a grpc.ClientStream to decrement the load count when the stream is done.
// cachingStream in this LeastLoadedChannelPool is to hook into the stream's lifecycle
// to decrement the load counter (s.pool.load[s.connIndex]) when the stream is no longer usable.
// This is primarily detected by errors occurring during SendMsg or RecvMsg (including io.EOF on RecvMsg).

// Another option would have been to use grpc.OnFinish for streams is about the timing of when the load should be considered "finished".
// The grpc.OnFinish callback is executed only when the entire stream is fully closed and the final status is determined.
type cachingStream struct {
	grpc.ClientStream
	pool      *LeastLoadedChannelPool
	connIndex int
	once      sync.Once
}

// SendMsg calls the embedded stream's SendMsg method.
func (s *cachingStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.decrementLoad()
	}
	return err
}

// RecvMsg calls the embedded stream's RecvMsg method and decrements load on error.
func (s *cachingStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil { // io.EOF is also an error, indicating stream end.
		s.decrementLoad()
	}
	return err
}

// decrementLoad ensures the load count is decremented exactly once.
func (s *cachingStream) decrementLoad() {
	s.once.Do(func() {
		atomic.AddInt64(&s.pool.load[s.connIndex], -1)
	})
}

type multiError []error

func (m multiError) Error() string {
	s, n := "", 0
	for _, e := range m {
		if e != nil {
			if n == 0 {
				s = e.Error()
			}
			n++
		}
	}
	switch n {
	case 0:
		return "(0 errors)"
	case 1:
		return s
	case 2:
		return s + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", s, n-1)
}
