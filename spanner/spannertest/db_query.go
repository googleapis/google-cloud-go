/*
Copyright 2020 Google LLC

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

import (
	"fmt"
	"io"
	"sort"

	"cloud.google.com/go/spanner/spansql"
)

/*
There's several ways to conceptualise SQL queries. The simplest, and what
we implement here, is a series of pipelines that transform the data, whether
pulling from a table (FROM tbl), filtering (WHERE expr), re-ordering (ORDER BY expr)
or other transformations.

The order of operations among those supported by Cloud Spanner is
	FROM + JOIN + set ops [TODO: set ops]
	WHERE
	GROUP BY
	aggregation
	HAVING [TODO]
	SELECT
	DISTINCT
	ORDER BY
	OFFSET
	LIMIT
*/

// rowIter represents some iteration over rows of data.
// It is returned by reads and queries.
type rowIter interface {
	// Cols returns the metadata about the returned data.
	Cols() []colInfo

	// Next returns the next row.
	// If done, it returns (nil, io.EOF).
	Next() (row, error)
}

// aggSentinel is a synthetic expression that refers to an aggregated value.
// It is transient only; it is never stored and only used during evaluation.
type aggSentinel struct {
	spansql.Expr
	Type     spansql.Type
	AggIndex int // Index+1 of SELECT list.
}

// nullIter is a rowIter that returns one empty row only.
// This is used for queries without a table.
type nullIter struct {
	done bool
}

func (ni *nullIter) Cols() []colInfo { return nil }
func (ni *nullIter) Next() (row, error) {
	if ni.done {
		return nil, io.EOF
	}
	ni.done = true
	return nil, nil
}

// tableIter is a rowIter that walks a table.
// It assumes the table is locked for the duration.
type tableIter struct {
	t        *table
	rowIndex int // index of next row to return

	alias spansql.ID // if non-empty, "AS <alias>"
}

func (ti *tableIter) Cols() []colInfo {
	// Build colInfo in the original column order.
	cis := make([]colInfo, len(ti.t.cols))
	for _, ci := range ti.t.cols {
		if ti.alias != "" {
			ci.Alias = spansql.PathExp{ti.alias, ci.Name}
		}
		cis[ti.t.origIndex[ci.Name]] = ci
	}
	return cis
}

func (ti *tableIter) Next() (row, error) {
	if ti.rowIndex >= len(ti.t.rows) {
		return nil, io.EOF
	}
	r := ti.t.rows[ti.rowIndex]
	ti.rowIndex++

	// Build output row in the original column order.
	res := make(row, len(r))
	for i, ci := range ti.t.cols {
		res[ti.t.origIndex[ci.Name]] = r[i]
	}

	return res, nil
}

// rawIter is a rowIter with fixed data.
type rawIter struct {
	// cols is the metadata about the returned data.
	cols []colInfo

	// rows holds the result data itself.
	rows []row
}

func (raw *rawIter) Cols() []colInfo { return raw.cols }
func (raw *rawIter) Next() (row, error) {
	if len(raw.rows) == 0 {
		return nil, io.EOF
	}
	res := raw.rows[0]
	raw.rows = raw.rows[1:]
	return res, nil
}

func (raw *rawIter) add(src row, colIndexes []int) {
	raw.rows = append(raw.rows, src.copyData(colIndexes))
}

// clone makes a shallow copy.
func (raw *rawIter) clone() *rawIter {
	return &rawIter{cols: raw.cols, rows: raw.rows}
}

func toRawIter(ri rowIter) (*rawIter, error) {
	if raw, ok := ri.(*rawIter); ok {
		return raw, nil
	}
	raw := &rawIter{cols: ri.Cols()}
	for {
		row, err := ri.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		raw.rows = append(raw.rows, row.copyAllData())
	}
	return raw, nil
}

// whereIter applies a WHERE clause.
type whereIter struct {
	ri    rowIter
	ec    evalContext
	where spansql.BoolExpr
}

func (wi whereIter) Cols() []colInfo { return wi.ri.Cols() }
func (wi whereIter) Next() (row, error) {
	for {
		row, err := wi.ri.Next()
		if err != nil {
			return nil, err
		}
		wi.ec.row = row

		b, err := wi.ec.evalBoolExpr(wi.where)
		if err != nil {
			return nil, err
		}
		if b != nil && *b {
			return row, nil
		}
	}
}

// selIter applies a SELECT list.
type selIter struct {
	ri   rowIter
	ec   evalContext
	cis  []colInfo
	list []spansql.Expr

	distinct bool // whether this is a SELECT DISTINCT
	seen     []row
}

func (si *selIter) Cols() []colInfo { return si.cis }
func (si *selIter) Next() (row, error) {
	for {
		r, err := si.next()
		if err != nil {
			return nil, err
		}
		if si.distinct && !si.keep(r) {
			continue
		}
		return r, nil
	}
}

// next retrieves the next row for the SELECT and evaluates its expression list.
func (si *selIter) next() (row, error) {
	r, err := si.ri.Next()
	if err != nil {
		return nil, err
	}
	si.ec.row = r

	var out row
	for _, e := range si.list {
		if e == spansql.Star {
			for _, j := range si.ec.starCols {
				out = append(out, r[j])
			}
		} else {
			v, err := si.ec.evalExpr(e)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func (si *selIter) keep(r row) bool {
	// This is hilariously inefficient; O(N^2) in the number of returned rows.
	// Some sort of hashing could be done to deduplicate instead.
	// This also breaks on array/struct types.
	for _, prev := range si.seen {
		if rowEqual(prev, r) {
			return false
		}
	}
	si.seen = append(si.seen, r)
	return true
}

// offsetIter applies an OFFSET clause.
type offsetIter struct {
	ri   rowIter
	skip int64
}

func (oi *offsetIter) Cols() []colInfo { return oi.ri.Cols() }
func (oi *offsetIter) Next() (row, error) {
	for oi.skip > 0 {
		_, err := oi.ri.Next()
		if err != nil {
			return nil, err
		}
		oi.skip--
	}
	row, err := oi.ri.Next()
	if err != nil {
		return nil, err
	}
	return row, nil
}

// limitIter applies a LIMIT clause.
type limitIter struct {
	ri  rowIter
	rem int64
}

func (li *limitIter) Cols() []colInfo { return li.ri.Cols() }
func (li *limitIter) Next() (row, error) {
	if li.rem <= 0 {
		return nil, io.EOF
	}
	row, err := li.ri.Next()
	if err != nil {
		return nil, err
	}
	li.rem--
	return row, nil
}

type queryParam struct {
	Value interface{} // internal representation
	Type  spansql.Type
}

type queryParams map[string]queryParam // TODO: change key to spansql.Param?

type queryContext struct {
	params queryParams

	tables     []*table // sorted by name
	tableIndex map[spansql.ID]*table
	locks      int
}

func (qc *queryContext) Lock() {
	// Take locks in name order.
	for _, t := range qc.tables {
		t.mu.Lock()
		qc.locks++
	}
}

func (qc *queryContext) Unlock() {
	for _, t := range qc.tables {
		t.mu.Unlock()
		qc.locks--
	}
}

func (d *database) Query(q spansql.Query, params queryParams) (ri rowIter, err error) {
	// Figure out the context of the query and take any required locks.
	qc, err := d.queryContext(q, params)
	if err != nil {
		return nil, err
	}
	qc.Lock()
	// On the way out, if there were locks taken, flatten the output
	// and release the locks.
	if qc.locks > 0 {
		defer func() {
			if err == nil {
				ri, err = toRawIter(ri)
			}
			qc.Unlock()
		}()
	}

	// Prepare auxiliary expressions to evaluate for ORDER BY.
	var aux []spansql.Expr
	var desc []bool
	for _, o := range q.Order {
		aux = append(aux, o.Expr)
		desc = append(desc, o.Desc)
	}

	si, err := d.evalSelect(q.Select, qc)
	if err != nil {
		return nil, err
	}
	ri = si

	// Apply ORDER BY.
	if len(q.Order) > 0 {
		// Evaluate the selIter completely, and sort the rows by the auxiliary expressions.
		rows, keys, err := evalSelectOrder(si, aux)
		if err != nil {
			return nil, err
		}
		sort.Sort(externalRowSorter{rows: rows, keys: keys, desc: desc})
		ri = &rawIter{cols: si.cis, rows: rows}
	}

	// Apply LIMIT, OFFSET.
	if q.Limit != nil {
		if q.Offset != nil {
			off, err := evalLiteralOrParam(q.Offset, params)
			if err != nil {
				return nil, err
			}
			ri = &offsetIter{ri: ri, skip: off}
		}

		lim, err := evalLiteralOrParam(q.Limit, params)
		if err != nil {
			return nil, err
		}
		ri = &limitIter{ri: ri, rem: lim}
	}

	return ri, nil
}

func (d *database) queryContext(q spansql.Query, params queryParams) (*queryContext, error) {
	qc := &queryContext{
		params: params,
	}

	// Look for any mentioned tables and add them to qc.tableIndex.
	addTable := func(name spansql.ID) error {
		if _, ok := qc.tableIndex[name]; ok {
			return nil // Already found this table.
		}
		t, err := d.table(name)
		if err != nil {
			return err
		}
		if qc.tableIndex == nil {
			qc.tableIndex = make(map[spansql.ID]*table)
		}
		qc.tableIndex[name] = t
		return nil
	}
	var findTables func(sf spansql.SelectFrom) error
	findTables = func(sf spansql.SelectFrom) error {
		switch sf := sf.(type) {
		default:
			return fmt.Errorf("can't prepare query context for SelectFrom of type %T", sf)
		case spansql.SelectFromList:
			for j := range sf {
				if err := findTables(sf[j]); err != nil {
					return err
				}
			}
			return nil
		case spansql.SelectFromTable:
			return addTable(sf.Table)
		case spansql.SelectFromJoin:
			if err := findTables(sf.LHS); err != nil {
				return err
			}
			return findTables(sf.RHS)
		case spansql.SelectFromUnnest:
			// TODO: if array paths get supported, this will need more work.
			return nil
		}
	}
	for _, sf := range q.Select.From {
		if err := findTables(sf); err != nil {
			return nil, err
		}
	}

	// Build qc.tables in name order so we can take locks in a well-defined order.
	var names []spansql.ID
	for name := range qc.tableIndex {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	for _, name := range names {
		qc.tables = append(qc.tables, qc.tableIndex[name])
	}

	return qc, nil
}

// evalCartesian returns the cartesian product of the provided evalContext and rowIter slices, which
// are expected to be correlated by slice index.
func (d *database) evalCartesian(ff []evalContext, rr []rowIter) (evalContext, rowIter, error) {
	if len(ff) != len(rr) {
		return evalContext{}, nil, fmt.Errorf("mismatched context and iterator sizes")
	}
	mergedEC := evalContext{}

	// When we cartesian iterators, we convert them to raw iterators, which copies all rows. We want to avoid
	// this overhead in the common case of computing a cartesian with a nullIter (which has a single empty row).
	// However, the evalContext that corresponds to a nullIter may contain columns and query parameters,
	// so we merge it and ignore the iterator.
	for j := len(rr) - 1; j >= 0; j-- {
		if _, ok := rr[j].(*nullIter); ok {
			var err error
			mergedEC, err = mergedEC.merge(ff[j])
			if err != nil {
				return evalContext{}, nil, err
			}
			ff, rr = append(ff[:j], ff[j+1:]...), append(rr[:j], rr[j+1:]...)
		}
	}
	if len(ff) == 0 {
		return mergedEC, &nullIter{}, nil
	}
	if len(ff) == 1 {
		var err error
		mergedEC, err = mergedEC.merge(ff[0])
		if err != nil {
			return evalContext{}, nil, err
		}
		return mergedEC, rr[0], nil
	}
	rows := make([][]row, len(ff))
	var cols []colInfo
	for j := range ff {
		raw, err := toRawIter(rr[j])
		if err != nil {
			return evalContext{}, nil, err
		}
		rows[j] = raw.rows
		cols = append(cols, raw.cols...)
		mergedEC, err = mergedEC.merge(ff[j])
		if err != nil {
			return evalContext{}, nil, err
		}
	}
	return mergedEC, &cartesianIterator{
		rows: rows,
		cols: cols,
	}, nil
}

type cartesianIterator struct {
	rows [][]row
	cols []colInfo

	loop     int
	counters []int
}

func (ci *cartesianIterator) Cols() []colInfo { return ci.cols }
func (ci *cartesianIterator) Next() (row, error) {
	if len(ci.rows) == 0 {
		return nil, io.EOF
	}
	if len(ci.counters) != len(ci.rows) {
		ci.counters = make([]int, len(ci.rows))
		ci.loop = len(ci.rows) - 1
	}
	if ci.loop < 0 {
		return nil, io.EOF
	}
	res := make(row, 0, len(ci.cols))
	for j := range ci.rows {
		if ci.counters[j] < len(ci.rows[j]) {
			r := ci.rows[j][ci.counters[j]]
			res = append(res, r...)
		}
	}
	ci.increment()
	return res, nil
}

func (ci *cartesianIterator) increment() bool {
	if ci.loop < 0 {
		return false
	}

	ci.counters[ci.loop]++
	res := true
	if ci.counters[ci.loop] >= len(ci.rows[ci.loop]) {
		ci.counters[ci.loop] = 0
		ci.loop--
		res = ci.increment()
		if res {
			ci.loop++
		}
	}
	return res
}

// unnestIter is a rowIter that processes an unnest operation per input row
type unnestIter struct {
	vals []interface{}
	cols []colInfo
	pos  int

	ec   evalContext
	sf   spansql.SelectFromUnnest
	iter rowIter
	err  error
}

func (u *unnestIter) Bind(ec evalContext) error {
	u.ec = ec
	// TODO: Do all relevant types flow through here? Path expressions might be tricky here.
	col, err := u.ec.colInfo(u.sf.Expr)
	if err != nil {
		return fmt.Errorf("evaluating type of UNNEST arg: %v", err)
	}
	if !col.Type.Array {
		return fmt.Errorf("type of UNNEST arg is non-array %s", col.Type.SQL())
	}
	// The output of this UNNEST is the non-array version.
	col.Name = u.sf.Alias // may be empty
	col.Type.Array = false
	col.Alias = nil

	offsetName := u.sf.OffsetAlias
	if offsetName == "" {
		offsetName = "offset"
	}
	offsetCol := colInfo{
		Type:    int64Type,
		Name:    offsetName,
		NotNull: true,
	}

	cols := []colInfo{col, offsetCol}
	u.cols = cols
	return nil
}

func (u *unnestIter) Cols() []colInfo {
	var res []colInfo
	res = append(res, u.iter.Cols()...)
	res = append(res, u.cols...)
	return res
}

func (u *unnestIter) Next() (row, error) {
	if u.err != nil {
		return nil, u.err
	}
	for u.pos >= len(u.vals) {
		r, err := u.iter.Next()
		if err != nil {
			u.err = err
			return nil, err
		}

		u.ec.row = r
		u.pos = 0

		// Evaluate the expression, and yield a virtual table with one column.
		e, err := u.ec.evalExpr(u.sf.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating UNNEST arg: %v", err)
		}
		if e == nil {
			u.vals = nil
			continue
		}
		arr, ok := e.([]interface{})
		if !ok {
			return nil, fmt.Errorf("evaluating UNNEST arg with %+v from %+v gave %t, want array", u.ec, u.iter, e)
		}
		u.vals = arr
	}
	var r row
	r = append(r, u.ec.row...)
	r = append(r, row{u.vals[u.pos], int64(u.pos)}...)
	u.pos++
	return r, nil
}

func (d *database) evalSelect(sel spansql.Select, qc *queryContext) (si *selIter, evalErr error) {
	ec := evalContext{
		params: qc.params,
	}

	// First stage is to identify the data source.
	// If there's a FROM then that names a table to use.
	ec, ri, err := d.evalSelectFrom(qc, ec, spansql.SelectFromList(sel.From), &nullIter{})
	if err != nil {
		return nil, err
	}

	// Apply WHERE.
	if sel.Where != nil {
		ri = whereIter{
			ri:    ri,
			ec:    ec,
			where: sel.Where,
		}
	}

	// Load aliases visible to any future iterators,
	// including GROUP BY and ORDER BY. These are not visible to the WHERE clause.
	ec.aliases = make(map[spansql.ID]spansql.Expr)
	for i, alias := range sel.ListAliases {
		ec.aliases[alias] = sel.List[i]
	}
	// TODO: Add aliases for "1", "2", etc.

	// Apply GROUP BY.
	// This only reorders rows to group rows together;
	// aggregation happens next.
	var rowGroups [][2]int // Sequence of half-open intervals of row numbers.
	if len(sel.GroupBy) > 0 {
		raw, err := toRawIter(ri)
		if err != nil {
			return nil, err
		}
		keys := make([][]interface{}, 0, len(raw.rows))
		for _, row := range raw.rows {
			// Evaluate sort key for this row.
			ec.row = row
			key, err := ec.evalExprList(sel.GroupBy)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}

		// Reorder rows base on the evaluated keys.
		ers := externalRowSorter{rows: raw.rows, keys: keys}
		sort.Sort(ers)
		raw.rows = ers.rows

		// Record groups as a sequence of row intervals.
		// Each group is a run of the same keys.
		start := 0
		for i := 1; i < len(keys); i++ {
			if compareValLists(keys[i-1], keys[i], nil) == 0 {
				continue
			}
			rowGroups = append(rowGroups, [2]int{start, i})
			start = i
		}
		if len(keys) > 0 {
			rowGroups = append(rowGroups, [2]int{start, len(keys)})
		}

		// Clear aliases, since they aren't visible elsewhere.
		ec.aliases = nil

		ri = raw
	}

	// Handle aggregation.
	// TODO: Support more than one aggregation function; does Spanner support that?
	aggI := -1
	for i, e := range sel.List {
		// Supported aggregate funcs have exactly one arg.
		f, ok := e.(spansql.Func)
		if !ok || len(f.Args) != 1 {
			continue
		}
		_, ok = aggregateFuncs[f.Name]
		if !ok {
			continue
		}
		if aggI > -1 {
			return nil, fmt.Errorf("only one aggregate function is supported")
		}
		aggI = i
	}
	if aggI > -1 {
		raw, err := toRawIter(ri)
		if err != nil {
			return nil, err
		}
		if len(sel.GroupBy) == 0 {
			// No grouping, so aggregation applies to the entire table (e.g. COUNT(*)).
			// This may result in a [0,0) entry for empty inputs.
			rowGroups = [][2]int{{0, len(raw.rows)}}
		}
		fexpr := sel.List[aggI].(spansql.Func)
		fn := aggregateFuncs[fexpr.Name]
		starArg := fexpr.Args[0] == spansql.Star
		if starArg && !fn.AcceptStar {
			return nil, fmt.Errorf("aggregate function %s does not accept * as an argument", fexpr.Name)
		}
		var argType spansql.Type
		if !starArg {
			ci, err := ec.colInfo(fexpr.Args[0])
			if err != nil {
				return nil, fmt.Errorf("evaluating aggregate function %s arg type: %v", fexpr.Name, err)
			}
			argType = ci.Type
		}

		// Prepare output.
		rawOut := &rawIter{
			// Same as input columns, but also the aggregate value.
			// Add the colInfo for the aggregate at the end
			// so we know the type.
			// Make a copy for safety.
			cols: append([]colInfo(nil), raw.cols...),
		}

		var aggType spansql.Type
		for _, rg := range rowGroups {
			// Compute aggregate value across this group.
			var values []interface{}
			for i := rg[0]; i < rg[1]; i++ {
				ec.row = raw.rows[i]
				if starArg {
					// A non-NULL placeholder is sufficient for aggregation.
					values = append(values, 1)
				} else {
					x, err := ec.evalExpr(fexpr.Args[0])
					if err != nil {
						return nil, err
					}
					values = append(values, x)
				}
			}
			x, typ, err := fn.Eval(values, argType)
			if err != nil {
				return nil, err
			}
			aggType = typ

			var outRow row
			// Output for the row group is the first row of the group (arbitrary,
			// but it should be representative), and the aggregate value.
			// TODO: Should this exclude the aggregated expressions so they can't be selected?
			// If the row group is empty then only the aggregation value is used;
			// this covers things like COUNT(*) with no matching rows.
			if rg[0] < len(raw.rows) {
				repRow := raw.rows[rg[0]]
				for i := range repRow {
					outRow = append(outRow, repRow.copyDataElem(i))
				}
			} else {
				// Fill with NULLs to keep the rows and colInfo aligned.
				for i := 0; i < len(rawOut.cols); i++ {
					outRow = append(outRow, nil)
				}
			}
			outRow = append(outRow, x)
			rawOut.rows = append(rawOut.rows, outRow)
		}

		if aggType == (spansql.Type{}) {
			// Fallback; there might not be any groups.
			// TODO: Should this be in aggregateFunc?
			aggType = int64Type
		}
		rawOut.cols = append(raw.cols, colInfo{
			Name:     spansql.ID(fexpr.SQL()), // TODO: this is a bit hokey, but it is output only
			Type:     aggType,
			AggIndex: aggI + 1,
		})

		ri = rawOut
		ec.cols = rawOut.cols
		sel.List[aggI] = aggSentinel{ // Mutate query so evalExpr in selIter picks out the new value.
			Type:     aggType,
			AggIndex: aggI + 1,
		}
	}

	// TODO: Support table sampling.

	// Apply SELECT list.
	var colInfos []colInfo
	for i, e := range sel.List {
		if e == spansql.Star {
			for _, j := range ec.starCols {
				colInfos = append(colInfos, ec.cols[j])
			}
		} else {
			ci, err := ec.colInfo(e)
			if err != nil {
				return nil, err
			}
			if len(sel.ListAliases) > 0 {
				alias := sel.ListAliases[i]
				if alias != "" {
					ci.Name = alias
				}
			}
			// TODO: deal with ci.Name == ""?
			colInfos = append(colInfos, ci)
		}
	}

	return &selIter{
		ri:   ri,
		ec:   ec,
		cis:  colInfos,
		list: sel.List,

		distinct: sel.Distinct, // Apply DISTINCT.
	}, nil
}

func (d *database) evalSelectFrom(qc *queryContext, ec evalContext, sf spansql.SelectFrom, iter rowIter) (evalContext, rowIter, error) {
	switch sf := sf.(type) {
	default:
		return ec, nil, fmt.Errorf("selecting with FROM clause of type %T not yet supported", sf)
	case spansql.SelectFromList:
		ri := iter
		for j := range sf {
			fec, fri, err := d.evalSelectFrom(qc, ec, sf[j], ri)
			if err != nil {
				return ec, nil, err
			}

			ri = fri
			ec = fec
		}
		return ec, ri, nil
	case spansql.SelectFromTable:
		t, ok := qc.tableIndex[sf.Table]
		if !ok {
			// This shouldn't be possible; the queryContext should have discovered missing tables already.
			return ec, nil, fmt.Errorf("unknown table %q", sf.Table)
		}
		ti := &tableIter{t: t}
		if sf.Alias != "" {
			ti.alias = sf.Alias
		} else {
			// There is an implicit alias using the table name.
			// https://cloud.google.com/spanner/docs/query-syntax#implicit_aliases
			ti.alias = sf.Table
		}
		cols := ti.Cols()
		starCols := make([]int, len(cols))
		for j := range starCols {
			starCols[j] = j
		}

		fec := evalContext{
			cols:     cols,
			starCols: starCols,
		}
		return d.evalCartesian([]evalContext{ec, fec}, []rowIter{iter, ti})
	case spansql.SelectFromJoin:
		// TODO: Avoid the toRawIter calls here by doing the RHS recursive evalSelectFrom in joinIter.Next on demand.

		var lhsEC evalContext
		var lhs rowIter
		var err error
		if sf.LHS == nil {
			lhsEC, lhs = ec, iter
		} else {
			lhsEC, lhs, err = d.evalSelectFrom(qc, ec, sf.LHS, iter)
		}
		if err != nil {
			return ec, nil, err
		}
		lhsRaw, err := toRawIter(lhs)
		if err != nil {
			return ec, nil, err
		}

		rhsEC, rhs, err := d.evalSelectFrom(qc, ec, sf.RHS, iter)
		if err != nil {
			return ec, nil, err
		}
		rhsRaw, err := toRawIter(rhs)
		if err != nil {
			return ec, nil, err
		}

		ji, jec, err := newJoinIter(lhsRaw, rhsRaw, lhsEC, rhsEC, sf)
		if err != nil {
			return ec, nil, err
		}
		return jec, ji, nil
	case spansql.SelectFromUnnest:
		u := &unnestIter{
			sf:   sf,
			iter: iter,
		}
		if err := u.Bind(ec); err != nil {
			return ec, nil, err
		}
		uec, err := ec.merge(evalContext{
			cols:     u.cols,
			starCols: []int{0},
		})
		if err != nil {
			return ec, nil, err
		}
		return uec, u, nil
	}
}

func newJoinIter(lhs, rhs *rawIter, lhsEC, rhsEC evalContext, sfj spansql.SelectFromJoin) (*joinIter, evalContext, error) {
	if sfj.On != nil && len(sfj.Using) > 0 {
		return nil, evalContext{}, fmt.Errorf("JOIN may not have both ON and USING clauses")
	}
	if sfj.On == nil && len(sfj.Using) == 0 && sfj.Type != spansql.CrossJoin {
		// TODO: This isn't correct for joining against a non-table.
		return nil, evalContext{}, fmt.Errorf("non-CROSS JOIN must have ON or USING clause")
	}

	// Start with the context from the LHS (aliases and params should be the same on both sides).
	ji := &joinIter{
		jt: sfj.Type,
		ec: lhsEC,

		primary:       lhs,
		secondaryOrig: rhs,

		primaryOffset:   0,
		secondaryOffset: len(lhsEC.cols),
	}
	switch ji.jt {
	case spansql.LeftJoin:
		ji.nullPad = true
	case spansql.RightJoin:
		ji.nullPad = true
		// Primary is RHS.
		ji.ec = rhsEC
		ji.primary, ji.secondaryOrig = rhs, lhs
		ji.primaryOffset, ji.secondaryOffset = len(rhsEC.cols), 0
	case spansql.FullJoin:
		// FULL JOIN is implemented as a LEFT JOIN with tracking for which rows of the RHS
		// have been used. Then, at the end of the iteration, the unused RHS rows are emitted.
		ji.nullPad = true
		ji.used = make([]bool, 0, 10) // arbitrary preallocation
	}
	ji.ec.cols, ji.ec.row = nil, nil

	// Construct a merged evalContext, and prepare the join condition evaluation.
	// TODO: Remove ambiguous names here? Or catch them when evaluated?
	// TODO: aliases might need work?
	if len(sfj.Using) == 0 {
		ji.prepNonUsing(sfj.On, lhsEC, rhsEC)
	} else {
		if err := ji.prepUsing(sfj.Using, lhsEC, rhsEC, ji.jt == spansql.RightJoin); err != nil {
			return nil, evalContext{}, err
		}
	}

	return ji, ji.ec, nil
}

// prepNonUsing configures the joinIter to evaluate with an ON clause or no join clause.
// The arg is nil in the latter case.
func (ji *joinIter) prepNonUsing(on spansql.BoolExpr, lhsEC, rhsEC evalContext) {
	// Having ON or no clause results in the full set of columns from both sides.
	// Force a copy.
	lhsOffset := len(ji.ec.cols)
	ji.ec.cols = append(ji.ec.cols, lhsEC.cols...)
	rhsOffset := len(ji.ec.cols)
	ji.ec.cols = append(ji.ec.cols, rhsEC.cols...)
	ji.ec.row = make(row, len(ji.ec.cols))
	ji.ec.starCols = make([]int, 0, len(lhsEC.starCols)+len(rhsEC.starCols))
	for j := range lhsEC.starCols {
		ji.ec.starCols = append(ji.ec.starCols, lhsEC.starCols[j]+lhsOffset)
	}
	for j := range rhsEC.starCols {
		ji.ec.starCols = append(ji.ec.starCols, rhsEC.starCols[j]+rhsOffset)
	}

	ji.cond = func(primary, secondary row) (bool, error) {
		copy(ji.ec.row[ji.primaryOffset:], primary)
		copy(ji.ec.row[ji.secondaryOffset:], secondary)
		if on == nil {
			// No condition; all rows match.
			return true, nil
		}
		b, err := ji.ec.evalBoolExpr(on)
		if err != nil {
			return false, err
		}
		return b != nil && *b, nil
	}
	ji.zero = func(primary, secondary row) {
		for i := range ji.ec.row {
			ji.ec.row[i] = nil
		}
		copy(ji.ec.row[ji.primaryOffset:], primary)
		copy(ji.ec.row[ji.secondaryOffset:], secondary)
	}
}

func (ji *joinIter) prepUsing(using []spansql.ID, lhsEC, rhsEC evalContext, flipped bool) error {
	// Having a USING clause results in the set of named columns once,
	// followed by the unnamed columns from both sides.

	// lhsUsing is the column indexes in the LHS that the USING clause references.
	// rhsUsing is similar.
	// lhsNotUsing/rhsNotUsing are the complement.
	var lhsUsing, rhsUsing []int
	// lhsUsed, rhsUsed are the set of column indexes in lhsUsing/rhsUsing.
	lhsUsed, rhsUsed := make(map[int]bool), make(map[int]bool)
	primaryLength, secondaryLength := len(lhsEC.cols), len(rhsEC.cols)
	ji.ec.cols = nil
	ji.ec.starCols = nil
	for _, id := range using {
		lhsi, err := lhsEC.resolveColumnIndex(id)
		if err != nil {
			return err
		}
		lhsUsing = append(lhsUsing, lhsi)
		lhsUsed[lhsi] = true

		rhsi, err := rhsEC.resolveColumnIndex(id)
		if err != nil {
			return err
		}
		rhsUsing = append(rhsUsing, rhsi)
		rhsUsed[rhsi] = true
		ji.ec.starCols = append(ji.ec.starCols, len(ji.ec.cols))
		col := lhsEC.cols[lhsi]
		if flipped {
			col = rhsEC.cols[rhsi]
		}
		col.Alias = nil
		col.AggIndex = 0
		ji.ec.cols = append(ji.ec.cols, col)
	}

	for i := range lhsEC.cols {
		if !lhsUsed[i] {
			ji.ec.starCols = append(ji.ec.starCols, len(ji.ec.cols))
		}
		ji.ec.cols = append(ji.ec.cols, lhsEC.cols[i])
	}
	for i := range rhsEC.cols {
		if !rhsUsed[i] {
			ji.ec.starCols = append(ji.ec.starCols, len(ji.ec.cols))
		}
		ji.ec.cols = append(ji.ec.cols, rhsEC.cols[i])
	}

	primaryUsing, secondaryUsing := lhsUsing, rhsUsing
	primaryUsed, secondaryUsed := lhsUsed, rhsUsed
	if flipped {
		primaryUsing, secondaryUsing = secondaryUsing, primaryUsing
		primaryUsed, secondaryUsed = secondaryUsed, primaryUsed
	}

	ji.ec.row = make(row, len(ji.ec.cols))

	orNil := func(r row, i int) interface{} {
		if len(r) == 0 {
			return nil
		}
		return r[i]
	}

	// populate writes the data to ji.ec.row in the correct positions.
	populate := func(primary, secondary row) { // either may be nil
		j := 0
		if primary != nil {
			for _, i := range primaryUsing {
				ji.ec.row[j] = orNil(primary, i)
				j++
			}
		} else {
			for _, i := range secondaryUsing {
				ji.ec.row[j] = orNil(secondary, i)
				j++
			}
		}
		if flipped {
			primary, secondary = secondary, primary
		}
		for i := 0; i < primaryLength; i++ {
			ji.ec.row[j] = orNil(primary, i)
			j++
		}
		for i := 0; i < secondaryLength; i++ {
			ji.ec.row[j] = orNil(secondary, i)
			j++
		}
	}
	ji.cond = func(primary, secondary row) (bool, error) {
		for i, pi := range primaryUsing {
			si := secondaryUsing[i]
			if compareVals(primary[pi], secondary[si]) != 0 {
				return false, nil
			}
		}
		populate(primary, secondary)
		return true, nil
	}
	ji.zero = func(primary, secondary row) {
		populate(primary, secondary)
	}
	return nil
}

type joinIter struct {
	jt spansql.JoinType
	ec evalContext // combined context

	// The "primary" is scanned (consumed), but the secondary is cloned for each primary row.
	// Most join types have primary==LHS; a RIGHT JOIN is the exception.
	primary, secondaryOrig *rawIter

	// The offsets into ec.row that the primary/secondary rows should appear
	// in the final output. Not used when there's a USING clause.
	primaryOffset, secondaryOffset int
	// nullPad is whether primary rows without matching secondary rows
	// should be yielded with null padding (e.g. OUTER JOINs).
	nullPad bool

	primaryRow    row      // current row from primary, or nil if it is time to advance
	secondary     *rawIter // current clone of secondary
	secondaryRead int      // number of rows already read from secondary
	any           bool     // true if any secondary rows have matched primaryRow

	// cond reports whether the primary and secondary rows "join" (e.g. the ON clause is true).
	// It populates ec.row with the output.
	cond func(primary, secondary row) (bool, error)
	// zero populates ec.row with the primary or secondary row data (either of which may be nil),
	// and sets the remainder to NULL.
	// This is used when nullPad is true and a primary or secondary row doesn't match.
	zero func(primary, secondary row)

	// For FULL JOIN, this tracks the secondary rows that have been used.
	// It is non-nil when being used.
	used        []bool
	zeroUnused  bool // set when emitting unused secondary rows
	unusedIndex int  // next index of used to check
}

func (ji *joinIter) Cols() []colInfo { return ji.ec.cols }

func (ji *joinIter) nextPrimary() error {
	var err error
	ji.primaryRow, err = ji.primary.Next()
	if err != nil {
		return err
	}
	ji.secondary = ji.secondaryOrig.clone()
	ji.secondaryRead = 0
	ji.any = false
	return nil
}

func (ji *joinIter) Next() (row, error) {
	if ji.primaryRow == nil && !ji.zeroUnused {
		err := ji.nextPrimary()
		if err == io.EOF && ji.used != nil {
			// Drop down to emitting unused secondary rows.
			ji.zeroUnused = true
			ji.secondary = nil
			goto scanJiUsed
		}
		if err != nil {
			return nil, err
		}
	}
scanJiUsed:
	if ji.zeroUnused {
		if ji.secondary == nil {
			ji.secondary = ji.secondaryOrig.clone()
			ji.secondaryRead = 0
		}
		for ji.unusedIndex < len(ji.used) && ji.used[ji.unusedIndex] {
			ji.unusedIndex++
		}
		if ji.unusedIndex >= len(ji.used) || ji.secondaryRead >= len(ji.used) {
			// Truly finished.
			return nil, io.EOF
		}
		var secondaryRow row
		for ji.secondaryRead <= ji.unusedIndex {
			var err error
			secondaryRow, err = ji.secondary.Next()
			if err != nil {
				return nil, err
			}
			ji.secondaryRead++
		}
		ji.zero(nil, secondaryRow)
		return ji.ec.row, nil
	}

	for {
		secondaryRow, err := ji.secondary.Next()
		if err == io.EOF {
			// Finished the current primary row.

			if !ji.any && ji.nullPad {
				ji.zero(ji.primaryRow, nil)
				ji.primaryRow = nil
				return ji.ec.row, nil
			}

			// Advance to next one.
			err := ji.nextPrimary()
			if err == io.EOF && ji.used != nil {
				ji.zeroUnused = true
				ji.secondary = nil
				goto scanJiUsed
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		ji.secondaryRead++
		if ji.used != nil {
			for len(ji.used) < ji.secondaryRead {
				ji.used = append(ji.used, false)
			}
		}

		// We have a pair of rows to consider.
		match, err := ji.cond(ji.primaryRow, secondaryRow)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}
		ji.any = true
		if ji.used != nil {
			// Make a note that we used this secondary row.
			ji.used[ji.secondaryRead-1] = true
		}
		return ji.ec.row, nil
	}
}

func evalSelectOrder(si *selIter, aux []spansql.Expr) (rows []row, keys [][]interface{}, err error) {
	// This is like toRawIter except it also evaluates the auxiliary expressions for ORDER BY.
	for {
		r, err := si.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, err
		}
		key, err := si.ec.evalExprList(aux)
		if err != nil {
			return nil, nil, err
		}

		rows = append(rows, r.copyAllData())
		keys = append(keys, key)
	}
	return
}

// externalRowSorter implements sort.Interface for a slice of rows
// with an external sort key.
type externalRowSorter struct {
	rows []row
	keys [][]interface{}
	desc []bool // may be nil
}

func (ers externalRowSorter) Len() int { return len(ers.rows) }
func (ers externalRowSorter) Less(i, j int) bool {
	return compareValLists(ers.keys[i], ers.keys[j], ers.desc) < 0
}
func (ers externalRowSorter) Swap(i, j int) {
	ers.rows[i], ers.rows[j] = ers.rows[j], ers.rows[i]
	ers.keys[i], ers.keys[j] = ers.keys[j], ers.keys[i]
}
