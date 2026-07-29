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

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
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

// ─── Session-backend-wired tests (EnableSessionPool=true) ─────────────

// fakeSessionClient is a minimal session.Client used by the tests
// below. Records which Open* helpers were called and returns a shared
// noopSessionTable so identity comparisons prove the cache works.
type fakeSessionClient struct {
	openTableCalls  []string
	openAVCalls     [][2]string
	openMVCalls     []string
	closeCalls      int
	loadListenerSet bool

	// One stub per resource-key so distinct resources get distinct
	// handles but repeat opens of the same resource return the same
	// pointer (cache-hit assertion in the tests).
	handles map[string]*noopSessionTable
}

func newFakeSessionClient() *fakeSessionClient {
	return &fakeSessionClient{handles: map[string]*noopSessionTable{}}
}

func (f *fakeSessionClient) handle(key string) session.TableAPI {
	if h, ok := f.handles[key]; ok {
		return h
	}
	h := &noopSessionTable{key: key}
	f.handles[key] = h
	return h
}

func (f *fakeSessionClient) OpenTable(t string) session.TableAPI {
	f.openTableCalls = append(f.openTableCalls, t)
	return f.handle("tbl:" + t)
}

func (f *fakeSessionClient) OpenAuthorizedView(t, v string) session.TableAPI {
	f.openAVCalls = append(f.openAVCalls, [2]string{t, v})
	return f.handle("av:" + t + ":" + v)
}

func (f *fakeSessionClient) OpenMaterializedView(v string) session.TableAPI {
	f.openMVCalls = append(f.openMVCalls, v)
	return f.handle("mv:" + v)
}

func (f *fakeSessionClient) MeterProvider() metric.MeterProvider          { return nil }
func (f *fakeSessionClient) SessionDebug() btransport.SessionDebugProvider { return nil }
func (f *fakeSessionClient) ChannelDebug() btransport.ChannelDebugProvider { return nil }
func (f *fakeSessionClient) ConfigDebug() btransport.ConfigDebugProvider   { return nil }
func (f *fakeSessionClient) AddSessionLoadListener(_ func(load float64)) func() {
	f.loadListenerSet = true
	return func() {}
}
func (f *fakeSessionClient) Close() error { f.closeCalls++; return nil }

// noopSessionTable is the session.TableAPI vended by fakeSessionClient.
// Both RPC methods are no-ops; the tests below only inspect identity
// (pointer equality) so cache-hit vs. cache-miss can be distinguished.
type noopSessionTable struct{ key string }

func (n *noopSessionTable) ReadRow(context.Context, *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
	return &btpb.SessionReadRowResponse{}, nil
}
func (n *noopSessionTable) MutateRow(context.Context, *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
	return &btpb.SessionMutateRowResponse{}, nil
}
func (n *noopSessionTable) Close() error { return nil }

func newSessionWiredClient(t *testing.T, fsc *fakeSessionClient) *Client {
	t.Helper()
	return &Client{
		project:        "p",
		instance:       "i",
		appProfile:     "ap",
		featureFlagsMD: metadata.MD{},
		diverter:       btransport.NewDiverter(0.0),
		sessionImpl:    fsc,
		sessionTables:  map[string]session.TableAPI{},
	}
}

// TestOpenTable_WithSessionBackend_WiresSessionTableAPI pins that when
// EnableSessionPool is on, OpenTable's returned shim carries a non-nil
// session TableAPI produced by sessionImpl.OpenTable(table).
func TestOpenTable_WithSessionBackend_WiresSessionTableAPI(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)

	shim := c.OpenTable("my-table").(*TableShim)
	if shim.session == nil {
		t.Fatal("OpenTable→TableShim.session = nil, want non-nil (session backend is wired)")
	}
	nst, ok := shim.session.(*noopSessionTable)
	if !ok {
		t.Fatalf("session = %T, want *noopSessionTable (from fakeSessionClient)", shim.session)
	}
	if nst.key != "tbl:my-table" {
		t.Errorf("session key = %q, want %q", nst.key, "tbl:my-table")
	}
	if len(fsc.openTableCalls) != 1 || fsc.openTableCalls[0] != "my-table" {
		t.Errorf("sessionImpl.OpenTable calls = %v, want [my-table]", fsc.openTableCalls)
	}
}

// TestOpenTable_SessionTableAPICacheHit pins that a second OpenTable on
// the same table reuses the cached session TableAPI (identity check on
// the pointer) — the cache prevents leaking a fresh pair of read/write
// session pools per Open call.
func TestOpenTable_SessionTableAPICacheHit(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)

	s1 := c.OpenTable("t").(*TableShim).session
	s2 := c.OpenTable("t").(*TableShim).session
	if s1 != s2 {
		t.Errorf("cache miss on second OpenTable: s1=%p s2=%p (want identity)", s1, s2)
	}
	if len(fsc.openTableCalls) != 1 {
		t.Errorf("sessionImpl.OpenTable called %d times, want 1 (cache miss)", len(fsc.openTableCalls))
	}
}

// TestOpenAuthorizedView_SessionCacheIsTableQualified pins that two
// authorized views with the same view-id under DIFFERENT tables get
// DISTINCT session TableAPI instances — the cache key must be
// "av:<table>:<view>", not "av:<view>", so per-resource session pools
// and per-pool metric labels stay disjoint.
func TestOpenAuthorizedView_SessionCacheIsTableQualified(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)

	sA := c.OpenAuthorizedView("tableA", "sharedView").(*TableShim).session
	sB := c.OpenAuthorizedView("tableB", "sharedView").(*TableShim).session
	if sA == sB {
		t.Errorf("two AVs with the same view-id under different tables share a session TableAPI (identity match) — cache key must be table-qualified")
	}
	if len(fsc.openAVCalls) != 2 {
		t.Errorf("sessionImpl.OpenAuthorizedView calls = %d, want 2 (once per (table, view))", len(fsc.openAVCalls))
	}
}

// TestOpenMaterializedView_WithSessionBackend_WiresSessionTableAPI is
// the parallel to TestOpenTable_WithSessionBackend for MVs.
func TestOpenMaterializedView_WithSessionBackend_WiresSessionTableAPI(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)

	shim := c.OpenMaterializedView("mv").(*TableShim)
	if shim.session == nil {
		t.Fatal("session = nil, want non-nil")
	}
	if len(fsc.openMVCalls) != 1 || fsc.openMVCalls[0] != "mv" {
		t.Errorf("sessionImpl.OpenMaterializedView calls = %v, want [mv]", fsc.openMVCalls)
	}
	// Cache-hit check: second Open reuses the same session TableAPI.
	if s2 := c.OpenMaterializedView("mv").(*TableShim).session; s2 != shim.session {
		t.Errorf("cache miss on repeat OpenMaterializedView")
	}
	if len(fsc.openMVCalls) != 1 {
		t.Errorf("sessionImpl.OpenMaterializedView called %d times, want 1 (repeat call must hit cache)", len(fsc.openMVCalls))
	}
}

// TestGetOrCreateSession_NilSessionImplReturnsNil pins that with
// sessionImpl == nil (EnableSessionPool=false), every getOrCreateSession*
// short-circuits to nil BEFORE touching the map — so the classic-only
// path pays zero lock cost per Open call.
func TestGetOrCreateSession_NilSessionImplReturnsNil(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0) // sessionImpl unset

	if got := c.getOrCreateSessionTable("t"); got != nil {
		t.Errorf("getOrCreateSessionTable(nil sessionImpl) = %v, want nil", got)
	}
	if got := c.getOrCreateSessionAuthorizedView("t", "v"); got != nil {
		t.Errorf("getOrCreateSessionAuthorizedView(nil sessionImpl) = %v, want nil", got)
	}
	if got := c.getOrCreateSessionMaterializedView("v"); got != nil {
		t.Errorf("getOrCreateSessionMaterializedView(nil sessionImpl) = %v, want nil", got)
	}
}
