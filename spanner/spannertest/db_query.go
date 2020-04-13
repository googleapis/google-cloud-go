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
	FROM + JOIN + set ops [TODO: JOIN and set ops]
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
}

func (ti *tableIter) Cols() []colInfo { return ti.t.cols }
func (ti *tableIter) Next() (row, error) {
	if ti.rowIndex >= len(ti.t.rows) {
		return nil, io.EOF
	}
	res := ti.t.rows[ti.rowIndex]
	ti.rowIndex++
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
		if !b {
			continue
		}
		return row, nil
	}
}

// selIter applies a SELECT list.
type selIter struct {
	ri   rowIter
	ec   evalContext
	cis  []colInfo
	list []spansql.Expr
}

func (si selIter) Cols() []colInfo { return si.cis }
func (si selIter) Next() (row, error) {
	r, err := si.ri.Next()
	if err != nil {
		return nil, err
	}
	si.ec.row = r

	var out row
	for _, e := range si.list {
		if e == spansql.Star {
			out = append(out, r...)
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

// distinctIter applies a DISTINCT filter.
type distinctIter struct {
	ri   rowIter
	seen []row
}

func (di *distinctIter) Cols() []colInfo { return di.ri.Cols() }
func (di *distinctIter) Next() (row, error) {
	// This is hilariously inefficient; O(N^2) in the number of returned rows.
	// Some sort of hashing could be done to deduplicate instead.
	// This also breaks on array/struct types.
	for {
		row, err := di.ri.Next()
		if err != nil {
			return nil, err
		}
		dupe := false
		for _, prev := range di.seen {
			if rowEqual(prev, row) {
				dupe = true
				break
			}
		}
		if dupe {
			continue
		}
		di.seen = append(di.seen, row)
		return row, nil
	}
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
	Value interface{}
	Type  spansql.Type
}

type queryParams map[string]queryParam

func (d *database) Query(q spansql.Query, params queryParams) (rowIter, error) {
	// If there's an ORDER BY clause, extend the query to include the expressions we need
	// so they get evaluated during evalSelect. TODO: Is this actually okay?
	var aux []spansql.Expr
	var desc []bool
	for _, o := range q.Order {
		aux = append(aux, o.Expr)
		desc = append(desc, o.Desc)
	}
	q.Select.List = append(q.Select.List, aux...)

	ri, err := d.evalSelect(q.Select, params)
	if err != nil {
		return nil, err
	}

	// Apply ORDER BY.
	if len(q.Order) > 0 {
		raw, err := toRawIter(ri)
		if err != nil {
			return nil, err
		}
		sort.Slice(raw.rows, func(one, two int) bool {
			r1, r2 := raw.rows[one], raw.rows[two]
			aux1, aux2 := r1[len(r1)-len(aux):], r2[len(r2)-len(aux):] // sort keys
			return compareValLists(aux1, aux2, desc) < 0
		})
		// Remove ORDER BY values.
		raw.cols = raw.cols[:len(raw.cols)-len(aux)]
		for i, row := range raw.rows {
			raw.rows[i] = row[:len(row)-len(aux)]
		}
		ri = raw
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

func (d *database) evalSelect(sel spansql.Select, params queryParams) (ri rowIter, evalErr error) {
	ri = &nullIter{}
	ec := evalContext{
		params: params,
	}

	// First stage is to identify the data source.
	// If there's a FROM then that names a table to use.
	if len(sel.From) > 1 {
		return nil, fmt.Errorf("selecting from more than one table not yet supported")
	}
	if len(sel.From) == 1 {
		tableName := sel.From[0].Table
		t, err := d.table(tableName)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		ri = &tableIter{t: t}
		ec.cols = t.cols

		// On the way out, convert the result to a rawIter
		// so that the table may be safely unlocked.
		defer func() {
			if evalErr == nil {
				ri, evalErr = toRawIter(ri)
			}
		}()
	}

	// Apply WHERE.
	if sel.Where != nil {
		ri = whereIter{
			ri:    ri,
			ec:    ec,
			where: sel.Where,
		}
	}

	// Apply GROUP BY.
	// This only reorders rows to group rows together;
	// aggregation happens next.
	var rowGroups [][2]int // Sequence of half-open intervals of row numbers.
	if len(sel.GroupBy) > 0 {
		// Load aliases visible to this GROUP BY.
		ec.aliases = make(map[string]spansql.Expr)
		for i, alias := range sel.ListAliases {
			ec.aliases[alias] = sel.List[i]
		}
		// TODO: Add aliases for "1", "2", etc.

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
		if len(rowGroups) == 0 {
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
				return nil, err
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
			Name:     fexpr.SQL(),
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
			colInfos = append(colInfos, ec.cols...)
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
	ri = selIter{
		ri:   ri,
		ec:   ec,
		cis:  colInfos,
		list: sel.List,
	}

	// Apply DISTINCT.
	if sel.Distinct {
		ri = &distinctIter{ri: ri}
	}

	return ri, nil
}

// externalRowSorter implements sort.Interface for a slice of rows
// with an external sort key.
type externalRowSorter struct {
	rows []row
	keys [][]interface{}
}

func (ers externalRowSorter) Len() int { return len(ers.rows) }
func (ers externalRowSorter) Less(i, j int) bool {
	return compareValLists(ers.keys[i], ers.keys[j], nil) < 0
}
func (ers externalRowSorter) Swap(i, j int) {
	ers.rows[i], ers.rows[j] = ers.rows[j], ers.rows[i]
	ers.keys[i], ers.keys[j] = ers.keys[j], ers.keys[i]
}
