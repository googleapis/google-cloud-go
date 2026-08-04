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
	"time"
)

// OutlierDebugProvider exposes per-pool outlier-scorer state to the
// debugview package. Symmetric with SessionDebugProvider /
// ChannelDebugProvider / ConfigDebugProvider — the client implements
// this to feed the /debug/outlierz page.
type OutlierDebugProvider interface {
	// Snapshot returns one entry per session pool the client owns. For
	// pools running NoopScorer the entry carries ScorerName="noop"
	// with empty Params / Scores / Recent, so the debug page still
	// renders a row per pool showing "no outlier detection wired."
	Snapshot() []OutlierPoolSnapshot
}

// OutlierPoolSnapshot is the per-pool outlier-detection state a debug
// page renders — what scorer is plugged in, its config knobs, the
// current per-AFE score map, and recent score transitions.
type OutlierPoolSnapshot struct {
	// PoolName matches the SessionPoolImpl's pool name so debug pages
	// can cross-link back to sessionz / loadz etc.
	PoolName string `json:"pool"`
	// ScorerName is OutlierScorer.Name() — e.g. "noop", "latency-outlier".
	ScorerName string `json:"scorer"`
	// Params is the scorer's configuration serialized as name/value
	// pairs for direct table rendering. Empty for scorers that carry
	// no configuration (NoopScorer).
	Params []OutlierParam `json:"params"`
	// Scores is the current per-AFE score map, sorted by AfeID. Empty
	// for scorers that don't expose a score map (NoopScorer, or any
	// scorer that hasn't run its first tick yet).
	Scores []OutlierScoreRow `json:"scores"`
	// Recent is the audit ring of the scorer's most recent score
	// transitions, oldest-first. Empty for scorers without an audit
	// ring. Rendered newest-first by debug templates.
	Recent []OutlierDecision `json:"recent"`
	// CapturedAt is the wall-clock time this snapshot was assembled.
	CapturedAt time.Time `json:"capturedAt"`
}

// OutlierParam is one config knob in an OutlierPoolSnapshot's Params
// list — name/value strings the debug template renders directly.
type OutlierParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// OutlierScoreRow is one row in an OutlierPoolSnapshot's Scores list —
// the current cost multiplier the scorer assigns to a given AFE.
type OutlierScoreRow struct {
	AfeID AfeID `json:"afeId"`
	// Score is the multiplier the picker's cost function will apply.
	// 1.0 = no penalty. Values > 1.0 downweight; the scorer decides
	// the exact magnitude.
	Score float64 `json:"score"`
	// Penalized is true when Score > 1.0 — a convenience flag so debug
	// templates don't have to re-derive the boolean from the float.
	Penalized bool `json:"penalized"`
}
