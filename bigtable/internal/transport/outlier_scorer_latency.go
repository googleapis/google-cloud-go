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

package internal

import (
	"context"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

// LatencyOutlierConfig tunes LatencyOutlierScorer. Zero values in any
// field are replaced by the same defaults DefaultLatencyOutlierConfig
// produces, so callers can override just the knobs they care about.
type LatencyOutlierConfig struct {
	// Interval is the detector tick cadence. Default 30s.
	Interval time.Duration
	// LatencyMultiplier is the outlier threshold. An AFE is flagged when
	// its worst PeakEwma (max of e2e and transport) exceeds
	// LatencyMultiplier × cohort_median AND MinLatencyFloor. Default 3.0.
	LatencyMultiplier float64
	// MinLatencyFloor is an absolute floor below which no AFE is ever
	// penalized — prevents "3× median" from firing when median is
	// microseconds. Default 20ms.
	MinLatencyFloor time.Duration
	// MinCohortSize is the smallest cohort the detector will evaluate.
	// Pools with fewer AFEs than this get no protection (median is not
	// meaningful below 3 peers). Default 3.
	MinCohortSize int
	// PenaltyMultiplier is the cost-function multiplier applied to
	// flagged AFEs. A latency-based picker will pick a penalized AFE
	// with cost = base × PenaltyMultiplier; K-choice-of-2 almost never
	// picks it vs a healthy sibling at 10x. Default 10.0.
	PenaltyMultiplier float64
	// AuditRingSize caps the OutlierSnapshot audit ring. Default 500.
	AuditRingSize int
}

// DefaultLatencyOutlierConfig returns the calibrated defaults for
// production use.
func DefaultLatencyOutlierConfig() LatencyOutlierConfig {
	return LatencyOutlierConfig{
		Interval:          30 * time.Second,
		LatencyMultiplier: 3.0,
		MinLatencyFloor:   20 * time.Millisecond,
		MinCohortSize:     3,
		PenaltyMultiplier: 10.0,
		AuditRingSize:     500,
	}
}

// applyDefaults fills any zero-valued field with the value from
// DefaultLatencyOutlierConfig. Called from NewLatencyOutlierScorer so
// callers can pass a partial config without losing the calibrated
// defaults on the fields they didn't set.
func (c LatencyOutlierConfig) applyDefaults() LatencyOutlierConfig {
	def := DefaultLatencyOutlierConfig()
	if c.Interval == 0 {
		c.Interval = def.Interval
	}
	if c.LatencyMultiplier == 0 {
		c.LatencyMultiplier = def.LatencyMultiplier
	}
	if c.MinLatencyFloor == 0 {
		c.MinLatencyFloor = def.MinLatencyFloor
	}
	if c.MinCohortSize == 0 {
		c.MinCohortSize = def.MinCohortSize
	}
	if c.PenaltyMultiplier == 0 {
		c.PenaltyMultiplier = def.PenaltyMultiplier
	}
	if c.AuditRingSize == 0 {
		c.AuditRingSize = def.AuditRingSize
	}
	return c
}

// afeSnapshotSource is the minimal read-only surface LatencyOutlierScorer
// needs from a sessionList. *sessionList satisfies it in production;
// tests pass a fake that returns hand-authored snapshots without
// standing up a real sessionList.
type afeSnapshotSource interface {
	Snapshot() []AfeSnapshotRow
}

// LatencyOutlierScorer is the built-in OutlierScorer that periodically
// reads per-AFE PeakEwma snapshots from a sessionList, compares each
// AFE's worst latency against the cohort median, and inflates the score
// of outliers so latency-based pickers steer traffic away.
//
// Implements OutlierScorer, LifecycleScorer, and SnapshottingScorer.
type LatencyOutlierScorer struct {
	src afeSnapshotSource
	cfg LatencyOutlierConfig

	// scores is the hot-path read surface: atomic.Pointer to an
	// immutable map from AfeID to cost multiplier. Score(id) does one
	// atomic load + one map lookup. tick() builds a fresh map each
	// iteration and atomic-swaps it in — no lock held on the read
	// path.
	scores atomic.Pointer[map[AfeID]float64]

	// auditMu guards audit + auditNext. Contention is negligible —
	// writes only during tick transitions (rare), reads only from
	// OutlierSnapshot (debug tooling).
	auditMu   sync.Mutex
	audit     []OutlierDecision
	auditNext int // next write index when the ring is full
}

// NewLatencyOutlierScorer constructs a LatencyOutlierScorer that reads
// AFE snapshots from sl. Pass DefaultLatencyOutlierConfig() (or a
// partially-populated LatencyOutlierConfig) as cfg — zero-valued fields
// pick up defaults. The returned scorer's Score always returns 1.0
// until Start is called and the first tick lands.
func NewLatencyOutlierScorer(sl *sessionList, cfg LatencyOutlierConfig) *LatencyOutlierScorer {
	return newLatencyOutlierScorer(sl, cfg)
}

// newLatencyOutlierScorer is the unexported constructor accepting the
// afeSnapshotSource interface — tests inject a fake source; production
// callers use NewLatencyOutlierScorer which passes a *sessionList.
func newLatencyOutlierScorer(src afeSnapshotSource, cfg LatencyOutlierConfig) *LatencyOutlierScorer {
	s := &LatencyOutlierScorer{
		src: src,
		cfg: cfg.applyDefaults(),
	}
	empty := make(map[AfeID]float64)
	s.scores.Store(&empty)
	return s
}

// Score returns the current cost multiplier for id. Returns 1.0 for
// AFEs the scorer hasn't observed yet — safe default so
// LeastLatencyAfePicker's cost math never sees a zero multiplier.
func (s *LatencyOutlierScorer) Score(id AfeID) float64 {
	m := s.scores.Load()
	if m == nil {
		return 1.0
	}
	if v, ok := (*m)[id]; ok {
		return v
	}
	return 1.0
}

// Start spawns the periodic detector loop. Cancels on ctx.Done — the
// pool's poolCtx is what SessionPoolImpl.Start passes here.
// Idempotent-safe: SessionPoolImpl.Start already funnels through
// startOnce so Start is called at most once per scorer instance.
func (s *LatencyOutlierScorer) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick()
			}
		}
	}()
}

// OutlierSnapshot returns a copy of the audit ring in insertion order
// (oldest to newest). Safe to render without further locking; the
// returned slice is a fresh allocation.
func (s *LatencyOutlierScorer) OutlierSnapshot() []OutlierDecision {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	n := len(s.audit)
	if n == 0 {
		return nil
	}
	out := make([]OutlierDecision, n)
	if n < s.cfg.AuditRingSize {
		copy(out, s.audit)
		return out
	}
	// Ring is full — walk from auditNext (oldest) to auditNext-1 (newest).
	for i := 0; i < n; i++ {
		out[i] = s.audit[(s.auditNext+i)%n]
	}
	return out
}

// tick runs one detection cycle. Reads a fresh AFE snapshot from the
// sessionList (single lock acquisition, released before we compute
// anything), evaluates the outlier rule, and atomically swaps in the
// new score map. Emits transition tags for AFEs whose score changed
// from the previous tick.
func (s *LatencyOutlierScorer) tick() {
	defer func() {
		if r := recover(); r != nil {
			btopt.Debugf(nil, "outlier scorer tick panic: %v\n%s", r, debug.Stack())
		}
	}()
	rows := s.src.Snapshot()
	if len(rows) < s.cfg.MinCohortSize {
		// Not enough peers for a stable median. Do NOT clear existing
		// scores — a shrinking cohort might briefly drop below the
		// threshold; keeping the prior tick's decision is less
		// surprising than snapping back to 1.0 on every AFE.
		return
	}

	// Compute the outlier signal for each AFE: worst of e2eEwma and
	// transportEwma. Backend-slow and network-slow are both bad; taking
	// the max flags either.
	worsts := make([]time.Duration, len(rows))
	sorted := make([]time.Duration, len(rows))
	for i, r := range rows {
		w := r.E2eEwma
		if r.TransportEwma > w {
			w = r.TransportEwma
		}
		worsts[i] = w
		sorted[i] = w
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := sorted[len(sorted)/2]

	old := *s.scores.Load()
	newScores := make(map[AfeID]float64, len(rows))
	now := time.Now()
	for i, r := range rows {
		newScore := 1.0
		if worsts[i] > s.cfg.MinLatencyFloor &&
			float64(worsts[i]) > s.cfg.LatencyMultiplier*float64(median) {
			newScore = s.cfg.PenaltyMultiplier
		}
		newScores[r.ID] = newScore

		oldScore := old[r.ID]
		if oldScore == 0 {
			oldScore = 1.0
		}
		if oldScore == newScore {
			continue
		}
		reason := "penalized-latency"
		tag := tagOutlierAfePenalizedLatency
		if newScore == 1.0 {
			reason = "recovered-latency"
			tag = tagOutlierAfeRecoveredLatency
		}
		recordDebugTag(tag)
		s.pushAudit(OutlierDecision{
			When:         now,
			AfeID:        r.ID,
			OldScore:     oldScore,
			NewScore:     newScore,
			Signal:       float64(worsts[i]),
			CohortMedian: float64(median),
			Reason:       reason,
		})
	}
	s.scores.Store(&newScores)
}

// pushAudit appends d to the audit ring, overwriting the oldest entry
// when the ring is full.
func (s *LatencyOutlierScorer) pushAudit(d OutlierDecision) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	if len(s.audit) < s.cfg.AuditRingSize {
		s.audit = append(s.audit, d)
		return
	}
	s.audit[s.auditNext] = d
	s.auditNext = (s.auditNext + 1) % s.cfg.AuditRingSize
}
