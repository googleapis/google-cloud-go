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
	"math"
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

// HealthCheckConfig holds the parameters for channel pool health checking.
type HealthCheckConfig struct {
	// Enabled for toggle
	Enabled bool
	// ProbeInterval is the interval at which channel health is probed.
	ProbeInterval time.Duration
	// ProbeTimeout is the deadline for each individual health check probe RPC.
	ProbeTimeout time.Duration
	// WindowDuration is the duration over which probe results are kept for health evaluation.
	WindowDuration time.Duration
	// MinProbesForEval is the minimum number of probes required before a channel's health is evaluated.
	MinProbesForEval int
	// FailurePercentThresh is the percentage of failed probes within the window duration
	// that will cause a channel to be considered unhealthy.
	FailurePercentThresh int
	// PoolwideBadThreshPercent is the "circuit breaker" threshold. If this percentage
	// of channels in the pool are unhealthy, no evictions will occur.
	PoolwideBadThreshPercent int
	// MinEvictionInterval is the minimum time that must pass between eviction of unhealthy channels.
	MinEvictionInterval time.Duration
}

func DefaultDynamicChannelPoolConfig(initialConns int) DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		Enabled:              true, // Enabled by default
		MinConns:             10,
		MaxConns:             200,
		AvgLoadHighThreshold: 50, // Example thresholds, these likely need tuning
		AvgLoadLowThreshold:  10,
		MinScalingInterval:   1 * time.Minute,
		CheckInterval:        30 * time.Second,
		MaxRemoveConns:       2, // Cap for removals
	}
}

func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Enabled:                  true,
		ProbeInterval:            30 * time.Second,
		ProbeTimeout:             1 * time.Second,
		WindowDuration:           5 * time.Minute,
		MinProbesForEval:         4,
		FailurePercentThresh:     60,
		PoolwideBadThreshPercent: 70,
		MinEvictionInterval:      1 * time.Minute,
	}
}

// Constants for Channel Pool Health Checking
var (
	// MetricReportingInterval is the interval at which pool metrics are reported.
	MetricReportingInterval = 1 * time.Minute
)

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
	config           HealthCheckConfig
	pool             *BigtableChannelPool
	ticker           *time.Ticker
	done             chan struct{}
	stopOnce         sync.Once  // Add sync.Once
	evictionMu       sync.Mutex // Guards lastEvictionTime
	lastEvictionTime time.Time
	evictionDone     chan struct{} // Notification for test

}

// NewChannelHealthMonitor creates a new ChannelHealthMonitor.
func NewChannelHealthMonitor(config HealthCheckConfig, pool *BigtableChannelPool) *ChannelHealthMonitor {
	return &ChannelHealthMonitor{
		config:       config,
		pool:         pool,
		done:         make(chan struct{}),
		evictionDone: make(chan struct{}, 1), // Buffered, non-blocking send

	}
}

// Start begins the periodic health checking loop. It takes functions to probe all connections
// and to evict unhealthy ones.
func (chm *ChannelHealthMonitor) Start(ctx context.Context) {
	if !chm.config.Enabled {
		return
	}
	chm.ticker = time.NewTicker(chm.config.ProbeInterval)
	go func() {
		defer chm.ticker.Stop()
		for {
			select {
			case <-chm.ticker.C:
				chm.pool.runProbes(ctx, chm.config)

				// Check if the eviction method returned true
				if chm.pool.detectAndEvictUnhealthy(chm.config, chm.AllowEviction, chm.RecordEviction) {
					// The notification logic now lives here, inside the monitor.
					select {
					case chm.evictionDone <- struct{}{}:
					default: // Don't block if the channel is full or nil
					}
				}
			case <-chm.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the health checking loop.
func (chm *ChannelHealthMonitor) Stop() {
	if chm.config.Enabled {
		chm.stopOnce.Do(func() {
			close(chm.done)
		})
	}
}

// AllowEviction checks if enough time has passed since the last eviction.
func (chm *ChannelHealthMonitor) AllowEviction() bool {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	return time.Since(chm.lastEvictionTime) >= chm.config.MinEvictionInterval
}

// RecordEviction updates the last eviction time to the current time.
func (chm *ChannelHealthMonitor) RecordEviction() {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	chm.lastEvictionTime = time.Now()
}

// DynamicChannelPoolConfig holds the parameters for dynamic channel pool scaling.
type DynamicChannelPoolConfig struct {
	Enabled              bool          // Whether dynamic scaling is enabled.
	MinConns             int           // Minimum number of connections in the pool.
	MaxConns             int           // Maximum number of connections in the pool.
	AvgLoadHighThreshold int32         // Average weighted load per connection to trigger scale-up.
	AvgLoadLowThreshold  int32         // Average weighted load per connection to trigger scale-down.
	MinScalingInterval   time.Duration // Minimum time between scaling operations (both up and down).
	CheckInterval        time.Duration // How often to check if scaling is needed.
	MaxRemoveConns       int           // Maximum number of connections to remove at once.
}

// DynamicScaleMonitor manages the dynamic scaling of the connection pool.
type DynamicScaleMonitor struct {
	config          DynamicChannelPoolConfig
	pool            *BigtableChannelPool
	lastScalingTime time.Time
	mu              sync.Mutex // Protects lastScalingTime and a scaling operation
	ticker          *time.Ticker
	done            chan struct{}
	stopOnce        sync.Once
}

// NewDynamicScaleMonitor creates a new DynamicScaleMonitor.
func NewDynamicScaleMonitor(config DynamicChannelPoolConfig, pool *BigtableChannelPool) *DynamicScaleMonitor {
	return &DynamicScaleMonitor{
		config: config,
		pool:   pool,
		done:   make(chan struct{}),
	}
}

// Start begins the periodic scaling check loop.
func (dsm *DynamicScaleMonitor) Start(ctx context.Context) {
	if !dsm.config.Enabled {
		return
	}
	dsm.ticker = time.NewTicker(dsm.config.CheckInterval)
	go func() {
		for {
			select {
			case <-dsm.ticker.C:
				dsm.evaluateAndScale()
			case <-dsm.done:
				dsm.ticker.Stop()
				return
			case <-ctx.Done(): // Stop when the pool context is done
				dsm.ticker.Stop()
				return
			}
		}
	}()
}

// Stop terminates the scaling check loop.
func (dsm *DynamicScaleMonitor) Stop() {
	if !dsm.config.Enabled {
		return
	}
	dsm.stopOnce.Do(func() {
		close(dsm.done)
	})
}

func (dsm *DynamicScaleMonitor) evaluateAndScale() {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()

	if time.Since(dsm.lastScalingTime) < dsm.config.MinScalingInterval {
		return // Too soon since last scaling operation
	}

	conns := dsm.pool.getConns()
	numConns := len(conns)
	if numConns == 0 {
		if dsm.config.MinConns > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: WARNING: Pool empty, attempting to scale up to MinConns\n")
			if dsm.pool.addConnections(dsm.config.MinConns) {
				dsm.lastScalingTime = time.Now()
			}
		}
		return
	}

	var totalWeightedLoad int32
	for _, entry := range conns {
		totalWeightedLoad += entry.calculateWeightedLoad()
	}
	avgLoad := totalWeightedLoad / int32(numConns)

	targetLoad := (dsm.config.AvgLoadLowThreshold + dsm.config.AvgLoadHighThreshold) / 2
	if targetLoad == 0 {
		targetLoad = 1
	} // Avoid division by zero

	if avgLoad >= dsm.config.AvgLoadHighThreshold && numConns < dsm.config.MaxConns {
		// Scale Up
		desiredConns := int(math.Ceil(float64(totalWeightedLoad) / float64(targetLoad)))
		addCount := desiredConns - numConns
		if addCount < 1 {
			addCount = 1 // Add at least one
		}
		if numConns+addCount > dsm.config.MaxConns {
			addCount = dsm.config.MaxConns - numConns
		}

		if addCount > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling up: AvgLoad=%d, CurrentSize=%d, Adding=%d\n", avgLoad, numConns, addCount)
			if dsm.pool.addConnections(addCount) {
				dsm.lastScalingTime = time.Now()
			}
		}
	} else if avgLoad <= dsm.config.AvgLoadLowThreshold && numConns > dsm.config.MinConns {
		// Scale Down
		desiredConns := int(math.Ceil(float64(totalWeightedLoad) / float64(targetLoad)))
		if desiredConns < dsm.config.MinConns {
			desiredConns = dsm.config.MinConns
		}
		removeCount := numConns - desiredConns
		if removeCount < 1 && numConns > dsm.config.MinConns {
			removeCount = 1 // Try to remove at least one if needed.
		}

		// Enforce the maximum number of connections to remove at once.
		if removeCount > dsm.config.MaxRemoveConns {
			removeCount = dsm.config.MaxRemoveConns
		}

		// Ensure we don't go below MinConns.
		if numConns-removeCount < dsm.config.MinConns {
			removeCount = numConns - dsm.config.MinConns
		}

		if removeCount > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling down: AvgLoad=%d, CurrentSize=%d, Removing=%d\n", avgLoad, numConns, removeCount)
			if dsm.pool.removeConnections(removeCount) {
				dsm.lastScalingTime = time.Now()
			}
		}
	}
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
	hcConfig      HealthCheckConfig
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
	dynamicConfig  DynamicChannelPoolConfig // Keep the config for options
	dynamicMonitor *DynamicScaleMonitor
}

// WithHealthCheckConfig sets the health check configuration for the pool.
func WithHealthCheckConfig(hcConfig HealthCheckConfig) BigtableChannelPoolOption {
	return func(p *BigtableChannelPool) {
		p.hcConfig = hcConfig
	}
}

// WithDynamicChannelPool sets the dynamic channel pool configuration.
func WithDynamicChannelPool(config DynamicChannelPoolConfig) BigtableChannelPoolOption {
	return func(p *BigtableChannelPool) {
		p.dynamicConfig = config
	}
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
		if err != nil {
			pool.perConnectionErrorCountHistogram = nil
		}
	}

	for _, opt := range opts {
		opt(pool)
	}

	// Validate dynamic config if enabled
	if pool.hcConfig.Enabled {
		pool.healthMonitor = NewChannelHealthMonitor(pool.hcConfig, pool)
	}

	// Validate dynamic config if enabled
	if pool.dynamicConfig.Enabled {
		if pool.dynamicConfig.MinConns <= 0 {
			pool.dynamicConfig.MinConns = 10
			btopt.Debugf(pool.logger, "bigtable_connpool: DynamicChannelPoolConfig.MinConns must be positive, adjusted to 1\n")
		}
		if pool.dynamicConfig.MaxConns < pool.dynamicConfig.MinConns {
			pool.dynamicConfig.MaxConns = pool.dynamicConfig.MinConns
			btopt.Debugf(pool.logger, "bigtable_connpool: DynamicChannelPoolConfig.MaxConns was less than MinConns, adjusted to %d\n", pool.dynamicConfig.MaxConns)
		}
		if connPoolSize < pool.dynamicConfig.MinConns || connPoolSize > pool.dynamicConfig.MaxConns {
			pool.poolCancel()
			return nil, fmt.Errorf("bigtable_connpool: initial connPoolSize (%d) must be between DynamicChannelPoolConfig.MinConns (%d) and MaxConns (%d)", connPoolSize, pool.dynamicConfig.MinConns, pool.dynamicConfig.MaxConns)
		}
		if pool.dynamicConfig.AvgLoadLowThreshold >= pool.dynamicConfig.AvgLoadHighThreshold {
			pool.poolCancel()
			return nil, fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.AvgLoadLowThreshold (%d) must be less than AvgLoadHighThreshold (%d)", pool.dynamicConfig.AvgLoadLowThreshold, pool.dynamicConfig.AvgLoadHighThreshold)
		}
		if pool.dynamicConfig.CheckInterval <= 0 {
			pool.poolCancel()
			return nil, fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.CheckInterval must be positive")
		}
		if pool.dynamicConfig.MinScalingInterval < 0 {
			pool.poolCancel()
			return nil, fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.MinScalingInterval cannot be negative")
		}
		if pool.dynamicConfig.MaxRemoveConns <= 0 {
			pool.dynamicConfig.MaxRemoveConns = 1
			btopt.Debugf(pool.logger, "bigtable_connpool: DynamicChannelPoolConfig.MaxRemoveConns must be positive, adjusted to 1\n")
		}
		pool.dynamicMonitor = NewDynamicScaleMonitor(pool.dynamicConfig, pool)
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

	if pool.dynamicMonitor != nil {
		pool.dynamicMonitor.Start(pool.poolCtx)
	}
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
	if p.hcConfig.Enabled {
		p.healthMonitor.Start(p.poolCtx)
	}
}

// Num returns the number of connections in the pool.
func (p *BigtableChannelPool) Num() int {
	return len(p.getConns())
}

// Close closes all connections in the pool.
func (p *BigtableChannelPool) Close() error {
	p.poolCancel() // Cancel the context for background tasks
	if p.healthMonitor != nil {
		p.healthMonitor.Stop()
	}

	if p.dynamicMonitor != nil {
		p.dynamicMonitor.Stop()
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
func (p *BigtableChannelPool) runProbes(ctx context.Context, hcConfig HealthCheckConfig) {
	conns := p.getConns()

	var wg sync.WaitGroup
	for _, entry := range conns {
		wg.Add(1)
		go func(e *connEntry, cfg HealthCheckConfig) {
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
func (p *BigtableChannelPool) detectAndEvictUnhealthy(hcConfig HealthCheckConfig, allowEviction func() bool, recordEviction func()) bool {
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
		recordEviction() // Record eviction time *before* replacing. // Record eviction time *before* replacing.
		p.replaceConnection(worstIdx)
		return true // Eviction happened
	}

	return false
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
				atomic.StoreInt32(&e.altsUsed, 1)
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
	removedConns := make([]*BigtableConn, 0, n)
	for i := 0; i < n; i++ {
		toRemove[entries[i].index] = true
		removedConns = append(removedConns, entries[i].entry.conn)
	}

	newConns := make([]*connEntry, 0, numCurrent-n)
	for i, entry := range currentConns {
		if !toRemove[i] {
			newConns = append(newConns, entry)
		}
	}

	p.conns.Store(newConns)
	btopt.Debugf(p.logger, "bigtable_connpool: Removed %d connections, new size: %d\n", n, len(newConns))

	for _, conn := range removedConns {
		go func(c *BigtableConn) {
			if err := c.Close(); err != nil {
				btopt.Debugf(p.logger, "bigtable_connpool: Error closing removed connection: %v\n", err)
			}
		}(conn)
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
