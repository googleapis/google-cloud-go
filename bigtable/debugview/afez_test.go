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

func afezSampleSnapshot() []btransport.PoolSnapshot {
	now := time.Now()
	// Provider returns rows in whatever order it chooses. Production
	// sessionList.Snapshot sorts by ID; this fake mirrors that (0
	// first, 0xa1b2c3 next) so the test asserts against a
	// deterministic order.
	return []btransport.PoolSnapshot{
		{
			Name: "my-table:read",
			AFEs: []btransport.AfeSnapshotRow{
				{
					ID:            0,
					RefCount:      0, // pending-GC
					IdleCount:     0,
					LastConnected: now.Add(-11 * time.Minute),
				},
				{
					ID:            0xa1b2c3,
					RefCount:      3,
					IdleCount:     2,
					TransportEwma: 500 * time.Microsecond,
					E2eEwma:       4 * time.Millisecond,
					LastConnected: now.Add(-30 * time.Second),
				},
			},
		},
	}
}

func TestAfez_Index_HTML_RendersAFEs(t *testing.T) {
	h := newAfezHandler(fakeSessionProvider{pools: afezSampleSnapshot()})
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"my-table:read", "0xa1b2c3", "Bigtable AFE view", "pending-gc"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
	if !strings.Contains(body, "unknown") {
		t.Errorf("expected AFE id=0 rendered as 'unknown', body: %s", body)
	}
}

func TestAfez_Index_JSON_RoundTrips(t *testing.T) {
	h := newAfezHandler(fakeSessionProvider{pools: afezSampleSnapshot()})
	rec := get(t, h, "/?format=json")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		AFEs  []afezRow `json:"afes"`
		Total int       `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Total != 2 {
		t.Errorf("total = %d, want 2", got.Total)
	}
	if len(got.AFEs) != 2 {
		t.Fatalf("len(afes) = %d, want 2", len(got.AFEs))
	}
	// Sorted by ID ascending → 0 first, then 0xa1b2c3.
	if got.AFEs[0].AfeID != 0 || !got.AFEs[0].PendingGC {
		t.Errorf("afes[0] = %+v, want id=0 pending-gc", got.AFEs[0])
	}
	if got.AFEs[1].AfeID != 0xa1b2c3 || got.AFEs[1].PendingGC {
		t.Errorf("afes[1] = %+v, want id=0xa1b2c3 active", got.AFEs[1])
	}
}

func TestAfez_Index_NilProvider(t *testing.T) {
	h := newAfezHandler(nil)
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No AFE buckets recorded yet") {
		t.Errorf("expected empty-state message, got: %s", rec.Body.String())
	}
}
