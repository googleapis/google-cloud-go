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
	"errors"
	"strings"
	"testing"

	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewClientWithConfig_Characterization(t *testing.T) {
	if useGRPCgcp {
		t.Skip("Skipping pool characterization test when using grpc-gcp")
	}

	t.Run("static and fallback pool sizing and mismatch error chain", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")
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

		// Conflicting NumChannels and WithGRPCConnectionPool returns an exact InvalidArgument
		// *spanner.Error -> *apierror.APIError -> *status.Error chain on both static and fallback pools.
		_, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
		defer serverTeardown()

		const wantStaticDesc = "Connection pool mismatch: NumChannels=4, WithGRPCConnectionPool=6. Only set one of these options, or set both to the same value."
		for _, legacyStatic := range []string{"false", "true"} {
			t.Setenv("GOOGLE_SPANNER_ENABLE_LEGACY_STATIC_CHANNEL_POOL", legacyStatic)
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
				t.Fatalf("legacyStatic=%s: expected InvalidArgument when NumChannels=4 conflicts with WithGRPCConnectionPool(6)", legacyStatic)
			}
			assertPoolMismatchErrorChain(t, err, wantStaticDesc)
		}
		t.Setenv("GOOGLE_SPANNER_ENABLE_LEGACY_STATIC_CHANNEL_POOL", "")

		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
		const wantFallbackDesc = "Connection pool mismatch: NumChannels=4, primaryConn.Num()=6, fallbackConn.Num()=6"
		badFbClient, fbErr := NewClientWithConfig(
			context.Background(),
			"projects/p/instances/i/databases/d",
			ClientConfig{
				NumChannels:          4,
				EnableDirectAccess:   true,
				DisableNativeMetrics: true,
			},
			append(opts, option.WithGRPCConnectionPool(6))...,
		)
		if fbErr == nil {
			badFbClient.Close()
			t.Fatal("expected InvalidArgument on fallback pool NumChannels mismatch")
		}
		assertPoolMismatchErrorChain(t, fbErr, wantFallbackDesc)
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
			t.Fatalf("expected 3 DCP channels (distinct from default NumChannels=%d), got %d", numChannels, got)
		}

		// When SPANNER_EMULATOR_HOST is set, dynamic DCP mode is bypassed (pool uses NumChannels
		// instead of DCPInitialChannels) and a single leading emulator scheme is stripped.
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
						DCPEnabled:         true,
						DCPInitialChannels: 3,
						DCPMinChannels:     2,
						DCPMaxChannels:     6,
					},
				},
			)
			if err != nil {
				t.Fatalf("NewClientWithConfig with SPANNER_EMULATOR_HOST(%q) failed: %v", scheme, err)
			}
			if got := emuClient.sc.connPool.Num(); got != numChannels {
				emuClient.Close()
				t.Fatalf("expected dynamic DCP mode to be bypassed on SPANNER_EMULATOR_HOST(%q): pool.Num()=%d, want default NumChannels=%d", scheme, got, numChannels)
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
		// and fail locally in the passthrough resolver rather than dialing production Spanner in plaintext.
		t.Setenv("SPANNER_EMULATOR_HOST", "http://")
		badEmuClient, err := NewClientWithConfig(
			context.Background(),
			"projects/p/instances/i/databases/d",
			ClientConfig{DisableNativeMetrics: true},
		)
		if err != nil {
			if !strings.Contains(err.Error(), "passthrough") || strings.Contains(err.Error(), "googleapis.com") {
				t.Fatalf("expected local passthrough resolver failure for SPANNER_EMULATOR_HOST=http://, got %v", err)
			}
		} else {
			conn := badEmuClient.sc.connPool.Conn()
			target := ""
			if conn != nil {
				target = conn.Target()
			}
			badEmuClient.Close()
			if conn == nil || target != "passthrough:///" {
				t.Fatalf("expected target passthrough:/// when SPANNER_EMULATOR_HOST=http://, got %q", target)
			}
		}
	})

	t.Run("compression, end-to-end tracing, user-agent, session labels, and DirectPath env", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_ENABLE_END_TO_END_TRACING", "true")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "false")

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
		// GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=false overrides EnableDirectAccess=true,
		// so the DirectPath-implied AFE server-timing header must remain unset.
		if got := client.sc.md.Get(afeMetricHeader); len(got) != 0 {
			t.Fatalf("expected GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=false to keep %s unset, got %v", afeMetricHeader, got)
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

func assertPoolMismatchErrorChain(t *testing.T, err error, wantDesc string) {
	t.Helper()
	if ErrCode(err) != codes.InvalidArgument {
		t.Fatalf("ErrCode(err) = %v, want InvalidArgument (%v)", ErrCode(err), err)
	}
	wantStatusErr := status.Error(codes.InvalidArgument, wantDesc)
	if !errors.Is(err, wantStatusErr) {
		t.Fatalf("expected errors.Is(err, %v) to be true; got err=%v", wantStatusErr, err)
	}
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierror.APIError in error chain, got %T (%v)", err, err)
	}
	if apiErr.Error() != wantStatusErr.Error() {
		t.Fatalf("apiErr.Error() = %q, want %q", apiErr.Error(), wantStatusErr.Error())
	}
}
