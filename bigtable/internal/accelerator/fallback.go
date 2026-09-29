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
	"strings"
	"sync"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// classicFallback wraps a bigtable.Client for use as a fallback when the
// session path returns codes.Unimplemented. It operates at the proto level
// (accepting V2 request protos directly) but delegates execution to
// bigtable.Client so retry logic, backoff, and RetryInfo header handling
// stay in sync with the rest of the Go client without duplication.
//
// Resource handles are cached by full V2 resource name, so repeated RPCs to
// the same table / authorized view / materialized view share one handle.
type classicFallback struct {
	c           *bigtable.Client
	scopePrefix string // "projects/<project>/instances/<instance>/"
	mu          sync.Mutex
	tables      map[string]bigtable.TableAPI // keyed by full V2 resource name
}

// newClassicFallback constructs a classic bigtable.Client scoped to
// (project, instance, appProfile). DisableSession is set so this client never
// spins up session infrastructure — it is a pure classic-path fallback.
func newClassicFallback(ctx context.Context, project, instance, appProfile string, opts ...option.ClientOption) (*classicFallback, error) {
	c, err := bigtable.NewClientWithConfig(ctx, project, instance, bigtable.ClientConfig{
		AppProfile:     appProfile,
		DisableSession: true,
	}, opts...)
	if err != nil {
		return nil, err
	}
	return &classicFallback{
		c:           c,
		scopePrefix: scopePrefixFor(project, instance),
		tables:      make(map[string]bigtable.TableAPI),
	}, nil
}

// tableAPI returns a cached bigtable.TableAPI for the given resource.
// The cache key is the full V2 resource name so table, authorized-view, and
// materialized-view keys never collide.
//
// Table construction (Open*) is done outside the lock since it is pure
// in-memory struct initialisation with no I/O. The lock only guards the
// cache map. If two goroutines race on the same name the second gets the
// already-stored handle; both handles are equivalent.
func (f *classicFallback) tableAPI(res adapters.Resource) (bigtable.TableAPI, error) {
	f.mu.Lock()
	if t, ok := f.tables[res.Name]; ok {
		f.mu.Unlock()
		return t, nil
	}
	f.mu.Unlock()

	// Build the handle outside the lock.
	t, err := f.buildTableHandle(res)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	// Double-check: another goroutine may have raced us and stored a handle.
	if existing, ok := f.tables[res.Name]; ok {
		f.mu.Unlock()
		return existing, nil
	}
	f.tables[res.Name] = t
	f.mu.Unlock()
	return t, nil
}

// buildTableHandle constructs a bigtable.TableAPI for res. It validates that
// the resource belongs to f.project/f.instance before extracting the leaf IDs,
// so cross-project/cross-instance routing is caught as an internal error.
func (f *classicFallback) buildTableHandle(res adapters.Resource) (bigtable.TableAPI, error) {
	switch res.Kind {
	case adapters.ResourceTable:
		tableID, err := f.parseTableID(res.Name)
		if err != nil {
			return nil, err
		}
		return f.c.OpenTable(tableID), nil
	case adapters.ResourceAuthorizedView:
		tableID, viewID, err := f.parseAVIDs(res.Name)
		if err != nil {
			return nil, err
		}
		return f.c.OpenAuthorizedView(tableID, viewID), nil
	case adapters.ResourceMaterializedView:
		mvID, err := f.parseMVID(res.Name)
		if err != nil {
			return nil, err
		}
		return f.c.OpenMaterializedView(mvID), nil
	default:
		return nil, status.Errorf(codes.Internal, "accelerator fallback: unknown resource kind %v", res.Kind)
	}
}

// ReadRow executes a single-row read via the classic bigtable.Client and
// returns a ReadRowsResponse (possibly with no chunks for a missing row).
// req must have already passed validateSingleRowReadRequest.
func (f *classicFallback) ReadRow(ctx context.Context, res adapters.Resource, req *v2pb.ReadRowsRequest) (*v2pb.ReadRowsResponse, error) {
	// Fetch the table handle first to fail fast on resource mismatches (wrong
	// project/instance, unknown resource kind) before doing key extraction.
	tbl, err := f.tableAPI(res)
	if err != nil {
		return nil, err
	}

	// Extract and validate the row key. validateSingleRowReadRequest (called by
	// the session stream's SendMsg) should have ensured exactly one key or a
	// closed-closed single-row range with no overlap. We re-validate explicitly
	// here as a defensive guard: the cases are mutually exclusive (RowKeys XOR
	// RowRanges) to catch any request where both fields are set.
	var key string
	nKeys := len(req.Rows.GetRowKeys())
	nRanges := len(req.Rows.GetRowRanges())
	switch {
	case nKeys == 1 && nRanges == 0:
		key = string(req.Rows.RowKeys[0])
	case nKeys == 0 && nRanges == 1:
		r := req.Rows.RowRanges[0]
		start := r.GetStartKeyClosed()
		if len(start) == 0 || string(start) != string(r.GetEndKeyClosed()) {
			return nil, status.Errorf(codes.Unimplemented,
				"accelerator fallback: unsupported row range shape")
		}
		key = string(start)
	default:
		// Unimplemented, not Internal: shape-based rejections must use
		// Unimplemented so the caller's per-method native fallback breaker fires.
		// This branch is unreachable when SendMsg called validateSingleRowReadRequest
		// first, but is kept as a defensive guard.
		return nil, status.Errorf(codes.Unimplemented,
			"accelerator fallback: unsupported row set shape (keys=%d, ranges=%d)",
			nKeys, nRanges)
	}

	var opts []bigtable.ReadOption
	if req.Filter != nil {
		opts = append(opts, bigtable.RowFilter(bigtable.InternalRawProtoFilter(req.Filter)))
	}

	row, err := tbl.ReadRow(ctx, key, opts...)
	if err != nil {
		return nil, err
	}
	return rowToReadRowsResponse(row, []byte(key)), nil
}

// MutateRow executes a MutateRow via the classic bigtable.Client.
func (f *classicFallback) MutateRow(ctx context.Context, res adapters.Resource, req *v2pb.MutateRowRequest) error {
	tbl, err := f.tableAPI(res)
	if err != nil {
		return err
	}
	m := bigtable.InternalNewMutationFromProto(req.Mutations)
	return tbl.Apply(ctx, string(req.RowKey), m)
}

// Close shuts down the underlying bigtable.Client.
func (f *classicFallback) Close() error {
	if f.c == nil {
		return nil
	}
	return f.c.Close()
}

// parseTableID validates that name is in this fallback's scope and returns
// the table leaf ID. Reuses the package-level scopePrefixFor / leafAfter
// helpers from resourcename.go (same logic as Channel.parseTableName).
func (f *classicFallback) parseTableID(name string) (string, error) {
	rest, ok := strings.CutPrefix(name, f.scopePrefix)
	if !ok {
		return "", status.Errorf(codes.Internal,
			"accelerator fallback: resource %q is not in scope %s", name, strings.TrimSuffix(f.scopePrefix, "/"))
	}
	return leafAfter(name, rest, "tables/")
}

// parseAVIDs validates that name is in this fallback's scope and returns
// the table and authorized-view leaf IDs.
// Mirrors Channel.parseAuthorizedViewName.
func (f *classicFallback) parseAVIDs(name string) (tableID, viewID string, err error) {
	rest, ok := strings.CutPrefix(name, f.scopePrefix)
	if !ok {
		return "", "", status.Errorf(codes.Internal,
			"accelerator fallback: resource %q is not in scope %s", name, strings.TrimSuffix(f.scopePrefix, "/"))
	}
	afterTables, ok := strings.CutPrefix(rest, "tables/")
	if !ok {
		return "", "", malformedResource(name)
	}
	tableID, viewID, ok = strings.Cut(afterTables, "/authorizedViews/")
	if !ok || tableID == "" || viewID == "" ||
		strings.Contains(tableID, "/") || strings.Contains(viewID, "/") {
		return "", "", malformedResource(name)
	}
	return tableID, viewID, nil
}

// parseMVID validates that name is in this fallback's scope and returns
// the materialized-view leaf ID.
// Mirrors Channel.parseMaterializedViewName.
func (f *classicFallback) parseMVID(name string) (string, error) {
	rest, ok := strings.CutPrefix(name, f.scopePrefix)
	if !ok {
		return "", status.Errorf(codes.Internal,
			"accelerator fallback: resource %q is not in scope %s", name, strings.TrimSuffix(f.scopePrefix, "/"))
	}
	return leafAfter(name, rest, "materializedViews/")
}
