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
	"testing"
	"time"
)

// TestTableSessionSandbox drives Client.OpenTable end-to-end against
// the sandbox and asserts a write+read round-trip works on both the
// classic and session paths of the diverter. Subtests share one
// Client so we exercise the sessionTables cache too — a second
// OpenTable(same name) must hit the cached handle.
func TestTableSessionSandbox(t *testing.T) {
	tgt := sandboxFromEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c := newSandboxClient(ctx, t, tgt)
	defer c.Close()

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
			pinSessionLoad(c, tc.load)
			tbl := c.OpenTable(tgt.table)
			rowKey := fmt.Sprintf("__sandbox-table-%s-%d", tc.name, runID)
			value := []byte(fmt.Sprintf("val-%s-%d", tc.name, runID))

			mut := NewMutation()
			mut.Set(tgt.family, "colq", ServerTime, value)
			if err := tbl.Apply(ctx, rowKey, mut); err != nil {
				t.Fatalf("Apply(%q): %v", rowKey, err)
			}

			row, err := tbl.ReadRow(ctx, rowKey)
			if err != nil {
				t.Fatalf("ReadRow(%q): %v", rowKey, err)
			}
			items := row[tgt.family]
			if len(items) == 0 {
				t.Fatalf("ReadRow(%q): family %q empty; row=%+v", rowKey, tgt.family, row)
			}
			if got := items[0].Value; string(got) != string(value) {
				t.Errorf("ReadRow(%q): value=%q want=%q", rowKey, got, value)
			}
			t.Logf("%s: round-trip OK on family=%s column=%s",
				tc.name, tgt.family, items[0].Column)
		})
	}
}

// TestReadNonExistentRowSandbox probes the "row not found → nil row"
// contract on both paths. Same key is used for classic and session so
// any shape divergence (e.g. session accidentally emitting an empty-
// but-non-nil Row) is immediately visible.
func TestReadNonExistentRowSandbox(t *testing.T) {
	tgt := sandboxFromEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := newSandboxClient(ctx, t, tgt)
	defer c.Close()

	missingKey := fmt.Sprintf("__missing-row-%d", time.Now().UnixNano())
	tbl := c.OpenTable(tgt.table)

	cases := []struct {
		name string
		load float64
	}{
		{"classic", 0.0},
		{"session", 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinSessionLoad(c, tc.load)
			row, err := tbl.ReadRow(ctx, missingKey)
			if err != nil {
				t.Fatalf("%s ReadRow(%q): %v", tc.name, missingKey, err)
			}
			if row != nil {
				t.Errorf("%s ReadRow(%q): expected nil for missing row, got %#v",
					tc.name, missingKey, row)
			}
			t.Logf("%s: row==nil=%v len=%d", tc.name, row == nil, len(row))
		})
	}
}
