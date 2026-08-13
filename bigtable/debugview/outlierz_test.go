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
	"strings"
	"testing"
	"time"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func TestOutlierz_NilProviderRendersNotEnabled(t *testing.T) {
	h := newOutlierzHandler(nil)
	rec := get(t, h, "/")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "outlier debug not enabled") {
		t.Errorf("body missing not-enabled banner: %s", head([]byte(body), 300))
	}
}

func TestOutlierz_RendersPools(t *testing.T) {
	prov := fakeOutlierProvider{
		pools: []btransport.OutlierPoolSnapshot{{
			PoolName:   "read-a",
			ScorerName: "latency-outlier",
			CapturedAt: time.Now(),
			Params: []btransport.OutlierParam{
				{Name: "Interval", Value: "30s"},
				{Name: "LatencyMultiplier", Value: "3.00x"},
			},
			Scores: []btransport.OutlierScoreRow{
				{AfeID: 1, Score: 1.0, Penalized: false},
				{AfeID: 2, Score: 10.0, Penalized: true},
			},
			Recent: []btransport.OutlierDecision{{
				When:         time.Now(),
				AfeID:        2,
				OldScore:     1.0,
				NewScore:     10.0,
				Signal:       float64(100 * time.Millisecond),
				CohortMedian: float64(1 * time.Millisecond),
				Reason:       "penalized-latency",
			}},
		}},
	}
	rec := get(t, newOutlierzHandler(prov), "/")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"read-a",
		"latency-outlier",
		"1/2 AFEs penalized",
		"Interval",
		"30s",
		"LatencyMultiplier",
		"10.00×",
		"penalized-latency",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n\n%s", want, head([]byte(body), 800))
		}
	}
}

func TestOutlierz_JSON(t *testing.T) {
	prov := fakeOutlierProvider{
		pools: []btransport.OutlierPoolSnapshot{{
			PoolName:   "write-x",
			ScorerName: "noop",
			CapturedAt: time.Now(),
		}},
	}
	rec := get(t, newOutlierzHandler(prov), "/?format=json")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Available bool `json:"available"`
		Pools     []struct {
			Pool   string `json:"pool"`
			Scorer string `json:"scorer"`
		} `json:"pools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if !got.Available {
		t.Errorf("Available = false, want true (provider is non-nil)")
	}
	if len(got.Pools) != 1 || got.Pools[0].Pool != "write-x" || got.Pools[0].Scorer != "noop" {
		t.Errorf("pools = %+v, want one entry {write-x, noop}", got.Pools)
	}
}
