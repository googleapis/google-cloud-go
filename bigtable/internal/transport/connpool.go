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
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/status"

	btopt "cloud.google.com/go/bigtable/internal/option"

	"google.golang.org/grpc"
)

// Constants for Channel Pool Health Checking
var (
	// ProbeInterval is the interval at which channel health is probed.
	ProbeInterval = 30 * time.Second
	// ProbeTimeout is the deadline for each individual health check probe RPC.
	ProbeTimeout = 1 * time.Second
	// WindowDuration is the duration over which probe results are kept for health evaluation.
	WindowDuration = 5 * time.Minute
	// MinProbesForEval is the minimum number of probes required before a channel's health is evaluated.
	MinProbesForEval = 4
	// FailurePercentThresh is the percentage of failed probes within the window duration
	// that will cause a channel to be considered unhealthy.
	FailurePercentThresh = 60
	// PoolwideBadThreshPercent is the "circuit breaker" threshold. If this percentage
	// of channels in the pool are unhealthy, no evictions will occur.
	PoolwideBadThreshPercent = 70
	// MinEvictionInterval is the minimum time that must pass between eviction of unhealthy channels.
	MinEvictionInterval = 1 * time.Minute
)

const primeRPCTimeout = 10 * time.Second

var errNoConnections = fmt.Errorf("bigtable_connpool: no connections available in the pool")
var _ gtransport.ConnPool = &BigtableChannelPool{}
var _ grpc.ClientConn = &BigtableConn{}

// BigtableConn wraps grpc.ClientConn to add Bigtable specific methods.
type BigtableConn struct {
	*grpc.ClientConn
	instanceName string // Needed for PingAndWarm
	appProfile   string // Needed for PingAndWarm
}

// Prime sends a PingAndWarm request to warm up the connection.
func (bc *BigtableConn) Prime(ctx context.Context) error {
	if bc.instanceName == "" {
		return status.Error(codes.FailedPrecondition, "bigtable: instanceName is required for conn:Prime operation")
	}
	if bc.appProfile == "" {
		return status.Error(codes.FailedPrecondition, "bigtable: appProfile is required for conn:Prime operation")

	}

	client := btpb.NewBigtableClient(bc.ClientConn)
	req := &btpb.PingAndWarmRequest{
		Name:         bc.instanceName,
		AppProfileId: bc.appProfile,
	}

	// Use a timeout for the prime operation
	primeCtx, cancel := context.WithTimeout(ctx, primeRPCTimeout)
	defer cancel()

	_, err := client.PingAndWarm(primeCtx, req)
	return err
}

func NewBigtableConn(conn *grpc.ClientConn, instanceName, appProfileId string) *BigtableConn {
	return &BigtableConn{
		ClientConn:   conn,
		instanceName: instanceName,
		appProfile:   appProfileId,
	}
}

// Health Check related types
type probeResult struct {
	t          time.Time
	successful bool
}

type connEntry struct {
	conn *BigtableConn // Changed to BigtableConn
	load int64

	// Health Check Fields
	mu               sync.Mutex
	probeHistory     []probeResult
	successfulProbes int
	failedProbes     int
	lastProbeTime    time.Time
}

// BigtableChannelPool implements ConnPool and routes requests to the connection
// pool according to load balancing strategy.
//
// To benefit from automatic load tracking, use the Invoke and NewStream methods
// directly on the BigtableChannelPool instance.
type BigtableChannelPool struct {
	conns []*connEntry // Changed to connEntry

	// Mutex is only used for selecting the least loaded connection.
	// The load array itself is manipulated using atomic operations.
	mu         sync.Mutex
	dial       func() (*grpc.ClientConn, error)
	strategy   btopt.LoadBalancingStrategy
	rrIndex    uint64              // For round-robin selection
	selectFunc func() (int, error) // Stored function for connection selection

	// Health Checker Fields
	healthCheckTicker *time.Ticker
	healthCheckDone   chan struct{}
	// for dailing
	dialMu           sync.Mutex
	lastEvictionTime time.Time
	// for eviction
	evictionMu sync.Mutex
}

// NewBigtableChannelPool creates a pool of connPoolSize and takes the dial func()
func NewBigtableChannelPool(connPoolSize int, strategy btopt.LoadBalancingStrategy, dial func() (*BigtableConn, error)) (*BigtableChannelPool, error) {
	if connPoolSize <= 0 {
		return nil, fmt.Errorf("bigtable_connpool: connPoolSize must be positive")
	}

	if dial == nil {
		return nil, fmt.Errorf("bigtable_connpool: dial function cannot be nil")
	}
	pool := &BigtableChannelPool{
		dial:     dial,
		strategy: strategy,
		rrIndex:  0,
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

	for i := 0; i < connPoolSize; i++ {
		conn, err := dial()
		if err != nil {
			defer pool.Close()
			return nil, err
		}
		pool.conns = append(pool.conns, &connEntry{conn: conn, load: 0})
	}

	pool.healthCheckDone = make(chan struct{})
	pool.startHealthChecker()
	return pool, nil

}

func (p *BigtableChannelPool) startHealthChecker() {
	p.healthCheckTicker = time.NewTicker(ProbeInterval)
	go func() {
		for {
			select {
			case <-p.healthCheckTicker.C:
				p.runProbes()
				p.detectAndEvictUnhealthy()
			case <-p.healthCheckDone:
				p.healthCheckTicker.Stop()
				return
			}
		}
	}()
}

func (p *BigtableChannelPool) stopHealthChecker() {
	if p.healthCheckDone != nil {
		close(p.healthCheckDone)
		p.healthCheckDone = nil // Prevent reentrance
	}
}

// Num returns the number of connections in the pool.
func (p *BigtableChannelPool) Num() int {
	return len(p.conns)
}

// Close closes all connections in the pool.
func (p *BigtableChannelPool) Close() error {
	p.stopHealthChecker()

	var errs multiError
	for _, entry := range p.conns {
		if err := entry.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (p *BigtableChannelPool) runProbes() {
	var wg sync.WaitGroup
	for i, entry := range p.conns {
		wg.Add(1)
		go func(idx int, e *connEntry) {
			defer wg.Done()
			p.probeConnection(idx, e)
		}(i, entry)
	}
	wg.Wait()
}

func (p *BigtableChannelPool) probeConnection(idx int, entry *connEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout) // USE CONSTANT
	defer cancel()

	startTime := time.Now()
	err := entry.conn.Prime(ctx)
	successful := err == nil
	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.lastProbeTime = startTime
	entry.probeHistory = append(entry.probeHistory, probeResult{t: startTime, successful: successful})

	if successful {
		entry.successfulProbes++
	} else {
		entry.failedProbes++
	}
	p.pruneHistoryLocked(entry)
}

// pruneHistoryLocked assumes entry.mu is held.
func (p *BigtableChannelPool) pruneHistoryLocked(entry *connEntry) {
	windowStart := time.Now().Add(-WindowDuration)
	// Find the index of the first element within the window.
	firstValid := 0
	for firstValid < len(entry.probeHistory) && entry.probeHistory[firstValid].t.Before(windowStart) {
		result := entry.probeHistory[firstValid]
		if result.successful {
			entry.successfulProbes--
		} else {
			entry.failedProbes--
		}
		firstValid++
	}
	if firstValid > 0 {
		entry.probeHistory = entry.probeHistory[firstValid:]
	}
}

// isHealthy reports if the connection is currently considered healthy.
func (e *connEntry) isHealthy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isHealthyLocked()
}

// isHealthyLocked assumes e.mu is already held.
func (e *connEntry) isHealthyLocked() bool {
	totalProbes := e.successfulProbes + e.failedProbes
	if totalProbes < MinProbesForEval {
		return true // Not enough data to make a judgment
	}

	failureRate := float64(e.failedProbes) / float64(totalProbes) * 100.0
	return failureRate < float64(FailurePercentThresh)
}

func (p *BigtableChannelPool) detectAndEvictUnhealthy() {
	p.evictionMu.Lock()
	if time.Since(p.lastEvictionTime) < MinEvictionInterval {
		p.evictionMu.Unlock()
		return
	}
	p.evictionMu.Unlock()

	var unhealthyIndices []int
	numConns := len(p.conns)
	if numConns == 0 {
		return
	}
	for i, entry := range p.conns {
		if !entry.isHealthy() { // isHealthy() locks internally
			unhealthyIndices = append(unhealthyIndices, i)
		}
	}

	if len(unhealthyIndices) == 0 {
		return
	}

	unhealthyPercent := float64(len(unhealthyIndices)) / float64(numConns) * 100.0
	if unhealthyPercent >= float64(PoolwideBadThreshPercent) {
		fmt.Printf("bigtable_connpool: Circuit breaker tripped, %d%% unhealthy, not evicting\n", int(unhealthyPercent))
		return
	}

	worstIdx := -1
	maxFailed := -1
	for _, idx := range unhealthyIndices {
		entry := p.conns[idx]
		entry.mu.Lock()
		if entry.failedProbes > maxFailed {
			maxFailed = entry.failedProbes
			worstIdx = idx
		}
		entry.mu.Unlock()
	}

	if worstIdx != -1 {
		p.evictionMu.Lock()
		p.lastEvictionTime = time.Now()
		p.evictionMu.Unlock()
		p.replaceConnection(worstIdx)
	}
}

func (p *BigtableChannelPool) replaceConnection(idx int) {
	p.dialMu.Lock()
	defer p.dialMu.Unlock()

	if idx < 0 || idx >= len(p.conns) {
		return // Should not happen
	}

	oldEntry := p.conns[idx]
	fmt.Printf("bigtable_connpool: Evicting connection at index %d\n", idx)
	go oldEntry.conn.Close()

	newConn, err := p.dial()
	if err != nil {
		fmt.Printf("bigtable_connpool: Failed to redial connection at index %d: %v\n", idx, err)
		return
	}

	newEntry := &connEntry{
		conn: newConn,
	}
	p.conns[idx] = newEntry
	// Atomically reset load for the new connection index
	atomic.StoreInt64(&oldEntry.load, 0)
	fmt.Printf("bigtable_connpool: Replaced connection at index %d\n", idx)
}

// Invoke selects the least loaded connection and calls Invoke on it.
// This method provides automatic load tracking.
func (p *BigtableChannelPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	index, err := p.selectFunc()
	if err != nil {
		return err
	}
	entry := p.conns[index]
	atomic.AddInt64(&entry.load, 1)
	defer atomic.AddInt64(&entry.load, -1)
	return entry.conn.Invoke(ctx, method, args, reply, opts...)
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
	index, err := p.selectFunc()
	if err != nil {
		// no conn available
		return nil
	}
	return p.conns[index].conn
}

// NewStream selects the least loaded connection and calls NewStream on it.
// This method provides automatic load tracking via a wrapped stream.
func (p *BigtableChannelPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	index, err := p.selectFunc()
	if err != nil {
		return nil, err
	}
	entry := p.conns[index]

	atomic.AddInt64(&entry.load, 1)

	stream, err := entry.conn.NewStream(ctx, desc, method, opts...)

	if err != nil {
		atomic.AddInt64(&entry.load, -1) // Decrement if stream creation failed
		return nil, err
	}

	// Wrap the stream to decrement load when the stream finishes.
	return &refCountedStream{
		ClientStream: stream,
		pool:         p,
		connIndex:    index,
		once:         sync.Once{},
	}, nil
}

// selectLeastLoadedRandomOfTwo() returns the index of the connection via random of two
func (p *BigtableChannelPool) selectLeastLoadedRandomOfTwo() (int, error) {
	numConns := p.Num()
	if numConns == 0 {
		return -1, errNoConnections
	}
	if numConns == 1 {
		return 0, nil
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

	load1 := atomic.LoadInt64(&p.conns[idx1].load)
	load2 := atomic.LoadInt64(&p.conns[idx2].load)

	if load1 <= load2 {
		return idx1, nil
	}
	return idx2, nil
}

func (p *BigtableChannelPool) selectRoundRobin() (int, error) {
	numConns := p.Num()
	if numConns == 0 {
		return -1, errNoConnections
	}
	if numConns == 1 {
		return 0, nil
	}

	// Atomically increment and get the next index
	nextIndex := atomic.AddUint64(&p.rrIndex, 1) - 1
	return int(nextIndex % uint64(numConns)), nil
}

// selectLeastLoaded returns the index of the connection with the minimum load.
func (p *BigtableChannelPool) selectLeastLoaded() (int, error) {
	numConns := p.Num()

	if numConns == 0 {
		return -1, errNoConnections
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	minIndex := 0
	minLoad := atomic.LoadInt64(&p.conns[0].load)

	for i := 1; i < p.Num(); i++ {
		currentLoad := atomic.LoadInt64(&p.conns[i].load)
		if currentLoad < minLoad {
			minLoad = currentLoad
			minIndex = i
		}
	}
	return minIndex, nil
}

// refCountedStream wraps a grpc.ClientStream to decrement the load count when the stream is done.
// refCountedStream in this BigtableConnectionPool is to hook into the stream's lifecycle
// to decrement the load counter (s.pool.load[s.connIndex]) when the stream is no longer usable.
// This is primarily detected by errors occurring during SendMsg or RecvMsg (including io.EOF on RecvMsg).

// Another option would have been to use grpc.OnFinish for streams is about the timing of when the load should be considered "finished".
// The grpc.OnFinish callback is executed only when the entire stream is fully closed and the final status is determined.
type refCountedStream struct {
	grpc.ClientStream
	pool      *BigtableChannelPool
	connIndex int
	once      sync.Once
}

// SendMsg calls the embedded stream's SendMsg method.
func (s *refCountedStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.decrementLoad()
	}
	return err
}

// RecvMsg calls the embedded stream's RecvMsg method and decrements load on error.
func (s *refCountedStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil { // io.EOF is also an error, indicating stream end.
		s.decrementLoad()
	}
	return err
}

// decrementLoad ensures the load count is decremented exactly once.
func (s *refCountedStream) decrementLoad() {
	s.once.Do(func() {
		entry := s.pool.conns[s.connIndex]
		atomic.AddInt64(&entry.load, -1)
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
