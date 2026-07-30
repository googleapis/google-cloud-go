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
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestAuthorizedViewSessionSandbox drives Client.OpenAuthorizedView
// end-to-end against a live Bigtable instance and asserts round-trip
// on both the classic and session paths of the diverter. Creates the
// AV if missing (SubsetView pinned to a stable row prefix + single
// family qualifier) so the test is idempotent across runs — the AV
// is not deleted between runs so multiple test invocations reuse it.
//
// AV path is distinct from the plain-table path: different RPC field
// (AuthorizedViewName vs TableName), different session pool per
// (av, permission) tuple, different resource-prefix header. Failure
// on the session path here means the session-side AV wiring
// regressed independently of table reads working.
//
// Gated on CBT_RUN_SANDBOX so `go test ./...` in dev doesn't dial a
// real instance. Defaults target autonomous-mote-782 / sushanb-uc1 /
// sushanb / cf12; every field env-var-overridable via CBT_SANDBOX_*.
func TestAuthorizedViewSessionSandbox(t *testing.T) {
	tgt := sandboxAVTargetFromEnv(t)
	const (
		authorizedViewID = "session-test-av"
		rowPrefix        = "session-av:"
		qualifier        = "colq"
	)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ensure the AV exists. Uses the prod admin endpoint by default;
	// override with CBT_SANDBOX_ADMIN_ENDPOINT for sandbox admin
	// hosts. Skip cleanly (not fail) if admin creds are unavailable —
	// this test isn't meant to gate on admin auth working, only to
	// exercise the AV data path.
	adminOpts := []option.ClientOption{}
	if tgt.adminEndpoint != "" {
		adminOpts = append(adminOpts, option.WithEndpoint(tgt.adminEndpoint))
	}
	adminClient, err := NewAdminClient(ctx, tgt.project, tgt.instance, adminOpts...)
	if err != nil {
		t.Skipf("admin client unavailable, skipping AV sandbox test: %v", err)
	}
	defer adminClient.Close()

	conf := &AuthorizedViewConf{
		TableID:          tgt.table,
		AuthorizedViewID: authorizedViewID,
		AuthorizedView: &SubsetViewConf{
			RowPrefixes: [][]byte{[]byte(rowPrefix)},
			FamilySubsets: map[string]FamilySubset{
				tgt.family: {Qualifiers: [][]byte{[]byte(qualifier)}},
			},
		},
	}
	if err := adminClient.CreateAuthorizedView(ctx, conf); err != nil {
		if status.Code(err) != codes.AlreadyExists {
			t.Fatalf("CreateAuthorizedView: %v", err)
		}
		t.Logf("AV %q already exists — reusing", authorizedViewID)
	} else {
		t.Logf("created AV %q", authorizedViewID)
	}

	c := newSandboxAVClient(ctx, t, tgt)
	defer c.Close()
	av := c.OpenAuthorizedView(tgt.table, authorizedViewID)

	runID := time.Now().UnixNano()
	cases := []struct {
		name string
		load float64
	}{
		{"classic", 0.0},
		{"session", 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.diverter.SetSessionLoad(tc.load)
			rowKey := fmt.Sprintf("%s%s-%d", rowPrefix, tc.name, runID)
			value := []byte(fmt.Sprintf("av-%s-%d", tc.name, runID))

			mut := NewMutation()
			mut.Set(tgt.family, qualifier, ServerTime, value)
			if err := av.Apply(ctx, rowKey, mut); err != nil {
				t.Fatalf("Apply(%q): %v", rowKey, err)
			}

			row, err := av.ReadRow(ctx, rowKey)
			if err != nil {
				t.Fatalf("ReadRow(%q): %v", rowKey, err)
			}
			items := row[tgt.family]
			if len(items) == 0 {
				families := make([]string, 0, len(row))
				for k := range row {
					families = append(families, k)
				}
				t.Fatalf("ReadRow(%q): family %q missing; got families=%v",
					rowKey, tgt.family, families)
			}
			if got := items[0].Value; string(got) != string(value) {
				t.Errorf("ReadRow(%q): value=%q want=%q", rowKey, got, value)
			}
			t.Logf("%s: AV round-trip OK column=%s value=%s",
				tc.name, items[0].Column, items[0].Value)
		})
	}
}

type sandboxAVTarget struct {
	project, instance, table, family string
	endpoint, adminEndpoint          string
}

// sandboxAVTargetFromEnv resolves the sandbox target from env vars.
// Skips the test when the CBT_RUN_SANDBOX gate is unset so the suite
// stays inert under `go test ./...`.
func sandboxAVTargetFromEnv(t *testing.T) sandboxAVTarget {
	t.Helper()
	const gate = "CBT_RUN_SANDBOX"
	if os.Getenv(gate) == "" {
		t.Skipf("sandbox test skipped: set %s=1 to run", gate)
	}
	getenv := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return sandboxAVTarget{
		project:       getenv("CBT_SANDBOX_PROJECT", "autonomous-mote-782"),
		instance:      getenv("CBT_SANDBOX_INSTANCE", "sushanb-uc1"),
		table:         getenv("CBT_SANDBOX_TABLE", "sushanb"),
		family:        getenv("CBT_SANDBOX_COLUMN_FAMILY", "cf12"),
		endpoint:      os.Getenv("CBT_SANDBOX_ENDPOINT"),
		adminEndpoint: os.Getenv("CBT_SANDBOX_ADMIN_ENDPOINT"),
	}
}

func newSandboxAVClient(ctx context.Context, t *testing.T, tgt sandboxAVTarget) *Client {
	t.Helper()
	opts := []option.ClientOption{}
	if tgt.endpoint != "" {
		opts = append(opts, option.WithEndpoint(tgt.endpoint))
	}
	c, err := NewClientWithConfig(ctx, tgt.project, tgt.instance,
		ClientConfig{MetricsProvider: NoopMetricsProvider{}}, opts...)
	if err != nil {
		t.Fatalf("NewClientWithConfig(project=%s instance=%s endpoint=%q): %v",
			tgt.project, tgt.instance, tgt.endpoint, err)
	}
	return c
}
