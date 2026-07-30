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
	"testing"

	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/grpc/metadata"
)

// newBareClientForOpenTests builds a Client with just enough state for
// the Open* factory paths — no gRPC connection, no metrics tracer, no
// pools. Sufficient because Open* only reads project/instance/
// appProfile/featureFlagsMD/diverter and constructs Table + TableShim
// values without dialing.
func newBareClientForOpenTests(t *testing.T, sessionLoad float64) *Client {
	t.Helper()
	return &Client{
		project:        "p",
		instance:       "i",
		appProfile:     "ap",
		featureFlagsMD: metadata.MD{},
		diverter:       btransport.NewDiverter(sessionLoad),
	}
}

// TestOpen_ReturnsBareTable pins the post-Option-B-revert contract:
// Client.Open returns a plain *Table with no session-routing wrapper.
// Callers holding *Table (BulkMutation, ReadModifyWrite consumers,
// external code) stay on the classic path regardless of Client's
// diverter setting.
func TestOpen_ReturnsBareTable(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0)

	tbl := c.Open("mytable")
	if tbl == nil {
		t.Fatal("Open returned nil")
	}
	if tbl.c != c {
		t.Errorf("Open→Table.c = %p, want %p", tbl.c, c)
	}
	if tbl.table != "mytable" {
		t.Errorf("Open→Table.table = %q, want %q", tbl.table, "mytable")
	}
	if tbl.authorizedView != "" {
		t.Errorf("Open→Table.authorizedView = %q, want empty", tbl.authorizedView)
	}
	if tbl.materializedView != "" {
		t.Errorf("Open→Table.materializedView = %q, want empty", tbl.materializedView)
	}
}

// TestOpenTable_ProducesNilSessionShim pins the shim shape returned by
// OpenTable: a *TableShim whose classic side is a *tableImpl and whose
// session side is nil (session data path isn't wired here). The
// client's Diverter is passed through so a future ratio bump takes
// effect without re-opening. The already-existing
// TestTableShim_NilSession_AllMethodsFallBackToClassic covers the
// behavioral consequence — with session == nil, every routing decision
// falls through to classic.
func TestOpenTable_ProducesNilSessionShim(t *testing.T) {
	c := newBareClientForOpenTests(t, 1.0) // SessionLoad=1.0 to prove the shim still picks classic when session is nil.

	got := c.OpenTable("mytable")
	shim, ok := got.(*TableShim)
	if !ok {
		t.Fatalf("OpenTable returned %T, want *TableShim", got)
	}
	if shim.session != nil {
		t.Errorf("OpenTable→TableShim.session = %v, want nil (session data path not wired)", shim.session)
	}
	if shim.diverter != c.diverter {
		t.Errorf("OpenTable→TableShim.diverter = %p, want client's diverter %p", shim.diverter, c.diverter)
	}
	inner, ok := shim.classic.(*tableImpl)
	if !ok {
		t.Fatalf("OpenTable→TableShim.classic = %T, want *tableImpl", shim.classic)
	}
	if inner.table != "mytable" {
		t.Errorf("classic inner Table.table = %q, want %q", inner.table, "mytable")
	}
	if inner.authorizedView != "" {
		t.Errorf("classic inner Table.authorizedView = %q, want empty", inner.authorizedView)
	}
	if inner.materializedView != "" {
		t.Errorf("classic inner Table.materializedView = %q, want empty", inner.materializedView)
	}
	// useSession() must be false because session is nil, even with
	// SessionLoad=1.0 on the diverter — proves the nil-session
	// short-circuit runs before the diverter is consulted.
	if shim.useSession() {
		t.Errorf("useSession() = true, want false (session is nil so we must not consult the diverter)")
	}
}

// TestOpenAuthorizedView_ProducesNilSessionShim — same contract as
// OpenTable, plus authorizedView is threaded through to the inner
// Table.
func TestOpenAuthorizedView_ProducesNilSessionShim(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0)

	got := c.OpenAuthorizedView("mytable", "myview")
	shim, ok := got.(*TableShim)
	if !ok {
		t.Fatalf("OpenAuthorizedView returned %T, want *TableShim", got)
	}
	if shim.session != nil {
		t.Errorf("session = %v, want nil", shim.session)
	}
	if shim.diverter != c.diverter {
		t.Errorf("diverter = %p, want client's diverter %p", shim.diverter, c.diverter)
	}
	inner, ok := shim.classic.(*tableImpl)
	if !ok {
		t.Fatalf("classic = %T, want *tableImpl", shim.classic)
	}
	if inner.table != "mytable" {
		t.Errorf("classic table = %q, want %q", inner.table, "mytable")
	}
	if inner.authorizedView != "myview" {
		t.Errorf("classic authorizedView = %q, want %q", inner.authorizedView, "myview")
	}
}

// TestOpenMaterializedView_ProducesNilSessionShim — same contract as
// OpenTable, plus materializedView is threaded through to the inner
// Table (and table is empty since MVs are addressed by view name only).
func TestOpenMaterializedView_ProducesNilSessionShim(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0)

	got := c.OpenMaterializedView("myview")
	shim, ok := got.(*TableShim)
	if !ok {
		t.Fatalf("OpenMaterializedView returned %T, want *TableShim", got)
	}
	if shim.session != nil {
		t.Errorf("session = %v, want nil", shim.session)
	}
	if shim.diverter != c.diverter {
		t.Errorf("diverter = %p, want client's diverter %p", shim.diverter, c.diverter)
	}
	inner, ok := shim.classic.(*tableImpl)
	if !ok {
		t.Fatalf("classic = %T, want *tableImpl", shim.classic)
	}
	if inner.materializedView != "myview" {
		t.Errorf("classic materializedView = %q, want %q", inner.materializedView, "myview")
	}
	if inner.table != "" {
		t.Errorf("classic table = %q, want empty (MV addressed by view name only)", inner.table)
	}
}

// TestOpenFactories_ShareOneClientDiverter pins that every Open*
// factory returns a shim referencing the SAME *Diverter — so a future
// SetSessionLoad call from a ConfigurationManager updates every open
// resource on the client at once, no per-resource iteration.
func TestOpenFactories_ShareOneClientDiverter(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0)

	tbl := c.OpenTable("t").(*TableShim)
	av := c.OpenAuthorizedView("t", "v").(*TableShim)
	mv := c.OpenMaterializedView("mv").(*TableShim)

	if tbl.diverter != c.diverter || av.diverter != c.diverter || mv.diverter != c.diverter {
		t.Errorf("diverters diverge: tbl=%p av=%p mv=%p client=%p",
			tbl.diverter, av.diverter, mv.diverter, c.diverter)
	}
	// Flip the client's diverter to prove the reference is live, not
	// a copy.
	c.diverter.SetSessionLoad(0.42)
	if got := tbl.diverter.SessionLoad(); got != 0.42 {
		t.Errorf("after SetSessionLoad(0.42), tbl.diverter.SessionLoad() = %v, want 0.42", got)
	}
}
