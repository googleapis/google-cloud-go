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
	"os"
	"testing"

	"google.golang.org/api/option"
)

// Shared configuration for sandbox tests that toggle between the
// classic and session data paths via the client's diverter. Defaults
// point at sushanb-uc1 / sushanb on the prod endpoint; every field is
// env-var-overridable so operators can retarget the suite without
// editing source.
//
// Sandbox tests are gated on CBT_RUN_SANDBOX so `go test ./...` in
// dev doesn't accidentally dial a real instance. Set CBT_RUN_SANDBOX=1
// (or any non-empty value) to run them.
const (
	sandboxProjectDefault  = "autonomous-mote-782"
	sandboxInstanceDefault = "sushanb-uc1"
	sandboxTableDefault    = "sushanb"
	sandboxFamilyDefault   = "cf12"

	sandboxEnvGate     = "CBT_RUN_SANDBOX"
	sandboxEnvProject  = "CBT_SANDBOX_PROJECT"
	sandboxEnvInstance = "CBT_SANDBOX_INSTANCE"
	sandboxEnvTable    = "CBT_SANDBOX_TABLE"
	sandboxEnvFamily   = "CBT_SANDBOX_COLUMN_FAMILY"
	sandboxEnvEndpoint = "CBT_SANDBOX_ENDPOINT"       // optional; empty → prod default
	sandboxEnvAdmin    = "CBT_SANDBOX_ADMIN_ENDPOINT" // optional; empty → prod default
)

type sandboxTarget struct {
	project, instance, table, family string
	endpoint, adminEndpoint          string
}

// sandboxFromEnv reads the target from env vars, falling back to the
// sushanb-uc1 / sushanb defaults. Calls t.Skip if CBT_RUN_SANDBOX is
// unset so the suite stays inert under `go test ./...`.
func sandboxFromEnv(t *testing.T) sandboxTarget {
	t.Helper()
	if os.Getenv(sandboxEnvGate) == "" {
		t.Skipf("sandbox test skipped: set %s=1 to run", sandboxEnvGate)
	}
	getenv := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return sandboxTarget{
		project:       getenv(sandboxEnvProject, sandboxProjectDefault),
		instance:      getenv(sandboxEnvInstance, sandboxInstanceDefault),
		table:         getenv(sandboxEnvTable, sandboxTableDefault),
		family:        getenv(sandboxEnvFamily, sandboxFamilyDefault),
		endpoint:      os.Getenv(sandboxEnvEndpoint),
		adminEndpoint: os.Getenv(sandboxEnvAdmin),
	}
}

// newSandboxClient builds a Client for the sandbox target. Endpoint is
// injected only if the operator overrode it — empty falls back to the
// package's prod default (bigtable.googleapis.com:443) so the suite
// hits prod-shaped stacks like sushanb-uc1 without extra plumbing.
func newSandboxClient(ctx context.Context, t *testing.T, tgt sandboxTarget) *Client {
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

// pinSessionLoad forces the diverter to one side of the split. Same-
// package access so tests can drive the routing directly instead of
// waiting for ClientConfigurationManager to push a SessionLoad update.
// Pass 0.0 for classic-only, 1.0 for session-only.
func pinSessionLoad(c *Client, load float64) {
	c.diverter.SetSessionLoad(load)
}
