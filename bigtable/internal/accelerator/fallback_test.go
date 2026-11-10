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

package accelerator

import (
	"context"
	"sync"
	"testing"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mockTableAPI ------------------------------------------------------------

// mockTableAPI implements bigtable.TableAPI for use in tests. Only ReadRow and
// Apply are wired; the rest panic to catch unexpected calls.
type mockTableAPI struct {
	readRowFn func(ctx context.Context, row string, opts ...bigtable.ReadOption) (bigtable.Row, error)
	applyFn   func(ctx context.Context, row string, m *bigtable.Mutation, opts ...bigtable.ApplyOption) error
}

func (m *mockTableAPI) ReadRow(ctx context.Context, row string, opts ...bigtable.ReadOption) (bigtable.Row, error) {
	if m.readRowFn != nil {
		return m.readRowFn(ctx, row, opts...)
	}
	return nil, nil
}

func (m *mockTableAPI) Apply(ctx context.Context, row string, mut *bigtable.Mutation, opts ...bigtable.ApplyOption) error {
	if m.applyFn != nil {
		return m.applyFn(ctx, row, mut, opts...)
	}
	return nil
}

func (m *mockTableAPI) ReadRows(_ context.Context, _ bigtable.RowSet, _ func(bigtable.Row) bool, _ ...bigtable.ReadOption) error {
	panic("mockTableAPI.ReadRows not implemented")
}
func (m *mockTableAPI) SampleRowKeys(_ context.Context) ([]string, error) {
	panic("mockTableAPI.SampleRowKeys not implemented")
}
func (m *mockTableAPI) ApplyBulk(_ context.Context, _ []string, _ []*bigtable.Mutation, _ ...bigtable.ApplyOption) ([]error, error) {
	panic("mockTableAPI.ApplyBulk not implemented")
}
func (m *mockTableAPI) ApplyReadModifyWrite(_ context.Context, _ string, _ *bigtable.ReadModifyWrite) (bigtable.Row, error) {
	panic("mockTableAPI.ApplyReadModifyWrite not implemented")
}

// --- helpers -----------------------------------------------------------------

// testProject and testInstance are defined in resourcename_test.go.
const testTableName = "projects/p/instances/i/tables/t"

// newClassicFallbackWithMock builds a classicFallback without dialing a real
// Bigtable server by pre-populating the table cache with mocks. The supplied
// mocks map must use full V2 resource names as keys.
func newClassicFallbackWithMock(mocks map[string]bigtable.TableAPI) *classicFallback {
	tables := make(map[string]bigtable.TableAPI, len(mocks))
	for k, v := range mocks {
		tables[k] = v
	}
	return &classicFallback{
		instance:    testInstance,
		scopePrefix: scopePrefixFor(testProject, testInstance),
		mu:          sync.Mutex{},
		tables:      tables,
	}
}

// --- parseTableID ------------------------------------------------------------

func TestParseTableID_HappyPath(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	got, err := cf.parseTableID("projects/p/instances/i/tables/T")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "T" {
		t.Errorf("parseTableID = %q, want %q", got, "T")
	}
}

func TestParseTableID_WrongProject(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseTableID("projects/OTHER/instances/i/tables/T")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseTableID_WrongInstance(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseTableID("projects/p/instances/OTHER/tables/T")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseTableID_EmptyLeaf(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseTableID("projects/p/instances/i/tables/")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseTableID_LeafWithSlash(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseTableID("projects/p/instances/i/tables/T/extra")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

// --- parseAVIDs --------------------------------------------------------------

func TestParseAVIDs_HappyPath(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	tableID, viewID, err := cf.parseAVIDs("projects/p/instances/i/tables/T/authorizedViews/V")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tableID != "T" {
		t.Errorf("tableID = %q, want T", tableID)
	}
	if viewID != "V" {
		t.Errorf("viewID = %q, want V", viewID)
	}
}

func TestParseAVIDs_WrongProject(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, _, err := cf.parseAVIDs("projects/OTHER/instances/i/tables/T/authorizedViews/V")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseAVIDs_MissingSegment(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, _, err := cf.parseAVIDs("projects/p/instances/i/tables/T")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseAVIDs_EmptyViewID(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, _, err := cf.parseAVIDs("projects/p/instances/i/tables/T/authorizedViews/")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

// --- parseMVID ---------------------------------------------------------------

func TestParseMVID_HappyPath(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	got, err := cf.parseMVID("projects/p/instances/i/materializedViews/MV")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "MV" {
		t.Errorf("parseMVID = %q, want MV", got)
	}
}

func TestParseMVID_WrongProject(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseMVID("projects/OTHER/instances/i/materializedViews/MV")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

func TestParseMVID_EmptyLeaf(t *testing.T) {
	cf := &classicFallback{instance: "i", scopePrefix: scopePrefixFor("p", "i")}
	_, err := cf.parseMVID("projects/p/instances/i/materializedViews/")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want codes.InvalidArgument", err)
	}
}

// --- classicFallback.ReadRow -------------------------------------------------

func TestClassicFallbackReadRow_RowKeyPath(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		readRowFn: func(_ context.Context, row string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
			gotKey = row
			return bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("v")}}}, nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte("mykey")}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	resp, err := cf.ReadRow(context.Background(), res, req)
	if err != nil {
		t.Fatalf("ReadRow error: %v", err)
	}
	if gotKey != "mykey" {
		t.Errorf("called ReadRow with key %q, want %q", gotKey, "mykey")
	}
	if len(resp.Chunks) == 0 {
		t.Error("expected non-empty chunks for found row")
	}
}

func TestClassicFallbackReadRow_ClosedClosedRange(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		readRowFn: func(_ context.Context, row string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
			gotKey = row
			return nil, nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("k")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("k")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	if _, err := cf.ReadRow(context.Background(), res, req); err != nil {
		t.Fatalf("ReadRow error: %v", err)
	}
	if gotKey != "k" {
		t.Errorf("called ReadRow with key %q, want %q", gotKey, "k")
	}
}

func TestClassicFallbackReadRow_NonSingleRowRange_ReturnsUnimplemented(t *testing.T) {
	// Pre-populate a mock so tableAPI (called first) succeeds; the shape
	// validation that follows it returns Unimplemented.
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: &mockTableAPI{}})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("z")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

func TestClassicFallbackReadRow_EmptyStartKey_ReturnsUnimplemented(t *testing.T) {
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: &mockTableAPI{}})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{RowRanges: []*v2pb.RowRange{{
			EndKey: &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("k")},
		}}},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

func TestClassicFallbackReadRow_BothKeysAndRanges_ReturnsUnimplemented(t *testing.T) {
	// Both RowKeys and RowRanges set — the XOR check in ReadRow catches this.
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: &mockTableAPI{}})

	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows: &v2pb.RowSet{
			RowKeys: [][]byte{[]byte("k")},
			RowRanges: []*v2pb.RowRange{{
				StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("k")},
				EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("k")},
			}},
		},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

func TestClassicFallbackReadRow_DefaultCase_ReturnsUnimplemented(t *testing.T) {
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: &mockTableAPI{}})

	// No RowKeys and no RowRanges — hits the default case.
	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{},
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	_, err := cf.ReadRow(context.Background(), res, req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want codes.Unimplemented", err)
	}
}

// --- classicFallback.Close nil guard ----------------------------------------

func TestClassicFallbackClose_NilClient(t *testing.T) {
	cf := &classicFallback{}
	if err := cf.Close(); err != nil {
		t.Errorf("Close with nil client: %v", err)
	}
}

// --- classicFallback.MutateRow ----------------------------------------------

func TestClassicFallbackMutateRow_CallsApply(t *testing.T) {
	var gotKey string
	mock := &mockTableAPI{
		applyFn: func(_ context.Context, row string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
			gotKey = row
			return nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})

	req := &v2pb.MutateRowRequest{
		TableName: testTableName,
		RowKey:    []byte("thekey"),
	}
	res := adapters.Resource{Kind: adapters.ResourceTable, Name: testTableName}
	if err := cf.MutateRow(context.Background(), res, req); err != nil {
		t.Fatalf("MutateRow error: %v", err)
	}
	if gotKey != "thekey" {
		t.Errorf("Apply called with key %q, want %q", gotKey, "thekey")
	}
}

type mockClassicClient struct {
	table         bigtable.TableAPI
	openTableHits int
	lastTable     string
	openAVHits    int
	lastAVTable   string
	lastAVView    string
	openMVHits    int
	lastMV        string
	closeCalled   int
}

func (m *mockClassicClient) OpenTable(table string) bigtable.TableAPI {
	m.openTableHits++
	m.lastTable = table
	return m.table
}

func (m *mockClassicClient) OpenAuthorizedView(table, authorizedView string) bigtable.TableAPI {
	m.openAVHits++
	m.lastAVTable = table
	m.lastAVView = authorizedView
	return m.table
}

func (m *mockClassicClient) OpenMaterializedView(materializedView string) bigtable.TableAPI {
	m.openMVHits++
	m.lastMV = materializedView
	return m.table
}

func (m *mockClassicClient) Close() error {
	m.closeCalled++
	return nil
}

func TestClassicFallback_ProjectScoped_MultiInstanceAndAppProfile(t *testing.T) {
	orig := newClassicClient
	var (
		mu      sync.Mutex
		clients = make(map[clientKey]*mockClassicClient)
	)
	newClassicClient = func(
		_ context.Context,
		project, instance string,
		cfg bigtable.ClientConfig,
		_ ...option.ClientOption,
	) (classicClient, error) {
		mu.Lock()
		defer mu.Unlock()
		if project != "p" {
			t.Errorf("newClassicClient project = %q; want p", project)
		}
		mc := &mockClassicClient{table: &mockTableAPI{}}
		clients[clientKey{instance: instance, appProfile: cfg.AppProfile}] = mc
		return mc, nil
	}
	t.Cleanup(func() { newClassicClient = orig })

	cf, err := newClassicFallback(context.Background(), "p", "", "")
	if err != nil {
		t.Fatalf("newClassicFallback: %v", err)
	}

	// Route Table on inst-a, default app profile.
	if err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind: adapters.ResourceTable,
		Name: "projects/p/instances/inst-a/tables/t1",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")}); err != nil {
		t.Fatalf("MutateRow(inst-a, default): %v", err)
	}

	// Route Table on inst-a, prof-1 (distinct cache key and client).
	if err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind:       adapters.ResourceTable,
		Name:       "projects/p/instances/inst-a/tables/t1",
		AppProfile: "prof-1",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")}); err != nil {
		t.Fatalf("MutateRow(inst-a, prof-1): %v", err)
	}

	// Route AuthorizedView on inst-b, prof-2.
	if err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind:       adapters.ResourceAuthorizedView,
		Name:       "projects/p/instances/inst-b/tables/t2/authorizedViews/av1",
		AppProfile: "prof-2",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")}); err != nil {
		t.Fatalf("MutateRow(inst-b, prof-2 AV): %v", err)
	}

	// Route MaterializedView on inst-b, prof-2 (reuses inst-b/prof-2 client).
	if _, err := cf.ReadRow(context.Background(), adapters.Resource{
		Kind:       adapters.ResourceMaterializedView,
		Name:       "projects/p/instances/inst-b/materializedViews/mv1",
		AppProfile: "prof-2",
	}, &v2pb.ReadRowsRequest{Rows: &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}}}); err != nil {
		t.Fatalf("ReadRow(inst-b, prof-2 MV): %v", err)
	}

	if len(clients) != 3 {
		t.Fatalf("expected 3 classic clients; got %d", len(clients))
	}
	if mc := clients[clientKey{instance: "inst-b", appProfile: "prof-2"}]; mc == nil || mc.openAVHits != 1 || mc.openMVHits != 1 {
		t.Errorf("inst-b/prof-2 client stats: %+v", mc)
	}

	// Close cf and verify all clients are closed and subsequent calls return Unavailable.
	if err := cf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for k, mc := range clients {
		if mc.closeCalled != 1 {
			t.Errorf("expected client %+v to be closed on cf.Close; got %d", k, mc.closeCalled)
		}
	}
	err = cf.MutateRow(context.Background(), adapters.Resource{
		Kind: adapters.ResourceTable,
		Name: "projects/p/instances/inst-a/tables/t1",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("MutateRow after Close = %v, want Unavailable", err)
	}
}

func TestClassicFallback_PinnedAppProfileEnforcement(t *testing.T) {
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{
		testTableName + "\x00ap": &mockTableAPI{},
	})
	cf.appProfile = "ap"

	// Empty AppProfile uses pinned "ap".
	if err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind: adapters.ResourceTable,
		Name: testTableName,
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")}); err != nil {
		t.Fatalf("MutateRow with empty app profile: %v", err)
	}

	// Matching AppProfile "ap" succeeds.
	if err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind:       adapters.ResourceTable,
		Name:       testTableName,
		AppProfile: "ap",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")}); err != nil {
		t.Fatalf("MutateRow with matching app profile: %v", err)
	}

	// Mismatched AppProfile returns InvalidArgument.
	err := cf.MutateRow(context.Background(), adapters.Resource{
		Kind:       adapters.ResourceTable,
		Name:       testTableName,
		AppProfile: "wrong-ap",
	}, &v2pb.MutateRowRequest{RowKey: []byte("k")})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("MutateRow with mismatched app profile: got %v, want InvalidArgument", err)
	}
}
