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
	"sort"
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

// Compile-time checks to ensure monitors implement the interface.
var _ Monitor = (*ChannelHealthMonitor)(nil)
var _ Monitor = (*DynamicScaleMonitor)(nil)
var _ Monitor = (*MetricsReporter)(nil)

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
func (chs *connHealthState) addProbeResult(successful bool, windowDuration time.Duration) {
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
	chs.pruneHistoryLocked(windowDuration)
}

// pruneHistoryLocked removes probe results older than WindowDuration. Assumes chs.mu is held.
func (chs *connHealthState) pruneHistoryLocked(windowDuration time.Duration) {
	windowStart := time.Now().Add(-windowDuration)
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
func (chs *connHealthState) isHealthy(minProbesForEval int, failurePercentThresh int) bool {
	chs.mu.Lock()
	defer chs.mu.Unlock()
	totalProbes := chs.successfulProbes + chs.failedProbes
	if totalProbes < minProbesForEval {
		return true // Not enough data to make a judgment
	}
	failureRate := float64(chs.failedProbes) / float64(totalProbes) * 100.0
	return failureRate < float64(failurePercentThresh)
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
	unaryLoad     atomic.Int32    // In-flight unary requests
	streamingLoad atomic.Int32    // Active streams
	health        connHealthState // Embedded health state
	altsUsed      atomic.Bool     // alts used, best effort only checked during Prime()
	errorCount    int64           // Errors since the last metric report
	drainingState atomic.Bool     // True if the connection is being gracefully drained.

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

// ChannelHealthMonitor manages the overall health checking process for a pool of connections.
type ChannelHealthMonitor struct {
	config           btopt.HealthCheckConfig
	pool             *BigtableChannelPool
	ticker           *time.Ticker
	done             chan struct{}
	stopOnce         sync.Once  // Add sync.Once
	evictionMu       sync.Mutex // Guards lastEvictionTime
	lastEvictionTime time.Time
	evictionDone     chan struct{} // Notification for test

}

// BigtableChannelPool implements ConnPool and routes requests to the connection
// pool according to load balancing strategy.
type BigtableChannelPool struct {
	conns atomic.Value // Stores []*connEntry

	dial       func() (*BigtableConn, error)
	strategy   btopt.LoadBalancingStrategy
	rrIndex    uint64                     // For round-robin selection
	selectFunc func() (*connEntry, error) // returns *connEntry

	healthMonitor *ChannelHealthMonitor
	dialMu        sync.Mutex // Serializes dial/replace/resize operations

	poolCtx    context.Context    // Context for the pool's background tasks
	poolCancel context.CancelFunc // Function to cancel the poolCtx

	logger *log.Logger // logging events

	// OpenTelemetry MeterProvider for custom metrics
	meterProvider metric.MeterProvider
	// OpenTelemetry metric instruments
	outstandingRPCsHistogram         metric.Float64Histogram
	perConnectionErrorCountHistogram metric.Float64Histogram

	// Dynamic Channel Pool fields
	// Dynamic Channel Pool Monitor
	dynamicConfig btopt.DynamicChannelPoolConfig // Keep the config for options
	hcConfig      btopt.HealthCheckConfig
	metricsConfig btopt.MetricsReporterConfig

	monitors []Monitor
}

// WithHealthCheckConfig sets the health check configuration for the pool.
func WithHealthCheckConfig(hcConfig btopt.HealthCheckConfig) BigtableChannelPoolOption {
	return func(p *BigtableChannelPool) {
		p.hcConfig = hcConfig
	}
}

// WithDynamicChannelPool sets the dynamic channel pool configuration.
func WithDynamicChannelPool(config btopt.DynamicChannelPoolConfig) BigtableChannelPoolOption {
	return func(p *BigtableChannelPool) {
		p.dynamicConfig = config
	}
}

func WithMetricsReporterConfig(config btopt.MetricsReporterConfig) BigtableChannelPoolOption {
	return func(p *BigtableChannelPool) { p.metricsConfig = config }
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
		dial:          dial,
		strategy:      strategy,
		rrIndex:       0,
		poolCtx:       poolCtx,
		poolCancel:    poolCancel,
		logger:        logger,
		meterProvider: mp,
	}

	for _, opt := range opts {
		opt(pool)
	}

	// Setup monitors based on final configurations.
	// The MetricsReporter constructor is now responsible for initializing the metric instruments.
	pool.monitors = append(pool.monitors, NewMetricsReporter(pool.metricsConfig, pool, logger))

	if pool.hcConfig.Enabled {
		pool.monitors = append(pool.monitors, NewChannelHealthMonitor(pool.hcConfig, pool))
	}
	if pool.dynamicConfig.Enabled {
		if err := validateDynamicConfig(pool.dynamicConfig, connPoolSize); err != nil {
			pool.poolCancel()
			return nil, err
		}
		pool.monitors = append(pool.monitors, NewDynamicScaleMonitor(pool.dynamicConfig, pool))
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
	pool.startMonitors()
	return pool, nil
}

func (p *BigtableChannelPool) startMonitors() {
	for _, m := range p.monitors {
		m.Start(p.poolCtx)
	}
}

// called by MetricsExporter
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
		if p.outstandingRPCsHistogram != nil {
			// Record distribution sample for unary load
			unaryAttrs := attribute.NewSet(append(baseAttrs, attribute.Bool("streaming", false))...)
			unaryLoad := entry.unaryLoad.Load()
			p.outstandingRPCsHistogram.Record(ctx, float64(unaryLoad), metric.WithAttributeSet(unaryAttrs))

			// Record distribution sample for streaming load
			streamingAttrs := attribute.NewSet(append(baseAttrs, attribute.Bool("streaming", true))...)
			streamingLoad := entry.streamingLoad.Load()
			p.outstandingRPCsHistogram.Record(ctx, float64(streamingLoad), metric.WithAttributeSet(streamingAttrs))
		}
		// Record per-connection error count for the interval
		if p.perConnectionErrorCountHistogram != nil {
			// Atomically get the current error count and reset it to 0
			errorCount := atomic.SwapInt64(&entry.errorCount, 0)
			p.perConnectionErrorCountHistogram.Record(ctx, float64(errorCount), metric.WithAttributeSet(attribute.NewSet()))
		}
	}
}

// Num returns the number of connections in the pool.
func (p *BigtableChannelPool) Num() int {
	return len(p.getConns())
}

// Close closes all connections in the pool.
func (p *BigtableChannelPool) Close() error {
	p.poolCancel() // Cancel the context for background tasks
	// Stop all monitors.
	for _, m := range p.monitors {
		m.Stop()
	}

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

// runProbes executes a Prime check on all connections concurrently.
func (p *BigtableChannelPool) runProbes(ctx context.Context, hcConfig btopt.HealthCheckConfig) {
	conns := p.getConns()

	var wg sync.WaitGroup
	for _, entry := range conns {
		wg.Add(1)
		go func(e *connEntry, cfg btopt.HealthCheckConfig) {
			defer wg.Done()
			// Derive the probe context from the passed-in context
			probeCtx, cancel := context.WithTimeout(ctx, cfg.ProbeTimeout)
			defer cancel()
			// Check if context was already done before priming
			select {
			case <-probeCtx.Done():
				e.health.addProbeResult(false, cfg.WindowDuration) // Count as failure if context is done
				return
			default:
			}
			// don't update conn  isAlts used entry for now.
			_, err := e.conn.Prime(probeCtx)
			e.health.addProbeResult(err == nil, cfg.WindowDuration)
		}(entry, hcConfig)
	}
	wg.Wait()
}

// detectAndEvictUnhealthy checks connection health and evicts the worst unhealthy one if allowed.
func (p *BigtableChannelPool) detectAndEvictUnhealthy(hcConfig btopt.HealthCheckConfig, allowEviction func() bool, recordEviction func()) bool {
	if !allowEviction() {
		return false // Too soon since the last eviction.
	}

	conns := p.getConns()
	numConns := len(conns)
	if numConns == 0 {
		return false
	}

	var unhealthyIndices []int
	for i, entry := range conns {
		if !entry.health.isHealthy(hcConfig.MinProbesForEval, hcConfig.FailurePercentThresh) { // isHealthy() locks internally
			unhealthyIndices = append(unhealthyIndices, i)
		}
	}

	if len(unhealthyIndices) == 0 {
		return false // All connections are healthy.
	}

	unhealthyPercent := float64(len(unhealthyIndices)) / float64(numConns) * 100.0
	if unhealthyPercent >= float64(hcConfig.PoolwideBadThreshPercent) {
		btopt.Debugf(p.logger, "bigtable_connpool: Circuit breaker tripped, %d%% unhealthy, not evicting\n", int(unhealthyPercent))
		return false // Too many unhealthy connections, don't evict.
	}

	// Find the connection with the most failed probes among the unhealthy ones.
	var worstEntry *connEntry
	maxFailed := -1
	for _, idx := range unhealthyIndices {
		entry := conns[idx]                      // Safe, using snapshot
		failed := entry.health.getFailedProbes() // getFailedProbes() locks internally
		if failed > maxFailed {
			maxFailed = failed
			worstEntry = entry
		}
	}

	if worstEntry != nil {
		recordEviction() // Record eviction time *before* replacing. // Record eviction time *before* replacing.
		p.replaceConnection(worstEntry)
		return true // Eviction happened
	}

	return false
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
		conn:   newConn,
		health: connHealthState{},
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

// addConnections returns true if the pool size changed.
func (p *BigtableChannelPool) addConnections(n int) bool {
	p.dialMu.Lock()
	defer p.dialMu.Unlock()

	currentConns := p.getConns()
	numCurrent := len(currentConns)
	if numCurrent >= p.dynamicConfig.MaxConns {
		return false
	}

	if numCurrent+n > p.dynamicConfig.MaxConns {
		n = p.dynamicConfig.MaxConns - numCurrent
	}

	if n <= 0 {
		return false
	}

	newEntries := make([]*connEntry, n)
	for i := 0; i < n; i++ {
		select {
		case <-p.poolCtx.Done():
			btopt.Debugf(p.logger, "bigtable_connpool: Context done, aborting addConnections: %v\n", p.poolCtx.Err())
			return false // Pool is closing
		default:
		}

		conn, err := p.dial()
		if err != nil {
			btopt.Debugf(p.logger, "bigtable_connpool: Failed to dial new connection for scale up: %v\n", err)
			n = i
			break
		}
		entry := &connEntry{conn: conn}
		newEntries[i] = entry
		go func(e *connEntry) {
			primeCtx, cancel := context.WithTimeout(p.poolCtx, primeRPCTimeout)
			defer cancel()
			isALTS, err := e.conn.Prime(primeCtx)
			if err != nil {
				btopt.Debugf(p.logger, "bigtable_connpool: failed to prime new connection: %v\n", err)
			} else if isALTS {
				e.altsUsed.Store(true)
			}
		}(entry)
	}

	if n == 0 {
		return false
	}

	newConns := make([]*connEntry, numCurrent+n)
	copy(newConns, currentConns)
	copy(newConns[numCurrent:], newEntries[:n])
	p.conns.Store(newConns)
	btopt.Debugf(p.logger, "bigtable_connpool: Added %d connections, new size: %d\n", n, len(newConns))
	return true
}

type entryWithLoad struct {
	entry *connEntry
	load  int32
	index int
}

// removeConnections returns true if the pool size changed.
func (p *BigtableChannelPool) removeConnections(n int) bool {
	p.dialMu.Lock()
	defer p.dialMu.Unlock()

	currentConns := p.getConns()
	numCurrent := len(currentConns)

	if n <= 0 || numCurrent <= p.dynamicConfig.MinConns {
		return false
	}

	// Cap the number of connections to remove.
	if n > p.dynamicConfig.MaxRemoveConns {
		n = p.dynamicConfig.MaxRemoveConns
	}

	// Ensure we don't go below MinConns.
	if numCurrent-n < p.dynamicConfig.MinConns {
		n = numCurrent - p.dynamicConfig.MinConns
	}

	if n <= 0 {
		return false
	}

	entries := make([]entryWithLoad, numCurrent)
	for i, entry := range currentConns {
		entries[i] = entryWithLoad{entry: entry, load: entry.calculateWeightedLoad(), index: i}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].load < entries[j].load
	})

	toRemove := make(map[int]bool)
	removedConns := make([]*connEntry, 0, n)
	for i := 0; i < n; i++ {
		toRemove[entries[i].index] = true
		entries[i].entry.markAsDraining()
		removedConns = append(removedConns, entries[i].entry)
	}

	newConns := make([]*connEntry, 0, numCurrent-n)
	for i, entry := range currentConns {
		if !toRemove[i] {
			newConns = append(newConns, entry)
		}
	}

	p.conns.Store(newConns)
	btopt.Debugf(p.logger, "bigtable_connpool: Removed %d connections, new size: %d\n", n, len(newConns))

	for _, entry := range removedConns {
		go p.waitForDrainAndClose(entry)
	}
	return true
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
