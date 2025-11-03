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
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/metric"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/peer"

	"google.golang.org/grpc/status"

	btopt "cloud.google.com/go/bigtable/internal/option"

	"google.golang.org/grpc"
)

// A safety net to prevent a connection from draining indefinitely if a stream hangs.
var maxDrainingTimeout = 2 * time.Minute

// BigtableChannelPool options
type BigtableChannelPoolOption func(*BigtableChannelPool)

const (
	primeRPCTimeout     = 10 * time.Second
	unaryLoadFactor     = 1
	streamingLoadFactor = 2
)

var errNoConnections = fmt.Errorf("bigtable_connpool: no connections available in the pool")
var _ gtransport.ConnPool = &BigtableChannelPool{}

// BigtableConn wraps grpc.ClientConn to add Bigtable specific methods.
type BigtableConn struct {
	*grpc.ClientConn
	instanceName string // Needed for PingAndWarm
	appProfile   string // Needed for PingAndWarm
}

// Prime sends a PingAndWarm request to warm up the connection.
func (bc *BigtableConn) Prime(ctx context.Context) (bool, error) {
	if bc.instanceName == "" {
		return false, status.Error(codes.FailedPrecondition, "bigtable: instanceName is required for conn:Prime operation")
	}
	if bc.appProfile == "" {
		return false, status.Error(codes.FailedPrecondition, "bigtable: appProfile is required for conn:Prime operation")

	}

	client := btpb.NewBigtableClient(bc.ClientConn)
	req := &btpb.PingAndWarmRequest{
		Name:         bc.instanceName,
		AppProfileId: bc.appProfile,
	}

	// Use a timeout for the prime operation
	primeCtx, cancel := context.WithTimeout(ctx, primeRPCTimeout)
	defer cancel()

	var p peer.Peer
	_, err := client.PingAndWarm(primeCtx, req, grpc.Peer(&p))
	if err != nil {
		return false, err
	}

	isALTS := false
	if p.AuthInfo != nil {
		if _, ok := p.AuthInfo.(alts.AuthInfo); ok {
			isALTS = true
		}
	}
	return isALTS, nil
}

func NewBigtableConn(conn *grpc.ClientConn, instanceName, appProfileId string) *BigtableConn {
	return &BigtableConn{
		ClientConn:   conn,
		instanceName: instanceName,
		appProfile:   appProfileId,
	}
}

// connEntry represents a single connection in the pool.
type connEntry struct {
	conn          *BigtableConn
	unaryLoad     atomic.Int32 // In-flight unary requests
	streamingLoad atomic.Int32 // Active streams
	altsUsed      atomic.Bool  // alts used, best effort only checked during Prime()
	errorCount    int64        // Errors since the last metric report
	drainingState atomic.Bool  // True if the connection is being gracefully drained.

}

// isALTSUsed reports whether the connection is using ALTS aka Direct Access.
// best effort basis
func (e *connEntry) isALTSUsed() bool {
	return e.altsUsed.Load()
}

// isDraining atomically checks if the connection is in the draining state.
func (e *connEntry) isDraining() bool {
	return e.drainingState.Load()
}

// markAsDraining atomically sets the connection's state to draining.
// It returns true if it successfully marked it, false if it was already marked.
func (e *connEntry) markAsDraining() bool {
	return e.drainingState.CompareAndSwap(false, true)
}

// waitForDrainAndClose waits for a connection's in-flight request count to drop to zero
// before closing it. It runs in a separate goroutine.
func (p *BigtableChannelPool) waitForDrainAndClose(entry *connEntry) {
	// Create a context with a drain timeout
	ctx, cancel := context.WithTimeout(p.poolCtx, maxDrainingTimeout)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond) // 250ms tick
	defer ticker.Stop()

	btopt.Debugf(p.logger, "bigtable_connpool: Connection is draining, waiting for load to become 0.")

	for {
		select {
		case <-ticker.C:
			if entry.calculateWeightedLoad() == 0 {
				btopt.Debugf(p.logger, "bigtable_connpool: Draining connection is idle, closing now.")
				entry.conn.Close()
				return
			}
		case <-ctx.Done():
			btopt.Debugf(p.logger, "bigtable_connpool: Draining connection timed out after %v with load %d. Force closing.", maxDrainingTimeout, entry.calculateWeightedLoad())
			entry.conn.Close()
			return
		}
	}
}

func (e *connEntry) calculateWeightedLoad() int32 {
	unary := e.unaryLoad.Load()
	streaming := e.streamingLoad.Load()
	return (unaryLoadFactor * unary) + (streamingLoadFactor * streaming)
}

// BigtableChannelPool implements ConnPool and routes requests to the connection
// pool according to load balancing strategy.
type BigtableChannelPool struct {
	conns atomic.Value // Stores []*connEntry

	dial       func() (*BigtableConn, error)
	strategy   btopt.LoadBalancingStrategy
	rrIndex    uint64                     // For round-robin selection
	selectFunc func() (*connEntry, error) // returns *connEntry

	dialMu sync.Mutex // Serializes dial/replace/resize operations

	poolCtx    context.Context    // Context for the pool's background tasks
	poolCancel context.CancelFunc // Function to cancel the poolCtx

	logger *log.Logger // logging events
}

// getConns safely loads the current slice of connections.
func (p *BigtableChannelPool) getConns() []*connEntry {
	val := p.conns.Load()
	if val == nil {
		return nil
	}
	return val.([]*connEntry)
}

// NewBigtableChannelPool creates a pool of connPoolSize and takes the dial func()
func NewBigtableChannelPool(ctx context.Context, connPoolSize int, strategy btopt.LoadBalancingStrategy, dial func() (*BigtableConn, error), logger *log.Logger, mp metric.MeterProvider, opts ...BigtableChannelPoolOption) (*BigtableChannelPool, error) {
	if connPoolSize <= 0 {
		return nil, fmt.Errorf("bigtable_connpool: connPoolSize must be positive")
	}

	if dial == nil {
		return nil, fmt.Errorf("bigtable_connpool: dial function cannot be nil")
	}
	poolCtx, poolCancel := context.WithCancel(ctx)

	pool := &BigtableChannelPool{
		dial:       dial,
		strategy:   strategy,
		rrIndex:    0,
		poolCtx:    poolCtx,
		poolCancel: poolCancel,
		logger:     logger,
	}

	for _, opt := range opts {
		opt(pool)
	}

	// Set the selection function based on the strategy
	switch strategy {
	case btopt.LeastInFlight:
		pool.selectFunc = pool.selectLeastLoaded
	case btopt.PowerOfTwoLeastInFlight:
		pool.selectFunc = pool.selectLeastLoadedRandomOfTwo
	default: // RoundRobin is the default
		pool.selectFunc = pool.selectRoundRobin
	}

	initialConns := make([]*connEntry, connPoolSize)
	for i := 0; i < connPoolSize; i++ {
		select {
		case <-pool.poolCtx.Done():
			// Manually close connections created so far in this loop
			for j := 0; j < i; j++ {
				initialConns[j].conn.Close()
			}
			pool.poolCancel() // Ensure context is cancelled
			return nil, pool.poolCtx.Err()
		default:
		}

		// TODO Dial the initial connections in parallel using goroutines and a sync.WaitGroup
		conn, err := dial()
		if err != nil {
			// Manually close connections created so far in this loop
			for j := 0; j < i; j++ {
				initialConns[j].conn.Close()
			}
			pool.poolCancel() // Ensure context is cancelled
			return nil, err
		}
		entry := &connEntry{conn: conn}
		initialConns[i] = entry // TODO prime the connection
		// Prime the new connection in a non-blocking goroutine to warm it up.
		// We pass the conn object as an argument to avoid closing over the loop variable.
		go func(e *connEntry) {
			primeCtx, cancel := context.WithTimeout(pool.poolCtx, primeRPCTimeout)
			defer cancel()
			isALTS, err := e.conn.Prime(primeCtx)
			if err != nil {
				btopt.Debugf(pool.logger, "bigtable_connpool: failed to prime initial connection: %v\n", err)
			} else if isALTS {
				e.altsUsed.Store(true)
			}
		}(entry)
	}
	pool.conns.Store(initialConns)
	return pool, nil
}

// Num returns the number of connections in the pool.
func (p *BigtableChannelPool) Num() int {
	return len(p.getConns())
}

// Close closes all connections in the pool.
func (p *BigtableChannelPool) Close() error {
	p.poolCancel() // Cancel the context for background tasks
	conns := p.getConns()
	var errs multiError

	p.conns.Store(([]*connEntry)(nil)) // Mark as closed
	for _, entry := range conns {
		if err := entry.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// replaceConnection closes the connection for the oldEntry
func (p *BigtableChannelPool) replaceConnection(oldEntry *connEntry) {
	p.dialMu.Lock() // Serialize replacements
	defer p.dialMu.Unlock()

	// Mark the connection
	// if it is marked,
	// it means another routine (health eviction or dynamic scale down) took over it.
	if !oldEntry.markAsDraining() {
		return
	}

	currentConns := p.getConns()
	idx := -1
	for i, entry := range currentConns {
		if entry == oldEntry {
			idx = i
			break
		}
	}

	// If the connection isn't in the slice, it was already removed.
	// The drain process should still be kicked off.
	if idx == -1 {
		btopt.Debugf(p.logger, "bigtable_connpool: Connection to replace was already removed. Draining it.")
		// thread safe to call waitForDrainAndClose as conn.Close() can be called multiple times.
		go p.waitForDrainAndClose(oldEntry)
		return
	}
	// Simple eviction logic.
	btopt.Debugf(p.logger, "bigtable_connpool: Evicting connection at index %d\n", idx)
	select {
	case <-p.poolCtx.Done():
		btopt.Debugf(p.logger, "bigtable_connpool: Pool context done, skipping redial: %v\n", p.poolCtx.Err())
		return
	default:
	}
	newConn, err := p.dial()
	if err != nil {
		btopt.Debugf(p.logger, "bigtable_connpool: Failed to redial connection at index %d: %v\n", idx, err)
		return
	}

	newEntry := &connEntry{
		conn: newConn,
	}

	go func() {
		primeCtx, cancel := context.WithTimeout(p.poolCtx, primeRPCTimeout)
		defer cancel()
		isALTS, err := newConn.Prime(primeCtx)
		if err != nil {
			btopt.Debugf(p.logger, "bigtable_connpool: failed to prime replacement connection at index %d: %v\n", idx, err)
		} else if isALTS {
			newEntry.altsUsed.Store(true)
		}
	}()

	// Copy-on-write
	newConns := make([]*connEntry, len(currentConns))
	copy(newConns, currentConns)
	newConns[idx] = newEntry
	p.conns.Store(newConns)

	btopt.Debugf(p.logger, "bigtable_connpool: Replacing connection at index %d\n", idx)

	// Start the graceful shutdown process for the old connection
	go p.waitForDrainAndClose(oldEntry)
}

// Invoke selects the least loaded connection and calls Invoke on it.
// This method provides automatic load tracking.
// Load is tracked as a unary call.
func (p *BigtableChannelPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	entry, err := p.selectFunc()
	if err != nil {
		return err
	}
	entry.unaryLoad.Add(1)
	defer entry.unaryLoad.Add(-1)

	err = entry.conn.Invoke(ctx, method, args, reply, opts...)
	if err != nil {
		atomic.AddInt64(&entry.errorCount, 1)
	}
	return err

}

// Conn provides connbased on selectfunc()
func (p *BigtableChannelPool) Conn() *grpc.ClientConn {
	bigtableConn := p.GetBigtableConn()
	if bigtableConn == nil {
		return nil
	}
	return bigtableConn.ClientConn
}

func (p *BigtableChannelPool) GetBigtableConn() *BigtableConn {
	entry, err := p.selectFunc()
	if err != nil {
		return nil
	}
	return entry.conn
}

// NewStream selects the least loaded connection and calls NewStream on it.
// This method provides automatic load tracking via a wrapped stream.
func (p *BigtableChannelPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	entry, err := p.selectFunc()
	if err != nil {
		return nil, err
	}

	entry.streamingLoad.Add(1)
	stream, err := entry.conn.NewStream(ctx, desc, method, opts...)
	if err != nil {
		atomic.AddInt64(&entry.errorCount, 1)
		entry.streamingLoad.Add(-1) // Decrement immediately on creation failure
		return nil, err
	}

	return &refCountedStream{
		ClientStream: stream,
		entry:        entry, // Store the entry itself
		once:         sync.Once{},
	}, nil
}

// selectLeastLoadedRandomOfTwo() returns the index of the connection via random of two
func (p *BigtableChannelPool) selectLeastLoadedRandomOfTwo() (*connEntry, error) {
	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return nil, errNoConnections
	}
	if numConns == 1 {
		if conns[0].isDraining() {
			return nil, errNoConnections
		}
		return conns[0], nil
	}

	// Retry numConns * 2 times in worst case.
	for i := 0; i < numConns*2 && numConns > 1; i++ {
		idx1 := rand.Intn(numConns)
		idx2 := rand.Intn(numConns)

		entry1 := conns[idx1]
		entry2 := conns[idx2]

		if entry1.isDraining() || entry2.isDraining() {
			continue // Find another pair
		}

		if idx1 == idx2 {
			return entry1, nil // Both random choices were the same and it's not draining
		}

		load1 := entry1.calculateWeightedLoad()
		load2 := entry2.calculateWeightedLoad()
		if load1 <= load2 {
			return entry1, nil
		}
		return entry2, nil
	}
	//  Fallback to finding any active connection if the random strategy fails.,
	return p.selectLeastLoaded()
}

func (p *BigtableChannelPool) selectRoundRobin() (*connEntry, error) {
	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return nil, errNoConnections
	}
	// Add a retry loop to handle draining connections.
	// We iterate at most numConns times to prevent an infinite loop if all connections are draining.
	for i := 0; i < numConns; i++ {
		nextIndex := atomic.AddUint64(&p.rrIndex, 1) - 1
		entry := conns[int(nextIndex%uint64(numConns))]
		if !entry.isDraining() {
			return entry, nil
		}
	}

	return nil, errNoConnections // All connections we checked are draining
}

// selectLeastLoaded returns the index of the connection with the minimum load.
func (p *BigtableChannelPool) selectLeastLoaded() (*connEntry, error) {
	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return nil, errNoConnections
	}

	minIndex := -1
	minLoad := int32(1<<31 - 1) // maxInt32

	for i, entry := range conns {
		if entry.isDraining() {
			continue
		}
		currentLoad := entry.calculateWeightedLoad()
		if currentLoad < minLoad {
			minLoad = currentLoad
			minIndex = i
		}
	}
	if minIndex == -1 {
		return nil, errNoConnections // All connections are draining
	}
	return conns[minIndex], nil
}

// refCountedStream wraps a grpc.ClientStream to decrement the load count when the stream is done.
// refCountedStream in this BigtableConnectionPool is to hook into the stream's lifecycle
// to decrement the load counter (s.pool.load[s.connIndex]) when the stream is no longer usable.
// This is primarily detected by errors occurring during SendMsg or RecvMsg (including io.EOF on RecvMsg).

// Another option would have been to use grpc.OnFinish for streams is about the timing of when the load should be considered "finished".
// The grpc.OnFinish callback is executed only when the entire stream is fully closed and the final status is determined.
type refCountedStream struct {
	grpc.ClientStream
	entry *connEntry // Reference to the connection entry
	once  sync.Once
}

// SendMsg calls the embedded stream's SendMsg method.
func (s *refCountedStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		atomic.AddInt64(&s.entry.errorCount, 1)
		s.decrementLoad()
	}
	return err
}

// RecvMsg calls the embedded stream's RecvMsg method and decrements load on error.
func (s *refCountedStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil { // io.EOF is also an error, indicating stream end.
		// io.EOF is a normal stream termination, not an error to be counted.
		if !errors.Is(err, io.EOF) {
			atomic.AddInt64(&s.entry.errorCount, 1)
		}
		s.decrementLoad()
	}
	return err
}

// decrementLoad ensures the load count is decremented exactly once.
func (s *refCountedStream) decrementLoad() {
	s.once.Do(func() {
		s.entry.streamingLoad.Add(-1)
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
