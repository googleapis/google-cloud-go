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

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sessionUnimplementedThreshold is the number of CONSECUTIVE
// codes.Unimplemented responses from the session path required to
// trip TableShim's sticky per-resource breaker. Matches Java
// (ShimImpl.java:77 MAX_CONSECUTIVE_UNIMPLEMENTED_FAILURES = 30). A
// var (not const) so tests can drop it to 1 for fast breaker-trip
// assertions without touching production code — same pattern as
// sessionTableCacheTTL / sessionTableCacheSweepInt in
// session_table_cache.go.
//
// "Consecutive" means the counter resets to 0 on any non-Unimplemented
// session response (success OR non-Unimplemented error) — see
// recordSessionOutcome. Rationale: a non-Unimplemented response proves
// the session RPC is understood by whatever backend served it (either
// wire is fine, or the failure is transport/app-level); the previous
// Unimplemented streak must have been transient or from a different
// AFE in the pool.
var sessionUnimplementedThreshold int32 = 30

// TableShim implements TableAPI by routing between a classic gRPC
// data-plane and a proto-native session data-plane
// (session.TableAPI). Traffic direction is decided per-call by
// the Diverter's SessionLoad ratio. TableShim owns the proto ↔
// bigtable.Row conversion so the session package can stay proto-native.
//
// Methods with no session equivalent (ReadRows, SampleRowKeys,
// ApplyBulk, ApplyReadModifyWrite) always delegate to classic.
// Conditional mutations always route to classic — session vRPC does
// not support the CheckAndMutateRow shape today.
//
// UNIMPLEMENTED fallback (two-layer):
//   - Per-call rescue: whenever a session-path RPC returns
//     codes.Unimplemented, TableShim transparently re-issues on the
//     classic path so the user's request always succeeds — even
//     BEFORE the sticky breaker trips. Improves UX over Java (which
//     fails the request until the breaker trips) at zero extra cost
//     (classic re-issue would happen on the next call anyway).
//   - Sticky per-resource breaker: after
//     sessionUnimplementedThreshold consecutive Unimplemented
//     responses, all subsequent calls skip the session path outright
//     via useSession()'s short-circuit. See sessionUnimplemented +
//     unimplementedCount + recordSessionOutcome.
type TableShim struct {
	classic  TableAPI
	session  session.TableAPI
	diverter *btransport.Diverter
	// sessionUnimplemented is the sticky per-resource UNIMPLEMENTED
	// breaker. Flipped by recordSessionOutcome when unimplementedCount
	// reaches sessionUnimplementedThreshold; useSession() short-circuits
	// on it so subsequent calls never hit session for the same resource.
	// Sticky for the shim's lifetime — no retry, no probe. UNIMPLEMENTED
	// is a backend capability signal (this AFE doesn't implement the
	// session RPC), not a transient failure; a fresh shim (via
	// bigtable.Client.OpenTable after the sessionTableCache TTL-evicts
	// this handle) starts un-tripped and can re-observe if backends
	// have since rolled out.
	sessionUnimplemented atomic.Bool
	// unimplementedCount is the running count of consecutive
	// Unimplemented responses from the session path. Incremented by
	// recordSessionOutcome on Unimplemented; reset to 0 on any other
	// session response (success OR non-Unimplemented error). Reaches
	// sessionUnimplementedThreshold → sessionUnimplemented flips true.
	// atomic.Int32 (not int32 + mutex) because updates are single-writer
	// per goroutine call and readers only take the value momentarily.
	unimplementedCount atomic.Int32
}

// NewTableShim wraps a classic TableAPI + a proto-native session API
// with a Diverter-gated router. Any of session or diverter may be nil,
// in which case the shim behaves like classic-only.
func NewTableShim(classic TableAPI, sessionAPI session.TableAPI, diverter *btransport.Diverter) TableAPI {
	return &TableShim{
		classic:  classic,
		session:  sessionAPI,
		diverter: diverter,
	}
}

// ReadRow implements TableAPI. Routes through the session path when
// the diverter allows and the session API is available; otherwise
// delegates to classic. On the session path, translates
// (row, opts) → SessionReadRowRequest, then translates the response
// back via protoRowToRow. WithFullReadStats callbacks fire from here.
//
// If the session path returns codes.Unimplemented, transparently
// re-issues the request via classic and trips the breaker so future
// ReadRow / Apply calls skip session directly.
func (t *TableShim) ReadRow(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
	if !t.useSession() {
		return t.classic.ReadRow(ctx, row, opts...)
	}
	// Parse opts using the classic settings shape so filter + full-read
	// stats callback plumbing stays in one place.
	tmpReq := &btpb.ReadRowsRequest{}
	settings := makeReadSettings(tmpReq, 0)
	for _, opt := range opts {
		opt.set(&settings)
	}
	req := &btpb.SessionReadRowRequest{
		Key:    []byte(row),
		Filter: tmpReq.Filter,
	}
	resp, err := t.session.ReadRow(ctx, req)
	t.recordSessionOutcome(err)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return t.classic.ReadRow(ctx, row, opts...)
		}
		return nil, err
	}
	if resp.GetStats() != nil && settings.fullReadStatsFunc != nil {
		stats := makeFullReadStats(resp.GetStats())
		settings.fullReadStatsFunc(&stats)
	}
	return protoRowToRow(resp.GetRow()), nil
}

// Apply implements TableAPI. Non-conditional mutations may route
// through the session path (subject to the diverter); conditional
// mutations always go to classic — CheckAndMutateRow has no session
// equivalent. Nil mutations also route to classic so the classic
// client's nil-handling error surfaces exactly as it always has,
// rather than panicking here on m.ops.
//
// If the session path returns codes.Unimplemented, transparently
// re-issues the mutation via classic and trips the breaker so future
// ReadRow / Apply calls skip session directly.
func (t *TableShim) Apply(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
	if m == nil || m.isConditional {
		return t.classic.Apply(ctx, row, m, opts...)
	}
	if !t.useSession() {
		return t.classic.Apply(ctx, row, m, opts...)
	}
	req := &btpb.SessionMutateRowRequest{
		Key:       []byte(row),
		Mutations: m.ops,
	}
	_, err := t.session.MutateRow(ctx, req)
	t.recordSessionOutcome(err)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return t.classic.Apply(ctx, row, m, opts...)
		}
		return err
	}
	return nil
}

// ReadRows delegates to classic — session support is not implemented.
func (t *TableShim) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	return t.classic.ReadRows(ctx, arg, f, opts...)
}

// SampleRowKeys delegates to classic.
func (t *TableShim) SampleRowKeys(ctx context.Context) ([]string, error) {
	return t.classic.SampleRowKeys(ctx)
}

// ApplyBulk delegates to classic.
func (t *TableShim) ApplyBulk(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
	return t.classic.ApplyBulk(ctx, rowKeys, muts, opts...)
}

// ApplyReadModifyWrite delegates to classic.
func (t *TableShim) ApplyReadModifyWrite(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
	return t.classic.ApplyReadModifyWrite(ctx, row, m)
}

// useSession returns true when both a session API and diverter are
// configured, the UNIMPLEMENTED breaker has NOT tripped for this
// resource, AND the diverter says this call should go over session.
// The sessionUnimplemented check is a cheap atomic Load that gates
// the diverter roll — a tripped shim never rolls, never dials, and
// never calls into the session backend.
func (t *TableShim) useSession() bool {
	return t.session != nil &&
		t.diverter != nil &&
		!t.sessionUnimplemented.Load() &&
		t.diverter.UseSession()
}

// recordSessionOutcome updates the consecutive-Unimplemented counter
// (and possibly flips the sticky breaker) based on the outcome of a
// session-path RPC. Called by ReadRow and Apply immediately after the
// session response, before the per-call fallback decision.
//
// Semantics:
//   - err == nil (session succeeded) → counter reset to 0. Proves the
//     RPC is understood — any prior Unimplemented streak was transient
//     or from a different AFE in the pool.
//   - err carries any non-Unimplemented code → counter reset to 0.
//     Same reasoning: the wire understood the RPC (failure is
//     transport / app-level), so the streak breaks.
//   - err carries codes.Unimplemented → counter increment. On the
//     transition from N-1 → N == sessionUnimplementedThreshold, flip
//     sessionUnimplemented true via CompareAndSwap so a follow-up
//     debug tag / metric hook (to be wired when RecordDebugTag lands
//     in the public transport surface) fires exactly once per
//     resource under concurrent trip races.
//
// Matches Java's SessionPoolImpl reset semantics
// (SessionPoolImpl.java:489, 578) adapted to our per-op observation
// surface — Java's signal source is session-close, ours is per-RPC.
func (t *TableShim) recordSessionOutcome(err error) {
	if err == nil || status.Code(err) != codes.Unimplemented {
		t.unimplementedCount.Store(0)
		return
	}
	if n := t.unimplementedCount.Add(1); n >= sessionUnimplementedThreshold {
		t.sessionUnimplemented.CompareAndSwap(false, true)
	}
}

// protoRowToRow converts a proto Row (the wire shape returned by a
// proto-native session backend's SessionReadRowResponse) into the
// classic bigtable.Row map shape that TableAPI.ReadRow callers expect.
//
// Contract: preserves wire order for cells within a column and columns
// within a family; a row with no cells (or nil input) returns nil to
// match classic Table.ReadRow's row-not-found signal — callers check
// `row == nil`. Same-named families that appear twice in Families are
// appended, not deduped, matching the server's wire format. Test suite
// (TestProtoRowToRow) pins every branch of this contract.
func protoRowToRow(pr *btpb.Row) Row {
	if pr == nil {
		return nil
	}
	rowMap := make(Row)
	rowKey := string(pr.Key)
	for _, fam := range pr.Families {
		familyName := fam.Name
		for _, col := range fam.Columns {
			columnName := familyName + ":" + string(col.Qualifier)
			var items []ReadItem
			for _, cell := range col.Cells {
				items = append(items, ReadItem{
					Row:       rowKey,
					Column:    columnName,
					Timestamp: Timestamp(cell.TimestampMicros),
					Value:     cell.Value,
					Labels:    cell.Labels,
				})
			}
			if len(items) > 0 {
				rowMap[familyName] = append(rowMap[familyName], items...)
			}
		}
	}
	// Match classic Table.ReadRow: a row with no cells is a not-found
	// signal; callers check `row == nil`. Server should send pr=nil for
	// not-found, but defensively collapse the empty-but-non-nil case too.
	if len(rowMap) == 0 {
		return nil
	}
	return rowMap
}
