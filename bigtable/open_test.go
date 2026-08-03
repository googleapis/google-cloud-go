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
	"time"

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

// TestOpen_WiresDivertibleWhenDiverterPresent pins Open's session-
// routing contract: when c.diverter is set, the returned *Table has
// divertible populated with a *TableShim, so Apply / ReadRow route
// through the shim. Backward-compatible: the return type stays
// *Table and every non-Apply/ReadRow method is untouched.
func TestOpen_WiresDivertibleWhenDiverterPresent(t *testing.T) {
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
	if tbl.divertible == nil {
		t.Fatal("Open→Table.divertible = nil, want *TableShim (c.diverter is set)")
	}
	shim, ok := tbl.divertible.(*TableShim)
	if !ok {
		t.Fatalf("Open→Table.divertible = %T, want *TableShim", tbl.divertible)
	}
	if shim.diverter != c.diverter {
		t.Errorf("shim.diverter = %p, want client's diverter %p", shim.diverter, c.diverter)
	}
}

// TestOpen_NoDivertibleWhenDiverterAbsent pins the zero-cost classic
// path: with c.diverter nil, Open must return a *Table with
// divertible=nil so the Apply / ReadRow gate short-circuits to the
// *Classic helpers without any shim allocation.
func TestOpen_NoDivertibleWhenDiverterAbsent(t *testing.T) {
	c := &Client{
		project:        "p",
		instance:       "i",
		appProfile:     "ap",
		featureFlagsMD: metadata.MD{},
		// diverter deliberately unset.
	}
	tbl := c.Open("mytable")
	if tbl.divertible != nil {
		t.Errorf("Open→Table.divertible = %v, want nil (no diverter → no shim)", tbl.divertible)
	}
}

// TestOpen_DivertibleShimBreaksRecursionLoop pins the anti-recursion
// invariant that makes Table.Apply / Table.ReadRow gate-then-classic
// pattern safe: the *tableImpl on the shim's classic side wraps a
// SNAPSHOT of the outer Table with divertible EXPLICITLY nil-ed.
//
// Without this break, tableImpl.Apply → Table.Apply would re-enter
// the gate → shim → tableImpl.Apply → ... forever. The tableImpl
// override calling applyClassic/readRowClassic directly is the second
// half of the safety net; this test guards the first half — the
// value-copy + nil field trick.
func TestOpen_DivertibleShimBreaksRecursionLoop(t *testing.T) {
	c := newBareClientForOpenTests(t, 0.0)

	tbl := c.Open("mytable")
	shim, ok := tbl.divertible.(*TableShim)
	if !ok {
		t.Fatalf("divertible = %T, want *TableShim", tbl.divertible)
	}
	inner, ok := shim.classic.(*tableImpl)
	if !ok {
		t.Fatalf("shim.classic = %T, want *tableImpl", shim.classic)
	}
	if inner.table != "mytable" {
		t.Errorf("inner.table = %q, want %q (snapshot must carry the same table id)", inner.table, "mytable")
	}
	if inner.divertible != nil {
		t.Errorf("inner.divertible = %v, want nil (must be cleared to break the recursion loop)", inner.divertible)
	}
}

// TestOpen_ApplyRoutesThroughDivertible drives an end-to-end Apply on
// the *Table returned by Open with a wired session backend and
// SessionLoad=1.0. Proves the session path is reached — i.e., the
// divertible gate fires and the shim dispatches to session, not the
// classic MutateRow (which would panic here because the client has
// no gRPC connection).
func TestOpen_ApplyRoutesThroughDivertible(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)
	c.diverter.SetSessionLoad(1.0)

	tbl := c.Open("mytable")
	if tbl.divertible == nil {
		t.Fatal("divertible = nil, want shim (diverter + sessionImpl wired)")
	}

	mut := NewMutation()
	mut.Set("cf", "q", 1_000_000, []byte("v"))
	if err := tbl.Apply(context.Background(), "row-1", mut); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := len(fsc.openTableCalls); got != 1 || fsc.openTableCalls[0] != "mytable" {
		t.Errorf("fake session OpenTable calls = %v, want [mytable]", fsc.openTableCalls)
	}
}

// TestOpen_ReadRowRoutesThroughDivertible mirrors the Apply test for
// the read side. Same setup, same proof — the divertible gate on
// Table.ReadRow dispatches to the shim's session path.
func TestOpen_ReadRowRoutesThroughDivertible(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)
	c.diverter.SetSessionLoad(1.0)

	tbl := c.Open("mytable")
	if _, err := tbl.ReadRow(context.Background(), "row-1"); err != nil {
		t.Fatalf("ReadRow: %v", err)
	}
	if got := len(fsc.openTableCalls); got != 1 || fsc.openTableCalls[0] != "mytable" {
		t.Errorf("fake session OpenTable calls = %v, want [mytable]", fsc.openTableCalls)
	}
}

// TestTableImpl_BypassesDivertibleGate pins the second half of the
// anti-recursion safety net: even if a tableImpl is somehow given a
// Table with divertible set (defensive — buildDivertible always nil-s
// it), the tableImpl.Apply / tableImpl.ReadRow overrides must call
// the *Classic helpers directly and NOT re-enter the gate.
//
// Uses a spy divertible whose methods panic — if the gate fires, the
// test panics; if the bypass works, the test reaches classic and
// fails predictably (no gRPC conn → panic in unrelated code); we
// catch that with recover to distinguish the two failure modes.
func TestTableImpl_BypassesDivertibleGate(t *testing.T) {
	spy := &panickingTableAPI{}
	ti := &tableImpl{Table: Table{
		c:          newBareClientForOpenTests(t, 0.0),
		table:      "mytable",
		divertible: spy, // if the gate fires, spy panics with a distinctive message
	}}

	// Apply: catch any panic and check it's NOT from the spy.
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(spyPanic); ok {
					t.Errorf("tableImpl.Apply reached the divertible spy — gate must be bypassed")
				}
				// Classic body panicking on missing gRPC conn is expected here.
			}
		}()
		_ = ti.Apply(context.Background(), "row-1", NewMutation())
	}()

	// ReadRow: same shape.
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(spyPanic); ok {
					t.Errorf("tableImpl.ReadRow reached the divertible spy — gate must be bypassed")
				}
			}
		}()
		_, _ = ti.ReadRow(context.Background(), "row-1")
	}()
}

type spyPanic struct{}

type panickingTableAPI struct{}

func (p *panickingTableAPI) ReadRow(context.Context, string, ...ReadOption) (Row, error) {
	panic(spyPanic{})
}
func (p *panickingTableAPI) Apply(context.Context, string, *Mutation, ...ApplyOption) error {
	panic(spyPanic{})
}
func (p *panickingTableAPI) ReadRows(context.Context, RowSet, func(Row) bool, ...ReadOption) error {
	panic(spyPanic{})
}
func (p *panickingTableAPI) SampleRowKeys(context.Context) ([]string, error) { panic(spyPanic{}) }
func (p *panickingTableAPI) ApplyBulk(context.Context, []string, []*Mutation, ...ApplyOption) ([]error, error) {
	panic(spyPanic{})
}
func (p *panickingTableAPI) ApplyReadModifyWrite(context.Context, string, *ReadModifyWrite) (Row, error) {
	panic(spyPanic{})
}

// TestOpen_ClassicOnlyMethodsSkipDivertibleGate pins the invariant
// that Table methods with no session equivalent (ReadRows, ApplyBulk,
// SampleRowKeys, ApplyReadModifyWrite) MUST NOT gate on t.divertible.
// They run the classic body directly — TableShim's own delegation
// would just be pure indirection since the shim also always delegates
// them to classic (see TableShim.ReadRows / ApplyBulk / etc).
//
// Fixture matches a realistic session-wired client (session backend +
// diverter with SessionLoad=1.0), then swaps divertible for the
// panickingTableAPI spy so we can distinguish gate-dispatch (spy
// panic) from classic-body execution (nil-conn panic in the classic
// body). Under the natural configuration where a stray gate would
// actually route to session, this is where the accident would show.
func TestOpen_ClassicOnlyMethodsSkipDivertibleGate(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)
	c.diverter.SetSessionLoad(1.0)

	newTable := func() *Table {
		return &Table{
			c:          c,
			table:      "mytable",
			divertible: &panickingTableAPI{},
		}
	}

	assertNotSpyPanic := func(t *testing.T, method string) {
		t.Helper()
		if r := recover(); r != nil {
			if _, ok := r.(spyPanic); ok {
				t.Errorf("Table.%s reached divertible spy — this method must NOT gate on divertible", method)
			}
			// Any other panic (nil-conn from classic body) is expected.
		}
	}

	t.Run("ReadRows", func(t *testing.T) {
		defer assertNotSpyPanic(t, "ReadRows")
		_ = newTable().ReadRows(context.Background(), InfiniteRange(""), func(Row) bool { return true })
	})
	t.Run("ApplyBulk", func(t *testing.T) {
		defer assertNotSpyPanic(t, "ApplyBulk")
		_, _ = newTable().ApplyBulk(context.Background(), []string{"r"}, []*Mutation{NewMutation()})
	})
	t.Run("SampleRowKeys", func(t *testing.T) {
		defer assertNotSpyPanic(t, "SampleRowKeys")
		_, _ = newTable().SampleRowKeys(context.Background())
	})
	t.Run("ApplyReadModifyWrite", func(t *testing.T) {
		defer assertNotSpyPanic(t, "ApplyReadModifyWrite")
		_, _ = newTable().ApplyReadModifyWrite(context.Background(), "r", &ReadModifyWrite{})
	})
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

// ─── Session-backend-wired tests ──────────────────────────────────────
//
// Every NewClientWithConfig-produced Client now carries a real
// session.Client (it's always constructed), so the tests below
// substitute a lightweight fakeSessionClient to keep them dial-free
// and deterministic. The nil-sessionImpl case (below in
// TestGetOrCreateSession_NilSessionImplReturnsNil) is still a valid
// programmatic construction — internal helpers guard for it so
// hand-built or emulator-only Clients don't panic — even though the
// public factory path always wires one.

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

func (f *fakeSessionClient) MeterProvider() metric.MeterProvider           { return nil }
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
	c := &Client{
		project:        "p",
		instance:       "i",
		appProfile:     "ap",
		featureFlagsMD: metadata.MD{},
		diverter:       btransport.NewDiverter(0.0),
		sessionImpl:    fsc,
	}
	// Cache with a huge TTL and huge sweep interval so eviction never
	// fires during these tests. Cache-behavior tests build their own
	// cache with tight timings.
	c.sessionTables = newSessionTableCache(24*time.Hour, 24*time.Hour, nil)
	t.Cleanup(c.sessionTables.close)
	return c
}

// unwrap peels the sessionTableHandle wrapper off a shim's session so
// tests can inspect the underlying fakeSessionClient-vended
// noopSessionTable directly.
func unwrapSession(t *testing.T, api session.TableAPI) *noopSessionTable {
	t.Helper()
	h, ok := api.(*sessionTableHandle)
	if !ok {
		t.Fatalf("session = %T, want *sessionTableHandle (cache wrapper)", api)
	}
	nst, ok := h.api.(*noopSessionTable)
	if !ok {
		t.Fatalf("handle.api = %T, want *noopSessionTable (from fakeSessionClient)", h.api)
	}
	return nst
}

// TestOpenTable_WithSessionBackend_WiresSessionTableAPI pins that
// OpenTable's returned shim carries a non-nil session TableAPI produced
// by sessionImpl.OpenTable(table) — the default state now that
// NewClientWithConfig always wires a session.Client.
func TestOpenTable_WithSessionBackend_WiresSessionTableAPI(t *testing.T) {
	fsc := newFakeSessionClient()
	c := newSessionWiredClient(t, fsc)

	shim := c.OpenTable("my-table").(*TableShim)
	if shim.session == nil {
		t.Fatal("OpenTable→TableShim.session = nil, want non-nil (session backend is wired)")
	}
	nst := unwrapSession(t, shim.session)
	// noopSessionTable.key is fakeSessionClient's internal identity
	// tag (prefixed inside fakeSessionClient.handle to disambiguate
	// its own bookkeeping) — unrelated to what the cache uses as its
	// key, which is now the fully-qualified resource name from
	// c.fullTableName.
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
// sessionImpl == nil, every getOrCreateSession* short-circuits to nil
// BEFORE touching the map. NewClientWithConfig always sets sessionImpl,
// but a hand-built Client (used here and in tests / emulator setups)
// can leave it nil — the guard exists so those paths don't panic.
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
