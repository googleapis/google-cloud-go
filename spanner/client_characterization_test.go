/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"strings"
	"testing"

	. "cloud.google.com/go/spanner/internal/testutil"
	"cloud.google.com/go/spanner/internal/transport"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
)

func TestNewClientWithConfig_Characterization(t *testing.T) {
	t.Run("static pool default and custom NumChannels and conflict check", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			DisableNativeMetrics: true,
		})
		defer teardown()
		if client.sc == nil || client.sc.connPool == nil {
			t.Fatal("expected non-nil sessionClient and connPool")
		}
		if got := client.sc.connPool.Num(); got != numChannels {
			t.Fatalf("expected default pool.Num()=%d, got %d", numChannels, got)
		}

		// Custom NumChannels configures the static pool size.
		_, customClient, customTeardown := setupMockedTestServerWithConfig(t, ClientConfig{
			NumChannels:          2,
			DisableNativeMetrics: true,
		})
		defer customTeardown()
		if got := customClient.sc.connPool.Num(); got != 2 {
			t.Fatalf("expected custom pool.Num()=2, got %d", got)
		}

		// Conflicting NumChannels and WithGRPCConnectionPool returns InvalidArgument *spanner.Error.
		_, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
		defer serverTeardown()
		badClient, err := NewClientWithConfig(
			context.Background(),
			"projects/p/instances/i/databases/d",
			ClientConfig{
				NumChannels:          4,
				DisableNativeMetrics: true,
			},
			append(opts, option.WithGRPCConnectionPool(6))...,
		)
		if err == nil {
			badClient.Close()
			t.Fatal("expected InvalidArgument when NumChannels=4 conflicts with WithGRPCConnectionPool(6)")
		}
		if ErrCode(err) != codes.InvalidArgument {
			t.Fatalf("expected codes.InvalidArgument, got %v", err)
		}
	})

	t.Run("DCP enabled vs bypassed on SPANNER_EMULATOR_HOST", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		_, dcpClient, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			DisableNativeMetrics: true,
			DynamicChannelPoolConfig: DynamicChannelPoolConfig{
				DCPEnabled:         true,
				DCPInitialChannels: 3,
				DCPMinChannels:     2,
				DCPMaxChannels:     6,
			},
		})
		defer teardown()
		if dcpClient.sc.dynamicPool == nil {
			t.Fatal("expected dynamicPool to be initialized when DCPEnabled=true and emulator unset")
		}
		if got := dcpClient.sc.dynamicPool.Num(); got != 3 {
			t.Fatalf("expected 3 DCP channels, got %d", got)
		}

		// When SPANNER_EMULATOR_HOST is set, DCP is bypassed and a single leading emulator scheme is stripped.
		server, _, serverTeardown := NewMockedSpannerInMemTestServer(t)
		defer serverTeardown()
		for _, scheme := range []string{"", "http://", "https://", "passthrough:///"} {
			t.Setenv("SPANNER_EMULATOR_HOST", scheme+server.ServerAddress)
			emuClient, err := NewClientWithConfig(
				context.Background(),
				"projects/p/instances/i/databases/d",
				ClientConfig{
					DisableNativeMetrics: true,
					DynamicChannelPoolConfig: DynamicChannelPoolConfig{
						DCPEnabled: true,
					},
				},
			)
			if err != nil {
				t.Fatalf("NewClientWithConfig with SPANNER_EMULATOR_HOST(%q) failed: %v", scheme, err)
			}
			if emuClient.sc.dynamicPool != nil {
				emuClient.Close()
				t.Fatalf("expected DCP to be disabled when SPANNER_EMULATOR_HOST(%q) is set", scheme)
			}
			iter := emuClient.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
			for {
				_, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					iter.Stop()
					emuClient.Close()
					t.Fatalf("query via SPANNER_EMULATOR_HOST(%q) failed: %v", scheme, err)
				}
			}
			iter.Stop()
			emuClient.Close()
		}

		// Malformed scheme-only SPANNER_EMULATOR_HOST="http://" must still bind WithEndpoint("passthrough:///")
		// and fail locally rather than dialing production Spanner in plaintext.
		t.Setenv("SPANNER_EMULATOR_HOST", "http://")
		badEmuClient, err := NewClientWithConfig(
			context.Background(),
			"projects/p/instances/i/databases/d",
			ClientConfig{DisableNativeMetrics: true},
		)
		if err == nil {
			defer badEmuClient.Close()
			if conn := badEmuClient.sc.connPool.Conn(); conn != nil && strings.Contains(conn.Target(), "googleapis.com") {
				t.Fatalf("SPANNER_EMULATOR_HOST=http:// dialed production target %q", conn.Target())
			}
		}
	})

	t.Run("compression, end-to-end tracing, user-agent, session labels, and DirectPath env", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_ENABLE_END_TO_END_TRACING", "true")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "false")

		if transport.ResolveDirectPathEnabled(true) {
			t.Fatal("expected GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=false to override EnableDirectAccess=true")
		}
		if transport.ResolveGCPFallbackEnabled() {
			t.Fatal("expected GOOGLE_SPANNER_ENABLE_GCP_FALLBACK=false to disable fallback")
		}

		_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
			Compression:        "gzip",
			UserAgent:          "custom-agent/2.0",
			SessionLabels:      map[string]string{"env": "test"},
			EnableDirectAccess: true,
		})
		defer teardown()

		if client.sc.userAgent != "custom-agent/2.0" {
			t.Fatalf("expected sc.userAgent=custom-agent/2.0, got %q", client.sc.userAgent)
		}
		if got := client.sc.md.Get(requestsCompressionHeader); len(got) != 1 || got[0] != "gzip" {
			t.Fatalf("expected %s=[gzip] in sc.md, got %v", requestsCompressionHeader, got)
		}
		if got := client.sc.md.Get(endToEndTracingHeader); len(got) != 1 || got[0] != "true" {
			t.Fatalf("expected %s=[true] in sc.md, got %v", endToEndTracingHeader, got)
		}
		if client.sc.sessionLabels["env"] != "test" {
			t.Fatalf("expected session label env=test, got %v", client.sc.sessionLabels)
		}
		iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
		defer iter.Stop()
		rowCount := 0
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("unexpected query error: %v", err)
			}
			rowCount++
		}
		if rowCount != 2 {
			t.Fatalf("expected 2 rows from SelectFooFromBar, got %d", rowCount)
		}
	})
}
