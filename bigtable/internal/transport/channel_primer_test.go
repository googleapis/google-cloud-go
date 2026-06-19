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
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

// TestPingAndWarmChannelPrimer_Prime verifies the primer delegates to
// BigtableConn.Prime carrying the configured instance name, app profile,
// and feature-flag metadata — i.e. that the primer is a thin wrapper that
// keeps the (instance, profile, flags) tuple in one place.
func TestPingAndWarmChannelPrimer_Prime(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	conn, err := dialBigtableserver(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	flagsMD := metadata.Pairs("bigtable-features", "primer-test")
	primer := newPingAndWarmChannelPrimer(testInstanceName, testAppProfile, flagsMD)

	if err := primer.Prime(context.Background(), conn); err != nil {
		t.Fatalf("Prime returned error: %v", err)
	}

	if got := fake.getPingCount(); got != 1 {
		t.Errorf("PingAndWarm call count = %d, want 1", got)
	}

	gotMD := fake.getPrimeMetadata()
	if got := gotMD.Get("bigtable-features"); len(got) != 1 || got[0] != "primer-test" {
		t.Errorf("feature-flag metadata on PingAndWarm = %v, want [primer-test]", got)
	}
	if got := gotMD.Get("x-goog-request-params"); len(got) != 1 {
		t.Errorf("x-goog-request-params on PingAndWarm = %v, want one entry derived from instance/profile", got)
	}
}

// TestConnectionFactory_NilPrimerSkipsPriming verifies the contract that a
// nil ChannelPrimer turns priming off: newEntry dials the channel and
// returns it without issuing PingAndWarm.
func TestConnectionFactory_NilPrimerSkipsPriming(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)

	factory := &connectionFactory{
		dial:   func() (*BigtableConn, error) { return dialBigtableserver(addr) },
		primer: nil,
	}

	entry, err := factory.newEntry(context.Background())
	if err != nil {
		t.Fatalf("newEntry returned error: %v", err)
	}
	t.Cleanup(func() { entry.conn.Close() })

	if got := fake.getPingCount(); got != 0 {
		t.Errorf("PingAndWarm call count with nil primer = %d, want 0", got)
	}
}
