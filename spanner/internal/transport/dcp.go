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

package transport

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func dcpLogf(logger *log.Logger, format string, v ...any) {
	if logger == nil {
		log.Printf(format, v...)
	} else {
		logger.Printf(format, v...)
	}
}

const (
	// DCPStateActive means the entry is eligible for new picks.
	DCPStateActive int32 = iota
	// DCPStateDraining means the entry was removed from the active slice and is
	// only serving operations that already hold a reference to it.
	DCPStateDraining
	// DCPStateClosed means the entry's channel has been closed. It is final.
	DCPStateClosed
)

// DCPRefsClosed is the DCPEntry.Refs value of a closed entry. Every negative
// value means closed; the magnitude leaves room for late acquire and release calls.
const DCPRefsClosed = math.MinInt64 / 2

// DCPDrainIdleFloor is the minimum time a draining entry must be idle before it
// closes, whatever DCPDrainIdleGrace is. Tests lower it before creating pools.
var DCPDrainIdleFloor = 15 * time.Second

// DCPSelectionStrategy controls how DCP chooses an active channel.
type DCPSelectionStrategy int

const (
	// DCPPowerOfTwoLeastBusy compares two random active channels and returns the
	// channel with lower picker load, including any active error penalty.
	DCPPowerOfTwoLeastBusy DCPSelectionStrategy = iota
	// DCPRoundRobin cycles through active channels and skips draining entries.
	DCPRoundRobin
)

// DCPConfig holds the configuration knobs for a DynamicChannelPool.
type DCPConfig struct {
	DCPEnabled                           bool
	DCPInitialChannels                   int
	DCPMinChannels                       int
	DCPMaxChannels                       int
	DCPMaxRPCPerChannel                  float64
	DCPMinRPCPerChannel                  float64
	DCPErrorPenaltyStep                  int32
	DCPErrorPenaltyDuration              time.Duration
	DCPScaleDownCheckInterval            time.Duration
	DCPScaleUpCooldown                   time.Duration
	DCPDownscaleConsecutiveLowLoadChecks int
	DCPMaxScaleUpPercent                 int
	DCPMaxRemoveChannels                 int
	DCPDrainIdleGrace                    time.Duration
	DCPPrimeTimeout                      time.Duration
	DCPPrimeMaxAttempts                  int
	DCPSelectionStrategy                 DCPSelectionStrategy
}

// DefaultDCPConfig returns the default DCP settings.
func DefaultDCPConfig() DCPConfig {
	return DCPConfig{
		DCPInitialChannels:                   4,
		DCPMinChannels:                       2,
		DCPMaxChannels:                       10,
		DCPMaxRPCPerChannel:                  25,
		DCPMinRPCPerChannel:                  15,
		DCPErrorPenaltyStep:                  5,
		DCPErrorPenaltyDuration:              5 * time.Second,
		DCPScaleDownCheckInterval:            3 * time.Minute,
		DCPScaleUpCooldown:                   10 * time.Second,
		DCPDownscaleConsecutiveLowLoadChecks: 3,
		DCPMaxScaleUpPercent:                 30,
		DCPMaxRemoveChannels:                 2,
		DCPDrainIdleGrace:                    time.Minute,
		DCPPrimeTimeout:                      10 * time.Second,
		DCPPrimeMaxAttempts:                  3,
		DCPSelectionStrategy:                 DCPPowerOfTwoLeastBusy,
	}
}

// NormalizeDCPConfig fills zero-value knobs and validates internal consistency.
func NormalizeDCPConfig(cfg DCPConfig) (DCPConfig, error) {
	def := DefaultDCPConfig()
	initialChannelsSet := cfg.DCPInitialChannels != 0
	if cfg.DCPMinChannels == 0 {
		cfg.DCPMinChannels = def.DCPMinChannels
	}
	if cfg.DCPInitialChannels == 0 {
		cfg.DCPInitialChannels = def.DCPInitialChannels
		if cfg.DCPInitialChannels < cfg.DCPMinChannels {
			cfg.DCPInitialChannels = cfg.DCPMinChannels
		}
	}
	if cfg.DCPMaxChannels == 0 {
		cfg.DCPMaxChannels = def.DCPMaxChannels
	}
	if cfg.DCPMaxRPCPerChannel == 0 {
		cfg.DCPMaxRPCPerChannel = def.DCPMaxRPCPerChannel
	}
	if cfg.DCPMinRPCPerChannel == 0 {
		cfg.DCPMinRPCPerChannel = def.DCPMinRPCPerChannel
	}
	if cfg.DCPErrorPenaltyStep == 0 {
		cfg.DCPErrorPenaltyStep = min(def.DCPErrorPenaltyStep, int32(math.Ceil(cfg.DCPMaxRPCPerChannel)))
	}
	if cfg.DCPErrorPenaltyDuration == 0 {
		cfg.DCPErrorPenaltyDuration = def.DCPErrorPenaltyDuration
	}
	if cfg.DCPScaleDownCheckInterval == 0 {
		cfg.DCPScaleDownCheckInterval = def.DCPScaleDownCheckInterval
	}
	if cfg.DCPScaleUpCooldown == 0 {
		cfg.DCPScaleUpCooldown = def.DCPScaleUpCooldown
	}
	if cfg.DCPDownscaleConsecutiveLowLoadChecks == 0 {
		cfg.DCPDownscaleConsecutiveLowLoadChecks = def.DCPDownscaleConsecutiveLowLoadChecks
	}
	if cfg.DCPMaxScaleUpPercent == 0 {
		cfg.DCPMaxScaleUpPercent = def.DCPMaxScaleUpPercent
	}
	if cfg.DCPMaxRemoveChannels == 0 {
		cfg.DCPMaxRemoveChannels = def.DCPMaxRemoveChannels
	}
	if cfg.DCPDrainIdleGrace == 0 {
		cfg.DCPDrainIdleGrace = def.DCPDrainIdleGrace
	}
	if cfg.DCPPrimeTimeout == 0 {
		cfg.DCPPrimeTimeout = def.DCPPrimeTimeout
	}
	if cfg.DCPPrimeMaxAttempts == 0 {
		cfg.DCPPrimeMaxAttempts = def.DCPPrimeMaxAttempts
	}
	switch {
	case cfg.DCPInitialChannels <= 0:
		return cfg, fmt.Errorf("DCPInitialChannels must be positive")
	case cfg.DCPMinChannels <= 0:
		return cfg, fmt.Errorf("DCPMinChannels must be positive")
	case cfg.DCPMaxChannels < cfg.DCPMinChannels:
		return cfg, fmt.Errorf("DCPMaxChannels must be >= DCPMinChannels")
	case initialChannelsSet && cfg.DCPInitialChannels < cfg.DCPMinChannels:
		return cfg, fmt.Errorf("DCPInitialChannels must be >= DCPMinChannels when explicitly set")
	case cfg.DCPInitialChannels > cfg.DCPMaxChannels:
		return cfg, fmt.Errorf("DCPInitialChannels must be <= DCPMaxChannels")
	case cfg.DCPMinRPCPerChannel >= cfg.DCPMaxRPCPerChannel:
		return cfg, fmt.Errorf("DCPMinRPCPerChannel must be less than DCPMaxRPCPerChannel")
	case cfg.DCPErrorPenaltyStep < 0:
		return cfg, fmt.Errorf("DCPErrorPenaltyStep must be non-negative")
	case cfg.DCPErrorPenaltyStep > int32(math.Ceil(cfg.DCPMaxRPCPerChannel)):
		return cfg, fmt.Errorf("DCPErrorPenaltyStep must be <= ceil(DCPMaxRPCPerChannel)")
	case cfg.DCPScaleDownCheckInterval <= 0:
		return cfg, fmt.Errorf("DCPScaleDownCheckInterval must be positive")
	case cfg.DCPMaxScaleUpPercent <= 0 || cfg.DCPMaxScaleUpPercent > 100:
		return cfg, fmt.Errorf("DCPMaxScaleUpPercent must be in (0,100]")
	case cfg.DCPMaxRemoveChannels <= 0:
		return cfg, fmt.Errorf("DCPMaxRemoveChannels must be positive")
	case cfg.DCPSelectionStrategy != DCPPowerOfTwoLeastBusy && cfg.DCPSelectionStrategy != DCPRoundRobin:
		return cfg, fmt.Errorf("DCPSelectionStrategy must be DCPPowerOfTwoLeastBusy or DCPRoundRobin")
	}
	return cfg, nil
}

// DCPClient is the minimal channel client lifecycle interface required by DCPEntry.
type DCPClient interface {
	Close() error
}

// DCPMetricsRecorder records DynamicChannelPool scale-up/scale-down events and closes metric registrations.
type DCPMetricsRecorder interface {
	RecordScaleUp(ctx context.Context, channels int64)
	RecordScaleDown(ctx context.Context, channels int64)
	Close(logger *log.Logger)
}

// DynamicChannelPool owns the copy-on-write slice of DCP entries and the
// background scaling/draining loops.
type DynamicChannelPool[C DCPClient] struct {
	Entries             atomic.Pointer[[]*DCPEntry[C]]
	Cfg                 DCPConfig
	TargetRPCPerChannel float64
	PenaltyMax          int32

	Ctx    context.Context
	cancel context.CancelFunc
	logger *log.Logger

	Dial             func(context.Context) (gtransport.ConnPool, error)
	newEntryClient   func(ctx context.Context, e *DCPEntry[C]) error
	primeRPC         func(ctx context.Context, e *DCPEntry[C], sessionID string) error
	noEntriesErr     error
	primeNotReadyErr error

	rrIndex          atomic.Uint64
	nextID           atomic.Uint64
	TotalRPCLoad     atomic.Int32
	TotalPenaltyLoad atomic.Int64
	DialMu           sync.Mutex
	LastScaleUp      atomic.Int64
	ScaleUpSignal    chan struct{}
	done             chan struct{}
	stopOnce         sync.Once
	lowLoadRuns      int
	monitorMu        sync.Mutex
	PrimeSession     atomic.Value // string
	Metrics          DCPMetricsRecorder

	DrainingCount  atomic.Int64
	DrainIdleFloor time.Duration
}

// DCPEntry represents one logical DCP slot.
type DCPEntry[C DCPClient] struct {
	ID            uint64
	Pool          gtransport.ConnPool
	Delegate      C
	Client        C
	Parent        *DynamicChannelPool[C]
	UnaryLoad     atomic.Int32
	StreamLoad    atomic.Int32
	State         atomic.Int32 // DCPState*
	CreatedAt     atomic.Int64 // UnixNano creation time
	LastActivity  atomic.Int64 // UnixNano last pick/RPC/release time
	PenaltyExpiry atomic.Int64 // UnixNano penalty expiry; zero means no penalty
	PenaltyLoad   atomic.Int32 // Accumulated error penalty, capped by config
	penaltyMu     sync.Mutex   // Serializes rare penalty updates and removal
	Refs          atomic.Int64 // RPCs keeping the channel open; negative once closed
}

// NewDynamicChannelPool creates the initial channel set and starts scale workers.
func NewDynamicChannelPool[C DCPClient](
	ctx context.Context,
	cfg DCPConfig,
	logger *log.Logger,
	dial func(context.Context) (gtransport.ConnPool, error),
	newEntryClient func(ctx context.Context, e *DCPEntry[C]) error,
	primeRPC func(ctx context.Context, e *DCPEntry[C], sessionID string) error,
	initMetrics func(p *DynamicChannelPool[C]) DCPMetricsRecorder,
	noEntriesErr error,
	primeNotReadyErr error,
) (*DynamicChannelPool[C], error) {
	cfg, err := NormalizeDCPConfig(cfg)
	if err != nil {
		return nil, err
	}
	if noEntriesErr == nil {
		noEntriesErr = status.Error(codes.Unavailable, "spanner_dcp: no available channels")
	}
	if primeNotReadyErr == nil {
		primeNotReadyErr = status.Error(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")
	}
	poolCtx, cancel := context.WithCancel(ctx)
	p := &DynamicChannelPool[C]{
		Cfg:                 cfg,
		TargetRPCPerChannel: math.Max(1, math.Floor((cfg.DCPMinRPCPerChannel+cfg.DCPMaxRPCPerChannel)/2)),
		PenaltyMax:          int32(math.Ceil(cfg.DCPMaxRPCPerChannel)),
		Ctx:                 poolCtx,
		cancel:              cancel,
		logger:              logger,
		Dial:                dial,
		newEntryClient:      newEntryClient,
		primeRPC:            primeRPC,
		noEntriesErr:        noEntriesErr,
		primeNotReadyErr:    primeNotReadyErr,
		ScaleUpSignal:       make(chan struct{}, 1),
		done:                make(chan struct{}),
		DrainIdleFloor:      DCPDrainIdleFloor,
	}
	entries := make([]*DCPEntry[C], 0, cfg.DCPInitialChannels)
	for i := 0; i < cfg.DCPInitialChannels; i++ {
		e, err := p.NewEntry(ctx, false)
		if err != nil {
			for _, entry := range entries {
				entry.CloseNow()
			}
			cancel()
			return nil, err
		}
		entries = append(entries, e)
	}
	p.Entries.Store(&entries)
	if initMetrics != nil {
		p.Metrics = initMetrics(p)
	}
	go p.scaleUpWorker()
	go p.scaleDownMonitor()
	return p, nil
}

func (p *DynamicChannelPool[C]) noEntriesError() error {
	if p != nil && p.noEntriesErr != nil {
		return p.noEntriesErr
	}
	return status.Error(codes.Unavailable, "spanner_dcp: no available channels")
}

func (p *DynamicChannelPool[C]) primeNotReadyError() error {
	if p != nil && p.primeNotReadyErr != nil {
		return p.primeNotReadyErr
	}
	return status.Error(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")
}

// Num returns the number of active channels in the pool.
func (p *DynamicChannelPool[C]) Num() int { return len(p.GetEntries()) }

// Conn returns the underlying grpc.ClientConn of the first active entry, or nil if empty.
func (p *DynamicChannelPool[C]) Conn() *grpc.ClientConn {
	entries := p.GetEntries()
	if len(entries) == 0 {
		return nil
	}
	return entries[0].Pool.Conn()
}

// Invoke picks an active entry and executes a unary gRPC call on it.
func (p *DynamicChannelPool[C]) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	e, err := p.Pick(ctx)
	if err != nil {
		return err
	}
	e.UnaryLoad.Add(1)
	p.TotalRPCLoad.Add(1)
	p.MaybeSignalScaleUp(e)
	defer func() {
		e.UnaryLoad.Add(-1)
		p.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
	}()
	err = e.Pool.Invoke(ctx, method, args, reply, opts...)
	e.ApplyErrorPenalty(err)
	return err
}

// NewStream picks an active entry and opens a tracked gRPC client stream on it.
func (p *DynamicChannelPool[C]) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	e, err := p.Pick(ctx)
	if err != nil {
		return nil, err
	}
	e.StreamLoad.Add(1)
	p.TotalRPCLoad.Add(1)
	p.MaybeSignalScaleUp(e)
	s, err := e.Pool.NewStream(ctx, desc, method, opts...)
	if err != nil {
		e.ApplyErrorPenalty(err)
		e.StreamLoad.Add(-1)
		p.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
		return nil, err
	}
	return &DCPConnPoolTrackedStream[C]{ClientStream: s, Entry: e}, nil
}

// Close stops background workers, unregisters metrics, and closes all active entries.
func (p *DynamicChannelPool[C]) Close() error {
	p.stopOnce.Do(func() {
		if p.Metrics != nil {
			p.Metrics.Close(p.logger)
		}
		if p.cancel != nil {
			p.cancel()
		}
		if p.done != nil {
			close(p.done)
		}
	})
	p.DialMu.Lock()
	defer p.DialMu.Unlock()
	entries := p.GetEntries()
	p.Entries.Store(&[]*DCPEntry[C]{})
	var errs []error
	for _, e := range entries {
		if err := e.CloseNow(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// GetEntries returns the current active entry slice.
func (p *DynamicChannelPool[C]) GetEntries() []*DCPEntry[C] {
	ptr := p.Entries.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// SetPrimeSession records the multiplexed session ID used for scaled-up channel priming.
func (p *DynamicChannelPool[C]) SetPrimeSession(id string) {
	if id != "" {
		p.PrimeSession.Store(id)
		select {
		case p.ScaleUpSignal <- struct{}{}:
		default:
		}
	}
}

// HasPrimeSession reports whether a scaled-up channel can be primed.
func (p *DynamicChannelPool[C]) HasPrimeSession() bool {
	v := p.PrimeSession.Load()
	if v == nil {
		return false
	}
	sid, _ := v.(string)
	return sid != ""
}

// NewEntry dials one DCP entry and optionally primes it.
func (p *DynamicChannelPool[C]) NewEntry(ctx context.Context, prime bool) (*DCPEntry[C], error) {
	id := p.nextID.Add(1)
	entryPool, err := p.Dial(ctx)
	if err != nil {
		return nil, err
	}
	e := &DCPEntry[C]{ID: id, Pool: entryPool, Parent: p}
	now := time.Now().UnixNano()
	e.CreatedAt.Store(now)
	e.LastActivity.Store(now)
	if p.newEntryClient != nil {
		if err := p.newEntryClient(ctx, e); err != nil {
			entryPool.Close()
			return nil, err
		}
	}
	if prime {
		if err := p.Prime(ctx, e); err != nil {
			e.CloseNow()
			return nil, err
		}
	}
	return e, nil
}

// Prime verifies a scaled-up channel before publishing it to the active slice.
func (p *DynamicChannelPool[C]) Prime(ctx context.Context, e *DCPEntry[C]) error {
	v := p.PrimeSession.Load()
	if v == nil {
		return p.primeNotReadyError()
	}
	sid, _ := v.(string)
	if sid == "" {
		return p.primeNotReadyError()
	}
	if p.primeRPC == nil {
		return nil
	}
	var last error
	for i := 0; i < p.Cfg.DCPPrimeMaxAttempts; i++ {
		primeCtx, cancel := context.WithTimeout(ctx, p.Cfg.DCPPrimeTimeout)
		last = p.primeRPC(primeCtx, e, sid)
		cancel()
		if last == nil {
			return nil
		}
		if i < p.Cfg.DCPPrimeMaxAttempts-1 {
			timer := time.NewTimer(time.Duration(100*(1<<i)) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return last
}

// Pick selects an active entry according to the configured selection strategy.
func (p *DynamicChannelPool[C]) Pick(ctx context.Context) (*DCPEntry[C], error) {
	var e *DCPEntry[C]
	var err error
	if p.Cfg.DCPSelectionStrategy == DCPRoundRobin {
		e, err = p.PickRoundRobin()
	} else {
		e, err = p.PickPowerOfTwo()
	}
	if err != nil {
		return nil, err
	}
	e.LastActivity.Store(time.Now().UnixNano())
	return e, nil
}

// MaxActiveRPCPerChannel returns the highest active RPC load across entries.
func (p *DynamicChannelPool[C]) MaxActiveRPCPerChannel() int64 {
	var maxLoad int32
	for _, e := range p.GetEntries() {
		if load := e.RPCLoad(); load > maxLoad {
			maxLoad = load
		}
	}
	return int64(maxLoad)
}

// DCPConnPoolTrackedStream wraps a grpc.ClientStream from the low-level ConnPool path
// and decrements stream load when the stream finishes.
type DCPConnPoolTrackedStream[C DCPClient] struct {
	grpc.ClientStream
	Entry *DCPEntry[C]
	once  sync.Once
}

// RecvMsg receives a message and finishes stream tracking on error/EOF.
func (s *DCPConnPoolTrackedStream[C]) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.Finish(err)
	}
	return err
}

// CloseSend half-closes the stream and finishes tracking if CloseSend fails.
func (s *DCPConnPoolTrackedStream[C]) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.Finish(err)
	}
	return err
}

// Finish records error penalty, decrements stream load, and updates LastActivity once.
func (s *DCPConnPoolTrackedStream[C]) Finish(err error) {
	s.once.Do(func() {
		s.Entry.ApplyErrorPenalty(err)
		s.Entry.StreamLoad.Add(-1)
		s.Entry.Parent.TotalRPCLoad.Add(-1)
		s.Entry.LastActivity.Store(time.Now().UnixNano())
	})
}

// PickPowerOfTwo selects the lower picker-load entry from two random active entries.
func (p *DynamicChannelPool[C]) PickPowerOfTwo() (*DCPEntry[C], error) {
	entries := p.GetEntries()
	n := len(entries)
	if n == 0 {
		return nil, p.noEntriesError()
	}
	if n == 1 {
		if entries[0].IsDraining() {
			return nil, p.noEntriesError()
		}
		return entries[0], nil
	}
	for i := 0; i < n*2; i++ {
		e1, e2 := entries[rand.IntN(n)], entries[rand.IntN(n)]
		if e1.IsDraining() || e2.IsDraining() {
			continue
		}
		if e1.PickLoad() <= e2.PickLoad() {
			return e1, nil
		}
		return e2, nil
	}
	return p.PickLeastLoaded()
}

// PickRoundRobin cycles through active entries and skips draining entries.
func (p *DynamicChannelPool[C]) PickRoundRobin() (*DCPEntry[C], error) {
	entries := p.GetEntries()
	n := len(entries)
	if n == 0 {
		return nil, p.noEntriesError()
	}
	for i := 0; i < n; i++ {
		idx := p.rrIndex.Add(1) - 1
		e := entries[int(idx%uint64(n))]
		if !e.IsDraining() {
			return e, nil
		}
	}
	return nil, p.noEntriesError()
}

// PickLeastLoaded returns the active entry with the lowest picker load.
func (p *DynamicChannelPool[C]) PickLeastLoaded() (*DCPEntry[C], error) {
	var best *DCPEntry[C]
	minLoad := int32(math.MaxInt32)
	for _, e := range p.GetEntries() {
		if e.IsDraining() {
			continue
		}
		l := e.PickLoad()
		if l < minLoad {
			minLoad = l
			best = e
		}
	}
	if best == nil {
		return nil, p.noEntriesError()
	}
	return best, nil
}

// MaybeSignalScaleUp notifies the scale-up worker when the selected channel or
// average pool load exceeds DCPMaxRPCPerChannel.
func (p *DynamicChannelPool[C]) MaybeSignalScaleUp(e *DCPEntry[C]) {
	active := p.Num()
	avg := float64(0)
	if active > 0 {
		load := int64(p.TotalRPCLoad.Load()) + int64(p.TotalPenaltyLoad.Load())
		avg = float64(load) / float64(active)
	}
	entryLoad := int64(e.RPCLoad()) + int64(e.CurrentPenalty())
	if float64(entryLoad) <= p.Cfg.DCPMaxRPCPerChannel && avg <= p.Cfg.DCPMaxRPCPerChannel {
		return
	}
	select {
	case p.ScaleUpSignal <- struct{}{}:
	default:
	}
}

func (p *DynamicChannelPool[C]) scaleUpWorker() {
	for {
		select {
		case <-p.done:
			return
		case <-p.ScaleUpSignal:
			p.ScaleUp()
		}
	}
}

// ScaleUp adds and primes channels based on current total load.
func (p *DynamicChannelPool[C]) ScaleUp() {
	select {
	case <-p.done:
		return
	default:
	}
	p.DialMu.Lock()
	now := time.Now()
	last := time.Unix(0, p.LastScaleUp.Load())
	if !last.IsZero() && now.Sub(last) < p.Cfg.DCPScaleUpCooldown {
		p.DialMu.Unlock()
		return
	}
	if p.Ctx != nil && p.Ctx.Err() != nil {
		p.DialMu.Unlock()
		return
	}
	if !p.HasPrimeSession() {
		p.DialMu.Unlock()
		return
	}
	entries := p.GetEntries()
	active := 0
	var load int64
	for _, e := range entries {
		if !e.IsDraining() {
			active++
			load += int64(e.RPCLoad()) + int64(e.CurrentPenalty())
		}
	}
	if active == 0 {
		p.DialMu.Unlock()
		return
	}
	desired := int(math.Ceil(float64(load) / p.TargetRPCPerChannel))
	add := desired - active
	capPct := int(math.Ceil(float64(active) * float64(p.Cfg.DCPMaxScaleUpPercent) / 100))
	if capPct < 2 {
		capPct = 2
	}
	if add > capPct {
		add = capPct
	}
	if maxAdd := p.Cfg.DCPMaxChannels - len(entries); add > maxAdd {
		add = maxAdd
	}
	if add <= 0 {
		p.DialMu.Unlock()
		return
	}
	p.LastScaleUp.Store(now.UnixNano())
	p.DialMu.Unlock()

	newEntries := make([]*DCPEntry[C], 0, add)
	for i := 0; i < add; i++ {
		if p.Ctx != nil && p.Ctx.Err() != nil {
			break
		}
		e, err := p.NewEntry(p.Ctx, true)
		if err == nil {
			newEntries = append(newEntries, e)
		} else {
			dcpLogf(p.logger, "spanner_dcp: failed to create or prime scaled-up channel: %v", err)
		}
	}
	if len(newEntries) == 0 {
		return
	}

	p.DialMu.Lock()
	defer p.DialMu.Unlock()
	if p.Ctx != nil && p.Ctx.Err() != nil {
		closeDCPEntries(newEntries)
		return
	}
	entries = p.GetEntries()
	headroom := p.Cfg.DCPMaxChannels - len(entries)
	if headroom <= 0 {
		closeDCPEntries(newEntries)
		return
	}
	if headroom < len(newEntries) {
		closeDCPEntries(newEntries[headroom:])
		newEntries = newEntries[:headroom]
	}
	combined := make([]*DCPEntry[C], 0, len(entries)+len(newEntries))
	combined = append(combined, entries...)
	combined = append(combined, newEntries...)
	p.Entries.Store(&combined)
	if p.Metrics != nil {
		p.Metrics.RecordScaleUp(p.Ctx, int64(len(newEntries)))
	}
}

func closeDCPEntries[C DCPClient](entries []*DCPEntry[C]) {
	for _, e := range entries {
		e.CloseNow()
	}
}

func (p *DynamicChannelPool[C]) scaleDownMonitor() {
	t := time.NewTicker(p.Cfg.DCPScaleDownCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.EvaluateScaleDown()
		}
	}
}

// EvaluateScaleDown debounces low-load observations before removing channels.
func (p *DynamicChannelPool[C]) EvaluateScaleDown() {
	p.monitorMu.Lock()
	defer p.monitorMu.Unlock()
	entries := p.GetEntries()
	active := 0
	var load int32
	for _, e := range entries {
		if !e.IsDraining() {
			active++
			load += e.RPCLoad()
		}
	}
	if active == 0 {
		return
	}
	avg := float64(load) / float64(active)
	if avg > p.Cfg.DCPMinRPCPerChannel {
		p.lowLoadRuns = 0
		return
	}
	p.lowLoadRuns++
	if p.lowLoadRuns < p.Cfg.DCPDownscaleConsecutiveLowLoadChecks {
		return
	}
	p.lowLoadRuns = 0
	desired := int(math.Ceil(float64(load) / p.TargetRPCPerChannel))
	if desired < p.Cfg.DCPMinChannels {
		desired = p.Cfg.DCPMinChannels
	}
	remove := active - desired
	if remove <= 0 {
		return
	}
	if remove > p.Cfg.DCPMaxRemoveChannels {
		remove = p.Cfg.DCPMaxRemoveChannels
	}
	p.RemoveEntries(remove)
}

// RemoveEntries revalidates low load under DialMu, removes selected entries from
// the active slice, and starts graceful drain goroutines.
func (p *DynamicChannelPool[C]) RemoveEntries(count int) {
	p.DialMu.Lock()
	entries := p.GetEntries()
	active := 0
	var load int32
	type candidate struct {
		e       *DCPEntry[C]
		created int64
		load    int32
	}
	candidates := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if !e.IsDraining() {
			active++
			load += e.RPCLoad()
			candidates = append(candidates, candidate{e, e.CreatedAt.Load(), e.WeightedLoad()})
		}
	}
	if active == 0 {
		p.DialMu.Unlock()
		return
	}
	avg := float64(load) / float64(active)
	if avg > p.Cfg.DCPMinRPCPerChannel {
		p.DialMu.Unlock()
		return
	}
	desired := int(math.Ceil(float64(load) / p.TargetRPCPerChannel))
	if desired < p.Cfg.DCPMinChannels {
		desired = p.Cfg.DCPMinChannels
	}
	recomputed := active - desired
	if recomputed <= 0 {
		p.DialMu.Unlock()
		return
	}
	if count > recomputed {
		count = recomputed
	}
	if count > active-p.Cfg.DCPMinChannels {
		count = active - p.Cfg.DCPMinChannels
	}
	if count <= 0 {
		p.DialMu.Unlock()
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].load != candidates[j].load {
			return candidates[i].load < candidates[j].load
		}
		return candidates[i].created < candidates[j].created
	})
	toDrain := make(map[*DCPEntry[C]]bool)
	for i := 0; i < count && i < len(candidates); i++ {
		candidates[i].e.State.Store(DCPStateDraining)
		candidates[i].e.ClearErrorPenalty()
		toDrain[candidates[i].e] = true
	}
	keep := make([]*DCPEntry[C], 0, len(entries)-len(toDrain))
	for _, e := range entries {
		if !toDrain[e] {
			keep = append(keep, e)
		}
	}
	p.Entries.Store(&keep)
	p.DialMu.Unlock()
	removed := int64(len(toDrain))
	p.DrainingCount.Add(removed)
	if p.Metrics != nil {
		p.Metrics.RecordScaleDown(p.Ctx, removed)
	}
	for e := range toDrain {
		go p.WaitForDrainAndClose(e)
	}
}

// WaitForDrainAndClose waits until a draining entry has no RPC load and has
// been idle for DCPDrainIdleGrace, but at least DrainIdleFloor.
func (p *DynamicChannelPool[C]) WaitForDrainAndClose(e *DCPEntry[C]) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	var doneCh <-chan struct{}
	if p.Ctx != nil {
		doneCh = p.Ctx.Done()
	}
	for {
		select {
		case <-t.C:
			if e.CloseIfIdle(max(p.Cfg.DCPDrainIdleGrace, p.DrainIdleFloor)) {
				p.DrainingCount.Add(-1)
				return
			}
		case <-doneCh:
			e.CloseNow()
			p.DrainingCount.Add(-1)
			return
		}
	}
}

// Conn returns the underlying grpc.ClientConn of the entry's pool.
func (e *DCPEntry[C]) Conn() *grpc.ClientConn { return e.Pool.Conn() }

// Num returns 1 (each DCPEntry wraps a single channel pool).
func (e *DCPEntry[C]) Num() int { return 1 }

// Close closes the entry immediately.
func (e *DCPEntry[C]) Close() error { return e.CloseNow() }

// Invoke executes a unary gRPC call on the entry's underlying pool.
func (e *DCPEntry[C]) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return e.Pool.Invoke(ctx, method, args, reply, opts...)
}

// NewStream opens a gRPC stream on the entry's underlying pool.
func (e *DCPEntry[C]) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return e.Pool.NewStream(ctx, desc, method, opts...)
}

// Acquire takes a reference that keeps drain from closing the entry until Release is called.
func (e *DCPEntry[C]) Acquire() bool {
	if e.Refs.Add(1) > 0 {
		return true
	}
	e.Refs.Add(-1)
	return false
}

// Release releases a reference acquired via Acquire.
func (e *DCPEntry[C]) Release() { e.Refs.Add(-1) }

// CloseIfIdle closes a draining entry that has been idle for grace, unless an RPC holds a reference.
func (e *DCPEntry[C]) CloseIfIdle(grace time.Duration) bool {
	return e.IdleFor(grace) && e.CloseIfUnreferenced()
}

// CloseIfUnreferenced closes the entry unless an RPC holds a reference to it.
func (e *DCPEntry[C]) CloseIfUnreferenced() bool {
	if e.Refs.CompareAndSwap(0, DCPRefsClosed) {
		e.State.Store(DCPStateClosed)
		e.closeResources()
	}
	return e.Refs.Load() < 0
}

// IdleFor reports whether the entry has 0 active RPCs and has been idle for at least grace.
func (e *DCPEntry[C]) IdleFor(grace time.Duration) bool {
	return e.RPCLoad() == 0 && time.Since(time.Unix(0, e.LastActivity.Load())) >= grace
}

// CloseNow closes the entry immediately even if RPCs hold references to it.
func (e *DCPEntry[C]) CloseNow() error {
	if e.Refs.Swap(DCPRefsClosed) < 0 {
		return nil
	}
	e.State.Store(DCPStateClosed)
	return e.closeResources()
}

func (e *DCPEntry[C]) closeResources() error {
	e.ClearErrorPenalty()
	var errs []error
	if any(e.Client) != nil {
		var zero C
		if any(e.Client) != any(zero) {
			errs = append(errs, e.Client.Close())
		}
	}
	if e.Pool != nil {
		errs = append(errs, e.Pool.Close())
	}
	return errors.Join(errs...)
}

// IsDraining atomically checks whether the entry has been removed from normal selection.
func (e *DCPEntry[C]) IsDraining() bool { return e.State.Load() == DCPStateDraining }

// RPCLoad returns the current in-flight RPC load for this entry.
func (e *DCPEntry[C]) RPCLoad() int32 { return e.UnaryLoad.Load() + e.StreamLoad.Load() }

// WeightedLoad returns the current in-flight RPC load for this entry.
func (e *DCPEntry[C]) WeightedLoad() int32 { return e.RPCLoad() }

// ApplyErrorPenalty accumulates load for Unavailable and ResourceExhausted errors.
func (e *DCPEntry[C]) ApplyErrorPenalty(err error) {
	if err == nil || e.Parent == nil || e.Parent.Cfg.DCPErrorPenaltyDuration < 0 {
		return
	}
	code := status.Code(err)
	if code != codes.Unavailable && code != codes.ResourceExhausted {
		return
	}
	e.penaltyMu.Lock()
	defer e.penaltyMu.Unlock()
	if e.State.Load() != DCPStateActive {
		return
	}
	now := time.Now()
	expiry := e.PenaltyExpiry.Load()
	current := int32(0)
	oldContribution := int32(0)
	if expiry != 0 {
		oldContribution = e.PenaltyLoad.Load()
		if now.UnixNano() < expiry {
			current = oldContribution
		}
	}
	load := e.Parent.Cfg.DCPErrorPenaltyStep
	if current != 0 {
		maxPenalty := e.Parent.PenaltyMax
		if current >= maxPenalty-e.Parent.Cfg.DCPErrorPenaltyStep {
			load = maxPenalty
		} else {
			load = current + e.Parent.Cfg.DCPErrorPenaltyStep
		}
	}
	e.PenaltyLoad.Store(load)
	e.PenaltyExpiry.Store(now.Add(e.Parent.Cfg.DCPErrorPenaltyDuration).UnixNano())
	e.Parent.TotalPenaltyLoad.Add(int64(load - oldContribution))
}

// CurrentPenalty returns active accumulated error load and lazily clears an expired penalty.
func (e *DCPEntry[C]) CurrentPenalty() int32 {
	expiry := e.PenaltyExpiry.Load()
	if expiry == 0 {
		return 0
	}
	if time.Now().UnixNano() < expiry {
		return e.PenaltyLoad.Load()
	}
	e.penaltyMu.Lock()
	defer e.penaltyMu.Unlock()
	expiry = e.PenaltyExpiry.Load()
	if expiry == 0 {
		return 0
	}
	if time.Now().UnixNano() < expiry {
		return e.PenaltyLoad.Load()
	}
	e.PenaltyExpiry.Store(0)
	if e.Parent != nil {
		e.Parent.TotalPenaltyLoad.Add(-int64(e.PenaltyLoad.Load()))
	}
	return 0
}

// ClearErrorPenalty removes an entry's aggregate contribution when the entry leaves the live pool.
func (e *DCPEntry[C]) ClearErrorPenalty() {
	e.penaltyMu.Lock()
	defer e.penaltyMu.Unlock()
	if e.PenaltyExpiry.Swap(0) != 0 && e.Parent != nil {
		e.Parent.TotalPenaltyLoad.Add(-int64(e.PenaltyLoad.Load()))
	}
}

// PickLoad returns the in-flight RPC load plus any active error penalty.
func (e *DCPEntry[C]) PickLoad() int32 { return e.RPCLoad() + e.CurrentPenalty() }

// StartUnary increments unary load and returns a completion callback.
func (e *DCPEntry[C]) StartUnary(ctx context.Context) func(error) {
	e.UnaryLoad.Add(1)
	e.Parent.TotalRPCLoad.Add(1)
	e.Parent.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	return func(err error) {
		e.ApplyErrorPenalty(err)
		e.UnaryLoad.Add(-1)
		e.Parent.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
	}
}

// StartStream increments stream load and entry reference count, returning a DCPStreamRef.
func (e *DCPEntry[C]) StartStream(ctx context.Context) *DCPStreamRef {
	e.Refs.Add(1)
	e.StreamLoad.Add(1)
	e.Parent.TotalRPCLoad.Add(1)
	e.Parent.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	ref := &DCPStreamRef{finish: func(err error) {
		e.ApplyErrorPenalty(err)
		e.StreamLoad.Add(-1)
		e.Parent.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
		e.Release()
	}}
	if ctx != nil && ctx.Done() != nil {
		if err := ctx.Err(); err != nil {
			ref.Done(err)
			return ref
		}
		ref.SetStop(context.AfterFunc(ctx, func() {
			ref.Done(ctx.Err())
		}))
	}
	return ref
}

// DCPStreamRef tracks an in-flight streaming RPC's load and entry reference.
type DCPStreamRef struct {
	once       sync.Once
	finish     func(error)
	stopMu     sync.Mutex
	stop       func() bool
	doneCalled bool
}

// Done releases the stream's load and entry reference at most once.
func (r *DCPStreamRef) Done(err error) {
	r.once.Do(func() {
		r.stopMu.Lock()
		r.doneCalled = true
		stop := r.stop
		r.stopMu.Unlock()
		if stop != nil {
			stop()
		}
		r.finish(err)
	})
}

// SetStop registers the context.AfterFunc stop callback for the stream.
func (r *DCPStreamRef) SetStop(stop func() bool) {
	r.stopMu.Lock()
	doneCalled := r.doneCalled
	if !doneCalled {
		r.stop = stop
	}
	r.stopMu.Unlock()
	if doneCalled && stop != nil {
		stop()
	}
}

// DCPExecuteStreamingSQLClient wraps Spanner_ExecuteStreamingSqlClient to release
// stream load and entry reference when Recv/RecvMsg/CloseSend encounters a terminal error or EOF.
type DCPExecuteStreamingSQLClient struct {
	spannerpb.Spanner_ExecuteStreamingSqlClient
	Ref *DCPStreamRef
}

// Recv receives the next PartialResultSet and releases stream load on error/EOF.
func (c *DCPExecuteStreamingSQLClient) Recv() (*spannerpb.PartialResultSet, error) {
	resp, err := c.Spanner_ExecuteStreamingSqlClient.Recv()
	if err != nil {
		c.Ref.Done(err)
	}
	return resp, err
}

// RecvMsg receives into m and releases stream load on error/EOF.
func (c *DCPExecuteStreamingSQLClient) RecvMsg(m any) error {
	err := c.Spanner_ExecuteStreamingSqlClient.RecvMsg(m)
	if err != nil {
		c.Ref.Done(err)
	}
	return err
}

// CloseSend half-closes the client send side and releases stream load if CloseSend fails.
func (c *DCPExecuteStreamingSQLClient) CloseSend() error {
	err := c.Spanner_ExecuteStreamingSqlClient.CloseSend()
	if err != nil {
		c.Ref.Done(err)
	}
	return err
}

// DCPResolver keeps using its bound DCPEntry while it is active or draining,
// and atomically re-picks a fresh entry from Pool only once the bound entry is closed.
type DCPResolver[C DCPClient] struct {
	Pool  *DynamicChannelPool[C]
	Entry atomic.Pointer[DCPEntry[C]]
}

// Acquire returns the bound entry with a reference held, or re-picks if the bound entry is closed.
func (c *DCPResolver[C]) Acquire(ctx context.Context) (*DCPEntry[C], error) {
	if c == nil || c.Pool == nil {
		return nil, (*DynamicChannelPool[C])(nil).noEntriesError()
	}
	e := c.Entry.Load()
	for e == nil || !e.Acquire() {
		picked, err := c.Pool.Pick(ctx)
		if err != nil {
			return nil, err
		}
		if !c.Entry.CompareAndSwap(e, picked) {
			picked = c.Entry.Load()
		}
		e = picked
	}
	return e, nil
}

// Resolve returns the bound entry's Client without holding a reference for an RPC.
func (c *DCPResolver[C]) Resolve(ctx context.Context) (C, error) {
	var zero C
	e, err := c.Acquire(ctx)
	if err != nil {
		return zero, err
	}
	e.Release()
	return e.Client, nil
}
