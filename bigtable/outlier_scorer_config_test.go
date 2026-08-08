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

package bigtable

import "testing"

// TestOutlierScorer_PublicAPI is a compile-time check: the public
// ClientConfig field, factory helper, and type aliases must all resolve
// to the transport-package types and be usable together in the shape
// the docstring example promises. If any of the type aliases or the
// LatencyOutlierFactory signature drift, this test stops compiling.
func TestOutlierScorer_PublicAPI(t *testing.T) {
	// Default config is usable directly.
	def := DefaultLatencyOutlierConfig()
	if def.LatencyMultiplier == 0 {
		t.Errorf("DefaultLatencyOutlierConfig().LatencyMultiplier = 0, want non-zero")
	}
	if def.PenaltyMultiplier == 0 {
		t.Errorf("DefaultLatencyOutlierConfig().PenaltyMultiplier = 0, want non-zero")
	}

	// LatencyOutlierFactory returns a factory that satisfies the
	// public OutlierScorerFactory type — assign into ClientConfig
	// exactly as the docstring example shows.
	cfg := ClientConfig{
		EnableDebug:          true,
		OutlierScorerFactory: LatencyOutlierFactory(def),
	}
	if cfg.OutlierScorerFactory == nil {
		t.Fatal("LatencyOutlierFactory returned nil factory")
	}

	// Invoking the factory with a NoopScorer-shaped source is fine —
	// the returned scorer must be a *LatencyOutlierScorer implementing
	// OutlierScorer.
	scorer := cfg.OutlierScorerFactory(fakeSource{})
	if scorer == nil {
		t.Fatal("factory returned nil scorer")
	}
	if got := scorer.Name(); got != "latency-outlier" {
		t.Errorf("scorer.Name() = %q, want %q", got, "latency-outlier")
	}
	// Unknown AFE (empty source) → 1.0 (default).
	if got := scorer.Score(42); got != 1.0 {
		t.Errorf("scorer.Score(42) on empty source = %v, want 1.0", got)
	}
}

// fakeSource satisfies AfeSnapshotSource with zero rows — a placeholder
// for the compile-check test that only needs the factory to invoke.
type fakeSource struct{}

func (fakeSource) Snapshot() []AfeSnapshotRow { return nil }
