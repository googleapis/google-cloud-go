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
// (session.TableAPI). Traffic direction is decided per-call by
// the Diverter's SessionLoad ratio. TableShim owns the proto ↔
// bigtable.Row conversion so the session package can stay proto-native.
//
// Methods with no session equivalent (ReadRows, SampleRowKeys,
// ApplyBulk, ApplyReadModifyWrite) always delegate to classic.
// Conditional mutations always route to classic — session vRPC does
// not support the CheckAndMutateRow shape today.
type TableShim struct {
	classic  TableAPI
	session  session.TableAPI
	diverter *btransport.Diverter
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
	if err != nil {
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
// equivalent.
func (t *TableShim) Apply(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
	if m != nil && m.isConditional {
		return t.classic.Apply(ctx, row, m, opts...)
	}
	if !t.useSession() {
		return t.classic.Apply(ctx, row, m, opts...)
	}
	req := &btpb.SessionMutateRowRequest{
		Key:       []byte(row),
		Mutations: m.ops,
	}
	if _, err := t.session.MutateRow(ctx, req); err != nil {
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
// configured AND the diverter says this call should go over session.
func (t *TableShim) useSession() bool {
	return t.session != nil && t.diverter != nil && t.diverter.UseSession()
}

// protoRowToRow converts a proto Row (the wire shape returned by the
// session-path SessionReadRowResponse) into the classic bigtable.Row
// map shape that TableAPI.ReadRow callers expect. Moved from the
// deleted session_table.go — the shim is the only consumer.
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
