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
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// dcpStateActive means the entry is eligible for new picks.
	dcpStateActive int32 = iota
	// dcpStateDraining means the entry was removed from the active slice and is
	// only serving operations that already hold a reference to it.
	dcpStateDraining
	// dcpStateClosed means the entry has been closed and its metric slot returned.
	dcpStateClosed
)

// DynamicChannelSelectionStrategy controls how DCP chooses an active channel.
type DynamicChannelSelectionStrategy int

const (
	// DCPPowerOfTwoLeastBusy compares two random active channels and returns the
	// lower weighted-load channel. It falls back to a full scan if random picks
	// only find draining entries.
	DCPPowerOfTwoLeastBusy DynamicChannelSelectionStrategy = iota
	// DCPRoundRobin cycles through active channels and skips draining entries.
	DCPRoundRobin
)

// DynamicChannelPoolConfig holds the knobs for Spanner dynamic channel pool.
// Zero values use DefaultDynamicChannelPoolConfig unless noted otherwise.
type DynamicChannelPoolConfig struct {
	DCPEnabled         bool // DCPEnabled opts the client into dynamic channel pool.
	DCPInitialChannels int  // DCPInitialChannels is the number of channels created at client startup.
	DCPMinChannels     int  // DCPMinChannels is the lower bound retained during scale-down.
	DCPMaxChannels     int  // DCPMaxChannels is the upper bound created during scale-up.

	// DCPMaxRPCPerChannel triggers event-driven scale-up when per-channel load or
	// average load exceeds this value.
	DCPMaxRPCPerChannel float64
	// DCPMinRPCPerChannel is the low-load threshold used by scale-down checks.
	DCPMinRPCPerChannel float64

	DCPScaleDownCheckInterval            time.Duration // DCPScaleDownCheckInterval controls periodic downscale evaluation.
	DCPScaleUpCooldown                   time.Duration // DCPScaleUpCooldown prevents repeated scale-up bursts.
	DCPDownscaleConsecutiveLowLoadChecks int           // DCPDownscaleConsecutiveLowLoadChecks debounces scale-down.
	DCPMaxScaleUpPercent                 int           // DCPMaxScaleUpPercent caps channels added per scale-up event.
	DCPMaxRemoveChannels                 int           // DCPMaxRemoveChannels caps channels marked draining per scale-down.
	DCPDrainIdleGrace                    time.Duration // DCPDrainIdleGrace keeps an idle drained entry briefly before close.
	DCPPrimeTimeout                      time.Duration // DCPPrimeTimeout bounds the SELECT 1 priming attempt for scaled-up channels.
	DCPPrimeMaxAttempts                  int           // DCPPrimeMaxAttempts bounds scaled-up channel priming retries.
	DCPSelectionStrategy                 DynamicChannelSelectionStrategy
}

// DefaultDynamicChannelPoolConfig returns the default DCP settings.
func DefaultDynamicChannelPoolConfig() DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		DCPInitialChannels:                   4,
		DCPMinChannels:                       4,
		DCPMaxChannels:                       256,
		DCPMaxRPCPerChannel:                  50,
		DCPMinRPCPerChannel:                  5,
		DCPScaleDownCheckInterval:            30 * time.Second,
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

// normalizeDCPConfig fills zero-value knobs and validates internal consistency.
func normalizeDCPConfig(cfg DynamicChannelPoolConfig) (DynamicChannelPoolConfig, error) {
	def := DefaultDynamicChannelPoolConfig()
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
	case cfg.DCPMaxScaleUpPercent <= 0 || cfg.DCPMaxScaleUpPercent > 100:
		return cfg, fmt.Errorf("DCPMaxScaleUpPercent must be in (0,100]")
	case cfg.DCPMaxRemoveChannels <= 0:
		return cfg, fmt.Errorf("DCPMaxRemoveChannels must be positive")
	case cfg.DCPSelectionStrategy != DCPPowerOfTwoLeastBusy && cfg.DCPSelectionStrategy != DCPRoundRobin:
		return cfg, fmt.Errorf("DCPSelectionStrategy must be DCPPowerOfTwoLeastBusy or DCPRoundRobin")
	}
	return cfg, nil
}

// dynamicChannelPool owns the copy-on-write slice of DCP entries and the
// background scaling/draining loops.
type dynamicChannelPool struct {
	entries             atomic.Pointer[[]*dcpEntry]
	cfg                 DynamicChannelPoolConfig
	targetRPCPerChannel float64

	ctx                  context.Context
	cancel               context.CancelFunc
	sc                   *sessionClient
	database             string
	disableRouteToLeader bool

	dial          func(context.Context) (gtransport.ConnPool, error)
	fallbackDial  func(context.Context) (gtransport.ConnPool, error)
	fallbackState *dcpFallbackState

	rrIndex        atomic.Uint64
	nextID         atomic.Uint64
	totalRPCLoad   atomic.Int32
	dialMu         sync.Mutex
	metricSlotMu   sync.Mutex
	freeMetricSlot []int64
	nextMetricSlot int64
	lastScaleUp    atomic.Int64
	scaleUpSignal  chan struct{}
	done           chan struct{}
	stopOnce       sync.Once
	lowLoadRuns    int
	monitorMu      sync.Mutex
	primeSession   atomic.Value // string

	drainingCount atomic.Int64
}

// dcpEntry represents one logical DCP slot. In DirectPath fallback mode the
// entry pool is a wrapper containing one DirectPath channel and one CloudPath
// fallback channel.
type dcpEntry struct {
	id           uint64
	metricSlot   int64 // bounded slot id used for metric cardinality
	pool         gtransport.ConnPool
	delegate     spannerClient
	client       spannerClient
	parent       *dynamicChannelPool
	unaryLoad    atomic.Int32
	streamLoad   atomic.Int32
	errorCount   atomic.Int64 // errors since process start; used for debug/diagnostics
	state        atomic.Int32 // dcpState*
	createdAt    atomic.Int64 // UnixNano creation time
	lastActivity atomic.Int64 // UnixNano last pick/RPC/release time
}

// newDynamicChannelPool creates the initial channel set and starts scale workers.
func newDynamicChannelPool(ctx context.Context, sc *sessionClient, cfg DynamicChannelPoolConfig, initial int, dial func(context.Context) (gtransport.ConnPool, error), fallbackDial func(context.Context) (gtransport.ConnPool, error)) (*dynamicChannelPool, error) {
	cfg, err := normalizeDCPConfig(cfg)
	if err != nil {
		return nil, err
	}
	if initial > 0 {
		cfg.DCPInitialChannels = initial
	}
	poolCtx, cancel := context.WithCancel(ctx)
	p := &dynamicChannelPool{
		cfg:                  cfg,
		targetRPCPerChannel:  math.Max(1, math.Floor((cfg.DCPMinRPCPerChannel+cfg.DCPMaxRPCPerChannel)/2)),
		ctx:                  poolCtx,
		cancel:               cancel,
		sc:                   sc,
		database:             sc.database,
		disableRouteToLeader: sc.disableRouteToLeader,
		dial:                 dial,
		fallbackDial:         fallbackDial,
		fallbackState:        &dcpFallbackState{},
		scaleUpSignal:        make(chan struct{}, 1),
		done:                 make(chan struct{}),
	}
	entries := make([]*dcpEntry, 0, cfg.DCPInitialChannels)
	for i := 0; i < cfg.DCPInitialChannels; i++ {
		e, err := p.newEntry(ctx, false)
		if err != nil {
			for _, entry := range entries {
				entry.close()
			}
			cancel()
			return nil, err
		}
		entries = append(entries, e)
	}
	p.entries.Store(&entries)
	go p.scaleUpWorker()
	go p.scaleDownMonitor()
	return p, nil
}

func (p *dynamicChannelPool) Num() int { return len(p.getEntries()) }
func (p *dynamicChannelPool) Conn() *grpc.ClientConn {
	entries := p.getEntries()
	if len(entries) == 0 {
		return nil
	}
	return entries[0].pool.Conn()
}

func (p *dynamicChannelPool) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	e, err := p.pick(ctx)
	if err != nil {
		return err
	}
	e.unaryLoad.Add(1)
	p.totalRPCLoad.Add(1)
	p.maybeSignalScaleUp(e)
	e.lastActivity.Store(time.Now().UnixNano())
	defer func() {
		e.unaryLoad.Add(-1)
		p.totalRPCLoad.Add(-1)
		e.lastActivity.Store(time.Now().UnixNano())
	}()
	err = e.pool.Invoke(ctx, method, args, reply, opts...)
	if err != nil {
		e.errorCount.Add(1)
	}
	return err
}

func (p *dynamicChannelPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	e, err := p.pick(ctx)
	if err != nil {
		return nil, err
	}
	e.streamLoad.Add(1)
	p.totalRPCLoad.Add(1)
	p.maybeSignalScaleUp(e)
	e.lastActivity.Store(time.Now().UnixNano())
	stream, err := e.pool.NewStream(ctx, desc, method, opts...)
	if err != nil {
		e.streamLoad.Add(-1)
		p.totalRPCLoad.Add(-1)
		e.errorCount.Add(1)
		return nil, err
	}
	return &dcpConnPoolTrackedStream{ClientStream: stream, entry: e}, nil
}

func (p *dynamicChannelPool) Close() error {
	p.stopOnce.Do(func() { p.cancel(); close(p.done) })
	p.dialMu.Lock()
	defer p.dialMu.Unlock()
	entries := p.getEntries()
	p.entries.Store(&[]*dcpEntry{})
	var errs []error
	for _, e := range entries {
		if err := e.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *dynamicChannelPool) getEntries() []*dcpEntry {
	ptr := p.entries.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// setPrimeSession records the multiplexed session used for scaled-up channel
// priming. Initial channels are created during client startup and are not
// primed through this path.
func (p *dynamicChannelPool) setPrimeSession(id string) {
	if id != "" {
		p.primeSession.Store(id)
		select {
		case p.scaleUpSignal <- struct{}{}:
		default:
		}
	}
}

// hasPrimeSession reports whether a scaled-up channel can be primed.
func (p *dynamicChannelPool) hasPrimeSession() bool {
	v := p.primeSession.Load()
	if v == nil {
		return false
	}
	sid, _ := v.(string)
	return sid != ""
}

// allocateMetricSlot returns a bounded per-entry metric slot. Slots are reused
// after channel close to avoid unbounded OTel attribute cardinality.
func (p *dynamicChannelPool) allocateMetricSlot() (int64, error) {
	p.metricSlotMu.Lock()
	defer p.metricSlotMu.Unlock()
	n := len(p.freeMetricSlot)
	if n > 0 {
		slot := p.freeMetricSlot[n-1]
		p.freeMetricSlot = p.freeMetricSlot[:n-1]
		return slot, nil
	}
	if p.nextMetricSlot < int64(p.cfg.DCPMaxChannels) {
		p.nextMetricSlot++
		return p.nextMetricSlot, nil
	}
	return 0, spannerErrorf(codes.ResourceExhausted, "spanner_dcp: no metric slots available")
}

// releaseMetricSlot returns a bounded metric slot for later reuse.
func (p *dynamicChannelPool) releaseMetricSlot(slot int64) {
	if slot <= 0 {
		return
	}
	p.metricSlotMu.Lock()
	p.freeMetricSlot = append(p.freeMetricSlot, slot)
	p.metricSlotMu.Unlock()
}

// newEntry dials one DCP entry. When fallbackDial is set, the entry uses a
// DirectPath/CloudPath wrapper but still appears as one logical DCP slot.
func (p *dynamicChannelPool) newEntry(ctx context.Context, prime bool) (*dcpEntry, error) {
	id := p.nextID.Add(1)
	metricSlot, err := p.allocateMetricSlot()
	if err != nil {
		return nil, err
	}
	primary, err := p.dial(ctx)
	if err != nil {
		p.releaseMetricSlot(metricSlot)
		return nil, err
	}
	var entryPool gtransport.ConnPool = primary
	if p.fallbackDial != nil {
		fallback, err := p.fallbackDial(ctx)
		if err != nil {
			primary.Close()
			p.releaseMetricSlot(metricSlot)
			return nil, err
		}
		entryPool = &dcpFallbackSlot{id: id, direct: primary, cloud: fallback, state: p.fallbackState}
	}
	e := &dcpEntry{id: id, metricSlot: metricSlot, pool: entryPool, parent: p}
	now := time.Now().UnixNano()
	e.createdAt.Store(now)
	e.lastActivity.Store(now)
	client, err := newGRPCSpannerClient(ctx, p.sc, id, gtransport.WithConnPool(e))
	if err != nil {
		entryPool.Close()
		p.releaseMetricSlot(metricSlot)
		return nil, err
	}
	e.delegate = client
	e.client = &dcpSpannerClient{entry: e, delegate: client}
	if prime {
		if err := p.prime(ctx, e); err != nil {
			e.close()
			return nil, err
		}
	}
	return e, nil
}

// prime verifies a scaled-up channel before publishing it to the active slice.
// It uses SELECT 1 through the new entry's delegate so failed channels are never
// visible to normal request picking.
func (p *dynamicChannelPool) prime(ctx context.Context, e *dcpEntry) error {
	v := p.primeSession.Load()
	if v == nil {
		return spannerErrorf(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")
	}
	sid, _ := v.(string)
	if sid == "" {
		return spannerErrorf(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")
	}
	stmt := &spannerpb.ExecuteSqlRequest{Session: sid, Sql: "SELECT 1"}
	var last error
	for i := 0; i < p.cfg.DCPPrimeMaxAttempts; i++ {
		primeCtx, cancel := context.WithTimeout(ctx, p.cfg.DCPPrimeTimeout)
		_, last = e.delegate.ExecuteSql(contextWithOutgoingMetadata(primeCtx, p.sc.md, p.disableRouteToLeader), stmt)
		cancel()
		if last == nil {
			p.recordPrimeSuccess()
			return nil
		}
		if i < p.cfg.DCPPrimeMaxAttempts-1 {
			timer := time.NewTimer(time.Duration(100*(1<<i)) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	p.recordPrimeFailure()
	return last
}

// pick selects an active entry.
func (p *dynamicChannelPool) pick(ctx context.Context) (*dcpEntry, error) {
	var e *dcpEntry
	var err error
	if p.cfg.DCPSelectionStrategy == DCPRoundRobin {
		e, err = p.pickRoundRobin()
	} else {
		e, err = p.pickPowerOfTwo()
	}
	if err != nil {
		return nil, err
	}
	p.recordSelection(ctx, e)
	e.lastActivity.Store(time.Now().UnixNano())
	return e, nil
}

func (p *dynamicChannelPool) lookupActive(id uint64) *dcpEntry {
	if id == 0 {
		return nil
	}
	for _, e := range p.getEntries() {
		if e.id == id && e.state.Load() == dcpStateActive {
			return e
		}
	}
	return nil
}

// dcpConnPoolTrackedStream wraps a grpc stream from the low-level ConnPool path
// and decrements stream load when the stream finishes.
type dcpConnPoolTrackedStream struct {
	grpc.ClientStream
	entry *dcpEntry
	once  sync.Once
}

func (s *dcpConnPoolTrackedStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.finish(err)
	}
	return err
}

func (s *dcpConnPoolTrackedStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.finish(err)
	}
	return err
}

func (s *dcpConnPoolTrackedStream) finish(err error) {
	s.once.Do(func() {
		s.entry.streamLoad.Add(-1)
		s.entry.parent.totalRPCLoad.Add(-1)
		s.entry.lastActivity.Store(time.Now().UnixNano())
		if err != nil && !errors.Is(err, io.EOF) {
			s.entry.errorCount.Add(1)
		}
	})
}

var errDCPNoEntries = spannerErrorf(codes.Unavailable, "spanner_dcp: no available channels")

// pickPowerOfTwo selects the lower weighted-load entry from two random active
// entries. It retries when either random choice is draining and falls back to a
// full least-loaded scan if random sampling cannot find an active pair.
func (p *dynamicChannelPool) pickPowerOfTwo() (*dcpEntry, error) {
	entries := p.getEntries()
	n := len(entries)
	if n == 0 {
		return nil, errDCPNoEntries
	}
	if n == 1 {
		if entries[0].isDraining() {
			return nil, errDCPNoEntries
		}
		return entries[0], nil
	}
	for i := 0; i < n*2; i++ {
		e1, e2 := entries[rand.IntN(n)], entries[rand.IntN(n)]
		if e1.isDraining() || e2.isDraining() {
			continue
		}
		if e1.weightedLoad() <= e2.weightedLoad() {
			return e1, nil
		}
		return e2, nil
	}
	return p.pickLeastLoaded()
}

// pickRoundRobin cycles through active entries and skips draining entries.
func (p *dynamicChannelPool) pickRoundRobin() (*dcpEntry, error) {
	entries := p.getEntries()
	n := len(entries)
	if n == 0 {
		return nil, errDCPNoEntries
	}
	for i := 0; i < n; i++ {
		idx := p.rrIndex.Add(1) - 1
		e := entries[int(idx%uint64(n))]
		if !e.isDraining() {
			return e, nil
		}
	}
	return nil, errDCPNoEntries
}

// pickLeastLoaded returns the active entry with the lowest weighted load.
func (p *dynamicChannelPool) pickLeastLoaded() (*dcpEntry, error) {
	var best *dcpEntry
	min := int32(math.MaxInt32)
	for _, e := range p.getEntries() {
		if e.isDraining() {
			continue
		}
		l := e.weightedLoad()
		if l < min {
			min = l
			best = e
		}
	}
	if best == nil {
		return nil, errDCPNoEntries
	}
	return best, nil
}

// maybeSignalScaleUp notifies the scale-up worker when the selected channel or
// average pool load exceeds DCPMaxRPCPerChannel. The signal channel is buffered
// so many hot requests collapse into one scale-up evaluation.
func (p *dynamicChannelPool) maybeSignalScaleUp(e *dcpEntry) {
	active := p.Num()
	avg := float64(0)
	if active > 0 {
		avg = float64(p.totalRPCLoad.Load()) / float64(active)
	}
	if float64(e.rpcLoad()) <= p.cfg.DCPMaxRPCPerChannel && avg <= p.cfg.DCPMaxRPCPerChannel {
		return
	}
	select {
	case p.scaleUpSignal <- struct{}{}:
	default:
	}
}

// scaleUpWorker serializes event-driven scale-up requests.
func (p *dynamicChannelPool) scaleUpWorker() {
	for {
		select {
		case <-p.done:
			return
		case <-p.scaleUpSignal:
			p.scaleUp()
		}
	}
}

// scaleUp adds and primes channels based on current total load. The new entries
// are published only after successful dial and SELECT 1 priming.
func (p *dynamicChannelPool) scaleUp() {
	select {
	case <-p.done:
		return
	default:
	}
	now := time.Now()
	last := time.Unix(0, p.lastScaleUp.Load())
	if !last.IsZero() && now.Sub(last) < p.cfg.DCPScaleUpCooldown {
		return
	}
	p.dialMu.Lock()
	defer p.dialMu.Unlock()
	if p.ctx.Err() != nil {
		return
	}
	if !p.hasPrimeSession() {
		return
	}
	entries := p.getEntries()
	active := 0
	var load int32
	for _, e := range entries {
		if !e.isDraining() {
			active++
			load += e.rpcLoad()
		}
	}
	if active == 0 {
		return
	}
	desired := int(math.Ceil(float64(load) / p.targetRPCPerChannel))
	add := desired - active
	capPct := int(math.Ceil(float64(active) * float64(p.cfg.DCPMaxScaleUpPercent) / 100))
	if add > capPct {
		add = capPct
	}
	if maxAdd := p.cfg.DCPMaxChannels - len(entries); add > maxAdd {
		add = maxAdd
	}
	if add <= 0 {
		return
	}
	newEntries := make([]*dcpEntry, 0, add)
	for i := 0; i < add; i++ {
		e, err := p.newEntry(p.ctx, true)
		if err == nil {
			newEntries = append(newEntries, e)
		} else {
			logf(p.sc.logger, "spanner_dcp: failed to create or prime scaled-up channel: %v", err)
		}
	}
	if len(newEntries) == 0 {
		return
	}
	combined := make([]*dcpEntry, 0, len(entries)+len(newEntries))
	combined = append(combined, entries...)
	combined = append(combined, newEntries...)
	p.entries.Store(&combined)
	p.lastScaleUp.Store(now.UnixNano())
	p.recordScaleUp(len(newEntries))
}

// scaleDownMonitor periodically evaluates whether sustained low load can drain
// channels.
func (p *dynamicChannelPool) scaleDownMonitor() {
	t := time.NewTicker(p.cfg.DCPScaleDownCheckInterval)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.evaluateScaleDown()
		}
	}
}

// evaluateScaleDown debounces low-load observations before removing channels.
func (p *dynamicChannelPool) evaluateScaleDown() {
	p.monitorMu.Lock()
	defer p.monitorMu.Unlock()
	entries := p.getEntries()
	active := 0
	var load int32
	for _, e := range entries {
		if !e.isDraining() {
			active++
			load += e.rpcLoad()
		}
	}
	if active == 0 {
		return
	}
	avg := float64(load) / float64(active)
	if avg > p.cfg.DCPMinRPCPerChannel {
		p.lowLoadRuns = 0
		return
	}
	p.lowLoadRuns++
	if p.lowLoadRuns < p.cfg.DCPDownscaleConsecutiveLowLoadChecks {
		return
	}
	p.lowLoadRuns = 0
	desired := int(math.Ceil(float64(load) / p.targetRPCPerChannel))
	if desired < p.cfg.DCPMinChannels {
		desired = p.cfg.DCPMinChannels
	}
	remove := active - desired
	if remove <= 0 {
		return
	}
	if remove > p.cfg.DCPMaxRemoveChannels {
		remove = p.cfg.DCPMaxRemoveChannels
	}
	p.removeEntries(remove)
}

// removeEntries revalidates low load under dialMu, removes selected entries from
// the active slice, and starts graceful drain goroutines.
func (p *dynamicChannelPool) removeEntries(count int) {
	p.dialMu.Lock()
	entries := p.getEntries()
	active := 0
	var load int32
	type candidate struct {
		e       *dcpEntry
		created int64
		load    int32
	}
	candidates := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if !e.isDraining() {
			active++
			load += e.rpcLoad()
			candidates = append(candidates, candidate{e, e.createdAt.Load(), e.weightedLoad()})
		}
	}
	if active == 0 {
		p.dialMu.Unlock()
		return
	}
	avg := float64(load) / float64(active)
	if avg > p.cfg.DCPMinRPCPerChannel {
		p.dialMu.Unlock()
		return
	}
	desired := int(math.Ceil(float64(load) / p.targetRPCPerChannel))
	if desired < p.cfg.DCPMinChannels {
		desired = p.cfg.DCPMinChannels
	}
	recomputed := active - desired
	if recomputed <= 0 {
		p.dialMu.Unlock()
		return
	}
	if count > recomputed {
		count = recomputed
	}
	if count > active-p.cfg.DCPMinChannels {
		count = active - p.cfg.DCPMinChannels
	}
	if count <= 0 {
		p.dialMu.Unlock()
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].load != candidates[j].load {
			return candidates[i].load < candidates[j].load
		}
		return candidates[i].created < candidates[j].created
	})
	toDrain := make(map[*dcpEntry]bool)
	for i := 0; i < count && i < len(candidates); i++ {
		candidates[i].e.state.Store(dcpStateDraining)
		toDrain[candidates[i].e] = true
	}
	keep := make([]*dcpEntry, 0, len(entries)-len(toDrain))
	for _, e := range entries {
		if !toDrain[e] {
			keep = append(keep, e)
		}
	}
	p.entries.Store(&keep)
	p.dialMu.Unlock()
	p.drainingCount.Add(int64(len(toDrain)))
	p.recordScaleDown(len(toDrain))
	for e := range toDrain {
		go p.waitForDrainAndClose(e)
	}
}

// waitForDrainAndClose waits until a draining entry has no RPC load and has
// been idle for DCPDrainIdleGrace.
func (p *dynamicChannelPool) waitForDrainAndClose(e *dcpEntry) {
	start := time.Now()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if e.rpcLoad() == 0 && time.Since(time.Unix(0, e.lastActivity.Load())) >= p.cfg.DCPDrainIdleGrace {
				e.close()
				p.drainingCount.Add(-1)
				p.recordDrainWait(time.Since(start))
				return
			}
		case <-p.ctx.Done():
			if e.client != nil {
				e.close()
			} else if e.pool != nil {
				e.pool.Close()
			}
			p.drainingCount.Add(-1)
			p.recordDrainWait(time.Since(start))
			return
		}
	}
}

func (e *dcpEntry) Conn() *grpc.ClientConn { return e.pool.Conn() }
func (e *dcpEntry) Num() int               { return 1 }
func (e *dcpEntry) Close() error           { return e.close() }

func (e *dcpEntry) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	return e.pool.Invoke(ctx, method, args, reply, opts...)
}

func (e *dcpEntry) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return e.pool.NewStream(ctx, desc, method, opts...)
}

func (e *dcpEntry) close() error {
	if !e.state.CompareAndSwap(dcpStateActive, dcpStateClosed) && !e.state.CompareAndSwap(dcpStateDraining, dcpStateClosed) {
		return nil
	}
	var errs []error
	if e.client != nil {
		errs = append(errs, e.client.Close())
	}
	if e.pool != nil {
		errs = append(errs, e.pool.Close())
	}
	e.parent.releaseMetricSlot(e.metricSlot)
	return errors.Join(errs...)
}

// isDraining atomically checks whether the entry has been removed from normal
// selection and is waiting for in-flight operations to finish.
func (e *dcpEntry) isDraining() bool { return e.state.Load() == dcpStateDraining }

// rpcLoad returns the current in-flight RPC load for this entry.
func (e *dcpEntry) rpcLoad() int32 { return e.unaryLoad.Load() + e.streamLoad.Load() }

// weightedLoad returns the current in-flight RPC load for this entry.
func (e *dcpEntry) weightedLoad() int32 { return e.rpcLoad() }

func (e *dcpEntry) applyPenalty(ctx context.Context, err error) {}

// dcpFallbackState is shared by all DirectPath fallback slots in the pool so a
// primary DirectPath outage can move the whole DCP wrapper pool to CloudPath.
type dcpFallbackState struct {
	fallbackActive   atomic.Bool
	primarySuccesses atomic.Uint64
	primaryFailures  atomic.Uint64
	lastPrimaryReset atomic.Int64
}

// dcpFallbackSlot is one logical DCP slot backed by two physical channels:
// DirectPath for the primary path and CloudPath for fallback.
type dcpFallbackSlot struct {
	id     uint64
	direct gtransport.ConnPool
	cloud  gtransport.ConnPool
	state  *dcpFallbackState
}

func (s *dcpFallbackSlot) Conn() *grpc.ClientConn {
	if s.state.fallbackActive.Load() {
		return s.cloud.Conn()
	}
	return s.direct.Conn()
}

func (s *dcpFallbackSlot) Num() int { return 1 }

func (s *dcpFallbackSlot) Close() error {
	e1 := s.direct.Close()
	e2 := s.cloud.Close()
	return errors.Join(e1, e2)
}

func (s *dcpFallbackSlot) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	if s.state.fallbackActive.Load() {
		err := s.cloud.Invoke(ctx, method, args, reply, opts...)
		s.recordFallback(err)
		return err
	}
	err := s.direct.Invoke(ctx, method, args, reply, opts...)
	s.recordPrimary(err)
	return err
}

func (s *dcpFallbackSlot) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if s.state.fallbackActive.Load() {
		st, err := s.cloud.NewStream(ctx, desc, method, opts...)
		if err != nil {
			s.recordFallback(err)
			return st, err
		}
		return &dcpFallbackMonitoredStream{ClientStream: st, record: s.recordFallback}, nil
	}
	st, err := s.direct.NewStream(ctx, desc, method, opts...)
	if err != nil {
		s.recordPrimary(err)
		return st, err
	}
	return &dcpFallbackMonitoredStream{ClientStream: st, record: s.recordPrimary}, nil
}

// recordPrimary updates shared DirectPath health counters.
func (s *dcpFallbackSlot) recordPrimary(err error) {
	s.resetPrimaryFallbackWindowIfNeeded()
	if isDCPFallbackFailure(err) {
		s.state.primaryFailures.Add(1)
		s.maybeActivateFallback()
	} else {
		s.state.primarySuccesses.Add(1)
	}
}

// resetPrimaryFallbackWindowIfNeeded keeps DCP fallback activation counters on
// the same time window as the non-DCP grpc-gcp fallback Period.
func (s *dcpFallbackSlot) resetPrimaryFallbackWindowIfNeeded() {
	now := time.Now().UnixNano()
	last := s.state.lastPrimaryReset.Load()
	if last == 0 {
		s.state.lastPrimaryReset.CompareAndSwap(0, now)
		return
	}
	if time.Duration(now-last) < directPathFallbackPeriod {
		return
	}
	if s.state.lastPrimaryReset.CompareAndSwap(last, now) {
		s.state.primaryFailures.Store(0)
		s.state.primarySuccesses.Store(0)
	}
}

// recordFallback is intentionally a no-op. DCP DirectPath fallback is sticky
// once activated, matching non-DCP grpc-gcp fallback behavior.
func (s *dcpFallbackSlot) recordFallback(err error) {
}

// maybeActivateFallback enables CloudPath after enough DirectPath samples show a
// sustained Unavailable rate. The activation threshold, minimum failed calls,
// and counter window intentionally mirror the non-DCP grpc-gcp fallback config.
func (s *dcpFallbackSlot) maybeActivateFallback() {
	failures := s.state.primaryFailures.Load()
	successes := s.state.primarySuccesses.Load()
	total := failures + successes
	if total == 0 || failures < uint64(directPathFallbackMinFailedCalls) {
		return
	}
	if float32(failures)/float32(total) < directPathFallbackErrorRateThreshold {
		return
	}
	s.state.fallbackActive.Store(true)
}

// isDCPFallbackFailure returns true for errors that should move DirectPath
// traffic to CloudPath fallback.
func isDCPFallbackFailure(err error) bool {
	c := status.Code(err)
	return c == codes.Unavailable
}

type dcpFallbackMonitoredStream struct {
	grpc.ClientStream
	once   sync.Once
	record func(error)
}

func (s *dcpFallbackMonitoredStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.once.Do(func() {
			if errors.Is(err, io.EOF) {
				s.record(nil)
			} else {
				s.record(err)
			}
		})
	}
	return err
}

func (s *dcpFallbackMonitoredStream) CloseSend() error {
	return s.ClientStream.CloseSend()
}

func (p *dynamicChannelPool) recordScaleUp(added int) {}

func (p *dynamicChannelPool) recordScaleDown(draining int) {}

func (p *dynamicChannelPool) recordDrainWait(d time.Duration) {}

func (p *dynamicChannelPool) recordSelection(ctx context.Context, e *dcpEntry) {}

func (p *dynamicChannelPool) recordErrorPenalty(ctx context.Context) {}

func (p *dynamicChannelPool) recordPrimeSuccess() {}

func (p *dynamicChannelPool) recordPrimeFailure() {}

type dcpSpannerClient struct {
	entry    *dcpEntry
	delegate spannerClient
}

func (c *dcpSpannerClient) CallOptions() *vkit.CallOptions { return c.delegate.CallOptions() }
func (c *dcpSpannerClient) Close() error                   { return c.delegate.Close() }
func (c *dcpSpannerClient) Connection() *grpc.ClientConn   { return c.delegate.Connection() }

func (c *dcpSpannerClient) startUnary(ctx context.Context) func(error) {
	c.entry.unaryLoad.Add(1)
	c.entry.parent.totalRPCLoad.Add(1)
	c.entry.parent.maybeSignalScaleUp(c.entry)
	c.entry.lastActivity.Store(time.Now().UnixNano())
	return func(err error) {
		c.entry.unaryLoad.Add(-1)
		c.entry.parent.totalRPCLoad.Add(-1)
		c.entry.lastActivity.Store(time.Now().UnixNano())
		if err != nil {
			c.entry.errorCount.Add(1)
		}
	}
}

type dcpStreamRef struct {
	once   sync.Once
	finish func(error)
	closed chan struct{}
}

func (r *dcpStreamRef) done(err error) {
	r.once.Do(func() {
		r.finish(err)
		close(r.closed)
	})
}

func (c *dcpSpannerClient) startStream(ctx context.Context) *dcpStreamRef {
	c.entry.streamLoad.Add(1)
	c.entry.parent.totalRPCLoad.Add(1)
	c.entry.parent.maybeSignalScaleUp(c.entry)
	c.entry.lastActivity.Store(time.Now().UnixNano())
	ref := &dcpStreamRef{closed: make(chan struct{}), finish: func(err error) {
		c.entry.streamLoad.Add(-1)
		c.entry.parent.totalRPCLoad.Add(-1)
		c.entry.lastActivity.Store(time.Now().UnixNano())
		if err != nil && !errors.Is(err, io.EOF) {
			c.entry.errorCount.Add(1)
		}
	}}
	if ctx != nil && ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				ref.done(ctx.Err())
			case <-ref.closed:
			}
		}()
	}
	return ref
}

func (c *dcpSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.CreateSession(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.BatchCreateSessions(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.GetSession(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	iter := c.delegate.ListSessions(ctx, req, opts...)
	if iter != nil && iter.InternalFetch != nil {
		fetch := iter.InternalFetch
		iter.InternalFetch = func(pageSize int, pageToken string) ([]*spannerpb.Session, string, error) {
			done := c.startUnary(ctx)
			results, nextPageToken, err := fetch(pageSize, pageToken)
			done(err)
			return results, nextPageToken, err
		}
	}
	return iter
}

func (c *dcpSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	done := c.startUnary(ctx)
	err := c.delegate.DeleteSession(ctx, req, opts...)
	done(err)
	return err
}

func (c *dcpSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.ExecuteSql(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		ref.done(err)
		return nil, err
	}
	return &dcpExecuteStreamingSqlClient{Spanner_ExecuteStreamingSqlClient: stream, ref: ref}, nil
}

func (c *dcpSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.ExecuteBatchDml(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.Read(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.StreamingRead(ctx, req, opts...)
	if err != nil {
		ref.done(err)
		return nil, err
	}
	return &dcpStreamingReadClient{Spanner_StreamingReadClient: stream, ref: ref}, nil
}

func (c *dcpSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.BeginTransaction(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.Commit(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	done := c.startUnary(ctx)
	err := c.delegate.Rollback(ctx, req, opts...)
	done(err)
	return err
}

func (c *dcpSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.PartitionQuery(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.PartitionRead(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.BatchWrite(ctx, req, opts...)
	if err != nil {
		ref.done(err)
		return nil, err
	}
	return &dcpBatchWriteClient{Spanner_BatchWriteClient: stream, ref: ref}, nil
}

type dcpExecuteStreamingSqlClient struct {
	spannerpb.Spanner_ExecuteStreamingSqlClient
	ref *dcpStreamRef
}

func (c *dcpExecuteStreamingSqlClient) Recv() (*spannerpb.PartialResultSet, error) {
	resp, err := c.Spanner_ExecuteStreamingSqlClient.Recv()
	if err != nil {
		c.ref.done(err)
	}
	return resp, err
}

func (c *dcpExecuteStreamingSqlClient) CloseSend() error {
	err := c.Spanner_ExecuteStreamingSqlClient.CloseSend()
	if err != nil {
		c.ref.done(err)
	}
	return err
}

type dcpStreamingReadClient struct {
	spannerpb.Spanner_StreamingReadClient
	ref *dcpStreamRef
}

func (c *dcpStreamingReadClient) Recv() (*spannerpb.PartialResultSet, error) {
	resp, err := c.Spanner_StreamingReadClient.Recv()
	if err != nil {
		c.ref.done(err)
	}
	return resp, err
}

func (c *dcpStreamingReadClient) CloseSend() error {
	err := c.Spanner_StreamingReadClient.CloseSend()
	if err != nil {
		c.ref.done(err)
	}
	return err
}

type dcpBatchWriteClient struct {
	spannerpb.Spanner_BatchWriteClient
	ref *dcpStreamRef
}

func (c *dcpBatchWriteClient) Recv() (*spannerpb.BatchWriteResponse, error) {
	resp, err := c.Spanner_BatchWriteClient.Recv()
	if err != nil {
		c.ref.done(err)
	}
	return resp, err
}

func (c *dcpBatchWriteClient) CloseSend() error {
	err := c.Spanner_BatchWriteClient.CloseSend()
	if err != nil {
		c.ref.done(err)
	}
	return err
}

type dcpResolvingSpannerClient struct {
	pool    *dynamicChannelPool
	entryID atomic.Uint64
}

func newDCPResolvingSpannerClient(pool *dynamicChannelPool, entryID uint64) *dcpResolvingSpannerClient {
	c := &dcpResolvingSpannerClient{pool: pool}
	c.entryID.Store(entryID)
	return c
}

func (c *dcpResolvingSpannerClient) resolve(ctx context.Context) (spannerClient, error) {
	if c == nil || c.pool == nil {
		return nil, errDCPNoEntries
	}
	if e := c.pool.lookupActive(c.entryID.Load()); e != nil {
		c.pool.recordSelection(ctx, e)
		e.lastActivity.Store(time.Now().UnixNano())
		return e.client, nil
	}
	e, err := c.pool.pick(ctx)
	if err != nil {
		return nil, err
	}
	c.entryID.Store(e.id)
	return e.client, nil
}

func (c *dcpResolvingSpannerClient) CallOptions() *vkit.CallOptions {
	client, err := c.resolve(context.Background())
	if err != nil || client == nil {
		return &vkit.CallOptions{}
	}
	return client.CallOptions()
}

func (c *dcpResolvingSpannerClient) Close() error { return nil }

func (c *dcpResolvingSpannerClient) Connection() *grpc.ClientConn {
	client, err := c.resolve(context.Background())
	if err != nil || client == nil {
		return nil
	}
	return client.Connection()
}

func (c *dcpResolvingSpannerClient) generateRequestIDHeaderInjector() *requestIDWrap {
	client, err := c.resolve(context.Background())
	if err != nil || client == nil {
		return nil
	}
	gsc := asGRPCSpannerClient(client)
	if gsc == nil {
		return nil
	}
	return gsc.generateRequestIDHeaderInjector()
}

func (c *dcpResolvingSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.CreateSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.BatchCreateSessions(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	client, err := c.resolve(ctx)
	if err != nil {
		return &vkit.SessionIterator{}
	}
	return client.ListSessions(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	client, err := c.resolve(ctx)
	if err != nil {
		return err
	}
	return client.DeleteSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.ExecuteSql(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.ExecuteStreamingSql(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.ExecuteBatchDml(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.Read(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.StreamingRead(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.BeginTransaction(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.Commit(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	client, err := c.resolve(ctx)
	if err != nil {
		return err
	}
	return client.Rollback(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.PartitionQuery(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.PartitionRead(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	client, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return client.BatchWrite(ctx, req, opts...)
}
