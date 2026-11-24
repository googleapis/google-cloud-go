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
	"math/rand/v2"
)

// defaultAfeRandomSubsetSize is the default K for K-choice random draws
// in LeastInFlightAfePicker / LeastLatencyAfePicker when the caller
// doesn't specify one. Two candidates ("power of two choices") is the
// standard K-choice draw size.
const defaultAfeRandomSubsetSize = 2

// PickCandidate is one AFE the picker considered during a K-choice draw,
// with the cost value the picker's decision rule used to score it.
// Cost's interpretation depends on the picker in play: NumOutstanding
// (int as float) for LeastInFlight, e2e PeakEwma nanos for LeastLatency,
// 0 for Simple (which ignores cost).
type PickCandidate struct {
	AfeID AfeID
	Cost  float64
}

// PickDecision captures what candidates a picker sampled, which one won,
// and why. Populated on every pickLocked call so operators can trace
// picker reasoning through the debug surface without re-running the pick.
type PickDecision struct {
	// Candidates is the K sampled AFEs (K == 1 for SimplePicker,
	// otherwise K == min(RandomSubsetSize, len(ready)) via partial
	// Fisher-Yates). Empty when no ready AFE existed or recordCandidates
	// is false.
	Candidates []PickCandidate
	// Winner is the AFE the picker returned. Zero when ready was empty.
	Winner AfeID
	// Reason is a short lower-kebab tag identifying the decision rule
	// ("uniform-random" / "min-inflight" / "min-latency" /
	// "no-candidates"). Machine-readable; consumers map to prose.
	Reason string
}

// AfePicker selects one AFE bucket from sessionList.afesWithReady while
// sessionList.mu is held, returning the chosen bucket and decision trace.
// Running selection and dequeue under the same sessionList.mu critical
// section eliminates lock-split checkout races and avoids per-RPC slice
// allocations or O(N) EWMA reads across unsampled AFEs.
//
// Implementations MAY reorder ready in place ONLY via swapReadyAfesLocked
// so each afeHandle.readyIdx stays consistent with its slice position (I3).
type AfePicker interface {
	pickLocked(ready []*afeHandle) (winner *afeHandle, decision PickDecision)
	Name() string
}

// swapReadyAfesLocked swaps ready[i] and ready[j] while keeping each
// bucket's readyIdx invariant intact (I3). Caller must hold sessionList.mu.
func swapReadyAfesLocked(ready []*afeHandle, i, j int) {
	if i == j {
		return
	}
	ready[i], ready[j] = ready[j], ready[i]
	ready[i].readyIdx = i
	ready[j].readyIdx = j
}

// SimpleAfePicker chooses an AFE uniformly at random from the ready set.
type SimpleAfePicker struct {
	// recordCandidates gates PickDecision.Candidates construction.
	// Owned by SessionPoolImpl at construction; matches
	// SessionPoolImpl.debugEnabled. When false, PickDecision returns
	// Winner+Reason only — Candidates stays nil so the per-pick slice
	// allocation is skipped.
	recordCandidates bool
}

// NewSimpleAfePicker constructs a SimpleAfePicker.
func NewSimpleAfePicker(recordCandidates bool) *SimpleAfePicker {
	return &SimpleAfePicker{recordCandidates: recordCandidates}
}

// Name returns "simple".
func (SimpleAfePicker) Name() string { return "simple" }

func (p SimpleAfePicker) pickLocked(ready []*afeHandle) (*afeHandle, PickDecision) {
	if len(ready) == 0 {
		return nil, PickDecision{Reason: "no-candidates"}
	}
	winner := ready[rand.IntN(len(ready))]
	d := PickDecision{Winner: winner.id, Reason: "uniform-random"}
	if p.recordCandidates {
		d.Candidates = []PickCandidate{{AfeID: winner.id, Cost: 0}}
	}
	return winner, d
}

// LeastInFlightAfePicker picks the AFE with the smallest active in-flight
// count (excluding Closing handles), breaking ties toward higher idle
// session count. Draws K distinct candidates via partial Fisher-Yates over
// ready. RandomSubsetSize caps K; <=0 or >= len(ready) scans all.
type LeastInFlightAfePicker struct {
	RandomSubsetSize int
	recordCandidates bool
}

// NewLeastInFlightAfePicker constructs a LeastInFlightAfePicker.
func NewLeastInFlightAfePicker(randomSubsetSize int, recordCandidates bool) *LeastInFlightAfePicker {
	return &LeastInFlightAfePicker{RandomSubsetSize: randomSubsetSize, recordCandidates: recordCandidates}
}

// Name returns "least-inflight".
func (LeastInFlightAfePicker) Name() string { return "least-inflight" }

func (p LeastInFlightAfePicker) pickLocked(ready []*afeHandle) (*afeHandle, PickDecision) {
	winner, cands := kChoiceMinCostLocked(
		ready,
		p.RandomSubsetSize,
		p.recordCandidates,
		func(a *afeHandle) float64 { return float64(a.inFlightLocked()) },
		func(cand, best *afeHandle) bool { return len(cand.sessions) > len(best.sessions) },
	)
	return decisionFor(winner, cands, "min-inflight")
}

// LeastLatencyAfePicker picks the AFE with the lowest per-AFE e2e PeakEwma
// cost among K randomly-drawn ready candidates. Evaluates e2eEwma.Value()
// only on the K sampled candidates rather than the full ready set.
type LeastLatencyAfePicker struct {
	RandomSubsetSize int
	recordCandidates bool
}

// NewLeastLatencyAfePicker constructs a LeastLatencyAfePicker.
func NewLeastLatencyAfePicker(randomSubsetSize int, recordCandidates bool) *LeastLatencyAfePicker {
	return &LeastLatencyAfePicker{RandomSubsetSize: randomSubsetSize, recordCandidates: recordCandidates}
}

// Name returns "least-latency".
func (LeastLatencyAfePicker) Name() string { return "least-latency" }

func (p LeastLatencyAfePicker) pickLocked(ready []*afeHandle) (*afeHandle, PickDecision) {
	winner, cands := kChoiceMinCostLocked(
		ready,
		p.RandomSubsetSize,
		p.recordCandidates,
		func(a *afeHandle) float64 { return a.e2eEwma.Value() },
		nil,
	)
	return decisionFor(winner, cands, "min-latency")
}

func decisionFor(winner *afeHandle, cands []PickCandidate, reason string) (*afeHandle, PickDecision) {
	if winner == nil {
		return nil, PickDecision{Reason: "no-candidates"}
	}
	return winner, PickDecision{
		Candidates: cands,
		Winner:     winner.id,
		Reason:     reason,
	}
}

// kChoiceMinCostLocked implements in-place partial Fisher-Yates + min-cost
// selection over sessionList.afesWithReady under sessionList.mu, updating
// readyIdx via swapReadyAfesLocked so O(1) removal stays valid.
func kChoiceMinCostLocked(
	ready []*afeHandle,
	k int,
	recordCandidates bool,
	cost func(*afeHandle) float64,
	tieBreak func(cand, best *afeHandle) bool,
) (*afeHandle, []PickCandidate) {
	n := len(ready)
	if n == 0 {
		return nil, nil
	}
	if k <= 0 || k > n {
		k = n
	}

	var sampled []PickCandidate
	if recordCandidates {
		sampled = make([]PickCandidate, 0, k)
	}
	var best *afeHandle
	bestCost := -1.0
	for i := 0; i < k; i++ {
		j := i + rand.IntN(n-i)
		a := ready[j]
		c := cost(a)
		if recordCandidates {
			sampled = append(sampled, PickCandidate{AfeID: a.id, Cost: c})
		}
		if best == nil || c < bestCost || (c == bestCost && tieBreak != nil && tieBreak(a, best)) {
			bestCost = c
			best = a
		}
		swapReadyAfesLocked(ready, i, j)
	}
	return best, sampled
}
