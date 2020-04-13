/*
Copyright 2019 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spannertest

// This file contains the implementation of the Spanner fake itself,
// namely the part behind the RPC interface.

// TODO: missing transactionality in a serious way!

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	structpb "github.com/golang/protobuf/ptypes/struct"

	"cloud.google.com/go/spanner/spansql"
)

type database struct {
	mu      sync.Mutex
	lastTS  time.Time // last commit timestamp
	tables  map[string]*table
	indexes map[string]struct{} // only record their existence

	rwMu sync.Mutex // held by read-write transactions
}

type table struct {
	mu sync.Mutex

	// Information about the table columns.
	// They are reordered on table creation so the primary key columns come first.
	cols     []colInfo
	colIndex map[string]int // col name to index
	pkCols   int            // number of primary key columns (may be 0)
	pkDesc   []bool         // whether each primary key column is in descending order

	// Rows are stored in primary key order.
	rows []row
}

// colInfo represents information about a column in a table or result set.
type colInfo struct {
	Name     string
	Type     spansql.Type
	AggIndex int // Index+1 of SELECT list for which this is an aggregate value.
}

// commitTimestampSentinel is a sentinel value for TIMESTAMP fields with allow_commit_timestamp=true.
// It is accepted, but never stored.
var commitTimestampSentinel = &struct{}{}

// transaction records information about a running transaction.
// This is not safe for concurrent use.
type transaction struct {
	// readOnly is whether this transaction was constructed
	// for read-only use, and should yield errors if used
	// to perform a mutation.
	readOnly bool

	d               *database
	commitTimestamp time.Time // not set if readOnly
	unlock          func()    // may be nil
}

func (d *database) NewReadOnlyTransaction() *transaction {
	return &transaction{
		readOnly: true,
	}
}

func (d *database) NewTransaction() *transaction {
	return &transaction{
		d: d,
	}
}

// Start starts the transaction and commits to a specific commit timestamp.
// This also locks out any other read-write transaction on this database
// until Commit/Rollback are called.
func (tx *transaction) Start() {
	// Commit timestamps are only guaranteed to be unique
	// when transactions write to overlapping sets of fields.
	// This simulated database exceeds that guarantee.

	// Grab rwMu for the duration of this transaction.
	// Take it before d.mu so we don't hold that lock
	// while waiting for d.rwMu, which is held for longer.
	tx.d.rwMu.Lock()

	tx.d.mu.Lock()
	const tsRes = 1 * time.Microsecond
	now := time.Now().UTC().Truncate(tsRes)
	if !now.After(tx.d.lastTS) {
		now = tx.d.lastTS.Add(tsRes)
	}
	tx.d.lastTS = now
	tx.d.mu.Unlock()

	tx.commitTimestamp = now
	tx.unlock = tx.d.rwMu.Unlock
}

func (tx *transaction) checkMutable() error {
	if tx.readOnly {
		// TODO: is this the right status?
		return status.Errorf(codes.InvalidArgument, "transaction is read-only")
	}
	return nil
}

func (tx *transaction) Commit() (time.Time, error) {
	if tx.unlock != nil {
		tx.unlock()
	}
	return tx.commitTimestamp, nil
}

func (tx *transaction) Rollback() {
	if tx.unlock != nil {
		tx.unlock()
	}
	// TODO: actually rollback
}

/*
row represents a list of data elements.

The mapping between Spanner types and Go types internal to this package are:
	BOOL		bool
	INT64		int64
	FLOAT64		float64
	STRING		string
	BYTES		[]byte
	DATE		string (RFC 3339 date; "YYYY-MM-DD")
	TIMESTAMP	string (RFC 3339 timestamp with zone; "YYYY-MM-DDTHH:MM:SSZ")
	ARRAY<T>	[]interface{}
	STRUCT		TODO
*/
type row []interface{}

func (r row) copyDataElem(index int) interface{} {
	v := r[index]
	if is, ok := v.([]interface{}); ok {
		// Deep-copy array values.
		v = append([]interface{}(nil), is...)
	}
	return v
}

// copyData returns a copy of the row.
func (r row) copyAllData() row {
	dst := make(row, 0, len(r))
	for i := range r {
		dst = append(dst, r.copyDataElem(i))
	}
	return dst
}

// copyData returns a copy of a subset of a row.
func (r row) copyData(indexes []int) row {
	if len(indexes) == 0 {
		return nil
	}
	dst := make(row, 0, len(indexes))
	for _, i := range indexes {
		dst = append(dst, r.copyDataElem(i))
	}
	return dst
}

func (d *database) LastCommitTimestamp() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastTS
}

func (d *database) GetDDL() []spansql.DDLStmt {
	// This lacks fidelity, but captures the details we support.
	d.mu.Lock()
	defer d.mu.Unlock()

	var stmts []spansql.DDLStmt

	for name, t := range d.tables {
		ct := &spansql.CreateTable{
			Name: name,
		}

		t.mu.Lock()
		for i, col := range t.cols {
			ct.Columns = append(ct.Columns, spansql.ColumnDef{
				Name: col.Name,
				Type: col.Type,
				// TODO: NotNull, AllowCommitTimestamp
			})
			if i < t.pkCols {
				ct.PrimaryKey = append(ct.PrimaryKey, spansql.KeyPart{
					Column: col.Name,
					Desc:   t.pkDesc[i],
				})
			}
		}
		t.mu.Unlock()

		stmts = append(stmts, ct)
	}

	return stmts
}

func (d *database) ApplyDDL(stmt spansql.DDLStmt) *status.Status {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Lazy init.
	if d.tables == nil {
		d.tables = make(map[string]*table)
	}
	if d.indexes == nil {
		d.indexes = make(map[string]struct{})
	}

	switch stmt := stmt.(type) {
	default:
		return status.Newf(codes.Unimplemented, "unhandled DDL statement type %T", stmt)
	case *spansql.CreateTable:
		if _, ok := d.tables[stmt.Name]; ok {
			return status.Newf(codes.AlreadyExists, "table %s already exists", stmt.Name)
		}
		if len(stmt.PrimaryKey) == 0 {
			return status.Newf(codes.InvalidArgument, "table %s has no primary key", stmt.Name)
		}

		// TODO: check stmt.Interleave details.

		// Move primary keys first, preserving their order.
		pk := make(map[string]int)
		var pkDesc []bool
		for i, kp := range stmt.PrimaryKey {
			pk[kp.Column] = -1000 + i
			pkDesc = append(pkDesc, kp.Desc)
		}
		sort.SliceStable(stmt.Columns, func(i, j int) bool {
			a, b := pk[stmt.Columns[i].Name], pk[stmt.Columns[j].Name]
			return a < b
		})

		t := &table{
			colIndex: make(map[string]int),
			pkCols:   len(pk),
			pkDesc:   pkDesc,
		}
		for _, cd := range stmt.Columns {
			if st := t.addColumn(cd, true); st.Code() != codes.OK {
				return st
			}
		}
		for col := range pk {
			if _, ok := t.colIndex[col]; !ok {
				return status.Newf(codes.InvalidArgument, "primary key column %q not in table", col)
			}
		}
		d.tables[stmt.Name] = t
		return nil
	case *spansql.CreateIndex:
		if _, ok := d.indexes[stmt.Name]; ok {
			return status.Newf(codes.AlreadyExists, "index %s already exists", stmt.Name)
		}
		d.indexes[stmt.Name] = struct{}{}
		return nil
	case *spansql.DropTable:
		if _, ok := d.tables[stmt.Name]; !ok {
			return status.Newf(codes.NotFound, "no table named %s", stmt.Name)
		}
		// TODO: check for indexes on this table.
		delete(d.tables, stmt.Name)
		return nil
	case *spansql.DropIndex:
		if _, ok := d.indexes[stmt.Name]; !ok {
			return status.Newf(codes.NotFound, "no index named %s", stmt.Name)
		}
		delete(d.indexes, stmt.Name)
		return nil
	case *spansql.AlterTable:
		t, ok := d.tables[stmt.Name]
		if !ok {
			return status.Newf(codes.NotFound, "no table named %s", stmt.Name)
		}
		switch alt := stmt.Alteration.(type) {
		default:
			return status.Newf(codes.Unimplemented, "unhandled DDL table alteration type %T", alt)
		case spansql.AddColumn:
			if st := t.addColumn(alt.Def, false); st.Code() != codes.OK {
				return st
			}
			return nil
		case spansql.AlterColumn:
			if st := t.alterColumn(alt.Def); st.Code() != codes.OK {
				return st
			}
			return nil
		}
	}

}

func (d *database) table(tbl string) (*table, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	t, ok := d.tables[tbl]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no table named %s", tbl)
	}
	return t, nil
}

// writeValues executes a write option (Insert, Update, etc.).
func (d *database) writeValues(tx *transaction, tbl string, cols []string, values []*structpb.ListValue, f func(t *table, colIndexes []int, r row) error) error {
	if err := tx.checkMutable(); err != nil {
		return err
	}

	t, err := d.table(tbl)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	colIndexes, err := t.colIndexes(cols)
	if err != nil {
		return err
	}
	revIndex := make(map[int]int) // table index to col index
	for j, i := range colIndexes {
		revIndex[i] = j
	}

	for pki := 0; pki < t.pkCols; pki++ {
		_, ok := revIndex[pki]
		if !ok {
			return status.Errorf(codes.InvalidArgument, "primary key column %s not included in write", t.cols[pki].Name)
		}
	}

	for _, vs := range values {
		if len(vs.Values) != len(colIndexes) {
			return status.Errorf(codes.InvalidArgument, "row of %d values can't be written to %d columns", len(vs.Values), len(colIndexes))
		}

		r := make(row, len(t.cols))
		for j, v := range vs.Values {
			i := colIndexes[j]

			x, err := valForType(v, t.cols[i].Type)
			if err != nil {
				return err
			}
			if x == commitTimestampSentinel {
				// Cloud Spanner commit timestamps have microsecond granularity.
				x = tx.commitTimestamp.Format("2006-01-02T15:04:05.999999Z")
			}

			r[i] = x
		}
		// TODO: enforce NOT NULL?
		// TODO: enforce that provided timestamp for commit_timestamp=true columns
		// are not ahead of the transaction's commit timestamp.

		if err := f(t, colIndexes, r); err != nil {
			return err
		}
	}

	return nil
}

func (d *database) Insert(tx *transaction, tbl string, cols []string, values []*structpb.ListValue) error {
	return d.writeValues(tx, tbl, cols, values, func(t *table, colIndexes []int, r row) error {
		pk := r[:t.pkCols]
		rowNum, found := t.rowForPK(pk)
		if found {
			return status.Errorf(codes.AlreadyExists, "row already in table")
		}
		t.insertRow(rowNum, r)
		return nil
	})
}

func (d *database) Update(tx *transaction, tbl string, cols []string, values []*structpb.ListValue) error {
	return d.writeValues(tx, tbl, cols, values, func(t *table, colIndexes []int, r row) error {
		if t.pkCols == 0 {
			return status.Errorf(codes.InvalidArgument, "cannot update table %s with no columns in primary key", tbl)
		}
		pk := r[:t.pkCols]
		rowNum, found := t.rowForPK(pk)
		if !found {
			// TODO: is this the right way to return `NOT_FOUND`?
			return status.Errorf(codes.NotFound, "row not in table")
		}

		for _, i := range colIndexes {
			t.rows[rowNum][i] = r[i]
		}
		return nil
	})
}

func (d *database) InsertOrUpdate(tx *transaction, tbl string, cols []string, values []*structpb.ListValue) error {
	return d.writeValues(tx, tbl, cols, values, func(t *table, colIndexes []int, r row) error {
		pk := r[:t.pkCols]
		rowNum, found := t.rowForPK(pk)
		if !found {
			// New row; do an insert.
			t.insertRow(rowNum, r)
		} else {
			// Existing row; do an update.
			for _, i := range colIndexes {
				t.rows[rowNum][i] = r[i]
			}
		}
		return nil
	})
}

// TODO: Replace

func (d *database) Delete(tx *transaction, table string, keys []*structpb.ListValue, keyRanges keyRangeList, all bool) error {
	if err := tx.checkMutable(); err != nil {
		return err
	}

	t, err := d.table(table)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if all {
		t.rows = nil
		return nil
	}

	for _, key := range keys {
		pk, err := t.primaryKey(key.Values)
		if err != nil {
			return err
		}
		// Not an error if the key does not exist.
		rowNum, found := t.rowForPK(pk)
		if found {
			copy(t.rows[rowNum:], t.rows[rowNum+1:])
			t.rows = t.rows[:len(t.rows)-1]
		}
	}

	for _, r := range keyRanges {
		r.startKey, err = t.primaryKeyPrefix(r.start.Values)
		if err != nil {
			return err
		}
		r.endKey, err = t.primaryKeyPrefix(r.end.Values)
		if err != nil {
			return err
		}
		startRow, endRow := t.findRange(r)
		if n := endRow - startRow; n > 0 {
			copy(t.rows[startRow:], t.rows[endRow:])
			t.rows = t.rows[:len(t.rows)-n]
		}
	}

	return nil
}

// readTable executes a read option (Read, ReadAll).
func (d *database) readTable(table string, cols []string, f func(*table, *rawIter, []int) error) (*rawIter, error) {
	t, err := d.table(table)
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	colIndexes, err := t.colIndexes(cols)
	if err != nil {
		return nil, err
	}

	ri := &rawIter{}
	for _, i := range colIndexes {
		ri.cols = append(ri.cols, t.cols[i])
	}
	return ri, f(t, ri, colIndexes)
}

func (d *database) Read(tbl string, cols []string, keys []*structpb.ListValue, keyRanges keyRangeList, limit int64) (rowIter, error) {
	return d.readTable(tbl, cols, func(t *table, ri *rawIter, colIndexes []int) error {
		// "If the same key is specified multiple times in the set (for
		// example if two ranges, two keys, or a key and a range
		// overlap), Cloud Spanner behaves as if the key were only
		// specified once."
		done := make(map[int]bool) // row numbers we've included in ri.

		// Specific keys.
		for _, key := range keys {
			pk, err := t.primaryKey(key.Values)
			if err != nil {
				return err
			}
			// Not an error if the key does not exist.
			rowNum, found := t.rowForPK(pk)
			if !found {
				continue
			}
			if done[rowNum] {
				continue
			}
			done[rowNum] = true
			ri.add(t.rows[rowNum], colIndexes)
			if limit > 0 && len(ri.rows) >= int(limit) {
				return nil
			}
		}

		// Key ranges.
		for _, r := range keyRanges {
			var err error
			r.startKey, err = t.primaryKeyPrefix(r.start.Values)
			if err != nil {
				return err
			}
			r.endKey, err = t.primaryKeyPrefix(r.end.Values)
			if err != nil {
				return err
			}
			startRow, endRow := t.findRange(r)
			for rowNum := startRow; rowNum < endRow; rowNum++ {
				if done[rowNum] {
					continue
				}
				done[rowNum] = true
				ri.add(t.rows[rowNum], colIndexes)
				if limit > 0 && len(ri.rows) >= int(limit) {
					return nil
				}
			}
		}

		return nil
	})
}

func (d *database) ReadAll(tbl string, cols []string, limit int64) (*rawIter, error) {
	return d.readTable(tbl, cols, func(t *table, ri *rawIter, colIndexes []int) error {
		for _, r := range t.rows {
			ri.add(r, colIndexes)
			if limit > 0 && len(ri.rows) >= int(limit) {
				break
			}
		}
		return nil
	})
}

func (t *table) addColumn(cd spansql.ColumnDef, newTable bool) *status.Status {
	if !newTable && cd.NotNull {
		return status.Newf(codes.InvalidArgument, "new non-key columns cannot be NOT NULL")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.rows) > 0 {
		if cd.NotNull {
			// TODO: what happens in this case?
			return status.Newf(codes.Unimplemented, "can't add NOT NULL columns to non-empty tables yet")
		}
		for i := range t.rows {
			t.rows[i] = append(t.rows[i], nil)
		}
	}

	t.cols = append(t.cols, colInfo{
		Name: cd.Name,
		Type: cd.Type,
	})
	t.colIndex[cd.Name] = len(t.cols) - 1

	return nil
}

func (t *table) alterColumn(cd spansql.ColumnDef) *status.Status {
	// Supported changes here are:
	//	Add NOT NULL to a non-key column, excluding ARRAY columns.
	//	Remove NOT NULL from a non-key column.
	//	Change a STRING column to a BYTES column or a BYTES column to a STRING column.
	//	Increase or decrease the length limit for a STRING or BYTES type (including to MAX).
	//	Enable or disable commit timestamps in value and primary key columns.
	// https://cloud.google.com/spanner/docs/schema-updates#supported-updates

	t.mu.Lock()
	defer t.mu.Unlock()

	ci, ok := t.colIndex[cd.Name]
	if !ok {
		// TODO: What's the right response code?
		return status.Newf(codes.InvalidArgument, "unknown column %q", cd.Name)
	}

	// Check and make type transformations.
	oldT, newT := t.cols[ci].Type, cd.Type
	stringOrBytes := func(bt spansql.TypeBase) bool { return bt == spansql.String || bt == spansql.Bytes }

	// If the only change is adding NOT NULL, this is okay except for primary key columns and array types.
	// We don't track whether commit timestamps are permitted on a per-column basis, so that's ignored.
	// TODO: Do this when we track NOT NULL-ness.

	// Change between STRING and BYTES is fine, as is increasing/decreasing the length limit.
	// TODO: This should permit array conversions too.
	if stringOrBytes(oldT.Base) && stringOrBytes(newT.Base) && !oldT.Array && !newT.Array {
		// TODO: Validate data; length limit changes should be rejected if they'd lead to data loss, for instance.
		var conv func(x interface{}) interface{}
		if oldT.Base == spansql.Bytes && newT.Base == spansql.String {
			conv = func(x interface{}) interface{} { return string(x.([]byte)) }
		} else if oldT.Base == spansql.String && newT.Base == spansql.Bytes {
			conv = func(x interface{}) interface{} { return []byte(x.(string)) }
		}
		if conv != nil {
			for _, row := range t.rows {
				if row[ci] != nil { // NULL stays as NULL.
					row[ci] = conv(row[ci])
				}
			}
		}
		t.cols[ci].Type = newT
		return nil
	}

	// TODO: Support other alterations.

	return status.Newf(codes.InvalidArgument, "unsupported ALTER COLUMN %s", cd.SQL())
}

func (t *table) insertRow(rowNum int, r row) {
	t.rows = append(t.rows, nil)
	copy(t.rows[rowNum+1:], t.rows[rowNum:])
	t.rows[rowNum] = r
}

// findRange finds the rows included in the key range,
// reporting it as a half-open interval.
// r.startKey and r.endKey should be populated.
func (t *table) findRange(r *keyRange) (int, int) {
	// startRow is the first row matching the range.
	startRow := sort.Search(len(t.rows), func(i int) bool {
		return rowCmp(r.startKey, t.rows[i][:t.pkCols], t.pkDesc) <= 0
	})
	if startRow == len(t.rows) {
		return startRow, startRow
	}
	if !r.startClosed && rowCmp(r.startKey, t.rows[startRow][:t.pkCols], t.pkDesc) == 0 {
		startRow++
	}

	// endRow is one more than the last row matching the range.
	endRow := sort.Search(len(t.rows), func(i int) bool {
		return rowCmp(r.endKey, t.rows[i][:t.pkCols], t.pkDesc) < 0
	})
	if !r.endClosed && rowCmp(r.endKey, t.rows[endRow-1][:t.pkCols], t.pkDesc) == 0 {
		endRow--
	}

	return startRow, endRow
}

// colIndexes returns the indexes for the named columns.
func (t *table) colIndexes(cols []string) ([]int, error) {
	var is []int
	for _, col := range cols {
		i, ok := t.colIndex[col]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "column %s not in table", col)
		}
		is = append(is, i)
	}
	return is, nil
}

// primaryKey constructs the internal representation of a primary key.
// The list of given values must be in 1:1 correspondence with the primary key of the table.
func (t *table) primaryKey(values []*structpb.Value) ([]interface{}, error) {
	if len(values) != t.pkCols {
		return nil, status.Errorf(codes.InvalidArgument, "primary key length mismatch: got %d values, table has %d", len(values), t.pkCols)
	}
	return t.primaryKeyPrefix(values)
}

// primaryKeyPrefix constructs the internal representation of a primary key prefix.
func (t *table) primaryKeyPrefix(values []*structpb.Value) ([]interface{}, error) {
	if len(values) > t.pkCols {
		return nil, status.Errorf(codes.InvalidArgument, "primary key length too long: got %d values, table has %d", len(values), t.pkCols)
	}

	var pk []interface{}
	for i, value := range values {
		v, err := valForType(value, t.cols[i].Type)
		if err != nil {
			return nil, err
		}
		pk = append(pk, v)
	}
	return pk, nil
}

// rowForPK returns the index of t.rows that holds the row for the given primary key, and true.
// If the given primary key isn't found, it returns the row that should hold it, and false.
func (t *table) rowForPK(pk []interface{}) (row int, found bool) {
	if len(pk) != t.pkCols {
		panic(fmt.Sprintf("primary key length mismatch: got %d values, table has %d", len(pk), t.pkCols))
	}

	i := sort.Search(len(t.rows), func(i int) bool {
		return rowCmp(pk, t.rows[i][:t.pkCols], t.pkDesc) <= 0
	})
	if i == len(t.rows) {
		return i, false
	}
	return i, rowEqual(pk, t.rows[i][:t.pkCols])
}

// rowCmp compares two rows, returning -1/0/+1.
// The desc arg indicates whether each column is in a descending order.
// This is used for primary key matching and so doesn't support array/struct types.
// a is permitted to be shorter than b.
func rowCmp(a, b []interface{}, desc []bool) int {
	for i := 0; i < len(a); i++ {
		if cmp := compareVals(a[i], b[i]); cmp != 0 {
			if desc[i] {
				cmp = -cmp
			}
			return cmp
		}
	}
	return 0
}

// rowEqual reports whether two rows are equal.
// This doesn't support array/struct types.
func rowEqual(a, b []interface{}) bool {
	for i := 0; i < len(a); i++ {
		if compareVals(a[i], b[i]) != 0 {
			return false
		}
	}
	return true
}

func valForType(v *structpb.Value, t spansql.Type) (interface{}, error) {
	if _, ok := v.Kind.(*structpb.Value_NullValue); ok {
		// TODO: enforce NOT NULL constraints?
		return nil, nil
	}

	if lv, ok := v.Kind.(*structpb.Value_ListValue); ok && t.Array {
		et := t // element type
		et.Array = false

		// Construct the non-nil slice for the list.
		arr := make([]interface{}, 0, len(lv.ListValue.Values))
		for _, v := range lv.ListValue.Values {
			x, err := valForType(v, et)
			if err != nil {
				return nil, err
			}
			arr = append(arr, x)
		}
		return arr, nil
	}

	switch t.Base {
	case spansql.Bool:
		bv, ok := v.Kind.(*structpb.Value_BoolValue)
		if ok {
			return bv.BoolValue, nil
		}
	case spansql.Int64:
		// The Spanner protocol encodes int64 as a decimal string.
		sv, ok := v.Kind.(*structpb.Value_StringValue)
		if ok {
			x, err := strconv.ParseInt(sv.StringValue, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad int64 string %q: %v", sv.StringValue, err)
			}
			return x, nil
		}
	case spansql.Float64:
		nv, ok := v.Kind.(*structpb.Value_NumberValue)
		if ok {
			return nv.NumberValue, nil
		}
	case spansql.String:
		sv, ok := v.Kind.(*structpb.Value_StringValue)
		if ok {
			return sv.StringValue, nil
		}
	case spansql.Bytes:
		sv, ok := v.Kind.(*structpb.Value_StringValue)
		if ok {
			// The Spanner protocol encodes BYTES in base64.
			return base64.StdEncoding.DecodeString(sv.StringValue)
		}
	case spansql.Date:
		// The Spanner protocol encodes DATE in RFC 3339 date format.
		sv, ok := v.Kind.(*structpb.Value_StringValue)
		if ok {
			// Store it internally as a string, but validate its value.
			s := sv.StringValue
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return nil, fmt.Errorf("bad DATE string %q: %v", s, err)
			}
			return s, nil
		}
	case spansql.Timestamp:
		// The Spanner protocol encodes TIMESTAMP in RFC 3339 timestamp format with zone Z.
		sv, ok := v.Kind.(*structpb.Value_StringValue)
		if ok {
			s := sv.StringValue
			if strings.ToLower(s) == "spanner.commit_timestamp()" {
				return commitTimestampSentinel, nil
			}
			// Store it internally as a string, but validate its value.
			if _, err := time.Parse("2006-01-02T15:04:05.999999999Z", s); err != nil {
				return nil, fmt.Errorf("bad TIMESTAMP string %q: %v", s, err)
			}
			return s, nil
		}
	}
	return nil, fmt.Errorf("unsupported inserting value kind %T into column of type %s", v.Kind, t.SQL())
}

type keyRange struct {
	start, end             *structpb.ListValue
	startClosed, endClosed bool

	// These are populated during an operation
	// when we know what table this keyRange applies to.
	startKey, endKey []interface{}
}

func (r *keyRange) String() string {
	var sb bytes.Buffer // TODO: Switch to strings.Builder when we drop support for Go 1.9.
	if r.startClosed {
		sb.WriteString("[")
	} else {
		sb.WriteString("(")
	}
	fmt.Fprintf(&sb, "%v,%v", r.startKey, r.endKey)
	if r.endClosed {
		sb.WriteString("]")
	} else {
		sb.WriteString(")")
	}
	return sb.String()
}

type keyRangeList []*keyRange

// Execute runs a DML statement.
// It returns the number of affected rows.
func (d *database) Execute(stmt spansql.DMLStmt, params queryParams) (int, error) { // TODO: return *status.Status instead?
	switch stmt := stmt.(type) {
	default:
		return 0, status.Errorf(codes.Unimplemented, "unhandled DML statement type %T", stmt)
	case *spansql.Delete:
		t, err := d.table(stmt.Table)
		if err != nil {
			return 0, err
		}

		t.mu.Lock()
		defer t.mu.Unlock()

		n := 0
		for i := 0; i < len(t.rows); {
			ec := evalContext{
				cols:   t.cols,
				row:    t.rows[i],
				params: params,
			}
			b, err := ec.evalBoolExpr(stmt.Where)
			if err != nil {
				return 0, err
			}
			if b {
				copy(t.rows[i:], t.rows[i+1:])
				t.rows = t.rows[:len(t.rows)-1]
				n++
				continue
			}
			i++
		}
		return n, nil
	}
}
