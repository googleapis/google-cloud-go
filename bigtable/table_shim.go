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
// Methods with no session equivalent (ReadRows, SampleRowKeys,
// ApplyBulk, ApplyReadModifyWrite) always delegate to classic.
// Conditional mutations always route to classic — session vRPC does
// not support the CheckAndMutateRow shape today.
//
// UNIMPLEMENTED fallback lives in session.UnimplementedErrorInterceptor.
// TableShim owns one instance per resource; ReadRow and Apply route
// their session call through session.InterceptUnimplemented so a server that
// returns codes.Unimplemented on the session RPC transparently
// falls back to the classic path AND, after N consecutive
// Unimplemented responses, trips the interceptor's sticky breaker
// so useSession() short-circuits future calls at the routing gate.
type TableShim struct {
	classic  TableAPI
	session  session.TableAPI
	diverter *btransport.Diverter
	// unimplemented handles both the per-call classic fallback on
	// codes.Unimplemented AND the sticky breaker that skips session
	// entirely after N consecutive Unimplemented responses. See
	// session.UnimplementedErrorInterceptor for reset/trip semantics.
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
		unimplemented: session.NewUnimplementedErrorInterceptor(),
	}
}

// ReadRow implements TableAPI. Routes through the session path when
// the diverter allows and the session API is available; otherwise
// delegates to classic. On the session path, translates
// (row, opts) → SessionReadRowRequest, then translates the response
// back via protoRowToRow. WithFullReadStats callbacks fire from here.
//
// A session-path Unimplemented is transparently rescued via classic
// by the interceptor; see session.UnimplementedErrorInterceptor doc.
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
//
// A session-path Unimplemented is transparently rescued via classic
// by the interceptor; see session.UnimplementedErrorInterceptor doc.
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
	// Apply returns only an error; use struct{} for the value slot so
	// the interceptor's generic contract is satisfied without a
	// pretend-response type.
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
// configured, the interceptor's sticky UNIMPLEMENTED breaker has NOT
// tripped for this resource, AND the diverter says this call should
// go over session. The bypass check is a cheap atomic Load — a
// tripped shim never rolls the diverter, never dials, and never
// calls into the session backend.
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
