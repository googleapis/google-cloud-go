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
	internal "cloud.google.com/go/bigtable/internal/transport"
)

// TableShim wraps a classic and a session-based TableAPI and diverts
// traffic between them via the Diverter. The session TableAPI may be nil
// when the session data path isn't wired yet — pickSession only returns
// true when both the diverter says so AND a non-nil session backend is
// available, so nil-session is safe under any diverter ratio.
type TableShim struct {
	classic  TableAPI
	session  TableAPI
	diverter *internal.Diverter
}

// NewTableShim creates a new TableShim. Pass nil for session to run
// classic-only (with the diverter's UseSession outcome ignored).
func NewTableShim(classic, session TableAPI, diverter *internal.Diverter) TableAPI {
	return &TableShim{
		classic:  classic,
		session:  session,
		diverter: diverter,
	}
}

// pickSession returns true only when a session backend is wired, a
// diverter is wired, AND the diverter says to route this call to
// session. Consulting the diverter is side-effectful (it updates the
// per-outcome pick counters used by the debug UI), so the nil-session
// short-circuit runs FIRST — otherwise the pick-count histogram would
// show session picks that were silently downgraded to classic here.
// The nil-diverter guard covers direct-construction cases: TableShim
// is exported, and a test or external caller can build one with only
// classic + session set, without wiring a Diverter.
func (t *TableShim) pickSession() bool {
	if t.session == nil || t.diverter == nil {
		return false
	}
	return t.diverter.UseSession()
}

// ReadRow implements TableAPI.
func (t *TableShim) ReadRow(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
	if t.pickSession() {
		return t.session.ReadRow(ctx, row, opts...)
	}
	return t.classic.ReadRow(ctx, row, opts...)
}

// Apply implements TableAPI.
func (t *TableShim) Apply(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
	if t.pickSession() {
		return t.session.Apply(ctx, row, m, opts...)
	}
	return t.classic.Apply(ctx, row, m, opts...)
}

// ReadRows implements TableAPI. It delegates to classic as session support is not yet implemented.
func (t *TableShim) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	return t.classic.ReadRows(ctx, arg, f, opts...)
}

// SampleRowKeys implements TableAPI. It delegates to classic.
func (t *TableShim) SampleRowKeys(ctx context.Context) ([]string, error) {
	return t.classic.SampleRowKeys(ctx)
}

// ApplyBulk implements TableAPI. It delegates to classic.
func (t *TableShim) ApplyBulk(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
	return t.classic.ApplyBulk(ctx, rowKeys, muts, opts...)
}

// ApplyReadModifyWrite implements TableAPI. It delegates to classic.
func (t *TableShim) ApplyReadModifyWrite(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
	return t.classic.ApplyReadModifyWrite(ctx, row, m)
}

// protoRowToRow converts a proto Row (the wire shape returned by a
// proto-native session backend's SessionReadRowResponse) into the
// classic bigtable.Row map shape that TableAPI.ReadRow callers expect.
//
// Staged here so the follow-up change that swaps TableShim.session for a
// proto-native backend can call this without also introducing the
// conversion contract; the test suite pins every branch of that
// contract today (see TestProtoRowToRow).
//
// Contract: preserves wire order for cells within a column and columns
// within a family; a row with no cells (or nil input) returns nil to
// match classic Table.ReadRow's row-not-found signal — callers check
// `row == nil`. Same-named families that appear twice in Families are
// appended, not deduped, matching the server's wire format.
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
