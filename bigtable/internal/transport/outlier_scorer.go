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
	"time"
)

// OutlierScorer produces a per-AFE cost multiplier that latency-based
// AfePickers fold into their decision. Score returns:
//
//   - 1.0 for healthy AFEs (no penalty; picker cost is unchanged)
//   - a value > 1.0 for outliers (picker cost inflates by this factor)
//   - 1.0 for unknown IDs (any AFE the scorer has not observed)
//
// Score is called on the CheckoutSession hot path — once per ready AFE
// per pick — so implementations must be fast; sub-microsecond is the
// target. Implementations must be safe for concurrent Score calls from
// multiple goroutines without external synchronization.
//
// Name returns a short, stable identifier for debug tooling and metrics
// attribution ("noop", "latency-outlier", …). Must be safe to call
// concurrently.
//
// This is the plug-point for outlier detection: pass an implementation
// to SessionPoolImpl.SetOutlierScorer. Default is NoopScorer, which
// returns 1.0 for every AFE and makes the picker behave exactly as it
// did before this framework existed.
type OutlierScorer interface {
	Score(id AfeID) float64
	Name() string
}

// LifecycleScorer is optionally implemented by scorers that maintain
// background state (a periodic detection loop, for example) and need a
// context to bound their lifetime. When present, SessionPoolImpl.Start
// invokes Start(poolCtx) exactly once during pool startup; cancellation
// of the context signals the scorer to wind down its background work.
//
// Scorers that are stateless (NoopScorer, or a caller-supplied
// hard-coded score map) don't need to implement this interface.
type LifecycleScorer interface {
	OutlierScorer
	Start(ctx context.Context)
}

// SnapshottingScorer is optionally implemented by scorers that produce
// audit data (score transitions, decision reasons) for the debug pages
// and post-mortem tooling. OutlierSnapshot returns a self-contained
// copy of the audit state — no locks are required on the returned
// slice, and implementations must not mutate it after returning.
type SnapshottingScorer interface {
	OutlierScorer
	OutlierSnapshot() []OutlierDecision
}

// OutlierDecision is one entry in a scorer's audit log — a per-target
// record of a score transition with the signal that drove it. Emitted
// by SnapshottingScorer.OutlierSnapshot() and rendered by future
// /debug/outlierz handlers.
type OutlierDecision struct {
	// When is the wall-clock time the decision was made.
	When time.Time
	// AfeID is the target the decision applies to.
	AfeID AfeID
	// OldScore and NewScore bracket the transition. Only transitions
	// are recorded — no-change ticks do not produce OutlierDecisions.
	OldScore float64
	NewScore float64
	// Signal is the observed metric (latency in ns for a latency-based
	// scorer) the scorer compared against the cohort.
	Signal float64
	// CohortMedian is the reference value the signal was compared to.
	CohortMedian float64
	// Reason names the decision rule that fired, e.g. "penalized-latency"
	// or "recovered-latency". Lower-kebab; stable enough for machine
	// consumption in debug tooling.
	Reason string
}

// NoopScorer returns 1.0 for every AFE — the picker's cost function is
// unmodified. Zero-cost sentinel used when the pool is constructed
// without an outlier scorer or when the caller wants to explicitly opt
// out of outlier downweight.
type NoopScorer struct{}

// Score always returns 1.0.
func (NoopScorer) Score(AfeID) float64 { return 1.0 }

// Name returns "noop".
func (NoopScorer) Name() string { return "noop" }
