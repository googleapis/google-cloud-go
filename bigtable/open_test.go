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
	"sync/atomic"
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

// ─── sessionTableCache tests ──────────────────────────────────────────

// newTestSessionTableCache builds a cache with an injectable clock.
// Sweep interval is intentionally long — tests drive sweepOnce
// directly instead of waiting on the background ticker, which is
// wall-clock and flaky under CI scheduler pressure (#20266). TTL is
// set per test.
func newTestSessionTableCache(t *testing.T, ttl time.Duration, clock *fakeClock) *sessionTableCache {
	t.Helper()
	c := newSessionTableCache(ttl, 1*time.Hour, clock.now)
	t.Cleanup(c.close)
	return c
}

// openNoop returns an openFn that constructs a *noopSessionTable
// stamped with the given key. Used by cache-internal tests that
// want to inspect which key the cache asked to open.
func openNoop(key string) func() session.TableAPI {
	return func() session.TableAPI { return &noopSessionTable{key: key} }
}

// openClosing returns an openFn that constructs a
// *closeCountingTable stamped with the given key, atomically
// incrementing counter on every Close call. Counter is atomic so
// sweeper-goroutine writes don't race the test's Load.
func openClosing(key string, counter *atomic.Int32) func() session.TableAPI {
	return func() session.TableAPI {
		return &closeCountingTable{noopSessionTable{key: key}, counter}
	}
}

// fakeClock is a monotonic clock that only advances on explicit
// advance() calls. Concurrency: single writer via advance(), many
// readers via now() — protected by an atomic.
type fakeClock struct{ nano atomic.Int64 }

func newFakeClock(start time.Time) *fakeClock {
	c := &fakeClock{}
	c.nano.Store(start.UnixNano())
	return c
}
func (c *fakeClock) now() time.Time          { return time.Unix(0, c.nano.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.nano.Add(int64(d)) }

// TestSessionTableCache_HandleIsCacheEntry pins that the returned
// handle satisfies session.TableAPI, wraps the underlying api, and
// is the same value the cache holds (identity check on repeat Open).
func TestSessionTableCache_HandleIsCacheEntry(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h1 := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	h2 := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	if h1 != h2 {
		t.Errorf("repeat getOrOpen on same key = distinct handles: h1=%p h2=%p", h1, h2)
	}
	if _, ok := h1.api.(*noopSessionTable); !ok {
		t.Errorf("handle.api = %T, want *noopSessionTable", h1.api)
	}
}

// TestSessionTableCache_ReadRowTouchesLastAccess pins that the
// wrapper's ReadRow updates lastAccess so a caller polling every
// ReadRow keeps the entry alive.
func TestSessionTableCache_ReadRowTouchesLastAccess(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	before := h.lastAccessNano.Load()

	clock.advance(30 * time.Minute)
	_, _ = h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})

	after := h.lastAccessNano.Load()
	if after <= before {
		t.Errorf("ReadRow did not bump lastAccess: before=%d after=%d", before, after)
	}
}

// TestSessionTableCache_CloseEvictsAndFires pins that handle.Close
// removes the entry from the cache map AND calls the underlying
// api.Close, and that a subsequent getOrOpen mints a fresh handle
// (the closed one is not resurrected).
func TestSessionTableCache_CloseEvictsAndFires(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Second, clock.now)
	t.Cleanup(c.close)

	h1 := c.getOrOpen("tbl:t", openClosing("tbl:t", &closeCount)).(*sessionTableHandle)
	if err := h1.Close(); err != nil {
		t.Fatalf("h1.Close: %v", err)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying Close called %d times, want 1", got)
	}
	// Map should no longer contain the key.
	c.mu.Lock()
	_, still := c.entries["tbl:t"]
	c.mu.Unlock()
	if still {
		t.Error("entry still present after handle.Close()")
	}
	// Second Open mints a fresh handle.
	h2 := c.getOrOpen("tbl:t", openClosing("tbl:t", &closeCount)).(*sessionTableHandle)
	if h2 == h1 {
		t.Error("getOrOpen after Close returned the evicted handle")
	}
	// Double-Close on h1 is fully idempotent — closeOnce guards both
	// the map removal AND the underlying api.Close call, so the
	// counter stays at 1 even after a second h1.Close().
	if err := h1.Close(); err != nil {
		t.Errorf("h1.Close (idempotent) err = %v, want nil", err)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying Close called %d times after double-Close, want 1 (Close is fully idempotent)", got)
	}
}

// TestSessionTableCache_TTLSweepEvictsIdle pins that a sweep evicts
// entries whose lastAccess is older than TTL, and calls the
// underlying Close on eviction.
//
// Drives sweepOnce directly instead of polling on the background
// ticker: the ticker fires at a real-wall-clock cadence and, under CI
// scheduler pressure, may not run within the assertion's deadline
// window even at a 1ms interval — see #20266 for the flake pattern.
// Same-package access to sweepOnce lets us exercise the sweep logic
// deterministically without any wall-clock dependency.
func TestSessionTableCache_TTLSweepEvictsIdle(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	// Use a long sweepInterval so the background ticker never races the
	// direct sweepOnce call below — the test asserts sweep behavior,
	// not scheduler timing.
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	// Open two handles, touch neither.
	_ = c.getOrOpen("tbl:a", openClosing("tbl:a", &closeCount))
	_ = c.getOrOpen("tbl:b", openClosing("tbl:b", &closeCount))

	// Advance past TTL and drive a sweep synchronously.
	clock.advance(2 * time.Hour)
	c.sweepOnce()

	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("entries remaining after TTL sweep = %d, want 0", n)
	}
	if got := closeCount.Load(); got != 2 {
		t.Errorf("underlying Close called %d times on TTL evict, want 2", got)
	}
}

// TestSessionTableCache_TouchDefersEviction pins that a ReadRow
// touch resets the idle timer — an entry touched every half-TTL
// stays alive indefinitely.
func TestSessionTableCache_TouchDefersEviction(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	// Every half-TTL, touch and step past a full TTL from the LAST
	// touch. Each touch resets the clock reference so eviction never
	// triggers.
	for i := 0; i < 4; i++ {
		clock.advance(30 * time.Minute)
		_, _ = h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})
	}
	// Drive a sweep directly; touch-driven lastAccess should keep the
	// entry alive despite the clock advance.
	c.sweepOnce()
	c.mu.Lock()
	_, present := c.entries["tbl:t"]
	c.mu.Unlock()
	if !present {
		t.Error("touched entry got evicted; touch is not deferring eviction")
	}
}

// TestSessionTableCache_CloseEvictsAll pins that closing the cache
// itself stops the sweeper and closes every remaining entry.
func TestSessionTableCache_CloseEvictsAll(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	_ = c.getOrOpen("tbl:a", openClosing("tbl:a", &closeCount))
	_ = c.getOrOpen("tbl:b", openClosing("tbl:b", &closeCount))
	_ = c.getOrOpen("mv:v", openClosing("mv:v", &closeCount))

	c.close()

	if got := closeCount.Load(); got != 3 {
		t.Errorf("close(): underlying Close called %d times, want 3", got)
	}
	// close() is idempotent.
	c.close()
}

// TestSessionTableCache_ClosedGate_SlowPathInsertNoLeak reproduces
// audit finding #5: without the closed-gate in getOrOpen's slow path,
// a caller whose openFn straddles cache.close() would leak the
// freshly-opened api (installed into a cache the sweeper has already
// stopped clearing). This test forces that interleaving via a
// synchronization channel on the openFn.
//
// Shape: caller A begins getOrOpen("k") on an empty cache. Fast-path
// misses. Slow-path calls openFn. openFn blocks until we signal it to
// return. Meanwhile close() runs on the cache (flips closed, walks the
// empty map). Then openFn returns. getOrOpen re-locks, sees closed,
// releases the fresh api itself, and returns nil. Underlying api's
// Close counter must reach 1.
func TestSessionTableCache_ClosedGate_SlowPathInsertNoLeak(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	release := make(chan struct{})
	opened := make(chan struct{})
	openFn := func() session.TableAPI {
		close(opened)
		<-release
		return &closeCountingTable{noopSessionTable{key: "k"}, &closeCount}
	}

	var result session.TableAPI
	done := make(chan struct{})
	go func() {
		result = c.getOrOpen("k", openFn)
		close(done)
	}()

	// Wait until openFn is in-flight, THEN close the cache. This is the
	// race window: openFn returns AFTER close() completed.
	<-opened
	c.close()
	close(release)
	<-done

	if result != nil {
		t.Errorf("getOrOpen returned non-nil after cache close: %v (want nil so TableShim falls back to classic)", result)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying api Close called %d times, want 1 (slow-path insert must release its api when cache is closed)", got)
	}
	// Cache map must be empty — the fresh api MUST NOT have been
	// installed into an already-closed cache.
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("cache.entries has %d entries after close-race; want 0 (fresh handle must not be installed)", n)
	}
}

// closeCountingTable is a noopSessionTable that atomically
// increments a counter on Close so tests can assert eviction actually
// called Close from any goroutine (including the cache sweeper).
type closeCountingTable struct {
	noopSessionTable
	counter *atomic.Int32
}

func (c *closeCountingTable) Close() error {
	c.counter.Add(1)
	return nil
}

// TestSessionTableHandle_EvictedSelfHeals pins the Design C invariant
// that makes bug #6 fixable without TableShim changes: a
// *sessionTableHandle whose Close() has run (sweeper eviction, or
// direct handle.Close from any owner) transparently re-opens via the
// cache on the next RPC. The caller — TableShim or any other holder —
// keeps its *sessionTableHandle pointer forever; the wrapper does the
// self-heal.
//
// Verifies:
//   - After handle.Close(), evicted is true.
//   - Subsequent ReadRow / MutateRow succeed by delegating to a
//     freshly-minted successor handle.
//   - openFn IS invoked a second time (fresh sessionTable is minted).
//   - Handle identity is stable — the same *sessionTableHandle
//     transparently dispatches to whichever underlying api is live.
func TestSessionTableHandle_EvictedSelfHeals(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	var opens atomic.Int32
	openFn := func() session.TableAPI {
		opens.Add(1)
		return &noopSessionTable{key: "tbl:t"}
	}

	// Install h1 via first getOrOpen.
	h1 := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)
	if opens.Load() != 1 {
		t.Fatalf("openFn calls after first install = %d, want 1", opens.Load())
	}

	// Sweeper-equivalent: evict h1 by calling its Close directly.
	if err := h1.Close(); err != nil {
		t.Fatalf("h1.Close: %v", err)
	}
	if !h1.evicted.Load() {
		t.Fatal("h1.evicted = false after Close(); want true (self-heal fast-path check would miss)")
	}

	// Next ReadRow on the (evicted) h1 must transparently succeed via
	// a fresh handle. openFn should fire a second time; a new handle
	// should now be installed in the cache.
	if _, err := h1.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")}); err != nil {
		t.Fatalf("post-evict h1.ReadRow: %v (self-heal failed)", err)
	}
	if got := opens.Load(); got != 2 {
		t.Errorf("openFn calls after post-evict ReadRow = %d, want 2 (self-heal must re-invoke openFn)", got)
	}
	c.mu.Lock()
	installed, ok := c.entries["tbl:t"]
	c.mu.Unlock()
	if !ok {
		t.Fatal("cache.entries missing 'tbl:t' after self-heal; getOrOpen must have installed the successor")
	}
	if installed == h1 {
		t.Error("cache.entries['tbl:t'] == h1 (evicted); self-heal must have installed a distinct successor")
	}

	// MutateRow must also self-heal via the same successor (or an
	// equivalent one if the cache TTL evicted between calls; not
	// possible here with 1h TTL + injected clock).
	if _, err := h1.MutateRow(context.Background(), &btpb.SessionMutateRowRequest{Key: []byte("r")}); err != nil {
		t.Fatalf("post-evict h1.MutateRow: %v", err)
	}
	if got := opens.Load(); got != 2 {
		t.Errorf("openFn calls after MutateRow = %d, want 2 (successor should be reused on the same key)", got)
	}
}

// TestSessionTableHandle_EvictedReopenFailsGracefully pins the terminal
// branch: when openFn returns nil (cache has been close()d or the
// session backend is unavailable), the evicted handle's ReadRow /
// MutateRow surface whatever error the dead api produces rather than
// looping forever. reopenAfterEviction returns nil; the handle falls
// through to h.api.ReadRow, which the fake here signals by returning
// an error from the deadApi.
func TestSessionTableHandle_EvictedReopenFailsGracefully(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	openFn := func() session.TableAPI { return &noopSessionTable{key: "tbl:t"} }
	h := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)

	// Close the cache itself so getOrOpen returns nil on any subsequent
	// call — the evicted-handle self-heal path can't recover.
	c.close()

	// h.evicted should now be true (Close cascaded via cache.close).
	if !h.evicted.Load() {
		t.Fatal("cache.close() did not propagate to h.evicted; expected true")
	}

	// The fake noopSessionTable returns success even after Close, so we
	// still get a valid response — this test's real assertion is that
	// we don't panic or loop when reopen refuses.
	_, err := h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})
	if err != nil {
		t.Logf("post-cache-close ReadRow returned err = %v (expected for a dead pool in prod; noopSessionTable is lenient)", err)
	}
	// Assert the call actually returned rather than looped.
}
