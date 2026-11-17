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
	"errors"
	"sync"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// classicClient is the narrow surface classicFallback needs from
// *bigtable.Client. Interfaced so unit tests can stub client creation and
// Close without dialing a real Bigtable server.
type classicClient interface {
	OpenTable(table string) bigtable.TableAPI
	OpenAuthorizedView(table, authorizedView string) bigtable.TableAPI
	OpenMaterializedView(materializedView string) bigtable.TableAPI
	Close() error
}

var newClassicClient = func(
	ctx context.Context,
	project, instance string,
	config bigtable.ClientConfig,
	opts ...option.ClientOption,
) (classicClient, error) {
	return bigtable.NewClientWithConfig(ctx, project, instance, config, opts...)
}

// classicFallback wraps bigtable.Client(s) for use as a fallback when the
// session path returns codes.Unimplemented. It operates at the proto level
// (accepting V2 request protos directly) but delegates execution to
// bigtable.Client so retry logic, backoff, and RetryInfo header handling
// stay in sync with the rest of the Go client without duplication.
//
// Resource handles are cached by full V2 resource name and app profile, so
// repeated RPCs to the same table / authorized view / materialized view share
// one handle.
type classicFallback struct {
	ctx         context.Context
	project     string
	instance    string
	appProfile  string
	opts        []option.ClientOption
	scopePrefix string // "projects/<project>/instances/<instance>/" or "projects/<project>/instances/"

	mu      sync.Mutex
	clients map[clientKey]classicClient
	loading map[clientKey]*clientLoadState
	tables  map[string]bigtable.TableAPI // keyed by full V2 resource name (+ app profile)
	closed  bool
}

// newClassicFallback constructs a classicFallback scoped to project (and
// optionally instance and appProfile). DisableSession is set so the underlying
// client(s) never spin up session infrastructure — this is a pure classic-path
// fallback.
func newClassicFallback(ctx context.Context, project, instance, appProfile string, opts ...option.ClientOption) (*classicFallback, error) {
	cf := &classicFallback{
		ctx:         ctx,
		project:     project,
		instance:    instance,
		appProfile:  appProfile,
		opts:        opts,
		scopePrefix: scopePrefixFor(project, instance),
		clients:     make(map[clientKey]classicClient),
		loading:     make(map[clientKey]*clientLoadState),
		tables:      make(map[string]bigtable.TableAPI),
	}
	if instance != "" {
		c, err := newClassicClient(ctx, project, instance, bigtable.ClientConfig{
			AppProfile:     appProfile,
			DisableSession: true,
		}, opts...)
		if err != nil {
			return nil, err
		}
		cf.clients[clientKey{instance: instance, appProfile: appProfile}] = c
	}
	return cf, nil
}

func (f *classicFallback) getOrCreateClient(instanceID, appProfile string) (classicClient, error) {
	key := clientKey{instance: instanceID, appProfile: appProfile}
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return nil, status.Error(codes.Unavailable, "accelerator fallback: closed")
		}
		if c, ok := f.clients[key]; ok {
			f.mu.Unlock()
			return c, nil
		}
		if f.loading == nil {
			f.loading = make(map[clientKey]*clientLoadState)
		}
		if ls, ok := f.loading[key]; ok {
			f.mu.Unlock()
			<-ls.ready
			continue
		}
		ls := &clientLoadState{ready: make(chan struct{})}
		f.loading[key] = ls
		f.mu.Unlock()

		var (
			c   classicClient
			err error
		)
		func() {
			defer func() {
				f.mu.Lock()
				delete(f.loading, key)
				f.mu.Unlock()
				close(ls.ready)
			}()

			ctx := f.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			c, err = newClassicClient(ctx, f.project, instanceID, bigtable.ClientConfig{
				AppProfile:     appProfile,
				DisableSession: true,
			}, f.opts...)
			if err != nil {
				return
			}

			f.mu.Lock()
			if f.closed {
				f.mu.Unlock()
				_ = c.Close()
				c = nil
				err = status.Error(codes.Unavailable, "accelerator fallback: closed")
				return
			}
			if f.clients == nil {
				f.clients = make(map[clientKey]classicClient)
			}
			f.clients[key] = c
			f.mu.Unlock()
		}()
		return c, err
	}
}

func (f *classicFallback) resolveAppProfile(reqAppProfile string) (string, error) {
	if f.appProfile != "" {
		if reqAppProfile != "" && reqAppProfile != f.appProfile {
			return "", status.Errorf(codes.InvalidArgument,
				"accelerator fallback: request app_profile_id %q does not match this daemon's app profile (%s)",
				reqAppProfile, f.appProfile)
		}
		return f.appProfile, nil
	}
	return reqAppProfile, nil
}

// tableAPI returns a cached bigtable.TableAPI for the given resource.
// The cache key is the full V2 resource name (plus effective app profile when
// non-empty) so table, authorized-view, and materialized-view keys never collide.
func (f *classicFallback) tableAPI(res adapters.Resource) (bigtable.TableAPI, error) {
	appProfile, err := f.resolveAppProfile(res.AppProfile)
	if err != nil {
		return nil, err
	}
	cacheKey := res.Name
	if appProfile != "" {
		cacheKey = res.Name + "\x00" + appProfile
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, status.Error(codes.Unavailable, "accelerator fallback: closed")
	}
	if t, ok := f.tables[cacheKey]; ok {
		f.mu.Unlock()
		return t, nil
	}
	f.mu.Unlock()

	// Build the handle outside the lock.
	t, err := f.buildTableHandle(res, appProfile)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, status.Error(codes.Unavailable, "accelerator fallback: closed")
	}
	// Double-check: another goroutine may have raced us and stored a handle.
	if existing, ok := f.tables[cacheKey]; ok {
		f.mu.Unlock()
		return existing, nil
	}
	f.tables[cacheKey] = t
	f.mu.Unlock()
	return t, nil
}

// buildTableHandle constructs a bigtable.TableAPI for res. It validates that
// the resource belongs to f's scope before extracting the leaf IDs, so
// cross-project/cross-instance routing is rejected with InvalidArgument.
func (f *classicFallback) buildTableHandle(res adapters.Resource, appProfile string) (bigtable.TableAPI, error) {
	switch res.Kind {
	case adapters.ResourceTable:
		instanceID, tableID, err := f.parseTableIDAndInstance(res.Name)
		if err != nil {
			return nil, err
		}
		c, err := f.getOrCreateClient(instanceID, appProfile)
		if err != nil {
			return nil, err
		}
		return c.OpenTable(tableID), nil
	case adapters.ResourceAuthorizedView:
		instanceID, tableID, viewID, err := f.parseAVIDsAndInstance(res.Name)
		if err != nil {
			return nil, err
		}
		c, err := f.getOrCreateClient(instanceID, appProfile)
		if err != nil {
			return nil, err
		}
		return c.OpenAuthorizedView(tableID, viewID), nil
	case adapters.ResourceMaterializedView:
		instanceID, mvID, err := f.parseMVIDAndInstance(res.Name)
		if err != nil {
			return nil, err
		}
		c, err := f.getOrCreateClient(instanceID, appProfile)
		if err != nil {
			return nil, err
		}
		return c.OpenMaterializedView(mvID), nil
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

// Close shuts down the underlying bigtable.Client(s).
func (f *classicFallback) Close() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	clients := make([]classicClient, 0, len(f.clients))
	for _, c := range f.clients {
		if c != nil {
			clients = append(clients, c)
		}
	}
	f.clients = nil
	f.tables = nil
	f.mu.Unlock()

	var errs []error
	for _, c := range clients {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

const fallbackScopeErrFmt = "accelerator fallback: resource %q is not in scope %s"

// parseTableID validates that name is in this fallback's scope and returns
// the table leaf ID. Reuses the package-level helpers from resourcename.go.
func (f *classicFallback) parseTableID(name string) (string, error) {
	_, tableID, err := f.parseTableIDAndInstance(name)
	return tableID, err
}

func (f *classicFallback) parseTableIDAndInstance(name string) (instanceID, tableID string, err error) {
	return parseTableResource(f.scopePrefix, f.instance, fallbackScopeErrFmt, name)
}

// parseAVIDs validates that name is in this fallback's scope and returns
// the table and authorized-view leaf IDs.
// Mirrors Channel.parseAuthorizedViewName.
func (f *classicFallback) parseAVIDs(name string) (tableID, viewID string, err error) {
	_, tableID, viewID, err = f.parseAVIDsAndInstance(name)
	return tableID, viewID, err
}

func (f *classicFallback) parseAVIDsAndInstance(name string) (instanceID, tableID, viewID string, err error) {
	return parseAuthorizedViewResource(f.scopePrefix, f.instance, fallbackScopeErrFmt, name)
}

// parseMVID validates that name is in this fallback's scope and returns
// the materialized-view leaf ID.
// Mirrors Channel.parseMaterializedViewName.
func (f *classicFallback) parseMVID(name string) (string, error) {
	_, mvID, err := f.parseMVIDAndInstance(name)
	return mvID, err
}

func (f *classicFallback) parseMVIDAndInstance(name string) (instanceID, mvID string, err error) {
	return parseMaterializedViewResource(f.scopePrefix, f.instance, fallbackScopeErrFmt, name)
}
