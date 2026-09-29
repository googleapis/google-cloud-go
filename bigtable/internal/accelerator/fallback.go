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
	c      *bigtable.Client
	mu     sync.Mutex
	tables map[string]bigtable.TableAPI // keyed by full V2 resource name
}

// newClassicFallback constructs a classic bigtable.Client scoped to
// (project, instance, appProfile). DisableSession is set so this client never
// spins up session infrastructure — it is a pure classic-path fallback.
func newClassicFallback(ctx context.Context, project, instance, appProfile string, opts ...option.ClientOption) (*classicFallback, error) {
	c, err := bigtable.NewClientWithConfig(ctx, project, instance, bigtable.ClientConfig{
		AppProfile: appProfile,
		// make sure this only goes through classic path
		DisableSession: true,
	}, opts...)
	if err != nil {
		return nil, err
	}
	return &classicFallback{
		c:      c,
		tables: make(map[string]bigtable.TableAPI),
	}, nil
}

// tableAPI returns a cached bigtable.TableAPI for the given resource.
// The cache key is the full V2 resource name so table, authorized-view, and
// materialized-view keys never collide.
func (f *classicFallback) tableAPI(res adapters.Resource) (bigtable.TableAPI, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if t, ok := f.tables[res.Name]; ok {
		return t, nil
	}

	var t bigtable.TableAPI
	switch res.Kind {
	case adapters.ResourceTable:
		tableID, err := classicLeafID(res.Name, "tables/")
		if err != nil {
			return nil, err
		}
		t = f.c.OpenTable(tableID)
	case adapters.ResourceAuthorizedView:
		tableID, viewID, err := classicAVLeafIDs(res.Name)
		if err != nil {
			return nil, err
		}
		t = f.c.OpenAuthorizedView(tableID, viewID)
	case adapters.ResourceMaterializedView:
		viewID, err := classicLeafID(res.Name, "materializedViews/")
		if err != nil {
			return nil, err
		}
		t = f.c.OpenMaterializedView(viewID)
	default:
		return nil, status.Errorf(codes.Internal, "accelerator fallback: unknown resource kind %v", res.Kind)
	}

	f.tables[res.Name] = t
	return t, nil
}

// ReadRow executes a single-row read via the classic bigtable.Client and
// returns a ReadRowsResponse (possibly with no chunks for a missing row).
// req must have already passed validateSingleRowReadRequest.
func (f *classicFallback) ReadRow(ctx context.Context, res adapters.Resource, req *v2pb.ReadRowsRequest) (*v2pb.ReadRowsResponse, error) {
	// Extract and validate the row key before touching the table handle.
	// validateSingleRowReadRequest (called by the session stream's SendMsg)
	// should have ensured exactly one key or a closed-closed single-row range,
	// but we re-validate the range case defensively to avoid silent data
	// omission on malformed inputs.
	var key string
	switch {
	case req.Rows != nil && len(req.Rows.RowKeys) == 1:
		key = string(req.Rows.RowKeys[0])
	case req.Rows != nil && len(req.Rows.RowRanges) == 1:
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
			len(req.Rows.GetRowKeys()), len(req.Rows.GetRowRanges()))
	}

	tbl, err := f.tableAPI(res)
	if err != nil {
		return nil, err
	}

	var opts []bigtable.ReadOption
	if req.Filter != nil {
		opts = append(opts, bigtable.RowFilter(bigtable.RawProtoFilter(req.Filter)))
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
	m := bigtable.NewMutationFromProto(req.Mutations)
	return tbl.Apply(ctx, string(req.RowKey), m)
}

// Close shuts down the underlying bigtable.Client.
func (f *classicFallback) Close() error {
	if f.c == nil {
		return nil
	}
	return f.c.Close()
}

// classicLeafID extracts the trailing leaf ID from a fully-qualified V2
// resource name, given the segment prefix immediately before the leaf.
// For example, classicLeafID("projects/P/instances/I/tables/T", "tables/")
// returns "T". The scope has already been validated by Channel.openHandle, so
// any malformed name here is an internal invariant violation.
func classicLeafID(name, segPrefix string) (string, error) {
	idx := strings.LastIndex(name, segPrefix)
	if idx < 0 {
		return "", status.Errorf(codes.Internal,
			"accelerator fallback: cannot find %q in resource name %q", segPrefix, name)
	}
	leaf := name[idx+len(segPrefix):]
	if leaf == "" || strings.Contains(leaf, "/") {
		return "", status.Errorf(codes.Internal,
			"accelerator fallback: malformed leaf in resource name %q", name)
	}
	return leaf, nil
}

// classicAVLeafIDs extracts the table and authorized-view leaf IDs from a
// fully-qualified authorized-view resource name of the form
// "projects/P/instances/I/tables/T/authorizedViews/V".
func classicAVLeafIDs(name string) (tableID, viewID string, err error) {
	const avSeg = "/authorizedViews/"
	idx := strings.Index(name, avSeg)
	if idx < 0 {
		return "", "", status.Errorf(codes.Internal,
			"accelerator fallback: not an authorized-view name: %q", name)
	}
	viewID = name[idx+len(avSeg):]
	tableID, err = classicLeafID(name[:idx], "tables/")
	if err != nil {
		return "", "", err
	}
	if viewID == "" || strings.Contains(viewID, "/") {
		return "", "", status.Errorf(codes.Internal,
			"accelerator fallback: malformed authorized-view name: %q", name)
	}
	return tableID, viewID, nil
}
