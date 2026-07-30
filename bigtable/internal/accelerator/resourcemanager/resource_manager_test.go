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

package resourcemanager

import (
	"context"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
)

// stubTable is a no-op session.TableAPI; ResourceManager returns it verbatim,
// so its behaviour is irrelevant — only the args it was opened with matter.
type stubTable struct{}

func (stubTable) ReadRow(context.Context, *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
	return nil, nil
}
func (stubTable) MutateRow(context.Context, *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
	return nil, nil
}
func (stubTable) Close() error { return nil }

// recordingClient records the leaf arguments the ResourceManager passes to each
// Open* method so tests can assert the full-resource-name → leaf parsing.
type recordingClient struct {
	table session.TableAPI

	openTableArg string
	avTable      string
	avView       string
	mvView       string
}

func (c *recordingClient) OpenTable(tableID string) session.TableAPI {
	c.openTableArg = tableID
	return c.table
}
func (c *recordingClient) OpenAuthorizedView(table, view string) session.TableAPI {
	c.avTable, c.avView = table, view
	return c.table
}
func (c *recordingClient) OpenMaterializedView(view string) session.TableAPI {
	c.mvView = view
	return c.table
}
func (c *recordingClient) MeterProvider() metric.MeterProvider           { return nil }
func (c *recordingClient) SessionDebug() btransport.SessionDebugProvider { return nil }
func (c *recordingClient) ChannelDebug() btransport.ChannelDebugProvider { return nil }
func (c *recordingClient) ConfigDebug() btransport.ConfigDebugProvider   { return nil }
func (c *recordingClient) AddSessionLoadListener(func(float64)) func()   { return func() {} }
func (c *recordingClient) Close() error                                  { return nil }

func newTestManager(t *testing.T) (*ResourceManager, *recordingClient) {
	t.Helper()
	rc := &recordingClient{table: stubTable{}}
	restore := TestHookSessionClient(func(context.Context, string, string, string, ...option.ClientOption) (session.Client, error) {
		return rc, nil
	})
	t.Cleanup(restore)

	rm, err := New(context.Background(), "p", "i", "ap")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { rm.Close() })
	return rm, rc
}

func TestGetSessionTable_ExtractsLeaf(t *testing.T) {
	rm, rc := newTestManager(t)

	tbl, release, err := rm.GetSessionTable("projects/p/instances/i/tables/t", "ReadRow")
	if err != nil {
		t.Fatalf("GetSessionTable: %v", err)
	}
	if tbl == nil || release == nil {
		t.Fatal("GetSessionTable returned nil handle or release")
	}
	if rc.openTableArg != "t" {
		t.Errorf("OpenTable leaf = %q; want %q", rc.openTableArg, "t")
	}
}

func TestGetSessionAuthorizedView_ExtractsLeaves(t *testing.T) {
	rm, rc := newTestManager(t)

	if _, _, err := rm.GetSessionAuthorizedView("projects/p/instances/i/tables/t/authorizedViews/v", "ReadRow"); err != nil {
		t.Fatalf("GetSessionAuthorizedView: %v", err)
	}
	if rc.avTable != "t" || rc.avView != "v" {
		t.Errorf("OpenAuthorizedView(table, view) = (%q, %q); want (%q, %q)", rc.avTable, rc.avView, "t", "v")
	}
}

func TestGetSessionMaterializedView_ExtractsLeaf(t *testing.T) {
	rm, rc := newTestManager(t)

	if _, _, err := rm.GetSessionMaterializedView("projects/p/instances/i/materializedViews/mv", "ReadRow"); err != nil {
		t.Fatalf("GetSessionMaterializedView: %v", err)
	}
	if rc.mvView != "mv" {
		t.Errorf("OpenMaterializedView view = %q; want %q", rc.mvView, "mv")
	}
}

func TestAuthorizedViewLeaves_NoMarker(t *testing.T) {
	// Best-effort fallback: a bare name with no "/authorizedViews/" marker is
	// treated wholly as the view leaf.
	table, view := authorizedViewLeaves("just-a-view")
	if table != "" || view != "just-a-view" {
		t.Errorf("authorizedViewLeaves = (%q, %q); want (%q, %q)", table, view, "", "just-a-view")
	}
}
