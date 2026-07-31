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
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/protobuf/types/known/durationpb"
)

func configzSampleSnapshot() btransport.ConfigSnapshot {
	return btransport.ConfigSnapshot{
		InstanceName: "projects/p/instances/inst",
		AppProfileID: "my-app-profile",
		ConfigSeq:    7,
		ValidUntil:   time.Now().Add(5 * time.Minute),
		FetchedAt:    time.Now().Add(-30 * time.Second),
		CapturedAt:   time.Now(),
		Response: &bigtablepb.ClientConfiguration{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval:  durationpb.New(5 * time.Minute),
					ValidityDuration: durationpb.New(10 * time.Minute),
					MaxRpcRetryCount: 3,
				},
			},
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{
				SessionLoad: 1.0,
				SessionPoolConfiguration: &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
					MinSessionCount: 5,
					MaxSessionCount: 100,
				},
			},
		},
	}
}

func TestConfigz_HTML_RendersFields(t *testing.T) {
	h := newConfigzHandler(fakeConfigProvider{snap: configzSampleSnapshot()})
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"projects/p/instances/inst",
		"my-app-profile",
		"polling_interval",
		"session_load",
		"max_session_count",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

func TestConfigz_JSON_ReturnsProtoJSON(t *testing.T) {
	h := newConfigzHandler(fakeConfigProvider{snap: configzSampleSnapshot()})
	rec := get(t, h, "/?format=json")

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"polling_configuration"`,
		`"session_configuration"`,
		`"session_load"`,
		`"max_session_count"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("JSON missing %q; body=%q", want, body)
		}
	}
}

func TestConfigz_NoProvider(t *testing.T) {
	h := newConfigzHandler(nil)
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No configuration manager") {
		t.Errorf("expected no-provider message; got %q", rec.Body.String())
	}
}

func TestConfigz_LastErr_RendersBanner(t *testing.T) {
	snap := configzSampleSnapshot()
	snap.LastErr = errors.New("simulated transient failure")
	snap.LastErrAt = time.Now().Add(-2 * time.Second)
	h := newConfigzHandler(fakeConfigProvider{snap: snap})
	rec := get(t, h, "/")

	body := rec.Body.String()
	if !strings.Contains(body, "Last poll error") {
		t.Errorf("expected error banner; got %q", body)
	}
	if !strings.Contains(body, "simulated transient failure") {
		t.Errorf("expected err message in page; got %q", body)
	}
}

func TestConfigz_NoResponseYet(t *testing.T) {
	snap := btransport.ConfigSnapshot{
		InstanceName: "projects/p/instances/inst",
		CapturedAt:   time.Now(),
	}
	h := newConfigzHandler(fakeConfigProvider{snap: snap})
	rec := get(t, h, "/")
	if !strings.Contains(rec.Body.String(), "No successful GetClientConfiguration") {
		t.Errorf("expected empty-response message; got %q", rec.Body.String())
	}
}

func TestConfigz_JSON_NullWhenEmpty(t *testing.T) {
	snap := btransport.ConfigSnapshot{CapturedAt: time.Now()}
	h := newConfigzHandler(fakeConfigProvider{snap: snap})
	rec := get(t, h, "/?format=json")
	if got := strings.TrimSpace(rec.Body.String()); got != "null" {
		t.Errorf("empty-response JSON = %q, want null", got)
	}
}

func TestConfigz_NotFound(t *testing.T) {
	h := newConfigzHandler(fakeConfigProvider{snap: configzSampleSnapshot()})
	rec := get(t, h, "/anything-else")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
