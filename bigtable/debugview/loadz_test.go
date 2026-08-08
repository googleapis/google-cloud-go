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

package debugview

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func loadzSampleLB() []btransport.LoadBalancingSnapshot {
	now := time.Now()
	return []btransport.LoadBalancingSnapshot{
		{
			PoolName:   "my-table:read",
			PickerName: "least-inflight",
			CapturedAt: now,
			AFEs: []btransport.AfeSnapshotRow{
				{
					ID: 1, RefCount: 3, IdleCount: 2,
					TransportEwma: 500 * time.Microsecond, E2eEwma: 4 * time.Millisecond,
					LastConnected: now.Add(-30 * time.Second),
				},
				{
					ID: 2, RefCount: 3, IdleCount: 2,
					TransportEwma: 600 * time.Microsecond, E2eEwma: 8 * time.Millisecond,
					LastConnected: now.Add(-10 * time.Second),
				},
			},
			PickCounts: map[int64]int64{1: 80, 2: 20}, // 80/20 split
			Recent: []btransport.PickHistoryEvent{
				{
					At:         now.Add(-2 * time.Second),
					PickerName: "least-inflight",
					Decision: btransport.PickDecision{
						Winner: 1,
						Reason: "min-inflight",
						Candidates: []btransport.PickCandidate{
							{AfeID: 1, Cost: 0},
							{AfeID: 2, Cost: 1},
						},
					},
				},
				{
					At:         now.Add(-1 * time.Second),
					PickerName: "least-inflight",
					Decision: btransport.PickDecision{
						Winner: 2,
						Reason: "min-inflight",
						Candidates: []btransport.PickCandidate{
							{AfeID: 2, Cost: 0},
						},
					},
				},
			},
		},
	}
}

func TestLoadz_Index_HTML_RendersPickerAndAFEs(t *testing.T) {
	h := newLoadzHandler(fakeSessionProvider{lb: loadzSampleLB()})
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"my-table:read",
		"least-inflight",
		"K-choice-2", // gloss
		"0x1",        // AFE id hex
		"0x2",
		"min-inflight", // reason in recent picks table
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestLoadz_Index_JSON_RoundTrips(t *testing.T) {
	h := newLoadzHandler(fakeSessionProvider{lb: loadzSampleLB()})
	rec := get(t, h, "/?format=json")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Pools []loadzPoolView `json:"pools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Pools) != 1 {
		t.Fatalf("pools len = %d, want 1", len(got.Pools))
	}
	p := got.Pools[0]
	if p.TotalPicks != 100 {
		t.Errorf("TotalPicks = %d, want 100 (80+20)", p.TotalPicks)
	}
	if len(p.AFEs) != 2 {
		t.Fatalf("AFEs len = %d, want 2", len(p.AFEs))
	}
	// AFE 1 has 80 picks of 100 → 80%; ideal is 50% (2 AFEs) → skew +30pp.
	if p.AFEs[0].ActualSharePct != 80 {
		t.Errorf("AFE 1 actual share = %.1f, want 80", p.AFEs[0].ActualSharePct)
	}
	if p.AFEs[0].IdealSharePct != 50 {
		t.Errorf("AFE 1 ideal share = %.1f, want 50", p.AFEs[0].IdealSharePct)
	}
	if p.AFEs[0].SkewPP != 30 {
		t.Errorf("AFE 1 skew = %.1f, want +30", p.AFEs[0].SkewPP)
	}
	// Recent picks are newest-first — assert that the last-in-buffer
	// (Winner=2) appears first.
	if len(p.RecentPicks) != 2 {
		t.Fatalf("RecentPicks len = %d, want 2", len(p.RecentPicks))
	}
	if p.RecentPicks[0].Winner != 2 {
		t.Errorf("RecentPicks[0].Winner = %d, want 2 (newest-first)", p.RecentPicks[0].Winner)
	}
}

func TestLoadz_Index_NilProvider(t *testing.T) {
	h := newLoadzHandler(nil)
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No session pools recorded yet") {
		t.Errorf("expected empty-state message, got: %s", rec.Body.String())
	}
}

func TestLoadz_GlossFor(t *testing.T) {
	for name, want := range map[string]string{
		"simple":         "Uniform random",
		"least-inflight": "in-flight",
		"least-latency":  "PeakEwma",
	} {
		if got := glossFor(name); !strings.Contains(got, want) {
			t.Errorf("glossFor(%q) = %q, want to contain %q", name, got, want)
		}
	}
	if got := glossFor("unknown-picker"); got != "" {
		t.Errorf("glossFor(unknown) = %q, want empty", got)
	}
}
