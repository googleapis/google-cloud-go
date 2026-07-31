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

func sessionzSampleSnapshot() []btransport.PoolSnapshot {
	return []btransport.PoolSnapshot{
		{
			Name:          "my-table:read",
			SessionType:   "table",
			MinSessions:   2,
			MaxSessions:   10,
			PickerType:    "RandomPicker",
			ReadyCount:    2,
			StartingCount: 0,
			InUseCount:    1,
			PendingCount:  3,
			TotalSessions: 2,
			CapturedAt:    time.Now(),
			Sessions: []btransport.SessionSnapshot{
				{
					LogName:           "session-read-1",
					State:             "Ready",
					SessionType:       "table",
					LastStateChange:   time.Now().Add(-90 * time.Second),
					OkRpcs:            42,
					ErrorRpcs:         1,
					ActiveRpcs:        3,
					HeartbeatInterval: 10 * time.Second,
					NextHeartbeat:     time.Now().Add(20 * time.Second),
					Peer: btransport.PeerInfoSnapshot{
						TransportType:              "session_directpath",
						GoogleFrontendID:           12345,
						ApplicationFrontendID:      67890,
						ApplicationFrontendRegion:  "us-central1",
						ApplicationFrontendSubzone: "us-central1-b1",
					},
					Handle: btransport.SessionHandleSnapshot{
						Outstanding:  3,
						LastActivity: time.Now().Add(-5 * time.Second),
					},
				},
				{
					LogName:     "session-read-2",
					State:       "Starting",
					SessionType: "table",
				},
			},
		},
	}
}

func TestSessionz_Index_HTML_RendersPools(t *testing.T) {
	h := newSessionzHandler(fakeSessionProvider{pools: sessionzSampleSnapshot()})
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"my-table:read", "RandomPicker", "table", "Bigtable Session Pools"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestSessionz_Index_JSON_RoundTrips(t *testing.T) {
	pools := sessionzSampleSnapshot()
	h := newSessionzHandler(fakeSessionProvider{pools: pools})
	rec := get(t, h, "/?format=json")

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got struct {
		Pools    []btransport.PoolSnapshot   `json:"pools"`
		Diverter btransport.DiverterSnapshot `json:"diverter"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(got.Pools) != 1 || got.Pools[0].Name != pools[0].Name {
		t.Errorf("JSON round-trip mismatch: %+v", got)
	}
	if len(got.Pools[0].Sessions) != 2 {
		t.Errorf("Sessions len = %d, want 2", len(got.Pools[0].Sessions))
	}
}

func TestSessionz_Pool_HTML_RendersSessions(t *testing.T) {
	h := newSessionzHandler(fakeSessionProvider{pools: sessionzSampleSnapshot()})
	rec := get(t, h, "/pool/my-table:read")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	wants := []string{
		"my-table:read",
		"session-read-1", "session-read-2",
		"Ready", "Starting",
		"session_directpath",
		"us-central1", "us-central1-b1",
		"42", // OK count
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("pool detail missing %q", w)
		}
	}
}

func TestSessionz_Pool_NotFound(t *testing.T) {
	h := newSessionzHandler(fakeSessionProvider{pools: sessionzSampleSnapshot()})
	rec := get(t, h, "/pool/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSessionz_Index_NoProvider_Disabled(t *testing.T) {
	h := newSessionzHandler(nil)
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Session pooling is disabled") {
		t.Errorf("expected disabled message; got %q", rec.Body.String())
	}
}

func TestSessionz_Index_NoPools_Empty(t *testing.T) {
	h := newSessionzHandler(fakeSessionProvider{pools: nil})
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No session pools") {
		t.Errorf("expected empty-state message; got %q", rec.Body.String())
	}
}

func TestSessionz_Pool_JSON(t *testing.T) {
	pools := sessionzSampleSnapshot()
	h := newSessionzHandler(fakeSessionProvider{pools: pools})
	rec := get(t, h, "/pool/my-table:read?format=json")

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got btransport.PoolSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if got.Name != pools[0].Name {
		t.Errorf("Name = %q, want %q", got.Name, pools[0].Name)
	}
}
