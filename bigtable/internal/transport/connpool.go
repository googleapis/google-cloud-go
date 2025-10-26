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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/peer"

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

	// MetricReportingInterval is the interval at which pool metrics are reported.
	MetricReportingInterval = 1 * time.Minute
)

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

// probeResult stores a single health check outcome.
type probeResult struct {
	t          time.Time
	successful bool
}

// connHealthState holds the health monitoring state for a single connection.
type connHealthState struct {
	mu               sync.Mutex // Guards fields below
	probeHistory     []probeResult
	successfulProbes int
	failedProbes     int
	lastProbeTime    time.Time
}

// addProbeResult records a new probe outcome and prunes old history.
func (chs *connHealthState) addProbeResult(successful bool) {
	chs.mu.Lock()
	defer chs.mu.Unlock()

	now := time.Now()
	chs.lastProbeTime = now
	chs.probeHistory = append(chs.probeHistory, probeResult{t: now, successful: successful})

	if successful {
		chs.successfulProbes++
	} else {
		chs.failedProbes++
	}
	chs.pruneHistoryLocked()
}

// pruneHistoryLocked removes probe results older than WindowDuration. Assumes chs.mu is held.
func (chs *connHealthState) pruneHistoryLocked() {
	windowStart := time.Now().Add(-WindowDuration)
	firstValid := 0
	for firstValid < len(chs.probeHistory) && chs.probeHistory[firstValid].t.Before(windowStart) {
		result := chs.probeHistory[firstValid]
		if result.successful {
			chs.successfulProbes--
		} else {
			chs.failedProbes--
		}
		firstValid++
	}
	if firstValid > 0 {
		chs.probeHistory = chs.probeHistory[firstValid:]
	}
}

// isHealthy reports if the connection is currently considered healthy based on probe history.
func (chs *connHealthState) isHealthy() bool {
	chs.mu.Lock()
	defer chs.mu.Unlock()
	totalProbes := chs.successfulProbes + chs.failedProbes
	if totalProbes < MinProbesForEval {
		return true // Not enough data to make a judgment
	}
	failureRate := float64(chs.failedProbes) / float64(totalProbes) * 100.0
	return failureRate < float64(FailurePercentThresh)
}

// getFailedProbes returns the number of failed probes in the current window.
func (chs *connHealthState) getFailedProbes() int {
	chs.mu.Lock()
	defer chs.mu.Unlock()
	return chs.failedProbes
}

// connEntry represents a single connection in the pool.
type connEntry struct {
	conn          *BigtableConn
	unaryLoad     int32           // In-flight unary requests
	streamingLoad int32           // Active streams
	health        connHealthState // Embedded health state
	altsUsed      int32           // Set to 1 atomically if ALTS is used, 0 otherwise.
	errorCount    int64           // Errors since the last metric report

}

func (e *connEntry) calculateWeightedLoad() int32 {
	unary := atomic.LoadInt32(&e.unaryLoad)
	streaming := atomic.LoadInt32(&e.streamingLoad)
	return (unaryLoadFactor * unary) + (streamingLoadFactor * streaming)
}

// isALTSUsed reports whether the connection is using ALTS.
func (e *connEntry) isALTSUsed() bool {
	return atomic.LoadInt32(&e.altsUsed) == 1
}

// ChannelHealthMonitor manages the overall health checking process for a pool of connections.
type ChannelHealthMonitor struct {
	ticker           *time.Ticker
	done             chan struct{}
	stopOnce         sync.Once  // Add sync.Once
	evictionMu       sync.Mutex // Guards lastEvictionTime
	lastEvictionTime time.Time
}

// NewChannelHealthMonitor creates a new ChannelHealthMonitor.
func NewChannelHealthMonitor() *ChannelHealthMonitor {
	return &ChannelHealthMonitor{
		done: make(chan struct{}),
	}
}

// Start begins the periodic health checking loop. It takes functions to probe all connections
// and to evict unhealthy ones.
func (chm *ChannelHealthMonitor) Start(ctx context.Context, probeAll func(context.Context), evictUnhealthy func()) {
	chm.ticker = time.NewTicker(ProbeInterval)
	go func() {
		for {
			select {
			case <-chm.ticker.C:
				probeAll(ctx)
				evictUnhealthy()
			case <-chm.done:
				chm.ticker.Stop()
				return
			}
		}
	}()
}

// Stop terminates the health checking loop.
func (chm *ChannelHealthMonitor) Stop() {
	chm.stopOnce.Do(func() {
		close(chm.done)
	})
}

// AllowEviction checks if enough time has passed since the last eviction.
func (chm *ChannelHealthMonitor) AllowEviction() bool {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	return time.Since(chm.lastEvictionTime) >= MinEvictionInterval
}

// RecordEviction updates the last eviction time to the current time.
func (chm *ChannelHealthMonitor) RecordEviction() {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	chm.lastEvictionTime = time.Now()
}

// BigtableChannelPool implements ConnPool and routes requests to the connection
// pool according to load balancing strategy.
type BigtableChannelPool struct {
	conns atomic.Value // Stores []*connEntry

	dial       func() (*BigtableConn, error)
	strategy   btopt.LoadBalancingStrategy
	rrIndex    uint64                     // For round-robin selection
	selectFunc func() (*connEntry, error) // returns *connEntry

	// Health Checker instance
	healthMonitor *ChannelHealthMonitor
	dialMu        sync.Mutex // Serializes dial/replace operations

	poolCtx    context.Context    // Context for the pool's background tasks
	poolCancel context.CancelFunc // Function to cancel the poolCtx

	logger *log.Logger // logging events

	// OpenTelemetry MeterProvider for custom metrics
	meterProvider metric.MeterProvider
	// OpenTelemetry metric instruments
	outstandingRPCsHistogram         metric.Float64Histogram
	perConnectionErrorCountHistogram metric.Float64Histogram
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
func NewBigtableChannelPool(ctx context.Context, connPoolSize int, strategy btopt.LoadBalancingStrategy, dial func() (*BigtableConn, error), logger *log.Logger, mp metric.MeterProvider) (*BigtableChannelPool, error) {
	if connPoolSize <= 0 {
		return nil, fmt.Errorf("bigtable_connpool: connPoolSize must be positive")
	}

	if dial == nil {
		return nil, fmt.Errorf("bigtable_connpool: dial function cannot be nil")
	}
	poolCtx, poolCancel := context.WithCancel(ctx)

	pool := &BigtableChannelPool{
		dial:          dial,
		strategy:      strategy,
		rrIndex:       0,
		healthMonitor: NewChannelHealthMonitor(),
		poolCtx:       poolCtx,
		poolCancel:    poolCancel,
		logger:        logger,
		meterProvider: mp,
	}

	// Initialize metrics
	if pool.meterProvider != nil {
		meter := pool.meterProvider.Meter("bigtable.googleapis.com/internal/client/")
		var err error
		pool.outstandingRPCsHistogram, err = meter.Float64Histogram(
			"connection_pool/outstanding_rpcs",
			metric.WithDescription("A distribution of the number of outstanding RPCs per connection in the client pool, sampled periodically."),
			metric.WithUnit("1"),
		)
		if err != nil {
			// Log error but don't fail pool creation
			btopt.Debugf(logger, "bigtable_connpool: failed to create outstanding_rpcs histogram: %v\n", err)
			pool.outstandingRPCsHistogram = nil // Ensure it's nil if creation failed
		}

		pool.perConnectionErrorCountHistogram, err = meter.Float64Histogram(
			"per_connection_error_count",
			metric.WithDescription("Distribution of counts of channels per 'error count per minute'."),
			metric.WithUnit("1"),
		)
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
		entry := &connEntry{conn: conn, unaryLoad: 0, streamingLoad: 0}
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
				atomic.StoreInt32(&e.altsUsed, 1)
			}
		}(entry)
	}
	pool.conns.Store(initialConns)

	pool.startHealthChecker()
	pool.startMetricsReporter()
	return pool, nil
}

func (p *BigtableChannelPool) startMetricsReporter() {
	if p.outstandingRPCsHistogram == nil && p.perConnectionErrorCountHistogram == nil {
		return // Metrics not enabled or failed to initialize
	}

	go func() {
		ticker := time.NewTicker(MetricReportingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.snapshotAndRecordMetrics(p.poolCtx)
			case <-p.poolCtx.Done():
				return
			}
		}
	}()
}

func (p *BigtableChannelPool) snapshotAndRecordMetrics(ctx context.Context) {
	conns := p.getConns()
	if len(conns) == 0 {
		return
	}

	lbPolicy := p.strategy.String()

	for _, entry := range conns {
		transportType := "CLOUDPATH"
		if entry.isALTSUsed() {
			transportType = "DIRECTPATH"
		}

		// Common attributes for this connection
		baseAttrs := []attribute.KeyValue{
			attribute.String("transport_type", transportType),
			attribute.String("lb_policy", lbPolicy),
		}

		// Record distribution sample for unary load
		unaryAttrs := attribute.NewSet(append(baseAttrs, attribute.Bool("streaming", false))...)
		unaryLoad := atomic.LoadInt32(&entry.unaryLoad)
		p.outstandingRPCsHistogram.Record(ctx, float64(unaryLoad), metric.WithAttributeSet(unaryAttrs))

		// Record distribution sample for streaming load
		streamingAttrs := attribute.NewSet(append(baseAttrs, attribute.Bool("streaming", true))...)
		streamingLoad := atomic.LoadInt32(&entry.streamingLoad)
		p.outstandingRPCsHistogram.Record(ctx, float64(streamingLoad), metric.WithAttributeSet(streamingAttrs))

		// Record per-connection error count for the interval
		if p.perConnectionErrorCountHistogram != nil {
			// Atomically get the current error count and reset it to 0
			errorCount := atomic.SwapInt64(&entry.errorCount, 0)
			p.perConnectionErrorCountHistogram.Record(ctx, float64(errorCount), metric.WithAttributeSet(attribute.NewSet()))
		}
	}
}

func (p *BigtableChannelPool) startHealthChecker() {
	p.healthMonitor.Start(p.poolCtx, p.runProbes, p.detectAndEvictUnhealthy)
}

// Num returns the number of connections in the pool.
func (p *BigtableChannelPool) Num() int {
	return len(p.getConns())
}

// Close closes all connections in the pool.
func (p *BigtableChannelPool) Close() error {
	p.poolCancel() // Cancel the context for background tasks
	p.healthMonitor.Stop()

	conns := p.getConns()
	var errs multiError

	p.conns.Store(([]*connEntry)(nil)) // Mark as closed
	if conns != nil {
		for _, entry := range conns {
			if err := entry.conn.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// runProbes executes a Prime check on all connections concurrently.
func (p *BigtableChannelPool) runProbes(ctx context.Context) {
	conns := p.getConns()

	var wg sync.WaitGroup
	for _, entry := range conns {
		wg.Add(1)
		go func(e *connEntry) {
			defer wg.Done()
			// Derive the probe context from the passed-in context
			probeCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
			defer cancel()
			// Check if context was already done before priming
			select {
			case <-probeCtx.Done():
				e.health.addProbeResult(false) // Count as failure if context is done
				return
			default:
			}
			// don't update conn  isAlts used entry for now.
			_, err := e.conn.Prime(probeCtx)
			e.health.addProbeResult(err == nil)
		}(entry)
	}
	wg.Wait()
}

// detectAndEvictUnhealthy checks connection health and evicts the worst unhealthy one if allowed.
func (p *BigtableChannelPool) detectAndEvictUnhealthy() {
	if !p.healthMonitor.AllowEviction() {
		return // Too soon since the last eviction.
	}

	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return
	}

	var unhealthyIndices []int
	for i, entry := range conns {
		if !entry.health.isHealthy() { // isHealthy() locks internally
			unhealthyIndices = append(unhealthyIndices, i)
		}
	}

	if len(unhealthyIndices) == 0 {
		return // All connections are healthy.
	}

	unhealthyPercent := float64(len(unhealthyIndices)) / float64(numConns) * 100.0
	if unhealthyPercent >= float64(PoolwideBadThreshPercent) {
		btopt.Debugf(p.logger, "bigtable_connpool: Circuit breaker tripped, %d%% unhealthy, not evicting\n", int(unhealthyPercent))
		return // Too many unhealthy connections, don't evict.
	}

	// Find the connection with the most failed probes among the unhealthy ones.
	worstIdx := -1
	maxFailed := -1
	for _, idx := range unhealthyIndices {
		entry := conns[idx]                      // Safe, using snapshot
		failed := entry.health.getFailedProbes() // getFailedProbes() locks internally
		if failed > maxFailed {
			maxFailed = failed
			worstIdx = idx
		}
	}

	if worstIdx != -1 {
		p.healthMonitor.RecordEviction() // Record eviction time *before* replacing.
		p.replaceConnection(worstIdx)
	}
}

// replaceConnection closes the connection at the given index and dials a new one.
func (p *BigtableChannelPool) replaceConnection(idx int) {
	p.dialMu.Lock() // 	p.dialMu.Lock() // Serialize replacements
	defer p.dialMu.Unlock()

	currentConns := p.getConns()

	if idx < 0 || idx >= len(currentConns) {
		return // Should not happen
	}

	oldEntry := currentConns[idx]
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
		conn:          newConn,
		unaryLoad:     0,
		streamingLoad: 0,
		health:        connHealthState{},
	}

	go func() {
		primeCtx, cancel := context.WithTimeout(p.poolCtx, primeRPCTimeout)
		defer cancel()
		isALTS, err := newConn.Prime(primeCtx)
		if err != nil {
			btopt.Debugf(p.logger, "bigtable_connpool: failed to prime replacement connection at index %d: %v\n", idx, err)
		} else if isALTS {
			atomic.StoreInt32(&newEntry.altsUsed, 1)
		}
	}()

	// Copy-on-write
	newConns := make([]*connEntry, len(currentConns))
	copy(newConns, currentConns)
	newConns[idx] = newEntry
	p.conns.Store(newConns)

	btopt.Debugf(p.logger, "bigtable_connpool: Replaced connection at index %d\n", idx)

	go func() {
		// TODO Implement graceful draining
		if err := oldEntry.conn.Close(); err != nil {
			btopt.Debugf(p.logger, "bigtable_connpool: Error closing evicted connection at index %d: %v\n", idx, err)
		}
	}()
}

// Invoke selects the least loaded connection and calls Invoke on it.
// This method provides automatic load tracking.
// Load is tracked as a unary call.
func (p *BigtableChannelPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	entry, err := p.selectFunc()
	if err != nil {
		return err
	}
	atomic.AddInt32(&entry.unaryLoad, 1)
	defer atomic.AddInt32(&entry.unaryLoad, -1)

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

	atomic.AddInt32(&entry.streamingLoad, 1)
	stream, err := entry.conn.NewStream(ctx, desc, method, opts...)
	if err != nil {
		atomic.AddInt64(&entry.errorCount, 1)
		atomic.AddInt32(&entry.streamingLoad, -1)
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
		return conns[0], nil
	}

	idx1 := rand.Intn(numConns)
	idx2 := rand.Intn(numConns)
	for idx2 == idx1 {
		idx2 = rand.Intn(numConns)
	}

	entry1 := conns[idx1]
	entry2 := conns[idx2]
	load1 := entry1.calculateWeightedLoad()
	load2 := entry2.calculateWeightedLoad()

	if load1 <= load2 {
		return entry1, nil
	}
	return entry2, nil
}

func (p *BigtableChannelPool) selectRoundRobin() (*connEntry, error) {
	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return nil, errNoConnections
	}
	if numConns == 1 {
		return conns[0], nil
	}

	nextIndex := atomic.AddUint64(&p.rrIndex, 1) - 1
	return conns[int(nextIndex%uint64(numConns))], nil
}

// selectLeastLoaded returns the index of the connection with the minimum load.
func (p *BigtableChannelPool) selectLeastLoaded() (*connEntry, error) {
	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return nil, errNoConnections
	}

	minIndex := 0
	minLoad := conns[0].calculateWeightedLoad()

	for i := 1; i < numConns; i++ {
		currentLoad := conns[i].calculateWeightedLoad()
		if currentLoad < minLoad {
			minLoad = currentLoad
			minIndex = i
		}
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
		atomic.AddInt32(&s.entry.streamingLoad, -1)
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
