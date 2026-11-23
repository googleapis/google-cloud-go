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

const (
	// DCPStateActive means the entry is eligible for new picks.
	DCPStateActive int32 = iota
	// DCPStateDraining means the entry was removed from the active slice and is
	// only serving operations that already hold a reference to it, including
	// session handles (and so read-write transactions) bound to it.
	DCPStateDraining
	// DCPStateClosed means the entry's channel has been closed. It is final.
	DCPStateClosed
)

// dcpRefsClosed is the DCPEntry.Refs value of a closed entry. Every negative
// value means closed; the magnitude leaves room for late acquire and release
// calls.
const dcpRefsClosed = math.MinInt64 / 2

// DCPDrainIdleFloor is the minimum time a draining entry must be idle before it
// closes, whatever DCPDrainIdleGrace is. Tests lower it before creating pools.
var DCPDrainIdleFloor = 15 * time.Second

var (
	defaultDCPNoEntriesErr     = status.Error(codes.Unavailable, "spanner_dcp: no available channels")
	defaultDCPPrimeNotReadyErr = status.Error(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")
)

// DCPSelectionStrategy controls how DCP chooses an active channel.
type DCPSelectionStrategy int

const (
	// DCPPowerOfTwoLeastBusy compares two random active channels and returns the
	// channel with lower picker load, including any active error penalty. It falls
	// back to a full scan if random picks only find draining entries.
	DCPPowerOfTwoLeastBusy DCPSelectionStrategy = iota
	// DCPRoundRobin cycles through active channels and skips draining entries.
	DCPRoundRobin
)

// DCPConfig holds the configuration knobs for a DynamicChannelPool.
// Its fields mirror spanner.DynamicChannelPoolConfig 1-for-1 (see that type for
// detailed knob semantics). DCPEnabled is retained for lossless config conversion;
// callers gate whether to construct a DynamicChannelPool.
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
	// Equality rejected: needs non-empty hysteresis band. Otherwise scale-up
	// settles target at the same boundary that triggers it, risking immediate
	// scale-down qualification and flapping.
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
	comparable
	Close() error
}

// DCPMetricsRecorder records DynamicChannelPool scale-up/scale-down events and closes metric registrations.
type DCPMetricsRecorder interface {
	RecordScaleUp(ctx context.Context, channels int64)
	RecordScaleDown(ctx context.Context, channels int64)
	Close(logger *log.Logger)
}

// DCPHooks configures the callbacks, lifecycle hooks, and error sentinels for
// NewDynamicChannelPool and NewFixedChannelPool.
type DCPHooks[C DCPClient] struct {
	// Logger receives scaled-up channel dial/priming failure warnings.
	// When nil, warnings are written via the standard log.Printf logger.
	Logger *log.Logger
	// Dial (required by NewDynamicChannelPool; unused by NewFixedChannelPool) dials
	// a single-channel gtransport.ConnPool for a new DCPEntry.
	Dial func(context.Context) (gtransport.ConnPool, error)
	// NewEntryClient (required) initializes e.Client (and optionally e.Delegate for
	// use by PrimeRPC, or wraps e.Pool) for a newly dialed DCPEntry. Returning nil
	// without setting e.Client to a non-zero value fails entry creation and closes
	// the entry.
	NewEntryClient func(ctx context.Context, e *DCPEntry[C]) error
	// PrimeRPC (required by NewDynamicChannelPool; unused by NewFixedChannelPool)
	// verifies a scaled-up DCPEntry against sessionID before the entry is published
	// to the active pool.
	PrimeRPC func(ctx context.Context, e *DCPEntry[C], sessionID string) error
	// InitMetrics optionally registers OpenTelemetry pool metrics after initial
	// entries are created. When nil (or returning nil), metrics are disabled.
	InitMetrics func(p *DynamicChannelPool[C]) DCPMetricsRecorder
	// NoEntriesErr overrides the Unavailable error returned when no active entries
	// can be picked. When nil, a default codes.Unavailable status error is returned.
	NoEntriesErr error
	// PrimeNotReadyErr overrides the FailedPrecondition error used when priming is
	// attempted before SetPrimeSession. When nil, a default status error is used.
	PrimeNotReadyErr error
	// OnClose optionally runs once, when the pool is closed.
	OnClose func()
	// PenaltyEpoch optionally returns a number. An error does not penalize its
	// entry if the number changed while the operation was in flight.
	PenaltyEpoch func() uint64
	// OnCallStart optionally runs when a call starts on an entry of a
	// non-fixed pool, with the entry's in-flight call count including it.
	OnCallStart func(ctx context.Context, load int32)
}

// DynamicChannelPool owns the copy-on-write slice of DCP entries and the
// background scaling/draining loops.
//
// Cfg, TotalRPCLoad, and DrainingCount are read by spanner OpenTelemetry metrics
// (spanner/dcp_metrics.go); remaining exported fields (such as Entries, Dial,
// DialMu, ScaleUpSignal, LastScaleUp, and PrimeSession) are exported for in-module
// white-box tests and must not be mutated by production callers.
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
	onClose          func()
	penaltyEpoch     func() uint64
	onCallStart      func(ctx context.Context, load int32)

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
	DrainIdleFloor time.Duration // DCPDrainIdleFloor when the pool was created

	fixed      bool                // constant round-robin entries; no scaling, priming, or metrics
	underlying gtransport.ConnPool // owns a fixed pool's connections
}

// DCPEntry represents one logical DCP slot.
//
// Production callers populate Client (and optionally Delegate for PrimeRPC or
// wrap Pool) inside DCPHooks.NewEntryClient and read ID, Pool, Delegate, and
// Client; remaining fields are exported for in-module white-box tests.
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
	penaltyEpoch  uint64       // Guarded by penaltyMu; epoch of the current penalty
	Refs          atomic.Int64 // RPCs keeping the channel open; negative once closed
}

// NewDynamicChannelPool creates the initial channel set and starts scale workers.
func NewDynamicChannelPool[C DCPClient](
	ctx context.Context,
	cfg DCPConfig,
	hooks DCPHooks[C],
) (*DynamicChannelPool[C], error) {
	if hooks.Dial == nil || hooks.NewEntryClient == nil || hooks.PrimeRPC == nil {
		return nil, errors.New("spanner_dcp: Dial, NewEntryClient, and PrimeRPC hooks are required")
	}
	cfg, err := NormalizeDCPConfig(cfg)
	if err != nil {
		return nil, err
	}
	p := newEmptyDCP[C](cfg, hooks)
	p.Ctx, p.cancel = context.WithCancel(ctx)
	p.ScaleUpSignal = make(chan struct{}, 1)
	p.done = make(chan struct{})
	if err := p.createInitialEntries(ctx); err != nil {
		p.cancel()
		return nil, err
	}
	if hooks.InitMetrics != nil {
		p.Metrics = hooks.InitMetrics(p)
	}
	go p.scaleUpWorker()
	go p.scaleDownMonitor()
	return p, nil
}

// NewFixedChannelPool returns a fixed-mode DynamicChannelPool with one entry
// per distinct connection in underlying: round-robin selection and no scaling,
// priming, error penalties, or pool metrics. The pool takes ownership of
// underlying and closes it in Close.
func NewFixedChannelPool[C DCPClient](
	ctx context.Context,
	underlying gtransport.ConnPool,
	hooks DCPHooks[C],
) (*DynamicChannelPool[C], error) {
	if underlying == nil || hooks.NewEntryClient == nil {
		return nil, errors.New("spanner_dcp: underlying ConnPool and NewEntryClient hook are required")
	}
	// Deduplicate so each connection has one request-id channel id.
	var conns []*grpc.ClientConn
	seen := make(map[*grpc.ClientConn]bool)
	for i := 0; i < underlying.Num(); i++ {
		c := underlying.Conn()
		if c != nil {
			if seen[c] {
				continue
			}
			seen[c] = true
		}
		conns = append(conns, c)
	}
	n := len(conns)
	cfg, err := NormalizeDCPConfig(DCPConfig{
		DCPInitialChannels:   n,
		DCPMinChannels:       n,
		DCPMaxChannels:       n,
		DCPSelectionStrategy: DCPRoundRobin,
	})
	if err != nil {
		return nil, err
	}
	// next needs no lock: createInitialEntries calls dial n times, in sequence.
	next := 0
	hooks.Dial = func(context.Context) (gtransport.ConnPool, error) {
		c := conns[next]
		next++
		if c == nil {
			// underlying hides its connections, so the entry routes through it.
			return borrowedPool{underlying}, nil
		}
		return borrowedConn{c}, nil
	}
	p := newEmptyDCP[C](cfg, hooks)
	p.fixed = true
	p.underlying = underlying
	if err := p.createInitialEntries(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// newEmptyDCP returns a pool with no entries and no background state.
func newEmptyDCP[C DCPClient](cfg DCPConfig, hooks DCPHooks[C]) *DynamicChannelPool[C] {
	return &DynamicChannelPool[C]{
		Cfg:                 cfg,
		TargetRPCPerChannel: math.Max(1, math.Floor((cfg.DCPMinRPCPerChannel+cfg.DCPMaxRPCPerChannel)/2)),
		PenaltyMax:          int32(math.Ceil(cfg.DCPMaxRPCPerChannel)),
		logger:              hooks.Logger,
		Dial:                hooks.Dial,
		newEntryClient:      hooks.NewEntryClient,
		primeRPC:            hooks.PrimeRPC,
		noEntriesErr:        hooks.NoEntriesErr,
		primeNotReadyErr:    hooks.PrimeNotReadyErr,
		onClose:             hooks.OnClose,
		penaltyEpoch:        hooks.PenaltyEpoch,
		onCallStart:         hooks.OnCallStart,
		DrainIdleFloor:      DCPDrainIdleFloor,
	}
}

func (p *DynamicChannelPool[C]) createInitialEntries(ctx context.Context) error {
	entries := make([]*DCPEntry[C], 0, p.Cfg.DCPInitialChannels)
	for i := 0; i < p.Cfg.DCPInitialChannels; i++ {
		e, err := p.newEntry(ctx, false)
		if err != nil {
			for _, entry := range entries {
				entry.CloseNow()
			}
			return err
		}
		entries = append(entries, e)
	}
	p.Entries.Store(&entries)
	return nil
}

// IsFixed reports whether this pool runs in fixed mode (constant round-robin
// entries with no background scaling, priming, or metrics).
func (p *DynamicChannelPool[C]) IsFixed() bool {
	return p != nil && p.fixed
}

func (p *DynamicChannelPool[C]) logf(format string, v ...any) {
	if p.logger == nil {
		log.Printf(format, v...)
	} else {
		p.logger.Printf(format, v...)
	}
}

func (p *DynamicChannelPool[C]) noEntriesError() error {
	if p != nil && p.noEntriesErr != nil {
		return p.noEntriesErr
	}
	return defaultDCPNoEntriesErr
}

func (p *DynamicChannelPool[C]) primeNotReadyError() error {
	if p != nil && p.primeNotReadyErr != nil {
		return p.primeNotReadyErr
	}
	return defaultDCPPrimeNotReadyErr
}

func (p *DynamicChannelPool[C]) currentPenaltyEpoch() uint64 {
	if p.penaltyEpoch == nil {
		return 0
	}
	return p.penaltyEpoch()
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
	if p.fixed {
		return e.Pool.Invoke(ctx, method, args, reply, opts...)
	}
	e.UnaryLoad.Add(1)
	p.TotalRPCLoad.Add(1)
	p.callStarted(ctx, e)
	p.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	defer func() {
		e.UnaryLoad.Add(-1)
		p.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
	}()
	epoch := p.currentPenaltyEpoch()
	err = e.Pool.Invoke(ctx, method, args, reply, opts...)
	e.applyErrorPenalty(epoch, err)
	return err
}

// NewStream picks an active entry and opens a tracked gRPC client stream on it.
func (p *DynamicChannelPool[C]) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	e, err := p.Pick(ctx)
	if err != nil {
		return nil, err
	}
	if p.fixed {
		return e.Pool.NewStream(ctx, desc, method, opts...)
	}
	e.StreamLoad.Add(1)
	p.TotalRPCLoad.Add(1)
	p.callStarted(ctx, e)
	p.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	epoch := p.currentPenaltyEpoch()
	s, err := e.Pool.NewStream(ctx, desc, method, opts...)
	if err != nil {
		e.applyErrorPenalty(epoch, err)
		e.StreamLoad.Add(-1)
		p.TotalRPCLoad.Add(-1)
		return nil, err
	}
	return &DCPConnPoolTrackedStream[C]{ClientStream: s, Entry: e, penaltyEpoch: epoch}, nil
}

// Close stops background workers, unregisters metrics, and closes all active entries.
func (p *DynamicChannelPool[C]) Close() error {
	p.stopOnce.Do(func() {
		if p.Metrics != nil {
			p.Metrics.Close(p.logger)
		}
		if !p.fixed {
			p.cancel()
			close(p.done)
		}
		if p.onClose != nil {
			p.onClose()
		}
	})
	p.DialMu.Lock()
	defer p.DialMu.Unlock()
	entries := p.GetEntries()
	// A fixed pool keeps its closed entries, so that RPCs after Close fail
	// with CANCELED as they do with the static pool.
	if !p.fixed {
		p.Entries.Store(&[]*DCPEntry[C]{})
	}
	var errs []error
	for _, e := range entries {
		if err := e.CloseNow(); err != nil {
			errs = append(errs, err)
		}
	}
	if p.underlying != nil {
		errs = append(errs, p.underlying.Close())
		p.underlying = nil
	}
	return errors.Join(errs...)
}

// GetEntries returns the current copy-on-write snapshot of active entries.
// Callers must treat the returned slice and its elements as read-only.
func (p *DynamicChannelPool[C]) GetEntries() []*DCPEntry[C] {
	ptr := p.Entries.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// SetPrimeSession records the multiplexed session used for scaled-up channel
// priming. Initial channels are created during client startup and are not
// primed through this path.
func (p *DynamicChannelPool[C]) SetPrimeSession(id string) {
	if id != "" && !p.fixed {
		p.PrimeSession.Store(id)
		select {
		case p.ScaleUpSignal <- struct{}{}:
		default:
		}
	}
}

// PrimeSessionID returns the currently recorded multiplexed session ID used for
// channel priming, or "" if none has been set.
func (p *DynamicChannelPool[C]) PrimeSessionID() string {
	sid, _ := p.PrimeSession.Load().(string)
	return sid
}

// hasPrimeSession reports whether a scaled-up channel can be primed.
func (p *DynamicChannelPool[C]) hasPrimeSession() bool {
	return p.PrimeSessionID() != ""
}

// newEntry dials one DCP entry and optionally primes it.
func (p *DynamicChannelPool[C]) newEntry(ctx context.Context, prime bool) (*DCPEntry[C], error) {
	id := p.nextID.Add(1)
	entryPool, err := p.Dial(ctx)
	if err != nil {
		return nil, err
	}
	e := &DCPEntry[C]{ID: id, Pool: entryPool, Parent: p}
	now := time.Now().UnixNano()
	e.CreatedAt.Store(now)
	e.LastActivity.Store(now)
	if err := p.newEntryClient(ctx, e); err != nil {
		entryPool.Close()
		return nil, err
	}
	var zero C
	if e.Client == zero {
		_ = e.CloseNow()
		return nil, errors.New("spanner_dcp: NewEntryClient must set DCPEntry.Client")
	}
	if prime {
		if err := p.prime(ctx, e); err != nil {
			e.CloseNow()
			return nil, err
		}
	}
	return e, nil
}

// prime verifies a scaled-up channel before publishing it to the active slice.
// It executes the PrimeRPC hook so failed channels are never visible to normal
// request picking.
func (p *DynamicChannelPool[C]) prime(ctx context.Context, e *DCPEntry[C]) error {
	sid := p.PrimeSessionID()
	if sid == "" {
		return p.primeNotReadyError()
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
	if !p.fixed {
		e.LastActivity.Store(time.Now().UnixNano())
	}
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
	Entry        *DCPEntry[C]
	penaltyEpoch uint64
	once         sync.Once
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
		s.Entry.applyErrorPenalty(s.penaltyEpoch, err)
		s.Entry.StreamLoad.Add(-1)
		s.Entry.Parent.TotalRPCLoad.Add(-1)
		s.Entry.LastActivity.Store(time.Now().UnixNano())
	})
}

// PickPowerOfTwo selects the lower picker-load entry from two random active
// entries. It retries when either random choice is draining and falls back to a
// full least-loaded scan if random sampling cannot find an active pair.
func (p *DynamicChannelPool[C]) PickPowerOfTwo() (*DCPEntry[C], error) {
	entries := p.GetEntries()
	n := len(entries)
	if n == 0 {
		return nil, p.noEntriesError()
	}
	if n == 1 {
		if entries[0].isDraining() {
			return nil, p.noEntriesError()
		}
		return entries[0], nil
	}
	for i := 0; i < n*2; i++ {
		e1, e2 := entries[rand.IntN(n)], entries[rand.IntN(n)]
		if e1.isDraining() || e2.isDraining() {
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
		if !e.isDraining() {
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
		if e.isDraining() {
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
// average pool load exceeds DCPMaxRPCPerChannel. The signal channel is buffered
// so many hot requests collapse into one scale-up evaluation.
func (p *DynamicChannelPool[C]) MaybeSignalScaleUp(e *DCPEntry[C]) {
	active := p.Num()
	avg := float64(0)
	if active > 0 {
		// The two totals may be observed at slightly different instants. This
		// signal is only a hint; ScaleUp recomputes exact per-entry load.
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

// scaleUpWorker serializes event-driven scale-up requests.
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

// ScaleUp adds and primes channels based on current total load. The new entries
// are published only after successful dial and verification via the PrimeRPC hook.
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
	if p.Ctx.Err() != nil {
		p.DialMu.Unlock()
		return
	}
	if !p.hasPrimeSession() {
		p.DialMu.Unlock()
		return
	}
	entries := p.GetEntries()
	active := 0
	var load int64
	for _, e := range entries {
		if !e.isDraining() {
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
	// Floor the percent cap so small pools can ramp during burst recovery.
	// Floor raises the %-cap only; final add is still clamped to desired and
	// to DCPMaxChannels headroom below.
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
	// Claim the cooldown before slow channel creation/priming so any subsequent
	// scale-up signal that arrives while priming is in progress is throttled.
	p.LastScaleUp.Store(now.UnixNano())
	p.DialMu.Unlock()

	newEntries := make([]*DCPEntry[C], 0, add)
	for i := 0; i < add; i++ {
		if p.Ctx.Err() != nil {
			break
		}
		e, err := p.newEntry(p.Ctx, true)
		if err == nil {
			newEntries = append(newEntries, e)
		} else {
			p.logf("spanner_dcp: failed to create or prime scaled-up channel: %v", err)
		}
	}
	if len(newEntries) == 0 {
		return
	}

	p.DialMu.Lock()
	defer p.DialMu.Unlock()
	if p.Ctx.Err() != nil {
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

// scaleDownMonitor periodically evaluates whether sustained low load can drain
// channels.
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
	// Error penalties are deliberately excluded from scale-down load so a
	// failing channel is not retained as busy.
	for _, e := range entries {
		if !e.isDraining() {
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
		if !e.isDraining() {
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
	for {
		select {
		case <-t.C:
			if e.CloseIfIdle(max(p.Cfg.DCPDrainIdleGrace, p.DrainIdleFloor)) {
				p.DrainingCount.Add(-1)
				return
			}
		case <-p.Ctx.Done():
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

// borrowedConn is a ConnPool over one connection that it does not close.
type borrowedConn struct{ *grpc.ClientConn }

func (c borrowedConn) Conn() *grpc.ClientConn { return c.ClientConn }
func (c borrowedConn) Num() int               { return 1 }
func (c borrowedConn) Close() error           { return nil }

// borrowedPool is a ConnPool that does not close the pool it wraps.
type borrowedPool struct{ gtransport.ConnPool }

func (borrowedPool) Close() error { return nil }

// Acquire takes a reference that keeps drain from closing the entry until
// Release is called. It fails once the entry is closed.
func (e *DCPEntry[C]) Acquire() bool {
	if e.Refs.Add(1) > 0 {
		return true
	}
	e.Refs.Add(-1)
	return false
}

// Release releases a reference acquired via Acquire.
func (e *DCPEntry[C]) Release() { e.Refs.Add(-1) }

// CloseIfIdle closes a draining entry that has been idle for grace, unless an
// RPC holds a reference to it. It reports whether the entry is closed.
func (e *DCPEntry[C]) CloseIfIdle(grace time.Duration) bool {
	return e.idleFor(grace) && e.CloseIfUnreferenced()
}

// CloseIfUnreferenced closes the entry unless an RPC holds a reference to it.
// Swapping zero references for closed is atomic, so an RPC that acquired the
// entry keeps it open and one that acquires later fails and re-picks. It
// reports whether the entry is closed.
func (e *DCPEntry[C]) CloseIfUnreferenced() bool {
	if e.Refs.CompareAndSwap(0, dcpRefsClosed) {
		e.State.Store(DCPStateClosed)
		e.closeResources()
	}
	return e.Refs.Load() < 0
}

func (e *DCPEntry[C]) idleFor(grace time.Duration) bool {
	return e.RPCLoad() == 0 && time.Since(time.Unix(0, e.LastActivity.Load())) >= grace
}

// CloseNow closes the entry now, even if RPCs hold references to it. Pool
// shutdown uses it; scale-down uses CloseIfIdle.
func (e *DCPEntry[C]) CloseNow() error {
	if e.Refs.Swap(dcpRefsClosed) < 0 {
		return nil
	}
	e.State.Store(DCPStateClosed)
	return e.closeResources()
}

func (e *DCPEntry[C]) closeResources() error {
	e.ClearErrorPenalty()
	var errs []error
	var zero C
	if e.Client != zero {
		errs = append(errs, e.Client.Close())
	} else if e.Delegate != zero {
		errs = append(errs, e.Delegate.Close())
	}
	if e.Pool != nil {
		errs = append(errs, e.Pool.Close())
	}
	return errors.Join(errs...)
}

// isDraining atomically checks whether the entry has been removed from normal
// selection and is waiting for in-flight operations to finish.
func (e *DCPEntry[C]) isDraining() bool { return e.State.Load() == DCPStateDraining }

// RPCLoad returns the current in-flight RPC load for this entry.
func (e *DCPEntry[C]) RPCLoad() int32 { return e.UnaryLoad.Load() + e.StreamLoad.Load() }

// WeightedLoad returns the current in-flight RPC load for this entry.
func (e *DCPEntry[C]) WeightedLoad() int32 { return e.RPCLoad() }

// ApplyErrorPenalty accumulates load for errors that indicate channel-specific
// health or capacity trouble. Internal is deliberately excluded: only
// Unavailable and ResourceExhausted steer subsequent picks away from a channel.
// Updates are serialized because they are rare and must keep the pool aggregate
// consistent. The hot pick path remains lock-free while the penalty is active.
func (e *DCPEntry[C]) ApplyErrorPenalty(err error) {
	e.applyErrorPenalty(e.Parent.currentPenaltyEpoch(), err)
}

func (e *DCPEntry[C]) applyErrorPenalty(epoch uint64, err error) {
	if err == nil || e.Parent.Cfg.DCPErrorPenaltyDuration < 0 {
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
	// Check the epoch under penaltyMu so a late error cannot apply after
	// ClearStaleErrorPenalties.
	if e.Parent.currentPenaltyEpoch() != epoch {
		return
	}
	now := time.Now()
	expiry := e.PenaltyExpiry.Load()
	current := int32(0)
	oldContribution := int32(0)
	if expiry != 0 {
		oldContribution = e.PenaltyLoad.Load()
		if now.UnixNano() < expiry && e.penaltyEpoch == epoch {
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
	e.penaltyEpoch = epoch
	e.PenaltyLoad.Store(load)
	e.PenaltyExpiry.Store(now.Add(e.Parent.Cfg.DCPErrorPenaltyDuration).UnixNano())
	e.Parent.TotalPenaltyLoad.Add(int64(load - oldContribution))
}

// CurrentPenalty returns active accumulated error load and lazily clears an
// expired penalty.
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
	// Leave PenaltyLoad intact; expiry alone gates whether load is used.
	e.PenaltyExpiry.Store(0)
	e.Parent.TotalPenaltyLoad.Add(-int64(e.PenaltyLoad.Load()))
	return 0
}

// ClearErrorPenalty removes an entry's aggregate contribution when the entry
// leaves the live pool. The expiry swap makes repeated close paths harmless.
func (e *DCPEntry[C]) ClearErrorPenalty() {
	e.penaltyMu.Lock()
	defer e.penaltyMu.Unlock()
	if e.PenaltyExpiry.Swap(0) != 0 {
		e.Parent.TotalPenaltyLoad.Add(-int64(e.PenaltyLoad.Load()))
	}
}

// ClearStaleErrorPenalties clears the penalties that active entries earned
// before the current PenaltyEpoch.
func (p *DynamicChannelPool[C]) ClearStaleErrorPenalties() {
	epoch := p.currentPenaltyEpoch()
	for _, e := range p.GetEntries() {
		e.penaltyMu.Lock()
		if e.penaltyEpoch != epoch && e.PenaltyExpiry.Swap(0) != 0 {
			p.TotalPenaltyLoad.Add(-int64(e.PenaltyLoad.Load()))
		}
		e.penaltyMu.Unlock()
	}
}

func (p *DynamicChannelPool[C]) callStarted(ctx context.Context, e *DCPEntry[C]) {
	if p.onCallStart != nil {
		p.onCallStart(ctx, e.RPCLoad())
	}
}

// PickLoad returns the in-flight RPC load plus any active error penalty.
func (e *DCPEntry[C]) PickLoad() int32 { return e.RPCLoad() + e.CurrentPenalty() }

// StartUnary increments unary load and returns a completion callback.
func (e *DCPEntry[C]) StartUnary(ctx context.Context) func(error) {
	epoch := e.Parent.currentPenaltyEpoch()
	e.UnaryLoad.Add(1)
	e.Parent.TotalRPCLoad.Add(1)
	e.Parent.callStarted(ctx, e)
	e.Parent.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	return func(err error) {
		e.applyErrorPenalty(epoch, err)
		e.UnaryLoad.Add(-1)
		e.Parent.TotalRPCLoad.Add(-1)
		e.LastActivity.Store(time.Now().UnixNano())
	}
}

// StartStream increments stream load and entry reference count, returning a DCPStreamRef.
func (e *DCPEntry[C]) StartStream(ctx context.Context) *DCPStreamRef {
	epoch := e.Parent.currentPenaltyEpoch()
	// A stream outlives the call that starts it, so it holds its own reference.
	e.Refs.Add(1)
	e.StreamLoad.Add(1)
	e.Parent.TotalRPCLoad.Add(1)
	e.Parent.callStarted(ctx, e)
	e.Parent.MaybeSignalScaleUp(e)
	e.LastActivity.Store(time.Now().UnixNano())
	ref := &DCPStreamRef{finish: func(err error) {
		e.applyErrorPenalty(epoch, err)
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
		ref.setStop(context.AfterFunc(ctx, func() {
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

func (r *DCPStreamRef) setStop(stop func() bool) {
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

// RecvMsg is Recv for callers that receive the PartialResultSet into a message
// of another type. See spanner/vtproto_stream.go.
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
	// Successful CloseSend only half-closes the client send side. The stream is
	// still active until Recv returns a terminal error/EOF or the context is
	// canceled, so do not release stream load here.
	return err
}

// DCPResolver keeps using its bound DCPEntry while it is active or draining,
// so all operations of a read-write transaction stay on one channel through a
// scale-down. It re-picks only once the bound entry is closed.
//
// Pool and Entry are initialized by spanner/sharedcore resolving wrappers and
// in-module white-box tests; Pool must be non-nil when Acquire is called, and
// must not be mutated after construction.
type DCPResolver[C DCPClient] struct {
	Pool  *DynamicChannelPool[C]
	Entry atomic.Pointer[DCPEntry[C]]
}

// Acquire returns the bound entry with a reference held, which the caller
// releases when its RPC returns. Scale-down drain cannot close the entry in
// between, so the RPC never starts on a channel drain closed; only pool
// shutdown closes a referenced entry. A closed entry is replaced by a fresh
// pick. Both c and c.Pool must be non-nil (embedding wrappers guard nil outer
// pointers and nil pools with their own sentinel error).
func (c *DCPResolver[C]) Acquire(ctx context.Context) (*DCPEntry[C], error) {
	e := c.Entry.Load()
	for e == nil || !e.Acquire() {
		picked, err := c.Pool.Pick(ctx)
		if err != nil {
			return nil, err
		}
		// Concurrent RPCs of one transaction must agree on the new entry.
		if !c.Entry.CompareAndSwap(e, picked) {
			picked = c.Entry.Load()
		}
		e = picked
	}
	return e, nil
}
