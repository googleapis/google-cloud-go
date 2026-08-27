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

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// TableShim implements TableAPI by routing between a classic gRPC
// data-plane and a proto-native session data-plane
// (session.TableAPI). Traffic direction is decided per-call by the
// Diverter's SessionLoad ratio. TableShim owns the proto ↔
// bigtable.Row conversion so the session package can stay proto-native.
//
// Methods with no session equivalent (SampleRowKeys, ApplyBulk,
// ApplyReadModifyWrite) always delegate to classic. ReadRows delegates
// to classic for the general multi-row case; the single-key shape
// (arg is a RowList of length 1 — what SingleRow(k) returns) routes
// through the session ReadRow path since it is semantically identical
// to Table.ReadRow. Conditional mutations always route to classic —
// session vRPC does not support the CheckAndMutateRow shape today.
//
// ReadRow, single-key ReadRows, and Apply route their session call
// through session.InterceptUnimplemented: a codes.Unimplemented
// response falls back to classic, and after enough consecutive
// Unimplementeds the interceptor's sticky breaker gates useSession()
// so future calls skip session entirely.
type TableShim struct {
	classic       TableAPI
	session       session.TableAPI
	diverter      *btransport.Diverter
	unimplemented *session.UnimplementedErrorInterceptor
}

// NewTableShim wraps a classic TableAPI + a proto-native session API
// with a Diverter-gated router. Any of session or diverter may be nil,
// in which case the shim behaves like classic-only.
func NewTableShim(classic TableAPI, sessionAPI session.TableAPI, diverter *btransport.Diverter) TableAPI {
	return &TableShim{
		classic:       classic,
		session:       sessionAPI,
		diverter:      diverter,
		unimplemented: session.NewUnimplementedErrorInterceptor(session.DefaultUnimplementedThreshold),
	}
}

// ReadRow implements TableAPI. Routes through the session path when
// the diverter allows and the session API is available; otherwise
// delegates to classic. On the session path, translates
// (row, opts) → SessionReadRowRequest, then translates the response
// back via protoRowToRow. WithFullReadStats callbacks fire from here.
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
	return session.InterceptUnimplemented(t.unimplemented,
		func() (Row, error) {
			resp, err := t.session.ReadRow(ctx, req)
			if err != nil {
				return nil, err
			}
			if resp.GetStats() != nil && settings.fullReadStatsFunc != nil {
				stats := makeFullReadStats(resp.GetStats())
				settings.fullReadStatsFunc(&stats)
			}
			return protoRowToRow(resp.GetRow()), nil
		},
		func() (Row, error) {
			return t.classic.ReadRow(ctx, row, opts...)
		},
	)
}

// Apply implements TableAPI. Non-conditional mutations may route
// through the session path (subject to the diverter); conditional
// mutations always go to classic — CheckAndMutateRow has no session
// equivalent. Nil mutations also route to classic so the classic
// client's nil-handling error surfaces exactly as it always has,
// rather than panicking here on m.ops.
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
	// Apply returns only an error; use struct{} to satisfy the
	// interceptor's generic value slot without a stand-in response.
	_, err := session.InterceptUnimplemented(t.unimplemented,
		func() (struct{}, error) {
			_, e := t.session.MutateRow(ctx, req)
			return struct{}{}, e
		},
		func() (struct{}, error) {
			return struct{}{}, t.classic.Apply(ctx, row, m, opts...)
		},
	)
	return err
}

// ReadRows delegates to classic for the general multi-row case. When
// arg is a RowList of exactly one key — the shape SingleRow(k) returns
// and what Table.readRowClassic issues under the hood — the call is a
// ReadRow in disguise, so it dispatches through TableShim.ReadRow.
// Sharing that entry point means one implementation of session-vs-
// classic routing, filter / full-read-stats plumbing, and the
// Unimplemented → classic fallback covers both surfaces.
//
// LimitRows(<1) is left to classic so ownership of its no-dial
// short-circuit (LimitRows(0) returns nil, negative → errNegativeRowLimit)
// stays in the ReadRows body it was designed for.
func (t *TableShim) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	if keys, ok := arg.(RowList); ok && len(keys) == 1 && !hasZeroOrNegativeLimit(opts) {
		row, err := t.ReadRow(ctx, keys[0], opts...)
		if err != nil {
			return err
		}
		if row != nil {
			// Callback's bool return governs continuation across
			// multiple rows; with at most one row it has no effect.
			f(row)
		}
		return nil
	}
	return t.classic.ReadRows(ctx, arg, f, opts...)
}

// hasZeroOrNegativeLimit reports whether opts contain a LimitRows
// whose limit is < 1. Only the last LimitRows in opts wins (matching
// classic ReadRows' loop behavior), so scan back-to-front and stop at
// the first hit.
func hasZeroOrNegativeLimit(opts []ReadOption) bool {
	for i := len(opts) - 1; i >= 0; i-- {
		if lr, ok := opts[i].(limitRows); ok {
			return lr.limit < 1
		}
	}
	return false
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

// useSession returns true when session + diverter are configured, the
// interceptor's Unimplemented breaker hasn't tripped, and the diverter
// says this call should go over session.
func (t *TableShim) useSession() bool {
	return t.session != nil &&
		t.diverter != nil &&
		!t.unimplemented.Bypass() &&
		t.diverter.UseSession()
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
