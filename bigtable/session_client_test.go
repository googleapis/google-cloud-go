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

import (
	"context"
	"testing"

	"cloud.google.com/go/bigtable/bttest"
)

// newTestSessionClient stands up an in-memory bttest server, points the
// emulator env var at it, and constructs a SessionClient. Returned
// cleanup closes both.
func newTestSessionClient(t *testing.T) (*SessionClient, func()) {
	t.Helper()
	srv, err := bttest.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("bttest.NewServer: %v", err)
	}
	t.Setenv("BIGTABLE_EMULATOR_HOST", srv.Addr)

	sc, err := NewSessionClient(context.Background(), "test-project", "test-instance",
		ClientConfig{AppProfile: "test-profile"})
	if err != nil {
		srv.Close()
		t.Fatalf("NewSessionClient: %v", err)
	}
	cleanup := func() {
		if err := sc.Close(); err != nil {
			t.Errorf("SessionClient.Close: %v", err)
		}
		srv.Close()
	}
	return sc, cleanup
}

func TestNewSessionClient_Accessors(t *testing.T) {
	sc, cleanup := newTestSessionClient(t)
	defer cleanup()

	if got, want := sc.Project(), "test-project"; got != want {
		t.Errorf("Project() = %q, want %q", got, want)
	}
	if got, want := sc.Instance(), "test-instance"; got != want {
		t.Errorf("Instance() = %q, want %q", got, want)
	}
	if got, want := sc.AppProfile(), "test-profile"; got != want {
		t.Errorf("AppProfile() = %q, want %q", got, want)
	}
	if sc.ConfigurationManager() == nil {
		t.Error("ConfigurationManager() = nil, want non-nil")
	}
}

func TestNewSessionClient_EmulatorNoopMetrics(t *testing.T) {
	sc, cleanup := newTestSessionClient(t)
	defer cleanup()

	// Emulator env forces NoopMetricsProvider inside the constructor
	// (mirrors NewClientWithConfig). The tracer factory must report
	// metrics disabled so we don't try to talk to Cloud Monitoring
	// during emulator runs.
	if sc.metricsTracerFactory.Enabled {
		t.Errorf("metricsTracerFactory.Enabled = true under BIGTABLE_EMULATOR_HOST, want false")
	}
}

func TestSessionClient_Close_Idempotent(t *testing.T) {
	sc, _ := newTestSessionClient(t) // don't defer cleanup — we drive Close manually below

	if err := sc.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second Close must not panic and must not error.
	if err := sc.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
