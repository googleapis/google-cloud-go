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
	"log"
)

// ExampleNewClient_multipleClientsShare shows that two NewClient calls
// with the same (project, instance, appProfile, endpoint) automatically
// share ONE underlying session.Client — one gRPC channel pool, one
// ClientConfigurationManager poll goroutine, and one per-resource
// session-pool set. c1 and c2 are distinct *Client values, but the
// session data plane behind them is a single shared instance. The
// shared session only tears down when the LAST *Client using it calls
// Close.
//
// This behavior is transparent to callers — the two Clients still have
// their own classic gRPC ConnPools, metrics tracer factories, and
// Diverters. Sharing is scoped to the session data plane. Applications
// that construct many logical *Client values against the same instance
// (multi-tenant serving, notebook cells, per-request client factories)
// get O(1) session-side connection cost instead of O(N).
func ExampleNewClient_multipleClientsShare() {
	ctx := context.Background()

	c1, err := NewClient(ctx, "my-project", "my-instance")
	if err != nil {
		log.Fatalf("NewClient c1: %v", err)
	}
	defer c1.Close()

	// A second NewClient call with matching identity reuses the
	// underlying session.Client. Total session-side connection cost:
	// one channel pool + one config-poll goroutine, not two.
	c2, err := NewClient(ctx, "my-project", "my-instance")
	if err != nil {
		log.Fatalf("NewClient c2: %v", err)
	}
	defer c2.Close()

	// Each Client is used independently. When both are closed, the
	// shared session's refcount drops to zero and the underlying
	// resources are released.
	_ = c1.Open("my-table")
	_ = c2.Open("my-table")
}

// ExampleNewClientWithConfig_incompatibleOptions shows the guardrail:
// two NewClient calls with matching identity but incompatible options
// (different MetricsProvider, different feature-flag settings, etc.)
// return an error at NewClient time so the second caller cannot
// silently attach to a session configured differently from what it
// asked for. Callers get a diff-style message naming the diverging
// fields.
//
// To use different options against the same (project, instance), use
// different resource identifiers — e.g. distinct app profiles — so the
// two Clients fall into different shared-session buckets.
func ExampleNewClientWithConfig_incompatibleOptions() {
	ctx := context.Background()

	// First Client: default MetricsProvider (enables Cloud Monitoring
	// export).
	c1, err := NewClientWithConfig(ctx, "my-project", "my-instance", ClientConfig{})
	if err != nil {
		log.Fatalf("NewClientWithConfig c1: %v", err)
	}
	defer c1.Close()

	// Second Client with matching identity but a different
	// MetricsProvider — this is rejected at NewClient time to prevent
	// a silent attach to a session with different metrics behavior.
	// The returned error names the diverging field.
	c2, err := NewClientWithConfig(ctx, "my-project", "my-instance", ClientConfig{
		MetricsProvider: NoopMetricsProvider{},
	})
	if err != nil {
		log.Printf("expected: %v", err)
		return
	}
	defer c2.Close()
}
